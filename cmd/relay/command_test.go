package main

import (
	"os"
	"path/filepath"
	"testing"

	relayapp "github.com/normahq/norma/internal/apps/relay"
	"github.com/normahq/norma/internal/config"
	runtimeconfig "github.com/normahq/norma/pkg/runtime/config"
)

type relayTestConfigDocument struct {
	Norma runtimeconfig.NormaConfig `mapstructure:"norma"`
	Relay relayapp.RelayConfig      `mapstructure:"relay"`
}

func TestLoadConfigDocument_AppliesProfileRelayOverrides(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("RELAY_TELEGRAM_WEBHOOK_ENABLED", "true")

	if err := writeFile(filepath.Join(repoRoot, ".norma", "relay.yaml"), `norma:
  agents:
    relay_agent:
      type: opencode_acp
      opencode_acp:
        model: opencode/big-pickle
cli:
  pdca:
    plan: relay_agent
    do: relay_agent
    check: relay_agent
    act: relay_agent
  planner: relay_agent
profiles:
  default:
    relay:
      orchestrator_agent: relay_agent
`); err != nil {
		t.Fatalf("write relay.yaml: %v", err)
	}

	var doc relayTestConfigDocument
	selectedProfile, err := config.LoadConfigDocument(
		config.RuntimeLoadOptions{RepoRoot: repoRoot, Profile: "default"},
		config.AppLoadOptions{
			AppName:      "relay",
			DefaultsYAML: defaultRelayConfig,
		},
		&doc,
	)
	if err != nil {
		t.Fatalf("LoadConfigDocument: %v", err)
	}
	if selectedProfile != "default" {
		t.Fatalf("profile = %q, want default", selectedProfile)
	}

	relayCfg := relayapp.Config{Relay: doc.Relay}

	if relayCfg.Relay.OrchestratorAgent != "relay_agent" {
		t.Fatalf("orchestrator_agent = %q, want relay_agent", relayCfg.Relay.OrchestratorAgent)
	}
	if !relayCfg.Relay.Telegram.Webhook.Enabled {
		t.Fatal("webhook.enabled = false, want true from env override")
	}
}

func TestNewRootCommand_RegistersServeAndFlags(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "test-google-api-key")

	cmd, err := newRootCommand()
	if err != nil {
		t.Fatalf("newRootCommand: %v", err)
	}

	if _, _, err := cmd.Find([]string{"serve"}); err != nil {
		t.Fatalf("serve command missing: %v", err)
	}

	for _, name := range []string{"config-dir", "profile", "debug", "trace"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("missing persistent flag %q", name)
		}
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
