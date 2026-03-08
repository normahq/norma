package acpagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
	"github.com/rs/zerolog"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Config configures an ACP-backed ADK agent.
type Config struct {
	Context           context.Context
	Name              string
	Description       string
	ClientName        string
	ClientVersion     string
	Command           []string
	WorkingDir        string
	Stderr            io.Writer
	PermissionHandler PermissionHandler
	Logger            *zerolog.Logger
}

// Agent adapts an ACP runtime to the ADK agent interface.
type Agent struct {
	adkagent.Agent

	client      *Client
	workingDir  string
	logger      zerolog.Logger
	sessionMu   sync.Mutex
	remoteByADK map[string]string
}

var _ adkagent.Agent = (*Agent)(nil)

// New creates an ADK agent backed by an ACP client process.
func New(cfg Config) (*Agent, error) {
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "ACPAgent"
	}
	if strings.TrimSpace(cfg.Description) == "" {
		cfg.Description = "ACP runtime exposed through ADK"
	}

	l := zerolog.Nop()
	if cfg.Logger != nil {
		l = cfg.Logger.With().Str("subcomponent", "acpagent.agent").Logger()
	}

	client, err := NewClient(ctx, ClientConfig{
		Command:           cfg.Command,
		WorkingDir:        cfg.WorkingDir,
		ClientName:        cfg.ClientName,
		ClientVersion:     cfg.ClientVersion,
		Stderr:            cfg.Stderr,
		PermissionHandler: cfg.PermissionHandler,
		Logger:            cfg.Logger,
	})
	if err != nil {
		return nil, err
	}
	if _, err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("initialize acp client: %w", err)
	}

	a := &Agent{
		client:      client,
		workingDir:  cfg.WorkingDir,
		logger:      l,
		remoteByADK: make(map[string]string),
	}
	base, err := adkagent.New(adkagent.Config{
		Name:        cfg.Name,
		Description: cfg.Description,
		Run:         a.run,
	})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("create adk acp agent: %w", err)
	}
	a.Agent = base
	return a, nil
}

// Close shuts down the underlying ACP client process.
func (a *Agent) Close() error {
	return a.client.Close()
}

func (a *Agent) run(ctx adkagent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		remoteSessionID, err := a.ensureRemoteSession(ctx, ctx.Session().ID())
		if err != nil {
			yield(nil, err)
			return
		}

		prompt := extractPromptText(ctx.UserContent())
		if strings.TrimSpace(prompt) == "" {
			yield(nil, fmt.Errorf("prompt is empty"))
			return
		}

		a.logger.Debug().
			Str("adk_session_id", ctx.Session().ID()).
			Str("acp_session_id", remoteSessionID).
			Int("prompt_len", len(prompt)).
			Msg("starting adk invocation")

		updates, resultCh, err := a.client.Prompt(ctx, remoteSessionID, prompt)
		if err != nil {
			yield(nil, err)
			return
		}

		var finalText strings.Builder
		partialEmitted := false
		var promptResult *PromptResult
		for updates != nil || resultCh != nil {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case note, ok := <-updates:
				if !ok {
					updates = nil
					continue
				}
				if note.Update.AgentMessageChunk != nil {
					if chunk := updateText(note.Update); chunk != "" {
						finalText.WriteString(chunk)
						partialEmitted = true
					}
				}
				ev, ok := mapACPUpdateToEvent(ctx.InvocationID(), note.Update)
				if !ok {
					continue
				}
				if !yield(ev, nil) {
					return
				}
			case result, ok := <-resultCh:
				if !ok {
					resultCh = nil
					continue
				}
				promptResult = &result
				resultCh = nil
			}
		}
		if promptResult != nil && promptResult.Err != nil {
			yield(nil, promptResult.Err)
			return
		}

		a.logger.Debug().
			Str("adk_session_id", ctx.Session().ID()).
			Str("acp_session_id", remoteSessionID).
			Int("response_len", finalText.Len()).
			Msg("completed adk invocation")

		ev := session.NewEvent(ctx.InvocationID())
		if !partialEmitted {
			ev.Content = genai.NewContentFromText(finalText.String(), genai.RoleModel)
		}
		ev.TurnComplete = true
		if !yield(ev, nil) {
			return
		}
	}
}

