package mcpcmd

import (
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mcp",
		Short:        "MCP playground servers for testing MCP clients",
		SilenceUsage: true,
	}

	cmd.AddCommand(PingPongCommand())
	return cmd
}
