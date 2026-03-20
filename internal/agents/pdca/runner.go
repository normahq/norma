// Package pdca provides the PDCA workflow runner.
package pdca

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/adk/structuredio"
	"github.com/metalagman/norma/internal/agents/pdca/contracts"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Runner executes an agent with a normalized request.
type Runner interface {
	Run(ctx context.Context, req contracts.AgentRequest, stdout, stderr, eventsLog io.Writer) (outBytes, errBytes []byte, exitCode int, err error)
}

// NewRunner constructs a runner for the given agent config and role.
func NewRunner(cfg config.AgentConfig, role contracts.Role, mcpServers map[string]agentconfig.MCPServerConfig) (Runner, error) {
	return &adkRunner{
		cfg:        cfg,
		role:       role,
		mcpServers: mcpServers,
	}, nil
}

type adkRunner struct {
	cfg        config.AgentConfig
	role       contracts.Role
	mcpServers map[string]agentconfig.MCPServerConfig
}

func (r *adkRunner) Run(ctx context.Context, req contracts.AgentRequest, stdout, stderr, eventsLog io.Writer) ([]byte, []byte, int, error) {
	l := log.With().
		Str("role", r.role.Name()).
		Str("run_id", req.Run.ID).
		Int("step_index", req.Step.Index).
		Str("step_name", req.Step.Name).
		Logger()
	ctx = l.WithContext(ctx)
	eventWriter := newADKEventLogWriter(eventsLog)

	// 1. Map request to JSON input for the role.
	input, err := r.role.MapRequest(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("map request: %w", err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("marshal input JSON: %w", err)
	}

	// 2. Resolve system instruction (role-specific prompt).
	systemInstruction, err := r.role.Prompt(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("generate role prompt: %w", err)
	}

	// 3. Resolve working directory.
	workingDirectory := strings.TrimSpace(req.Paths.WorkspaceDir)
	if workingDirectory == "" {
		workingDirectory = strings.TrimSpace(req.Paths.RunDir)
	}

	// 4. Create ephemeral inner agent via factory.
	agentRegistry := map[string]config.AgentConfig{
		r.role.Name(): r.cfg,
	}
	factory := agentfactory.NewFactory(agentRegistry)
	if len(r.mcpServers) > 0 {
		factory = agentfactory.NewFactoryWithMCPServers(agentRegistry, r.mcpServers)
	}
	creationReq := agentfactory.CreationRequest{
		Name:              "Norma" + toPascal(req.Step.Name) + "Agent",
		Description:       "Norma " + req.Step.Name + " agent",
		SystemInstruction: systemInstruction,
		WorkingDirectory:  workingDirectory,
		Stdout:            stdout,
		Stderr:            stderr,
		Logger:            &l,
		PermissionHandler: defaultACPPermissionHandler,
	}

	inner, err := factory.CreateAgent(ctx, r.role.Name(), creationReq)
	if err != nil {
		return nil, nil, 1, fmt.Errorf("failed to create inner agent: %w", err)
	}
	if closer, ok := inner.(interface{ Close() error }); ok {
		defer func() {
			if closeErr := closer.Close(); closeErr != nil {
				l.Warn().Err(closeErr).Msg("failed to close inner agent runtime")
			}
		}()
	}

	// 5. Wrap with structured I/O agent.
	a, err := structuredio.NewAgent(inner,
		structuredio.WithInputSchema(r.role.InputSchema()),
		structuredio.WithOutputSchema(r.role.OutputSchema()),
	)
	if err != nil {
		return nil, nil, 1, fmt.Errorf("failed to create structured wrapper: %w", err)
	}

	// 6. Execute via ADK runner.
	sessionService := session.InMemoryService()
	adkRunner, err := runner.New(runner.Config{
		AppName:        "norma",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to create adk runner: %w", err)
	}

	userID := "norma-user"
	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "norma",
		UserID:  userID,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to create session: %w", err)
	}

	userContent := genai.NewContentFromText(string(inputJSON), genai.RoleUser)
	events := adkRunner.Run(ctx, userID, sess.Session.ID(), userContent, agent.RunConfig{})

	var accumulatedOutput strings.Builder
	var lastExitCode int
	for ev, err := range events {
		if err != nil {
			if writeErr := eventWriter.WriteError(err); writeErr != nil {
				l.Warn().Err(writeErr).Msg("failed to write ADK error event log")
			}
			if exitErr, ok := err.(interface{ ExitCode() int }); ok {
				lastExitCode = exitErr.ExitCode()
			} else {
				lastExitCode = 1
			}
			return nil, nil, lastExitCode, fmt.Errorf("agent execution error: %w", err)
		}
		if writeErr := eventWriter.WriteEvent(ev); writeErr != nil {
			l.Warn().Err(writeErr).Msg("failed to write ADK event log")
		}
		appendVisibleTextFromEvent(&accumulatedOutput, ev)
	}

	outputText := strings.TrimSpace(accumulatedOutput.String())
	if outputText == "" {
		return nil, nil, 0, fmt.Errorf("no output from agent")
	}
	rawOutput := []byte(outputText)

	// 7. Map final response.
	agentResp, err := r.role.MapResponse(rawOutput)
	if err != nil {
		return rawOutput, nil, 0, fmt.Errorf("map agent response: %w", err)
	}

	// Final normalization to ensure it is clean JSON.
	normalized, err := json.Marshal(agentResp)
	if err != nil {
		return rawOutput, nil, 0, fmt.Errorf("marshal normalized response: %w", err)
	}

	return normalized, nil, 0, nil
}

func appendVisibleTextFromEvent(out *strings.Builder, ev *session.Event) {
	if out == nil || ev == nil || ev.Content == nil {
		return
	}
	for _, part := range ev.Content.Parts {
		if part == nil || part.Thought || part.Text == "" {
			continue
		}
		out.WriteString(part.Text)
	}
}

func toPascal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func defaultACPPermissionHandler(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindAllowOnce || option.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindRejectOnce || option.Kind == acp.PermissionOptionKindRejectAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}

type adkEventLogWriter struct {
	writer io.Writer
	seq    int
}

type adkEventLogEntry struct {
	Seq      int               `json:"seq"`
	Type     string            `json:"type"`
	LoggedAt string            `json:"logged_at"`
	Event    *adkEventLogEvent `json:"event,omitempty"`
	Error    *adkEventLogError `json:"error,omitempty"`
}

type adkEventLogError struct {
	Message string `json:"message"`
	Type    string `json:"error_type"`
}

type adkEventLogEvent struct {
	InvocationID       string            `json:"invocation_id,omitempty"`
	Partial            bool              `json:"partial"`
	TurnComplete       bool              `json:"turn_complete"`
	FinishReason       string            `json:"finish_reason,omitempty"`
	Author             string            `json:"author,omitempty"`
	Branch             string            `json:"branch,omitempty"`
	ContentRole        string            `json:"content_role,omitempty"`
	LongRunningToolIDs []string          `json:"long_running_tool_ids,omitempty"`
	Usage              *adkEventLogUsage `json:"usage,omitempty"`
	Parts              []adkEventLogPart `json:"parts,omitempty"`
}

type adkEventLogUsage struct {
	PromptTokenCount     int32 `json:"prompt_token_count,omitempty"`
	CandidatesTokenCount int32 `json:"candidates_token_count,omitempty"`
	TotalTokenCount      int32 `json:"total_token_count,omitempty"`
	CachedTokenCount     int32 `json:"cached_token_count,omitempty"`
}

type adkEventLogPart struct {
	Text             string                       `json:"text,omitempty"`
	Thought          bool                         `json:"thought,omitempty"`
	FunctionCall     *adkEventLogFunctionCall     `json:"function_call,omitempty"`
	FunctionResponse *adkEventLogFunctionResponse `json:"function_response,omitempty"`
}

type adkEventLogFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

type adkEventLogFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Response map[string]any `json:"response,omitempty"`
}

