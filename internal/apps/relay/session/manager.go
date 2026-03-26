package session

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/normahq/norma/internal/adk/agentconfig"
	"github.com/normahq/norma/internal/apps/relay/agent"
	"github.com/normahq/norma/internal/git"
	"github.com/rs/zerolog"
	"github.com/tgbotkit/client"
	"go.uber.org/fx"
)

const sessionIDPrefix = "relay"

// convertAgentConfigs converts map[string]interface{} to map[string]agentconfig.Config.
func convertAgentConfigs(in map[string]interface{}) map[string]agentconfig.Config {
	out := make(map[string]agentconfig.Config, len(in))
	for k, v := range in {
		cfg, ok := v.(agentconfig.Config)
		if !ok {
			panic(fmt.Sprintf("invalid agent config type for %q: %T", k, v))
		}
		out[k] = cfg
	}
	return out
}

// Manager manages per-topic ADK agent sessions (in-memory only, no persistence).
type Manager struct {
	agentBuilder *agent.Builder
	workingDir   string
	tgClient     client.ClientWithResponsesInterface
	workspaces   *agent.WorkspaceManager
	logger       zerolog.Logger

	// agentConfigs stores normalized agent configs (type is generic_acp after normalization)
	agentConfigs map[string]agentconfig.Config

	rootCtx    context.Context
	rootCancel context.CancelFunc

	mu       sync.RWMutex
	sessions map[string]*TopicSession
}

// ManagerParams provides dependencies for Manager.
type ManagerParams struct {
	fx.In

	LC           fx.Lifecycle
	AgentBuilder *agent.Builder
	WorkingDir   string
	TGClient     client.ClientWithResponsesInterface
	Logger       zerolog.Logger
	AgentConfigs map[string]interface{} `name:"relay_agent_configs"`
}

// NewManager creates a session Manager.
func NewManager(p ManagerParams) (*Manager, error) {
	rootCtx, rootCancel := context.WithCancel(context.Background())

	m := &Manager{
		agentBuilder: p.AgentBuilder,
		workingDir:   p.WorkingDir,
		tgClient:     p.TGClient,
		workspaces:   agent.NewWorkspaceManager(p.WorkingDir),
		logger:       p.Logger.With().Str("component", "relay.session_manager").Logger(),
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		sessions:     make(map[string]*TopicSession),
		agentConfigs: convertAgentConfigs(p.AgentConfigs),
	}

	p.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			m.logger.Info().Msg("session manager started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			m.logger.Info().Int("active_sessions", len(m.sessions)).Msg("session manager stopping")
			m.rootCancel()
			m.StopAll()
			return nil
		},
	})

	return m, nil
}

// ValidateAgent checks if an agent with the given name exists in the config.
func (m *Manager) ValidateAgent(agentName string) error {
	if _, ok := m.agentConfigs[agentName]; !ok {
		return fmt.Errorf("agent %q not found in registry", agentName)
	}
	return nil
}

func (m *Manager) sessionID(chatID int64, topicID int) string {
	return fmt.Sprintf("%s-%d-%d", sessionIDPrefix, chatID, topicID)
}

// SessionBranchName returns the git branch name for a relay session.
func (m *Manager) SessionBranchName(sessionID string) string {
	return fmt.Sprintf("norma/relay/%s", sessionID)
}

// CreateSession builds an agent for the given topic and stores it in memory.
func (m *Manager) CreateSession(ctx context.Context, chatID int64, topicID int, agentName string) error {
	sessionID := m.sessionID(chatID, topicID)

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("creating session")

	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		m.logger.Warn().Str("session_id", sessionID).Msg("session already exists")
		return fmt.Errorf("session already exists for topic %d", topicID)
	}
	m.mu.Unlock()

	branchName := m.SessionBranchName(sessionID)
	workspaceDir, err := m.workspaces.EnsureWorkspace(ctx, sessionID, branchName, "")
	if err != nil {
		m.logger.Error().Err(err).Str("session_id", sessionID).Msg("failed to create workspace")
		return fmt.Errorf("create workspace: %w", err)
	}
	m.logger.Debug().Str("session_id", sessionID).Str("workspace", workspaceDir).Msg("workspace created")

	built, err := m.agentBuilder.Build(m.rootCtx, sessionID, chatID, topicID, agentName, workspaceDir)
	if err != nil {
		m.logger.Error().Err(err).Str("session_id", sessionID).Str("agent", agentName).Msg("failed to build agent")
		_ = m.workspaces.CleanupWorkspace(ctx, workspaceDir)
		return err
	}

	ts := &TopicSession{
		sessionID:    sessionID,
		topicID:      topicID,
		agentName:    agentName,
		agent:        built.Agent,
		runner:       built.Runner,
		sessionSvc:   built.SessionSvc,
		sess:         built.Session,
		chatID:       chatID,
		workspaceDir: workspaceDir,
		branchName:   branchName,
	}

	m.mu.Lock()
	m.sessions[sessionID] = ts
	m.mu.Unlock()

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("session created successfully")

	return nil
}

