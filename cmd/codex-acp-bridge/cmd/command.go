package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/metalagman/norma/internal/apps/codexacpbridge"
	"github.com/metalagman/norma/internal/logging"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	runProxy    = codexacpbridge.RunProxy
	initLogging = logging.Init
)

func Command() *cobra.Command {
	opts := codexacpbridge.Options{}
	var codexConfigJSON string
	var debugLogs bool

	cmd := &cobra.Command{
		Use:          "codex-acp-bridge [flags]",
		Short:        "Expose Codex MCP server as ACP over stdio",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			runOpts := opts
			if strings.TrimSpace(codexConfigJSON) != "" {
				var config map[string]any
				if err := json.Unmarshal([]byte(codexConfigJSON), &config); err != nil {
					return fmt.Errorf("parse --codex-config JSON object: %w", err)
				}
				runOpts.CodexConfig = config
			}

			logLevel := logging.LevelInfo
			if debugLogs {
				logLevel = logging.LevelDebug
			}
			if err := initLogging(logging.WithLevel(logLevel)); err != nil {
				return fmt.Errorf("initialize logging: %w", err)
			}
			ctx := log.Logger.With().Str("component", "codex.acp.bridge").Logger().WithContext(cmd.Context())

			return runProxy(ctx, workingDir, runOpts, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "ACP agent name exposed via initialize (defaults to MCP server name)")
	cmd.Flags().StringVar(&opts.CodexModel, "codex-model", "", "Codex MCP `codex` tool model argument")
	cmd.Flags().StringVar(&opts.CodexSandbox, "codex-sandbox", "", "Codex MCP `codex` tool sandbox mode (read-only|workspace-write|danger-full-access)")
	cmd.Flags().StringVar(&opts.CodexApprovalPolicy, "codex-approval-policy", "", "Codex MCP `codex` tool approval policy (untrusted|on-failure|on-request|never)")
	cmd.Flags().StringVar(&opts.CodexProfile, "codex-profile", "", "Codex MCP `codex` tool profile argument")
	cmd.Flags().StringVar(&opts.CodexBaseInstructions, "codex-base-instructions", "", "Codex MCP `codex` tool base-instructions argument")
	cmd.Flags().StringVar(&opts.CodexDeveloperInstructions, "codex-developer-instructions", "", "Codex MCP `codex` tool developer-instructions argument")
	cmd.Flags().StringVar(&opts.CodexCompactPrompt, "codex-compact-prompt", "", "Codex MCP `codex` tool compact-prompt argument")
	cmd.Flags().StringVar(&codexConfigJSON, "codex-config", "", "Codex MCP `codex` tool config JSON object")
	cmd.Flags().BoolVar(&debugLogs, "debug", false, "enable debug logging")
	cmd.Long = "Run a local Codex MCP server and expose it as an ACP agent over stdio. Use --codex-* flags to configure the Codex MCP `codex` tool call."
	//nolint:dupword
	cmd.Example = `  codex-acp-bridge
  codex-acp-bridge --codex-model gpt-5.4 --codex-sandbox workspace-write
  codex-acp-bridge --name team-codex
  codex-acp-bridge --codex-approval-policy on-request --codex-config '{"env":"dev"}'`
	return cmd
}
