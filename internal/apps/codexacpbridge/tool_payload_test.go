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
	mcpCfg := mustCodexMCPServersConfig(t, args)
	if len(mcpCfg) != 2 {
		t.Fatalf("mcp_servers count = %d, want 2", len(mcpCfg))
	}
	stdioCfg, ok := mcpCfg["my-filesystem"].(map[string]any)
	if !ok {
		t.Fatalf("my-filesystem config missing or wrong type: %#v", mcpCfg["my-filesystem"])
	}
	if got := mapArgString(stdioCfg, "command"); got != "npx" {
		t.Fatalf("my-filesystem.command = %q, want %q", got, "npx")
	}
	envCfg, ok := stdioCfg["env"].(map[string]string)
	if !ok {
		t.Fatalf("my-filesystem.env type = %T, want map[string]string", stdioCfg["env"])
	}
	if got := envCfg["NODE_ENV"]; got != "production" {
		t.Fatalf("my-filesystem.env.NODE_ENV = %q, want %q", got, "production")
	}
	httpCfg, ok := mcpCfg["my-http"].(map[string]any)
	if !ok {
		t.Fatalf("my-http config missing or wrong type: %#v", mcpCfg["my-http"])
	}
	if got := mapArgString(httpCfg, "url"); got != "https://example.com/mcp" {
		t.Fatalf("my-http.url = %q, want %q", got, "https://example.com/mcp")
	}
	headersCfg, ok := httpCfg["http_headers"].(map[string]string)
	if !ok {
		t.Fatalf("my-http.http_headers type = %T, want map[string]string", httpCfg["http_headers"])
	}
	if got := headersCfg["Authorization"]; got != "Bearer token" {
		t.Fatalf("my-http.http_headers.Authorization = %q, want %q", got, "Bearer token")
	}
	if _, ok := args["mcpServers"]; ok {
		t.Fatalf("args unexpectedly contain legacy mcpServers: %v", args)
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
	if _, ok := args["config"]; ok {
		t.Fatalf("reply args unexpectedly contain config: %v", args)
	}
}

func TestBuildCodexToolInvocationSessionMCPServersOverrideConfigMCPServers(t *testing.T) {
	toolName, args := buildCodexToolInvocation(
		"",
		"/tmp/work",
		"hello",
		codexToolConfig{
			Config: map[string]any{
				"foo": "bar",
				"mcp_servers": map[string]any{
					"existing": map[string]any{
						"command": "old",
					},
				},
			},
		},
		"",
		map[string]acp.McpServer{
			"new-server": {
				Stdio: &acp.McpServerStdio{Name: "new-server", Command: "echo"},
			},
		},
	)
	if toolName != codexToolName {
		t.Fatalf("toolName = %q, want %q", toolName, codexToolName)
	}
	cfg, ok := args["config"].(map[string]any)
	if !ok {
		t.Fatalf("config is not a map: %#v", args["config"])
	}
	if got := mapArgString(cfg, "foo"); got != "bar" {
		t.Fatalf("config.foo = %q, want %q", got, "bar")
	}
	mcpCfg := mustCodexMCPServersConfig(t, args)
	if len(mcpCfg) != 1 {
		t.Fatalf("mcp_servers count = %d, want 1", len(mcpCfg))
	}
	if _, ok := mcpCfg["existing"]; ok {
		t.Fatalf("mcp_servers unexpectedly kept existing entry: %#v", mcpCfg)
	}
	newCfg, ok := mcpCfg["new-server"].(map[string]any)
	if !ok {
		t.Fatalf("new-server config missing or wrong type: %#v", mcpCfg["new-server"])
	}
	if got := mapArgString(newCfg, "command"); got != "echo" {
		t.Fatalf("new-server.command = %q, want %q", got, "echo")
	}
}

func mustCodexMCPServersConfig(t *testing.T, args map[string]any) map[string]any {
	t.Helper()
	cfg, ok := args["config"].(map[string]any)
	if !ok {
		t.Fatalf("args.config type = %T, want map[string]any", args["config"])
	}
	mcpCfg, ok := cfg["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("config.mcp_servers type = %T, want map[string]any", cfg["mcp_servers"])
	}
	return mcpCfg
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
