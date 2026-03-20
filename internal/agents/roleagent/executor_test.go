package roleagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRole struct {
	name         string
	inputSchema  string
	outputSchema string
	promptStr    string
	mapReqErr    error
	mapRespErr   error
}

func (r *testRole) Name() string { return r.name }
func (r *testRole) Schemas() SchemaPair {
	return SchemaPair{InputSchema: r.inputSchema, OutputSchema: r.outputSchema}
}
func (r *testRole) Prompt(_ AgentRequest) (string, error) { return r.promptStr, nil }
func (r *testRole) MapRequest(req AgentRequest) (any, error) {
	if r.mapReqErr != nil {
		return nil, r.mapReqErr
	}
	return req, nil
}
func (r *testRole) MapResponse(outBytes []byte) (AgentResponse, error) {
	if r.mapRespErr != nil {
		return AgentResponse{}, r.mapRespErr
	}
	var resp AgentResponse
	err := json.Unmarshal(outBytes, &resp)
	return resp, err
}

func TestNewExecutor(t *testing.T) {
	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  []string{"custom-acp", "--stdio"},
		},
	}

	executor := NewExecutor(cfg)
	assert.NotNil(t, executor)
}

func TestExecutorWithPermissionHandler(t *testing.T) {
	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  []string{"custom-acp", "--stdio"},
		},
	}

	executor := NewExecutor(cfg)
	customHandler := func(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		return acp.RequestPermissionResponse{}, nil
	}

	result := executor.WithPermissionHandler(customHandler)
	assert.Same(t, executor, result)
}

func TestExecutorCarriesMCPServers(t *testing.T) {
	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  []string{"custom-acp", "--stdio"},
		},
		MCPServers: map[string]agentconfig.MCPServerConfig{
			"tasks": {
				Type: agentconfig.MCPServerTypeStdio,
				Cmd:  []string{"norma", "mcp", "tasks"},
			},
		},
	}

	executor := NewExecutor(cfg)
	assert.NotNil(t, executor)
	assert.Len(t, cfg.MCPServers, 1)
}

func TestExecutor_Run(t *testing.T) {
	workingDir := t.TempDir()

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  helperACPCommand(t, `{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`),
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "plan",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 1, Name: "plan"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	ctx := context.Background()
	var events bytes.Buffer
	stdout, stderr, exitCode, err := executor.Run(ctx, role, req, nil, io.Discard, io.Discard, &events)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, events.String())

	var resp AgentResponse
	err = json.Unmarshal(stdout, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)

	eventLines := parseJSONLines(t, events.Bytes())
	require.NotEmpty(t, eventLines)
	first := eventLines[0]
	assert.Equal(t, "event", first["type"])
	assert.NotEmpty(t, first["logged_at"])
	assert.NotNil(t, t, first["event"])
}

func TestExecutor_RunHandlesChunkedStructuredOutput(t *testing.T) {
	workingDir := t.TempDir()

	response := "Let me inspect first.\n" +
		`{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  helperACPCommandChunked(t, response, 9),
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "plan",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 1, Name: "plan"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	var events bytes.Buffer
	stdout, stderr, exitCode, runErr := executor.Run(context.Background(), role, req, nil, io.Discard, io.Discard, &events)
	require.NoError(t, runErr)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)

	var resp AgentResponse
	err := json.Unmarshal(stdout, &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "success", resp.Summary.Text)
	assert.Equal(t, "done", resp.Progress.Title)
}

func TestExecutor_RunRejectsTrailingContentAfterJSON(t *testing.T) {
	workingDir := t.TempDir()

	response := "Let me inspect first.\n" +
		`{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}` +
		"\n```\nextra"

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  helperACPCommandChunked(t, response, 7),
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "plan",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 1, Name: "plan"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	var events bytes.Buffer
	_, _, exitCode, runErr := executor.Run(context.Background(), role, req, nil, io.Discard, io.Discard, &events)
	require.Error(t, runErr)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, runErr.Error(), "validate structured output")
	assert.Contains(t, runErr.Error(), "non-whitespace content after JSON")
}