func (a *Agent) ensureRemoteSession(ctx context.Context, adkSessionID string) (string, error) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	if sessionID := a.remoteByADK[adkSessionID]; sessionID != "" {
		a.logger.Debug().Str("adk_session_id", adkSessionID).Str("acp_session_id", sessionID).Msg("reusing acp session for adk session")
		return sessionID, nil
	}
	resp, err := a.client.NewSession(ctx, a.workingDir)
	if err != nil {
		return "", err
	}
	sessionID := string(resp.SessionId)
	a.remoteByADK[adkSessionID] = sessionID
	a.logger.Debug().Str("adk_session_id", adkSessionID).Str("acp_session_id", sessionID).Msg("created new acp session for adk session")
	return sessionID, nil
}

func extractPromptText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		builder.WriteString(part.Text)
	}
	return strings.TrimSpace(builder.String())
}

func updateText(update acp.SessionUpdate) string {
	if update.AgentMessageChunk == nil {
		return ""
	}
	content := update.AgentMessageChunk.Content
	if content.Text == nil {
		return ""
	}
	return content.Text.Text
}

func mapACPUpdateToEvent(invocationID string, update acp.SessionUpdate) (*session.Event, bool) {
	switch {
	case update.AgentMessageChunk != nil:
		part, ok := mapACPContentBlockToPart(update.AgentMessageChunk.Content)
		if !ok {
			return nil, false
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
		ev.Partial = true
		return ev, true
	case update.UserMessageChunk != nil:
		part, ok := mapACPContentBlockToPart(update.UserMessageChunk.Content)
		if !ok {
			return nil, false
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser)
		ev.Partial = true
		return ev, true
	case update.AgentThoughtChunk != nil:
		part, ok := mapACPContentBlockToPart(update.AgentThoughtChunk.Content)
		if !ok {
			return nil, false
		}
		part.Thought = true
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
		ev.Partial = true
		return ev, true
	case update.ToolCall != nil:
		tool := update.ToolCall
		args := map[string]any{
			"kind":      tool.Kind,
			"status":    tool.Status,
			"title":     tool.Title,
			"locations": tool.Locations,
			"rawInput":  tool.RawInput,
			"rawOutput": tool.RawOutput,
		}
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   string(tool.ToolCallId),
				Name: "acp_tool_call",
				Args: args,
			},
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
		if isACPToolStatusLongRunning(tool.Status) {
			ev.LongRunningToolIDs = []string{string(tool.ToolCallId)}
		}
		return ev, true
	case update.ToolCallUpdate != nil:
		tool := update.ToolCallUpdate
		response := map[string]any{
			"status":    tool.Status,
			"title":     tool.Title,
			"kind":      tool.Kind,
			"locations": tool.Locations,
			"rawInput":  tool.RawInput,
			"rawOutput": tool.RawOutput,
		}
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				ID:       string(tool.ToolCallId),
				Name:     "acp_tool_call_update",
				Response: response,
			},
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
		if tool.Status != nil && isACPToolStatusLongRunning(*tool.Status) {
			ev.LongRunningToolIDs = []string{string(tool.ToolCallId)}
		}
		return ev, true
	case update.Plan != nil:
		payload := map[string]any{"entries": update.Plan.Entries}
		return newACPTextEvent(invocationID, genai.RoleModel, marshalACPUpdatePayload(payload), false), true
	case update.AvailableCommandsUpdate != nil:
		payload := map[string]any{"availableCommands": update.AvailableCommandsUpdate.AvailableCommands}
		return newACPTextEvent(invocationID, genai.RoleModel, marshalACPUpdatePayload(payload), false), true
	case update.CurrentModeUpdate != nil:
		payload := map[string]any{"currentModeId": update.CurrentModeUpdate.CurrentModeId}
		return newACPTextEvent(invocationID, genai.RoleModel, marshalACPUpdatePayload(payload), false), true
	default:
		return newACPTextEvent(invocationID, genai.RoleModel, marshalACPUpdatePayload(update), false), true
	}
}

func mapACPContentBlockToPart(block acp.ContentBlock) (*genai.Part, bool) {
	if block.Text != nil {
		if block.Text.Text == "" {
			return nil, false
		}
		return genai.NewPartFromText(block.Text.Text), true
	}
	payload := marshalACPUpdatePayload(block)
	if payload == "" {
		return nil, false
	}
	return genai.NewPartFromText(payload), true
}

func newACPTextEvent(invocationID string, role genai.Role, text string, partial bool) *session.Event {
	ev := session.NewEvent(invocationID)
	ev.Content = genai.NewContentFromText(text, role)
	ev.Partial = partial
	return ev
}

func marshalACPUpdatePayload(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshalError":%q}`, err.Error())
	}
	return string(raw)
}

func isACPToolStatusLongRunning(status acp.ToolCallStatus) bool {
	return status == acp.ToolCallStatusPending || status == acp.ToolCallStatusInProgress
}
