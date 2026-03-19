package codexacpbridge

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/rs/zerolog"
)

func TestCodexACPProxyInitializeAdvertisesMCPHTTPOnly(t *testing.T) {
	l := zerolog.Nop()
	agent := newCodexACPProxyAgent(&fakeCodexMCPToolSession{}, "", codexToolConfig{}, &l)

	resp, err := agent.Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if !resp.AgentCapabilities.McpCapabilities.Http {
		t.Fatalf("McpCapabilities.Http = %t, want true", resp.AgentCapabilities.McpCapabilities.Http)
	}
	if resp.AgentCapabilities.McpCapabilities.Sse {
		t.Fatalf("McpCapabilities.Sse = %t, want false", resp.AgentCapabilities.McpCapabilities.Sse)
	}
}
