package plancmd

import (
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/metalagman/norma/internal/planner"
	"github.com/spf13/cobra"
)

func peclCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pecl",
		Short: "Run planner using ACP protocol only",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			plannerCfg, ok := cfg.Agents["planner"]
			if !ok {
				return fmt.Errorf("planner agent not configured in selected profile %q", cfg.Profile)
			}
			if !config.IsACPType(plannerCfg.Type) {
				return fmt.Errorf("plan pecl requires ACP planner type, got %q", plannerCfg.Type)
			}

			req := planner.Request{
				Mode: planner.ModeWizard,
			}

			return runACPPlanner(cmd, repoRoot, plannerCfg, req)
		},
	}
	return cmd
}
