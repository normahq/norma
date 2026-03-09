// Package agentfactory provides a registry and factory for creating ADK-compatible agents.
package agentfactory

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/metalagman/ainvoke/adk"
	"github.com/metalagman/norma/internal/adk/acpagent"
	"github.com/metalagman/norma/internal/config"
	"google.golang.org/adk/agent"
)

// CreationRequest defines the parameters for creating a new agent.
type CreationRequest struct {
	Name              string
	Description       string
	Prompt            string
	SystemPrompt      string
	InputSchema       string
	OutputSchema      string
	WorkingDir        string
	RunDir            string
	Stdout            io.Writer
	Stderr            io.Writer
	PermissionHandler func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
}

// constructor is a function that creates a new agent instance.
type constructor func(ctx context.Context, cfg config.AgentConfig, req CreationRequest) (agent.Agent, error)

// Factory is a registry of agent configurations.
type Factory struct {
	registry map[string]config.AgentConfig
}

// NewFactory creates a new Factory from a map of agent configurations.
func NewFactory(agents map[string]config.AgentConfig) *Factory {
	return &Factory{
		registry: agents,
	}
}

// CreateAgent creates an agent.Agent instance by name and creation request.
// It returns an error if the agent is not found or its type is unsupported.
func (f *Factory) CreateAgent(ctx context.Context, name string, req CreationRequest) (agent.Agent, error) {
	cfg, ok := f.registry[name]
	if !ok {
		return nil, fmt.Errorf("agent %q not found or unsupported", name)
	}

	create, ok := constructors[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported agent type %q for agent %q", cfg.Type, name)
	}

	ag, err := create(ctx, cfg, req)
	if err != nil {
		return nil, fmt.Errorf("create agent %q: %w", name, err)
	}

	return ag, nil
}

// constructors registry.
var constructors = map[string]constructor{
	config.AgentTypeExec: func(ctx context.Context, cfg config.AgentConfig, req CreationRequest) (agent.Agent, error) {
		cmd, err := ResolveCmd(cfg)
		if err != nil {
			return nil, err
		}
		fullPrompt := req.Prompt
		if req.SystemPrompt != "" {
			fullPrompt = req.SystemPrompt + "\n\n" + req.Prompt
		}
		return adk.NewExecAgent(
			req.Name,
			req.Description,
			cmd,
			adk.WithExecAgentPrompt(fullPrompt),
			adk.WithExecAgentInputSchema(req.InputSchema),
			adk.WithExecAgentOutputSchema(req.OutputSchema),
			adk.WithExecAgentRunDir(req.RunDir),
			adk.WithExecAgentUseTTY(cfg.UseTTY != nil && *cfg.UseTTY),
			adk.WithExecAgentStdout(req.Stdout),
			adk.WithExecAgentStderr(req.Stderr),
		)
	},
	config.AgentTypeClaude:  execConstructor,
	config.AgentTypeCodex:   execConstructor,
	config.AgentTypeGemini:  execConstructor,
	config.AgentTypeOpenCode: execConstructor,

	config.AgentTypeACPExec:      acpConstructor,
	config.AgentTypeGeminiACP:    acpConstructor,
	config.AgentTypeOpenCodeACP:  acpConstructor,
	config.AgentTypeCodexACP:     acpConstructor,
}

var execConstructor = func(ctx context.Context, cfg config.AgentConfig, req CreationRequest) (agent.Agent, error) {
	cmd, err := ResolveCmd(cfg)
	if err != nil {
		return nil, err
	}
	fullPrompt := req.Prompt
	if req.SystemPrompt != "" {
		fullPrompt = req.SystemPrompt + "\n\n" + req.Prompt
	}

	return adk.NewExecAgent(
		req.Name,
		req.Description,
		cmd,
		adk.WithExecAgentPrompt(fullPrompt),
		adk.WithExecAgentInputSchema(req.InputSchema),
		adk.WithExecAgentOutputSchema(req.OutputSchema),
		adk.WithExecAgentRunDir(req.RunDir),
		adk.WithExecAgentUseTTY(cfg.UseTTY != nil && *cfg.UseTTY),
		adk.WithExecAgentStdout(req.Stdout),
		adk.WithExecAgentStderr(req.Stderr),
	)
}

