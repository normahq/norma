package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/normahq/norma/internal/adk/agentconfig"
	"github.com/normahq/norma/internal/adk/mcpregistry"
	"github.com/normahq/norma/internal/apps/configmcp"
	"github.com/normahq/norma/internal/apps/relay/session"
	"github.com/normahq/norma/internal/apps/relaymcp"
	"github.com/normahq/norma/internal/apps/sessionmcp"
	"github.com/normahq/norma/internal/apps/workspacemcp"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// InternalMCPManager controls startup/shutdown of internal MCP servers configured for relay.
type InternalMCPManager struct {
	serverIDs        []string
	workspaceEnabled bool
	started          bool
	mu               sync.RWMutex
	logger           zerolog.Logger
	registry         mcpregistry.Registry
	workingDir       string
	sessionManager   *session.Manager
	cleanups         []func() error
}

type internalMCPParams struct {
	fx.In

	LC               fx.Lifecycle
	ServerIDs        []string `name:"relay_internal_mcp_servers"`
	WorkspaceEnabled bool     `name:"relay_workspace_enabled"`
	Logger           zerolog.Logger
	Registry         *mcpregistry.MapRegistry
	WorkingDir       string
	SessionManager   *session.Manager
	RelayMCPAddr     string `name:"relay_mcp_addr" optional:"true"`
}

// NewInternalMCPManager creates an internal MCP lifecycle manager.
func NewInternalMCPManager(params internalMCPParams) *InternalMCPManager {
	manager := &InternalMCPManager{
		serverIDs:        append([]string(nil), params.ServerIDs...),
		workspaceEnabled: params.WorkspaceEnabled,
		logger:           params.Logger.With().Str("component", "relay.internal_mcp").Logger(),
		registry:         params.Registry,
		workingDir:       params.WorkingDir,
		sessionManager:   params.SessionManager,
	}

	params.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			manager.logger.Info().Int("servers", len(manager.serverIDs)).Msg("starting internal MCP servers")

			// 1. Ensure bundled core servers are started if not already configured.
			if err := manager.ensureBundledServers(ctx, params.RelayMCPAddr); err != nil {
				return fmt.Errorf("ensuring bundled servers: %w", err)
			}

			// 2. Log any other configured internal servers.
			for _, id := range manager.serverIDs {
				serverID := strings.TrimSpace(id)
				if serverID == "" || isBundled(serverID) {
					continue
				}
				manager.logger.Info().Str("server_id", serverID).Msg("internal MCP server configured")
			}

			manager.mu.Lock()
			manager.started = true
			manager.mu.Unlock()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			manager.mu.Lock()
			defer manager.mu.Unlock()

			manager.logger.Info().Int("cleanups", len(manager.cleanups)).Msg("stopping internal MCP servers")
			for i := len(manager.cleanups) - 1; i >= 0; i-- {
				if err := manager.cleanups[i](); err != nil {
					manager.logger.Warn().Err(err).Msg("failed to stop internal MCP server")
				}
			}
			manager.cleanups = nil
			manager.started = false
			return nil
		},
	})

	return manager
}

func (m *InternalMCPManager) ensureBundledServers(ctx context.Context, relayMCPAddr string) error {
	// norma.config
	if _, ok := m.registry.Get("norma.config"); !ok {
		configPath := selectConfigPath(m.workingDir, "relay")
		svc, err := configmcp.NewConfigService(configPath)
		if err != nil {
			m.logger.Warn().Err(err).Msg("failed to create config service")
		} else {
			res, err := configmcp.StartHTTPServer(ctx, svc, "127.0.0.1:0")
			if err != nil {
				return fmt.Errorf("starting bundled config MCP: %w", err)
			}
			m.logger.Info().Str("addr", res.Addr).Msg("bundled norma.config server started")
			m.registry.Set("norma.config", agentconfig.MCPServerConfig{
				Type: agentconfig.MCPServerTypeHTTP,
				URL:  fmt.Sprintf("http://%s/mcp", res.Addr),
			})
			m.addCleanup(res.Close)
		}
	}

	// norma.state (session state)
	if _, ok := m.registry.Get("norma.state"); !ok {
		store := sessionmcp.NewMemoryStore()
		res, err := sessionmcp.StartHTTPServer(ctx, store, "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("starting bundled state MCP: %w", err)
		}
		m.logger.Info().Str("addr", res.Addr).Msg("bundled norma.state server started")
		m.registry.Set("norma.state", agentconfig.MCPServerConfig{
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  fmt.Sprintf("http://%s/mcp", res.Addr),
		})
		m.addCleanup(res.Close)
	}

	// norma.relay
	if _, ok := m.registry.Get("norma.relay"); !ok || relayMCPAddr != "" {
		// If it's already in factory but relayMCPAddr is set, it means it was configured in app.go
		// and we should start it on that address.
		// If it's NOT in factory, we start it on a random port.
		addr := relayMCPAddr
		if addr == "" {
			addr = "127.0.0.1:0"
		}

		svc := session.NewRelayMCPServer(m.sessionManager)
		res, err := relaymcp.StartHTTPServer(ctx, svc, addr)
		if err != nil {
			return fmt.Errorf("starting bundled relay MCP: %w", err)
		}
		m.logger.Info().Str("addr", res.Addr).Msg("bundled norma.relay server started")

		// Always update factory with actual address (especially if it was random)
		m.registry.Set("norma.relay", agentconfig.MCPServerConfig{
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  fmt.Sprintf("http://%s/mcp", res.Addr),
		})
		m.addCleanup(res.Close)
	}

	// norma.workspace
	if m.workspaceEnabled {
		if _, ok := m.registry.Get("norma.workspace"); !ok {
			svc := session.NewWorkspaceMCPServer(m.sessionManager)
			res, err := workspacemcp.StartHTTPServer(ctx, svc, "127.0.0.1:0")
			if err != nil {
				return fmt.Errorf("starting bundled workspace MCP: %w", err)
			}
			m.logger.Info().Str("addr", res.Addr).Msg("bundled norma.workspace server started")
			m.registry.Set("norma.workspace", agentconfig.MCPServerConfig{
				Type: agentconfig.MCPServerTypeHTTP,
				URL:  fmt.Sprintf("http://%s/mcp", res.Addr),
			})
			m.addCleanup(res.Close)
		}
	} else {
		m.logger.Info().Msg("workspace mode disabled; skipping bundled norma.workspace server")
	}

	return nil
}

func selectConfigPath(workDir, appName string) string {
	appPath := filepath.Join(workDir, ".norma", appName+".yaml")
	if info, err := os.Stat(appPath); err == nil && !info.IsDir() {
		return appPath
	}
	return filepath.Join(workDir, ".norma", "config.yaml")
}

func (m *InternalMCPManager) addCleanup(f func() error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanups = append(m.cleanups, f)
}

func isBundled(id string) bool {
	switch id {
	case "norma.config", "norma.state", "norma.relay", "norma.workspace":
		return true
	default:
		return false
	}
}

// Started reports whether internal MCP startup hook has completed.
func (m *InternalMCPManager) Started() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}
