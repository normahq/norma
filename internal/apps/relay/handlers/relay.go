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
)

// agentMessage represents a message to be processed by the agent.
type agentMessage struct {
	chatID  int64
	topicID int
	message string
}

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
	agentIn        chan agentMessage
	agentOut       chan string
	cancel         context.CancelFunc
	sessionService session.Service

	ag agent.Agent
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface, normaCfg config.Config, workingDir string, sessionManager *TopicSessionManager, ag agent.Agent) *RelayHandler {
	return &RelayHandler{
		ownerStore:     ownerStore,
		tgClient:       tgClient,
		normaCfg:       normaCfg,
		workingDir:     workingDir,
		sessionManager: sessionManager,
		agentIn:        make(chan agentMessage, defaultChannelSize),
		agentOut:       make(chan string, defaultChannelSize),
		sessionService: session.InMemoryService(),
		ag:             ag,
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

	select {
	case h.agentIn <- agentMessage{
		chatID:  event.Message.Chat.Id,
		topicID: 0,
		message: text,
	}:
		return nil
	default:
		log.Warn().Msg("Agent input channel full, dropping message")
		return nil
	}
}

// SetOwner sets the owner and starts the response forwarder.
func (h *RelayHandler) SetOwner(ctx context.Context, ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Int64("chat_id", chatID).Msg("Setting owner for relay")

	h.ownerID = ownerID
	h.chatID = chatID

	if h.cancel != nil {
		h.cancel()
	}

	runCtx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	go h.forwardAgentResponses(runCtx)
	go h.processAgentInput(runCtx)
}

// InitOwner sets only the owner ID and starts the forwarder.
func (h *RelayHandler) InitOwner(ctx context.Context, ownerID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Msg("Initializing relay with owner")

	h.ownerID = ownerID

	if h.cancel != nil {
		h.cancel()
	}

	runCtx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	go h.forwardAgentResponses(runCtx)
	go h.processAgentInput(runCtx)
}

// Stop stops the relay handler goroutines.
func (h *RelayHandler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *RelayHandler) processAgentInput(ctx context.Context) {
	log.Info().Msg("Agent input processor started")

	// Resolve agent name from profile.
	profileName := h.normaCfg.Profile
	if profileName == "" {
		profileName = "default"
	}

	log.Info().Str("profile", profileName).Interface("all_profiles", h.normaCfg.Profiles).Msg("Resolving relay agent")

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-h.agentIn:
			if !ok {
				return
			}

			sessionID := fmt.Sprintf("chat-%d", msg.chatID)
			var messageID atomic.Int32

			onProgress := func(text string) {
				// Limit updates to avoid rate limits? For now, we update on every chunk.
				// In a real app we might want to throttle this.
				id, err := h.sendMessageDraft(ctx, msg.chatID, int(messageID.Load()), text, msg.topicID)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to update message draft")
					return
				}
				if messageID.Load() == 0 {
					messageID.Store(int32(id))
				}
			}

			_, err := h.runAgent(ctx, sessionID, msg.message, onProgress)
			if err != nil {
				log.Error().Err(err).Msg("Failed to run agent")
				response := fmt.Sprintf("error: %v", err)
				select {
				case h.agentOut <- response:
				default:
					log.Warn().Msg("Agent output channel full, dropping error response")
				}
			}
		}
	}
}

func (h *RelayHandler) runAgent(ctx context.Context, sessionID, message string, onProgress func(string)) (string, error) {
	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "norma-relay",
		Agent:          h.ag,
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
		// Accumulate text from all content events (including partial).
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				// Skip thought/reasoning parts.
				if part.Thought {
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

func (h *RelayHandler) forwardAgentResponses(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-h.agentOut:
			if !ok {
				return
			}
			chatID := h.getChatID()
			if _, err := h.sendMessageDraft(ctx, chatID, 0, msg, 0); err != nil {
				log.Error().Err(err).Msg("Failed to send agent response")
			}
		}
	}
}

// SendToOwner sends a message from the agent to the owner.
func (h *RelayHandler) SendToOwner(ctx context.Context, msg string) error {
	chatID := h.getChatID()
	if chatID == 0 {
		return fmt.Errorf("owner not set")
	}

	select {
	case h.agentOut <- msg:
		return nil
	default:
		return fmt.Errorf("agent output channel full")
	}
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

// sendMessageDraft sends a new message or edits an existing one.
func (h *RelayHandler) sendMessageDraft(ctx context.Context, chatID int64, draftID int, text string, topicID int) (int, error) {
	if draftID == 0 {
		// 1. Send new message.
		parseMode := "Markdown"
		req := client.SendMessageJSONRequestBody{
			ChatId:    chatID,
			Text:      text,
			ParseMode: &parseMode,
		}
		if topicID != 0 {
			req.MessageThreadId = &topicID
		}

		resp, err := h.tgClient.SendMessageWithResponse(ctx, req)
		if err != nil {
			// If markdown fails, try sending without parse mode.
			req.ParseMode = nil
			resp, err = h.tgClient.SendMessageWithResponse(ctx, req)
			if err != nil {
				return 0, fmt.Errorf("sending message to chat %d: %w", chatID, err)
			}
		}
		if resp.JSON200 == nil {
			return 0, fmt.Errorf("failed to send message, no response body")
		}
		return resp.JSON200.Result.MessageId, nil
	}

	// 2. Edit existing message.
	parseMode := "Markdown"
	resp, err := h.tgClient.EditMessageTextWithResponse(ctx, client.EditMessageTextJSONRequestBody{
		ChatId:    &chatID,
		MessageId: &draftID,
		Text:      text,
		ParseMode: &parseMode,
	})
	if err != nil {
		// If markdown fails, try sending without parse mode.
		resp, err = h.tgClient.EditMessageTextWithResponse(ctx, client.EditMessageTextJSONRequestBody{
			ChatId:    &chatID,
			MessageId: &draftID,
			Text:      text,
		})
		if err != nil {
			return draftID, fmt.Errorf("editing message %d in chat %d: %w", draftID, chatID, err)
		}
	}
	if resp.JSON200 == nil {
		return draftID, nil
	}
	return draftID, nil
}
