package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/normahq/norma/internal/apps/relay/auth"
	"github.com/normahq/norma/internal/apps/relay/messenger"
	relaysession "github.com/normahq/norma/internal/apps/relay/session"
	"github.com/normahq/norma/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/genai"
)

// RelayHandler handles bidirectional message relay between owner and agent.
type RelayHandler struct {
	ownerStore     *auth.OwnerStore
	sessionManager *relaysession.Manager
	messenger      *messenger.Messenger
	tgClient       client.ClientWithResponsesInterface
	authToken      string
	relayAgentName string
	normaCfg       config.Config
	logger         zerolog.Logger

	mu          sync.RWMutex
	ownerID     int64
	chatID      int64
	botUsername string
}

type relayHandlerDeps struct {
	fx.In

	LC                 fx.Lifecycle
	OwnerStore         *auth.OwnerStore
	SessionManager     *relaysession.Manager
	Messenger          *messenger.Messenger
	TGClient           client.ClientWithResponsesInterface
	AuthToken          string `name:"relay_auth_token"`
	RelayAgentName     string `name:"relay_agent_name"`
	NormaCfg           config.Config
	Logger             zerolog.Logger
	InternalMCPManager *InternalMCPManager `optional:"true"`
}

func NewRelayHandler(deps relayHandlerDeps) (*RelayHandler, error) {
	h := &RelayHandler{
		ownerStore:     deps.OwnerStore,
		sessionManager: deps.SessionManager,
		messenger:      deps.Messenger,
		tgClient:       deps.TGClient,
		authToken:      strings.TrimSpace(deps.AuthToken),
		relayAgentName: strings.TrimSpace(deps.RelayAgentName),
		normaCfg:       deps.NormaCfg,
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

	var ts *relaysession.TopicSession
	var err error

	if topicID == 0 {
		// Main orchestrator: ensure session exists using relay agent from config
		// Check if session already exists to avoid sending spinning message again
		existingSession, _ := h.sessionManager.GetSession(chatID, topicID)
		if existingSession == nil {
			// Send spinning message for on-demand creation
			if agentCfg, ok := h.normaCfg.Agents[h.relayAgentName]; ok {
				spinningMsg := h.buildSpinningMessage(h.relayAgentName, agentCfg)
				_ = h.messenger.SendPlain(ctx, chatID, spinningMsg, topicID)
			}
		}
		ts, err = h.sessionManager.EnsureSession(ctx, chatID, topicID, h.relayAgentName)
		if err != nil {
			log.Error().Err(err).Str("agent", h.relayAgentName).Msg("Failed to ensure orchestrator session")
			_ = h.messenger.SendPlain(ctx, chatID, fmt.Sprintf("Failed to start orchestrator: %v", err), topicID)
			return nil
		}
	} else {
		// Subagent topic: require existing session
		ts, err = h.sessionManager.GetSession(chatID, topicID)
		if err != nil {
			log.Warn().Err(err).Int("topic_id", topicID).Msg("No session found, closing topic")
			h.sessionManager.CloseTopic(ctx, chatID, topicID)
			_ = h.messenger.SendPlain(ctx, chatID, "Agent unavailable. Use /new to start a new session.", topicID)
			return nil
		}
	}

	// Run turn
	if err := h.runTurn(ctx, text, ts.GetRunner(), ts.GetSessionID(), chatID, topicID, event.Message.MessageId); err != nil {
		log.Error().Err(err).Int("topic_id", topicID).Msg("Agent execution failed")
		if sendErr := h.messenger.SendPlain(ctx, chatID, fmt.Sprintf("Error: %v", err), topicID); sendErr != nil {
			log.Warn().Err(sendErr).Int("topic_id", topicID).Msg("failed to send relay error message")
		}
	}

	return nil
}

func (h *RelayHandler) runTurn(ctx context.Context, text string, r *runner.Runner, sessionID string, chatID int64, topicID, messageID int) error {
	userContent := genai.NewContentFromText(text, genai.RoleUser)
	userID := fmt.Sprintf("relay-%d-%d", chatID, topicID)
	draftID := messageID + 1

	var result strings.Builder
	thinkingStages := []string{"Thinking.", "Thinking..", "Thinking..."}
	thinkingIdx := 0

	for ev, err := range r.Run(ctx, userID, sessionID, userContent, agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("agent run: %w", err)
		}
		if ev == nil {
			continue
		}
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part == nil {
					continue
				}
				if part.Thought {
					if sendErr := h.messenger.SendDraftPlain(ctx, chatID, draftID, thinkingStages[thinkingIdx%len(thinkingStages)], topicID); sendErr != nil {
						log.Warn().Err(sendErr).Int("topic_id", topicID).Msg("failed to send thinking draft")
					}
					thinkingIdx++
					continue
				}
				if part.Text != "" {
					result.WriteString(part.Text)
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}

	if s := result.String(); strings.TrimSpace(s) != "" {
		if sendErr := h.messenger.SendMarkdown(ctx, chatID, s, topicID); sendErr != nil {
			log.Warn().Err(sendErr).Int("topic_id", topicID).Msg("failed to send relay response")
		}
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

	h.SetOwner(owner.UserID, owner.ChatID)

	// Precreate orchestrator session synchronously if chatID is known
	if owner.ChatID != 0 {
		h.ensureOrchestratorSession(ctx, owner.ChatID)
	}

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

func (h *RelayHandler) ensureOrchestratorSession(ctx context.Context, chatID int64) {
	agentName := h.relayAgentName
	if agentName == "" {
		h.logger.Warn().Msg("no relay agent configured, skipping orchestrator precreate")
		return
	}

	agentCfg, ok := h.normaCfg.Agents[agentName]
	if !ok {
		h.logger.Warn().Str("agent", agentName).Msg("relay agent not found in config")
		return
	}

	spinningMsg := h.buildSpinningMessage(agentName, agentCfg)
	if err := h.messenger.SendPlain(ctx, chatID, spinningMsg, 0); err != nil {
		h.logger.Warn().Err(err).Msg("failed to send spinning up message")
	}

	ts, err := h.sessionManager.EnsureSession(ctx, chatID, 0, agentName)
	if err != nil {
		h.logger.Error().Err(err).Str("agent", agentName).Msg("failed to precreate orchestrator session")
		return
	}

	h.logger.Info().
		Int64("chat_id", chatID).
		Str("agent", agentName).
		Str("session_id", ts.GetSessionID()).
		Msg("orchestrator session precreated")
}

func (h *RelayHandler) buildSpinningMessage(agentName string, cfg config.AgentConfig) string {
	return "Spinning up agent: " + cfg.Description(agentName)
}
