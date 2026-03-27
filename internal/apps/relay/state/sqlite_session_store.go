package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type sqliteSessionStore struct {
	db *sql.DB
}

func (s *sqliteSessionStore) Upsert(ctx context.Context, record SessionRecord) error {
	sessionID := strings.TrimSpace(record.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	if strings.TrimSpace(record.Status) == "" {
		record.Status = SessionStatusActive
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO relay_session_metadata (
			session_id, chat_id, topic_id, agent_name, workspace_dir, branch_name, status, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			chat_id = excluded.chat_id,
			topic_id = excluded.topic_id,
			agent_name = excluded.agent_name,
			workspace_dir = excluded.workspace_dir,
			branch_name = excluded.branch_name,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		sessionID,
		record.ChatID,
		record.TopicID,
		record.AgentName,
		record.WorkspaceDir,
		record.BranchName,
		record.Status,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upsert relay session %q: %w", sessionID, err)
	}

	return nil
}

func (s *sqliteSessionStore) GetByChatTopic(ctx context.Context, chatID int64, topicID int) (SessionRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT session_id, chat_id, topic_id, agent_name, workspace_dir, branch_name, status
		FROM relay_session_metadata
		WHERE chat_id = ? AND topic_id = ?`,
		chatID, topicID,
	)

	var record SessionRecord
	if err := row.Scan(
		&record.SessionID,
		&record.ChatID,
		&record.TopicID,
		&record.AgentName,
		&record.WorkspaceDir,
		&record.BranchName,
		&record.Status,
	); err != nil {
		if err == sql.ErrNoRows {
			return SessionRecord{}, false, nil
		}
		return SessionRecord{}, false, fmt.Errorf("get relay session by chat/topic: %w", err)
	}

	return record, true, nil
}

func (s *sqliteSessionStore) DeleteBySessionID(ctx context.Context, sessionID string) error {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM relay_session_metadata
		WHERE session_id = ?`,
		trimmed,
	); err != nil {
		return fmt.Errorf("delete relay session %q: %w", trimmed, err)
	}
	return nil
}

func (s *sqliteSessionStore) List(ctx context.Context) ([]SessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, chat_id, topic_id, agent_name, workspace_dir, branch_name, status
		FROM relay_session_metadata
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list relay sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionRecord, 0)
	for rows.Next() {
		var record SessionRecord
		if err := rows.Scan(
			&record.SessionID,
			&record.ChatID,
			&record.TopicID,
			&record.AgentName,
			&record.WorkspaceDir,
			&record.BranchName,
			&record.Status,
		); err != nil {
			return nil, fmt.Errorf("scan relay session: %w", err)
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relay sessions: %w", err)
	}

	return out, nil
}