func TestExecutor_RunReturnsErrorOnMappingFailure(t *testing.T) {
	tests := []struct {
		name           string
		runID          string
		stepName       string
		iteration      int
		helperResponse string
		mapReqErr      error
		mapRespErr     error
		errContains    []string
	}{
		{
			name:           "map_request_failure",
			runID:          "run-map-req",
			stepName:       "mapreq",
			iteration:      2,
			helperResponse: `{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`,
			mapReqErr:      errors.New("map request failed"),
			errContains:    []string{"map request", "map request failed"},
		},
		{
			name:           "map_response_failure",
			runID:          "run-map-resp",
			stepName:       "mapresp",
			iteration:      3,
			helperResponse: `{}`,
			mapRespErr:     errors.New("map response failed"),
			errContains:    []string{"map agent response", "map response failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := t.TempDir()

			cfg := ExecutorConfig{
				AgentConfig: config.AgentConfig{
					Type: config.AgentTypeGenericACP,
					Cmd:  helperACPCommand(t, tt.helperResponse),
				},
			}

			executor := NewExecutor(cfg)
			role := &testRole{
				name:         tt.name,
				inputSchema:  "{}",
				outputSchema: "{}",
				promptStr:    "Test prompt for " + tt.name,
				mapReqErr:    tt.mapReqErr,
				mapRespErr:   tt.mapRespErr,
			}

			req := RoleRequest{
				Run:  RunInfo{ID: tt.runID, Iteration: tt.iteration},
				Step: StepInfo{Index: tt.iteration, Name: tt.stepName},
				Paths: RequestPaths{
					WorkspaceDir: workingDir,
					RunDir:       workingDir,
				},
			}

			_, _, exitCode, err := executor.Run(context.Background(), role, req, nil, io.Discard, io.Discard, io.Discard)
			require.Error(t, err)
			assert.Equal(t, 0, exitCode)
			for _, msg := range tt.errContains {
				assert.Contains(t, err.Error(), msg)
			}
		})
	}
}

func TestExecutor_RunWritesErrorEventLogOnError(t *testing.T) {
	workingDir := t.TempDir()

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  []string{"/non/existent/binary"},
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "plan",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 1, Name: "plan"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	ctx := context.Background()
	var stderr bytes.Buffer
	_, _, exitCode, err := executor.Run(ctx, role, req, nil, io.Discard, &stderr, io.Discard)
	assert.Error(t, err)
	assert.NotEqual(t, 0, exitCode)
}

func TestResolveWorkingDirectory(t *testing.T) {
	tests := []struct {
		name         string
		workspaceDir string
		runDir       string
		expected     string
	}{
		{
			name:         "uses workspace_dir when set",
			workspaceDir: "/workspace",
			runDir:       "/run",
			expected:     "/workspace",
		},
		{
			name:         "falls back to run_dir when workspace empty",
			workspaceDir: "",
			runDir:       "/run",
			expected:     "/run",
		},
		{
			name:         "trims whitespace from workspace",
			workspaceDir: "  /workspace  ",
			runDir:       "/run",
			expected:     "/workspace",
		},
		{
			name:         "trims whitespace from run_dir",
			workspaceDir: "",
			runDir:       "  /run  ",
			expected:     "/run",
		},
		{
			name:         "returns empty when both empty",
			workspaceDir: "",
			runDir:       "",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveWorkingDirectory(tt.workspaceDir, tt.runDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToPascal(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"plan", "Plan"},
		{"do", "Do"},
		{"check", "Check"},
		{"act", "Act"},
		{"", ""},
		{"  ", ""},
		{"P", "P"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toPascal(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultPermissionHandler(t *testing.T) {
	req := acp.RequestPermissionRequest{
		Options: []acp.PermissionOption{
			{OptionId: "allow-1", Kind: acp.PermissionOptionKindAllowOnce},
		},
	}

	resp, err := defaultPermissionHandler(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, acp.NewRequestPermissionOutcomeSelected("allow-1"), resp.Outcome)
}

func TestADKEventLogWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := newADKEventLogWriter(&buf)

	assert.NotNil(t, writer)
	assert.Equal(t, 0, writer.seq)

	entry := adkEventLogEntry{
		Seq:      1,
		Type:     "test",
		LoggedAt: "2024-01-01T00:00:00Z",
	}
	err := writer.write(entry)
	assert.NoError(t, err)

	assert.Equal(t, 1, writer.nextSeq())
}

func helperACPCommand(t *testing.T, response string) []string {
	t.Helper()
	return []string{
		"env",
		"GO_WANT_AGENT_ACP_HELPER=1",
		"GO_HELPER_RESPONSE=" + response,
		os.Args[0],
		"-test.run=TestAgentACPHelperProcess",
		"--",
	}
}

func helperACPCommandChunked(t *testing.T, response string, chunkSize int) []string {
	t.Helper()
	return []string{
		"env",
		"GO_WANT_AGENT_ACP_HELPER=1",
		"GO_HELPER_RESPONSE=" + response,
		"GO_HELPER_CHUNK_SIZE=" + strconv.Itoa(chunkSize),
		os.Args[0],
		"-test.run=TestAgentACPHelperProcess",
		"--",
	}
}

func TestAgentACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_ACP_HELPER") != "1" {
		t.Skip("skipping ACP helper process test")
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		switch req.Method {
		case acp.AgentMethodInitialize:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": acp.ProtocolVersionNumber,
				},
			})
		case acp.AgentMethodSessionNew:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"sessionId": "session-1",
				},
			})
		case acp.AgentMethodSessionPrompt:
			responseText := os.Getenv("GO_HELPER_RESPONSE")
			chunkSize := 0
			if raw := strings.TrimSpace(os.Getenv("GO_HELPER_CHUNK_SIZE")); raw != "" {
				parsed, parseErr := strconv.Atoi(raw)
				if parseErr == nil && parsed > 0 {
					chunkSize = parsed
				}
			}

			if chunkSize <= 0 {
				emitACPTextChunk(encoder, responseText)
			} else {
				for start := 0; start < len(responseText); start += chunkSize {
					end := start + chunkSize
					if end > len(responseText) {
						end = len(responseText)
					}
					emitACPTextChunk(encoder, responseText[start:end])
				}
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"stopReason": "end_turn",
				},
			})
		}
	}
	os.Exit(0)
}

