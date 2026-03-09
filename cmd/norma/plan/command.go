package plancmd

import (
	"github.com/spf13/cobra"
)

// Command builds the `norma plan` command group.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan subcommands: tui, pecl, web",
	}

	cmd.AddCommand(tuiCommand())
	cmd.AddCommand(peclCommand())
	cmd.AddCommand(webCommand())
	return cmd
}
