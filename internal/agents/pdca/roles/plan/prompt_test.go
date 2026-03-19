package plan

import (
	"strings"
	"testing"
)

func TestPromptTemplateUsesTasksMCPTools(t *testing.T) {
	if !strings.Contains(PromptTemplate, "tasks_*") {
		t.Fatalf("PromptTemplate missing tasks_* contract: %q", PromptTemplate)
	}
	if !strings.Contains(PromptTemplate, "Do not call `bd` directly") {
		t.Fatalf("PromptTemplate missing direct bd prohibition: %q", PromptTemplate)
	}
}
