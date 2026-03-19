package tasksmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/metalagman/norma/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "norma-tasks"
	serverVersion = "1.0.0"

	codeValidationError = "validation_error"
	codeBackendError    = "backend_error"
)

// Run serves the tasks MCP server over stdio.
func Run(ctx context.Context, tracker task.Tracker) error {
	server, err := NewServer(tracker)
	if err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

// NewServer builds the tasks MCP server with tracker-parity tools.
func NewServer(tracker task.Tracker) (*mcp.Server, error) {
	if tracker == nil {
		return nil, fmt.Errorf("tracker is required")
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		nil,
	)

	svc := &service{tracker: tracker}
	svc.registerTools(server)
	return server, nil
}

type service struct {
	tracker task.Tracker
}

func (s *service) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add", Description: "Create a task."}, s.addTask)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_epic", Description: "Create an epic."}, s.addEpic)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_feature", Description: "Create a feature under an epic."}, s.addFeature)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_follow_up", Description: "Create a follow-up task under a parent."}, s.addFollowUp)

	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.list", Description: "List tasks, optionally by status."}, s.listTasks)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.list_features", Description: "List features under an epic."}, s.listFeatures)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.children", Description: "List child tasks for a parent task."}, s.listChildren)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.get", Description: "Get a task by id."}, s.getTask)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.leaf", Description: "List ready leaf tasks."}, s.leafTasks)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.list_blocked_dependents", Description: "List tasks blocked by the given task id."}, s.listBlockedDependents)

	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.mark_done", Description: "Mark task as done."}, s.markDone)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.mark_status", Description: "Mark task with status."}, s.markStatus)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.update_workflow_state", Description: "Update granular workflow state label."}, s.updateWorkflowState)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.update", Description: "Update task title and goal."}, s.updateTask)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.delete", Description: "Delete a task."}, s.deleteTask)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.set_run", Description: "Set external run id for a task."}, s.setRun)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.set_notes", Description: "Set task notes."}, s.setNotes)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.close_with_reason", Description: "Close a task with explicit reason."}, s.closeWithReason)

	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_dependency", Description: "Add dependency: task_id depends on depends_on_id."}, s.addDependency)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_related_link", Description: "Add bidirectional related link between two tasks."}, s.addRelatedLink)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.add_label", Description: "Add a label to a task."}, s.addLabel)
	mcp.AddTool(server, &mcp.Tool{Name: "norma.tasks.remove_label", Description: "Remove a label from a task."}, s.removeLabel)
}

func (s *service) addTask(ctx context.Context, _ *mcp.CallToolRequest, in addTaskInput) (*mcp.CallToolResult, addTaskOutput, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		result, out := validationFailure("norma.tasks.add", "title is required")
		return result, addTaskOutput{ToolOutcome: out}, nil
	}

	runID := trimOptionalString(in.RunID)
	id, err := s.tracker.Add(ctx, title, strings.TrimSpace(in.Goal), toCriteria(in.Criteria), runID)
	if err != nil {
		result, out := backendFailure("norma.tasks.add", err)
		return result, addTaskOutput{ToolOutcome: out}, nil
	}
	return nil, addTaskOutput{ToolOutcome: okOutcome(), TaskID: id}, nil
}

func (s *service) addEpic(ctx context.Context, _ *mcp.CallToolRequest, in addEpicInput) (*mcp.CallToolResult, addEpicOutput, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		result, out := validationFailure("norma.tasks.add_epic", "title is required")
		return result, addEpicOutput{ToolOutcome: out}, nil
	}

	id, err := s.tracker.AddEpic(ctx, title, strings.TrimSpace(in.Goal))
	if err != nil {
		result, out := backendFailure("norma.tasks.add_epic", err)
		return result, addEpicOutput{ToolOutcome: out}, nil
	}
	return nil, addEpicOutput{ToolOutcome: okOutcome(), TaskID: id}, nil
}

