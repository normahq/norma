package config

import (
	"os"
	"path/filepath"
	"testing"

	runtimeconfig "github.com/normahq/norma/pkg/runtime/config"
)

type relayConfigDocumentForTest struct {
	Norma runtimeconfig.NormaConfig `mapstructure:"norma"`
	Relay struct {
		OrchestratorAgent string `mapstructure:"orchestrator_agent"`
		Telegram          struct {
			Webhook struct {
				URL     string `mapstructure:"url"`
				Enabled bool   `mapstructure:"enabled"`
			} `mapstructure:"webhook"`
		} `mapstructure:"telegram"`
		Logger struct {
			Level string `mapstructure:"level"`
		} `mapstructure:"logger"`
	} `mapstructure:"relay"`
}

func TestLoadRuntime_PrefersConfigDirOverRepoAndGlobal(t *testing.T) {
	repoRoot := t.TempDir()
	xdgRoot := t.TempDir()
	extraRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)

	if err := writeRuntimeFile(filepath.Join(xdgRoot, "norma", "cli.yaml"), runtimeYAMLWithCmd("global")); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "cli.yaml"), runtimeYAMLWithCmd("repo")); err != nil {
		t.Fatalf("write repo config: %v", err)
	}
	if err := writeRuntimeFile(filepath.Join(extraRoot, "cli.yaml"), runtimeYAMLWithCmd("extra")); err != nil {
		t.Fatalf("write extra config: %v", err)
	}

	cfg, err := LoadRuntime(RuntimeLoadOptions{RepoRoot: repoRoot, ConfigDir: extraRoot})
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	agentCfg := cfg.Norma.Agents["agent"]
	if agentCfg.GenericACP == nil || len(agentCfg.GenericACP.Cmd) == 0 {
		t.Fatalf("agent generic_acp block missing cmd: %#v", agentCfg)
	}
	if got := agentCfg.GenericACP.Cmd[0]; got != "extra" {
		t.Fatalf("agent generic_acp.cmd[0] = %q, want extra", got)
	}
}

func TestLoadRuntime_UsesSingleEffectiveFileWithoutCrossRootMerge(t *testing.T) {
	repoRoot := t.TempDir()
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)

	if err := writeRuntimeFile(filepath.Join(xdgRoot, "norma", "cli.yaml"), runtimeYAMLWithCmd("global")); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "cli.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["repo-only"]
`); err != nil {
		t.Fatalf("write repo config: %v", err)
	}

	_, err := LoadRuntime(RuntimeLoadOptions{RepoRoot: repoRoot})
	if err == nil {
		t.Fatal("LoadRuntime returned nil error, want validation error from incomplete repo config")
	}
}

func TestLoadConfigDocument_AppliesProfileOverridesAndEnv(t *testing.T) {
	repoRoot := t.TempDir()
	t.Setenv("RELAY_TELEGRAM_WEBHOOK_URL", "https://example.com/webhook")
	t.Setenv("RELAY_TELEGRAM_WEBHOOK_ENABLED", "true")

	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "relay.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["agent"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
relay:
  orchestrator_agent: from_relay_file
profiles:
  default:
    relay:
      logger:
        level: debug
`); err != nil {
		t.Fatalf("write relay config: %v", err)
	}

	var doc relayConfigDocumentForTest
	_, err := LoadConfigDocument(
		RuntimeLoadOptions{RepoRoot: repoRoot, Profile: "default"},
		AppLoadOptions{
			AppName: "relay",
			DefaultsYAML: []byte(`relay:
  telegram:
    webhook:
      url: ""
      enabled: false
`),
		},
		&doc,
	)
	if err != nil {
		t.Fatalf("LoadConfigDocument: %v", err)
	}

	if got := doc.Relay.OrchestratorAgent; got != "from_relay_file" {
		t.Fatalf("orchestrator_agent = %q, want from_relay_file", got)
	}
	if got := doc.Relay.Telegram.Webhook.URL; got != "https://example.com/webhook" {
		t.Fatalf("telegram.webhook.url = %q, want https://example.com/webhook", got)
	}
	if !doc.Relay.Telegram.Webhook.Enabled {
		t.Fatalf("telegram.webhook.enabled = false, want true from env")
	}
	if got := doc.Relay.Logger.Level; got != "debug" {
		t.Fatalf("logger.level = %q, want debug", got)
	}
}

