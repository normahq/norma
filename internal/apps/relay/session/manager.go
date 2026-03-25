package session

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/metalagman/norma/internal/apps/relay/agent"
	"github.com/metalagman/norma/internal/git"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
)

// Manager manages per-topic ADK agent sessions.
type Manager struct {
	agentBuilder *agent.Builder
	workingDir   string
	tgClient     client.ClientWithResponsesInterface // Used only for topic management, NOT messaging
	workspaces   *agent.WorkspaceManager
	store        *Store

	mu       sync.RWMutex
	sessions map[string]*topicSession
	records  map[string]Record
}

const (
	sessionIDPrefix       = "relay"
	legacySessionIDPrefix = "topic"
)

// NewManager creates a session Manager.
func NewManager(agentBuilder *agent.Builder, workingDir string, tgClient client.ClientWithResponsesInterface) (*Manager, error) {
	normaDir := filepath.Join(workingDir, ".norma")
	store, err := newStore(normaDir)
	if err != nil {
		return nil, fmt.Errorf("create topic session store: %w", err)
	}

	return &Manager{
		agentBuilder: agentBuilder,
		workingDir:   workingDir,
		tgClient:     tgClient,
		workspaces:   agent.NewWorkspaceManager(workingDir),
		store:        store,
		sessions:     make(map[string]*topicSession),
		records:      make(map[string]Record),
	}, nil
}

func (m *Manager) sessionID(chatID int64, topicID int) string {
	return fmt.Sprintf("%s-%d-%d", sessionIDPrefix, chatID, topicID)
}

