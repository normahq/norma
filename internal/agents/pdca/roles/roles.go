package roles

import (
	"encoding/json"
	"fmt"

	"github.com/metalagman/norma/internal/agents/pdca/contracts"
	"github.com/metalagman/norma/internal/agents/pdca/roles/act"
	"github.com/metalagman/norma/internal/agents/pdca/roles/check"
	"github.com/metalagman/norma/internal/agents/pdca/roles/do"
	"github.com/metalagman/norma/internal/agents/pdca/roles/plan"
)

const (
	rolePlan  = "plan"
	roleDo    = "do"
	roleCheck = "check"
	roleAct   = "act"
)

// DefaultRoles returns the built-in PDCA role implementations keyed by role name.
func DefaultRoles() map[string]contracts.Role {
	return map[string]contracts.Role{
		rolePlan:  &planRole{baseRole: *newBaseRole(rolePlan, plan.InputSchema, plan.OutputSchema, plan.PromptTemplate)},
		roleDo:    &doRole{baseRole: *newBaseRole(roleDo, do.InputSchema, do.OutputSchema, do.PromptTemplate)},
		roleCheck: &checkRole{baseRole: *newBaseRole(roleCheck, check.InputSchema, check.OutputSchema, check.PromptTemplate)},
		roleAct:   &actRole{baseRole: *newBaseRole(roleAct, act.InputSchema, act.OutputSchema, act.PromptTemplate)},
	}
}

type planRole struct {
	baseRole
}

//nolint:dupl // Typed generated requests require repeated field mapping.
func (r *planRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	acs := make([]plan.PlanAcceptanceCriteria, 0, len(contractReq.Task.AcceptanceCriteria))
	for _, ac := range contractReq.Task.AcceptanceCriteria {
		hints := ac.VerifyHints
		if hints == nil {
			hints = []string{}
		}
		acs = append(acs, plan.PlanAcceptanceCriteria{
			Id:          ac.ID,
			Text:        ac.Text,
			VerifyHints: hints,
		})
	}
	links := contractReq.Context.Links
	if links == nil {
		links = []string{}
	}
	return &plan.PlanRequest{
		Run:   &plan.PlanRun{Id: contractReq.Run.ID, Iteration: int64(contractReq.Run.Iteration)},
		Task:  &plan.PlanTask{Id: contractReq.Task.ID, Title: contractReq.Task.Title, Description: contractReq.Task.Description, AcceptanceCriteria: acs},
		Step:  &plan.PlanStep{Index: int64(contractReq.Step.Index), Name: contractReq.Step.Name},
		Paths: &plan.PlanPaths{WorkspaceDir: contractReq.Paths.WorkspaceDir, RunDir: contractReq.Paths.RunDir},
		Budgets: &plan.PlanBudgets{
			MaxIterations:      int64(contractReq.Budgets.MaxIterations),
			MaxWallTimeMinutes: int64(contractReq.Budgets.MaxWallTimeMinutes),
			MaxFailedChecks:    int64(contractReq.Budgets.MaxFailedChecks),
		},
		Context: &plan.PlanContext{
			Attempt: int64(contractReq.Context.Attempt),
			Links:   links,
		},
		StopReasonsAllowed: contractReq.StopReasonsAllowed,
		PlanInput:          contractReq.Plan,
	}, nil
}

func (r *planRole) MapResponse(outBytes []byte) (contracts.RawAgentResponse, error) {
	var roleResp plan.PlanResponse
	if err := json.Unmarshal(outBytes, &roleResp); err != nil {
		return contracts.RawAgentResponse{}, err
	}
	res := contracts.RawAgentResponse{
		Status:     roleResp.Status,
		StopReason: roleResp.StopReason,
	}
	if roleResp.Summary != nil {
		res.Summary = contracts.ResponseSummary{Text: roleResp.Summary.Text}
	}
	if roleResp.Progress != nil {
		res.Progress = contracts.StepProgress{Title: roleResp.Progress.Title, Details: roleResp.Progress.Details}
	}
	if roleResp.PlanOutput != nil {
		if planBytes, err := json.Marshal(roleResp.PlanOutput); err == nil {
			res.PlanOutput = planBytes
		}
	}
	return res, nil
}

type doRole struct {
	baseRole
}

