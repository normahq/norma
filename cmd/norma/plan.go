package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/metalagman/norma/internal/adk/modelfactory"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/metalagman/norma/internal/planner"
	"github.com/metalagman/norma/internal/task"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

func planCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "plan <epic-description>",
		Short:        "Decompose an epic into features and tasks and persist them to Beads",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			if !git.Available(cmd.Context(), repoRoot) {
				return fmt.Errorf("current directory is not a git repository")
			}

			rawCfg, err := loadRawConfig(repoRoot)
			if err != nil {
				return err
			}

			req := planner.Request{
				EpicDescription: strings.TrimSpace(strings.Join(args, " ")),
			}

			app := fx.New(
				fx.Supply(cmd.Context()),
				fx.Supply(repoRoot),
				fx.Supply(rawCfg),
				fx.Supply(req),
				fx.Provide(func(cfg config.Config) modelfactory.FactoryConfig {
					return planner.ToFactoryConfig(cfg)
				}),
				modelfactory.Module,
				task.Module,
				planner.Module,
				fx.Invoke(runPlan),
				fx.NopLogger,
			)

			return app.Start(cmd.Context())
		},
	}

	return cmd
}

func runPlan(
	ctx context.Context,
	p *planner.LLMPlanner,
	bt *planner.BeadsTool,
	req planner.Request,
	shutdown fx.Shutdowner,
) error {
	plan, runDir, err := p.Generate(ctx, req)
	if err != nil {
		_ = shutdown.Shutdown()
		return err
	}

	applied, err := bt.Apply(ctx, plan)
	if err != nil {
		_ = shutdown.Shutdown()
		return err
	}

	fmt.Printf("\nPlan generated and persisted to Beads.\n")
	fmt.Printf("Epic: %s\n", applied.EpicID)
	for i, feature := range applied.Features {
		fmt.Printf("Feature %d: %s\n", i+1, feature.FeatureID)
		for _, taskID := range feature.TaskIDs {
			fmt.Printf("  - Task: %s\n", taskID)
		}
	}
	fmt.Printf("Planning artifacts: %s\n", runDir)

	return shutdown.Shutdown()
}
