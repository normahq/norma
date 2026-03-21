// Package roleagent wraps an ADK agent with structured I/O validation.
package roleagent

import (
	"github.com/metalagman/norma/internal/adk/structuredio"
	"google.golang.org/adk/agent"
)

// New wraps an embedded agent with structured I/O validation.
// The returned agent validates input against inputSchema and output against outputSchema.
func New(wrapped agent.Agent, inputSchema, outputSchema string) (agent.Agent, error) {
	return structuredio.NewAgent(wrapped,
		structuredio.WithInputSchema(inputSchema),
		structuredio.WithOutputSchema(outputSchema),
	)
}
