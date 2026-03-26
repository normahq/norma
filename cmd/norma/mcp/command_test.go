package mcpcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/normahq/norma/internal/task"
)

func TestCommandRegistersTasks(t *testing.T) {
	cmd := Command()
	sub, _, err := cmd.Find([]string{"tasks"})
	if err != nil {
		t.Fatalf("Find(tasks) error = %v", err)
	}
	if sub == nil || sub.Name() != "tasks" {
		t.Fatalf("tasks subcommand = %v, want tasks", sub)
	}
}

func TestTasksCommandUsesRepoRootFlag(t *testing.T) {
	var gotRepoRoot string
	prevNewTracker := newTracker
	prevRunTasksServer := runTasksServer
	t.Cleanup(func() {
		newTracker = prevNewTracker
		runTasksServer = prevRunTasksServer
	})

	newTracker = func(repoRoot string) task.Tracker {
		gotRepoRoot = repoRoot
		return &task.BeadsTracker{}
	}
	runTasksServer = func(_ context.Context, _ task.Tracker) error {
		return nil
	}

	cmd := TasksCommand()
	repoRoot := filepath.Join(t.TempDir(), "repo")
	cmd.SetArgs([]string{"--repo-root", repoRoot})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	want := filepath.Clean(repoRoot)
	got := filepath.Clean(gotRepoRoot)
	if got != want {
		t.Fatalf("repo root = %q, want %q", got, want)
	}
}

func TestTasksCommandDefaultsRepoRootToCurrentDirectory(t *testing.T) {
	var gotRepoRoot string
	prevNewTracker := newTracker
	prevRunTasksServer := runTasksServer
	t.Cleanup(func() {
		newTracker = prevNewTracker
		runTasksServer = prevRunTasksServer
	})

	newTracker = func(repoRoot string) task.Tracker {
		gotRepoRoot = repoRoot
		return &task.BeadsTracker{}
	}
	runTasksServer = func(_ context.Context, _ task.Tracker) error {
		return nil
	}

	workingDir := t.TempDir()
	prevWD, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("filepath.Abs(.) error = %v", err)
	}
	if err := chdirForTest(workingDir); err != nil {
		t.Fatalf("chdir working dir: %v", err)
	}
	t.Cleanup(func() {
		_ = chdirForTest(prevWD)
	})

	cmd := TasksCommand()
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	want := filepath.Clean(workingDir)
	got := filepath.Clean(gotRepoRoot)
	if got != want {
		t.Fatalf("repo root = %q, want %q", got, want)
	}
}

func chdirForTest(path string) error {
	return os.Chdir(path)
}
