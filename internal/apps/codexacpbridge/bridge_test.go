package codexacpbridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

const (
	codexToolName      = "codex"
	codexReplyToolName = "codex-reply"
	mcpTransportStdio  = "stdio"
	mcpTransportHTTP   = "http"
)

func TestBuildCodexMCPCommand(t *testing.T) {
	got := buildCodexMCPCommand(Options{})
	want := []string{"codex", "mcp-server"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("buildCodexMCPCommand() = %v, want %v", got, want)
	}
}

func TestValidateMCPServersEmpty(t *testing.T) {
	result, err := validateMCPServers(nil)
	if err != nil {
		t.Fatalf("validateMCPServers(nil) error = %v, want nil", err)
	}
	if result != nil {
		t.Fatalf("validateMCPServers(nil) = %v, want nil", result)
	}

	result, err = validateMCPServers([]acp.McpServer{})
	if err != nil {
		t.Fatalf("validateMCPServers([]) error = %v, want nil", err)
	}
	if result != nil {
		t.Fatalf("validateMCPServers([]) = %v, want nil", result)
	}
}

func TestValidateMCPServersStdio(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{
				Name:    "my-server",
				Command: "node",
				Args:    []string{"server.js"},
			},
		},
	}
	result, err := validateMCPServers(servers)
	if err != nil {
		t.Fatalf("validateMCPServers(stdio) error = %v, want nil", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if _, ok := result["my-server"]; !ok {
		t.Fatalf("result does not contain 'my-server'")
	}
}

func TestValidateMCPServersHttp(t *testing.T) {
	servers := []acp.McpServer{
		{
			Http: &acp.McpServerHttp{
				Name: "http-server",
				Url:  "http://localhost:8080",
			},
		},
	}
	result, err := validateMCPServers(servers)
	if err != nil {
		t.Fatalf("validateMCPServers(http) error = %v, want nil", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if _, ok := result["http-server"]; !ok {
		t.Fatalf("result does not contain 'http-server'")
	}
}

func TestValidateMCPServersRejectsSse(t *testing.T) {
	servers := []acp.McpServer{
		{
			Sse: &acp.McpServerSse{
				Name: "sse-server",
				Url:  "http://localhost:8080/sse",
			},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(sse) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "sse") {
		t.Fatalf("error = %q, want containing 'sse'", err.Error())
	}
}

func TestValidateMCPServersRejectsNoTransport(t *testing.T) {
	servers := []acp.McpServer{
		{},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(no transport) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "transport") {
		t.Fatalf("error = %q, want containing 'transport'", err.Error())
	}
}

func TestValidateMCPServersRejectsDuplicateNames(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{Name: "server1", Command: "echo"},
		},
		{
			Stdio: &acp.McpServerStdio{Name: "server1", Command: "echo"},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(duplicate) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error = %q, want containing 'duplicated'", err.Error())
	}
}

func TestValidateMCPServersRejectsEmptyName(t *testing.T) {
	servers := []acp.McpServer{
		{
			Stdio: &acp.McpServerStdio{Name: "", Command: "echo"},
		},
	}
	_, err := validateMCPServers(servers)
	if err == nil {
		t.Fatal("validateMCPServers(empty name) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("error = %q, want containing 'name is required'", err.Error())
	}
}

func TestBuildCodexMCPCommandDoesNotInjectModelConfig(t *testing.T) {
	got := buildCodexMCPCommand(Options{
		CodexModel: "gpt-5.4",
	})
	want := []string{"codex", "mcp-server"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("buildCodexMCPCommand() = %v, want %v", got, want)
	}
}

func TestRunProxyRejectsInvalidCodexSandbox(t *testing.T) {
	ctx := context.Background()
	err := RunProxy(
		ctx,
		t.TempDir(),
		Options{CodexSandbox: "invalid"},
		strings.NewReader(""),
		io.Discard,
		io.Discard,
	)
	if err == nil {
		t.Fatal("RunProxy() error = nil, want invalid codex sandbox error")
	}
	if !strings.Contains(err.Error(), "invalid codex sandbox") {
		t.Fatalf("RunProxy() error = %q, want invalid codex sandbox", err.Error())
	}
}

func TestBuildCodexToolInvocationIncludesCodexConfigOnInitialCall(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"",
		"/tmp/work",
		"hello",
		codexToolConfig{
			ApprovalPolicy:        "on-request",
			BaseInstructions:      "base",
			CompactPrompt:         "compact",
			Config:                map[string]any{"foo": "bar"},
			DeveloperInstructions: "dev",
			Model:                 "gpt-5.2-codex",
			Profile:               "team",
			Sandbox:               "workspace-write",
		},
		"",
		nil,
	)
	if toolName != codexToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexToolName)
	}
	if got := mapArgString(args, "prompt"); got != "hello" {
		t.Fatalf("prompt = %q, want %q", got, "hello")
	}
	if got := mapArgString(args, "cwd"); got != "/tmp/work" {
		t.Fatalf("cwd = %q, want %q", got, "/tmp/work")
	}
	if got := mapArgString(args, "approval-policy"); got != "on-request" {
		t.Fatalf("approval-policy = %q, want %q", got, "on-request")
	}
	if got := mapArgString(args, "base-instructions"); got != "base" {
		t.Fatalf("base-instructions = %q, want %q", got, "base")
	}
	if got := mapArgString(args, "compact-prompt"); got != "compact" {
		t.Fatalf("compact-prompt = %q, want %q", got, "compact")
	}
	if got := mapArgString(args, "developer-instructions"); got != "dev" {
		t.Fatalf("developer-instructions = %q, want %q", got, "dev")
	}
	if got := mapArgString(args, "model"); got != "gpt-5.2-codex" {
		t.Fatalf("model = %q, want %q", got, "gpt-5.2-codex")
	}
	if got := mapArgString(args, "profile"); got != "team" {
		t.Fatalf("profile = %q, want %q", got, "team")
	}
	if got := mapArgString(args, "sandbox"); got != "workspace-write" {
		t.Fatalf("sandbox = %q, want %q", got, "workspace-write")
	}
	cfgArg, ok := args["config"].(map[string]any)
	if !ok {
		t.Fatalf("config type = %T, want map[string]any", args["config"])
	}
	if got, ok := cfgArg["foo"].(string); !ok || got != "bar" {
		t.Fatalf("config.foo = %v, want %q", cfgArg["foo"], "bar")
	}
}

func TestBuildCodexToolInvocationReplyOmitsCodexConfig(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"thread-1",
		"/tmp/work",
		"hello",
		codexToolConfig{
			Model:   "gpt-5.2-codex",
			Sandbox: "workspace-write",
		},
		"",
		nil,
	)
	if toolName != codexReplyToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexReplyToolName)
	}
	if got := mapArgString(args, "threadId"); got != "thread-1" {
		t.Fatalf("threadId = %q, want %q", got, "thread-1")
	}
	if got := mapArgString(args, "prompt"); got != "hello" {
		t.Fatalf("prompt = %q, want %q", got, "hello")
	}
	if _, ok := args["model"]; ok {
		t.Fatalf("reply args unexpectedly contain model: %v", args)
	}
	if _, ok := args["sandbox"]; ok {
		t.Fatalf("reply args unexpectedly contain sandbox: %v", args)
	}
	if _, ok := args["cwd"]; ok {
		t.Fatalf("reply args unexpectedly contain cwd: %v", args)
	}
}

