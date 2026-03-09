package proxycmd

import (
	"fmt"
	"os"

	codexacp "github.com/metalagman/norma/internal/codex/acp"
	"github.com/spf13/cobra"
)

func codexACPProxyCommand() *cobra.Command {
	opts := codexacp.Options{Name: codexacp.DefaultAgentName}
	cmd := &cobra.Command{
		Use:          "codex-acp [flags] [-- <codex mcp-server args...>]",
		Short:        "Expose Codex MCP server as ACP over stdio",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			runOpts := opts
			runOpts.CodexArgs = append([]string(nil), args...)
			return codexacp.RunProxy(cmd.Context(), repoRoot, runOpts, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&opts.Model, "model", "", "Codex model configured for the backend mcp-server")
	cmd.Flags().StringVar(&opts.Name, "name", opts.Name, "ACP agent name exposed via initialize")
	cmd.Long = "Run a local Codex MCP server and expose it as an ACP agent over stdio. Use --model for normal model selection and -- to forward raw arguments to codex mcp-server."
	cmd.Example = "  norma proxy codex-acp\n  norma proxy codex-acp --model gpt-5.4\n  norma proxy codex-acp --name team-codex\n  norma proxy codex-acp --model gpt-5.4 -- --trace --raw"
	return cmd
}
