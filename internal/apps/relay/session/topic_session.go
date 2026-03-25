package session

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
)

// TopicSession represents a single Telegram topic's ADK agent session.
type TopicSession struct {
	sessionID    string
	topicID      int
	agentName    string
	agent        agent.Agent
	runner       *runner.Runner
	sessionSvc   session.Service
	sess         session.Session
	chatID       int64
	workspaceDir string
	branchName   string
}

func (s *TopicSession) GetRunner() *runner.Runner {
	return s.runner
}

func (s *TopicSession) GetSessionID() string {
	return s.sess.ID()
}

func (s *TopicSession) GetWorkspaceDir() string {
	return s.workspaceDir
}
