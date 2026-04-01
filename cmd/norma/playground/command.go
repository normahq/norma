package playgroundcmd

import (
	acpcmd "github.com/normahq/norma/cmd/norma/playground/acp"
	mcpcmd "github.com/normahq/norma/cmd/norma/playground/mcp"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "playground",
		Short:        "Experimental playground commands for agent integrations",
		SilenceUsage: true,
	}
	cmd.AddCommand(acpcmd.Command())
	cmd.AddCommand(mcpcmd.Command())
	return cmd
}
