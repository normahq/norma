package relay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipfans/fxlogger"
	"github.com/normahq/norma/internal/adk/agentconfig"
	"github.com/normahq/norma/internal/adk/agentfactory"
	"github.com/normahq/norma/internal/adk/mcpregistry"
	relayagent "github.com/normahq/norma/internal/apps/relay/agent"
	"github.com/normahq/norma/internal/apps/relay/auth"
	"github.com/normahq/norma/internal/apps/relay/handlers"
	"github.com/normahq/norma/internal/apps/relay/tgbotkit"
	"github.com/normahq/norma/internal/git"
	runtimeconfig "github.com/normahq/norma/pkg/runtime/config"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/runtime"
	"go.uber.org/fx"
)

// App creates a new fx.App for the relay bot with the provided configuration.
func App(cfg Config, normaCfg runtimeconfig.NormaConfig) *fx.App {
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
func Module(cfg Config, normaCfg runtimeconfig.NormaConfig) fx.Option {
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
	mcpReg := mcpregistry.New(mcpServers)
	// If relay MCP address is configured, pre-register the external relay endpoint.
	if cfg.Relay.MCP.Address != "" {
		mcpReg.Set("norma.relay", agentconfig.MCPServerConfig{
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  fmt.Sprintf("http://%s/mcp", cfg.Relay.MCP.Address),
		})
	}

	return fx.Module("relay",
		fx.Supply(
			tgbotkitCfg,
			logger,
			normaCfg,
			workingDir,
			mcpReg,
		),
		fx.Provide(
			fx.Annotate(
				func() (bool, error) {
					mode, enabled, err := ResolveWorkspaceEnabled(
						context.Background(),
						cfg.Relay.Workspace.Mode,
						workingDir,
						git.Available,
					)
					if err != nil {
						return false, err
					}
					logger.Info().
						Str("workspace_mode", string(mode)).
						Bool("workspace_enabled", enabled).
						Str("working_dir", workingDir).
						Msg("relay workspace mode resolved")
					return enabled, nil
				},
				fx.ResultTags(`name:"relay_workspace_enabled"`),
			),
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
				func() string {
					return cfg.Relay.OrchestratorAgent
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
		fx.Provide(func(reg *mcpregistry.MapRegistry) *agentfactory.Factory {
			return agentfactory.New(
				normaCfg.Agents,
				reg,
				agentfactory.WithPermissionHandler(relayagent.DefaultPermissionHandler),
			)
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