var acpConstructor = func(ctx context.Context, cfg config.AgentConfig, req CreationRequest) (agent.Agent, error) {
	cmd, err := ResolveACPCommand(cfg)
	if err != nil {
		return nil, err
	}
	workingDir := req.WorkingDir
	if strings.TrimSpace(workingDir) == "" {
		workingDir = req.RunDir
	}

	return acpagent.New(acpagent.Config{
		Context:           ctx,
		Name:              req.Name,
		Description:       req.Description,
		Model:             cfg.Model,
		SystemPrompt:      req.SystemPrompt,
		Command:           cmd,
		WorkingDir:        workingDir,
		Stderr:            req.Stderr,
		PermissionHandler: req.PermissionHandler,
		HasSetModel:       config.HasSetModelSupport(cfg.Type),
	})
}

// ResolveCmd resolves the command for an agent config.
func ResolveCmd(cfg config.AgentConfig) ([]string, error) {
	cmd := cfg.Cmd
	if len(cmd) == 0 {
		switch cfg.Type {
		case config.AgentTypeExec:
			return nil, fmt.Errorf("exec agent requires cmd")
		case config.AgentTypeClaude:
			cmd = []string{"claude"}
			if cfg.Model != "" {
				cmd = append(cmd, "--model", cfg.Model)
			}
		case config.AgentTypeCodex:
			cmd = []string{"codex", "exec"}
			if cfg.Model != "" {
				cmd = append(cmd, "--model", cfg.Model)
			}
			cmd = append(cmd, "--sandbox", "workspace-write")
		case config.AgentTypeGemini:
			cmd = []string{"gemini"}
			if cfg.Model != "" {
				cmd = append(cmd, "--model", cfg.Model)
			}
			cmd = append(cmd, "--approval-mode", "yolo")
		case config.AgentTypeOpenCode:
			cmd = []string{"opencode", "run"}
			if cfg.Model != "" {
				cmd = append(cmd, "--model", cfg.Model)
			}
		default:
			return nil, fmt.Errorf("unknown agent type %q", cfg.Type)
		}
	}
	res := resolveTemplatedCmd(cmd, cfg.Model)
	if len(cfg.ExtraArgs) > 0 {
		res = append(res, cfg.ExtraArgs...)
	}
	return res, nil
}

// ResolveACPCommand resolves the command for ACP-backed agent types.
func ResolveACPCommand(cfg config.AgentConfig) ([]string, error) {
	var cmd []string
	switch cfg.Type {
	case config.AgentTypeACPExec:
		if len(cfg.Cmd) == 0 {
			return nil, fmt.Errorf("acp_exec agent requires cmd")
		}
		cmd = cfg.Cmd
	case config.AgentTypeGeminiACP:
		cmd = []string{"gemini", "--experimental-acp"}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
	case config.AgentTypeOpenCodeACP:
		cmd = []string{"opencode", "acp"}
	case config.AgentTypeCodexACP:
		exePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable path: %w", err)
		}
		cmd = []string{exePath, "proxy", "codex-acp"}
		if cfg.Model != "" {
			cmd = append(cmd, "--model", cfg.Model)
		}
	default:
		return nil, fmt.Errorf("unknown acp agent type %q", cfg.Type)
	}
	res := resolveTemplatedCmd(cmd, cfg.Model)
	if len(cfg.ExtraArgs) > 0 {
		if cfg.Type == config.AgentTypeCodexACP {
			res = append(res, "--")
		}
		res = append(res, cfg.ExtraArgs...)
	}
	return res, nil
}

func resolveTemplatedCmd(cmd []string, model string) []string {
	if len(cmd) == 0 {
		return nil
	}
	res := make([]string, len(cmd))
	for i, arg := range cmd {
		res[i] = strings.ReplaceAll(arg, "{{.Model}}", model)
	}
	return res
}
