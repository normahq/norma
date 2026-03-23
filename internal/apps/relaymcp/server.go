package relaymcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName     = "norma-relay"
	serverVersion  = "1.0.0"
	defaultAddress = "127.0.0.1:9090"
)

type ToolError struct {
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type ToolOutcome struct {
	OK    bool       `json:"ok"`
	Error *ToolError `json:"error,omitempty"`
}

func okOutcome() ToolOutcome {
	return ToolOutcome{OK: true}
}

func validationFailure(operation string, message string) (*mcp.CallToolResult, ToolOutcome) {
	return failure(operation, "validation_error", message)
}

func backendFailure(operation string, err error) (*mcp.CallToolResult, ToolOutcome) {
	return failure(operation, "backend_error", err.Error())
}

func failure(operation string, code string, message string) (*mcp.CallToolResult, ToolOutcome) {
	return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: message}},
		}, ToolOutcome{
			OK: false,
			Error: &ToolError{
				Operation: operation,
				Code:      code,
				Message:   message,
			},
		}
}

type RelayService interface {
	StartAgent(ctx context.Context, agentName string) (string, error)
	StopAgent(ctx context.Context, sessionID string) error
	ListAgents(ctx context.Context) ([]AgentInfo, error)
	GetSession(ctx context.Context, sessionID string) (AgentInfo, error)
}

type AgentInfo struct {
	SessionID  string
	AgentName  string
	ChatID     int64
	TopicID    int
	WorkingDir string
	Status     string
}

func Run(ctx context.Context, svc RelayService) error {
	server, err := NewServer(svc)
	if err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

func RunHTTP(ctx context.Context, svc RelayService, addr string) error {
	result, err := StartHTTPServer(ctx, svc, addr)
	if err != nil {
		return err
	}
	<-ctx.Done()
	return result.Close()
}

type HTTPServerResult struct {
	Addr  string
	Close func() error
}

func StartHTTPServer(ctx context.Context, svc RelayService, addr string) (*HTTPServerResult, error) {
	if svc == nil {
		return nil, fmt.Errorf("service is required")
	}
	address := strings.TrimSpace(addr)
	if address == "" {
		address = defaultAddress
	}

	getServer := func(_ *http.Request) *mcp.Server {
		server, err := NewServer(svc)
		if err != nil {
			return nil
		}
		return server
	}

	handler := mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{})

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", address, err)
	}

	actualAddr := listener.Addr().String()
	httpServer := &http.Server{Handler: handler}

	go func() {
		<-ctx.Done()
		_ = httpServer.Close()
	}()

	go func() {
		_ = httpServer.Serve(listener)
	}()

	return &HTTPServerResult{
		Addr: actualAddr,
		Close: func() error {
			return httpServer.Close()
		},
	}, nil
}

func NewServer(svc RelayService) (*mcp.Server, error) {
	if svc == nil {
		return nil, fmt.Errorf("service is required")
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		nil,
	)

	srv := &service{svc: svc}
	srv.registerTools(server)
	return server, nil
}

type service struct {
	svc RelayService
}

func (s *service) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "norma.relay.start_agent",
		Description: "Start a new relay agent session",
	}, s.startAgent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "norma.relay.stop_agent",
		Description: "Stop a running relay agent session",
	}, s.stopAgent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "norma.relay.list_agents",
		Description: "List all active relay agent sessions",
	}, s.listAgents)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "norma.relay.get_agent",
		Description: "Get information about a specific relay agent session",
	}, s.getAgent)
}

type startAgentInput struct {
	AgentName string `json:"agent_name" jsonschema:"name of the agent to start"`
}

type startAgentOutput struct {
	ToolOutcome
	SessionID string `json:"session_id,omitempty"`
}

func (s *service) startAgent(ctx context.Context, _ *mcp.CallToolRequest, in startAgentInput) (*mcp.CallToolResult, startAgentOutput, error) {
	if strings.TrimSpace(in.AgentName) == "" {
		result, out := validationFailure("norma.relay.start_agent", "agent_name is required")
		return result, startAgentOutput{ToolOutcome: out}, nil
	}

	sessionID, err := s.svc.StartAgent(ctx, in.AgentName)
	if err != nil {
		result, out := backendFailure("norma.relay.start_agent", err)
		return result, startAgentOutput{ToolOutcome: out}, nil
	}

	return nil, startAgentOutput{
		ToolOutcome: okOutcome(),
		SessionID:   sessionID,
	}, nil
}

type stopAgentInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID to stop"`
}

func (s *service) stopAgent(ctx context.Context, _ *mcp.CallToolRequest, in stopAgentInput) (*mcp.CallToolResult, ToolOutcome, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		result, out := validationFailure("norma.relay.stop_agent", "session_id is required")
		return result, out, nil
	}

	if err := s.svc.StopAgent(ctx, in.SessionID); err != nil {
		result, out := backendFailure("norma.relay.stop_agent", err)
		return result, out, nil
	}

	return nil, okOutcome(), nil
}

func (s *service) listAgents(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ToolOutcome, error) {
	agents, err := s.svc.ListAgents(ctx)
	if err != nil {
		result, out := backendFailure("norma.relay.list_agents", err)
		return result, out, nil
	}

	if len(agents) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No active agent sessions"}},
		}, okOutcome(), nil
	}

	var sb strings.Builder
	sb.WriteString("Active agent sessions:\n")
	for _, a := range agents {
		fmt.Fprintf(&sb, "- %s: %s (chat=%d, topic=%d, status=%s)\n",
			a.SessionID, a.AgentName, a.ChatID, a.TopicID, a.Status)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, okOutcome(), nil
}

type getAgentInput struct {
	SessionID string `json:"session_id" jsonschema:"session ID to retrieve"`
}

func (s *service) getAgent(ctx context.Context, _ *mcp.CallToolRequest, in getAgentInput) (*mcp.CallToolResult, ToolOutcome, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		result, out := validationFailure("norma.relay.get_agent", "session_id is required")
		return result, out, nil
	}

	agent, err := s.svc.GetSession(ctx, in.SessionID)
	if err != nil {
		result, out := validationFailure("norma.relay.get_agent", fmt.Sprintf("session not found: %v", err))
		return result, out, nil
	}

	text := fmt.Sprintf("Session: %s\nAgent: %s\nChat: %d\nTopic: %d\nStatus: %s\nWorking Dir: %s",
		agent.SessionID, agent.AgentName, agent.ChatID, agent.TopicID, agent.Status, agent.WorkingDir)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, okOutcome(), nil
}
