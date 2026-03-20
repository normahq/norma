package pdca

import (
	"encoding/json"
	"testing"

	"github.com/metalagman/norma/internal/agents/roleagent"
)

func TestDoRoleMapRequestRefinesDefaultsToEmptySlice(t *testing.T) {
	role := GetRole(RoleDo)
	if role == nil {
		t.Fatal("GetRole(RoleDo) returned nil")
	}

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[]},"step":{"index":2,"name":"do"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1,"max_wall_time_minutes":10,"max_failed_checks":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"do_input":{"work_plan":{"timebox_minutes":10,"do_steps":[],"check_steps":[],"stop_triggers":[]},"acceptance_criteria_effective":[{"id":"AC-1","origin":"baseline","text":"ok","checks":[{"id":"CHK-1","cmd":"true","expect_exit_codes":[0]}]}]}}`)

	mapped, err := role.MapRequest(roleagent.AgentRequest(reqJSON))
	if err != nil {
		t.Fatalf("role.MapRequest() error = %v", err)
	}

	data, err := json.Marshal(mapped)
	if err != nil {
		t.Fatalf("json.Marshal(mapped) error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal(data) error = %v", err)
	}

	doInput, ok := payload["do_input"].(map[string]any)
	if !ok {
		t.Fatalf("payload[\"do_input\"] type = %T, want map[string]any", payload["do_input"])
	}
	paths, ok := payload["paths"].(map[string]any)
	if !ok {
		t.Fatalf("payload[\"paths\"] type = %T, want map[string]any", payload["paths"])
	}
	if _, hasProgress := paths["progress"]; hasProgress {
		t.Fatalf("payload[\"paths\"] unexpectedly contains progress")
	}

	effectiveAny, ok := doInput["acceptance_criteria_effective"].([]any)
	if !ok {
		t.Fatalf("do_input[\"acceptance_criteria_effective\"] type = %T, want []any", doInput["acceptance_criteria_effective"])
	}
	if len(effectiveAny) != 1 {
		t.Fatalf("len(effectiveAny) = %d, want 1", len(effectiveAny))
	}

	ac, ok := effectiveAny[0].(map[string]any)
	if !ok {
		t.Fatalf("effectiveAny[0] type = %T, want map[string]any", effectiveAny[0])
	}

	refines, ok := ac["refines"].([]any)
	if !ok {
		t.Fatalf("ac[\"refines\"] type = %T, want []any (array, not null)", ac["refines"])
	}
	if len(refines) != 0 {
		t.Fatalf("len(refines) = %d, want 0", len(refines))
	}
}
