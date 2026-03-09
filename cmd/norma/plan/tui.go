package plancmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/metalagman/norma/internal/adk/modelfactory"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/metalagman/norma/internal/planner"
	"github.com/spf13/cobra"
)

func tuiCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui [epic goal]",
		Short: "Interactively decompose an epic into features and tasks and persist them to Beads",
		Args:  cobra.MaximumNArgs(1),
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

			epicDescription := ""
			if len(args) > 0 {
				epicDescription = args[0]
			}
			req := planner.Request{
				EpicDescription: epicDescription,
				Mode:            planner.ModeWizard,
			}

			plannerCfg, ok := cfg.Agents["planner"]
			if !ok {
				return fmt.Errorf("planner agent not configured in selected profile %q", cfg.Profile)
			}
			if !plannerSupportedType(plannerCfg.Type) {
				return fmt.Errorf("planner mode supports only llm and acp agent types, got %q", plannerCfg.Type)
			}
			if isACPType(plannerCfg.Type) {
				return runACPPlanner(cmd, repoRoot, plannerCfg, req)
			}
			return runLLMPlanner(cmd, repoRoot, cfg, req)
		},
	}
	return cmd
}

func plannerSupportedType(t string) bool {
	return t == "llm" || t == "acp" || t == "gemini" || t == "openai"
}

func isACPType(t string) bool {
	return t == "acp"
}

func runLLMPlanner(cmd *cobra.Command, repoRoot string, cfg config.Config, req planner.Request) error {
	factory := modelfactory.NewFactory(modelfactory.FactoryConfig{})
	m, err := factory.CreateModel("planner")
	if err != nil {
		return fmt.Errorf("create planner model %q: %w", "planner", err)
	}
	p, err := planner.NewLLMPlanner(repoRoot, m)
	if err != nil {
		return fmt.Errorf("create planner runtime: %w", err)
	}

	ctx := cmd.Context()
	runDir, err := p.RunInteractive(ctx, req)
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
