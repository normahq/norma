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
	Model             string
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

	client       *Client
	workingDir   string
	sessionModel string
	logger       zerolog.Logger
	sessionMu    sync.Mutex
	remoteByADK  map[string]string
}

const (
	acpTypeText     = "text"
	acpTypeImage    = "image"
	acpTypeAudio    = "audio"
	acpTypeResource = "resource"
)

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
		client:       client,
		workingDir:   cfg.WorkingDir,
		sessionModel: strings.TrimSpace(cfg.Model),
		logger:       l,
		remoteByADK:  make(map[string]string),
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
				ev, ok := mapACPUpdateToEvent(a.logger, ctx.InvocationID(), note.Update)
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
	resp, err := a.client.CreateSession(ctx, a.workingDir, a.sessionModel)
	if err != nil {
		return "", err
	}
	sessionID := string(resp.SessionId)
	a.remoteByADK[adkSessionID] = sessionID
	event := a.logger.Debug().
		Str("adk_session_id", adkSessionID).
		Str("acp_session_id", sessionID)
	if a.sessionModel != "" {
		event = event.Str("model", a.sessionModel)
	}
	event.Msg("created new acp session for adk session")
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

func mapACPUpdateToEvent(logger zerolog.Logger, invocationID string, update acp.SessionUpdate) (*session.Event, bool) {
	switch {
	case update.AgentMessageChunk != nil:
		part, ok := mapACPContentBlockToPart(logger, update.AgentMessageChunk.Content)
		if !ok {
			return nil, false
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
		ev.Partial = true
		return ev, true
	case update.UserMessageChunk != nil:
		part, ok := mapACPContentBlockToPart(logger, update.UserMessageChunk.Content)
		if !ok {
			return nil, false
		}
		ev := session.NewEvent(invocationID)
		ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleUser)
		ev.Partial = true
		return ev, true
	case update.AgentThoughtChunk != nil:
		part, ok := mapACPContentBlockToPart(logger, update.AgentThoughtChunk.Content)
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
		logIgnoredACPUpdate(logger, "plan", map[string]any{"entries": update.Plan.Entries})
		return nil, false
	case update.AvailableCommandsUpdate != nil:
		logIgnoredACPUpdate(logger, "available_commands_update", map[string]any{
			"availableCommands": update.AvailableCommandsUpdate.AvailableCommands,
		})
		return nil, false
	case update.CurrentModeUpdate != nil:
		logIgnoredACPUpdate(logger, "current_mode_update", map[string]any{
			"currentModeId": update.CurrentModeUpdate.CurrentModeId,
		})
		return nil, false
	default:
		logUnsupportedACPUpdate(logger, update)
		return nil, false
	}
}

func mapACPContentBlockToPart(logger zerolog.Logger, block acp.ContentBlock) (*genai.Part, bool) {
	if block.Text != nil {
		if block.Text.Text == "" {
			return nil, false
		}
		return genai.NewPartFromText(block.Text.Text), true
	}
	logIgnoredACPContentBlock(logger, block)
	return nil, false
}

func marshalACPUpdatePayload(logger zerolog.Logger, payloadType string, v any) (string, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		logger.Debug().
			Err(err).
			Str("acp_payload_type", payloadType).
			Msg("ignoring acp payload that failed to marshal")
		return "", false
	}
	return string(raw), true
}

func isACPToolStatusLongRunning(status acp.ToolCallStatus) bool {
	return status == acp.ToolCallStatusPending || status == acp.ToolCallStatusInProgress
}

func logUnsupportedACPUpdate(logger zerolog.Logger, update acp.SessionUpdate) {
	logEvent := logger.Debug().
		Str("acp_update_type", sessionUpdateType(update))

	if payload, ok := marshalACPUpdatePayload(logger, "session_update_"+sessionUpdateType(update), update); ok {
		logEvent = logEvent.Str("acp_update_payload", payload)
	}
	logEvent.Msg("ignoring unsupported acp session update")
}

func logIgnoredACPUpdate(logger zerolog.Logger, updateType string, payload any) {
	logEvent := logger.Debug().
		Str("acp_update_type", updateType)

	if marshaled, ok := marshalACPUpdatePayload(logger, "session_update_"+updateType, payload); ok {
		logEvent = logEvent.Str("acp_update_payload", marshaled)
	}
	logEvent.Msg("ignoring non-user-visible acp session update")
}

func logIgnoredACPContentBlock(logger zerolog.Logger, block acp.ContentBlock) {
	blockType := contentBlockType(block)
	logEvent := logger.Debug().
		Str("acp_content_block_type", blockType).
		Str("acp_content_block_text", acpContentBlockLogText(block)).
		Interface("acp_content_block", acpContentBlockLogValue(block))

	if blockType == unknownValue {
		logEvent.Msg("ignoring unsupported acp content block")
		return
	}
	logEvent.Msg("ignoring non-text acp content block")
}

