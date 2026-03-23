package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/rs/zerolog"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
)

const (
	// defaultChannelSize is the default buffer size for agent channels.
	defaultChannelSize = 100

	// agentResponsePrefix is the prefix for agent responses.
	agentResponsePrefix = "🤖 Agent: "
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
	quit     chan struct{}
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface) *RelayHandler {
	return &RelayHandler{
		ownerStore: ownerStore,
		tgClient:   tgClient,
		agentIn:    make(chan string, defaultChannelSize),
		agentOut:   make(chan string, defaultChannelSize),
		quit:       make(chan struct{}),
	}
}

// Register registers the handler with the registry.
func (h *RelayHandler) Register(registry handlers.RegistryInterface) {
	registry.OnMessage(h.onMessage)
}

func (h *RelayHandler) onMessage(ctx context.Context, event *events.MessageEvent) error {
	logger := zerolog.Ctx(ctx)

	ownerID := h.getOwnerID()
	if ownerID == 0 {
		return nil
	}

	if event.Message.From.Id != ownerID {
		return nil
	}

	if event.Message.Text == nil {
		return nil
	}

	text := *event.Message.Text
	if text == "" {
		return nil
	}

	logger.Debug().
		Int64("user_id", ownerID).
		Str("text", text).
		Msg("Relaying message to agent")

	select {
	case h.agentIn <- text:
		return nil
	default:
		logger.Warn().Msg("Agent input channel full, dropping message")
		return nil
	}
}

// SetOwner sets the owner and starts the response forwarder.
// It is safe to call multiple times; subsequent calls update the owner info.
func (h *RelayHandler) SetOwner(ctx context.Context, ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ownerID = ownerID
	h.chatID = chatID

	// Signal existing forwarder to stop
	close(h.quit)

	// Start new forwarder with fresh context
	h.quit = make(chan struct{})
	go h.forwardAgentResponses(ctx)
}

func (h *RelayHandler) forwardAgentResponses(ctx context.Context) {
	logger := zerolog.Ctx(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Debug().Msg("Agent response forwarder stopped by context")
			return
		case <-h.quit:
			logger.Debug().Msg("Agent response forwarder stopped by quit signal")
			return
		case msg, ok := <-h.agentOut:
			if !ok {
				return
			}
			chatID := h.getChatID()
			if err := h.sendMessage(ctx, chatID, agentResponsePrefix+msg); err != nil {
				logger.Error().Err(err).Msg("Failed to send agent response")
			}
		}
	}
}

// GetInputChannel returns the channel for sending messages to the agent.
func (h *RelayHandler) GetInputChannel() <-chan string {
	return h.agentIn
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

func (h *RelayHandler) sendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := h.tgClient.SendMessageWithResponse(ctx, client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	if err != nil {
		return fmt.Errorf("send message to chat %d: %w", chatID, err)
	}
	return nil
}
