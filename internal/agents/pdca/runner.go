package pdca

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/agents/roleagent"
	"github.com/metalagman/norma/internal/config"
)

type Runner interface {
	Run(ctx context.Context, req []byte, stdout, stderr, eventsLog io.Writer) (outBytes, errBytes []byte, exitCode int, err error)
}

func NewRunner(cfg config.AgentConfig, role roleagent.RoleContract, mcpServers map[string]agentconfig.MCPServerConfig) (Runner, error) {
	executorCfg := roleagent.ExecutorConfig{
		AgentConfig: cfg,
		MCPServers:  mcpServers,
	}
	return &adkRunner{
		cfg:        cfg,
		role:       role,
		mcpServers: mcpServers,
		executor:   roleagent.NewExecutor(executorCfg),
	}, nil
}

type adkRunner struct {
	cfg        config.AgentConfig
	role       roleagent.RoleContract
	mcpServers map[string]agentconfig.MCPServerConfig
	executor   *roleagent.Executor
}

type requestFields struct {
	Run struct {
		ID        string `json:"id"`
		Iteration int    `json:"iteration"`
	} `json:"run"`
	Step struct {
		Index int    `json:"index"`
		Name  string `json:"name"`
	} `json:"step"`
	Paths struct {
		WorkspaceDir string `json:"workspace_dir"`
		RunDir       string `json:"run_dir"`
	} `json:"paths"`
}

func (r *adkRunner) Run(ctx context.Context, req []byte, stdout, stderr, eventsLog io.Writer) ([]byte, []byte, int, error) {
	var fields requestFields
	if err := json.Unmarshal(req, &fields); err != nil {
		return nil, nil, 0, fmt.Errorf("unmarshal request fields: %w", err)
	}

	roleReq := roleagent.RoleRequest{
		Run: roleagent.RunInfo{
			ID:        fields.Run.ID,
			Iteration: fields.Run.Iteration,
		},
		Step: roleagent.StepInfo{
			Index: fields.Step.Index,
			Name:  fields.Step.Name,
		},
		Paths: roleagent.RequestPaths{
			WorkspaceDir: fields.Paths.WorkspaceDir,
			RunDir:       fields.Paths.RunDir,
		},
	}

	return r.executor.Run(ctx, r.role, roleReq, req, stdout, stderr, eventsLog)
}