func emitACPTextChunk(encoder *json.Encoder, text string) {
	if encoder == nil {
		return
	}
	_ = encoder.Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  acp.ClientMethodSessionUpdate,
		"params": map[string]any{
			"sessionId": "session-1",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]any{
					"type": "text",
					"text": text,
				},
			},
		},
	})
}

func parseJSONLines(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lines := make([]map[string]any, 0)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatalf("unmarshal json line %q: %v", text, err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan json lines: %v", err)
	}
	return lines
}

func TestAppendVisibleTextFromEvent(t *testing.T) {
	var buf strings.Builder
	appendVisibleTextFromEvent(&buf, nil)
	assert.Empty(t, buf.String())
}

func TestADKEventLogParts(t *testing.T) {
	parts := adkEventLogParts(nil)
	assert.Empty(t, parts)
}

func TestADKEventLogWriterNil(t *testing.T) {
	var writer *adkEventLogWriter

	assert.Nil(t, writer.write(adkEventLogEntry{}))
	assert.Nil(t, writer.write(adkEventLogEntry{}))
}

func TestEventWriterWriteError(t *testing.T) {
	var buf bytes.Buffer
	writer := newADKEventLogWriter(&buf)

	err := writer.WriteError(nil)
	assert.NoError(t, err)

	err = writer.WriteError(errors.New("test error"))
	assert.NoError(t, err)
	assert.NotEmpty(t, buf.String())
}

func TestEventWriterWriteEventNilEvent(t *testing.T) {
	var buf bytes.Buffer
	writer := newADKEventLogWriter(&buf)

	err := writer.WriteEvent(nil)
	assert.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestExecutor_RunPreservesRoleSpecificPayloads(t *testing.T) {
	workingDir := t.TempDir()

	response := `{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]},"plan_output":{"task_id":"task-1","goal":"test goal"}}`

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  helperACPCommand(t, response),
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "plan",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 1, Name: "plan"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	stdout, _, exitCode, err := executor.Run(context.Background(), role, req, nil, io.Discard, io.Discard, io.Discard)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.NotEmpty(t, stdout)

	var resp AgentResponse
	err = json.Unmarshal(stdout, &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.NotEmpty(t, resp.PlanOutput, "plan_output should be preserved")
	assert.JSONEq(t, `{"task_id":"task-1","goal":"test goal"}`, string(resp.PlanOutput))
}

func TestExecutor_RunPreservesDoOutput(t *testing.T) {
	workingDir := t.TempDir()

	response := `{"status":"ok","summary":{"text":"done"},"progress":{"title":"work complete","details":[]},"do_output":{"execution":{"executed_step_ids":["DO-1"],"skipped_step_ids":[]}}}`

	cfg := ExecutorConfig{
		AgentConfig: config.AgentConfig{
			Type: config.AgentTypeGenericACP,
			Cmd:  helperACPCommand(t, response),
		},
	}

	executor := NewExecutor(cfg)
	role := &testRole{
		name:         "do",
		inputSchema:  "{}",
		outputSchema: "{}",
		promptStr:    "Test prompt",
	}

	req := RoleRequest{
		Run:  RunInfo{ID: "run-1", Iteration: 1},
		Step: StepInfo{Index: 2, Name: "do"},
		Paths: RequestPaths{
			WorkspaceDir: workingDir,
			RunDir:       workingDir,
		},
	}

	stdout, _, exitCode, err := executor.Run(context.Background(), role, req, nil, io.Discard, io.Discard, io.Discard)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	var resp AgentResponse
	err = json.Unmarshal(stdout, &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.DoOutput, "do_output should be preserved")
	assert.JSONEq(t, `{"execution":{"executed_step_ids":["DO-1"],"skipped_step_ids":[]}}`, string(resp.DoOutput))
}
