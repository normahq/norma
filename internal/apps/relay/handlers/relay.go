package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
)

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore *auth.OwnerStore
	tgClient   client.ClientWithResponsesInterface

	mu       sync.RWMutex
	active   bool
	ownerID  int64
	chatID   int64
	agentIn  chan string // messages to agent
	agentOut chan string // messages from agent
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface) *RelayHandler {
	return &RelayHandler{
		ownerStore: ownerStore,
		tgClient:   tgClient,
		agentIn:    make(chan string, 100),
		agentOut:   make(chan string, 100),
	}
}

// Register registers the handler with the registry.
func (h *RelayHandler) Register(registry handlers.RegistryInterface) {
	registry.OnMessage(h.onMessage)
	registry.OnCommand(h.onCommand)
}

func (h *RelayHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id

	switch event.Command {
	case "relay":
		return h.handleRelayCommand(ctx, chatID, userID, event.Args)
	case "stop":
		return h.handleStopCommand(ctx, chatID, userID)
	}
	return nil
}

func (h *RelayHandler) onMessage(ctx context.Context, event *events.MessageEvent) error {
	h.mu.RLock()
	active := h.active
	ownerID := h.ownerID
	h.mu.RUnlock()

	if !active {
		return nil
	}

	// Only process messages from owner
	if event.Message.From.Id != ownerID {
		return nil
	}

	// Don't process commands here (handled separately)
	if event.Message.Text != nil && len(*event.Message.Text) > 0 && (*event.Message.Text)[0] == '/' {
		return nil
	}

	text := ""
	if event.Message.Text != nil {
		text = *event.Message.Text
	}

	if text == "" {
		return nil
	}

	log.Debug().
		Int64("user_id", ownerID).
		Str("text", text).
		Msg("Relaying message to agent")

	// Forward to agent input channel
	select {
	case h.agentIn <- text:
	default:
		log.Warn().Msg("Agent input channel full, dropping message")
	}

	return nil
}

func (h *RelayHandler) handleRelayCommand(ctx context.Context, chatID, userID int64, args string) error {
	// Check if user is owner
	if !h.ownerStore.IsOwner(userID) {
		return h.sendMessage(chatID, "Only the bot owner can start relay mode.")
	}

	h.mu.Lock()
	if h.active {
		h.mu.Unlock()
		return h.sendMessage(chatID, "Relay is already active. Send /stop to end relay mode.")
	}
	h.active = true
	h.ownerID = userID
	h.chatID = chatID
	h.mu.Unlock()

	log.Info().
		Int64("owner_id", userID).
		Msg("Relay mode started")

	// Start goroutine to forward agent responses to owner
	go h.forwardAgentResponses()

	return h.sendMessage(chatID, "🔄 Relay mode activated!\n\nMessages you send will be forwarded to the agent. Agent responses will appear here.\n\nSend /stop to end relay mode.")
}

func (h *RelayHandler) handleStopCommand(ctx context.Context, chatID, userID int64) error {
	if !h.ownerStore.IsOwner(userID) {
		return nil
	}

	h.mu.Lock()
	if !h.active {
		h.mu.Unlock()
		return h.sendMessage(chatID, "Relay is not active.")
	}
	h.active = false
	h.mu.Unlock()

	log.Info().Int64("owner_id", userID).Msg("Relay mode stopped")

	return h.sendMessage(chatID, "⏹ Relay mode deactivated.")
}

func (h *RelayHandler) forwardAgentResponses() {
	for msg := range h.agentOut {
		h.mu.RLock()
		chatID := h.chatID
		active := h.active
		h.mu.RUnlock()

		if !active {
			return
		}

		if err := h.sendMessage(chatID, fmt.Sprintf("🤖 Agent: %s", msg)); err != nil {
			log.Error().Err(err).Msg("Failed to send agent response")
		}
	}
}

// GetInputChannel returns the channel for sending messages to the agent.
func (h *RelayHandler) GetInputChannel() <-chan string {
	return h.agentIn
}

// SendToOwner sends a message from the agent to the owner.
func (h *RelayHandler) SendToOwner(msg string) error {
	h.mu.RLock()
	active := h.active
	h.mu.RUnlock()

	if !active {
		return fmt.Errorf("relay not active")
	}

	select {
	case h.agentOut <- msg:
		return nil
	default:
		return fmt.Errorf("agent output channel full")
	}
}

// IsActive returns whether relay mode is active.
func (h *RelayHandler) IsActive() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.active
}

// GetOwnerChatID returns the owner's chat ID if active.
func (h *RelayHandler) GetOwnerChatID() (int64, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.active {
		return 0, false
	}
	return h.chatID, true
}

func (h *RelayHandler) sendMessage(chatID int64, text string) error {
	_, err := h.tgClient.SendMessageWithResponse(context.Background(), client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	return err
}
