package plancmd

import (
	"context"
	"fmt"
	"os"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/metalagman/norma/internal/adk/acpagent"
	normaagent "github.com/metalagman/norma/internal/agent"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/git"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func peclCommand() *cobra.Command {
	var prompt string

	cmd := &cobra.Command{
		Use:   "pecl",
		Short: "Run ACP CLI in REPL mode (like playground)",
		Long: `Run the configured ACP planner CLI in interactive REPL mode.
This is a simple ACP execution without the planner TUI.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			return runACPREPL(cmd.Context(), repoRoot, plannerCfg, prompt)
		},
	}

	cmd.Flags().StringVar(&prompt, "prompt", "", "single prompt to run; if empty starts a REPL")
	return cmd
}

func runACPREPL(ctx context.Context, repoRoot string, plannerCfg config.AgentConfig, prompt string) error {
	acpCmd, err := normaagent.ResolveACPCommand(plannerCfg)
	if err != nil {
		return fmt.Errorf("resolve ACP command: %w", err)
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("component", "plan.pecl").Logger()

	acpRuntime, err := acpagent.New(acpagent.Config{
		Context:           ctx,
		Name:              "NormaPlanPecl",
		Description:       "Norma plan pecl via ACP",
		Model:             plannerCfg.Model,
		Command:           acpCmd,
		WorkingDir:        repoRoot,
		Stderr:            os.Stderr,
		PermissionHandler: autoAllowPermission,
		Logger:            &logger,
	})
	if err != nil {
		return fmt.Errorf("create ACP runtime: %w", err)
	}
	defer func() { _ = acpRuntime.Close() }()

	sessionService := session.InMemoryService()
	adkRunner, err := runner.New(runner.Config{
		AppName:        "norma-plan-pecl",
		Agent:          acpRuntime,
		SessionService: sessionService,
	})
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}

	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "norma-plan-pecl",
		UserID:  "norma-pecl-user",
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	if prompt != "" {
		logger.Info().Str("prompt", prompt).Msg("running one-shot prompt")
		return runACPTurn(ctx, adkRunner, sess.Session, prompt)
	}

	logger.Info().Msg("starting interactive REPL (type 'exit' or 'quit' to stop)")
	return runREPL(ctx, adkRunner, sess.Session)
}

func runREPL(ctx context.Context, r *runner.Runner, sess session.Session) error {
	fmt.Print("> ")
	for {
		var input string
		_, err := fmt.Scanln(&input)
		if err != nil {
			if err.Error() == "unexpected newline" || err.Error() == "EOF" {
				fmt.Println("\nGoodbye!")
				return nil
			}
			return err
		}
		trimmed := input
		if trimmed == "" {
			fmt.Print("> ")
			continue
		}
		if trimmed == "exit" || trimmed == "quit" {
			fmt.Println("Goodbye!")
			return nil
		}
		if err := runACPTurn(ctx, r, sess, trimmed); err != nil {
			return err
		}
		fmt.Print("> ")
	}
}

func runACPTurn(ctx context.Context, r *runner.Runner, sess session.Session, prompt string) error {
	events := r.Run(
		ctx,
		"norma-pecl-user",
		sess.ID(),
		genai.NewContentFromText(prompt, genai.RoleUser),
		agent.RunConfig{},
	)
	for ev, err := range events {
		if err != nil {
			return fmt.Errorf("ACP turn failed: %w", err)
		}
		if ev == nil || ev.Content == nil {
			continue
		}
		for _, part := range ev.Content.Parts {
			if part == nil || part.Text == "" {
				continue
			}
			fmt.Print(part.Text)
		}
		fmt.Println()
	}
	return nil
}

func autoAllowPermission(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindAllowOnce || option.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	for _, option := range req.Options {
		if option.Kind == acp.PermissionOptionKindRejectOnce || option.Kind == acp.PermissionOptionKindRejectAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId),
			}, nil
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
}
