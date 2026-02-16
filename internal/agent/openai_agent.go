package agent

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/metalagman/norma/internal/agent/openaiapi"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// OpenAIModel implements model.LLM using the OpenAI Responses API.
type OpenAIModel struct {
	Client completionClient
	Model  string
}

// Name returns the model name.
func (m *OpenAIModel) Name() string {
	return "openai"
}

// GenerateContent executes a request against the OpenAI Responses API.
func (m *OpenAIModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if stream {
			yield(nil, fmt.Errorf("streaming is not supported by OpenAI Responses API in this implementation"))
			return
		}

		var instructions []string
		var inputs []string

		if req.Config != nil && req.Config.SystemInstruction != nil {
			for _, part := range req.Config.SystemInstruction.Parts {
				if part.Text != "" {
					instructions = append(instructions, part.Text)
				}
			}
		}

		for _, content := range req.Contents {
			if content.Role == "system" {
				for _, part := range content.Parts {
					if part.Text != "" {
						instructions = append(instructions, part.Text)
					}
				}
			} else {
				for _, part := range content.Parts {
					if part.Text != "" {
						inputs = append(inputs, part.Text)
					}
				}
			}
		}

		resp, err := m.Client.Complete(ctx, openaiapi.CompletionRequest{
			Instructions: strings.Join(instructions, "\n"),
			Input:        strings.Join(inputs, "\n"),
		})
		if err != nil {
			yield(nil, err)
			return
		}

		yield(&model.LLMResponse{
			Content: genai.NewContentFromText(resp.OutputText, genai.RoleModel),
		}, nil)
	}
}

// NewOpenAIAgent creates a role-agnostic OpenAI agent using llmagent.
func NewOpenAIAgent(client completionClient, modelName, agentName, description, instruction string) (agent.Agent, error) {
	m := &OpenAIModel{
		Client: client,
		Model:  modelName,
	}

	return llmagent.New(llmagent.Config{
		Name:        agentName,
		Description: description,
		Model:       m,
		Instruction: instruction,
	})
}
