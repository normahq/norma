package command

import (
	"context"
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/apps/mcpdump"
	"github.com/rs/zerolog"
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
			logLevel := resolveLogLevel(debugLogs)
			return mcpdump.Run(context.Background(), mcpdump.RunConfig{
				Command:      serverCommand,
				WorkingDir:   workingDir,
				Component:    "tool.mcp_dump",
				StartMessage: "inspecting MCP server from custom command",
				JSONOutput:   jsonOutput,
				LogLevel:     logLevel,
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

func resolveLogLevel(debugLogs bool) zerolog.Level {
	if debugLogs {
		return zerolog.DebugLevel
	}
	return zerolog.ErrorLevel
}