func (m *Manager) CreateSession(ctx context.Context, chatID int64, topicID int, agentName string) error {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	if _, exists := m.sessions[sessionID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("session already exists for topic %d", topicID)
	}
	m.mu.Unlock()

	workspaceDir, err := m.workspaces.EnsureWorkspace(ctx, sessionID, fmt.Sprintf("norma/relay/%d/%d", chatID, topicID), "")
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	built, err := m.agentBuilder.Build(ctx, sessionID, chatID, topicID, agentName, workspaceDir)
	if err != nil {
		_ = m.workspaces.CleanupWorkspace(ctx, workspaceDir)
		return err
	}

	ts := &topicSession{
		sessionID:    sessionID,
		topicID:      topicID,
		agentName:    agentName,
		agent:        built.Agent,
		runner:       built.Runner,
		sessionSvc:   built.SessionSvc,
		sess:         built.Session,
		chatID:       chatID,
		workspaceDir: workspaceDir,
	}

	m.mu.Lock()
	m.sessions[sessionID] = ts
	m.records[sessionID] = Record{
		SessionID:    sessionID,
		ChatID:       chatID,
		TopicID:      topicID,
		AgentName:    agentName,
		WorkspaceDir: workspaceDir,
		Status:       statusActive,
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

func (m *Manager) CreateTopicSession(ctx context.Context, chatID int64, agentName string) (string, int, error) {
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

func (m *Manager) Restore(ctx context.Context) error {
	records, err := m.store.Load()
	if err != nil {
		return err
	}
	normalizedRecords, recordsChanged := m.normalizeRecords(records)

	m.mu.Lock()
	m.records = normalizedRecords
	m.mu.Unlock()

	keys := make([]string, 0, len(normalizedRecords))
	for sessionID := range normalizedRecords {
		keys = append(keys, sessionID)
	}
	sort.Strings(keys)

	restored := 0
	updated := recordsChanged
	for _, sessionID := range keys {
		rec := normalizedRecords[sessionID]
		if rec.Status != statusActive {
			continue
		}

		workspaceDir, err := m.workspaces.EnsureWorkspace(ctx, sessionID, fmt.Sprintf("norma/relay/%d/%d", rec.ChatID, rec.TopicID), rec.WorkspaceDir)
		if err != nil {
			log.Warn().Err(err).Str("session_id", rec.SessionID).Msg("restore session workspace failed")
			rec.Status = statusError
			rec.UpdatedAt = nowRFC3339()
			m.mu.Lock()
			m.records[sessionID] = rec
			m.mu.Unlock()
			updated = true
			continue
		}

		built, err := m.agentBuilder.Build(ctx, sessionID, rec.ChatID, rec.TopicID, rec.AgentName, workspaceDir)
		if err != nil {
			log.Warn().Err(err).Str("session_id", rec.SessionID).Msg("restore session failed")
			rec.Status = statusError
			rec.UpdatedAt = nowRFC3339()
			m.mu.Lock()
			m.records[sessionID] = rec
			m.mu.Unlock()
			updated = true
			continue
		}

		ts := &topicSession{
			sessionID:    sessionID,
			topicID:      rec.TopicID,
			agentName:    rec.AgentName,
			agent:        built.Agent,
			runner:       built.Runner,
			sessionSvc:   built.SessionSvc,
			sess:         built.Session,
			chatID:       rec.ChatID,
			workspaceDir: workspaceDir,
		}

		m.mu.Lock()
		m.sessions[sessionID] = ts
		m.records[sessionID] = Record{
			SessionID:    sessionID,
			ChatID:       rec.ChatID,
			TopicID:      rec.TopicID,
			AgentName:    rec.AgentName,
			WorkspaceDir: workspaceDir,
			Status:       statusActive,
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

// GetOrRestoreSession retrieves an existing session or attempts to restore it from records.
// It returns nil if no session exists or can be restored.
func (m *Manager) GetOrRestoreSession(ctx context.Context, chatID int64, topicID int) (*topicSession, error) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.RLock()
	ts, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if exists {
		return ts, nil
	}

	_, hasRecord := m.GetSessionRecord(sessionID)
	if !hasRecord {
		return nil, fmt.Errorf("no session for topic %d", topicID)
	}

	log.Info().Str("session_id", sessionID).Int("topic_id", topicID).Msg("Lazily restoring topic session")
	if err := m.restoreSession(ctx, sessionID, chatID, topicID); err != nil {
		return nil, fmt.Errorf("lazy restore failed for topic %d: %w", topicID, err)
	}

	m.mu.RLock()
	ts = m.sessions[sessionID]
	m.mu.RUnlock()

	if ts == nil {
		return nil, fmt.Errorf("session restore completed but session still not found")
	}

	return ts, nil
}

func (m *Manager) StopSession(chatID int64, topicID int) {
	sessionID := m.sessionID(chatID, topicID)

	m.mu.Lock()
	ts, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
	}
	if rec, ok := m.records[sessionID]; ok {
		rec.Status = statusStopped
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

func (m *Manager) StopAll() {
	m.mu.Lock()
	sessions := make([]*topicSession, 0, len(m.sessions))
	for sessionID, ts := range m.sessions {
		sessions = append(sessions, ts)
		if rec, ok := m.records[sessionID]; ok {
			rec.Status = statusStopped
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

func (m *Manager) closeTopicSession(ts *topicSession) error {
	var firstErr error
	if closer, ok := ts.agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			firstErr = err
		}
	}
	if ts.workspaceDir != "" {
		if err := m.workspaces.CleanupWorkspace(context.Background(), ts.workspaceDir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) ListSessionRecords() []Record {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Record, 0, len(m.records))
	for _, rec := range m.records {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

func (m *Manager) GetSessionRecord(sessionID string) (Record, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[sessionID]
	if ok {
		return rec, true
	}

	if strings.HasPrefix(sessionID, sessionIDPrefix+"-") {
		legacyID := strings.Replace(sessionID, sessionIDPrefix+"-", legacySessionIDPrefix+"-", 1)
		rec, ok = m.records[legacyID]
		if ok {
			rec.SessionID = sessionID
			return rec, true
		}
	}
	if strings.HasPrefix(sessionID, legacySessionIDPrefix+"-") {
		relayID := strings.Replace(sessionID, legacySessionIDPrefix+"-", sessionIDPrefix+"-", 1)
		rec, ok = m.records[relayID]
		if ok {
			return rec, true
		}
	}
	return rec, ok
}

func (m *Manager) restoreSession(ctx context.Context, sessionID string, chatID int64, topicID int) error {
	rec, ok := m.GetSessionRecord(sessionID)
	if !ok {
		return fmt.Errorf("no persisted record for session %s", sessionID)
	}
	workspaceDir, err := m.workspaces.EnsureWorkspace(ctx, sessionID, fmt.Sprintf("norma/relay/%d/%d", chatID, topicID), rec.WorkspaceDir)
	if err != nil {
		return fmt.Errorf("restore workspace: %w", err)
	}

	built, err := m.agentBuilder.Build(ctx, sessionID, chatID, topicID, rec.AgentName, workspaceDir)
	if err != nil {
		_ = m.workspaces.CleanupWorkspace(ctx, workspaceDir)
		return fmt.Errorf("build topic session: %w", err)
	}

	ts := &topicSession{
		sessionID:    sessionID,
		topicID:      rec.TopicID,
		agentName:    rec.AgentName,
		agent:        built.Agent,
		runner:       built.Runner,
		sessionSvc:   built.SessionSvc,
		sess:         built.Session,
		chatID:       rec.ChatID,
		workspaceDir: workspaceDir,
	}

	m.mu.Lock()
	m.sessions[sessionID] = ts
	m.records[sessionID] = Record{
		SessionID:    sessionID,
		ChatID:       rec.ChatID,
		TopicID:      rec.TopicID,
		AgentName:    rec.AgentName,
		WorkspaceDir: workspaceDir,
		Status:       statusActive,
		UpdatedAt:    nowRFC3339(),
	}
	err = m.persistLocked()
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist restored session: %w", err)
	}

	log.Info().Str("session_id", sessionID).Int("topic_id", topicID).Msg("session lazily restored")
	return nil
}

func (m *Manager) closeTopic(ctx context.Context, chatID int64, topicID int) {
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

func (m *Manager) persistLocked() error {
	copyRecords := make(map[string]Record, len(m.records))
	for k, v := range m.records {
		copyRecords[k] = v
	}
	return m.store.Save(copyRecords)
}

func (m *Manager) normalizeRecords(records map[string]Record) (map[string]Record, bool) {
	out := make(map[string]Record, len(records))
	changed := false

	for key, rec := range records {
		sessionID := m.sessionID(rec.ChatID, rec.TopicID)
		if rec.SessionID != sessionID || key != sessionID {
			changed = true
		}
		rec.SessionID = sessionID

		existing, exists := out[sessionID]
		if !exists || rec.UpdatedAt > existing.UpdatedAt {
			out[sessionID] = rec
		}
	}

	return out, changed
}
