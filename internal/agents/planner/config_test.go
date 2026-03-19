package planner

import (
	"strings"
	"testing"
)

func TestPlannerInstruction_ContainsCodexBaseline(t *testing.T) {
	t.Parallel()

	got := plannerInstruction()
	if !strings.Contains(got, codexBaseInstruction) {
		t.Fatalf("plannerInstruction() missing codex baseline: %q", got)
	}
}

func TestPlannerInstruction_ContainsNormaPlannerPolicy(t *testing.T) {
	t.Parallel()

	got := plannerInstruction()
	for _, mustContain := range []string{
		"You are Norma's planning agent.",
		"Use MCP tasks tools ('tasks_*')",
		"Do not call 'bd' directly",
		"Never claim a 'human' tool exists.",
	} {
		if !strings.Contains(got, mustContain) {
			t.Fatalf("plannerInstruction() missing %q: %q", mustContain, got)
		}
	}
	if strings.Contains(got, "Issue Tracker Interface: bd CLI") {
		t.Fatalf("plannerInstruction() should not reference bd CLI interface directly: %q", got)
	}
}
