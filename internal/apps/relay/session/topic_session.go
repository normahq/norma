package session

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

// topicSession represents a single Telegram topic's ADK agent session.
type topicSession struct {
	sessionID    string
	topicID      int
	agentName    string
	agent        agent.Agent
	runner       *runner.Runner
	sessionSvc   session.Service
	sess         session.Session
	chatID       int64
	workspaceDir string
}
