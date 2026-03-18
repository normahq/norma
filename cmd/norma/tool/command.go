package toolcmd

import (
	acpdump "github.com/metalagman/norma/internal/apps/acpdump"
	acprepl "github.com/metalagman/norma/internal/apps/acprepl"
	codexacpbridge "github.com/metalagman/norma/internal/apps/codexacpbridge"
	mcpdump "github.com/metalagman/norma/internal/apps/mcpdump"
	"github.com/spf13/cobra"
)

// Command builds the `norma tool` command group.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tool",
		Short:        "Tool commands for protocol bridges",
		Long:         "Run tool helpers that expose one agent protocol through another.",
		Example:      "  norma tool codex-acp-bridge --name team-codex\n  norma tool acp-dump -- opencode acp\n  norma tool mcp-dump -- codex mcp-server\n  norma tool acp-repl --model openai/gpt-5.4 --mode coding -- opencode acp",
		SilenceUsage: true,
	}
	cmd.AddCommand(acpdump.Command())
	cmd.AddCommand(mcpdump.Command())
	cmd.AddCommand(acprepl.Command())
	cmd.AddCommand(codexacpbridge.Command())
	return cmd
}