func TestLoadConfigDocument_PrefersAppSpecificFileOverCoreConfigWithoutMerging(t *testing.T) {
	repoRoot := t.TempDir()

	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "config.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["core-agent"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
relay:
  orchestrator_agent: from_core_file
`); err != nil {
		t.Fatalf("write core config: %v", err)
	}
	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "relay.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["relay-agent"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
relay:
  orchestrator_agent: from_relay_file
`); err != nil {
		t.Fatalf("write relay config: %v", err)
	}

	var doc relayConfigDocumentForTest
	_, err := LoadConfigDocument(
		RuntimeLoadOptions{RepoRoot: repoRoot},
		AppLoadOptions{AppName: "relay"},
		&doc,
	)
	if err != nil {
		t.Fatalf("LoadConfigDocument: %v", err)
	}

	if got := doc.Relay.OrchestratorAgent; got != "from_relay_file" {
		t.Fatalf("orchestrator_agent = %q, want from_relay_file", got)
	}
	if got := doc.Relay.Telegram.Webhook.URL; got != "" {
		t.Fatalf("relay.telegram.webhook unexpectedly loaded from config.yaml; app-specific file should be used without merge")
	}
}

func TestLoadConfigDocument_FallsBackToCoreConfigWhenAppSpecificMissing(t *testing.T) {
	repoRoot := t.TempDir()

	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "config.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["core-agent"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
relay:
  orchestrator_agent: from_core_file
`); err != nil {
		t.Fatalf("write core config: %v", err)
	}

	var doc relayConfigDocumentForTest
	_, err := LoadConfigDocument(
		RuntimeLoadOptions{RepoRoot: repoRoot},
		AppLoadOptions{AppName: "relay"},
		&doc,
	)
	if err != nil {
		t.Fatalf("LoadConfigDocument: %v", err)
	}

	if got := doc.Relay.OrchestratorAgent; got != "from_core_file" {
		t.Fatalf("orchestrator_agent = %q, want from_core_file", got)
	}
}

func TestLoadRuntime_AcceptsNormaMCPServersKey(t *testing.T) {
	repoRoot := t.TempDir()
	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "config.yaml"), `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["agent"]
  mcp_servers:
    tasks:
      type: stdio
      cmd: ["norma", "mcp", "tasks"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
`); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	cfg, err := LoadRuntime(RuntimeLoadOptions{RepoRoot: repoRoot})
	if err != nil {
		t.Fatalf("LoadRuntime returned error: %v", err)
	}
	if len(cfg.Norma.MCPServers) != 1 {
		t.Fatalf("len(cfg.Norma.MCPServers) = %d, want 1", len(cfg.Norma.MCPServers))
	}
}

func TestLoadRuntime_AllowsExtraOutOfScopeFields(t *testing.T) {
	repoRoot := t.TempDir()
	content := "norma:\n" +
		"  agents:\n" +
		"    agent:\n" +
		"      type: generic_acp\n" +
		"      generic_acp:\n" +
		"        cmd: [\"agent\"]\n" +
		"        api_key: \"secret\"\n" +
		"cli:\n" +
		"  pdca:\n" +
		"    plan: agent\n" +
		"    do: agent\n" +
		"    check: agent\n" +
		"    act: agent\n"
	if err := writeRuntimeFile(filepath.Join(repoRoot, ".norma", "config.yaml"), content); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	if _, err := LoadRuntime(RuntimeLoadOptions{RepoRoot: repoRoot}); err != nil {
		t.Fatalf("LoadRuntime returned error for extra field: %v", err)
	}
}

func runtimeYAMLWithCmd(cmd string) string {
	return `norma:
  agents:
    agent:
      type: generic_acp
      generic_acp:
        cmd: ["` + cmd + `"]
cli:
  pdca:
    plan: agent
    do: agent
    check: agent
    act: agent
`
}

func writeRuntimeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
