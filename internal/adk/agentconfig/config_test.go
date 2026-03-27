package agentconfig

import (
	"reflect"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid_generic_acp",
			cfg: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd: []string{"custom-acp", "--stdio"},
				},
			},
		},
		{
			name: "missing_type",
			cfg: Config{
				GenericACP: &ACPConfig{
					Cmd: []string{"ainvoke"},
				},
			},
			wantErr: "type is required",
		},
		{
			name: "invalid_type",
			cfg: Config{
				Type: "invalid",
				GenericACP: &ACPConfig{
					Cmd: []string{"ainvoke"},
				},
			},
			wantErr: "type must be one of:",
		},
		{
			name: "generic_acp_requires_cmd",
			cfg: Config{
				Type:       AgentTypeGenericACP,
				GenericACP: &ACPConfig{},
			},
			wantErr: "cmd is required for type generic_acp",
		},
		{
			name: "alias_forbids_cmd",
			cfg: Config{
				Type: AgentTypeGeminiACP,
				GeminiACP: &ACPConfig{
					Cmd: []string{"gemini", "--acp"},
				},
			},
			wantErr: "cmd must be omitted for type gemini_acp",
		},
		{
			name: "copilot_alias_forbids_cmd",
			cfg: Config{
				Type: AgentTypeCopilotACP,
				CopilotACP: &ACPConfig{
					Cmd: []string{"copilot", "--acp"},
				},
			},
			wantErr: "cmd must be omitted for type copilot_acp",
		},
		{
			name: "cmd_item_must_be_nonempty",
			cfg: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd: []string{"custom-acp", ""},
				},
			},
			wantErr: "cmd[1] must have at least 1 character",
		},
		{
			name: "extra_args_item_must_be_nonempty",
			cfg: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"custom-acp"},
					ExtraArgs: []string{"--trace", ""},
				},
			},
			wantErr: "extra_args[1] must have at least 1 character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() returned nil error, want substring %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNormalizeACPConfig(t *testing.T) {
	t.Parallel()

	const execPath = "/tmp/norma"

	tests := []struct {
		name    string
		cfg     Config
		exec    string
		want    Config
		wantErr string
	}{
		{
			name: "gemini_alias",
			cfg: Config{
				Type: AgentTypeGeminiACP,
				GeminiACP: &ACPConfig{
					Model:     "gemini-3-flash-preview",
					Mode:      "code",
					ExtraArgs: []string{"--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"gemini", "--acp", "--model", "gemini-3-flash-preview"},
					Model:     "gemini-3-flash-preview",
					Mode:      "code",
					ExtraArgs: []string{"--trace"},
				},
				Cmd:       []string{"gemini", "--acp", "--model", "gemini-3-flash-preview"},
				Model:     "gemini-3-flash-preview",
				Mode:      "code",
				ExtraArgs: []string{"--trace"},
			},
		},
		{
			name: "opencode_alias",
			cfg: Config{
				Type: AgentTypeOpenCodeACP,
				OpenCodeACP: &ACPConfig{
					ExtraArgs: []string{"--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"opencode", "acp"},
					ExtraArgs: []string{"--trace"},
				},
				Cmd:       []string{"opencode", "acp"},
				ExtraArgs: []string{"--trace"},
			},
		},
		{
			name: "codex_alias",
			cfg: Config{
				Type: AgentTypeCodexACP,
				CodexACP: &ACPConfig{
					Model:     "gpt-5-codex",
					Mode:      "code",
					ExtraArgs: []string{"--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{execPath, "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex"},
					Model:     "gpt-5-codex",
					Mode:      "code",
					ExtraArgs: []string{"--trace"},
				},
				Cmd:       []string{execPath, "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex"},
				Model:     "gpt-5-codex",
				Mode:      "code",
				ExtraArgs: []string{"--trace"},
			},
		},
		{
			name: "codex_alias_keeps_extra_args_for_manual_debug",
			cfg: Config{
				Type: AgentTypeCodexACP,
				CodexACP: &ACPConfig{
					Model:     "gpt-5-codex",
					ExtraArgs: []string{"--debug", "--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{execPath, "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex"},
					Model:     "gpt-5-codex",
					ExtraArgs: []string{"--debug", "--trace"},
				},
				Cmd:       []string{execPath, "tool", "codex-acp-bridge", "--codex-model", "gpt-5-codex"},
				Model:     "gpt-5-codex",
				ExtraArgs: []string{"--debug", "--trace"},
			},
		},
		{
			name: "copilot_alias",
			cfg: Config{
				Type: AgentTypeCopilotACP,
				CopilotACP: &ACPConfig{
					Model:     "gpt-5-codex",
					ExtraArgs: []string{"--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"copilot", "--acp"},
					Model:     "gpt-5-codex",
					ExtraArgs: []string{"--trace"},
				},
				Cmd:       []string{"copilot", "--acp"},
				Model:     "gpt-5-codex",
				ExtraArgs: []string{"--trace"},
			},
		},
		{
			name: "codex_alias_empty_exec_path",
			cfg: Config{
				Type:     AgentTypeCodexACP,
				CodexACP: &ACPConfig{},
			},
			wantErr: "resolve executable path: empty",
		},
		{
			name: "generic_is_unchanged",
			cfg: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"custom-acp", "--stdio"},
					ExtraArgs: []string{"--trace"},
				},
			},
			exec: execPath,
			want: Config{
				Type: AgentTypeGenericACP,
				GenericACP: &ACPConfig{
					Cmd:       []string{"custom-acp", "--stdio"},
					ExtraArgs: []string{"--trace"},
				},
				Cmd:       []string{"custom-acp", "--stdio"},
				ExtraArgs: []string{"--trace"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeACPConfig(tt.cfg, tt.exec)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeACPConfig returned nil error, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("NormalizeACPConfig error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeACPConfig returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeACPConfig = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeACPConfigs(t *testing.T) {
	t.Parallel()

	const execPath = "/tmp/norma"

	got, err := NormalizeACPConfigs(map[string]Config{
		"plan": {
			Type: AgentTypeGeminiACP,
			GeminiACP: &ACPConfig{
				Model: "gemini-3-flash-preview",
			},
		},
		"do": {
			Type:        AgentTypeOpenCodeACP,
			OpenCodeACP: &ACPConfig{},
		},
		"check": {
			Type: AgentTypeCodexACP,
			CodexACP: &ACPConfig{
				Model: "gpt-5-codex",
			},
		},
		"act": {
			Type:       AgentTypeCopilotACP,
			CopilotACP: &ACPConfig{},
		},
		"planner": {
			Type: AgentTypeGenericACP,
			GenericACP: &ACPConfig{
				Cmd: []string{"custom-acp"},
			},
		},
	}, execPath)
	if err != nil {
		t.Fatalf("NormalizeACPConfigs returned error: %v", err)
	}

	planCfg := got["plan"]
	if planCfg.Type != AgentTypeGenericACP {
		t.Fatalf("plan type = %q, want %q", planCfg.Type, AgentTypeGenericACP)
	}
	if len(planCfg.Cmd) == 0 || planCfg.Cmd[0] != "gemini" {
		t.Fatalf("plan cmd = %v, want gemini ACP command", planCfg.Cmd)
	}

	doCfg := got["do"]
	if doCfg.Type != AgentTypeGenericACP {
		t.Fatalf("do type = %q, want %q", doCfg.Type, AgentTypeGenericACP)
	}
	if len(doCfg.Cmd) < 2 || doCfg.Cmd[0] != "opencode" || doCfg.Cmd[1] != "acp" {
		t.Fatalf("do cmd = %v, want opencode acp", doCfg.Cmd)
	}

	checkCfg := got["check"]
	if checkCfg.Type != AgentTypeGenericACP {
		t.Fatalf("check type = %q, want %q", checkCfg.Type, AgentTypeGenericACP)
	}
	if len(checkCfg.Cmd) < 3 || checkCfg.Cmd[0] != execPath || checkCfg.Cmd[1] != "tool" || checkCfg.Cmd[2] != "codex-acp-bridge" {
		t.Fatalf("check cmd = %v, want codex tool command", checkCfg.Cmd)
	}

	actCfg := got["act"]
	if actCfg.Type != AgentTypeGenericACP {
		t.Fatalf("act type = %q, want %q", actCfg.Type, AgentTypeGenericACP)
	}
	if len(actCfg.Cmd) < 2 || actCfg.Cmd[0] != "copilot" || actCfg.Cmd[1] != "--acp" {
		t.Fatalf("act cmd = %v, want copilot --acp", actCfg.Cmd)
	}
}
