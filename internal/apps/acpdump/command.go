package acpdump

import (
	"github.com/metalagman/norma/internal/apps/tools"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return tools.NewACPDumpCommand(tools.StandaloneRuntimeConfig(), tools.DumpDeps{})
}