func newADKEventLogWriter(writer io.Writer) *adkEventLogWriter {
	return &adkEventLogWriter{writer: writer}
}

func (w *adkEventLogWriter) WriteEvent(ev *session.Event) error {
	if w == nil || w.writer == nil || ev == nil {
		return nil
	}

	eventPayload := adkEventLogEvent{
		InvocationID:       strings.TrimSpace(ev.InvocationID),
		Partial:            ev.Partial,
		TurnComplete:       ev.TurnComplete,
		Author:             strings.TrimSpace(ev.Author),
		Branch:             strings.TrimSpace(ev.Branch),
		LongRunningToolIDs: ev.LongRunningToolIDs,
	}
	if ev.FinishReason != "" {
		eventPayload.FinishReason = string(ev.FinishReason)
	}
	if ev.Content != nil {
		eventPayload.ContentRole = ev.Content.Role
		eventPayload.Parts = adkEventLogParts(ev.Content.Parts)
	}
	if ev.UsageMetadata != nil {
		eventPayload.Usage = &adkEventLogUsage{
			PromptTokenCount:     ev.UsageMetadata.PromptTokenCount,
			CandidatesTokenCount: ev.UsageMetadata.CandidatesTokenCount,
			TotalTokenCount:      ev.UsageMetadata.TotalTokenCount,
			CachedTokenCount:     ev.UsageMetadata.CachedContentTokenCount,
		}
	}

	return w.write(adkEventLogEntry{
		Seq:      w.nextSeq(),
		Type:     "event",
		LoggedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Event:    &eventPayload,
	})
}

func (w *adkEventLogWriter) WriteError(err error) error {
	if w == nil || w.writer == nil || err == nil {
		return nil
	}

	return w.write(adkEventLogEntry{
		Seq:      w.nextSeq(),
		Type:     "error",
		LoggedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Error: &adkEventLogError{
			Message: err.Error(),
			Type:    fmt.Sprintf("%T", err),
		},
	})
}

func (w *adkEventLogWriter) write(entry adkEventLogEntry) error {
	if w == nil || w.writer == nil {
		return nil
	}
	encoder := json.NewEncoder(w.writer)
	return encoder.Encode(entry)
}

func (w *adkEventLogWriter) nextSeq() int {
	w.seq++
	return w.seq
}

func adkEventLogParts(parts []*genai.Part) []adkEventLogPart {
	out := make([]adkEventLogPart, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			continue
		}
		p := adkEventLogPart{
			Text:    part.Text,
			Thought: part.Thought,
		}
		if part.FunctionCall != nil {
			p.FunctionCall = &adkEventLogFunctionCall{
				ID:   part.FunctionCall.ID,
				Name: part.FunctionCall.Name,
				Args: part.FunctionCall.Args,
			}
		}
		if part.FunctionResponse != nil {
			p.FunctionResponse = &adkEventLogFunctionResponse{
				ID:       part.FunctionResponse.ID,
				Name:     part.FunctionResponse.Name,
				Response: part.FunctionResponse.Response,
			}
		}

		if p.Text == "" && !p.Thought && p.FunctionCall == nil && p.FunctionResponse == nil {
			continue
		}
		out = append(out, p)
	}
	return out
}