func (s *service) addFeature(ctx context.Context, _ *mcp.CallToolRequest, in addFeatureInput) (*mcp.CallToolResult, addFeatureOutput, error) {
	epicID := strings.TrimSpace(in.EpicID)
	if epicID == "" {
		result, out := validationFailure("norma.tasks.add_feature", "epic_id is required")
		return result, addFeatureOutput{ToolOutcome: out}, nil
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		result, out := validationFailure("norma.tasks.add_feature", "title is required")
		return result, addFeatureOutput{ToolOutcome: out}, nil
	}

	id, err := s.tracker.AddFeature(ctx, epicID, title)
	if err != nil {
		result, out := backendFailure("norma.tasks.add_feature", err)
		return result, addFeatureOutput{ToolOutcome: out}, nil
	}
	return nil, addFeatureOutput{ToolOutcome: okOutcome(), TaskID: id}, nil
}

func (s *service) addFollowUp(ctx context.Context, _ *mcp.CallToolRequest, in addFollowUpInput) (*mcp.CallToolResult, addFollowUpOutput, error) {
	parentID := strings.TrimSpace(in.ParentID)
	if parentID == "" {
		result, out := validationFailure("norma.tasks.add_follow_up", "parent_id is required")
		return result, addFollowUpOutput{ToolOutcome: out}, nil
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		result, out := validationFailure("norma.tasks.add_follow_up", "title is required")
		return result, addFollowUpOutput{ToolOutcome: out}, nil
	}

	id, err := s.tracker.AddFollowUp(ctx, parentID, title, strings.TrimSpace(in.Goal), toCriteria(in.Criteria))
	if err != nil {
		result, out := backendFailure("norma.tasks.add_follow_up", err)
		return result, addFollowUpOutput{ToolOutcome: out}, nil
	}
	return nil, addFollowUpOutput{ToolOutcome: okOutcome(), TaskID: id}, nil
}

func (s *service) listTasks(ctx context.Context, _ *mcp.CallToolRequest, in listTasksInput) (*mcp.CallToolResult, listTasksOutput, error) {
	status := trimOptionalString(in.Status)
	items, err := s.tracker.List(ctx, status)
	if err != nil {
		result, out := backendFailure("norma.tasks.list", err)
		return result, listTasksOutput{ToolOutcome: out}, nil
	}
	return nil, listTasksOutput{ToolOutcome: okOutcome(), Tasks: mapTasks(items)}, nil
}

func (s *service) listFeatures(ctx context.Context, _ *mcp.CallToolRequest, in listFeaturesInput) (*mcp.CallToolResult, listFeaturesOutput, error) {
	epicID := strings.TrimSpace(in.EpicID)
	if epicID == "" {
		result, out := validationFailure("norma.tasks.list_features", "epic_id is required")
		return result, listFeaturesOutput{ToolOutcome: out}, nil
	}

	items, err := s.tracker.ListFeatures(ctx, epicID)
	if err != nil {
		result, out := backendFailure("norma.tasks.list_features", err)
		return result, listFeaturesOutput{ToolOutcome: out}, nil
	}
	return nil, listFeaturesOutput{ToolOutcome: okOutcome(), Tasks: mapTasks(items)}, nil
}

func (s *service) listChildren(ctx context.Context, _ *mcp.CallToolRequest, in childrenInput) (*mcp.CallToolResult, childrenOutput, error) {
	parentID := strings.TrimSpace(in.ParentID)
	if parentID == "" {
		result, out := validationFailure("norma.tasks.children", "parent_id is required")
		return result, childrenOutput{ToolOutcome: out}, nil
	}

	items, err := s.tracker.Children(ctx, parentID)
	if err != nil {
		result, out := backendFailure("norma.tasks.children", err)
		return result, childrenOutput{ToolOutcome: out}, nil
	}
	return nil, childrenOutput{ToolOutcome: okOutcome(), Tasks: mapTasks(items)}, nil
}

