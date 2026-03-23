package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentfactory"
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
	message string
}

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore *auth.OwnerStore
	tgClient   client.ClientWithResponsesInterface
	normaCfg   config.Config

	mu             sync.RWMutex
	ownerID        int64
	chatID         int64
	agentIn        chan agentMessage
	agentOut       chan string
	quit           chan struct{}
	sessionService session.Service
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface, normaCfg config.Config) *RelayHandler {
	return &RelayHandler{
		ownerStore:     ownerStore,
		tgClient:       tgClient,
		normaCfg:       normaCfg,
		agentIn:        make(chan agentMessage, defaultChannelSize),
		agentOut:       make(chan string, defaultChannelSize),
		quit:           make(chan struct{}),
		sessionService: session.InMemoryService(),
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

	// Update chatID if not set
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

	log.Info().Int64("user_id", ownerID).Str("text", text).Msg("Relaying message to agent")

	select {
	case h.agentIn <- agentMessage{chatID: event.Message.Chat.Id, message: text}:
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

	close(h.quit)
	h.quit = make(chan struct{})
	go h.forwardAgentResponses(context.Background())
	go h.processAgentInput(context.Background())
}

// InitOwner sets only the owner ID and starts the forwarder.
func (h *RelayHandler) InitOwner(ctx context.Context, ownerID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Msg("Initializing relay with owner")

	h.ownerID = ownerID

	h.quit = make(chan struct{})
	go h.forwardAgentResponses(context.Background())
	go h.processAgentInput(context.Background())
}

func (h *RelayHandler) processAgentInput(ctx context.Context) {
	log.Info().Msg("Agent input processor started")

	factory := agentfactory.NewFactoryWithMCPServers(h.normaCfg.Agents, h.normaCfg.MCPServers)

	// Get the do agent name from profile
	profile := h.normaCfg.Profiles[h.normaCfg.Profile]
	agentName := profile.PDCA.Do
	if agentName == "" {
		for name := range h.normaCfg.Agents {
			agentName = name
			break
		}
	}

	log.Info().Str("agent", agentName).Msg("Using agent for relay")

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.quit:
			return
		case msg, ok := <-h.agentIn:
			if !ok {
				return
			}

			sessionID := fmt.Sprintf("chat-%d", msg.chatID)
			response, err := h.runAgent(ctx, factory, agentName, sessionID, msg.message)
			if err != nil {
				log.Error().Err(err).Msg("Failed to run agent")
				response = fmt.Sprintf("Error: %v", err)
			}

			select {
			case h.agentOut <- response:
			default:
				log.Warn().Msg("Agent output channel full, dropping response")
			}
		}
	}
}

func (h *RelayHandler) runAgent(ctx context.Context, factory *agentfactory.Factory, agentName, sessionID, message string) (string, error) {
	workDir, err := os.MkdirTemp("", "norma-relay-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	req := agentfactory.CreationRequest{
		Name:             agentName,
		WorkingDirectory: workDir,
		Stdout:           stdout,
		Stderr:           stderr,
		Logger:           &log.Logger,
		PermissionHandler: func(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
			if len(req.Options) > 0 {
				return acp.RequestPermissionResponse{
					Outcome: acp.NewRequestPermissionOutcomeSelected(req.Options[0].OptionId),
				}, nil
			}
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeCancelled(),
			}, nil
		},
	}

	ag, err := factory.CreateAgent(ctx, agentName, req)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}

	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "norma-relay",
		Agent:          ag,
		SessionService: h.sessionService,
	})
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	sess, err := h.sessionService.Create(ctx, &session.CreateRequest{
		AppName: "norma-relay",
		UserID:  sessionID,
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	userContent := genai.NewContentFromText(message, genai.RoleUser)

	// Send typing action before starting
	h.sendChatAction(ctx, chatIDFromSessionID(sessionID), "typing")

	// Start a goroutine to keep sending typing action while agent runs
	typingCtx, cancelTyping := context.WithCancel(ctx)
	defer cancelTyping()
	go h.keepTyping(typingCtx, chatIDFromSessionID(sessionID))

	var result strings.Builder
	for ev, err := range r.Run(ctx, sessionID, sess.Session.ID(), userContent, agent.RunConfig{}) {
		if err != nil {
			return "", fmt.Errorf("agent run error: %w", err)
		}
		if ev == nil {
			continue
		}
		// Accumulate text from all content events (including partial)
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part.Text != "" {
					result.WriteString(part.Text)
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
		case <-h.quit:
			return
		case msg, ok := <-h.agentOut:
			if !ok {
				return
			}
			chatID := h.getChatID()
			if err := h.sendMessage(ctx, chatID, msg); err != nil {
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
