package pdca

import (
	"encoding/json"
	"testing"

	"github.com/normahq/norma/internal/agents/pdca/contracts"
)

func TestDoRoleMapRequestRefinesDefaultsToEmptySlice(t *testing.T) {
	role := Role(RoleDo)
	if role == nil {
		t.Fatal("Role(RoleDo) returned nil")
	}

	reqJSON := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[]},"step":{"index":2,"name":"do"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1,"max_wall_time_minutes":10,"max_failed_checks":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"task_state":{"plan":{"acceptance_criteria":{"effective":[{"id":"AC-1","origin":"baseline","text":"ok","checks":[{"id":"CHK-1","cmd":"true","expect_exit_codes":[0]}]}]},"work_plan":{"timebox_minutes":10,"do_steps":[],"check_steps":[],"stop_triggers":[]}}}}`)

	mapped, err := role.MapRequest(contracts.RawAgentRequest(reqJSON))
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

func TestAllRolesImplementRoleContract(t *testing.T) {
	t.Parallel()

	expectedRoles := []string{RolePlan, RoleDo, RoleCheck, RoleAct}

	for _, name := range expectedRoles {
		role := Role(name)
		if role == nil {
			t.Errorf("Role(%q) returned nil", name)
			continue
		}
		if role.Name() != name {
			t.Errorf("role.Name() = %q, want %q", role.Name(), name)
		}
	}
}

func TestAllRolesReturnValidSchemas(t *testing.T) {
	t.Parallel()

	expectedRoles := []string{RolePlan, RoleDo, RoleCheck, RoleAct}

	for _, name := range expectedRoles {
		role := Role(name)
		if role == nil {
			t.Errorf("Role(%q) returned nil", name)
			continue
		}

		schemas := role.Schemas()
		if schemas.InputSchema == "" {
			t.Errorf("role %q has empty InputSchema", name)
		}
		if schemas.OutputSchema == "" {
			t.Errorf("role %q has empty OutputSchema", name)
		}
		// Verify schemas are valid JSON
		if !json.Valid([]byte(schemas.InputSchema)) {
			t.Errorf("role %q InputSchema is not valid JSON", name)
		}
		if !json.Valid([]byte(schemas.OutputSchema)) {
			t.Errorf("role %q OutputSchema is not valid JSON", name)
		}
	}
}

func TestAllRolesMapResponseReturnsAgentResponse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response string
	}{
		{"plan", `{"status":"ok","summary":{"text":"done"},"progress":{"title":"plan done","details":["created plan"]},"plan_output":{"acceptance_criteria":{"effective":[]},"work_plan":{"timebox_minutes":10,"do_steps":[],"check_steps":[]}}}`},
		{"do", `{"status":"ok","summary":{"text":"done"},"progress":{"title":"do done","details":["executed"]},"do_output":{"execution":{"executed_step_ids":[],"skipped_step_ids":[]}}}`},
		{"check", `{"status":"ok","summary":{"text":"done"},"progress":{"title":"check done","details":["verified"]},"check_output":{"plan_match":{"do_steps":{"planned_ids":[],"executed_ids":[],"missing_ids":[],"unexpected_ids":[]},"commands":{"planned_ids":[],"executed_ids":[],"missing_ids":[],"unexpected_ids":[]}},"acceptance_results":[],"verdict":{"status":"PASS","recommendation":"standardize","basis":{"plan_match":"MATCH","all_acceptance_passed":true}}}}`},
		{"act", `{"status":"ok","summary":{"text":"done"},"progress":{"title":"act done","details":["decided"]},"act_output":{"decision":"close","rationale":"completed"}}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			role := Role(tc.name)
			if role == nil {
				t.Fatalf("Role(%q) returned nil", tc.name)
			}

			resp, err := role.MapResponse([]byte(tc.response))
			if err != nil {
				t.Fatalf("MapResponse() error = %v", err)
			}

			if resp.Status != "ok" {
				t.Errorf("resp.Status = %q, want %q", resp.Status, "ok")
			}
			if resp.Summary.Text != "done" {
				t.Errorf("resp.Summary.Text = %q, want %q", resp.Summary.Text, "done")
			}
			if resp.Progress.Title == "" {
				t.Error("resp.Progress.Title is empty")
			}
		})
	}
}

func TestAllRolesMapRequestAcceptsValidJSON(t *testing.T) {
	t.Parallel()

	// Plan only needs task ID
	planReq := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"test"}]},"step":{"index":1,"name":"plan"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"task_state":{}}`)

	// Do needs plan in task_state
	doReq := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"test"}]},"step":{"index":2,"name":"do"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"task_state":{"plan":{"acceptance_criteria":{"effective":[{"id":"AC1","origin":"baseline","text":"test","checks":[]}]},"work_plan":{"timebox_minutes":10,"do_steps":[],"check_steps":[]}}}}`)

	// Check needs plan and do in task_state
	checkReq := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"test"}]},"step":{"index":3,"name":"check"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"task_state":{"plan":{"acceptance_criteria":{"effective":[{"id":"AC1","origin":"baseline","text":"test","checks":[]}]},"work_plan":{"timebox_minutes":10,"do_steps":[],"check_steps":[]}},"do":{"execution":{"executed_step_ids":[],"skipped_step_ids":[]}}}}`)

	// Act needs check in task_state
	actReq := []byte(`{"run":{"id":"run-1","iteration":1},"task":{"id":"task-1","title":"title","description":"desc","acceptance_criteria":[{"id":"AC1","text":"test"}]},"step":{"index":4,"name":"act"},"paths":{"workspace_dir":"/tmp","run_dir":"/tmp"},"budgets":{"max_iterations":1},"context":{"facts":{},"links":[]},"stop_reasons_allowed":["budget_exceeded"],"task_state":{"check":{"verdict":{"status":"PASS","recommendation":"standardize","basis":{"plan_match":"MATCH","all_acceptance_passed":true}},"acceptance_results":[]}}}`)

	testCases := []struct {
		name    string
		request []byte
	}{
		{"plan", planReq},
		{"do", doReq},
		{"check", checkReq},
		{"act", actReq},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			role := Role(tc.name)
			if role == nil {
				t.Fatalf("Role(%q) returned nil", tc.name)
			}

			_, err := role.MapRequest(contracts.RawAgentRequest(tc.request))
			if err != nil {
				t.Errorf("MapRequest() error = %v", err)
			}
		})
	}
}
