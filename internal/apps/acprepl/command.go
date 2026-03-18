package acprepl

import (
	"github.com/metalagman/norma/internal/apps/tools"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	return tools.NewACPReplCommand(tools.StandaloneRuntimeConfig(), tools.ACPREPLDeps{})
}