func TestBuildCodexToolInvocationSessionModelOverridesConfiguredModel(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"",
		"",
		"hello",
		codexToolConfig{Model: "gpt-default"},
		"gpt-session",
		nil,
	)
	if toolName != codexToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexToolName)
	}
	if got := mapArgString(args, "model"); got != "gpt-session" {
		t.Fatalf("model = %q, want %q", got, "gpt-session")
	}
}

func TestBuildCodexToolInvocationIncludesMCPServersOnFirstTurn(t *testing.T) {
	mcpServers := map[string]acp.McpServer{
		"my-filesystem": {
			Stdio: &acp.McpServerStdio{
				Name:    "my-filesystem",
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
			},
		},
		"my-http": {
			Http: &acp.McpServerHttp{
				Name: "my-http",
				Url:  "http://localhost:8080",
				Headers: []acp.HttpHeader{
					{Name: "Authorization", Value: "Bearer token"},
				},
			},
		},
	}
	toolName, args := buildCodexToolInvocation(
		"",
		"/tmp/work",
		"list files",
		codexToolConfig{Model: "gpt-5.2-codex"},
		"",
		mcpServers,
	)
	if toolName != codexToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexToolName)
	}
	mcpServersArg, ok := args["mcpServers"].([]map[string]any)
	if !ok {
		t.Fatalf("mcpServers type = %T, want []map[string]any", args["mcpServers"])
	}
	if len(mcpServersArg) != 2 {
		t.Fatalf("len(mcpServers) = %d, want 2", len(mcpServersArg))
	}
	serverMap := make(map[string]map[string]any)
	for _, s := range mcpServersArg {
		name, _ := s["name"].(string)
		serverMap[name] = s
	}
	if _, ok := serverMap["my-filesystem"]; !ok {
		t.Fatal("mcpServers does not contain my-filesystem")
	}
	if serverMap["my-filesystem"]["transport"] != mcpTransportStdio {
		t.Fatalf("my-filesystem transport = %v, want %s", serverMap["my-filesystem"]["transport"], mcpTransportStdio)
	}
	if _, ok := serverMap["my-http"]; !ok {
		t.Fatal("mcpServers does not contain my-http")
	}
	if serverMap["my-http"]["transport"] != mcpTransportHTTP {
		t.Fatalf("my-http transport = %v, want %s", serverMap["my-http"]["transport"], mcpTransportHTTP)
	}
}

