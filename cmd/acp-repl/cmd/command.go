package command

import (
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/apps/acprepl"
	"github.com/metalagman/norma/internal/logging"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var sessionModel string
	var sessionMode string
	var debugLogs bool

	cmd := &cobra.Command{
		Use:          "acp-repl [--model <model>] [--mode <mode>] -- <acp-server-cmd> [args...]",
		Short:        "Run an interactive REPL against any stdio ACP server command",
		Long:         "Start a stdio ACP server command and run an interactive terminal REPL over ACP.",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			acpCommand, err := requireACPCommandAfterDash(cmd, args)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			ctx := log.Logger.WithContext(cmd.Context())
			return acprepl.RunREPL(
				ctx,
				workingDir,
				acpCommand,
				sessionModel,
				sessionMode,
				cmd.InOrStdin(),
				cmd.OutOrStdout(),
				cmd.ErrOrStderr(),
			)
		},
	}
	cmd.Flags().StringVar(&sessionModel, "model", "", "session model requested via ACP session/set_model")
	cmd.Flags().StringVar(&sessionMode, "mode", "", "session mode requested via ACP session/set_mode")
	cmd.Flags().BoolVar(&debugLogs, "debug", false, "enable debug logging")
	cmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		_ = logging.Init(logging.WithDebug(debugLogs))
	}
	cmd.Example = "  acp-repl -- opencode acp\n  acp-repl --model openai/gpt-5.4 --mode coding -- opencode acp\n  acp-repl -- gemini --experimental-acp"
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
