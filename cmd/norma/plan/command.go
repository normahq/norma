package plancmd

import (
	"github.com/spf13/cobra"
)

// Command builds the `norma plan` command group.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plan",
		Short:   "Plan subcommands: tui, repl, web",
		Long:    "Plan and decompose issues using AI agents. Use subcommands to launch the interactive planner:\n\n  $ codex plan tui   Launch the interactive TUI for planning\n  $ codex plan repl   Run the planner in a line-based REPL",
		Example: "  codex plan tui\n  codex plan repl\n  codex plan web",
	}

	cmd.AddCommand(tuiCommand())
	cmd.AddCommand(replCommand())
	cmd.AddCommand(webCommand())
	return cmd
}
