package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func MountWorktree(ctx context.Context, workingDir, workspaceDir, branchName, baseBranch string) (string, error) {
	// Ensure we prune any stale worktrees before adding a new one.
	_ = GitRunCmdErr(ctx, workingDir, "git", "worktree", "prune")

	// Check if we are in a git repo
	if !Available(ctx, workingDir) {
		return "", fmt.Errorf("not a git repository: %s", workingDir)
	}

	// Check if branch already exists
	branchExists := strings.TrimSpace(GitRunCmd(ctx, workingDir, "git", "branch", "--list", branchName)) != ""

	if branchExists {
		// Ensure it's not checked out in another worktree
		ForceCleanupStaleWorktree(ctx, workingDir, branchName)
	}

	args := []string{"worktree", "add", "-b", branchName, workspaceDir}
	if branchExists {
		args = []string{"worktree", "add", workspaceDir, branchName}
	} else if baseBranch != "" {
		args = append(args, baseBranch)
	}

	// Create worktree
	err := GitRunCmdErr(ctx, workingDir, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git worktree add: %w", err)
	}

	// Sync branch with base: merge base into existing branch to pick up new changes
	if baseBranch != "" && branchName != baseBranch {
		if err := GitRunCmdErr(ctx, workspaceDir, "git", "merge", "--no-edit", baseBranch); err != nil {
			_ = GitRunCmdErr(ctx, workingDir, "git", "worktree", "remove", "--force", workspaceDir)
			return "", fmt.Errorf("git merge %s into %s: %w", baseBranch, branchName, err)
		}
	}

	return workspaceDir, nil
}

func ForceCleanupStaleWorktree(ctx context.Context, workingDir, branchName string) {
	out := GitRunCmd(ctx, workingDir, "git", "worktree", "list", "--porcelain")
	lines := strings.Split(out, "\n")
	var currentWorktree string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			if branch == branchName {
				log.Warn().Str("branch", branchName).Str("stale_worktree", currentWorktree).Msg("found stale worktree, forcing removal")
				// Try to remove the worktree
				_ = GitRunCmdErr(ctx, workingDir, "git", "worktree", "remove", "--force", currentWorktree)
			}
		}
	}
}

func RemoveWorktree(ctx context.Context, workingDir, workspaceDir string) error {
	// Remove worktree only, keep the branch for restartable progress
	err := GitRunCmdErr(ctx, workingDir, "git", "worktree", "remove", "--force", workspaceDir)
	if err != nil {
		log.Warn().Err(err).Str("workspace_dir", workspaceDir).Msg("failed to remove git worktree")
	}

	return err
}
