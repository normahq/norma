package handlers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type topicSession struct {
	sessionID    string
	topicID      int
	agentName    string
	agent        agent.Agent
	runner       *runner.Runner
	sessionSvc   session.Service
	sess         session.Session
	draftCounter atomic.Int64
	chatID       int64
	tgClient     client.ClientWithResponsesInterface
	workspaceDir string
}

type TopicSessionManager struct {
	factory    *agentfactory.Factory
	normaCfg   config.Config
	workingDir string
	tgClient   client.ClientWithResponsesInterface
	store      *topicSessionStore

	mu       sync.RWMutex
	sessions map[string]*topicSession
	records  map[string]sessionRecord
}

func NewTopicSessionManager(factory *agentfactory.Factory, normaCfg config.Config, workingDir string, tgClient client.ClientWithResponsesInterface) (*TopicSessionManager, error) {
	normaDir := filepath.Join(workingDir, ".norma")
	store, err := newTopicSessionStore(normaDir)
	if err != nil {
		return nil, fmt.Errorf("create topic session store: %w", err)
	}

	return &TopicSessionManager{
		factory:    factory,
		normaCfg:   normaCfg,
		workingDir: workingDir,
		tgClient:   tgClient,
		store:      store,
		sessions:   make(map[string]*topicSession),
		records:    make(map[string]sessionRecord),
	}, nil
}

func (m *TopicSessionManager) sessionID(chatID int64, topicID int) string {
	return fmt.Sprintf("topic-%d-%d", chatID, topicID)
}

func (m *TopicSessionManager) CreateSession(ctx context.Context, chatID int64, topicID int, agentName string) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("session already exists for topic %d", topicID)
	}
	m.mu.Unlock()

	workspaceDir, err := m.ensureWorkspace(ctx, chatID, topicID, "")
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	ts, err := m.buildTopicSession(ctx, sessionID, chatID, topicID, agentName, workspaceDir)
	if err != nil {
		_ = m.cleanupWorkspace(ctx, workspaceDir)
		return err
	}

	m.mu.Lock()
	m.sessions[sessionID] = ts
	m.records[sessionID] = sessionRecord{
		SessionID:    sessionID,
		ChatID:       chatID,
		TopicID:      topicID,
		AgentName:    agentName,
		WorkspaceDir: workspaceDir,
		Status:       sessionStatusActive,
		UpdatedAt:    nowRFC3339(),
	}
	err = m.persistLocked()
	if err != nil {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if err != nil {
		if closeErr := m.closeTopicSession(ts); closeErr != nil {
			log.Warn().Err(closeErr).Str("session_id", sessionID).Msg("failed to close topic session after persistence error")
		}
		return fmt.Errorf("persist session: %w", err)
	}

	return nil
}

func (m *TopicSessionManager) CreateTopicSession(ctx context.Context, chatID int64, agentName string) (string, int, error) {
	topicName := fmt.Sprintf("Agent: %s", agentName)
	createTopicResp, err := m.tgClient.CreateForumTopicWithResponse(ctx, client.CreateForumTopicJSONRequestBody{
		ChatId: chatID,
		Name:   topicName,
	})
	if err != nil {
		return "", 0, fmt.Errorf("creating forum topic: %w", err)
	}
	if createTopicResp.JSON200 == nil {
		return "", 0, fmt.Errorf("failed to create forum topic: %s", createTopicResp.Status())
	}

	topic := createTopicResp.JSON200.Result
	topicID := topic.MessageThreadId

	if err := m.CreateSession(ctx, chatID, topicID, agentName); err != nil {
		m.closeTopic(ctx, chatID, topicID)
		return "", 0, fmt.Errorf("creating agent session: %w", err)
	}

	return m.sessionID(chatID, topicID), topicID, nil
}

