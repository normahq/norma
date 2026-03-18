package command

import (
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/apps/mcpdump"
	"github.com/metalagman/norma/internal/logging"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var jsonOutput bool
	var debugLogs bool

	cmd := &cobra.Command{
		Use:          "mcp-dump [--json] -- <mcp-server-cmd> [args...]",
		Short:        "Inspect any stdio MCP server command",
		Long:         "Start a stdio MCP server command and print initialize/capability information.",
		Example:      "  mcp-dump -- codex mcp-server\n  mcp-dump --json -- codex mcp-server --sandbox workspace-write",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCommand, err := requireMCPCommandAfterDash(cmd, args)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}

			_ = logging.Init(logging.WithDebug(debugLogs))
			ctx := log.Logger.With().Str("component", "tool.mcp_dump").Logger().WithContext(cmd.Context())

			return mcpdump.Run(ctx, mcpdump.RunConfig{
				Command:      serverCommand,
				WorkingDir:   workingDir,
				StartMessage: "inspecting MCP server from custom command",
				JSONOutput:   jsonOutput,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print output as JSON")
	cmd.Flags().BoolVar(&debugLogs, "debug", false, "enable debug logging")
	return cmd
}

func requireMCPCommandAfterDash(cmd *cobra.Command, args []string) ([]string, error) {
	dashIndex := cmd.ArgsLenAtDash()
	if dashIndex < 0 {
		return nil, fmt.Errorf("missing command delimiter --; pass MCP server command after --")
	}
	if dashIndex > 0 {
		return nil, fmt.Errorf("arguments before -- are not allowed; pass MCP server command only after --")
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("mcp server command is required after --")
	}
	return append([]string(nil), args...), nil
}
