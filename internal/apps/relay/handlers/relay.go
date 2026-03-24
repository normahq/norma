package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"google.golang.org/adk/agent"
	runnerpkg "google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const (
	defaultChannelSize = 100
	acpToolCallEvent   = "acp_tool_call"
)

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore     *auth.OwnerStore
	tgClient       client.ClientWithResponsesInterface
	normaCfg       config.Config
	workingDir     string
	sessionManager *TopicSessionManager

	mu             sync.RWMutex
	ownerID        int64
	chatID         int64
	sessionService session.Service
	draftCounter   atomic.Int64

	relayAgents *RelayAgentManager
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface, normaCfg config.Config, workingDir string, sessionManager *TopicSessionManager, relayAgents *RelayAgentManager) *RelayHandler {
	return &RelayHandler{
		ownerStore:     ownerStore,
		tgClient:       tgClient,
		normaCfg:       normaCfg,
		workingDir:     workingDir,
		sessionManager: sessionManager,
		sessionService: session.InMemoryService(),
		relayAgents:    relayAgents,
	}
}

// Register registers the handler with the registry.
func (h *RelayHandler) Register(registry handlers.RegistryInterface) {
	registry.OnMessage(h.onMessage)
}

func (h *RelayHandler) onMessage(ctx context.Context, event *events.MessageEvent) error {
	ownerID := h.getOwnerID()
	chatID := h.getChatID()

	if ownerID == 0 {
		return nil
	}

	if event.Message.From.Id != ownerID {
		return nil
	}

	// Update chatID if not set.
	if chatID == 0 {
		h.setChatID(event.Message.Chat.Id)
		log.Info().Int64("chat_id", event.Message.Chat.Id).Msg("Chat ID set from message")
	}

	if event.Message.Text == nil {
		return nil
	}

	text := *event.Message.Text
	if text == "" {
		return nil
	}

	// Check if this is a topic message
	var topicID int
	if event.Message.MessageThreadId != nil {
		topicID = *event.Message.MessageThreadId
	}

	// If topic message, route to TopicSessionManager
	if topicID != 0 && h.sessionManager != nil {
		log.Info().Int64("user_id", ownerID).Int("topic_id", topicID).Str("text", text).Msg("Relaying message to topic agent")
		if err := h.sessionManager.SendMessage(event.Message.Chat.Id, topicID, text); err != nil {
			log.Error().Err(err).Int("topic_id", topicID).Msg("Failed to send message to topic agent")
		}
		return nil
	}

	// Non-topic messages go to main relay agent
	log.Info().Int64("user_id", ownerID).Str("text", text).Msg("Relaying message to agent")

	draftID := int(h.draftCounter.Add(1))
	h.processMessage(ctx, event.Message.Chat.Id, 0, text, draftID)
	return nil
}

// SetOwner sets the owner and starts the response forwarder.
func (h *RelayHandler) SetOwner(ctx context.Context, ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Int64("chat_id", chatID).Msg("Setting owner for relay")

	h.ownerID = ownerID
	h.chatID = chatID
}

// InitOwner sets only the owner ID and starts the forwarder.
func (h *RelayHandler) InitOwner(ctx context.Context, ownerID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Msg("Initializing relay with owner")

	h.ownerID = ownerID
}

// Stop stops the relay handler (no-op, channels removed).
func (h *RelayHandler) Stop() {
}