func (m *TopicSessionManager) Restore(ctx context.Context) error {
	records, err := m.store.load()
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.records = records
	m.mu.Unlock()

	keys := make([]string, 0, len(records))
	for sessionID := range records {
		keys = append(keys, sessionID)
	}
	sort.Strings(keys)

	restored := 0
	updated := false
	for _, sessionID := range keys {
		rec := records[sessionID]
		if rec.Status != sessionStatusActive {
			continue
		}

		workspaceDir, err := m.ensureWorkspace(ctx, rec.ChatID, rec.TopicID, rec.WorkspaceDir)
		if err != nil {
			log.Warn().Err(err).Str("session_id", rec.SessionID).Msg("restore session workspace failed")
			rec.Status = sessionStatusError
			rec.UpdatedAt = nowRFC3339()
			m.mu.Lock()
			m.records[sessionID] = rec
			m.mu.Unlock()
			updated = true
			continue
		}

		ts, err := m.buildTopicSession(ctx, sessionID, rec.ChatID, rec.TopicID, rec.AgentName, workspaceDir)
		if err != nil {
			log.Warn().Err(err).Str("session_id", rec.SessionID).Msg("restore session failed")
			rec.Status = sessionStatusError
			rec.UpdatedAt = nowRFC3339()
			m.mu.Lock()
			m.records[sessionID] = rec
			m.mu.Unlock()
			updated = true
			continue
		}

		m.mu.Lock()
		m.sessions[sessionID] = ts
		m.records[sessionID] = sessionRecord{
			SessionID:    sessionID,
			ChatID:       rec.ChatID,
			TopicID:      rec.TopicID,
			AgentName:    rec.AgentName,
			WorkspaceDir: workspaceDir,
			Status:       sessionStatusActive,
			UpdatedAt:    nowRFC3339(),
		}
		m.mu.Unlock()
		restored++
		updated = true
	}

	if updated {
		m.mu.Lock()
		err = m.persistLocked()
		m.mu.Unlock()
		if err != nil {
			return fmt.Errorf("persist restored sessions: %w", err)
		}
	}

	if restored > 0 {
		log.Info().Int("sessions", restored).Msg("restored relay topic sessions")
	}
	return nil
}

func (m *TopicSessionManager) buildTopicSession(ctx context.Context, sessionID string, chatID int64, topicID int, agentName, workspaceDir string) (*topicSession, error) {
	req := agentfactory.CreationRequest{
		Name:              agentName,
		WorkingDirectory:  workspaceDir,
		Stderr:            os.Stderr,
		Logger:            nil,
		PermissionHandler: defaultPermissionHandler,
	}

	ag, err := m.factory.CreateAgent(ctx, agentName, req)
	if err != nil {
		return nil, fmt.Errorf("creating agent %q: %w", agentName, err)
	}

	sessionSvc := session.InMemoryService()
	sess, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: fmt.Sprintf("norma-relay-topic-%d", topicID),
		UserID:  sessionID,
	})
	if err != nil {
		if closer, ok := ag.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("creating session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        fmt.Sprintf("norma-relay-topic-%d", topicID),
		Agent:          ag,
		SessionService: sessionSvc,
	})
	if err != nil {
		if closer, ok := ag.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("creating runner: %w", err)
	}

	return &topicSession{
		sessionID:    sessionID,
		topicID:      topicID,
		agentName:    agentName,
		agent:        ag,
		runner:       r,
		sessionSvc:   sessionSvc,
		sess:         sess.Session,
		chatID:       chatID,
		tgClient:     m.tgClient,
		workspaceDir: workspaceDir,
	}, nil
}

func (m *TopicSessionManager) ensureWorkspace(ctx context.Context, chatID int64, topicID int, existingPath string) (string, error) {
	relayDir := filepath.Join(m.workingDir, ".norma")
	workspacesDir := filepath.Join(relayDir, "relay-workspaces")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		return "", fmt.Errorf("create workspaces dir: %w", err)
	}

	workspaceDir := existingPath
	if strings.TrimSpace(workspaceDir) == "" {
		workspaceDir = filepath.Join(workspacesDir, fmt.Sprintf("topic-%d-%d", chatID, topicID))
	}

	if fi, err := os.Stat(workspaceDir); err == nil && fi.IsDir() {
		return workspaceDir, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat workspace dir %q: %w", workspaceDir, err)
	}

	branchName := fmt.Sprintf("norma/relay/%d/%d", chatID, topicID)
	if _, err := git.MountWorktree(ctx, m.workingDir, workspaceDir, branchName, "HEAD"); err != nil {
		return "", fmt.Errorf("mount worktree: %w", err)
	}

	return workspaceDir, nil
}

func (m *TopicSessionManager) cleanupWorkspace(ctx context.Context, workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}
	if err := git.RemoveWorktree(ctx, m.workingDir, workspaceDir); err != nil {
		log.Warn().Err(err).Str("workspace", workspaceDir).Msg("failed to remove worktree")
		return err
	}
	return nil
}

