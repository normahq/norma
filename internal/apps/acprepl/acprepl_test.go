package acprepl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestExtractACPToolEventPartText(t *testing.T) {
	tests := []struct {
		name     string
		part     *genai.Part
		expected string
	}{
		{
			name:     "nil part",
			part:     nil,
			expected: "",
		},
		{
			name:     "empty text",
			part:     &genai.Part{},
			expected: "",
		},
		{
			name:     "text content",
			part:     &genai.Part{Text: "Hello, World!"},
			expected: "Hello, World!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractACPToolEventPartText(tt.part)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestACPThoughtOutput(t *testing.T) {
	part := &genai.Part{
		Text:    "Let me think about this problem step by step...",
		Thought: true,
	}

	text := extractACPToolEventPartText(part)
	assert.Equal(t, "Let me think about this problem step by step...", text)
	assert.True(t, part.Thought)
}

func TestACPFunctionCallDetection(t *testing.T) {
	part := &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   "call-123",
			Name: "acp_tool_call",
			Args: map[string]any{"prompt": "hello"},
		},
	}

	text := extractACPToolEventPartText(part)
	assert.Empty(t, text)
	assert.NotNil(t, part.FunctionCall)
	assert.Equal(t, "call-123", part.FunctionCall.ID)
	assert.Equal(t, "acp_tool_call", part.FunctionCall.Name)
}

func TestACPFunctionResponseDetection(t *testing.T) {
	part := &genai.Part{
		FunctionResponse: &genai.FunctionResponse{
			ID:       "call-123",
			Name:     "acp_tool_call_update",
			Response: map[string]any{"output": "response content"},
		},
	}

	text := extractACPToolEventPartText(part)
	assert.Empty(t, text)
	assert.NotNil(t, part.FunctionResponse)
	assert.Equal(t, "call-123", part.FunctionResponse.ID)
	assert.Equal(t, "acp_tool_call_update", part.FunctionResponse.Name)
}
