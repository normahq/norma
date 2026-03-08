package planner

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestPlannerACPPermissionHandler_RejectsMutatingKinds(t *testing.T) {
	t.Parallel()

	kind := acp.ToolKindEdit
	resp, err := PlannerACPPermissionHandler(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.RequestPermissionToolCall{Kind: &kind},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
			{Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"},
		},
	})
	if err != nil {
		t.Fatalf("PlannerACPPermissionHandler() error = %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != "reject" {
		t.Fatalf("selected option = %+v, want reject", resp.Outcome.Selected)
	}
}

func TestPlannerACPPermissionHandler_AllowsExecuteForPlanning(t *testing.T) {
	t.Parallel()

	kind := acp.ToolKindExecute
	resp, err := PlannerACPPermissionHandler(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.RequestPermissionToolCall{Kind: &kind},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
			{Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"},
		},
	})
	if err != nil {
		t.Fatalf("PlannerACPPermissionHandler() error = %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != "allow" {
		t.Fatalf("selected option = %+v, want allow", resp.Outcome.Selected)
	}
}
