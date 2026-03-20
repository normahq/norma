package roleagent

import (
	"encoding/json"
	"strings"
	"testing"
)

type mockRole struct {
	name         string
	inputSchema  string
	outputSchema string
	promptStr    string
}

func (r *mockRole) Name() string { return r.name }
func (r *mockRole) Schemas() SchemaPair {
	return SchemaPair{InputSchema: r.inputSchema, OutputSchema: r.outputSchema}
}
func (r *mockRole) Prompt(req AgentRequest) (string, error) {
	return r.promptStr, nil
}
func (r *mockRole) MapRequest(req AgentRequest) (any, error) { return nil, nil }
func (r *mockRole) MapResponse(outBytes []byte) (AgentResponse, error) {
	var resp AgentResponse
	err := json.Unmarshal(outBytes, &resp)
	return resp, err
}

func TestSchemaPair(t *testing.T) {
	sp := SchemaPair{
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
	}
	if sp.InputSchema == "" {
		t.Error("expected InputSchema to be set")
	}
	if sp.OutputSchema == "" {
		t.Error("expected OutputSchema to be set")
	}
}

func TestRoleContractInterface(t *testing.T) {
	role := &mockRole{
		name:         "test-role",
		inputSchema:  `{"type":"object","properties":{"id":{"type":"string"}}}`,
		outputSchema: `{"type":"object","properties":{"status":{"type":"string"}}}`,
		promptStr:    "Test prompt",
	}

	var _ RoleContract = role

	if role.Name() != "test-role" {
		t.Errorf("expected name 'test-role', got %q", role.Name())
	}

	schemas := role.Schemas()
	if schemas.InputSchema == "" {
		t.Error("expected InputSchema to be set")
	}
	if schemas.OutputSchema == "" {
		t.Error("expected OutputSchema to be set")
	}
}

func TestCommandRecord(t *testing.T) {
	record := CommandRecord{
		Id:             "CMD-1",
		Cmd:            "go test ./...",
		ExitCode:       0,
		ExpectExitCode: []int{0},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal CommandRecord: %v", err)
	}

	var parsed CommandRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal CommandRecord: %v", err)
	}

	if parsed.Id != "CMD-1" {
		t.Errorf("expected Id 'CMD-1', got %q", parsed.Id)
	}
	if parsed.Cmd != "go test ./..." {
		t.Errorf("expected Cmd 'go test ./...', got %q", parsed.Cmd)
	}
}

func TestExecutionResult(t *testing.T) {
	result := ExecutionResult{
		ExecutedStepIds: []string{"DO-1", "DO-2"},
		SkippedStepIds:  []string{},
		Commands: []CommandRecord{
			{Id: "CMD-1", Cmd: "go build ./...", ExitCode: 0},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal ExecutionResult: %v", err)
	}

	var parsed ExecutionResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ExecutionResult: %v", err)
	}

	if len(parsed.ExecutedStepIds) != 2 {
		t.Errorf("expected 2 executed steps, got %d", len(parsed.ExecutedStepIds))
	}
}

func TestBlocker(t *testing.T) {
	blocker := Blocker{
		Kind:                "dependency",
		Text:                "missing dependency xyz",
		SuggestedStopReason: "dependency_blocked",
	}

	data, err := json.Marshal(blocker)
	if err != nil {
		t.Fatalf("failed to marshal Blocker: %v", err)
	}

	var parsed Blocker
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Blocker: %v", err)
	}

	if parsed.Kind != "dependency" {
		t.Errorf("expected Kind 'dependency', got %q", parsed.Kind)
	}
}

func TestDoOutput(t *testing.T) {
	output := DoOutput{
		Execution: &ExecutionResult{
			ExecutedStepIds: []string{"DO-1"},
			SkippedStepIds:  []string{},
		},
		Blockers: []Blocker{
			{Kind: "env", Text: "missing env var"},
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal DoOutput: %v", err)
	}

	var parsed DoOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DoOutput: %v", err)
	}

	if len(parsed.Blockers) != 1 {
		t.Errorf("expected 1 blocker, got %d", len(parsed.Blockers))
	}
}

