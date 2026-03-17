package task

import (
	"context"
	"testing"
)

func TestBeadsTrackerImplementsTracker(t *testing.T) {
	_ = Tracker(&BeadsTracker{})
}

func TestCloseWithReason(t *testing.T) {
	tracker := &BeadsTracker{BinPath: "bd"}
	ctx := context.Background()

	issueID, err := tracker.Add(ctx, "Test CloseWithReason", "Testing close with reason", nil, nil)
	if err != nil {
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, issueID) }()

	err = tracker.CloseWithReason(ctx, issueID, "Test completed successfully")
	if err != nil {
		t.Errorf("CloseWithReason failed: %v", err)
	}
}

func TestAddRelatedLink(t *testing.T) {
	tracker := &BeadsTracker{BinPath: "bd"}
	ctx := context.Background()

	issue1, err := tracker.Add(ctx, "Test Issue 1", "First test issue", nil, nil)
	if err != nil {
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, issue1) }()

	issue2, err := tracker.Add(ctx, "Test Issue 2", "Second test issue", nil, nil)
	if err != nil {
		_ = tracker.Delete(ctx, issue1)
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, issue2) }()

	err = tracker.AddRelatedLink(ctx, issue1, issue2)
	if err != nil {
		t.Errorf("AddRelatedLink failed: %v", err)
	}
}

func TestListBlockedDependents(t *testing.T) {
	tracker := &BeadsTracker{BinPath: "bd"}
	ctx := context.Background()

	blocker, err := tracker.Add(ctx, "Test Blocker", "This issue blocks another", nil, nil)
	if err != nil {
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, blocker) }()

	blocked, err := tracker.Add(ctx, "Test Blocked", "This issue depends on blocker", nil, nil)
	if err != nil {
		_ = tracker.Delete(ctx, blocker)
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, blocked) }()

	err = tracker.AddDependency(ctx, blocked, blocker)
	if err != nil {
		_ = tracker.Delete(ctx, blocker)
		_ = tracker.Delete(ctx, blocked)
		t.Skipf("bd not available or not configured: %v", err)
	}

	dependents, err := tracker.ListBlockedDependents(ctx, blocker)
	if err != nil {
		t.Errorf("ListBlockedDependents failed: %v", err)
	}

	found := false
	for _, dep := range dependents {
		if dep.ID == blocked {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find blocked issue in dependents, got: %v", dependents)
	}
}

func TestAddFollowUp(t *testing.T) {
	tracker := &BeadsTracker{BinPath: "bd"}
	ctx := context.Background()

	parent, err := tracker.Add(ctx, "Test Parent", "Parent issue for follow-up", nil, nil)
	if err != nil {
		t.Skipf("bd not available or not configured: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, parent) }()

	followUpID, err := tracker.AddFollowUp(ctx, parent, "Test FollowUp", "Follow-up task created", nil)
	if err != nil {
		t.Errorf("AddFollowUp failed: %v", err)
	}
	defer func() { _ = tracker.Delete(ctx, followUpID) }()

	if followUpID == "" {
		t.Error("AddFollowUp returned empty ID")
	}
}
