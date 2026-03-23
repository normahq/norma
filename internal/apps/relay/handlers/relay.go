package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

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

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore *auth.OwnerStore
	tgClient   client.ClientWithResponsesInterface
	normaCfg   config.Config

	mu       sync.RWMutex
	ownerID  int64
	chatID   int64
	agentIn  chan string
	agentOut chan string
	quit     chan struct{}
}

// NewRelayHandler creates a new relay handler.
func NewRelayHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface, normaCfg config.Config) *RelayHandler {
	return &RelayHandler{
		ownerStore: ownerStore,
		tgClient:   tgClient,
		normaCfg:   normaCfg,
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
	case h.agentIn <- text:
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

			response, err := h.runAgent(ctx, factory, agentName, msg)
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

func (h *RelayHandler) runAgent(ctx context.Context, factory *agentfactory.Factory, agentName, message string) (string, error) {
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

	sessionService := session.InMemoryService()
	r, err := runnerpkg.New(runnerpkg.Config{
		AppName:        "norma-relay",
		Agent:          ag,
		SessionService: sessionService,
	})
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	sess, err := sessionService.Create(ctx, &session.CreateRequest{AppName: "norma-relay", UserID: "relay-user"})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	userContent := genai.NewContentFromText(message, genai.RoleUser)

	var result string
	for ev, err := range r.Run(ctx, "relay-user", sess.Session.ID(), userContent, agent.RunConfig{}) {
		if err != nil {
			return "", fmt.Errorf("agent run error: %w", err)
		}
		if ev == nil {
			continue
		}
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part.Text != "" {
					result += part.Text
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}

	return result, nil
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