func TestAcceptanceResult(t *testing.T) {
	result := AcceptanceResult{
		ACId:   "AC-1",
		Result: "PASS",
		Notes:  "All checks passed",
		LogRef: "steps/03-check/logs/stdout.txt",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal AcceptanceResult: %v", err)
	}

	var parsed AcceptanceResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AcceptanceResult: %v", err)
	}

	if parsed.ACId != "AC-1" {
		t.Errorf("expected ACId 'AC-1', got %q", parsed.ACId)
	}
	if parsed.Result != "PASS" {
		t.Errorf("expected Result 'PASS', got %q", parsed.Result)
	}
}

func TestCheckOutput(t *testing.T) {
	output := CheckOutput{}
	output.PlanMatch.DoSteps.PlannedIDs = []string{"DO-1", "DO-2"}
	output.PlanMatch.DoSteps.ExecutedIDs = []string{"DO-1"}
	output.PlanMatch.DoSteps.MissingIDs = []string{"DO-2"}
	output.PlanMatch.Commands.PlannedIDs = []string{"CMD-1"}
	output.PlanMatch.Commands.ExecutedIDs = []string{"CMD-1"}
	output.AcceptanceResults = []AcceptanceResult{
		{ACId: "AC-1", Result: "PASS"},
	}
	output.Verdict = &Verdict{
		Status:         "FAIL",
		Recommendation: "replan",
	}
	output.Verdict.Basis.PlanMatch = "MISMATCH"
	output.Verdict.Basis.AllAcceptancePassed = false

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal CheckOutput: %v", err)
	}

	var parsed CheckOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal CheckOutput: %v", err)
	}

	if len(parsed.PlanMatch.DoSteps.MissingIDs) != 1 {
		t.Errorf("expected 1 missing step, got %d", len(parsed.PlanMatch.DoSteps.MissingIDs))
	}
}

func TestVerdict(t *testing.T) {
	verdict := Verdict{
		Status:         "PASS",
		Recommendation: "continue",
	}
	verdict.Basis.PlanMatch = "MATCH"
	verdict.Basis.AllAcceptancePassed = true

	if verdict.Status != "PASS" {
		t.Errorf("expected Status 'PASS', got %q", verdict.Status)
	}
	if verdict.Basis.AllAcceptancePassed != true {
		t.Error("expected AllAcceptancePassed to be true")
	}
}

func TestActOutput(t *testing.T) {
	output := ActOutput{
		Decision:  "replan",
		Rationale: "check failed, need to fix issues",
	}
	nextStep := struct {
		Recommended bool   `json:"recommended"`
		Notes       string `json:"notes,omitempty"`
	}{
		Recommended: true,
		Notes:       "fix the failing test",
	}
	output.Next = &nextStep

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal ActOutput: %v", err)
	}

	var parsed ActOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ActOutput: %v", err)
	}

	if parsed.Decision != "replan" {
		t.Errorf("expected Decision 'replan', got %q", parsed.Decision)
	}
}

func TestPlanOutput(t *testing.T) {
	output := PlanOutput{
		TaskID: "norma-abc123",
		Goal:   "implement feature X",
		Constraints: []string{
			"must pass all tests",
		},
		WorkPlan: &WorkPlanOutput{
			TimeboxMinutes: 30,
			DoSteps: []DoStepOutput{
				{ID: "DO-1", Text: "implement feature", TargetsACIds: []string{"AC-1"}},
			},
			CheckSteps: []CheckStepOutput{
				{ID: "VER-1", Text: "verify implementation", Mode: "acceptance_criteria"},
			},
			StopTriggers: []string{"dependency_blocked", "budget_exceeded"},
		},
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal PlanOutput: %v", err)
	}

	var parsed PlanOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal PlanOutput: %v", err)
	}

	if parsed.TaskID != "norma-abc123" {
		t.Errorf("expected TaskID 'norma-abc123', got %q", parsed.TaskID)
	}
	if len(parsed.WorkPlan.DoSteps) != 1 {
		t.Errorf("expected 1 do step, got %d", len(parsed.WorkPlan.DoSteps))
	}
}

func TestACEffective(t *testing.T) {
	ac := ACEffective{
		ID:      "AC-1",
		Origin:  "baseline",
		Refines: []string{},
		Text:    "Unit tests pass",
		Checks: []AcceptanceCriteriaCheck{
			{ID: "CHK-1", Cmd: "go test ./...", ExpectExitCodes: []int{0}},
		},
		Reason: "core requirement",
	}

	data, err := json.Marshal(ac)
	if err != nil {
		t.Fatalf("failed to marshal ACEffective: %v", err)
	}

	var parsed ACEffective
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ACEffective: %v", err)
	}

	if parsed.Origin != "baseline" {
		t.Errorf("expected Origin 'baseline', got %q", parsed.Origin)
	}
	if len(parsed.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(parsed.Checks))
	}
}

