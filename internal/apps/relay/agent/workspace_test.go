package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceImportDiscardsDirtyChangesAndSyncsToMaster(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workingDir := t.TempDir()
	initGitRepo(t, ctx, workingDir)

	writeFile(t, filepath.Join(workingDir, "base.txt"), "base\n")
	runGit(t, ctx, workingDir, "add", "base.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: seed")

	workspaceDir := filepath.Join(t.TempDir(), "relay-workspace")
	branchName := "norma/relay/relay-1-0"
	runGit(t, ctx, workingDir, "worktree", "add", "-b", branchName, workspaceDir, "HEAD")
	t.Cleanup(func() {
		_ = runGitAllowError(ctx, workingDir, "worktree", "remove", "--force", workspaceDir)
	})

	writeFile(t, filepath.Join(workspaceDir, "base.txt"), "dirty change\n")
	writeFile(t, filepath.Join(workspaceDir, "scratch.txt"), "scratch\n")

	writeFile(t, filepath.Join(workingDir, "master-only.txt"), "master-only\n")
	runGit(t, ctx, workingDir, "add", "master-only.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: update master")

	m := NewWorkspaceManager(workingDir)
	if err := m.Import(ctx, workspaceDir); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	status := runGit(t, ctx, workspaceDir, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean workspace after import, got:\n%s", status)
	}

	if _, err := os.Stat(filepath.Join(workspaceDir, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected scratch.txt to be removed, stat err=%v", err)
	}

	if got := readFile(t, filepath.Join(workspaceDir, "base.txt")); got != "base\n" {
		t.Fatalf("base.txt mismatch: got %q", got)
	}
	if got := readFile(t, filepath.Join(workspaceDir, "master-only.txt")); got != "master-only\n" {
		t.Fatalf("master-only.txt mismatch: got %q", got)
	}
}

func TestWorkspaceImportRebasesCleanBranch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workingDir := t.TempDir()
	initGitRepo(t, ctx, workingDir)

	writeFile(t, filepath.Join(workingDir, "base.txt"), "base\n")
	runGit(t, ctx, workingDir, "add", "base.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: seed")

	workspaceDir := filepath.Join(t.TempDir(), "relay-workspace")
	branchName := "norma/relay/relay-1-1"
	runGit(t, ctx, workingDir, "worktree", "add", "-b", branchName, workspaceDir, "HEAD")
	t.Cleanup(func() {
		_ = runGitAllowError(ctx, workingDir, "worktree", "remove", "--force", workspaceDir)
	})

	writeFile(t, filepath.Join(workspaceDir, "branch.txt"), "branch\n")
	runGit(t, ctx, workspaceDir, "add", "branch.txt")
	runGit(t, ctx, workspaceDir, "commit", "-m", "feat: branch change")

	writeFile(t, filepath.Join(workingDir, "master.txt"), "master\n")
	runGit(t, ctx, workingDir, "add", "master.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: master change")

	m := NewWorkspaceManager(workingDir)
	if err := m.Import(ctx, workspaceDir); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	status := runGit(t, ctx, workspaceDir, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean workspace after import, got:\n%s", status)
	}

	if got := readFile(t, filepath.Join(workspaceDir, "branch.txt")); got != "branch\n" {
		t.Fatalf("branch.txt mismatch: got %q", got)
	}
	if got := readFile(t, filepath.Join(workspaceDir, "master.txt")); got != "master\n" {
		t.Fatalf("master.txt mismatch: got %q", got)
	}
}

func TestWorkspaceImportAbortsRebaseOnConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workingDir := t.TempDir()
	initGitRepo(t, ctx, workingDir)

	writeFile(t, filepath.Join(workingDir, "conflict.txt"), "base\n")
	runGit(t, ctx, workingDir, "add", "conflict.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: seed")

	workspaceDir := filepath.Join(t.TempDir(), "relay-workspace")
	branchName := "norma/relay/relay-1-2"
	runGit(t, ctx, workingDir, "worktree", "add", "-b", branchName, workspaceDir, "HEAD")
	t.Cleanup(func() {
		_ = runGitAllowError(ctx, workingDir, "worktree", "remove", "--force", workspaceDir)
	})

	writeFile(t, filepath.Join(workspaceDir, "conflict.txt"), "branch\n")
	runGit(t, ctx, workspaceDir, "add", "conflict.txt")
	runGit(t, ctx, workspaceDir, "commit", "-m", "feat: branch conflict")

	writeFile(t, filepath.Join(workingDir, "conflict.txt"), "master\n")
	runGit(t, ctx, workingDir, "add", "conflict.txt")
	runGit(t, ctx, workingDir, "commit", "-m", "chore: master conflict")

	m := NewWorkspaceManager(workingDir)
	err := m.Import(ctx, workspaceDir)
	if err == nil {
		t.Fatal("Import() error = nil, want conflict error")
	}
	if !strings.Contains(err.Error(), "rebase workspace onto master") {
		t.Fatalf("error = %q, want rebase context", err)
	}

	rebaseMergePath := strings.TrimSpace(runGit(t, ctx, workspaceDir, "rev-parse", "--git-path", "rebase-merge"))
	if _, statErr := os.Stat(rebaseMergePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no rebase-merge state after abort, stat err=%v", statErr)
	}

	rebaseApplyPath := strings.TrimSpace(runGit(t, ctx, workspaceDir, "rev-parse", "--git-path", "rebase-apply"))
	if _, statErr := os.Stat(rebaseApplyPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no rebase-apply state after abort, stat err=%v", statErr)
	}

	status := runGit(t, ctx, workspaceDir, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Fatalf("expected clean workspace after abort, got:\n%s", status)
	}
	if got := readFile(t, filepath.Join(workspaceDir, "conflict.txt")); got != "branch\n" {
		t.Fatalf("conflict.txt mismatch after abort: got %q", got)
	}
}

func initGitRepo(t *testing.T, ctx context.Context, workingDir string) {
	t.Helper()
	runGit(t, ctx, workingDir, "init")
	runGit(t, ctx, workingDir, "config", "user.name", "Norma Test")
	runGit(t, ctx, workingDir, "config", "user.email", "norma-test@example.com")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

func runGit(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runGitAllowError(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	_, err := cmd.CombinedOutput()
	return err
}