func (r *doRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	acs := make([]do.DoAcceptanceCriteria, 0, len(contractReq.Task.AcceptanceCriteria))
	for _, ac := range contractReq.Task.AcceptanceCriteria {
		hints := ac.VerifyHints
		if hints == nil {
			hints = []string{}
		}
		acs = append(acs, do.DoAcceptanceCriteria{
			Id:          ac.ID,
			Text:        ac.Text,
			VerifyHints: hints,
		})
	}

	links := contractReq.Context.Links
	if links == nil {
		links = []string{}
	}

	doInput := normalizeDoInput(contractReq.Do)

	return &do.DoRequest{
		Run:   &do.DoRun{Id: contractReq.Run.ID, Iteration: int64(contractReq.Run.Iteration)},
		Task:  &do.DoTask{Id: contractReq.Task.ID, Title: contractReq.Task.Title, Description: contractReq.Task.Description, AcceptanceCriteria: acs},
		Step:  &do.DoStep{Index: int64(contractReq.Step.Index), Name: contractReq.Step.Name},
		Paths: &do.DoPaths{WorkspaceDir: contractReq.Paths.WorkspaceDir, RunDir: contractReq.Paths.RunDir},
		Budgets: &do.DoBudgets{
			MaxIterations:      int64(contractReq.Budgets.MaxIterations),
			MaxWallTimeMinutes: int64(contractReq.Budgets.MaxWallTimeMinutes),
			MaxFailedChecks:    int64(contractReq.Budgets.MaxFailedChecks),
		},
		Context: &do.DoContext{
			Attempt: int64(contractReq.Context.Attempt),
			Links:   links,
		},
		StopReasonsAllowed: contractReq.StopReasonsAllowed,
		DoInput:            doInput,
	}, nil
}

func (r *doRole) MapResponse(outBytes []byte) (contracts.RawAgentResponse, error) {
	var roleResp do.DoResponse
	if err := json.Unmarshal(outBytes, &roleResp); err != nil {
		return contracts.RawAgentResponse{}, err
	}
	res := contracts.RawAgentResponse{
		Status:     roleResp.Status,
		StopReason: roleResp.StopReason,
	}
	if roleResp.Summary != nil {
		res.Summary = contracts.ResponseSummary{Text: roleResp.Summary.Text}
	}
	if roleResp.Progress != nil {
		res.Progress = contracts.StepProgress{Title: roleResp.Progress.Title, Details: roleResp.Progress.Details}
	}
	if roleResp.DoOutput != nil {
		if doBytes, err := json.Marshal(roleResp.DoOutput); err == nil {
			res.DoOutput = doBytes
		}
	}
	return res, nil
}

type checkRole struct {
	baseRole
}

//nolint:dupl // Typed generated requests require repeated field mapping.
func (r *checkRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	acs := make([]check.CheckAcceptanceCriteria, 0, len(contractReq.Task.AcceptanceCriteria))
	for _, ac := range contractReq.Task.AcceptanceCriteria {
		acs = append(acs, check.CheckAcceptanceCriteria{
			Id:   ac.ID,
			Text: ac.Text,
		})
	}

	links := contractReq.Context.Links
	if links == nil {
		links = []string{}
	}

	return &check.CheckRequest{
		Run:   &check.CheckRun{Id: contractReq.Run.ID, Iteration: int64(contractReq.Run.Iteration)},
		Task:  &check.CheckTask{Id: contractReq.Task.ID, Title: contractReq.Task.Title, Description: contractReq.Task.Description, AcceptanceCriteria: acs},
		Step:  &check.CheckStep{Index: int64(contractReq.Step.Index), Name: contractReq.Step.Name},
		Paths: &check.CheckPaths{WorkspaceDir: contractReq.Paths.WorkspaceDir, RunDir: contractReq.Paths.RunDir},
		Budgets: &check.CheckBudgets{
			MaxIterations:      int64(contractReq.Budgets.MaxIterations),
			MaxWallTimeMinutes: int64(contractReq.Budgets.MaxWallTimeMinutes),
			MaxFailedChecks:    int64(contractReq.Budgets.MaxFailedChecks),
		},
		Context: &check.CheckContext{
			Attempt: int64(contractReq.Context.Attempt),
			Links:   links,
		},
		StopReasonsAllowed: contractReq.StopReasonsAllowed,
		CheckInput:         contractReq.Check,
	}, nil
}

func (r *checkRole) MapResponse(outBytes []byte) (contracts.RawAgentResponse, error) {
	var roleResp check.CheckResponse
	if err := json.Unmarshal(outBytes, &roleResp); err != nil {
		return contracts.RawAgentResponse{}, err
	}
	res := contracts.RawAgentResponse{
		Status:     roleResp.Status,
		StopReason: roleResp.StopReason,
	}
	if roleResp.Summary != nil {
		res.Summary = contracts.ResponseSummary{Text: roleResp.Summary.Text}
	}
	if roleResp.Progress != nil {
		res.Progress = contracts.StepProgress{Title: roleResp.Progress.Title, Details: roleResp.Progress.Details}
	}
	if roleResp.CheckOutput != nil {
		if checkBytes, err := json.Marshal(roleResp.CheckOutput); err == nil {
			res.CheckOutput = checkBytes
		}
	}
	return res, nil
}

