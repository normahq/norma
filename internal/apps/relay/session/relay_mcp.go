package session

import (
	"context"
	"fmt"

	"github.com/metalagman/norma/internal/apps/relaymcp"
)

type relayMCPServer struct {
	manager *Manager
}

// NewRelayMCPServer wraps a session Manager as a RelayService.
func NewRelayMCPServer(manager *Manager) relaymcp.RelayService {
	return &relayMCPServer{manager: manager}
}

func (s *relayMCPServer) StartAgent(ctx context.Context, chatID int64, agentName string) (relaymcp.AgentInfo, error) {
	sessionID, _, err := s.manager.CreateTopicSession(ctx, chatID, agentName)
	if err != nil {
		return relaymcp.AgentInfo{}, err
	}
	rec, ok := s.manager.GetSessionRecord(sessionID)
	if !ok {
		return relaymcp.AgentInfo{}, fmt.Errorf("session %q was created but not found", sessionID)
	}
	return relaymcp.AgentInfo{
		SessionID:  rec.SessionID,
		AgentName:  rec.AgentName,
		ChatID:     rec.ChatID,
		TopicID:    rec.TopicID,
		WorkingDir: rec.WorkspaceDir,
		Status:     rec.Status,
	}, nil
}

func (s *relayMCPServer) StopAgent(ctx context.Context, sessionID string) error {
	rec, ok := s.manager.GetSessionRecord(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	s.manager.StopSession(rec.ChatID, rec.TopicID)
	return nil
}

func (s *relayMCPServer) ListAgents(ctx context.Context) ([]relaymcp.AgentInfo, error) {
	records := s.manager.ListSessionRecords()
	out := make([]relaymcp.AgentInfo, 0, len(records))
	for _, rec := range records {
		out = append(out, relaymcp.AgentInfo{
			SessionID:  rec.SessionID,
			AgentName:  rec.AgentName,
			ChatID:     rec.ChatID,
			TopicID:    rec.TopicID,
			WorkingDir: rec.WorkspaceDir,
			Status:     rec.Status,
		})
	}
	return out, nil
}

func (s *relayMCPServer) GetSession(ctx context.Context, sessionID string) (relaymcp.AgentInfo, error) {
	rec, ok := s.manager.GetSessionRecord(sessionID)
	if !ok {
		return relaymcp.AgentInfo{}, fmt.Errorf("session %q not found", sessionID)
	}
	return relaymcp.AgentInfo{
		SessionID:  rec.SessionID,
		AgentName:  rec.AgentName,
		ChatID:     rec.ChatID,
		TopicID:    rec.TopicID,
		WorkingDir: rec.WorkspaceDir,
		Status:     rec.Status,
	}, nil
}
