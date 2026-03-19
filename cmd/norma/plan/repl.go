package plancmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/metalagman/norma/internal/apps/acprepl"
	"github.com/metalagman/norma/internal/git"
	"github.com/spf13/cobra"
	adkagent "google.golang.org/adk/agent"
)

const (
	plannerREPLAppName = "norma-plan-repl"
	plannerREPLUserID  = "norma-plan-repl-user"
)

func replCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Run the planner agent in a line-based terminal REPL",
		Args:  cobra.NoArgs,
		RunE:  runREPL,
	}
}

func runREPL(cmd *cobra.Command, _ []string) error {
	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if !git.Available(cmd.Context(), repoRoot) {
		return fmt.Errorf("current directory is not a git repository")
	}

	cfg, err := loadConfig(repoRoot)
	if err != nil {
		return err
	}
	plannerID, ok := cfg.RoleIDs["planner"]
	if !ok {
		return fmt.Errorf("planner agent not configured in selected profile %q", cfg.Profile)
	}

	return acprepl.RunAgentREPL(cmd.Context(), acprepl.AgentREPLConfig{
		AppName: plannerREPLAppName,
		UserID:  plannerREPLUserID,
		Stdin:   cmd.InOrStdin(),
		Stdout:  cmd.OutOrStdout(),
		Stderr:  cmd.ErrOrStderr(),
		AgentFactory: func(
			ctx context.Context,
			permissionHandler acprepl.PermissionHandler,
			stderr io.Writer,
		) (adkagent.Agent, func() error, error) {
			return createPlannerAgentWithOptions(ctx, repoRoot, cfg.Agents, cfg.MCPServers, plannerID, plannerAgentCreateOptions{
				Stderr:            stderr,
				PermissionHandler: permissionHandler,
			})
		},
	})
}
