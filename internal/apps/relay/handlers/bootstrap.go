package handlers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
	"google.golang.org/adk/agent"
)

// InternalMCPManager controls startup/shutdown of internal MCP servers configured for relay.
type InternalMCPManager struct {
	serverIDs []string
	started   bool
	mu        sync.RWMutex
	logger    zerolog.Logger
}

type internalMCPParams struct {
	fx.In

	LC        fx.Lifecycle
	ServerIDs []string `name:"relay_internal_mcp_servers"`
	Logger    zerolog.Logger
}

// NewInternalMCPManager creates an internal MCP lifecycle manager.
func NewInternalMCPManager(params internalMCPParams) *InternalMCPManager {
	manager := &InternalMCPManager{
		serverIDs: append([]string(nil), params.ServerIDs...),
		logger:    params.Logger.With().Str("component", "relay.internal_mcp").Logger(),
	}

	params.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			manager.logger.Info().Int("servers", len(manager.serverIDs)).Msg("starting internal MCP servers")
			for _, id := range manager.serverIDs {
				serverID := strings.TrimSpace(id)
				if serverID == "" {
					continue
				}
				// Server process wiring will be added incrementally; this hook guarantees lifecycle ordering.
				manager.logger.Info().Str("server_id", serverID).Msg("internal MCP server configured")
			}
			manager.mu.Lock()
			manager.started = true
			manager.mu.Unlock()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			manager.mu.Lock()
			manager.started = false
			manager.mu.Unlock()
			return nil
		},
	})

	return manager
}

// Started reports whether internal MCP startup hook has completed.
func (m *InternalMCPManager) Started() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// RelayAgentManager owns lifecycle of the main relay orchestrator agent.
type RelayAgentManager struct {
	factory      *agentfactory.Factory
	normaCfg     config.Config
	workingDir   string
	logger       zerolog.Logger
	workspaceDir string

	mu        sync.RWMutex
	agentName string
	agent     agent.Agent
}

type relayAgentParams struct {
	fx.In

	LC          fx.Lifecycle
	Factory     *agentfactory.Factory
	NormaCfg    config.Config
	WorkingDir  string
	Logger      zerolog.Logger
	InternalMCP *InternalMCPManager
}

// NewRelayAgentManager creates lifecycle-managed relay orchestrator agent holder.
func NewRelayAgentManager(params relayAgentParams) *RelayAgentManager {
	manager := &RelayAgentManager{
		factory:    params.Factory,
		normaCfg:   params.NormaCfg,
		workingDir: params.WorkingDir,
		logger:     params.Logger.With().Str("component", "relay.agent_manager").Logger(),
	}

	params.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			agentName, err := resolveRelayAgentName(params.NormaCfg)
			if err != nil {
				return err
			}

			workspaceDir, err := manager.ensureWorkspace(ctx)
			if err != nil {
				return fmt.Errorf("create relay workspace: %w", err)
			}
			manager.workspaceDir = workspaceDir

			req := agentfactory.CreationRequest{
				Name:              agentName,
				WorkingDirectory:  workspaceDir,
				Stderr:            os.Stderr,
				Logger:            &manager.logger,
				PermissionHandler: defaultPermissionHandler,
			}
			ag, err := manager.factory.CreateAgent(ctx, agentName, req)
			if err != nil {
				return fmt.Errorf("creating relay agent %q: %w", agentName, err)
			}

			manager.mu.Lock()
			manager.agentName = agentName
			manager.agent = ag
			manager.mu.Unlock()
			manager.logger.Info().Str("agent", agentName).Msg("relay orchestrator agent started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			manager.mu.Lock()
			ag := manager.agent
			workspaceDir := manager.workspaceDir
			manager.agent = nil
			manager.workspaceDir = ""
			manager.mu.Unlock()

			if closer, ok := ag.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					manager.logger.Warn().Err(err).Msg("failed to close relay agent")
				}
			}

			if workspaceDir != "" {
				if err := manager.cleanupWorkspace(ctx, workspaceDir); err != nil {
					manager.logger.Warn().Err(err).Msg("failed to cleanup relay workspace")
				}
			}
			return nil
		},
	})

	return manager
}

// Agent returns the started relay orchestrator agent.
func (m *RelayAgentManager) Agent() (agent.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.agent == nil {
		return nil, fmt.Errorf("relay agent is not started")
	}
	return m.agent, nil
}

// WorkspaceDir returns the workspace directory for the relay agent.
func (m *RelayAgentManager) WorkspaceDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workspaceDir
}

func (m *RelayAgentManager) ensureWorkspace(ctx context.Context) (string, error) {
	relayDir := filepath.Join(m.workingDir, ".norma")
	workspacesDir := filepath.Join(relayDir, "relay-workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		return "", fmt.Errorf("create workspaces dir: %w", err)
	}

	workspaceDir := filepath.Join(workspacesDir, "main-relay")

	if fi, err := os.Stat(workspaceDir); err == nil && fi.IsDir() {
		return workspaceDir, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat workspace dir %q: %w", workspaceDir, err)
	}

	branchName := "norma/relay/main"
	if _, err := git.MountWorktree(ctx, m.workingDir, workspaceDir, branchName, "HEAD"); err != nil {
		return "", fmt.Errorf("mount worktree: %w", err)
	}

	return workspaceDir, nil
}

func (m *RelayAgentManager) cleanupWorkspace(ctx context.Context, workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}
	if err := git.RemoveWorktree(ctx, m.workingDir, workspaceDir); err != nil {
		m.logger.Warn().Err(err).Str("workspace", workspaceDir).Msg("failed to remove worktree")
		return err
	}
	return nil
}

func resolveRelayAgentName(cfg config.Config) (string, error) {
	profileName := strings.TrimSpace(cfg.Profile)
	if profileName == "" {
		profileName = "default"
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return "", fmt.Errorf("relay profile %q not found", profileName)
	}
	agentName := strings.TrimSpace(profile.Relay)
	if agentName == "" {
		return "", fmt.Errorf("no relay agent configured in profile %q", profileName)
	}
	if _, exists := cfg.Agents[agentName]; !exists {
		return "", fmt.Errorf("relay profile %q references undefined agent %q", profileName, agentName)
	}
	return agentName, nil
}