func TestBuildCodexToolInvocationOmitsMCPServersOnReplyTurn(t *testing.T) {
	mcpServers := map[string]acp.McpServer{
		"my-server": {
			Stdio: &acp.McpServerStdio{Name: "my-server", Command: "echo"},
		},
	}
	toolName, args := buildCodexToolInvocation(
		"thread-123",
		"/tmp/work",
		"continue",
		codexToolConfig{Model: "gpt-5.2-codex"},
		"",
		mcpServers,
	)
	if toolName != codexReplyToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexReplyToolName)
	}
	if _, ok := args["mcpServers"]; ok {
		t.Fatalf("reply args unexpectedly contain mcpServers: %v", args)
	}
}

func TestSessionUpdateType(t *testing.T) {
	t.Run("tool call update", func(t *testing.T) {
		update := acp.UpdateToolCall(acp.ToolCallId("call-1"), acp.WithUpdateStatus(acp.ToolCallStatusCompleted))
		if got := sessionUpdateType(update); got != "tool_call_update" {
			t.Fatalf("sessionUpdateType() = %q, want %q", got, "tool_call_update")
		}
	})

	t.Run("agent message chunk", func(t *testing.T) {
		update := acp.UpdateAgentMessageText("hello")
		if got := sessionUpdateType(update); got != "agent_message_chunk" {
			t.Fatalf("sessionUpdateType() = %q, want %q", got, "agent_message_chunk")
		}
	})
}

func TestSessionUpdatePayloadIncludesDiscriminator(t *testing.T) {
	update := acp.UpdateToolCall(acp.ToolCallId("call-1"), acp.WithUpdateStatus(acp.ToolCallStatusCompleted))
	payload := sessionUpdatePayload(update)
	if !strings.Contains(payload, `"sessionUpdate":"tool_call_update"`) {
		t.Fatalf("sessionUpdatePayload() = %q, want sessionUpdate discriminator", payload)
	}
}

func TestExtractCodexToolResultPrefersStructuredContentText(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "content channel response"},
		},
		StructuredContent: map[string]any{
			"threadId": "thread-123",
			"content":  "structured response",
		},
	}

	threadID, text := extractCodexToolResult(result)
	if threadID != "thread-123" {
		t.Fatalf("threadID = %q, want %q", threadID, "thread-123")
	}
	if text != "structured response" {
		t.Fatalf("text = %q, want %q", text, "structured response")
	}
}

