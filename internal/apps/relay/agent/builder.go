package agent

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/normahq/norma/internal/adk/agentfactory"
	runtimeconfig "github.com/normahq/norma/pkg/runtime/config"
	"go.uber.org/fx"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

//go:embed system_instruction.gotmpl
var relaySystemInstructionTmpl string

type Builder struct {
	factory          *agentfactory.Factory
	normaCfg         runtimeconfig.NormaConfig
	workspaceEnabled bool
}

type relayPromptData struct {
	SessionID         string
	BranchName        string
	WorkspaceDir      string
	WorkspaceEnabled  bool
	AgentInstructions string
}

func (b *Builder) buildRelaySystemInstruction(sessionID, agentName, branchName, workspaceDir string) string {
	data := relayPromptData{
		SessionID:        sessionID,
		BranchName:       branchName,
		WorkspaceDir:     workspaceDir,
		WorkspaceEnabled: b.workspaceEnabled,
	}

	agentCfg, ok := b.normaCfg.Agents[agentName]
	if ok {
		data.AgentInstructions = strings.TrimSpace(agentCfg.SystemInstruction)
	}

	var buf bytes.Buffer
	tmpl := template.Must(template.New("relay").Parse(relaySystemInstructionTmpl))
	if err := tmpl.Execute(&buf, data); err != nil {
		return relaySystemInstructionTmpl
	}
	return buf.String()
}

type BuilderParams struct {
	fx.In

	Factory          *agentfactory.Factory
	NormaCfg         runtimeconfig.NormaConfig
	WorkspaceEnabled bool `name:"relay_workspace_enabled"`
}

// NewBuilder creates a Builder with the given factory and config.
func NewBuilder(params BuilderParams) *Builder {
	return &Builder{
		factory:          params.Factory,
		normaCfg:         params.NormaCfg,
		workspaceEnabled: params.WorkspaceEnabled,
	}
}

// ValidateAgent checks if an agent with the given name can be created.
// It returns an error if the agent is not found or its type is unsupported.
func (b *Builder) ValidateAgent(agentName string) error {
	return b.factory.ValidateAgent(agentName)
}

type BuiltAgent struct {
	Agent      agent.Agent
	Runner     *runner.Runner
	SessionSvc session.Service
	Session    session.Session
}

func (b *Builder) Build(ctx context.Context, sessionID string, chatID int64, topicID int, agentName, workspaceDir string) (*BuiltAgent, error) {
	branchName := fmt.Sprintf("norma/relay/%s", sessionID)
	req := agentfactory.BuildRequest{
		AgentID:           agentName,
		Name:              agentName,
		Description:       b.buildAgentDescription(agentName),
		WorkingDirectory:  workspaceDir,
		SystemInstruction: b.buildRelaySystemInstruction(sessionID, agentName, branchName, workspaceDir),
	}

	ag, err := b.factory.Build(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating agent %q: %w", agentName, err)
	}

	sessionSvc := session.InMemoryService()
	sess, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: fmt.Sprintf("norma-relay-topic-%d", topicID),
		UserID:  sessionID,
	})
	if err != nil {
		if closer, ok := ag.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("creating session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        fmt.Sprintf("norma-relay-topic-%d", topicID),
		Agent:          ag,
		SessionService: sessionSvc,
	})
	if err != nil {
		if closer, ok := ag.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("creating runner: %w", err)
	}

	return &BuiltAgent{
		Agent:      ag,
		Runner:     r,
		SessionSvc: sessionSvc,
		Session:    sess.Session,
	}, nil
}

// buildAgentDescription returns a human-readable description of the agent.
func (b *Builder) buildAgentDescription(agentName string) string {
	agentCfg, ok := b.normaCfg.Agents[agentName]
	if !ok {
		return agentName
	}
	return agentCfg.Description(agentName)
}

// GetAgentInfo returns the description and list of MCP server names for an agent.
func (b *Builder) GetAgentInfo(agentName string) (description string, mcpServers []string) {
	agentCfg, ok := b.normaCfg.Agents[agentName]
	if !ok {
		return agentName, nil
	}
	return agentCfg.Description(agentName), agentCfg.MCPServers
}