func (s *service) getTask(ctx context.Context, _ *mcp.CallToolRequest, in getTaskInput) (*mcp.CallToolResult, getTaskOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.get", "id is required")
		return result, getTaskOutput{ToolOutcome: out}, nil
	}

	item, err := s.tracker.Task(ctx, id)
	if err != nil {
		result, out := backendFailure("norma.tasks.get", err)
		return result, getTaskOutput{ToolOutcome: out}, nil
	}
	return nil, getTaskOutput{ToolOutcome: okOutcome(), Task: toTaskRecord(item)}, nil
}

func (s *service) markDone(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.mark_done", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.MarkDone(ctx, id); err != nil {
		result, out := backendFailure("norma.tasks.mark_done", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) markStatus(ctx context.Context, _ *mcp.CallToolRequest, in markStatusInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.mark_status", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		result, out := validationFailure("norma.tasks.mark_status", "status is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.MarkStatus(ctx, id, status); err != nil {
		result, out := backendFailure("norma.tasks.mark_status", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) updateTask(ctx context.Context, _ *mcp.CallToolRequest, in updateTaskInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.update", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		result, out := validationFailure("norma.tasks.update", "title is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.Update(ctx, id, title, strings.TrimSpace(in.Goal)); err != nil {
		result, out := backendFailure("norma.tasks.update", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) deleteTask(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.delete", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.Delete(ctx, id); err != nil {
		result, out := backendFailure("norma.tasks.delete", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) setRun(ctx context.Context, _ *mcp.CallToolRequest, in setRunInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.set_run", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		result, out := validationFailure("norma.tasks.set_run", "run_id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.SetRun(ctx, id, runID); err != nil {
		result, out := backendFailure("norma.tasks.set_run", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) addDependency(ctx context.Context, _ *mcp.CallToolRequest, in addDependencyInput) (*mcp.CallToolResult, basicOutput, error) {
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		result, out := validationFailure("norma.tasks.add_dependency", "task_id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	dependsOnID := strings.TrimSpace(in.DependsOnID)
	if dependsOnID == "" {
		result, out := validationFailure("norma.tasks.add_dependency", "depends_on_id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.AddDependency(ctx, taskID, dependsOnID); err != nil {
		result, out := backendFailure("norma.tasks.add_dependency", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) leafTasks(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, listTasksOutput, error) {
	items, err := s.tracker.LeafTasks(ctx)
	if err != nil {
		result, out := backendFailure("norma.tasks.leaf", err)
		return result, listTasksOutput{ToolOutcome: out}, nil
	}
	return nil, listTasksOutput{ToolOutcome: okOutcome(), Tasks: mapTasks(items)}, nil
}

func (s *service) updateWorkflowState(ctx context.Context, _ *mcp.CallToolRequest, in workflowStateInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.update_workflow_state", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	state := strings.TrimSpace(in.State)
	if state == "" {
		result, out := validationFailure("norma.tasks.update_workflow_state", "state is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.UpdateWorkflowState(ctx, id, state); err != nil {
		result, out := backendFailure("norma.tasks.update_workflow_state", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) addLabel(ctx context.Context, _ *mcp.CallToolRequest, in labelInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.add_label", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		result, out := validationFailure("norma.tasks.add_label", "label is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.AddLabel(ctx, id, label); err != nil {
		result, out := backendFailure("norma.tasks.add_label", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) removeLabel(ctx context.Context, _ *mcp.CallToolRequest, in labelInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.remove_label", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		result, out := validationFailure("norma.tasks.remove_label", "label is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.RemoveLabel(ctx, id, label); err != nil {
		result, out := backendFailure("norma.tasks.remove_label", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) setNotes(ctx context.Context, _ *mcp.CallToolRequest, in setNotesInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.set_notes", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.SetNotes(ctx, id, in.Notes); err != nil {
		result, out := backendFailure("norma.tasks.set_notes", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) closeWithReason(ctx context.Context, _ *mcp.CallToolRequest, in closeWithReasonInput) (*mcp.CallToolResult, basicOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.close_with_reason", "id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		result, out := validationFailure("norma.tasks.close_with_reason", "reason is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.CloseWithReason(ctx, id, reason); err != nil {
		result, out := backendFailure("norma.tasks.close_with_reason", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) addRelatedLink(ctx context.Context, _ *mcp.CallToolRequest, in addRelatedLinkInput) (*mcp.CallToolResult, basicOutput, error) {
	from := strings.TrimSpace(in.FromID)
	if from == "" {
		result, out := validationFailure("norma.tasks.add_related_link", "from_id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}
	to := strings.TrimSpace(in.ToID)
	if to == "" {
		result, out := validationFailure("norma.tasks.add_related_link", "to_id is required")
		return result, basicOutput{ToolOutcome: out}, nil
	}

	if err := s.tracker.AddRelatedLink(ctx, from, to); err != nil {
		result, out := backendFailure("norma.tasks.add_related_link", err)
		return result, basicOutput{ToolOutcome: out}, nil
	}
	return nil, basicOutput{ToolOutcome: okOutcome()}, nil
}

func (s *service) listBlockedDependents(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, listTasksOutput, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		result, out := validationFailure("norma.tasks.list_blocked_dependents", "id is required")
		return result, listTasksOutput{ToolOutcome: out}, nil
	}

	items, err := s.tracker.ListBlockedDependents(ctx, id)
	if err != nil {
		result, out := backendFailure("norma.tasks.list_blocked_dependents", err)
		return result, listTasksOutput{ToolOutcome: out}, nil
	}
	return nil, listTasksOutput{ToolOutcome: okOutcome(), Tasks: mapTasks(items)}, nil
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func toCriteria(in []taskCriterionInput) []task.AcceptanceCriterion {
	if len(in) == 0 {
		return nil
	}
	out := make([]task.AcceptanceCriterion, 0, len(in))
	for _, item := range in {
		out = append(out, task.AcceptanceCriterion{
			ID:          strings.TrimSpace(item.ID),
			Text:        strings.TrimSpace(item.Text),
			VerifyHints: item.VerifyHints,
		})
	}
	return out
}

func mapTasks(items []task.Task) []taskRecord {
	if len(items) == 0 {
		return []taskRecord{}
	}
	out := make([]taskRecord, 0, len(items))
	for _, item := range items {
		out = append(out, toTaskRecord(item))
	}
	return out
}

func toTaskRecord(item task.Task) taskRecord {
	criteria := make([]taskCriterion, 0, len(item.Criteria))
	for _, ac := range item.Criteria {
		criteria = append(criteria, taskCriterion{ID: ac.ID, Text: ac.Text, VerifyHints: ac.VerifyHints})
	}
	labels := append([]string{}, item.Labels...)
	return taskRecord{
		ID:        item.ID,
		Type:      item.Type,
		ParentID:  item.ParentID,
		Title:     item.Title,
		Goal:      item.Goal,
		Criteria:  criteria,
		Status:    item.Status,
		RunID:     item.RunID,
		Priority:  item.Priority,
		Assignee:  item.Assignee,
		Labels:    labels,
		Notes:     item.Notes,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func okOutcome() ToolOutcome {
	return ToolOutcome{OK: true}
}

func validationFailure(operation string, message string) (*mcp.CallToolResult, ToolOutcome) {
	return failure(operation, codeValidationError, message)
}

func backendFailure(operation string, err error) (*mcp.CallToolResult, ToolOutcome) {
	return failure(operation, codeBackendError, err.Error())
}

func failure(operation string, code string, message string) (*mcp.CallToolResult, ToolOutcome) {
	return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: message}},
		}, ToolOutcome{
			OK: false,
			Error: &ToolError{
				Operation: operation,
				Code:      code,
				Message:   message,
			},
		}
}