func (m *TopicSessionManager) CommitWorkspace(ctx context.Context, chatID int64, topicID int) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no session for topic %d", topicID)
	}

	workspaceDir := ts.workspaceDir
	if workspaceDir == "" {
		return fmt.Errorf("no workspace for topic %d", topicID)
	}

	statusOut, err := git.GitRunCmdOutput(ctx, workspaceDir, "git", "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("read workspace status: %w", err)
	}
	status := strings.TrimSpace(statusOut)
	if status == "" {
		return nil
	}

	if err := git.GitRunCmdErr(ctx, workspaceDir, "git", "add", "-A"); err != nil {
		return fmt.Errorf("stage workspace changes: %w", err)
	}

	commitMsg := fmt.Sprintf("chore: relay session %d/%d", chatID, topicID)
	if err := git.GitRunCmdErr(ctx, workspaceDir, "git", "commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("commit workspace changes: %w", err)
	}

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

func (m *TopicSessionManager) processMessage(ctx context.Context, ts *topicSession, text string) {
	responseDraftID := int(ts.draftCounter.Add(1))
	eventsDraftID := int(ts.draftCounter.Add(1))
	userContent := genai.NewContentFromText(text, genai.RoleUser)
	userID := m.sessionID(ts.chatID, ts.topicID)

	var result strings.Builder

	for ev, err := range ts.runner.Run(ctx, userID, ts.sess.ID(), userContent, agent.RunConfig{}) {
		if err != nil {
			log.Error().Err(err).Int("topic_id", ts.topicID).Msg("agent run error")
			_ = m.sendMessageToTopic(ctx, ts.chatID, ts.topicID, responseDraftID, fmt.Sprintf("error: %v", err))
			return
		}
		if ev == nil {
			continue
		}
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part == nil {
					continue
				}
				if part.FunctionCall != nil && part.FunctionCall.Name == acpToolCallEvent {
					title := extractToolTitle(part.FunctionCall.Args)
					if title == "" {
						title = acpToolCallEvent
					}
					_ = m.sendMessagePlainToTopic(ctx, ts.chatID, ts.topicID, eventsDraftID, fmt.Sprintf("ToolCall: %s", title))
					continue
				}
				if part.FunctionResponse != nil && part.FunctionResponse.Name == "acp_tool_call_update" {
					_ = m.sendMessagePlainToTopic(ctx, ts.chatID, ts.topicID, eventsDraftID, formatToolUpdate(part.FunctionResponse.Response))
					continue
				}
				if part.Thought {
					if part.Text != "" {
						_ = m.sendMessagePlainToTopic(ctx, ts.chatID, ts.topicID, eventsDraftID, part.Text)
					}
					continue
				}
				if part.Text != "" {
					result.WriteString(part.Text)
					_ = m.sendMessageToTopic(ctx, ts.chatID, ts.topicID, responseDraftID, result.String())
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}
}

func (m *TopicSessionManager) sendMessageToTopic(ctx context.Context, chatID int64, topicID int, draftID int, text string) error {
	parseMode := "MarkdownV2"
	req := client.SendMessageDraftJSONRequestBody{
		ChatId:          chatID,
		DraftId:         draftID,
		Text:            escapeMarkdownV2(text),
		ParseMode:       &parseMode,
		MessageThreadId: &topicID,
	}

	resp, err := m.tgClient.SendMessageDraftWithResponse(ctx, req)
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Int("topic_id", topicID).Msg("send draft with MarkdownV2 failed, retrying without parse_mode")
		req.ParseMode = nil
		resp, err = m.tgClient.SendMessageDraftWithResponse(ctx, req)
		if err != nil {
			return fmt.Errorf("sending draft to topic %d: %w", topicID, err)
		}
	}
	if resp.JSON400 != nil {
		return fmt.Errorf("sending draft to topic %d: %s", topicID, resp.JSON400.Description)
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("sending draft to topic %d: no response body", topicID)
	}
	return nil
}

func (m *TopicSessionManager) sendMessagePlainToTopic(ctx context.Context, chatID int64, topicID int, draftID int, text string) error {
	req := client.SendMessageDraftJSONRequestBody{
		ChatId:          chatID,
		DraftId:         draftID,
		Text:            text,
		MessageThreadId: &topicID,
	}

	resp, err := m.tgClient.SendMessageDraftWithResponse(ctx, req)
	if err != nil {
		return fmt.Errorf("sending plain draft to topic %d: %w", topicID, err)
	}
	if resp.JSON400 != nil {
		return fmt.Errorf("sending plain draft to topic %d: %s", topicID, resp.JSON400.Description)
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("sending plain draft to topic %d: no response body", topicID)
	}
	return nil
}