func (h *RelayHandler) processMessage(ctx context.Context, chatID int64, topicID int, text string, draftID int) {
	sessionID := fmt.Sprintf("chat-%d", chatID)
	responseDraftID := draftID
	eventsDraftID := int(h.draftCounter.Add(1))

	onProgress := func(text string) {
		_, err := h.sendMessageDraft(ctx, chatID, responseDraftID, text, topicID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to update message draft")
			return
		}
	}

	onThought := func(text string) {
		_, err := h.sendMessageDraftPlain(ctx, chatID, eventsDraftID, text, topicID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to update thought draft")
			return
		}
	}

	onTool := func(text string) {
		_, err := h.sendMessageDraftPlain(ctx, chatID, eventsDraftID, text, topicID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to update tool-event draft")
			return
		}
	}

	_, err := h.runAgent(ctx, sessionID, text, onProgress, onThought, onTool)
	if err != nil {
		log.Error().Err(err).Msg("Failed to run agent")
		response := fmt.Sprintf("error: %v", err)
		if _, sendErr := h.sendMessageDraft(ctx, chatID, 0, response, topicID); sendErr != nil {
			log.Error().Err(sendErr).Msg("Failed to send error response")
		}
	}
}

func (h *RelayHandler) runAgent(ctx context.Context, sessionID, message string, onProgress func(string), onThought func(string), onTool func(string)) (string, error) {
	ag, err := h.relayAgents.Agent()
	if err != nil {
		return "", fmt.Errorf("getting relay agent: %w", err)
	}

	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "norma-relay",
		Agent:          ag,
		SessionService: h.sessionService,
	})
	if err != nil {
		return "", fmt.Errorf("creating runner: %w", err)
	}

	sess, err := h.sessionService.Create(ctx, &session.CreateRequest{
		AppName: "norma-relay",
		UserID:  sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	userContent := genai.NewContentFromText(message, genai.RoleUser)

	// Send typing action before starting.
	h.sendChatAction(ctx, chatIDFromSessionID(sessionID), "typing")

	// Start a goroutine to keep sending typing action while agent runs.
	typingCtx, cancelTyping := context.WithCancel(ctx)
	defer cancelTyping()
	go h.keepTyping(typingCtx, chatIDFromSessionID(sessionID))

	var result strings.Builder

	for ev, err := range r.Run(ctx, sessionID, sess.Session.ID(), userContent, agent.RunConfig{}) {
		if err != nil {
			return "", fmt.Errorf("agent run: %w", err)
		}
		if ev == nil {
			continue
		}
		// Handle tool call events: print tool name but hide parameters.
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part == nil {
					continue
				}
				// Handle tool call start - print tool name only, hide parameters.
				if part.FunctionCall != nil && part.FunctionCall.Name == acpToolCallEvent {
					// Extract tool title from args.
					args := part.FunctionCall.Args
					title := extractToolTitle(args)
					if title == "" {
						title = acpToolCallEvent
					}
					// Print tool call start - only the tool name.
					if onTool != nil {
						onTool(fmt.Sprintf("ToolCall: %s", title))
					}
					continue
				}
				// Forward tool call updates to the events stream.
				if part.FunctionResponse != nil && part.FunctionResponse.Name == "acp_tool_call_update" {
					if onTool != nil {
						onTool(formatToolUpdate(part.FunctionResponse.Response))
					}
					continue
				}
				// Handle thought parts.
				if part.Thought {
					if onThought != nil && part.Text != "" {
						onThought(part.Text)
					}
					continue
				}
				if part.Text != "" {
					result.WriteString(part.Text)
					if onProgress != nil {
						onProgress(result.String())
					}
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}

	return result.String(), nil
}

func formatToolUpdate(response any) string {
	if response == nil {
		return "ToolUpdate"
	}
	return fmt.Sprintf("ToolUpdate: %v", response)
}

// extractToolTitle extracts the tool title from function call args.
func extractToolTitle(args any) string {
	if args == nil {
		return ""
	}
	// Handle string args (raw JSON).
	if s, ok := args.(string); ok {
		// Try to parse as JSON to extract title.
		if strings.HasPrefix(s, "{") {
			// Simple extraction - look for "title".
			if idx := strings.Index(s, `"title"`); idx >= 0 {
				rest := s[idx+7:]
				if idx := strings.Index(rest, `","`); idx >= 0 {
					return strings.Trim(rest[:idx], `" :`)
				}
				if idx := strings.Index(rest, `"}`); idx >= 0 {
					return strings.Trim(rest[:idx], `" :`)
				}
			}
		}
		return ""
	}
	// Handle map/struct args.
	if m, ok := args.(map[string]any); ok {
		if title, ok := m["title"].(string); ok {
			return title
		}
	}
	return ""
}

// keepTyping sends typing action every 4 seconds until context is canceled.
func (h *RelayHandler) keepTyping(ctx context.Context, chatID int64) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sendChatAction(ctx, chatID, "typing")
		}
	}
}

