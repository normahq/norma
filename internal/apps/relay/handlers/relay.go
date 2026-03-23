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
	ownerID  int64
	chatID   int64
	agentIn  chan string
	agentOut chan string
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
}

func (h *RelayHandler) onMessage(ctx context.Context, event *events.MessageEvent) error {
	h.mu.RLock()
	ownerID := h.ownerID
	h.mu.RUnlock()

	if ownerID == 0 {
		return nil
	}

	if event.Message.From.Id != ownerID {
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

	select {
	case h.agentIn <- text:
	default:
		log.Warn().Msg("Agent input channel full, dropping message")
	}

	return nil
}

// SetOwner sets the owner and starts the response forwarder.
func (h *RelayHandler) SetOwner(ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ownerID = ownerID
	h.chatID = chatID

	go h.forwardAgentResponses()
}

func (h *RelayHandler) forwardAgentResponses() {
	for msg := range h.agentOut {
		h.mu.RLock()
		chatID := h.chatID
		h.mu.RUnlock()

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
	chatID := h.chatID
	h.mu.RUnlock()

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

func (h *RelayHandler) sendMessage(chatID int64, text string) error {
	_, err := h.tgClient.SendMessageWithResponse(context.Background(), client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	return err
}
