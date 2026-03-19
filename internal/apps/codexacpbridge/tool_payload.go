package codexacpbridge

import (
	"encoding/json"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func logJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"marshal_error":%q}`, err.Error())
	}
	const maxPayloadLen = 4096
	if len(raw) <= maxPayloadLen {
		return string(raw)
	}
	return string(raw[:maxPayloadLen]) + fmt.Sprintf(`...{"truncated_bytes":%d}`, len(raw)-maxPayloadLen)
}

func buildCodexToolInvocation(threadID, cwd, prompt string, defaultConfig codexToolConfig, sessionModel string, sessionMCPServers map[string]acp.McpServer) (string, map[string]any) {
	args := map[string]any{
		"prompt": prompt,
	}
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD != "" && threadID == "" {
		args["cwd"] = trimmedCWD
	}

	if strings.TrimSpace(threadID) == "" {
		defaultConfig.withModel(sessionModel).applyTo(args)
		if len(sessionMCPServers) > 0 {
			mcpServersList := make([]map[string]any, 0, len(sessionMCPServers))
			for _, server := range sessionMCPServers {
				serverMap := map[string]any{}
				if server.Stdio != nil {
					serverMap["name"] = server.Stdio.Name
					serverMap["command"] = server.Stdio.Command
					serverMap["args"] = server.Stdio.Args
					serverMap["env"] = flattenEnvVars(server.Stdio.Env)
					serverMap["transport"] = "stdio"
				} else if server.Http != nil {
					serverMap["name"] = server.Http.Name
					serverMap["url"] = server.Http.Url
					serverMap["headers"] = flattenHTTPHeaders(server.Http.Headers)
					serverMap["transport"] = "http"
				}
				if len(serverMap) > 0 {
					mcpServersList = append(mcpServersList, serverMap)
				}
			}
			if len(mcpServersList) > 0 {
				args["mcpServers"] = mcpServersList
			}
		}
		return "codex", args
	}

	args["threadId"] = strings.TrimSpace(threadID)
	return "codex-reply", args
}

func joinPromptText(blocks []acp.ContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, block := range blocks {
		if block.Text == nil {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(block.Text.Text)
	}
	return builder.String()
}

func extractCodexToolResult(result *mcp.CallToolResult) (threadID string, text string) {
	if result == nil {
		return "", ""
	}

	structuredContent := any(nil)
	structuredText := ""

	switch payload := result.StructuredContent.(type) {
	case map[string]any:
		structuredContent = payload
		if thread, ok := payload["threadId"].(string); ok {
			threadID = strings.TrimSpace(thread)
		}
		if contentText, ok := payload["content"].(string); ok {
			structuredText = strings.TrimSpace(contentText)
		}
	default:
		structuredContent = payload
	}

	if structuredText != "" {
		return threadID, structuredText
	}

	textParts := make([]string, 0, len(result.Content))
	for _, item := range result.Content {
		textContent, ok := item.(*mcp.TextContent)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(textContent.Text)
		if trimmed == "" {
			continue
		}
		textParts = append(textParts, trimmed)
	}
	if len(textParts) > 0 {
		return threadID, strings.Join(textParts, "\n")
	}

	if structuredContent != nil {
		raw, err := json.Marshal(structuredContent)
		if err == nil && len(raw) > 0 {
			return threadID, string(raw)
		}
	}
	return threadID, ""
}
