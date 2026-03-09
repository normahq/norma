package agentfactory

import (
	"context"
	"io"
	"testing"

	"github.com/metalagman/norma/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestFactory_CreateAgent(t *testing.T) {
	agents := map[string]config.AgentConfig{
		"test-exec": {
			Type: config.AgentTypeExec,
			Cmd:  []string{"echo", "hello"},
		},
		"test-claude": {
			Type: config.AgentTypeClaude,
		},
		"test-gemini": {
			Type: config.AgentTypeGemini,
		},
		"test-codex": {
			Type: config.AgentTypeCodex,
		},
		"test-opencode": {
			Type: config.AgentTypeOpenCode,
		},
		"test-acp": {
			Type: config.AgentTypeGeminiACP,
		},
	}
	f := NewFactory(agents)

	t.Run("Create Exec Agent", func(t *testing.T) {
		req := CreationRequest{
			Name:        "TestExec",
			Description: "Test Description",
			Stdout:      io.Discard,
			Stderr:      io.Discard,
		}
		ag, err := f.CreateAgent(context.Background(), "test-exec", req)
		assert.NoError(t, err)
		assert.NotNil(t, ag)
	})

	t.Run("Create Claude Agent", func(t *testing.T) {
		req := CreationRequest{
			Name:        "TestClaude",
			Description: "Test Description",
		}
		ag, err := f.CreateAgent(context.Background(), "test-claude", req)
		assert.NoError(t, err)
		assert.NotNil(t, ag)
	})

	t.Run("Create ACP Agent", func(t *testing.T) {
		req := CreationRequest{
			Name:        "TestACP",
			Description: "Test Description",
		}
		ag, err := f.CreateAgent(context.Background(), "test-acp", req)
		assert.NoError(t, err)
		assert.NotNil(t, ag)
	})

	t.Run("Unknown Agent", func(t *testing.T) {
		req := CreationRequest{
			Name: "Unknown",
		}
		ag, err := f.CreateAgent(context.Background(), "unknown", req)
		assert.Error(t, err)
		assert.Nil(t, ag)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestResolveCmd(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.AgentConfig
		want    []string
		wantErr bool
	}{
		{
			name: "Exec with cmd",
			cfg: config.AgentConfig{
				Type: config.AgentTypeExec,
				Cmd:  []string{"ls", "-la"},
			},
			want: []string{"ls", "-la"},
		},
		{
			name: "Exec without cmd",
			cfg: config.AgentConfig{
				Type: config.AgentTypeExec,
			},
			wantErr: true,
		},
		{
			name: "Claude default",
			cfg: config.AgentConfig{
				Type: config.AgentTypeClaude,
			},
			want: []string{"claude"},
		},
		{
			name: "Claude with model",
			cfg: config.AgentConfig{
				Type:  config.AgentTypeClaude,
				Model: "claude-3",
			},
			want: []string{"claude", "--model", "claude-3"},
		},
		{
			name: "Gemini with templated cmd",
			cfg: config.AgentConfig{
				Type:  config.AgentTypeGemini,
				Cmd:   []string{"gemini", "run", "--model", "{{.Model}}"},
				Model: "gemini-1.5",
			},
			want: []string{"gemini", "run", "--model", "gemini-1.5"},
		},
		{
			name: "Gemini default",
			cfg: config.AgentConfig{
				Type: config.AgentTypeGemini,
			},
			want: []string{"gemini", "--approval-mode", "yolo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveCmd(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestResolveACPCommand(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.AgentConfig
		want    []string
		wantErr bool
	}{
		{
			name: "ACP Exec with cmd",
			cfg: config.AgentConfig{
				Type: config.AgentTypeACPExec,
				Cmd:  []string{"custom-acp", "server"},
			},
			want: []string{"custom-acp", "server"},
		},
		{
			name: "Gemini ACP with model",
			cfg: config.AgentConfig{
				Type:  config.AgentTypeGeminiACP,
				Model: "gemini-pro",
			},
			want: []string{"gemini", "--experimental-acp", "--model", "gemini-pro"},
		},
		{
			name: "OpenCode ACP",
			cfg: config.AgentConfig{
				Type: config.AgentTypeOpenCodeACP,
			},
			want: []string{"opencode", "acp"},
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
