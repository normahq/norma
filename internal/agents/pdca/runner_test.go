package pdca

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/adk/structuredio"
	"github.com/metalagman/norma/internal/agents/roleagent"
	"github.com/metalagman/norma/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyRole struct{}

func (r *dummyRole) Name() string { return "plan" }
func (r *dummyRole) Schemas() roleagent.SchemaPair {
	return roleagent.SchemaPair{InputSchema: "{}", OutputSchema: "{}"}
}
func (r *dummyRole) Prompt(_ roleagent.AgentRequest) (string, error) { return "prompt", nil }
func (r *dummyRole) MapRequest(req roleagent.AgentRequest) (any, error) {
	return req, nil
}
func (r *dummyRole) MapResponse(outBytes []byte) (roleagent.AgentResponse, error) {
	var resp roleagent.AgentResponse
	err := json.Unmarshal(outBytes, &resp)
	return resp, err
}

type failingMapRole struct {
	dummyRole
}

func (r *failingMapRole) MapResponse(_ []byte) (roleagent.AgentResponse, error) {
	return roleagent.AgentResponse{}, errors.New("map failed")
}

func TestNewRunner(t *testing.T) {
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  []string{"custom-acp", "--stdio"},
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, runner)
}

func TestNewRunnerCarriesMCPServers(t *testing.T) {
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  []string{"custom-acp", "--stdio"},
	}
	mcpServers := map[string]agentconfig.MCPServerConfig{
		tasksMCPServerName: {
			Type: agentconfig.MCPServerTypeStdio,
			Cmd:  []string{"norma", "mcp", "tasks"},
		},
	}

	runner, err := NewRunner(cfg, &dummyRole{}, mcpServers)
	require.NoError(t, err)

	typed, ok := runner.(*adkRunner)
	require.True(t, ok)
	require.Len(t, typed.mcpServers, 1)
	assert.Equal(t, mcpServers[tasksMCPServerName], typed.mcpServers[tasksMCPServerName])
}

func TestAinvokeRunner_Run(t *testing.T) {
	workingDir, err := os.MkdirTemp("", "norma-agent-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(workingDir) }()

	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommand(t, `{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`),
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + workingDir + `","run_dir":"` + workingDir + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"plan_input":{"task":{"id":"task-1"}}}`)

	ctx := context.Background()
	var events bytes.Buffer
	stdout, stderr, exitCode, err := runner.Run(ctx, reqJSON, io.Discard, io.Discard, &events)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, events.String())

	var resp roleagent.AgentResponse
	err = json.Unmarshal(stdout, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)

	eventLines := parseJSONLines(t, events.Bytes())
	require.NotEmpty(t, eventLines)
	first := eventLines[0]
	assert.Equal(t, "event", first["type"])
	assert.NotEmpty(t, first["logged_at"])
	assert.NotNil(t, first["event"])
}

func TestAinvokeRunner_RunHandlesChunkedStructuredOutput(t *testing.T) {
	workingDir, err := os.MkdirTemp("", "norma-agent-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(workingDir) }()

	response := "Let me inspect first.\n" +
		`{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommandChunked(t, response, 9),
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + workingDir + `","run_dir":"` + workingDir + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"plan_input":{"task":{"id":"task-1"}}}`)

	var events bytes.Buffer
	stdout, stderr, exitCode, runErr := runner.Run(context.Background(), reqJSON, io.Discard, io.Discard, &events)
	require.NoError(t, runErr)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)

	var resp roleagent.AgentResponse
	err = json.Unmarshal(stdout, &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "success", resp.Summary.Text)
	assert.Equal(t, "done", resp.Progress.Title)
}

func TestAinvokeRunner_RunRejectsTrailingContentAfterMarkdownFence(t *testing.T) {
	workingDir, err := os.MkdirTemp("", "norma-agent-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(workingDir) }()

	response := "Let me inspect first.\n" +
		`{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}` +
		"\n```\nextra"
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommandChunked(t, response, 7),
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + workingDir + `","run_dir":"` + workingDir + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"plan_input":{"task":{"id":"task-1"}}}`)

	var events bytes.Buffer
	_, _, exitCode, runErr := runner.Run(context.Background(), reqJSON, io.Discard, io.Discard, &events)
	require.Error(t, runErr)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, runErr.Error(), "validate structured output")
	assert.NotContains(t, runErr.Error(), "map agent response")
}

func TestAinvokeRunner_RunWritesErrorToStderr(t *testing.T) {
	// For ACP agents, errors are usually reported via the protocol or connection failure.
	// Here we simulate a connection failure (binary not found).
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  []string{"/non/existent/binary"},
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + t.TempDir() + `","run_dir":"` + t.TempDir() + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"]}`)

	ctx := context.Background()
	var stderr bytes.Buffer
	_, _, exitCode, err := runner.Run(ctx, reqJSON, io.Discard, &stderr, io.Discard)
	assert.Error(t, err)
	assert.NotEqual(t, 0, exitCode)
}