func TestCodexACPProxyPromptUsesCodexThenReply(t *testing.T) {
	fakeSession := &fakeCodexMCPToolSession{
		listTools: []*mcp.Tool{
			{Name: codexToolName},
			{Name: codexReplyToolName},
		},
		callResults: []*mcp.CallToolResult{
			{
				StructuredContent: map[string]any{
					"threadId": "thread-1",
					"content":  "first response",
				},
			},
			{
				StructuredContent: map[string]any{
					"threadId": "thread-1",
					"content":  "second response",
				},
			},
		},
	}
	updater := &fakeACPSessionUpdater{}
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(fakeSession, "test-agent", codexToolConfig{}, &l)
	agent.setConnection(updater)

	newResp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("first prompt")},
	}); err != nil {
		t.Fatalf("first Prompt() error = %v", err)
	}

	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("second prompt")},
	}); err != nil {
		t.Fatalf("second Prompt() error = %v", err)
	}

	calls := fakeSession.callsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0].Name != codexToolName {
		t.Fatalf("calls[0].Name = %q, want %q", calls[0].Name, codexToolName)
	}
	if got := mapArgString(calls[0].Arguments, "prompt"); got != "first prompt" {
		t.Fatalf("first call prompt = %q, want %q", got, "first prompt")
	}
	if got := mapArgString(calls[0].Arguments, "cwd"); got != "/tmp/work" {
		t.Fatalf("first call cwd = %q, want %q", got, "/tmp/work")
	}

	if calls[1].Name != codexReplyToolName {
		t.Fatalf("calls[1].Name = %q, want %q", calls[1].Name, codexReplyToolName)
	}
	if got := mapArgString(calls[1].Arguments, "prompt"); got != "second prompt" {
		t.Fatalf("second call prompt = %q, want %q", got, "second prompt")
	}
	if got := mapArgString(calls[1].Arguments, "threadId"); got != "thread-1" {
		t.Fatalf("second call threadId = %q, want %q", got, "thread-1")
	}

	textUpdates := updater.agentMessageTexts(newResp.SessionId)
	if len(textUpdates) != 2 {
		t.Fatalf("len(agent message updates) = %d, want 2", len(textUpdates))
	}
	if textUpdates[0] != "first response" || textUpdates[1] != "second response" {
		t.Fatalf("agent message updates = %v, want [first response second response]", textUpdates)
	}
}