func (m *TopicSessionManager) SendMessage(chatID int64, topicID int, text string) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no session for topic %d", topicID)
	}

	m.processMessage(context.Background(), ts, text)
	return nil
}

func (m *TopicSessionManager) StopSession(chatID int64, topicID int) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	ts, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
	}
	if rec, ok := m.records[sessionID]; ok {
		rec.Status = sessionStatusStopped
		rec.UpdatedAt = nowRFC3339()
		m.records[sessionID] = rec
	}
	if err := m.persistLocked(); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to persist stopped session")
	}
	m.mu.Unlock()

	if !exists {
		return
	}
	if err := m.closeTopicSession(ts); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to close topic session")
	}
}

func (m *TopicSessionManager) StopAll() {
	m.mu.Lock()
	sessions := make([]*topicSession, 0, len(m.sessions))
	for sessionID, ts := range m.sessions {
		sessions = append(sessions, ts)
		if rec, ok := m.records[sessionID]; ok {
			rec.Status = sessionStatusStopped
			rec.UpdatedAt = nowRFC3339()
			m.records[sessionID] = rec
		}
	}
	m.sessions = make(map[string]*topicSession)
	if err := m.persistLocked(); err != nil {
		log.Warn().Err(err).Msg("failed to persist stopped sessions")
	}
	m.mu.Unlock()

	for _, ts := range sessions {
		if err := m.closeTopicSession(ts); err != nil {
			log.Warn().Err(err).Str("session_id", ts.sessionID).Msg("failed to close topic session")
		}
	}
}

func (m *TopicSessionManager) closeTopicSession(ts *topicSession) error {
	var firstErr error
	if closer, ok := ts.agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			firstErr = err
		}
	}
	if ts.workspaceDir != "" {
		if err := m.cleanupWorkspace(context.Background(), ts.workspaceDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *TopicSessionManager) ListSessionRecords() []sessionRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]sessionRecord, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

func (m *TopicSessionManager) GetSessionRecord(sessionID string) (sessionRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[sessionID]
	return rec, ok
}

func (m *TopicSessionManager) closeTopic(ctx context.Context, chatID int64, topicID int) {
	closeResp, err := m.tgClient.CloseForumTopicWithResponse(ctx, client.CloseForumTopicJSONRequestBody{
		ChatId:          chatID,
		MessageThreadId: topicID,
	})
	if err != nil {
		log.Warn().Err(err).Int64("chat_id", chatID).Int("topic_id", topicID).Msg("failed to close forum topic")
		return
	}
	if closeResp.JSON200 == nil {
		log.Warn().Int64("chat_id", chatID).Int("topic_id", topicID).Str("status", closeResp.Status()).Msg("failed to close forum topic")
	}
}

func (m *TopicSessionManager) persistLocked() error {
	copyRecords := make(map[string]sessionRecord, len(m.records))
	for k, v := range m.records {
		copyRecords[k] = v
	}
	return m.store.save(copyRecords)
}

type NewHandler struct {
	ownerStore     *auth.OwnerStore
	tgClient       client.ClientWithResponsesInterface
	sessionManager *TopicSessionManager
}

type NewHandlerParams struct {
	fx.In

	OwnerStore     *auth.OwnerStore
	TgClient       client.ClientWithResponsesInterface
	SessionManager *TopicSessionManager
}

func NewNewHandler(params NewHandlerParams) *NewHandler {
	return &NewHandler{
		ownerStore:     params.OwnerStore,
		tgClient:       params.TgClient,
		sessionManager: params.SessionManager,
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

	sessionID, topicID, err := h.sessionManager.CreateTopicSession(ctx, chatID, agentName)
	if err != nil {
		log.Error().Err(err).Str("agent", agentName).Msg("Failed to create topic with agent")
		return h.sendMessage(ctx, chatID, 0, fmt.Sprintf("Failed to create agent session: %v", err))
	}

	welcomeMsg := fmt.Sprintf("Started new %s agent session (%s).\n\nSend your messages here and I'll forward them to the agent.", agentName, sessionID)
	if err := h.sendMessage(ctx, chatID, topicID, welcomeMsg); err != nil {
		log.Error().Err(err).Msg("Failed to send welcome message")
	}

	return nil
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
