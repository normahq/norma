package main

import (
	codexacpbridge "github.com/normahq/norma/cmd/codex-acp-bridge/cmd"
	"github.com/spf13/cobra"
)

func toolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tool",
		Short:        "Tool commands for protocol bridges",
		Long:         "Run tool helpers that expose one agent protocol through another.",
		Example:      "  relay tool codex-acp-bridge --name team-codex",
		SilenceUsage: true,
	}
	cmd.AddCommand(codexacpbridge.Command())
	return cmd
}