func TestCodexACPProxyCancelStopsPrompt(t *testing.T) {
	started := make(chan struct{})
	fakeSession := &fakeCodexMCPToolSession{
		listTools: []*mcp.Tool{
			{Name: codexToolName},
			{Name: codexReplyToolName},
		},
		callHook: func(ctx context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	updater := &fakeACPSessionUpdater{}
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(fakeSession, "test-agent", codexToolConfig{}, &l)
	agent.setConnection(updater)

	newResp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	type promptResult struct {
		resp acp.PromptResponse
		err  error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		resp, promptErr := agent.Prompt(context.Background(), acp.PromptRequest{
			SessionId: newResp.SessionId,
			Prompt:    []acp.ContentBlock{acp.TextBlock("please block")},
		})
		promptDone <- promptResult{resp: resp, err: promptErr}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt start")
	}

	if err := agent.Cancel(context.Background(), acp.CancelNotification{SessionId: newResp.SessionId}); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	select {
	case got := <-promptDone:
		if got.err != nil {
			t.Fatalf("Prompt() error = %v", got.err)
		}
		if got.resp.StopReason != acp.StopReasonCancelled {
			t.Fatalf("StopReason = %q, want %q", got.resp.StopReason, acp.StopReasonCancelled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canceled prompt")
	}
}

func TestCodexACPProxySessionFactoryCreatesDistinctBackendsPerSession(t *testing.T) {
	backends := make([]*fakeCodexMCPToolSession, 0, 2)
	factoryCalls := 0
	l := zerolog.Nop()
	agent := newCodexACPProxyAgentWithFactory(
		func(context.Context, string) (codexMCPToolSession, error) {
			factoryCalls++
			backend := &fakeCodexMCPToolSession{
				listTools: []*mcp.Tool{{Name: codexToolName}, {Name: codexReplyToolName}},
			}
			backends = append(backends, backend)
			return backend, nil
		},
		"test-agent",
		codexToolConfig{},
		&l,
	)
	agent.setConnection(&fakeACPSessionUpdater{})

	first, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work-1"})
	if err != nil {
		t.Fatalf("first NewSession() error = %v", err)
	}
	second, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work-2"})
	if err != nil {
		t.Fatalf("second NewSession() error = %v", err)
	}

	if first.SessionId == second.SessionId {
		t.Fatalf("session ids must differ, got %q", first.SessionId)
	}
	if factoryCalls != 2 {
		t.Fatalf("factory calls = %d, want 2", factoryCalls)
	}
	if len(backends) != 2 || backends[0] == backends[1] {
		t.Fatalf("expected distinct backend instances, got %#v", backends)
	}
}

func TestCodexACPProxySetModelResetsThreadAndBackend(t *testing.T) {
	backends := make([]*fakeCodexMCPToolSession, 0, 2)
	l := zerolog.Nop()
	agent := newCodexACPProxyAgentWithFactory(
		func(context.Context, string) (codexMCPToolSession, error) {
			backend := &fakeCodexMCPToolSession{
				listTools: []*mcp.Tool{{Name: codexToolName}, {Name: codexReplyToolName}},
				callResults: []*mcp.CallToolResult{
					{
						StructuredContent: map[string]any{
							"threadId": "thread-1",
							"content":  "response",
						},
					},
				},
			}
			backends = append(backends, backend)
			return backend, nil
		},
		"test-agent",
		codexToolConfig{},
		&l,
	)
	agent.setConnection(&fakeACPSessionUpdater{})

	newResp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("first prompt")},
	}); err != nil {
		t.Fatalf("first Prompt() error = %v", err)
	}
	if len(backends) != 1 {
		t.Fatalf("backend count after first prompt = %d, want 1", len(backends))
	}
	firstCalls := backends[0].callsSnapshot()
	if len(firstCalls) == 0 || firstCalls[0].Name != codexToolName {
		t.Fatalf("first backend calls = %+v, want initial %q call", firstCalls, codexToolName)
	}

	if _, err := agent.SetSessionModel(context.Background(), acp.SetSessionModelRequest{
		SessionId: newResp.SessionId,
		ModelId:   "gpt-new",
	}); err != nil {
		t.Fatalf("SetSessionModel() error = %v", err)
	}
	if got := backends[0].closeCallsCount(); got == 0 {
		t.Fatalf("first backend close calls = %d, want > 0", got)
	}

	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("second prompt")},
	}); err != nil {
		t.Fatalf("second Prompt() error = %v", err)
	}
	if len(backends) != 2 {
		t.Fatalf("backend count after model change prompt = %d, want 2", len(backends))
	}
	secondCalls := backends[1].callsSnapshot()
	if len(secondCalls) == 0 || secondCalls[0].Name != codexToolName {
		t.Fatalf("second backend calls = %+v, want thread-reset %q call", secondCalls, codexToolName)
	}
	secondArgs, ok := secondCalls[0].Arguments.(map[string]any)
	if !ok {
		t.Fatalf("second backend call args type = %T, want map[string]any", secondCalls[0].Arguments)
	}
	if _, exists := secondArgs["mode"]; exists {
		t.Fatalf("mode argument should not be propagated to codex tool call: args=%v", secondArgs)
	}
}

