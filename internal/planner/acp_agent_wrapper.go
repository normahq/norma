package planner

import (
	"iter"
	"strings"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type plannerPromptInvocationContext struct {
	adkagent.InvocationContext
	userContent *genai.Content
}

func (c plannerPromptInvocationContext) UserContent() *genai.Content {
	return c.userContent
}

// WrapAgentWithPlannerPrompt ensures ACP agents always receive the planner system instruction.
func WrapAgentWithPlannerPrompt(base adkagent.Agent) (adkagent.Agent, error) {
	return adkagent.New(adkagent.Config{
		Name:        base.Name(),
		Description: base.Description(),
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*session.Event, error] {
			userMessage := ""
			if content := ctx.UserContent(); content != nil {
				userMessage = extractContentText(content)
			}
			wrappedContent := genai.NewContentFromText(PlannerPromptForUserInput(userMessage), genai.RoleUser)
			wrappedCtx := plannerPromptInvocationContext{
				InvocationContext: ctx,
				userContent:       wrappedContent,
			}
			return base.Run(wrappedCtx)
		},
	})
}

func extractContentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return strings.TrimSpace(b.String())
}