// CreateTopicSession creates a Telegram forum topic and an agent session for it.
// It first validates the agent can be built before creating the topic to avoid orphaned topics.
func (m *Manager) CreateTopicSession(ctx context.Context, chatID int64, agentName string) (string, int, error) {
	m.logger.Info().
		Int64("chat_id", chatID).
		Str("agent", agentName).
		Msg("creating topic session")

	// First validate agent can be built - this checks config without creating anything
	if err := m.ValidateAgent(agentName); err != nil {
		m.logger.Error().Err(err).Str("agent", agentName).Msg("agent validation failed, not creating topic")
		return "", 0, fmt.Errorf("agent %q not available: %w", agentName, err)
	}

	// Agent validated, now create the topic
	topicName := fmt.Sprintf("Relay: %s", agentName)
	createTopicResp, err := m.tgClient.CreateForumTopicWithResponse(ctx, client.CreateForumTopicJSONRequestBody{
		ChatId: chatID,
		Name:   topicName,
	})
	if err != nil {
		m.logger.Error().Err(err).Int64("chat_id", chatID).Msg("failed to create forum topic")
		return "", 0, fmt.Errorf("creating forum topic: %w", err)
	}
	if createTopicResp.JSON200 == nil {
		m.logger.Error().
			Int64("chat_id", chatID).
			Str("status", createTopicResp.Status()).
			Msg("forum topic creation returned non-200")
		return "", 0, fmt.Errorf("failed to create forum topic: %s", createTopicResp.Status())
	}

	topic := createTopicResp.JSON200.Result
	topicID := topic.MessageThreadId

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Msg("forum topic created, creating agent session")

	if err := m.CreateSession(ctx, chatID, topicID, agentName); err != nil {
		m.logger.Error().Err(err).Int64("chat_id", chatID).Int("topic_id", topicID).Msg("failed to create session, cleaning up topic")
		m.closeTopic(ctx, chatID, topicID)
		return "", 0, fmt.Errorf("creating agent session: %w", err)
	}

	sessionID := m.sessionID(chatID, topicID)
	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("topic session created successfully")

	return sessionID, topicID, nil
}

// GetSession returns the in-memory session for the given chat/topic.
func (m *Manager) GetSession(chatID int64, topicID int) (*TopicSession, error) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts := m.sessions[sessionID]
	m.mu.RUnlock()

	if ts == nil {
		m.logger.Debug().
			Int64("chat_id", chatID).
			Int("topic_id", topicID).
			Str("session_id", sessionID).
			Int("active_sessions", len(m.sessions)).
			Msg("session not found")
		return nil, fmt.Errorf("no session for topic %d", topicID)
	}

	return ts, nil
}

// EnsureSession returns the existing session or creates a new one if it doesn't exist.
// For topic 0 (main orchestrator), it creates the session without a forum topic.
func (m *Manager) EnsureSession(ctx context.Context, chatID int64, topicID int, agentName string) (*TopicSession, error) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts := m.sessions[sessionID]
	m.mu.RUnlock()

	if ts != nil {
		m.logger.Debug().
			Int64("chat_id", chatID).
			Int("topic_id", topicID).
			Str("session_id", sessionID).
			Msg("returning existing session")
		return ts, nil
	}

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("creating new session via EnsureSession")

	// Create new session
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if ts = m.sessions[sessionID]; ts != nil {
		return ts, nil
	}

	branchName := m.SessionBranchName(sessionID)
	workspaceDir, err := m.workspaces.EnsureWorkspace(ctx, sessionID, branchName, "")
	if err != nil {
		m.logger.Error().Err(err).Str("session_id", sessionID).Msg("failed to create workspace")
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	built, err := m.agentBuilder.Build(m.rootCtx, sessionID, chatID, topicID, agentName, workspaceDir)
	if err != nil {
		m.logger.Error().Err(err).Str("session_id", sessionID).Str("agent", agentName).Msg("failed to build agent")
		_ = m.workspaces.CleanupWorkspace(ctx, workspaceDir)
		return nil, err
	}

	ts = &TopicSession{
		sessionID:    sessionID,
		topicID:      topicID,
		agentName:    agentName,
		agent:        built.Agent,
		runner:       built.Runner,
		sessionSvc:   built.SessionSvc,
		sess:         built.Session,
		chatID:       chatID,
		workspaceDir: workspaceDir,
		branchName:   branchName,
	}

	m.sessions[sessionID] = ts

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("session created via EnsureSession")

	return ts, nil
}

