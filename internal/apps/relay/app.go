package relay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/handlers"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
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

	logger := log.Logger.With().Str("component", "relay").Logger()

	workingDir := cfg.Relay.WorkingDir
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	// Start with global MCP servers.
	mcpServers := make(map[string]agentconfig.MCPServerConfig, len(normaCfg.MCPServers))
	for k, v := range normaCfg.MCPServers {
		mcpServers[k] = v
	}

	// Add relay MCP server to factory if address is configured.
	// The actual server will be started by InternalMCPManager.
	if cfg.Relay.MCP.Address != "" {
		mcpServers["norma.relay"] = agentconfig.MCPServerConfig{
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  fmt.Sprintf("http://%s/mcp", cfg.Relay.MCP.Address),
		}
	}

	return fx.Module("relay",
		fx.Supply(
			tgbotkitCfg,
			logger,
			normaCfg,
			workingDir,
			mcpServers,
		),
		fx.Provide(
			fx.Annotate(
				func() string { return cfg.Relay.MCP.Address },
				fx.ResultTags(`name:"relay_mcp_addr"`),
			),
		),
		fx.Provide(
			fx.Annotate(
				func() []string { return append([]string(nil), cfg.Relay.InternalMCP.Servers...) },
				fx.ResultTags(`name:"relay_internal_mcp_servers"`),
			),
		),
		fx.Provide(
			fx.Annotate(
				func() string { return cfg.Relay.Auth.OwnerToken },
				fx.ResultTags(`name:"relay_auth_token"`),
			),
		),
		fx.Provide(
			fx.Annotate(
				func() map[string]interface{} {
					// Convert to interface{} to avoid import in session manager
					m := make(map[string]interface{})
					for k, v := range normaCfg.Agents {
						m[k] = v
					}
					return m
				},
				fx.ResultTags(`name:"relay_agent_configs"`),
			),
		),
		fx.Provide(
			fx.Annotate(
				func() string {
					_, profile, err := normaCfg.ResolveProfile("")
					if err != nil {
						return ""
					}
					return profile.Relay
				},
				fx.ResultTags(`name:"relay_agent_name"`),
			),
		),
		fx.Provide(func() (*auth.OwnerStore, error) {
			normaDir := filepath.Join(workingDir, ".norma")
			if err := os.MkdirAll(normaDir, 0755); err != nil {
				return nil, fmt.Errorf("creating norma dir: %w", err)
			}
			return auth.NewOwnerStore(normaDir)
		}),
		fx.Provide(func(mcpServers map[string]agentconfig.MCPServerConfig) *agentfactory.Factory {
			return agentfactory.NewFactoryWithMCPServers(normaCfg.Agents, mcpServers)
		}),
		tgbotkit.Module,
		handlers.Module,
		fx.Provide(
			handlers.NewInternalMCPManager,
		),
		// Start Telegram runtime only after internal MCP is started.
		fx.Invoke(func(lc fx.Lifecycle, bot *runtime.Bot, _ *handlers.InternalMCPManager) {
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
	)
}
