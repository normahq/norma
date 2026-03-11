package planner

import (
	"testing"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestStatusFromEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ev   *session.Event
		want string
	}{
		{
			name: "nil event",
			ev:   nil,
			want: "Waiting for agent updates...",
		},
		{
			name: "function call title",
			ev: eventWithPart(&genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "tool_call",
					Args: map[string]any{"title": "Inspect repo"},
				},
			}),
			want: "Running tool: Inspect repo...",
		},
		{
			name: "function call name fallback",
			ev: eventWithPart(&genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "beads",
				},
			}),
			want: "Running tool: beads...",
		},
		{
			name: "function response title",
			ev: eventWithPart(&genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "tool_update",
					Response: map[string]any{"title": "Inspect repo"},
				},
			}),
			want: "Tool finished: Inspect repo",
		},
		{
			name: "function response name fallback",
			ev: eventWithPart(&genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "beads",
				},
			}),
			want: "Tool finished: beads",
		},
		{
			name: "partial event",
			ev:   partialEvent(),
			want: "Agent is typing...",
		},
		{
			name: "turn complete",
			ev:   turnCompleteEvent(),
			want: "Waiting for next step...",
		},
		{
			name: "default status",
			ev:   &session.Event{},
			want: "Thinking...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := statusFromEvent(tc.ev)
			if got != tc.want {
				t.Fatalf("statusFromEvent() = %q, want %q", got, tc.want)
			}
		})
	}
}

func eventWithPart(part *genai.Part) *session.Event {
	ev := session.NewEvent("inv-1")
	ev.Content = genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel)
	return ev
}

func partialEvent() *session.Event {
	ev := session.NewEvent("inv-partial")
	ev.Partial = true
	return ev
}

func turnCompleteEvent() *session.Event {
	ev := session.NewEvent("inv-complete")
	ev.TurnComplete = true
	return ev
}
