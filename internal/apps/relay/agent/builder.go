package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/config"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

const relaySystemInstruction = `You are Norma Relay, an AI assistant operating inside a Telegram chat/topic.

Reply requirements:
- Use standard Markdown only (no HTML).
- Keep responses concise and directly actionable for chat.
- Return only user-facing final answers (no internal reasoning or tool traces).
- Use fenced code blocks for commands or code when needed.
- If the request is ambiguous, ask one short clarifying question.

Context:
- You are communicating with the relay bot owner.
- Your Markdown response will be converted before being sent to Telegram.

Built-in MCP Servers:
- norma.config — read/write Norma configuration
- norma.tasks — manage tasks, epics, features (Beads integration)
- norma.state — persistent session state storage
- norma.relay — spawn and manage subagent sessions
- norma.workspace — import/export workspace changes with master`

type Builder struct {
	factory  *agentfactory.Factory
	normaCfg config.Config
}

// NewBuilder creates a Builder with the given factory and config.
func NewBuilder(factory *agentfactory.Factory, normaCfg config.Config) *Builder {
	return &Builder{
		factory:  factory,
		normaCfg: normaCfg,
	}
}

// CanBuild checks if an agent can be built with the given name.
// It returns an error if the agent is not found in the factory registry.
func (b *Builder) CanBuild(agentName string) error {
	if _, err := b.factory.GetAgentConfig(agentName); err != nil {
		return fmt.Errorf("agent %q not found", agentName)
	}
	return nil
}

type BuiltAgent struct {
	Agent      agent.Agent
	Runner     *runner.Runner
	SessionSvc session.Service
	Session    session.Session
}

func (b *Builder) Build(ctx context.Context, sessionID string, chatID int64, topicID int, agentName, workspaceDir string) (*BuiltAgent, error) {
	branchName := fmt.Sprintf("norma/relay/%s", sessionID)
	req := agentfactory.CreationRequest{
		Name:              agentName,
		Description:       b.buildAgentDescription(agentName),
		WorkingDirectory:  workspaceDir,
		Stderr:            os.Stderr,
		Logger:            nil,
		SystemInstruction: b.buildRelaySystemInstruction(agentName, branchName, workspaceDir),
		PermissionHandler: DefaultPermissionHandler,
	}

	ag, err := b.factory.CreateAgent(ctx, agentName, req)
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

func (b *Builder) buildRelaySystemInstruction(agentName, branchName, workspaceDir string) string {
	base := relaySystemInstruction

	workspaceContext := fmt.Sprintf("\n\nWorkspace:\n- Branch: %s\n- Path: %s", branchName, workspaceDir)
	base += workspaceContext

	agentCfg, ok := b.normaCfg.Agents[agentName]
	if !ok {
		return base
	}
	agentSpecific := strings.TrimSpace(agentCfg.SystemInstruction)
	if agentSpecific == "" {
		return base
	}
	return base + "\n\nAgent-specific instructions:\n" + agentSpecific
}

// buildAgentDescription returns a human-readable description of the agent.
func (b *Builder) buildAgentDescription(agentName string) string {
	agentCfg, ok := b.normaCfg.Agents[agentName]
	if !ok {
		return agentName
	}
	return agentCfg.Description(agentName)
}
