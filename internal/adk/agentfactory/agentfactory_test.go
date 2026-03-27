package agentfactory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/normahq/norma/internal/adk/acpagent"
	"github.com/normahq/norma/internal/adk/agentconfig"
	"github.com/normahq/norma/internal/adk/mcpregistry"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"google.golang.org/adk/agent"
)

func TestFactory_CreateAgent(t *testing.T) {
	agents := map[string]agentconfig.Config{
		"test-acp": {
			Type: agentconfig.AgentTypeGenericACP,
			GenericACP: &agentconfig.ACPConfig{
				Cmd: helperACPCommand(t),
			},
		},
	}
	f := New(agents, mcpregistry.New(nil))

	t.Run("Create ACP Agent", func(t *testing.T) {
		req := BuildRequest{
			AgentID:          "test-acp",
			Name:             "TestACP",
			Description:      "Test Description",
			WorkingDirectory: t.TempDir(),
		}
		ag, err := f.Build(context.Background(), req)
		assert.NoError(t, err)
		assert.NotNil(t, ag)
	})

	t.Run("Unknown Agent", func(t *testing.T) {
		req := BuildRequest{
			AgentID:          "unknown",
			Name:             "Unknown",
			WorkingDirectory: t.TempDir(),
		}
		ag, err := f.Build(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, ag)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Missing working directory", func(t *testing.T) {
		req := BuildRequest{
			AgentID: "test-acp",
			Name:    "TestACP",
		}
		ag, err := f.Build(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, ag)
		assert.Contains(t, err.Error(), "working_directory is required")
	})
}

func helperACPCommand(t *testing.T) []string {
	t.Helper()
	return []string{
		"env",
		"GO_WANT_AGENTFACTORY_ACP_HELPER=1",
		os.Args[0],
		"-test.run=TestAgentFactoryACPHelperProcess",
		"--",
	}
}

func TestAgentFactoryACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENTFACTORY_ACP_HELPER") != "1" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      nil,
				"error": map[string]any{
					"code":    -32700,
					"message": "parse error",
				},
			})
			continue
		}

		if req.Method == acp.AgentMethodInitialize {
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": acp.ProtocolVersionNumber,
				},
			})
			continue
		}

		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": "unsupported",
			},
		})
	}
	os.Exit(0)
}

func TestResolveACPCommand(t *testing.T) {
	tests := []struct {
		name    string
		cfg     agentconfig.Config
		want    []string
		wantErr bool
	}{
		{
			name: "ACP Exec with cmd",
			cfg: agentconfig.Config{
				Type: agentconfig.AgentTypeGenericACP,
				GenericACP: &agentconfig.ACPConfig{
					Cmd: []string{"custom-acp", "server"},
				},
			},
			want: []string{"custom-acp", "server"},
		},
		{
			name: "ACP Exec with templated extra args",
			cfg: agentconfig.Config{
				Type: agentconfig.AgentTypeGenericACP,
				GenericACP: &agentconfig.ACPConfig{
					Cmd:       []string{"custom-acp", "--model", "{{.Model}}"},
					Model:     "gpt-5.4",
					ExtraArgs: []string{"--trace", "--model={{.Model}}"},
				},
			},
			want: []string{"custom-acp", "--model", "gpt-5.4", "--trace", "--model=gpt-5.4"},
		},
		{
			name: "ACP Exec appends extra args after normalized codex bridge command",
			cfg: agentconfig.Config{
				Type: agentconfig.AgentTypeGenericACP,
				GenericACP: &agentconfig.ACPConfig{
					Cmd:       []string{"/tmp/norma", "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex"},
					ExtraArgs: []string{"--debug", "--trace"},
				},
			},
			want: []string{"/tmp/norma", "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex", "--debug", "--trace"},
		},
		{
			name: "ACP Exec missing cmd",
			cfg: agentconfig.Config{
				Type: agentconfig.AgentTypeGenericACP,
			},
			wantErr: true,
		},
		{
			name: "Unknown ACP type",
			cfg: agentconfig.Config{
				Type: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveACPCommand(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestACPConstructor_PropagatesContextLogger(t *testing.T) {
	origNewACPAgent := newACPAgent
	t.Cleanup(func() {
		newACPAgent = origNewACPAgent
	})

	var capturedLogger *zerolog.Logger
	newACPAgent = func(cfg acpagent.Config) (agent.Agent, error) {
		capturedLogger = cfg.Logger
		return nil, nil
	}

	var logBuf bytes.Buffer
	baseLogger := zerolog.New(&logBuf).Level(zerolog.TraceLevel)
	ctx := baseLogger.WithContext(context.Background())

	_, err := acpConstructor(ctx, agentconfig.Config{
		Type: agentconfig.AgentTypeGenericACP,
		GenericACP: &agentconfig.ACPConfig{
			Cmd: []string{"fake-acp", "serve"},
		},
	}, BuildRequest{
		AgentID:          "test-acp",
		Name:             "test-acp",
		Description:      "test",
		WorkingDirectory: t.TempDir(),
	}, New(map[string]agentconfig.Config{}, nil), nil)
	if err != nil {
		t.Fatalf("acpConstructor() error = %v", err)
	}
	if capturedLogger == nil {
		t.Fatal("acpConstructor() did not pass logger to acpagent config")
	}
	if capturedLogger.GetLevel() != zerolog.TraceLevel {
		t.Fatalf("captured logger level = %s, want %s", capturedLogger.GetLevel(), zerolog.TraceLevel)
	}
}

func TestFactoryBuild_UsesBuildRequestMCPServerIDsOverride(t *testing.T) {
	origNewACPAgent := newACPAgent
	t.Cleanup(func() {
		newACPAgent = origNewACPAgent
	})

	var capturedMCP map[string]agentconfig.MCPServerConfig
	newACPAgent = func(cfg acpagent.Config) (agent.Agent, error) {
		capturedMCP = cfg.MCPServers
		return nil, nil
	}

	agents := map[string]agentconfig.Config{
		"test-acp": {
			Type: agentconfig.AgentTypeGenericACP,
			GenericACP: &agentconfig.ACPConfig{
				Cmd: []string{"fake-acp", "serve"},
			},
			MCPServers: []string{"cfg"},
		},
	}
	reg := mcpregistry.New(map[string]agentconfig.MCPServerConfig{
		"cfg": {
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  "http://cfg.example/mcp",
		},
		"override": {
			Type: agentconfig.MCPServerTypeHTTP,
			URL:  "http://override.example/mcp",
		},
	})
	f := New(agents, reg)

	_, err := f.Build(context.Background(), BuildRequest{
		AgentID:          "test-acp",
		WorkingDirectory: t.TempDir(),
		MCPServerIDs:     []string{"override"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(capturedMCP) != 1 {
		t.Fatalf("len(capturedMCP) = %d, want 1", len(capturedMCP))
	}
	if _, ok := capturedMCP["override"]; !ok {
		t.Fatalf("captured MCP does not contain override server: %#v", capturedMCP)
	}
	if _, ok := capturedMCP["cfg"]; ok {
		t.Fatalf("captured MCP unexpectedly contains cfg server: %#v", capturedMCP)
	}
}
