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
	log.Debug().Msg("RelayHandler registered for OnMessage events")
}

func (h *RelayHandler) onMessage(ctx context.Context, event *events.MessageEvent) error {
	ownerID := h.getOwnerID()
	chatID := h.getChatID()

	log.Debug().
		Int64("event_user_id", event.Message.From.Id).
		Int64("owner_id", ownerID).
		Int64("chat_id", chatID).
		Msg("RelayHandler.onMessage called")

	if ownerID == 0 {
		log.Debug().Msg("No owner set, ignoring message")
		return nil
	}

	if event.Message.From.Id != ownerID {
		log.Debug().
			Int64("from_id", event.Message.From.Id).
			Int64("owner_id", ownerID).
			Msg("Message not from owner, ignoring")
		return nil
	}

	// Update chatID if it's not set (e.g., after restart with existing owner)
	if chatID == 0 {
		h.setChatID(event.Message.Chat.Id)
		log.Info().Int64("chat_id", event.Message.Chat.Id).Msg("Chat ID set from message")
	}

	if event.Message.Text == nil {
		log.Debug().Msg("Message has no text, ignoring")
		return nil
	}

	text := *event.Message.Text
	if text == "" {
		log.Debug().Msg("Empty text message, ignoring")
		return nil
	}

	log.Info().
		Int64("user_id", ownerID).
		Str("text", text).
		Msg("Relaying message to agent")

	select {
	case h.agentIn <- text:
		log.Debug().Msg("Message sent to agentIn channel")
		return nil
	default:
		log.Warn().Msg("Agent input channel full, dropping message")
		return nil
	}
}

// SetOwner sets the owner and starts the response forwarder.
// It is safe to call multiple times; subsequent calls update the owner info.
func (h *RelayHandler) SetOwner(ctx context.Context, ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().
		Int64("owner_id", ownerID).
		Int64("chat_id", chatID).
		Msg("Setting owner for relay")

	h.ownerID = ownerID
	h.chatID = chatID

	// Signal existing forwarder to stop
	close(h.quit)

	// Start new forwarder and processor with background context (not the command context)
	h.quit = make(chan struct{})
	go h.forwardAgentResponses(context.Background())
	go h.processAgentInput(context.Background())
}

// InitOwner sets only the owner ID (without chatID) and starts the forwarder.
// Used when loading an existing owner on startup.
func (h *RelayHandler) InitOwner(ctx context.Context, ownerID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Msg("Initializing relay with owner")

	h.ownerID = ownerID
	// chatID will be set when first message arrives

	// Start forwarder and processor goroutines
	h.quit = make(chan struct{})
	go h.forwardAgentResponses(context.Background())
	go h.processAgentInput(context.Background())
}

// processAgentInput reads from agentIn and processes messages.
// Currently echoes back - replace with actual agent integration.
func (h *RelayHandler) processAgentInput(ctx context.Context) {
	log.Debug().Msg("Agent input processor started")

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Agent input processor stopped by context")
			return
		case <-h.quit:
			log.Debug().Msg("Agent input processor stopped by quit signal")
			return
		case msg, ok := <-h.agentIn:
			if !ok {
				log.Debug().Msg("Agent input channel closed")
				return
			}

			log.Debug().Str("message", msg).Msg("Processing message from agentIn")

			// For now, echo back with prefix
			response := "Echo: " + msg

			select {
			case h.agentOut <- response:
				log.Debug().Str("response", response).Msg("Sent response to agentOut")
			default:
				log.Warn().Msg("Agent output channel full, dropping response")
			}
		}
	}
}

func (h *RelayHandler) forwardAgentResponses(ctx context.Context) {
	log.Debug().Msg("Agent response forwarder started")

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Agent response forwarder stopped by context")
			return
		case <-h.quit:
			log.Debug().Msg("Agent response forwarder stopped by quit signal")
			return
		case msg, ok := <-h.agentOut:
			if !ok {
				log.Debug().Msg("Agent output channel closed")
				return
			}
			chatID := h.getChatID()
			log.Debug().
				Int64("chat_id", chatID).
				Str("message", msg).
				Msg("Forwarding agent response to owner")
			if err := h.sendMessage(ctx, chatID, agentResponsePrefix+msg); err != nil {
				log.Error().Err(err).Msg("Failed to send agent response")
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

	log.Debug().
		Int64("chat_id", chatID).
		Str("message", msg).
		Msg("Sending message to owner via agentOut channel")

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

func (h *RelayHandler) sendMessage(ctx context.Context, chatID int64, text string) error {
	if chatID == 0 {
		return fmt.Errorf("chatID is 0, cannot send message")
	}
	_, err := h.tgClient.SendMessageWithResponse(ctx, client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	if err != nil {
		return fmt.Errorf("send message to chat %d: %w", chatID, err)
	}
	return nil
}
