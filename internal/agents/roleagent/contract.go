package roleagent

import "encoding/json"

type AgentRequest = json.RawMessage

type AgentResponse struct {
	Status     string          `json:"status"`
	StopReason string          `json:"stop_reason,omitempty"`
	Summary    ResponseSummary `json:"summary"`
	Progress   StepProgress    `json:"progress"`
}

type ResponseSummary struct {
	Text string `json:"text"`
}

type StepProgress struct {
	Title   string   `json:"title"`
	Details []string `json:"details"`
}

type SchemaPair struct {
	InputSchema  string
	OutputSchema string
}

type RoleContract interface {
	Name() string
	Schemas() SchemaPair
	Prompt(req AgentRequest) (string, error)
	MapRequest(req AgentRequest) (any, error)
	MapResponse(outBytes []byte) (AgentResponse, error)
}

type CommandRecord struct {
	Id             string `json:"id"`
	Cmd            string `json:"cmd"`
	ExitCode       int    `json:"exit_code,omitempty"`
	ExpectExitCode []int  `json:"expect_exit_code,omitempty"`
}

type ExecutionResult struct {
	ExecutedStepIds []string        `json:"executed_step_ids"`
	SkippedStepIds  []string        `json:"skipped_step_ids"`
	Commands        []CommandRecord `json:"commands,omitempty"`
}

type Blocker struct {
	Kind                string `json:"kind"`
	Text                string `json:"text"`
	SuggestedStopReason string `json:"suggested_stop_reason,omitempty"`
}

type DoOutput struct {
	Execution *ExecutionResult `json:"execution"`
	Blockers  []Blocker        `json:"blockers,omitempty"`
}

type AcceptanceResult struct {
	ACId   string `json:"ac_id"`
	Result string `json:"result"`
	Notes  string `json:"notes,omitempty"`
	LogRef string `json:"log_ref,omitempty"`
}

type CheckOutput struct {
	PlanMatch struct {
		DoSteps struct {
			PlannedIDs    []string `json:"planned_ids"`
			ExecutedIDs   []string `json:"executed_ids"`
			MissingIDs    []string `json:"missing_ids"`
			UnexpectedIDs []string `json:"unexpected_ids"`
		} `json:"do_steps"`
		Commands struct {
			PlannedIDs    []string `json:"planned_ids"`
			ExecutedIDs   []string `json:"executed_ids"`
			MissingIDs    []string `json:"missing_ids"`
			UnexpectedIDs []string `json:"unexpected_ids"`
		} `json:"commands"`
	} `json:"plan_match"`
	AcceptanceResults []AcceptanceResult `json:"acceptance_results"`
	Verdict           *Verdict           `json:"verdict"`
	ProcessNotes      []ProcessNote      `json:"process_notes,omitempty"`
}

type Verdict struct {
	Status         string `json:"status"`
	Recommendation string `json:"recommendation"`
	Basis          struct {
		PlanMatch           string `json:"plan_match"`
		AllAcceptancePassed bool   `json:"all_acceptance_passed"`
	} `json:"basis"`
}

type ProcessNote struct {
	Kind                string `json:"kind"`
	Severity            string `json:"severity"`
	Text                string `json:"text"`
	SuggestedStopReason string `json:"suggested_stop_reason,omitempty"`
}

type ActOutput struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale,omitempty"`
	Next      *struct {
		Recommended bool   `json:"recommended"`
		Notes       string `json:"notes,omitempty"`
	} `json:"next,omitempty"`
}

type PlanOutput struct {
	TaskID             string                    `json:"task_id"`
	Goal               string                    `json:"goal,omitempty"`
	Constraints        []string                  `json:"constraints,omitempty"`
	AcceptanceCriteria *AcceptanceCriteriaOutput `json:"acceptance_criteria,omitempty"`
	WorkPlan           *WorkPlanOutput           `json:"work_plan,omitempty"`
	StopTriggers       []string                  `json:"stop_triggers,omitempty"`
}

type AcceptanceCriteriaOutput struct {
	Baseline  []ACEffective `json:"baseline,omitempty"`
	Effective []ACEffective `json:"effective,omitempty"`
}

type ACEffective struct {
	ID      string                    `json:"id"`
	Origin  string                    `json:"origin,omitempty"`
	Refines []string                  `json:"refines,omitempty"`
	Text    string                    `json:"text"`
	Checks  []AcceptanceCriteriaCheck `json:"checks,omitempty"`
	Reason  string                    `json:"reason,omitempty"`
}

type AcceptanceCriteriaCheck struct {
	ID              string `json:"id"`
	Cmd             string `json:"cmd"`
	ExpectExitCodes []int  `json:"expect_exit_codes"`
}

type WorkPlanOutput struct {
	TimeboxMinutes int               `json:"timebox_minutes"`
	DoSteps        []DoStepOutput    `json:"do_steps,omitempty"`
	CheckSteps     []CheckStepOutput `json:"check_steps,omitempty"`
	StopTriggers   []string          `json:"stop_triggers,omitempty"`
}

type DoStepOutput struct {
	ID           string        `json:"id"`
	Text         string        `json:"text"`
	Commands     []CommandSpec `json:"commands,omitempty"`
	TargetsACIds []string      `json:"targets_ac_ids,omitempty"`
}

type CommandSpec struct {
	ID              string `json:"id"`
	Cmd             string `json:"cmd"`
	ExpectExitCodes []int  `json:"expect_exit_codes,omitempty"`
}

type CheckStepOutput struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Mode string `json:"mode"`
}
