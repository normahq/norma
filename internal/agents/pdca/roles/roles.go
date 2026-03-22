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

	// Plan reads task_id from TaskState.Plan if resuming, otherwise from Task.ID
	taskID := contractReq.Task.ID
	planInput := &plan.PlanInput{Task: &plan.PlanTaskID{Id: taskID}}

	//nolint:dupl // Each role has structurally similar request building, but must return its own typed struct
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
		PlanInput:          planInput,
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

	// Do reads plan from TaskState
	var doInput *do.DoInput
	if len(contractReq.TaskState.Plan) > 0 {
		var planOutput plan.PlanOutput
		if err := json.Unmarshal(contractReq.TaskState.Plan, &planOutput); err != nil {
			return nil, fmt.Errorf("unmarshal plan from task state: %w", err)
		}
		doInput = planOutputToDoInput(&planOutput)
	} else {
		return nil, fmt.Errorf("missing plan in task state for do step")
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

	//nolint:dupl // Each role has structurally similar request building, but must return its own typed struct
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

func (r *checkRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	// Check reads plan and do from TaskState
	var checkInput *check.CheckInput
	if len(contractReq.TaskState.Plan) > 0 && len(contractReq.TaskState.Do) > 0 {
		var planOutput plan.PlanOutput
		if err := json.Unmarshal(contractReq.TaskState.Plan, &planOutput); err != nil {
			return nil, fmt.Errorf("unmarshal plan from task state: %w", err)
		}
		var doOutput do.DoOutput
		if err := json.Unmarshal(contractReq.TaskState.Do, &doOutput); err != nil {
			return nil, fmt.Errorf("unmarshal do from task state: %w", err)
		}
		checkInput = planAndDoToCheckInput(&planOutput, &doOutput)
	} else {
		return nil, fmt.Errorf("missing plan or do in task state for check step")
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

	//nolint:dupl // Each role has structurally similar request building, but must return its own typed struct
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
		CheckInput:         checkInput,
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

func (r *actRole) MapRequest(req contracts.RawAgentRequest) (any, error) {
	var contractReq contracts.AgentRequest
	if err := json.Unmarshal(req, &contractReq); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	// Act reads check from TaskState
	var actInput *act.ActInput
	if len(contractReq.TaskState.Check) > 0 {
		var checkOutput check.CheckOutput
		if err := json.Unmarshal(contractReq.TaskState.Check, &checkOutput); err != nil {
			return nil, fmt.Errorf("unmarshal check from task state: %w", err)
		}
		actInput = checkOutputToActInput(&checkOutput)
	} else {
		return nil, fmt.Errorf("missing check in task state for act step")
	}

	acs := make([]any, 0, len(contractReq.Task.AcceptanceCriteria))
	for _, ac := range contractReq.Task.AcceptanceCriteria {
		acs = append(acs, ac)
	}

	links := contractReq.Context.Links
	if links == nil {
		links = []string{}
	}

	//nolint:dupl // Each role has structurally similar request building, but must return its own typed struct
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
		ActInput:           actInput,
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

// Type conversion helpers - each role converts from its own types when reading TaskState.

func planOutputToDoInput(p *plan.PlanOutput) *do.DoInput {
	if p == nil {
		return nil
	}

	doInput := &do.DoInput{
		AcceptanceCriteriaEffective: make([]do.DoEffectiveAcceptanceCriteria, 0),
	}

	if p.WorkPlan != nil {
		doSteps := make([]do.DoDoStep, 0, len(p.WorkPlan.DoSteps))
		for _, step := range p.WorkPlan.DoSteps {
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

		checkSteps := make([]do.DoCheckStep, 0, len(p.WorkPlan.CheckSteps))
		for _, step := range p.WorkPlan.CheckSteps {
			checkSteps = append(checkSteps, do.DoCheckStep{
				Id:   step.Id,
				Mode: step.Mode,
				Text: step.Text,
			})
		}

		stopTriggers := p.WorkPlan.StopTriggers
		if stopTriggers == nil {
			stopTriggers = []string{}
		}

		doInput.WorkPlan = &do.DoWorkPlan{
			TimeboxMinutes: p.WorkPlan.TimeboxMinutes,
			DoSteps:        doSteps,
			CheckSteps:     checkSteps,
			StopTriggers:   stopTriggers,
		}
	}

	if p.AcceptanceCriteria != nil {
		for _, ac := range p.AcceptanceCriteria.Effective {
			refines := ac.Refines
			if refines == nil {
				refines = []string{}
			}
			checks := make([]do.DoAcceptanceCriteriaCheck, 0, len(ac.Checks))
			for _, c := range ac.Checks {
				checks = append(checks, do.DoAcceptanceCriteriaCheck{
					Id:              c.Id,
					Cmd:             c.Cmd,
					ExpectExitCodes: c.ExpectExitCodes,
				})
			}
			doInput.AcceptanceCriteriaEffective = append(doInput.AcceptanceCriteriaEffective, do.DoEffectiveAcceptanceCriteria{
				Id:      ac.Id,
				Origin:  ac.Origin,
				Refines: refines,
				Text:    ac.Text,
				Checks:  checks,
				Reason:  ac.Reason,
			})
		}
	}

	return doInput
}

func planAndDoToCheckInput(p *plan.PlanOutput, d *do.DoOutput) *check.CheckInput {
	input := &check.CheckInput{}

	if p != nil && p.WorkPlan != nil {
		doSteps := make([]check.CheckDoStep, 0, len(p.WorkPlan.DoSteps))
		for _, step := range p.WorkPlan.DoSteps {
			doSteps = append(doSteps, check.CheckDoStep{
				Id:   step.Id,
				Text: step.Text,
			})
		}

		checkSteps := make([]check.CheckCheckStep, 0, len(p.WorkPlan.CheckSteps))
		for _, step := range p.WorkPlan.CheckSteps {
			checkSteps = append(checkSteps, check.CheckCheckStep{
				Id:   step.Id,
				Mode: step.Mode,
				Text: step.Text,
			})
		}

		input.WorkPlan = &check.CheckWorkPlan{
			TimeboxMinutes: p.WorkPlan.TimeboxMinutes,
			DoSteps:        doSteps,
			CheckSteps:     checkSteps,
			StopTriggers:   p.WorkPlan.StopTriggers,
		}
	}

	if p != nil && p.AcceptanceCriteria != nil {
		effective := make([]check.CheckEffectiveAcceptanceCriteria, 0, len(p.AcceptanceCriteria.Effective))
		for _, ac := range p.AcceptanceCriteria.Effective {
			effective = append(effective, check.CheckEffectiveAcceptanceCriteria{
				Id:     ac.Id,
				Origin: ac.Origin,
				Text:   ac.Text,
			})
		}
		input.AcceptanceCriteriaEffective = effective
	}

	if d != nil && d.Execution != nil {
		input.DoExecution = &check.CheckDoExecution{
			ExecutedStepIds: d.Execution.ExecutedStepIds,
			SkippedStepIds:  d.Execution.SkippedStepIds,
		}
	}

	return input
}

func checkOutputToActInput(c *check.CheckOutput) *act.ActInput {
	if c == nil {
		return nil
	}

	input := &act.ActInput{}

	if c.Verdict != nil {
		input.CheckVerdict = &act.ActCheckVerdict{
			Status:         c.Verdict.Status,
			Recommendation: c.Verdict.Recommendation,
		}
		if c.Verdict.Basis != nil {
			input.CheckVerdict.Basis = &act.ActCheckVerdictBasis{
				PlanMatch:           c.Verdict.Basis.PlanMatch,
				AllAcceptancePassed: c.Verdict.Basis.AllAcceptancePassed,
			}
		}
	}

	if c.AcceptanceResults != nil {
		input.AcceptanceResults = make([]act.ActAcceptanceResult, 0, len(c.AcceptanceResults))
		for _, ar := range c.AcceptanceResults {
			input.AcceptanceResults = append(input.AcceptanceResults, act.ActAcceptanceResult{
				AcId:   ar.AcId,
				Result: ar.Result,
				Notes:  ar.Notes,
			})
		}
	}

	return input
}