func TestCodexACPProxySetModelPreservesMCPServers(t *testing.T) {
	fakeSession := &fakeCodexMCPToolSession{
		listTools: []*mcp.Tool{{Name: codexToolName}, {Name: codexReplyToolName}},
	}
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(fakeSession, "test", codexToolConfig{}, &l)
	agent.setConnection(&fakeACPSessionUpdater{})

	mcpServers := []acp.McpServer{{
		Stdio: &acp.McpServerStdio{
			Name:    "preserved-server",
			Command: "echo",
		},
	}}

	// 1. Create session with mcpServers
	resp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: mcpServers,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	// 2. Change model (should trigger backend reset)
	if _, err := agent.SetSessionModel(context.Background(), acp.SetSessionModelRequest{
		SessionId: resp.SessionId,
		ModelId:   "new-model",
	}); err != nil {
		t.Fatalf("SetSessionModel() error = %v", err)
	}

	// 3. Prompt (should use new backend, but include mcpServers because thread is reset)
	_, err = agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: resp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	calls := fakeSession.callsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}

	args := calls[0].Arguments.(map[string]any)
	serverList, ok := args["mcpServers"].([]map[string]any)
	if !ok {
		t.Fatalf("mcpServers not found in args after SetSessionModel")
	}
	if len(serverList) != 1 {
		t.Fatalf("serverList len = %d, want 1", len(serverList))
	}
	if serverList[0]["name"] != "preserved-server" {
		t.Errorf("name = %v, want preserved-server", serverList[0]["name"])
	}
}

func TestCodexACPProxySetModeResetsThreadAndBackend(t *testing.T) {
	backends := make([]*fakeCodexMCPToolSession, 0, 2)
	l := zerolog.Nop()
	agent := newCodexACPProxyAgentWithFactory(
		func(context.Context, string) (codexMCPToolSession, error) {
			backend := &fakeCodexMCPToolSession{
				listTools: []*mcp.Tool{{Name: codexToolName}, {Name: codexReplyToolName}},
				callResults: []*mcp.CallToolResult{
					{
						StructuredContent: map[string]any{
							"threadId": "thread-1",
							"content":  "response",
						},
					},
				},
			}
			backends = append(backends, backend)
			return backend, nil
		},
		"test-agent",
		codexToolConfig{},
		&l,
	)
	agent.setConnection(&fakeACPSessionUpdater{})

	newResp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "/tmp/work"})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("first prompt")},
	}); err != nil {
		t.Fatalf("first Prompt() error = %v", err)
	}

	if _, err := agent.SetSessionMode(context.Background(), acp.SetSessionModeRequest{
		SessionId: newResp.SessionId,
		ModeId:    "code",
	}); err != nil {
		t.Fatalf("SetSessionMode() error = %v", err)
	}
	if got := backends[0].closeCallsCount(); got == 0 {
		t.Fatalf("first backend close calls = %d, want > 0", got)
	}

	if _, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("second prompt")},
	}); err != nil {
		t.Fatalf("second Prompt() error = %v", err)
	}
	if len(backends) != 2 {
		t.Fatalf("backend count after mode change prompt = %d, want 2", len(backends))
	}
	secondCalls := backends[1].callsSnapshot()
	if len(secondCalls) == 0 || secondCalls[0].Name != codexToolName {
		t.Fatalf("second backend calls = %+v, want thread-reset %q call", secondCalls, codexToolName)
	}
}

func TestCodexACPProxyInitializeUsesConfiguredAgentName(t *testing.T) {
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(&fakeCodexMCPToolSession{}, "team-codex", codexToolConfig{}, &l)
	resp, err := agent.Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if resp.AgentInfo == nil {
		t.Fatalf("AgentInfo is nil")
	}
	if resp.AgentInfo.Name != "team-codex" {
		t.Fatalf("AgentInfo.Name = %q, want %q", resp.AgentInfo.Name, "team-codex")
	}
}

func TestCodexACPProxyInitializeUsesDefaultAgentNameWhenEmpty(t *testing.T) {
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(&fakeCodexMCPToolSession{}, "", codexToolConfig{}, &l)
	resp, err := agent.Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if resp.AgentInfo == nil {
		t.Fatalf("AgentInfo is nil")
	}
	if resp.AgentInfo.Name != DefaultAgentName {
		t.Fatalf("AgentInfo.Name = %q, want %q", resp.AgentInfo.Name, DefaultAgentName)
	}
}

