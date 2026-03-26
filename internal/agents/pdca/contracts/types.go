package contracts

import (
	"encoding/json"

	"github.com/normahq/norma/internal/task"
)

// RawAgentRequest is the raw JSON bytes passed to role MapRequest implementations.
type RawAgentRequest = json.RawMessage

// SchemaPair holds input and output JSON schemas for a role.
type SchemaPair struct {
	InputSchema  string
	OutputSchema string
}

// Budgets defines run budgets.
type Budgets struct {
	MaxIterations      int `json:"max_iterations"`
	MaxWallTimeMinutes int `json:"max_wall_time_minutes,omitempty"`
	MaxFailedChecks    int `json:"max_failed_checks,omitempty"`
}

// AgentRequest is the normalized request passed to agents.
// Each role reads what it needs from TaskState.
type AgentRequest struct {
	Run     RunInfo        `json:"run"`
	Task    TaskInfo       `json:"task"`
	Step    StepInfo       `json:"step"`
	Paths   RequestPaths   `json:"paths"`
	Budgets Budgets        `json:"budgets"`
	Context RequestContext `json:"context"`

	StopReasonsAllowed []string `json:"stop_reasons_allowed"`

	// TaskState contains outputs from all previous roles.
	// Each role reads what it needs from this shared state.
	TaskState TaskState `json:"task_state"`
}

// RunInfo identifies the current run and its iteration.
type RunInfo struct {
	ID        string `json:"id"`
	Iteration int    `json:"iteration"`
}

// TaskInfo contains identification and description info for an issue.
type TaskInfo struct {
	ID                 string                     `json:"id"`
	Title              string                     `json:"title"`
	Description        string                     `json:"description"`
	AcceptanceCriteria []task.AcceptanceCriterion `json:"acceptance_criteria"`
}

// StepInfo identifies the step in the run.
type StepInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"` // "plan", "do", "check", "act"
}

// RequestPaths are absolute paths for agent execution.
type RequestPaths struct {
	WorkspaceDir string `json:"workspace_dir"`
	RunDir       string `json:"run_dir"`
}

// RequestContext supplies artifacts from previous steps and optional notes.
type RequestContext struct {
	Facts   map[string]any `json:"facts"`
	Links   []string       `json:"links"`
	Attempt int            `json:"attempt,omitempty"`
}

// RawAgentResponse is the response with json.RawMessage fields used by role MapResponse implementations.
type RawAgentResponse struct {
	Status     string          `json:"status"`
	StopReason string          `json:"stop_reason,omitempty"`
	Summary    ResponseSummary `json:"summary"`
	Progress   StepProgress    `json:"progress"`

	PlanOutput  json.RawMessage `json:"plan_output,omitempty"`
	DoOutput    json.RawMessage `json:"do_output,omitempty"`
	CheckOutput json.RawMessage `json:"check_output,omitempty"`
	ActOutput   json.RawMessage `json:"act_output,omitempty"`
}

// ResponseSummary captures the outcome of an agent's task.
type ResponseSummary struct {
	Text string `json:"text"`
}

// StepProgress captures highlights for the run journal.
type StepProgress struct {
	Title   string   `json:"title"`
	Details []string `json:"details"`
}

// TaskState is stored in task notes to persist step outputs and journal across runs.
// Each role reads/writes its own output field.
type TaskState struct {
	Plan    json.RawMessage `json:"plan,omitempty"`
	Do      json.RawMessage `json:"do,omitempty"`
	Check   json.RawMessage `json:"check,omitempty"`
	Act     json.RawMessage `json:"act,omitempty"`
	Journal []JournalEntry  `json:"journal,omitempty"`
}

// JournalEntry records detailed progress for a single step.
type JournalEntry struct {
	Timestamp  string   `json:"timestamp"`
	RunID      string   `json:"run_id,omitempty"`
	Iteration  int      `json:"iteration,omitempty"`
	StepIndex  int      `json:"step_index"`
	Role       string   `json:"role"`
	Status     string   `json:"status"`
	StopReason string   `json:"stop_reason"`
	Title      string   `json:"title"`
	Details    []string `json:"details"`
}
