package plancmd

import (
	"fmt"
	"io"
	"os"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/metalagman/norma/internal/adk/agentfactory"
	"github.com/metalagman/norma/internal/adk/modelfactory"
	"github.com/metalagman/norma/internal/config"
	"github.com/metalagman/norma/internal/planner"
	"github.com/metalagman/norma/internal/planner/llmtools"
	"github.com/spf13/cobra"
)

func webCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "web [api|a2a|webui ...]",
		Short:              "Run the planner agent with the ADK web launcher",
		SilenceUsage:       true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			cfg, err := loadConfig(repoRoot)
			if err != nil {
				return err
			}
			plannerCfg, ok := cfg.Agents["planner"]
			if !ok {
				return fmt.Errorf("planner agent not configured in selected profile %q", cfg.Profile)
			}
			if !config.IsPlannerSupportedType(plannerCfg.Type) {
				return fmt.Errorf("planner web launcher supports only llm and acp planner agents, got %q", plannerCfg.Type)
			}

			var plannerDebugAgent adkagent.Agent
			if config.IsACPType(plannerCfg.Type) {
				factory := agentfactory.NewFactory(map[string]config.AgentConfig{
					"planner": plannerCfg,
				})
				creationReq := agentfactory.CreationRequest{
					Name:              "NormaPlannerACP",
					Description:       "Norma planner via ACP runtime",
					WorkingDir:        repoRoot,
					Stderr:            io.Discard,
					PermissionHandler: planner.PlannerACPPermissionHandler,
				}
				acpRuntime, newErr := factory.CreateAgent(cmd.Context(), "planner", creationReq)
				if newErr != nil {
					return fmt.Errorf("create planner ACP runtime: %w", newErr)
				}
				if closer, ok := acpRuntime.(interface{ Close() error }); ok {
					defer func() { _ = closer.Close() }()
				}
				wrappedAgent, wrapErr := planner.WrapAgentWithPlannerPrompt(acpRuntime)
				if wrapErr != nil {
					return fmt.Errorf("create planner ACP wrapper agent: %w", wrapErr)
				}
				plannerDebugAgent = wrappedAgent
			} else {
				factoryCfg := make(modelfactory.FactoryConfig, len(cfg.Agents))
				for name, agentCfg := range cfg.Agents {
					factoryCfg[name] = agentCfg
				}
				factory := modelfactory.NewFactory(factoryCfg)

				modelName := "planner"
				if _, ok := cfg.Agents[modelName]; !ok {
					modelName = "plan"
				}
				m, modelErr := factory.CreateModel(modelName)
				if modelErr != nil {
					return fmt.Errorf("create planner model %q: %w", modelName, modelErr)
				}

				humanTool, humanErr := llmtools.NewHumanTool(func(_ string) (string, error) {
					return "No additional clarification provided.", nil
				})
				if humanErr != nil {
					return fmt.Errorf("create human tool: %w", humanErr)
				}
				beadsTool, beadsErr := llmtools.NewBeadsCommandTool(repoRoot)
				if beadsErr != nil {
					return fmt.Errorf("create beads tool: %w", beadsErr)
				}

				llmPlannerAgent, newErr := llmagent.New(llmagent.Config{
					Name:        "NormaPlanner",
					Model:       m,
					Description: "Interactive Norma planning agent that decomposes epics into features and tasks.",
					Tools:       []tool.Tool{humanTool, beadsTool},
					Instruction: planner.PlannerInstruction(),
				})
				if newErr != nil {
					return fmt.Errorf("create planner debug agent: %w", newErr)
				}
				plannerDebugAgent = llmPlannerAgent
			}

			launchCfg := &launcher.Config{
				AgentLoader:     adkagent.NewSingleLoader(plannerDebugAgent),
				SessionService:  session.InMemoryService(),
				ArtifactService: artifact.InMemoryService(),
			}

			launcherArgs := make([]string, 0, len(args)+1)
			launcherArgs = append(launcherArgs, "web")
			if len(args) == 0 {
				launcherArgs = append(launcherArgs, "api", "webui")
			} else {
				launcherArgs = append(launcherArgs, args...)
			}

			l := full.NewLauncher()
			if err := l.Execute(cmd.Context(), launchCfg, launcherArgs); err != nil {
				return fmt.Errorf("run planner web launcher: %w\n\n%s", err, l.CommandLineSyntax())
			}
			return nil
		},
	}
}
