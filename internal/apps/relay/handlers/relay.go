package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/metalagman/norma/internal/apps/relay/agent"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/messenger"
	relaysession "github.com/metalagman/norma/internal/apps/relay/session"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
	"google.golang.org/genai"
)

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore     *auth.OwnerStore
	sessionManager *relaysession.Manager
	messenger      *messenger.Messenger
	normaCfg       config.Config
	tgClient       client.ClientWithResponsesInterface
	authToken      string
	logger         zerolog.Logger

	mu          sync.RWMutex
	ownerID     int64
	chatID      int64
	botUsername string
}

type relayHandlerDeps struct {
	fx.In

	LC             fx.Lifecycle
	OwnerStore     *auth.OwnerStore
	SessionManager *relaysession.Manager
	Messenger      *messenger.Messenger
	NormaCfg       config.Config
	TGClient       client.ClientWithResponsesInterface
	AuthToken      string `name:"relay_auth_token"`
	Logger         zerolog.Logger
}

func NewRelayHandler(deps relayHandlerDeps) (*RelayHandler, error) {
	h := &RelayHandler{
		ownerStore:     deps.OwnerStore,
		sessionManager: deps.SessionManager,
		messenger:      deps.Messenger,
		normaCfg:       deps.NormaCfg,
		tgClient:       deps.TGClient,
		authToken:      strings.TrimSpace(deps.AuthToken),
		logger:         deps.Logger.With().Str("component", "relay.handler").Logger(),
	}

	deps.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			h.onStart(ctx)
			return nil
		},
	})

	return h, nil
}

// Register registers the handler with the registry.
func (h *RelayHandler) Register(registry handlers.RegistryInterface) {
	registry.OnMessage(h.onMessage)
}

// SetOwner binds the handler to the owner. Pass chatID=0 when the chat
// is not yet known (it will be set from the first incoming message).
func (h *RelayHandler) SetOwner(ownerID, chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	log.Info().Int64("owner_id", ownerID).Int64("chat_id", chatID).Msg("Setting owner for relay")

	h.ownerID = ownerID
	if chatID != 0 {
		h.chatID = chatID
	}
}

// SendToOwner sends a message from the agent to the owner.
func (h *RelayHandler) SendToOwner(ctx context.Context, msg string) error {
	chatID := h.getChatID()
	if chatID == 0 {
		return fmt.Errorf("owner not set")
	}

	if err := h.messenger.SendPlain(ctx, chatID, msg, 0); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
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

	if chatID == 0 {
		h.setChatID(event.Message.Chat.Id)
		log.Info().Int64("chat_id", event.Message.Chat.Id).Msg("Chat ID set from message")
		chatID = event.Message.Chat.Id
	}

	if event.Message.Text == nil {
		return nil
	}
	if hasCommandEntity(event.Message) {
		return nil
	}

	text := *event.Message.Text
	if text == "" {
		return nil
	}

	var topicID int
	if event.Message.MessageThreadId != nil {
		topicID = *event.Message.MessageThreadId
	}

	log.Info().Int64("user_id", ownerID).Int("topic_id", topicID).Msg("Relaying message to agent")

	// Ensure session exists
	if topicID == 0 {
		sessionID := fmt.Sprintf("relay-%d-0", chatID)
		if _, ok := h.sessionManager.GetSessionRecord(sessionID); !ok {
			agentName := h.getRelayAgentName()
			if agentName == "" {
				log.Error().Msg("Failed to resolve relay agent name")
				return nil
			}
			if err := h.sessionManager.CreateSession(ctx, chatID, 0, agentName); err != nil {
				log.Error().Err(err).Msg("Failed to create main relay session")
				return nil
			}
		}
	}

	ts, err := h.sessionManager.GetOrRestoreSession(ctx, chatID, topicID)
	if err != nil {
		log.Error().Err(err).Int("topic_id", topicID).Msg("Failed to get/restore session")
		// Try to respond with error
		_ = h.messenger.SendPlain(ctx, chatID, fmt.Sprintf("Error restoring session: %v", err), topicID)
		return nil
	}

	// Send thinking feedback
	thoughtDraftID := event.Message.MessageId + 1
	if err := h.messenger.SendDraftPlain(ctx, chatID, thoughtDraftID, "Thinking...", topicID); err != nil {
		log.Warn().Err(err).Int("topic_id", topicID).Msg("failed to send thinking draft")
	}

	// Run agent
	userContent := genai.NewContentFromText(text, genai.RoleUser)
	userID := fmt.Sprintf("relay-%d-%d", chatID, topicID)

	result, err := agent.ProcessEvents(ctx, agent.EventParams{
		Runner:      ts.GetRunner(),
		UserID:      userID,
		SessionID:   ts.GetSessionID(),
		UserContent: userContent,
	})

	if err != nil {
		log.Error().Err(err).Int("topic_id", topicID).Msg("Agent execution failed")
		if sendErr := h.messenger.SendPlain(ctx, chatID, fmt.Sprintf("Error: %v", err), topicID); sendErr != nil {
			log.Warn().Err(sendErr).Int("topic_id", topicID).Msg("failed to send relay error message")
		}
		return nil
	}

	if strings.TrimSpace(result) != "" {
		if sendErr := h.messenger.SendMarkdown(ctx, chatID, result, topicID); sendErr != nil {
			log.Warn().Err(sendErr).Int("topic_id", topicID).Msg("failed to send relay response")
		}
	}

	return nil
}

