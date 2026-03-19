package plan

import (
	"strings"
	"testing"
)

func TestPromptTemplateUsesTasksMCPTools(t *testing.T) {
	if !strings.Contains(PromptTemplate, "norma.tasks.*") {
		t.Fatalf("PromptTemplate missing norma.tasks.* contract: %q", PromptTemplate)
	}
	if !strings.Contains(PromptTemplate, "Do not call `bd` directly") {
		t.Fatalf("PromptTemplate missing direct bd prohibition: %q", PromptTemplate)
	}
}
