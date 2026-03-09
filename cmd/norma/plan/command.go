package plancmd

import (
	"github.com/spf13/cobra"
)

// Command builds the `norma plan` command group.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan subcommands: tui, web",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd, args)
		},
	}

	cmd.AddCommand(tuiCommand())
	cmd.AddCommand(webCommand())
	return cmd
}