func (h *RelayHandler) getRelayAgentName() string {
	cfg := h.normaCfg
	profileName := strings.TrimSpace(cfg.Profile)
	if profileName == "" {
		profileName = "default"
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return ""
	}
	return strings.TrimSpace(profile.Relay)
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

func hasCommandEntity(msg *client.Message) bool {
	if msg == nil || msg.Entities == nil {
		return false
	}
	for _, entity := range *msg.Entities {
		if entity.Type == "bot_command" {
			return true
		}
	}
	return false
}

func (h *RelayHandler) onStart(ctx context.Context) {
	h.initializeBotUsername(ctx)

	if !h.ownerStore.HasOwner() {
		return
	}
	owner := h.ownerStore.GetOwner()
	if owner == nil {
		return
	}

	h.SetOwner(owner.UserID, 0)

	if err := h.messenger.SendPlain(ctx, owner.UserID, "Boss, I'm online and ready to work.", 0); err != nil {
		h.logger.Warn().Err(err).Int64("owner_id", owner.UserID).Msg("failed to send startup ready message to owner")
		return
	}
	h.logger.Info().Int64("owner_id", owner.UserID).Msg("startup ready message sent to owner")
}

func (h *RelayHandler) initializeBotUsername(ctx context.Context) {
	if h.tgClient == nil {
		return
	}

	meResp, err := h.tgClient.GetMeWithResponse(ctx)
	if err != nil {
		h.logger.Warn().Err(err).Msg("getMe failed; bot username unavailable")
		return
	}
	if meResp.JSON200 == nil || meResp.JSON200.Result.Username == nil {
		h.logger.Warn().Str("status", meResp.Status()).Msg("getMe response missing username")
		return
	}

	username := strings.TrimSpace(*meResp.JSON200.Result.Username)
	if username == "" {
		h.logger.Warn().Msg("getMe returned empty username")
		return
	}

	h.mu.Lock()
	h.botUsername = username
	h.mu.Unlock()

	if h.authToken != "" {
		deeplink := fmt.Sprintf("https://t.me/%s?start=%s", username, h.authToken)
		h.logger.Info().Str("bot_username", username).Str("start_deeplink", deeplink).Msg("relay start deeplink ready")
		return
	}
	h.logger.Info().Str("bot_username", username).Msg("relay bot username loaded")
}
