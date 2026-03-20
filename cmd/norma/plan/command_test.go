package plancmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestPlanCommand_HasRequiredSubcommands(t *testing.T) {
	t.Parallel()

	cmd := Command()

	subcmds := map[string]bool{
		"tui":  false,
		"repl": false,
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

func TestPlanCommand_RootIsRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()

	if cmd.RunE != nil || cmd.Run != nil {
		t.Error("plan root command should not be runnable (should show help)")
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

func TestReplCommand_IsRunnable(t *testing.T) {
	t.Parallel()

	cmd := Command()
	replCmd := findSubcommand(cmd, "repl")

	if replCmd == nil {
		t.Fatal("repl subcommand not found")
	}

	if replCmd.RunE == nil && replCmd.Run == nil {
		t.Error("plan repl should be runnable")
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
