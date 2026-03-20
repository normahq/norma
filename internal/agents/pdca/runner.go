package pdca

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/agents/pdca/contracts"
	"github.com/metalagman/norma/internal/agents/roleagent"
	"github.com/metalagman/norma/internal/config"
)

type Runner interface {
	Run(ctx context.Context, req contracts.AgentRequest, stdout, stderr, eventsLog io.Writer) (outBytes, errBytes []byte, exitCode int, err error)
}

func NewRunner(cfg config.AgentConfig, role contracts.Role, mcpServers map[string]agentconfig.MCPServerConfig) (Runner, error) {
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
	role       contracts.Role
	mcpServers map[string]agentconfig.MCPServerConfig
	executor   *roleagent.Executor
}

func (r *adkRunner) Run(ctx context.Context, req contracts.AgentRequest, stdout, stderr, eventsLog io.Writer) ([]byte, []byte, int, error) {
	roleAdapter := &roleAdapter{
		role: r.role,
	}

	roleReq := roleagent.RoleRequest{
		Run: roleagent.RunInfo{
			ID:        req.Run.ID,
			Iteration: req.Run.Iteration,
		},
		Step: roleagent.StepInfo{
			Index: req.Step.Index,
			Name:  req.Step.Name,
		},
		Paths: roleagent.RequestPaths{
			WorkspaceDir: req.Paths.WorkspaceDir,
			RunDir:       req.Paths.RunDir,
		},
	}

	roleInput, err := json.Marshal(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("marshal role input: %w", err)
	}

	return r.executor.Run(ctx, roleAdapter, roleReq, roleInput, stdout, stderr, eventsLog)
}

type roleAdapter struct {
	role contracts.Role
}

func (a *roleAdapter) Name() string {
	return a.role.Name()
}

func (a *roleAdapter) Schemas() roleagent.SchemaPair {
	return roleagent.SchemaPair{
		InputSchema:  a.role.InputSchema(),
		OutputSchema: a.role.OutputSchema(),
	}
}

func (a *roleAdapter) Prompt(req roleagent.AgentRequest) (string, error) {
	contractReq, err := unmarshalToAgentRequest(req)
	if err != nil {
		return "", fmt.Errorf("unmarshal request: %w", err)
	}
	return a.role.Prompt(contractReq)
}

func (a *roleAdapter) MapRequest(req roleagent.AgentRequest) (any, error) {
	contractReq, err := unmarshalToAgentRequest(req)
	if err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}
	return a.role.MapRequest(contractReq)
}

func (a *roleAdapter) MapResponse(outBytes []byte) (roleagent.AgentResponse, error) {
	resp, err := a.role.MapResponse(outBytes)
	if err != nil {
		return roleagent.AgentResponse{}, err
	}

	roleResp := roleagent.AgentResponse{
		Status:     resp.Status,
		StopReason: resp.StopReason,
		Summary: roleagent.ResponseSummary{
			Text: resp.Summary.Text,
		},
		Progress: roleagent.StepProgress{
			Title:   resp.Progress.Title,
			Details: resp.Progress.Details,
		},
	}

	if resp.Plan != nil {
		if planBytes, err := json.Marshal(resp.Plan); err == nil {
			roleResp.PlanOutput = planBytes
		}
	}
	if resp.Do != nil {
		if doBytes, err := json.Marshal(resp.Do); err == nil {
			roleResp.DoOutput = doBytes
		}
	}
	if resp.Check != nil {
		if checkBytes, err := json.Marshal(resp.Check); err == nil {
			roleResp.CheckOutput = checkBytes
		}
	}
	if resp.Act != nil {
		if actBytes, err := json.Marshal(resp.Act); err == nil {
			roleResp.ActOutput = actBytes
		}
	}

	return roleResp, nil
}

func unmarshalToAgentRequest(data roleagent.AgentRequest) (contracts.AgentRequest, error) {
	var req contracts.AgentRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return contracts.AgentRequest{}, err
	}
	return req, nil
}