func TestRunProxyStartsCodexMCPServer(t *testing.T) {
	wrapper, argsFile := writeCodexMCPWrapper(t)
	codexDir := t.TempDir()
	codexPath := filepath.Join(codexDir, "codex")

	wrapperContent, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", wrapper, err)
	}
	if err := os.WriteFile(codexPath, wrapperContent, 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", codexPath, err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", codexDir+string(os.PathListSeparator)+originalPath)

	var stderr bytes.Buffer
	ctx := context.Background()
	runErr := RunProxy(
		ctx,
		t.TempDir(),
		Options{},
		strings.NewReader(""),
		io.Discard,
		&stderr,
	)
	if runErr != nil {
		t.Fatalf("RunProxy() error = %v; stderr=%s", runErr, stderr.String())
	}

	args := readArgsFile(t, argsFile)
	for _, want := range []string{"mcp-server"} {
		if !containsArg(args, want) {
			t.Fatalf("args %v do not contain %q", args, want)
		}
	}
	if containsArg(args, "--trace") {
		t.Fatalf("args %v unexpectedly contain passthrough argument %q", args, "--trace")
	}
}

func TestProxyMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PROXY_MCP_HELPER") != "1" {
		return
	}
	mustHelper(runProxyMCPHelper(context.Background()))
	os.Exit(0)
}

func runProxyMCPHelper(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "proxy-mcp-helper", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: codexToolName, Description: "Starts a codex thread"}, func(_ context.Context, _ *mcp.CallToolRequest, input proxyCodexToolInput) (*mcp.CallToolResult, proxyCodexToolOutput, error) {
		return nil, proxyCodexToolOutput{
			ThreadID: "thread-test",
			Content:  "codex:" + input.Prompt,
		}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: codexReplyToolName, Description: "Continues a codex thread"}, func(_ context.Context, _ *mcp.CallToolRequest, input proxyCodexReplyInput) (*mcp.CallToolResult, proxyCodexToolOutput, error) {
		return nil, proxyCodexToolOutput{
			ThreadID: input.ThreadID,
			Content:  "reply:" + input.Prompt,
		}, nil
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}

func writeCodexMCPWrapper(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	wrapperPath := filepath.Join(dir, "codex-wrapper.sh")
	script := fmt.Sprintf(`#!/bin/sh
: > %s
for arg in "$@"; do
  printf '%%s\n' "$arg" >> %s
done
exec env GO_WANT_PROXY_MCP_HELPER=1 %s -test.run=TestProxyMCPHelperProcess -- "$@"
`, shellQuote(argsFile), shellQuote(argsFile), shellQuote(os.Args[0]))
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", wrapperPath, err)
	}
	return wrapperPath, argsFile
}

func readArgsFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func mapArgString(v any, key string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

type fakeCodexToolCall struct {
	Name      string
	Arguments any
}

type fakeCodexMCPToolSession struct {
	mu sync.Mutex

	listTools   []*mcp.Tool
	callResults []*mcp.CallToolResult
	callHook    func(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
	closeErr    error
	waitErr     error
	calls       []fakeCodexToolCall
	closeCalls  int
}

func (s *fakeCodexMCPToolSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	if params != nil {
		s.calls = append(s.calls, fakeCodexToolCall{Name: params.Name, Arguments: params.Arguments})
	}
	hook := s.callHook
	var result *mcp.CallToolResult
	if len(s.callResults) > 0 {
		result = s.callResults[0]
		s.callResults = s.callResults[1:]
	}
	s.mu.Unlock()

	if hook != nil {
		return hook(ctx, params)
	}
	if result != nil {
		return result, nil
	}
	return &mcp.CallToolResult{}, nil
}

func (s *fakeCodexMCPToolSession) ListTools(_ context.Context, _ *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &mcp.ListToolsResult{Tools: append([]*mcp.Tool(nil), s.listTools...)}, nil
}

func (s *fakeCodexMCPToolSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	return s.closeErr
}

func (s *fakeCodexMCPToolSession) Wait() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitErr
}

func (s *fakeCodexMCPToolSession) callsSnapshot() []fakeCodexToolCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]fakeCodexToolCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *fakeCodexMCPToolSession) closeCallsCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCalls
}

