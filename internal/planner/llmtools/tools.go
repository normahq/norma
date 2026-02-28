package llmtools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	HumanToolName        = "human"
	HumanToolDescription = "Ask the user a question for clarification."

	PersistPlanToolName        = "persist_plan"
	PersistPlanToolDescription = "Persist the final decomposition and finish the planning session."

	ShellToolName        = "run_shell_command"
	ShellToolDescription = "Run a shell command for project inspection. Available commands: ls, grep, cat, find, tree, git, go, bd, echo. No pipes or redirects allowed."
)

// HumanArgs defines args for the human tool call.
type HumanArgs struct {
	Question string `json:"question"`
}

// NewHumanTool creates the planner human tool.
func NewHumanTool(ask func(question string) (string, error)) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        HumanToolName,
		Description: HumanToolDescription,
	}, func(tctx tool.Context, args HumanArgs) (string, error) {
		return ask(args.Question)
	})
}

// NewPersistPlanTool creates the planner persist_plan tool.
func NewPersistPlanTool[T any](
	handler func(tctx tool.Context, args T) (string, error),
) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        PersistPlanToolName,
		Description: PersistPlanToolDescription,
	}, handler)
}
