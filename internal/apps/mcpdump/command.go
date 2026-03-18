package mcpdump

import (
	"github.com/metalagman/norma/internal/apps/tools"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return tools.NewMCPDumpCommand(tools.StandaloneRuntimeConfig(), tools.DumpDeps{})
}
