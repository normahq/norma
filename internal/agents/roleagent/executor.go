package roleagent

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
	"github.com/rs/zerolog/log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// RoleRequest contains the metadata for executing a role step.
type RoleRequest struct {
	RunID        string `json:"run_id"`
	RunIteration int    `json:"run_iteration"`
	StepIndex    int    `json:"step_index"`
	StepName     string `json:"step_name"`
	WorkspaceDir string `json:"workspace_dir"`
	RunDir       string `json:"run_dir"`
}

// ExecutorConfig holds the configuration for creating an Executor.
type ExecutorConfig struct {
	AgentConfig agentconfig.Config
	MCPServers  map[string]agentconfig.MCPServerConfig
}

// Executor runs a single role step using an ADK agent.
type Executor struct {
	cfg        ExecutorConfig
	permission func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)
}

// NewExecutor creates a new Executor with the given configuration.
func NewExecutor(cfg ExecutorConfig) *Executor {
	return &Executor{
		cfg:        cfg,
		permission: defaultPermissionHandler,
	}
}

func (e *Executor) WithPermissionHandler(fn func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error)) *Executor {
	e.permission = fn
	return e
}

func (e *Executor) Run(ctx context.Context, role RoleContract, req RoleRequest, roleInput AgentRequest, stdout, stderr, eventsLog io.Writer) ([]byte, []byte, int, error) {
	l := log.With().
		Str("role", role.Name()).
		Str("run_id", req.RunID).
		Int("step_index", req.StepIndex).
		Str("step_name", req.StepName).
		Logger()
	ctx = l.WithContext(ctx)
	eventWriter := newADKEventLogWriter(eventsLog)

	input, err := role.MapRequest(roleInput)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("map request: %w", err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("marshal input JSON: %w", err)
	}

	systemInstruction, err := role.Prompt(roleInput)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("generate role prompt: %w", err)
	}

	workingDirectory := resolveWorkingDirectory(req.WorkspaceDir, req.RunDir)

	agentRegistry := map[string]agentconfig.Config{
		role.Name(): e.cfg.AgentConfig,
	}
	factory := agentfactory.NewFactory(agentRegistry)
	if len(e.cfg.MCPServers) > 0 {
		factory = agentfactory.NewFactoryWithMCPServers(agentRegistry, e.cfg.MCPServers)
	}
	creationReq := agentfactory.CreationRequest{
		Name:              "Norma" + toPascal(req.StepName) + "Agent",
		Description:       "Norma " + req.StepName + " agent",
		SystemInstruction: systemInstruction,
		WorkingDirectory:  workingDirectory,
		Stdout:            stdout,
		Stderr:            stderr,
		Logger:            &l,
		PermissionHandler: e.permission,
	}

	inner, err := factory.CreateAgent(ctx, role.Name(), creationReq)
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

	schemas := role.Schemas()
	a, err := structuredio.NewAgent(inner,
		structuredio.WithInputSchema(schemas.InputSchema),
		structuredio.WithOutputSchema(schemas.OutputSchema),
	)
	if err != nil {
		return nil, nil, 1, fmt.Errorf("failed to create structured wrapper: %w", err)
	}

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

	agentResp, err := role.MapResponse(rawOutput)
	if err != nil {
		return rawOutput, nil, 0, fmt.Errorf("map agent response: %w", err)
	}

	normalized, err := json.Marshal(agentResp)
	if err != nil {
		return rawOutput, nil, 0, fmt.Errorf("marshal normalized response: %w", err)
	}

	return normalized, nil, 0, nil
}

func resolveWorkingDirectory(workspaceDir, runDir string) string {
	workingDirectory := strings.TrimSpace(workspaceDir)
	if workingDirectory == "" {
		workingDirectory = strings.TrimSpace(runDir)
	}
	return workingDirectory
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

func defaultPermissionHandler(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
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