func TestAcceptanceCriteriaCheck(t *testing.T) {
	check := AcceptanceCriteriaCheck{
		ID:              "CHK-AC-1-1",
		Cmd:             "go test -race ./internal/agents/roleagent/...",
		ExpectExitCodes: []int{0},
	}

	data, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("failed to marshal AcceptanceCriteriaCheck: %v", err)
	}

	var parsed AcceptanceCriteriaCheck
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AcceptanceCriteriaCheck: %v", err)
	}

	if parsed.ID != "CHK-AC-1-1" {
		t.Errorf("expected ID 'CHK-AC-1-1', got %q", parsed.ID)
	}
	if len(parsed.ExpectExitCodes) != 1 || parsed.ExpectExitCodes[0] != 0 {
		t.Errorf("expected ExpectExitCodes [0], got %v", parsed.ExpectExitCodes)
	}
}

func TestBasePromptBuilder(t *testing.T) {
	rolePrompt := `You are the {{.Role.Name}} agent.
{{.Common}}
Role-specific instructions here.`

	builder, err := newBasePromptBuilder("test", rolePrompt)
	if err != nil {
		t.Fatalf("failed to create basePromptBuilder: %v", err)
	}

	if builder.RoleName() != "test" {
		t.Errorf("expected role name 'test', got %q", builder.RoleName())
	}

	prompt, err := builder.Build(
		map[string]string{"Common": "Shared context"},
		map[string]string{"Name": "test-role"},
	)
	if err != nil {
		t.Fatalf("failed to build prompt: %v", err)
	}

	if !strings.Contains(prompt, "test agent") {
		t.Error("expected prompt to contain 'test agent'")
	}
	if !strings.Contains(prompt, "Shared context") {
		t.Error("expected prompt to contain 'Shared context'")
	}
	if !strings.Contains(prompt, "Role-specific instructions") {
		t.Error("expected prompt to contain 'Role-specific instructions'")
	}
}

func TestBasePromptBuilderContainsCommonInstructions(t *testing.T) {
	rolePrompt := `{{.Role}}`
	builder, err := newBasePromptBuilder("do", rolePrompt)
	if err != nil {
		t.Fatalf("failed to create basePromptBuilder: %v", err)
	}

	prompt, err := builder.Build(nil, "do-role-data")
	if err != nil {
		t.Fatalf("failed to build prompt: %v", err)
	}

	if !strings.Contains(prompt, "ACCESS RESTRICTION") {
		t.Error("expected prompt to contain 'ACCESS RESTRICTION'")
	}
	if !strings.Contains(prompt, "do agent") {
		t.Error("expected prompt to contain 'do agent'")
	}
	if !strings.Contains(prompt, "workspace") {
		t.Error("expected prompt to contain 'workspace'")
	}
}

func TestBasePromptBuilderBuildFromRequest(t *testing.T) {
	rolePrompt := `{{.Role}}`
	builder, err := newBasePromptBuilder("plan", rolePrompt)
	if err != nil {
		t.Fatalf("failed to create basePromptBuilder: %v", err)
	}

	req := json.RawMessage(`{"task_id":"norma-123"}`)
	prompt, err := builder.BuildFromRequest(req, "plan-data")
	if err != nil {
		t.Fatalf("failed to build prompt: %v", err)
	}

	if !strings.Contains(prompt, "plan agent") {
		t.Error("expected prompt to contain 'plan agent'")
	}
}

func TestAgentResponse(t *testing.T) {
	resp := AgentResponse{
		Status:     "ok",
		StopReason: "",
		Summary:    ResponseSummary{Text: "completed successfully"},
		Progress:   StepProgress{Title: "done", Details: []string{"step 1", "step 2"}},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal AgentResponse: %v", err)
	}

	var parsed AgentResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AgentResponse: %v", err)
	}

	if parsed.Status != "ok" {
		t.Errorf("expected Status 'ok', got %q", parsed.Status)
	}
	if len(parsed.Progress.Details) != 2 {
		t.Errorf("expected 2 progress details, got %d", len(parsed.Progress.Details))
	}
}
