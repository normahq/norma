package handlers

import (
	"context"
	"fmt"
	"strings"
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
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type topicSession struct {
	topicID   int
	agentName string
	agent     agent.Agent
	in        chan agentMessage
	out       chan<- string
	sessionID string
}

type TopicSessionManager struct {
	factory    *agentfactory.Factory
	normaCfg   config.Config
	workingDir string

	mu       sync.RWMutex
	sessions map[string]*topicSession
	cancels  map[string]context.CancelFunc
}

func NewTopicSessionManager(factory *agentfactory.Factory, normaCfg config.Config, workingDir string) *TopicSessionManager {
	return &TopicSessionManager{
		factory:    factory,
		normaCfg:   normaCfg,
		workingDir: workingDir,
		sessions:   make(map[string]*topicSession),
		cancels:    make(map[string]context.CancelFunc),
	}
}

func (m *TopicSessionManager) sessionID(chatID int64, topicID int) string {
	return fmt.Sprintf("topic-%d-%d", chatID, topicID)
}

func (m *TopicSessionManager) CreateSession(ctx context.Context, chatID int64, topicID int, agentName string, responseRelay *TopicResponseRelay) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("session already exists for topic %d", topicID)
	}
	m.mu.Unlock()

	req := agentfactory.CreationRequest{
		Name:              agentName,
		WorkingDirectory:  m.workingDir,
		Stderr:            nil,
		Logger:            nil,
		PermissionHandler: defaultPermissionHandler,
	}

	ag, err := m.factory.CreateAgent(ctx, agentName, req)
	if err != nil {
		return fmt.Errorf("creating agent %q: %w", agentName, err)
	}

	sessionSvc := session.InMemoryService()

	sess, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: fmt.Sprintf("norma-relay-topic-%d", topicID),
		UserID:  sessionID,
	})
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	ts := &topicSession{
		topicID:   topicID,
		agentName: agentName,
		agent:     ag,
		in:        make(chan agentMessage, defaultChannelSize),
		out:       nil,
		sessionID: sess.Session.ID(),
	}

	runCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.sessions[sessionID] = ts
	m.cancels[sessionID] = cancel
	m.mu.Unlock()

	go m.runAgentLoop(runCtx, ts, ag, sessionSvc, sess.Session, responseRelay, chatID, topicID)

	return nil
}

func defaultPermissionHandler(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if len(req.Options) > 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(req.Options[0].OptionId),
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.NewRequestPermissionOutcomeCancelled(),
	}, nil
}

func (m *TopicSessionManager) runAgentLoop(ctx context.Context, ts *topicSession, ag agent.Agent, sessionSvc session.Service, sess session.Session, responseRelay *TopicResponseRelay, chatID int64, topicID int) {
	r, err := runner.New(runner.Config{
		AppName:        fmt.Sprintf("norma-relay-topic-%d", ts.topicID),
		Agent:          ag,
		SessionService: sessionSvc,
	})
	if err != nil {
		log.Error().Err(err).Int("topic_id", ts.topicID).Msg("Failed to create runner")
		return
	}

	defer func() {
		close(ts.in)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ts.in:
			if !ok {
				return
			}
			m.processMessage(ctx, ts, r, sess, msg, responseRelay, chatID, topicID)
		}
	}
}

func (m *TopicSessionManager) processMessage(ctx context.Context, ts *topicSession, r *runner.Runner, sess session.Session, msg agentMessage, responseRelay *TopicResponseRelay, chatID int64, topicID int) {
	userContent := genai.NewContentFromText(msg.message, genai.RoleUser)

	for ev, err := range r.Run(ctx, ts.sessionID, sess.ID(), userContent, agent.RunConfig{}) {
		if err != nil {
			log.Error().Err(err).Int("topic_id", ts.topicID).Msg("agent run error")
			_ = responseRelay.SendToTopic(chatID, topicID, fmt.Sprintf("error: %v", err))
			return
		}
		if ev == nil {
			continue
		}
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part.Thought {
					continue
				}
				if part.Text != "" {
					_ = responseRelay.SendToTopic(chatID, topicID, part.Text)
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}
}

func (m *TopicSessionManager) SendMessage(chatID int64, topicID int, text string) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no session for topic %d", topicID)
	}

	select {
	case ts.in <- agentMessage{chatID: chatID, topicID: topicID, message: text}:
		return nil
	default:
		return fmt.Errorf("agent input channel full")
	}
}

