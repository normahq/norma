package agent

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/genai"
)

// EventParams configures ProcessEvents.
type EventParams struct {
	Runner      *runner.Runner
	UserID      string
	SessionID   string
	UserContent *genai.Content
}

// ProcessEvents runs the agent and returns the final accumulated text output.
func ProcessEvents(ctx context.Context, p EventParams) (string, error) {
	result := ""

	for ev, err := range p.Runner.Run(ctx, p.UserID, p.SessionID, p.UserContent, agent.RunConfig{}) {
		if err != nil {
			return "", fmt.Errorf("agent run: %w", err)
		}
		if ev == nil {
			continue
		}
		if ev.Content != nil {
			for _, part := range ev.Content.Parts {
				if part == nil {
					continue
				}
				if part.Thought {
					continue
				}
				if part.Text != "" {
					result += part.Text
				}
			}
		}
		if ev.TurnComplete {
			break
		}
	}

	return result, nil
}