// StopSession removes a session from memory and cleans up.
func (m *Manager) StopSession(chatID int64, topicID int) {
	sessionID := m.sessionID(chatID, topicID)

	m.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("session_id", sessionID).
		Msg("stopping session")

	m.mu.Lock()
	ts, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if !exists {
		m.logger.Warn().Str("session_id", sessionID).Msg("session not found for stop")
		return
	}
	if err := m.closeTopicSession(ts); err != nil {
		m.logger.Warn().Err(err).Str("session_id", sessionID).Msg("failed to close topic session")
	}

	m.logger.Info().Str("session_id", sessionID).Msg("session stopped")
}

// StopAll closes all sessions.
func (m *Manager) StopAll() {
	m.mu.Lock()
	sessions := make([]*TopicSession, 0, len(m.sessions))
	for _, ts := range m.sessions {
		sessions = append(sessions, ts)
	}
	m.sessions = make(map[string]*TopicSession)
	m.mu.Unlock()

	m.logger.Info().Int("count", len(sessions)).Msg("stopping all sessions")

	for _, ts := range sessions {
		if err := m.closeTopicSession(ts); err != nil {
			m.logger.Warn().Err(err).Str("session_id", ts.sessionID).Msg("failed to close topic session")
		}
	}

	m.logger.Info().Msg("all sessions stopped")
}

// CloseTopic closes the Telegram forum topic via API.
func (m *Manager) CloseTopic(ctx context.Context, chatID int64, topicID int) {
	m.closeTopic(ctx, chatID, topicID)
}

// ListSessions returns info about all active sessions.
func (m *Manager) ListSessions() []TopicSessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]TopicSessionInfo, 0, len(m.sessions))
	for _, ts := range m.sessions {
		out = append(out, TopicSessionInfo{
			SessionID:    ts.sessionID,
			AgentName:    ts.agentName,
			ChatID:       ts.chatID,
			TopicID:      ts.topicID,
			WorkspaceDir: ts.workspaceDir,
			BranchName:   ts.branchName,
		})
	}
	return out
}

type TopicSessionInfo struct {
	SessionID    string
	AgentName    string
	ChatID       int64
	TopicID      int
	WorkspaceDir string
	BranchName   string
}

func (m *Manager) closeTopicSession(ts *TopicSession) error {
	var firstErr error
	if closer, ok := ts.agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			firstErr = err
		}
	}
	if ts.workspaceDir != "" {
		if err := m.workspaces.CleanupWorkspace(m.rootCtx, ts.workspaceDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) closeTopic(ctx context.Context, chatID int64, topicID int) {
	m.logger.Debug().Int64("chat_id", chatID).Int("topic_id", topicID).Msg("closing forum topic")

	closeResp, err := m.tgClient.CloseForumTopicWithResponse(ctx, client.CloseForumTopicJSONRequestBody{
		ChatId:          chatID,
		MessageThreadId: topicID,
	})
	if err != nil {
		m.logger.Warn().Err(err).Int64("chat_id", chatID).Int("topic_id", topicID).Msg("failed to close forum topic")
		return
	}
	if closeResp.JSON200 == nil {
		m.logger.Warn().Int64("chat_id", chatID).Int("topic_id", topicID).Str("status", closeResp.Status()).Msg("failed to close forum topic")
	}
}

func (m *Manager) CommitWorkspace(ctx context.Context, chatID int64, topicID int) error {
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
	if status := statusOut; len(status) == 0 {
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
