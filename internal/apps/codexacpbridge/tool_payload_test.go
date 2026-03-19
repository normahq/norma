package codexacpbridge

import (
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	cfg, ok := args["config"].(map[string]any)
	if !ok {
		t.Fatalf("config is not a map: %#v", args["config"])
	}
	if cfg["foo"] != "bar" {
		t.Fatalf("config.foo = %v, want %q", cfg["foo"], "bar")
	}
}

func TestBuildCodexToolInvocationReplyOmitsCodexConfig(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"thread-1",
		"/tmp/work",
		"hello",
		codexToolConfig{
			Model:   "gpt-5.2-codex",
			Profile: "team",
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
		t.Fatalf("reply args unexpectedly include model: %v", args)
	}
	if _, ok := args["profile"]; ok {
		t.Fatalf("reply args unexpectedly include profile: %v", args)
	}
	if _, ok := args["cwd"]; ok {
		t.Fatalf("reply args unexpectedly include cwd: %v", args)
	}
}

func TestBuildCodexToolInvocationSessionModelOverridesConfiguredModel(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"",
		"/tmp/work",
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
				Env: []acp.EnvVariable{
					{Name: "NODE_ENV", Value: "production"},
				},
			},
		},
		"my-http": {
			Http: &acp.McpServerHttp{
				Name: "my-http",
				Url:  "https://example.com/mcp",
				Headers: []acp.HttpHeader{
					{Name: "Authorization", Value: "Bearer token"},
				},
			},
		},
	}
	toolName, args := buildCodexToolInvocation(
		"",
		"/tmp/work",
		"hello",
		codexToolConfig{Model: "gpt-5.2-codex"},
		"",
		mcpServers,
	)
	if toolName != codexToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexToolName)
	}
	raw, ok := args["mcpServers"]
	if !ok {
		t.Fatalf("args do not contain mcpServers: %v", args)
	}
	serverList, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("mcpServers type = %T, want []map[string]any", raw)
	}
	if len(serverList) != 2 {
		t.Fatalf("mcpServers count = %d, want 2", len(serverList))
	}
	serverMap := map[string]map[string]any{}
	for _, server := range serverList {
		name, _ := server["name"].(string)
		serverMap[name] = server
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
