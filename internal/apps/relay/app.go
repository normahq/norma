package relay

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	acp "github.com/coder/acp-go-sdk"
	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/handlers"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/metalagman/norma/internal/apps/relaymcp"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
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
		// Provide session service.
		fx.Provide(session.InMemoryService),
		// Provide agent factory.
		fx.Provide(func(normaCfg config.Config) *agentfactory.Factory {
			return agentfactory.NewFactoryWithMCPServers(normaCfg.Agents, normaCfg.MCPServers)
		}),
		// Provide relay agent.
		fx.Provide(func(lc fx.Lifecycle, factory *agentfactory.Factory, normaCfg config.Config, workingDir string) (agent.Agent, error) {
			profileName := normaCfg.Profile
			if profileName == "" {
				profileName = "default"
			}
			var agentName string
			if profile, ok := normaCfg.Profiles[profileName]; ok {
				agentName = profile.Relay
			}
			if agentName == "" {
				return nil, fmt.Errorf("no relay agent configured in profile %q", profileName)
			}

			req := agentfactory.CreationRequest{
				Name:             agentName,
				WorkingDirectory: workingDir,
				Stderr:           os.Stderr,
				Logger:           &log.Logger,
				PermissionHandler: func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
					if len(req.Options) > 0 {
						return acp.RequestPermissionResponse{
							Outcome: acp.NewRequestPermissionOutcomeSelected(req.Options[0].OptionId),
						}, nil
					}
					return acp.RequestPermissionResponse{
						Outcome: acp.NewRequestPermissionOutcomeCancelled(),
					}, nil
				},
			}

			ag, err := factory.CreateAgent(context.Background(), agentName, req)
			if err != nil {
				return nil, fmt.Errorf("creating relay agent: %w", err)
			}

			lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					if closer, ok := ag.(io.Closer); ok {
						return closer.Close()
					}
					return nil
				},
			})

			return ag, nil
		}),
		tgbotkit.Module,
		handlers.Module,
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
