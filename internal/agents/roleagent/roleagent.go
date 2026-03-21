// Package roleagent provides a role-agnostic ADK agent factory.
// It creates structured I/O agents from generic configuration:
// name, system instruction, input schema, output schema.
package roleagent

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/adk/structuredio"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"google.golang.org/adk/agent"
)

// Config holds the configuration for creating a role-agnostic structured agent.
type Config struct {
	// Name is the agent name (e.g. "plan", "do", "check", "act").
	Name string

	// SystemInstruction is the full system prompt for the agent.
	SystemInstruction string

	// InputSchema is the JSON schema for validating agent input.
	InputSchema string

	// OutputSchema is the JSON schema for validating agent output.
	OutputSchema string

	// AgentConfig is the underlying agent implementation config (e.g. ACP).
	AgentConfig agentconfig.Config

	// MCPServers are optional MCP server configurations.
	MCPServers map[string]agentconfig.MCPServerConfig

	// WorkingDir is the working directory for the agent.
	WorkingDir string

	// Stdout is the writer for agent stdout.
	Stdout io.Writer

	// Stderr is the writer for agent stderr.
	Stderr io.Writer

	// Logger is an optional logger. Defaults to the global logger.
	Logger *zerolog.Logger

	// PermissionHandler is an optional ACP permission handler.
	PermissionHandler func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
}

// New creates an ADK agent wrapped with structured I/O validation.
// The returned agent validates input against InputSchema and output against OutputSchema.
func New(ctx context.Context, cfg Config) (agent.Agent, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		return nil, fmt.Errorf("working directory is required")
	}
	if strings.TrimSpace(cfg.SystemInstruction) == "" {
		return nil, fmt.Errorf("system instruction is required")
	}

	l := cfg.Logger
	if l == nil {
		logger := log.With().Str("agent", cfg.Name).Logger()
		l = &logger
	}

	permHandler := cfg.PermissionHandler
	if permHandler == nil {
		permHandler = defaultPermissionHandler
	}

	agentRegistry := map[string]agentconfig.Config{
		cfg.Name: cfg.AgentConfig,
	}
	factory := agentfactory.NewFactory(agentRegistry)
	if len(cfg.MCPServers) > 0 {
		factory = agentfactory.NewFactoryWithMCPServers(agentRegistry, cfg.MCPServers)
	}

	creationReq := agentfactory.CreationRequest{
		Name:              "Norma" + toPascal(cfg.Name) + "Agent",
		Description:       "Norma " + cfg.Name + " agent",
		SystemInstruction: cfg.SystemInstruction,
		WorkingDirectory:  cfg.WorkingDir,
		Stdout:            cfg.Stdout,
		Stderr:            cfg.Stderr,
		Logger:            l,
		PermissionHandler: permHandler,
	}

	inner, err := factory.CreateAgent(ctx, cfg.Name, creationReq)
	if err != nil {
		return nil, fmt.Errorf("create inner agent: %w", err)
	}

	wrapped, err := structuredio.NewAgent(inner,
		structuredio.WithInputSchema(cfg.InputSchema),
		structuredio.WithOutputSchema(cfg.OutputSchema),
	)
	if err != nil {
		if closer, ok := inner.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("create structured wrapper: %w", err)
	}

	return wrapped, nil
}

func toPascal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func defaultPermissionHandler(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindAllowOnce || option.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindRejectOnce || option.Kind == acp.PermissionOptionKindRejectAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}
