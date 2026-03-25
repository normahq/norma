package session

import (
	"context"
	"fmt"

	"github.com/metalagman/norma/internal/apps/relaymcp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type relayMCPServer struct {
	manager *Manager
	logger  zerolog.Logger
}

// NewRelayMCPServer wraps a session Manager as a RelayService.
func NewRelayMCPServer(manager *Manager) relaymcp.RelayService {
	return &relayMCPServer{
		manager: manager,
		logger:  log.With().Str("component", "relay.mcp").Logger(),
	}
}

func (s *relayMCPServer) StartAgent(ctx context.Context, chatID int64, agentName string) (relaymcp.AgentInfo, error) {
	s.logger.Info().
		Int64("chat_id", chatID).
		Str("agent", agentName).
		Msg("MCP: StartAgent called")

	sessionID, topicID, err := s.manager.CreateTopicSession(ctx, chatID, agentName)
	if err != nil {
		s.logger.Error().
			Err(err).
			Int64("chat_id", chatID).
			Str("agent", agentName).
			Msg("MCP: StartAgent failed")
		return relaymcp.AgentInfo{}, err
	}

	s.logger.Info().
		Int64("chat_id", chatID).
		Int("topic_id", topicID).
		Str("agent", agentName).
		Str("session_id", sessionID).
		Msg("MCP: StartAgent succeeded")

	return relaymcp.AgentInfo{
		SessionID: sessionID,
		AgentName: agentName,
		ChatID:    chatID,
		TopicID:   topicID,
	}, nil
}

func (s *relayMCPServer) StopAgent(_ context.Context, sessionID string) error {
	s.logger.Info().Str("session_id", sessionID).Msg("MCP: StopAgent called")

	ts, err := s.findBySessionID(sessionID)
	if err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionID).Msg("MCP: StopAgent failed - session not found")
		return err
	}

	s.manager.StopSession(ts.chatID, ts.topicID)
	s.logger.Info().Str("session_id", sessionID).Msg("MCP: StopAgent succeeded")
	return nil
}

func (s *relayMCPServer) ListAgents(_ context.Context) ([]relaymcp.AgentInfo, error) {
	infos := s.manager.ListSessions()
	out := make([]relaymcp.AgentInfo, 0, len(infos))

	s.logger.Debug().Int("count", len(infos)).Msg("MCP: ListAgents called")

	for _, info := range infos {
		out = append(out, relaymcp.AgentInfo{
			SessionID:  info.SessionID,
			AgentName:  info.AgentName,
			ChatID:     info.ChatID,
			TopicID:    info.TopicID,
			WorkingDir: info.WorkspaceDir,
			Status:     "active",
		})
	}
	return out, nil
}

func (s *relayMCPServer) GetSession(_ context.Context, sessionID string) (relaymcp.AgentInfo, error) {
	s.logger.Debug().Str("session_id", sessionID).Msg("MCP: GetSession called")

	ts, err := s.findBySessionID(sessionID)
	if err != nil {
		s.logger.Warn().Err(err).Str("session_id", sessionID).Msg("MCP: GetSession failed - session not found")
		return relaymcp.AgentInfo{}, err
	}

	return relaymcp.AgentInfo{
		SessionID:  ts.sessionID,
		AgentName:  ts.agentName,
		ChatID:     ts.chatID,
		TopicID:    ts.topicID,
		WorkingDir: ts.workspaceDir,
		Status:     "active",
	}, nil
}

func (s *relayMCPServer) findBySessionID(sessionID string) (*TopicSession, error) {
	s.manager.mu.RLock()
	defer s.manager.mu.RUnlock()
	for _, ts := range s.manager.sessions {
		if ts.sessionID == sessionID {
			return ts, nil
		}
	}
	return nil, fmt.Errorf("session %q not found", sessionID)
}