func (m *TopicSessionManager) StopSession(chatID int64, topicID int) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.cancels[sessionID]; exists {
		cancel()
		delete(m.cancels, sessionID)
	}
	delete(m.sessions, sessionID)
}

func (m *TopicSessionManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cancel := range m.cancels {
		cancel()
	}
	m.cancels = make(map[string]context.CancelFunc)
	m.sessions = make(map[string]*topicSession)
}

type NewHandler struct {
	ownerStore         *auth.OwnerStore
	tgClient           client.ClientWithResponsesInterface
	sessionManager     *TopicSessionManager
	topicResponseRelay *TopicResponseRelay
}

type NewHandlerParams struct {
	OwnerStore         *auth.OwnerStore
	TgClient           client.ClientWithResponsesInterface
	SessionManager     *TopicSessionManager
	TopicResponseRelay *TopicResponseRelay
}

func NewNewHandler(params NewHandlerParams) *NewHandler {
	return &NewHandler{
		ownerStore:         params.OwnerStore,
		tgClient:           params.TgClient,
		sessionManager:     params.SessionManager,
		topicResponseRelay: params.TopicResponseRelay,
	}
}

func (h *NewHandler) Register(registry handlers.RegistryInterface) {
	registry.OnCommand(h.onCommand)
}

func (h *NewHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	if event.Command != "new" {
		return nil
	}

	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id

	if !h.ownerStore.HasOwner() || !h.ownerStore.IsOwner(userID) {
		return h.sendMessage(ctx, chatID, 0, "Only the bot owner can use this command.")
	}

	agentName := strings.TrimSpace(event.Args)
	if agentName == "" {
		return h.sendMessage(ctx, chatID, 0, "Usage: /new <agent_name>\n\nAvailable agents: gemini_agent, opencode_agent, etc.")
	}

	log.Info().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Str("agent", agentName).
		Msg("Creating new topic with agent")

	if err := h.createTopicWithAgent(ctx, chatID, agentName); err != nil {
		log.Error().Err(err).Str("agent", agentName).Msg("Failed to create topic with agent")
		return h.sendMessage(ctx, chatID, 0, fmt.Sprintf("Failed to create agent session: %v", err))
	}

	return nil
}

func (h *NewHandler) createTopicWithAgent(ctx context.Context, chatID int64, agentName string) error {
	topicName := fmt.Sprintf("Agent: %s", agentName)

	createTopicResp, err := h.tgClient.CreateForumTopicWithResponse(ctx, client.CreateForumTopicJSONRequestBody{
		ChatId: chatID,
		Name:   topicName,
	})
	if err != nil {
		return fmt.Errorf("creating forum topic: %w", err)
	}

	if createTopicResp.JSON200 == nil {
		return fmt.Errorf("failed to create forum topic: %s", createTopicResp.Status())
	}

	topic := createTopicResp.JSON200.Result
	topicID := topic.MessageThreadId

	log.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("topic_name", topic.Name).
		Msg("Forum topic created")

	h.topicResponseRelay.RegisterTopic(chatID, topicID)

	if err := h.sessionManager.CreateSession(ctx, chatID, topicID, agentName, h.topicResponseRelay); err != nil {
		h.topicResponseRelay.UnregisterTopic(chatID, topicID)
		h.closeTopic(ctx, chatID, topicID)
		return fmt.Errorf("creating agent session: %w", err)
	}

	welcomeMsg := fmt.Sprintf("Started new %s agent session.\n\nSend your messages here and I'll forward them to the agent.", agentName)
	if err := h.sendMessage(ctx, chatID, topicID, welcomeMsg); err != nil {
		log.Error().Err(err).Msg("Failed to send welcome message")
	}

	return nil
}

func (h *NewHandler) closeTopic(ctx context.Context, chatID int64, topicID int) {
	closeResp, err := h.tgClient.CloseForumTopicWithResponse(ctx, client.CloseForumTopicJSONRequestBody{
		ChatId:          chatID,
		MessageThreadId: topicID,
	})
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Int("topic_id", topicID).Msg("Failed to close forum topic")
		return
	}
	if closeResp.JSON200 == nil {
		log.Warn().Int64("chat_id", chatID).Int("topic_id", topicID).Str("status", closeResp.Status()).Msg("Failed to close forum topic")
	}
}

func (h *NewHandler) sendMessage(ctx context.Context, chatID int64, topicID int, text string) error {
	req := client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	}
	if topicID != 0 {
		req.MessageThreadId = &topicID
	}

	_, err := h.tgClient.SendMessageWithResponse(ctx, req)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}
