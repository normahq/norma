package plancmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/planner"
	"github.com/spf13/cobra"
)

func runACPPlanner(cmd *cobra.Command, repoRoot string, plannerCfg config.AgentConfig, req planner.Request) error {
	p := planner.NewACPPlanner(repoRoot, plannerCfg)
	runDir, err := p.RunInteractive(cmd.Context(), req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, planner.ErrHandledInTUI) {
			return nil
		}
		return err
	}

	fmt.Printf("\nPlanner session complete.\n")
	fmt.Printf("Planning run directory: %s\n", runDir)
	return nil
}