func acpContentBlockLogText(block acp.ContentBlock) string {
	switch {
	case block.Text != nil:
		return strings.TrimSpace(block.Text.Text)
	case block.Image != nil:
		return acpTypeImage
	case block.Audio != nil:
		return acpTypeAudio
	case block.ResourceLink != nil:
		return fmt.Sprintf("resource_link name=%q uri=%q", block.ResourceLink.Name, block.ResourceLink.Uri)
	case block.Resource != nil:
		return acpTypeResource
	default:
		return unknownValue
	}
}

func acpContentBlockLogValue(block acp.ContentBlock) map[string]any {
	obj := map[string]any{}
	switch {
	case block.Text != nil:
		obj["type"] = acpTypeText
		if block.Text.Text != "" {
			obj["text"] = block.Text.Text
		}
	case block.Image != nil:
		obj["type"] = acpTypeImage
		if block.Image.MimeType != "" {
			obj["mime_type"] = block.Image.MimeType
		}
		if block.Image.Uri != nil && *block.Image.Uri != "" {
			obj["uri"] = *block.Image.Uri
		}
		if block.Image.Data != "" {
			obj["data_len"] = len(block.Image.Data)
		}
	case block.Audio != nil:
		obj["type"] = acpTypeAudio
		if block.Audio.MimeType != "" {
			obj["mime_type"] = block.Audio.MimeType
		}
		if block.Audio.Data != "" {
			obj["data_len"] = len(block.Audio.Data)
		}
	case block.ResourceLink != nil:
		obj["type"] = "resource_link"
		if block.ResourceLink.Name != "" {
			obj["name"] = block.ResourceLink.Name
		}
		if block.ResourceLink.Uri != "" {
			obj["uri"] = block.ResourceLink.Uri
		}
		if block.ResourceLink.Description != nil && *block.ResourceLink.Description != "" {
			obj["description"] = *block.ResourceLink.Description
		}
		if block.ResourceLink.MimeType != nil && *block.ResourceLink.MimeType != "" {
			obj["mime_type"] = *block.ResourceLink.MimeType
		}
		if block.ResourceLink.Size != nil {
			obj["size"] = *block.ResourceLink.Size
		}
		if block.ResourceLink.Title != nil && *block.ResourceLink.Title != "" {
			obj["title"] = *block.ResourceLink.Title
		}
	case block.Resource != nil:
		obj["type"] = acpTypeResource
		obj["resource"] = acpEmbeddedResourceLogValue(block.Resource.Resource)
	default:
		obj["type"] = unknownValue
	}
	return obj
}

func acpEmbeddedResourceLogValue(resource acp.EmbeddedResourceResource) map[string]any {
	obj := map[string]any{}
	switch {
	case resource.TextResourceContents != nil:
		obj["kind"] = acpTypeText
		if resource.TextResourceContents.Uri != "" {
			obj["uri"] = resource.TextResourceContents.Uri
		}
		if resource.TextResourceContents.MimeType != nil && *resource.TextResourceContents.MimeType != "" {
			obj["mime_type"] = *resource.TextResourceContents.MimeType
		}
		if resource.TextResourceContents.Text != "" {
			obj["text_len"] = len(resource.TextResourceContents.Text)
		}
	case resource.BlobResourceContents != nil:
		obj["kind"] = "blob"
		if resource.BlobResourceContents.Uri != "" {
			obj["uri"] = resource.BlobResourceContents.Uri
		}
		if resource.BlobResourceContents.MimeType != nil && *resource.BlobResourceContents.MimeType != "" {
			obj["mime_type"] = *resource.BlobResourceContents.MimeType
		}
		if resource.BlobResourceContents.Blob != "" {
			obj["blob_len"] = len(resource.BlobResourceContents.Blob)
		}
	default:
		obj["kind"] = unknownValue
	}
	return obj
}

func sessionUpdateType(update acp.SessionUpdate) string {
	switch {
	case update.UserMessageChunk != nil:
		return "user_message_chunk"
	case update.AgentMessageChunk != nil:
		return "agent_message_chunk"
	case update.AgentThoughtChunk != nil:
		return "agent_thought_chunk"
	case update.ToolCall != nil:
		return "tool_call"
	case update.ToolCallUpdate != nil:
		return "tool_call_update"
	case update.Plan != nil:
		return "plan"
	case update.CurrentModeUpdate != nil:
		return "current_mode_update"
	case update.AvailableCommandsUpdate != nil:
		return "available_commands_update"
	default:
		return unknownValue
	}
}

func contentBlockType(block acp.ContentBlock) string {
	switch {
	case block.Text != nil:
		return acpTypeText
	case block.Image != nil:
		return acpTypeImage
	case block.Audio != nil:
		return acpTypeAudio
	case block.ResourceLink != nil:
		return "resource_link"
	case block.Resource != nil:
		return acpTypeResource
	default:
		return unknownValue
	}
}
