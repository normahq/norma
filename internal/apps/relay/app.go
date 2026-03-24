package relay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/handlers"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/metalagman/norma/internal/apps/relaymcp"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/runtime"
	"go.uber.org/fx"
)

// App creates a new fx.App for the relay bot with the provided configuration.
func App(cfg Config, normaCfg config.Config) *fx.App {
	return fx.New(
		fx.WithLogger(
			fxlogger.WithZerolog(
				log.Logger.With().Str("component", "relay").Logger(),
			),
		),
		Module(cfg, normaCfg),
	)
}

// Module returns the fx.Module for the relay bot, initialized with the provided configurations.
func Module(cfg Config, normaCfg config.Config) fx.Option {
	// Convert relay config to tgbotkit config.
	tgbotkitCfg := tgbotkit.Config{
		Token:        cfg.Relay.Telegram.Token,
		WebhookToken: cfg.Relay.Telegram.WebhookToken,
		WebhookURL:   cfg.Relay.Telegram.WebhookURL,
		ReceiverMode: cfg.Relay.Telegram.ReceiverMode,
	}

	// Create logger.
	logger := log.Logger.With().Str("component", "relay").Logger()

	// Determine working directory from config or fallback to current directory.
	workingDir := cfg.Relay.WorkingDir
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	return fx.Module("relay",
		fx.Supply(
			tgbotkitCfg,
			logger,
			normaCfg,
			workingDir,
		),
		fx.Provide(
			fx.Annotate(
				func() []string { return append([]string(nil), cfg.Relay.InternalMCP.Servers...) },
				fx.ResultTags(`name:"relay_internal_mcp_servers"`),
			),
		),
		// Provide auth token with named injection.
		fx.Provide(
			fx.Annotate(
				func() string { return cfg.Relay.Auth.OwnerToken },
				fx.ResultTags(`name:"relay_auth_token"`),
			),
		),
		// Provide owner store.
		fx.Provide(func() (*auth.OwnerStore, error) {
			normaDir := filepath.Join(workingDir, ".norma")
			// Ensure norma directory exists.
			if err := os.MkdirAll(normaDir, 0755); err != nil {
				return nil, fmt.Errorf("creating norma dir: %w", err)
			}
			return auth.NewOwnerStore(normaDir)
		}),
		// Provide agent factory.
		fx.Provide(func(normaCfg config.Config) *agentfactory.Factory {
			return agentfactory.NewFactoryWithMCPServers(normaCfg.Agents, normaCfg.MCPServers)
		}),
		tgbotkit.Module,
		handlers.Module,
		fx.Provide(
			handlers.NewInternalMCPManager,
			handlers.NewRelayAgentManager,
		),
		// Start Telegram runtime only after internal MCP + relay agent are started.
		fx.Invoke(func(lc fx.Lifecycle, bot *runtime.Bot, _ *handlers.InternalMCPManager, _ *handlers.RelayAgentManager) {
			runCtx, cancel := context.WithCancel(context.Background())
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					go func() {
						if err := bot.Run(runCtx); err != nil {
							bot.Logger().Errorf("bot run failed: %v", err)
						}
					}()
					return nil
				},
				OnStop: func(ctx context.Context) error {
					cancel()
					return nil
				},
			})
		}),
		// Start TCP MCP server if configured
		fx.Invoke(func(lc fx.Lifecycle, sessionManager *handlers.TopicSessionManager) {
			if cfg.Relay.MCP.Address != "" {
				lc.Append(fx.Hook{
					OnStart: func(ctx context.Context) error {
						go func() {
							svc := handlers.NewRelayMCPServer(sessionManager)
							if err := relaymcp.RunHTTP(ctx, svc, cfg.Relay.MCP.Address); err != nil {
								log.Error().Err(err).Str("address", cfg.Relay.MCP.Address).Msg("MCP server error")
							}
						}()
						log.Info().Str("address", cfg.Relay.MCP.Address).Msg("Relay MCP server started")
						return nil
					},
				})
			}
		}),
	)
}
