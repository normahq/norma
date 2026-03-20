package plancmd

import (
	"bytes"
	"strings"
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

func TestPlanCommand_NoSubcommandShowsHelp(t *testing.T) {
	t.Parallel()

	cmd := Command()

	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("plan command execution failed: %v", err)
	}

	output := outBuf.String()
	if output == "" {
		t.Fatal("plan command produced no output")
	}

	if !strings.Contains(output, "Usage:") && !strings.Contains(output, "Available Commands:") {
		t.Errorf("plan command output does not contain usage/help text:\n%s", output)
	}
}

func TestPlanCommand_TuiSubcommandResolved(t *testing.T) {
	t.Parallel()

	cmd := Command()
	tuiCmd := findSubcommand(cmd, "tui")

	if tuiCmd == nil {
		t.Fatal("plan tui subcommand not found")
	}

	if tuiCmd.RunE == nil && tuiCmd.Run == nil {
		t.Error("plan tui should be runnable")
	}

	if tuiCmd.Name() != "tui" {
		t.Errorf("plan tui name = %q, want %q", tuiCmd.Name(), "tui")
	}
}

func TestPlanCommand_ReplSubcommandResolved(t *testing.T) {
	t.Parallel()

	cmd := Command()
	replCmd := findSubcommand(cmd, "repl")

	if replCmd == nil {
		t.Fatal("plan repl subcommand not found")
	}

	if replCmd.RunE == nil && replCmd.Run == nil {
		t.Error("plan repl should be runnable")
	}

	if replCmd.Name() != "repl" {
		t.Errorf("plan repl name = %q, want %q", replCmd.Name(), "repl")
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