type actRole struct {
	baseRole
}

//nolint:dupl // Typed generated requests require repeated field mapping.
func (r *actRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	acs := make([]any, 0, len(contractReq.Task.AcceptanceCriteria))
	for _, ac := range contractReq.Task.AcceptanceCriteria {
		acs = append(acs, ac)
	}

	links := contractReq.Context.Links
	if links == nil {
		links = []string{}
	}

	return &act.ActRequest{
		Run:   &act.ActRun{Id: contractReq.Run.ID, Iteration: int64(contractReq.Run.Iteration)},
		Task:  &act.ActTask{Id: contractReq.Task.ID, Title: contractReq.Task.Title, Description: contractReq.Task.Description, AcceptanceCriteria: acs},
		Step:  &act.ActStep{Index: int64(contractReq.Step.Index), Name: contractReq.Step.Name},
		Paths: &act.ActPaths{WorkspaceDir: contractReq.Paths.WorkspaceDir, RunDir: contractReq.Paths.RunDir},
		Budgets: &act.ActBudgets{
			MaxIterations:      int64(contractReq.Budgets.MaxIterations),
			MaxWallTimeMinutes: int64(contractReq.Budgets.MaxWallTimeMinutes),
			MaxFailedChecks:    int64(contractReq.Budgets.MaxFailedChecks),
		},
		Context: &act.ActContext{
			Attempt: int64(contractReq.Context.Attempt),
			Links:   links,
		},
		StopReasonsAllowed: contractReq.StopReasonsAllowed,
		ActInput:           contractReq.Act,
	}, nil
}

func (r *actRole) MapResponse(outBytes []byte) (contracts.RawAgentResponse, error) {
	var roleResp act.ActResponse
	if err := json.Unmarshal(outBytes, &roleResp); err != nil {
		return contracts.RawAgentResponse{}, err
	}
	res := contracts.RawAgentResponse{
		Status:     roleResp.Status,
		StopReason: roleResp.StopReason,
	}
	if roleResp.Summary != nil {
		res.Summary = contracts.ResponseSummary{Text: roleResp.Summary.Text}
	}
	if roleResp.Progress != nil {
		res.Progress = contracts.StepProgress{Title: roleResp.Progress.Title, Details: roleResp.Progress.Details}
	}
	if roleResp.ActOutput != nil {
		if actBytes, err := json.Marshal(roleResp.ActOutput); err == nil {
			res.ActOutput = actBytes
		}
	}
	return res, nil
}

func normalizeDoInput(input *do.DoInput) *do.DoInput {
	if input == nil {
		return nil
	}

	out := &do.DoInput{
		AcceptanceCriteriaEffective: make([]do.DoEffectiveAcceptanceCriteria, 0, len(input.AcceptanceCriteriaEffective)),
	}

	if input.WorkPlan != nil {
		doSteps := make([]do.DoDoStep, 0, len(input.WorkPlan.DoSteps))
		for _, step := range input.WorkPlan.DoSteps {
			targets := step.TargetsAcIds
			if targets == nil {
				targets = []string{}
			}
			doSteps = append(doSteps, do.DoDoStep{
				Id:           step.Id,
				TargetsAcIds: targets,
				Text:         step.Text,
			})
		}

		checkSteps := make([]do.DoCheckStep, 0, len(input.WorkPlan.CheckSteps))
		checkSteps = append(checkSteps, input.WorkPlan.CheckSteps...)

		stopTriggers := input.WorkPlan.StopTriggers
		if stopTriggers == nil {
			stopTriggers = []string{}
		}

		out.WorkPlan = &do.DoWorkPlan{
			TimeboxMinutes: input.WorkPlan.TimeboxMinutes,
			DoSteps:        doSteps,
			CheckSteps:     checkSteps,
			StopTriggers:   stopTriggers,
		}
	}

	for _, ac := range input.AcceptanceCriteriaEffective {
		refines := ac.Refines
		if refines == nil {
			refines = []string{}
		}
		checks := make([]do.DoAcceptanceCriteriaCheck, 0, len(ac.Checks))
		checks = append(checks, ac.Checks...)

		out.AcceptanceCriteriaEffective = append(out.AcceptanceCriteriaEffective, do.DoEffectiveAcceptanceCriteria{
			Id:      ac.Id,
			Origin:  ac.Origin,
			Refines: refines,
			Text:    ac.Text,
			Checks:  checks,
			Reason:  ac.Reason,
		})
	}

	return out
}