func TestAinvokeRunner_RunReturnsErrorWhenResponseMappingFails(t *testing.T) {
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommand(t, "{}"),
	}

	runner, err := NewRunner(cfg, &failingMapRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + t.TempDir() + `","run_dir":"` + t.TempDir() + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"]}`)

	_, _, exitCode, err := runner.Run(context.Background(), reqJSON, io.Discard, io.Discard, io.Discard)
	require.Error(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, err.Error(), "map agent response")
	assert.Contains(t, err.Error(), "map failed")
}

func TestAinvokeRunner_RunWritesErrorEventLogOnPromptFailure(t *testing.T) {
	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommandWithPromptError(t, "prompt failed"),
	}

	runner, err := NewRunner(cfg, &dummyRole{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + t.TempDir() + `","run_dir":"` + t.TempDir() + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"]}`)

	var events bytes.Buffer
	_, _, exitCode, err := runner.Run(context.Background(), reqJSON, io.Discard, io.Discard, &events)
	require.Error(t, err)
	assert.NotEqual(t, 0, exitCode)

	lines := parseJSONLines(t, events.Bytes())
	require.NotEmpty(t, lines)

	last := lines[len(lines)-1]
	assert.Equal(t, "error", last["type"])
	assert.NotEmpty(t, last["logged_at"])
	errObj, ok := last["error"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, errObj["message"], "prompt failed")
	assert.NotEmpty(t, errObj["error_type"])
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

func helperACPCommandWithPromptError(t *testing.T, message string) []string {
	t.Helper()
	return []string{
		"env",
		"GO_WANT_AGENT_ACP_HELPER=1",
		"GO_HELPER_PROMPT_ERROR=" + message,
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
		return
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
			if promptErr := strings.TrimSpace(os.Getenv("GO_HELPER_PROMPT_ERROR")); promptErr != "" {
				_ = encoder.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error": map[string]any{
						"code":    -32000,
						"message": promptErr,
					},
				})
				continue
			}
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
			// Finalize prompt
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

func TestRunnerWrapsErrorsWithPercentW(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("outer: %w", structuredio.ErrStructuredOutputSchemaValidation)
	assert.True(t, errors.Is(err, structuredio.ErrStructuredOutputSchemaValidation),
		"errors.Is should work through %%w wrapping")
	assert.True(t, errors.Is(err, structuredio.ErrStructuredIOSchemaValidation),
		"errors.Is should work through %%w wrapping to umbrella error")

	err = fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", structuredio.ErrStructuredIOSchemaValidation))
	assert.True(t, errors.Is(err, structuredio.ErrStructuredIOSchemaValidation),
		"errors.Is should work through nested %%w wrapping")
}

type roleWithPlanOutput struct {
	dummyRole
}

func (r *roleWithPlanOutput) MapResponse(outBytes []byte) (roleagent.AgentResponse, error) {
	var resp roleagent.AgentResponse
	err := json.Unmarshal(outBytes, &resp)
	if err != nil {
		return resp, err
	}
	resp.PlanOutput = []byte(`{"acceptance_criteria":{"effective":[{"id":"AC-1","text":"test","origin":"baseline","checks":[]}]},"work_plan":{"timebox_minutes":30,"do_steps":[{"id":"DO-1","text":"test step","targets_ac_ids":["AC-1"]}]}}`)
	return resp, nil
}

func TestAinvokeRunner_RunPreservesPlanOutput(t *testing.T) {
	workingDir, err := os.MkdirTemp("", "norma-pdca-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(workingDir) }()

	cfg := config.AgentConfig{
		Type: config.AgentTypeGenericACP,
		Cmd:  helperACPCommand(t, `{"status":"ok","summary":{"text":"success"},"progress":{"title":"done","details":[]}}`),
	}

	runner, err := NewRunner(cfg, &roleWithPlanOutput{}, nil)
	require.NoError(t, err)

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"text"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"` + workingDir + `","run_dir":"` + workingDir + `"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"plan_input":{"task":{"id":"task-1"}}}`)

	ctx := context.Background()
	stdout, stderr, exitCode, err := runner.Run(ctx, reqJSON, io.Discard, io.Discard, io.Discard)
	assert.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stderr)
	assert.NotEmpty(t, stdout)

	var resp roleagent.AgentResponse
	err = json.Unmarshal(stdout, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	require.NotNil(t, resp.PlanOutput, "plan_output should be preserved")
}
