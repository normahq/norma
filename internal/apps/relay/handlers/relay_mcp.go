package handlers

import (
	"context"

	"github.com/metalagman/norma/internal/apps/relaymcp"
)

type relayMCPServer struct {
	manager *TopicSessionManager
}

func NewRelayMCPServer(manager *TopicSessionManager) relaymcp.RelayService {
	return &relayMCPServer{manager: manager}
}

func (s *relayMCPServer) StartAgent(ctx context.Context, agentName string) (string, error) {
	// For MCP-initiated sessions, we need default chat/topic IDs
	// This would typically come from the MCP request
	return "", nil
}

func (s *relayMCPServer) StopAgent(ctx context.Context, sessionID string) error {
	return nil
}

func (s *relayMCPServer) ListAgents(ctx context.Context) ([]relaymcp.AgentInfo, error) {
	return nil, nil
}

func (s *relayMCPServer) GetSession(ctx context.Context, sessionID string) (relaymcp.AgentInfo, error) {
	return relaymcp.AgentInfo{}, nil
}