// sendChatAction sends a chat action (typing, etc.) to the user.
func (h *RelayHandler) sendChatAction(ctx context.Context, chatID int64, action string) {
	if chatID == 0 {
		return
	}
	_, _ = h.tgClient.SendChatActionWithResponse(ctx, client.SendChatActionJSONRequestBody{
		ChatId: chatID,
		Action: action,
	})
}

// chatIDFromSessionID extracts chatID from sessionID (format: "chat-<chatID>").
func chatIDFromSessionID(sessionID string) int64 {
	var chatID int64
	_, _ = fmt.Sscanf(sessionID, "chat-%d", &chatID)
	return chatID
}

// SendToOwner sends a message from the agent to the owner.
func (h *RelayHandler) SendToOwner(ctx context.Context, msg string) error {
	chatID := h.getChatID()
	if chatID == 0 {
		return fmt.Errorf("owner not set")
	}

	if _, err := h.sendMessageDraft(ctx, chatID, 0, msg, 0); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}

func (h *RelayHandler) getOwnerID() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ownerID
}

func (h *RelayHandler) getChatID() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.chatID
}

func (h *RelayHandler) setChatID(chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.chatID = chatID
}

func escapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func (h *RelayHandler) sendMessageDraft(ctx context.Context, chatID int64, draftID int, text string, topicID int) (int, error) {
	parseMode := "MarkdownV2"
	req := client.SendMessageDraftJSONRequestBody{
		ChatId:    chatID,
		DraftId:   draftID,
		Text:      escapeMarkdownV2(text),
		ParseMode: &parseMode,
	}
	if topicID != 0 {
		req.MessageThreadId = &topicID
	}

	resp, err := h.tgClient.SendMessageDraftWithResponse(ctx, req)
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Msg("send draft with MarkdownV2 failed, retrying without parse_mode")
		req.ParseMode = nil
		resp, err = h.tgClient.SendMessageDraftWithResponse(ctx, req)
		if err != nil {
			return draftID, fmt.Errorf("sending draft to chat %d: %w", chatID, err)
		}
	}
	if resp.JSON400 != nil {
		return draftID, fmt.Errorf("sending draft to chat %d: %s", chatID, resp.JSON400.Description)
	}
	if resp.JSON200 == nil {
		return draftID, fmt.Errorf("sending draft to chat %d: no response body", chatID)
	}
	return draftID, nil
}

func (h *RelayHandler) sendMessageDraftPlain(ctx context.Context, chatID int64, draftID int, text string, topicID int) (int, error) {
	req := client.SendMessageDraftJSONRequestBody{
		ChatId:  chatID,
		DraftId: draftID,
		Text:    text,
	}
	if topicID != 0 {
		req.MessageThreadId = &topicID
	}

	resp, err := h.tgClient.SendMessageDraftWithResponse(ctx, req)
	if err != nil {
		return draftID, fmt.Errorf("sending plain draft to chat %d: %w", chatID, err)
	}
	if resp.JSON400 != nil {
		return draftID, fmt.Errorf("sending plain draft to chat %d: %s", chatID, resp.JSON400.Description)
	}
	if resp.JSON200 == nil {
		return draftID, fmt.Errorf("sending plain draft to chat %d: no response body", chatID)
	}
	return draftID, nil
}