type fakeACPSessionUpdater struct {
	mu      sync.Mutex
	updates []acp.SessionNotification
}

func (u *fakeACPSessionUpdater) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.updates = append(u.updates, params)
	return nil
}

func (u *fakeACPSessionUpdater) agentMessageTexts(sessionID acp.SessionId) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0)
	for _, update := range u.updates {
		if update.SessionId != sessionID {
			continue
		}
		if update.Update.AgentMessageChunk == nil {
			continue
		}
		if update.Update.AgentMessageChunk.Content.Text == nil {
			continue
		}
		text := update.Update.AgentMessageChunk.Content.Text.Text
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

type proxyCodexToolInput struct {
	Prompt string `json:"prompt"`
}

type proxyCodexReplyInput struct {
	ThreadID string `json:"threadId"`
	Prompt   string `json:"prompt"`
}

type proxyCodexToolOutput struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
}

func mustHelper(err error) {
	if err != nil {
		panic(err)
	}
}

func TestNewSession_WithMCP_Stdio(t *testing.T) {
	testNewSessionWithMCPTransport(t, acp.McpServer{
		Stdio: &acp.McpServerStdio{
			Name:    "my-stdio",
			Command: "echo",
		},
	}, "my-stdio", mcpTransportStdio)
}

func TestNewSession_WithMCP_HTTP(t *testing.T) {
	testNewSessionWithMCPTransport(t, acp.McpServer{
		Http: &acp.McpServerHttp{
			Name: "my-http",
			Url:  "http://localhost",
		},
	}, "my-http", mcpTransportHTTP)
}

func testNewSessionWithMCPTransport(t *testing.T, server acp.McpServer, wantName, wantTransport string) {
	t.Helper()

	fakeSession := &fakeCodexMCPToolSession{
		listTools: []*mcp.Tool{{Name: codexToolName}, {Name: codexReplyToolName}},
	}
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(fakeSession, "test", codexToolConfig{}, &l)
	agent.setConnection(&fakeACPSessionUpdater{})

	resp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{server},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	_, err = agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: resp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	calls := fakeSession.callsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}

	args := calls[0].Arguments.(map[string]any)
	serverList, ok := args["mcpServers"].([]map[string]any)
	if !ok {
		t.Fatalf("mcpServers not found in args")
	}
	if len(serverList) != 1 {
		t.Fatalf("serverList len = %d, want 1", len(serverList))
	}
	if serverList[0]["name"] != wantName {
		t.Errorf("name = %v, want %s", serverList[0]["name"], wantName)
	}
	if serverList[0]["transport"] != wantTransport {
		t.Errorf("transport = %v, want %s", serverList[0]["transport"], wantTransport)
	}
}

func TestNewSession_WithMCP_RejectInvalid(t *testing.T) {
	fakeSession := &fakeCodexMCPToolSession{}
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(fakeSession, "test", codexToolConfig{}, &l)

	// No transport
	_, err := agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{{}},
	})
	if err == nil {
		t.Fatal("expected error for empty server config")
	}

	// SSE (not supported)
	_, err = agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{{
			Sse: &acp.McpServerSse{Name: "sse", Url: "http://sse"},
		}},
	})
	if err == nil {
		t.Fatal("expected error for sse transport")
	}

	// Multiple transports on one server (ambiguous)
	_, err = agent.NewSession(context.Background(), acp.NewSessionRequest{
		McpServers: []acp.McpServer{{
			Stdio: &acp.McpServerStdio{Name: "mixed", Command: "echo"},
			Http:  &acp.McpServerHttp{Name: "mixed", Url: "http://localhost"},
		}},
	})
	if err == nil {
		t.Fatal("expected error for multiple transports in one mcp server entry")
	}
}
