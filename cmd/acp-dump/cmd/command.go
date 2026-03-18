package command

import (
	"context"
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/apps/acpdump"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var jsonOutput bool
	var debugLogs bool

	cmd := &cobra.Command{
		Use:          "acp-dump [--json] -- <acp-server-cmd> [args...]",
		Short:        "Inspect any stdio ACP server command",
		Long:         "Start a stdio ACP server command and print ACP initialize/session information.",
		Example:      "  acp-dump -- opencode acp\n  acp-dump --json -- gemini --experimental-acp",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCommand, err := requireACPCommandAfterDash(cmd, args)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}
			logLevel := resolveLogLevel(debugLogs)
			return acpdump.Run(context.Background(), acpdump.RunConfig{
				Command:      serverCommand,
				WorkingDir:   workingDir,
				Component:    "tool.acp_dump",
				StartMessage: "inspecting ACP agent from custom command",
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

func requireACPCommandAfterDash(cmd *cobra.Command, args []string) ([]string, error) {
	dashIndex := cmd.ArgsLenAtDash()
	if dashIndex < 0 {
		return nil, fmt.Errorf("missing command delimiter --; pass ACP server command after --")
	}
	if dashIndex > 0 {
		return nil, fmt.Errorf("arguments before -- are not allowed; pass ACP server command only after --")
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("acp server command is required after --")
	}
	return append([]string(nil), args...), nil
}

func resolveLogLevel(debugLogs bool) zerolog.Level {
	if debugLogs {
		return zerolog.DebugLevel
	}
	return zerolog.ErrorLevel
}
