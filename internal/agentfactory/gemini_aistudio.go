package agentfactory

import (
	"context"
	"fmt"
	"iter"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

var _ model.LLM = (*GeminiAIStudioModel)(nil)

// GeminiAIStudioModel implements model.LLM using the Google GenAI SDK for AI Studio.
type GeminiAIStudioModel struct {
	client *genai.Client
	model  string
}

// Name returns the model name.
func (m *GeminiAIStudioModel) Name() string {
	return "gemini_aistudio"
}

// GenerateContent executes a request against the Gemini AI Studio API.
func (m *GeminiAIStudioModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		config := &genai.GenerateContentConfig{}
		if req.Config != nil {
			config = req.Config
		}

		if stream {
			for resp, err := range m.client.Models.GenerateContentStream(ctx, m.model, req.Contents, config) {
				if err != nil {
					yield(nil, fmt.Errorf("gemini generate content stream: %w", err))
					return
				}

				if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
					continue
				}

				if !yield(&model.LLMResponse{
					Content: resp.Candidates[0].Content,
				}, nil) {
					return
				}
			}
			return
		}

		resp, err := m.client.Models.GenerateContent(ctx, m.model, req.Contents, config)
		if err != nil {
			yield(nil, fmt.Errorf("gemini generate content: %w", err))
			return
		}

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			yield(nil, fmt.Errorf("no candidates in gemini response"))
			return
		}

		if !yield(&model.LLMResponse{
			Content: resp.Candidates[0].Content,
		}, nil) {
			return
		}
	}
}

// NewGeminiAIStudioLLM creates a Gemini AI Studio LLM model.
func NewGeminiAIStudioLLM(cfg AgentConfig) (model.LLM, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}

	return &GeminiAIStudioModel{
		client: client,
		model:  cfg.Model,
	}, nil
}
