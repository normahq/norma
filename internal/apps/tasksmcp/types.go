package tasksmcp

type noInput struct{}

type taskCriterionInput struct {
	ID          string   `json:"id,omitempty" jsonschema:"acceptance criterion id"`
	Text        string   `json:"text" jsonschema:"acceptance criterion text"`
	VerifyHints []string `json:"verify_hints,omitempty" jsonschema:"verification hints"`
}

type taskCriterion struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	VerifyHints []string `json:"verify_hints,omitempty"`
}

type taskRecord struct {
	ID        string          `json:"id"`
	Type      string          `json:"type,omitempty"`
	ParentID  string          `json:"parent_id,omitempty"`
	Title     string          `json:"title"`
	Goal      string          `json:"goal,omitempty"`
	Criteria  []taskCriterion `json:"criteria,omitempty"`
	Status    string          `json:"status,omitempty"`
	RunID     *string         `json:"run_id,omitempty"`
	Priority  int             `json:"priority,omitempty"`
	Assignee  string          `json:"assignee,omitempty"`
	Labels    []string        `json:"labels,omitempty"`
	Notes     string          `json:"notes,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

type ToolError struct {
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type ToolOutcome struct {
	OK    bool       `json:"ok"`
	Error *ToolError `json:"error,omitempty"`
}

type basicOutput struct {
	ToolOutcome
}

type addTaskInput struct {
	Title    string               `json:"title" jsonschema:"task title"`
	Goal     string               `json:"goal,omitempty" jsonschema:"task goal"`
	Criteria []taskCriterionInput `json:"criteria,omitempty" jsonschema:"acceptance criteria"`
	RunID    *string              `json:"run_id,omitempty" jsonschema:"external run id"`
}

type addTaskOutput struct {
	ToolOutcome
	TaskID string `json:"task_id,omitempty"`
}

type addEpicInput struct {
	Title string `json:"title" jsonschema:"epic title"`
	Goal  string `json:"goal,omitempty" jsonschema:"epic goal"`
}

type addEpicOutput struct {
	ToolOutcome
	TaskID string `json:"task_id,omitempty"`
}

type addFeatureInput struct {
	EpicID string `json:"epic_id" jsonschema:"epic id"`
	Title  string `json:"title" jsonschema:"feature title"`
}

type addFeatureOutput struct {
	ToolOutcome
	TaskID string `json:"task_id,omitempty"`
}

type addFollowUpInput struct {
	ParentID string               `json:"parent_id" jsonschema:"parent task id"`
	Title    string               `json:"title" jsonschema:"follow-up title"`
	Goal     string               `json:"goal,omitempty" jsonschema:"follow-up goal"`
	Criteria []taskCriterionInput `json:"criteria,omitempty" jsonschema:"acceptance criteria"`
}

type addFollowUpOutput struct {
	ToolOutcome
	TaskID string `json:"task_id,omitempty"`
}

type listTasksInput struct {
	Status *string `json:"status,omitempty" jsonschema:"optional status filter"`
}

type listTasksOutput struct {
	ToolOutcome
	Tasks []taskRecord `json:"tasks,omitempty"`
}

type listFeaturesInput struct {
	EpicID string `json:"epic_id" jsonschema:"epic id"`
}

type listFeaturesOutput struct {
	ToolOutcome
	Tasks []taskRecord `json:"tasks,omitempty"`
}

type childrenInput struct {
	ParentID string `json:"parent_id" jsonschema:"parent task id"`
}

type childrenOutput struct {
	ToolOutcome
	Tasks []taskRecord `json:"tasks,omitempty"`
}

type getTaskInput struct {
	ID string `json:"id" jsonschema:"task id"`
}

type getTaskOutput struct {
	ToolOutcome
	Task taskRecord `json:"task,omitempty"`
}

type idInput struct {
	ID string `json:"id" jsonschema:"task id"`
}

type markStatusInput struct {
	ID     string `json:"id" jsonschema:"task id"`
	Status string `json:"status" jsonschema:"task status"`
}

type updateTaskInput struct {
	ID    string `json:"id" jsonschema:"task id"`
	Title string `json:"title" jsonschema:"new title"`
	Goal  string `json:"goal,omitempty" jsonschema:"new goal"`
}

type setRunInput struct {
	ID    string `json:"id" jsonschema:"task id"`
	RunID string `json:"run_id" jsonschema:"external run id"`
}

type addDependencyInput struct {
	TaskID      string `json:"task_id" jsonschema:"dependent task id"`
	DependsOnID string `json:"depends_on_id" jsonschema:"dependency task id"`
}

type workflowStateInput struct {
	ID    string `json:"id" jsonschema:"task id"`
	State string `json:"state" jsonschema:"workflow state"`
}

type labelInput struct {
	ID    string `json:"id" jsonschema:"task id"`
	Label string `json:"label" jsonschema:"label value"`
}

type setNotesInput struct {
	ID    string `json:"id" jsonschema:"task id"`
	Notes string `json:"notes" jsonschema:"task notes"`
}

type closeWithReasonInput struct {
	ID     string `json:"id" jsonschema:"task id"`
	Reason string `json:"reason" jsonschema:"close reason"`
}

type addRelatedLinkInput struct {
	FromID string `json:"from_id" jsonschema:"source task id"`
	ToID   string `json:"to_id" jsonschema:"target task id"`
}
