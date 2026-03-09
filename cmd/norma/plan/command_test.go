package plancmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestPlanCommand_HasRequiredSubcommands(t *testing.T) {
	t.Parallel()

	cmd := Command()

	subcmds := map[string]bool{
		"tui":  false,
		"pecl": false,
		"web":  false,
	}

	for _, c := range cmd.Commands() {
		subcmds[c.Name()] = true
	}

	for name, found := range subcmds {
		if !found {
			t.Errorf("plan command missing %q subcommand", name)
		}
	}
}

func TestPlanCommand_RootIsNotRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()

	if cmd.RunE != nil {
		t.Error("plan root command should not have RunE (should be pure command group)")
	}

	if cmd.Run != nil {
		t.Error("plan root command should not have Run (should be pure command group)")
	}
}

func TestTuiCommand_IsRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()
	tuiCmd := findSubcommand(cmd, "tui")

	if tuiCmd == nil {
		t.Fatal("tui subcommand not found")
	}

	if tuiCmd.RunE == nil && tuiCmd.Run == nil {
		t.Error("plan tui should be runnable")
	}
}

func TestPeclCommand_IsRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()
	peclCmd := findSubcommand(cmd, "pecl")

	if peclCmd == nil {
		t.Fatal("pecl subcommand not found")
	}

	if peclCmd.RunE == nil && peclCmd.Run == nil {
		t.Error("plan pecl should be runnable")
	}
}

func TestWebCommand_IsRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()
	webCmd := findSubcommand(cmd, "web")

	if webCmd == nil {
		t.Fatal("web subcommand not found")
	}

	if webCmd.RunE == nil && webCmd.Run == nil {
		t.Error("plan web should be runnable")
	}
}

func TestPeclCommand_RejectsNonACPPlanner(t *testing.T) {
	repoRoot := t.TempDir()

	cfgDir := filepath.Join(repoRoot, ".norma")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}

	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfgContent := `profile: default
agents:
  llm_planner:
    type: llm
    model: gpt-4
profiles:
  default:
    planner: llm_planner
budgets:
  max_iterations: 1
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("config", ".norma/config.yaml")

	cmd := peclCommand()
	cmd.SetContext(t.Context())

	err := cmd.RunE(cmd, []string{})

	if err == nil {
		t.Error("plan pecl should reject non-ACP planner, got nil error")
	}
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
