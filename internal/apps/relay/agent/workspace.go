package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/metalagman/norma/internal/git"
	"github.com/rs/zerolog/log"
)

// WorkspaceManager manages git worktrees for relay sessions.
type WorkspaceManager struct {
	workingDir string
}

// NewWorkspaceManager creates a WorkspaceManager for the given repo root.
func NewWorkspaceManager(workingDir string) *WorkspaceManager {
	return &WorkspaceManager{workingDir: workingDir}
}

const baseBranch = "HEAD"

// EnsureWorkspace creates or returns an existing workspace directory.
// If existingPath is non-empty and the directory exists, it is reused and synced with base.
// Otherwise a new worktree is mounted at workspacesDir/<key> using branch <branchName>.
func (m *WorkspaceManager) EnsureWorkspace(ctx context.Context, key, branchName, existingPath string) (string, error) {
	relayDir := filepath.Join(m.workingDir, ".norma")
	workspacesDir := filepath.Join(relayDir, "relay-sessions")
	if err := os.MkdirAll(workspacesDir, 0o755); err != nil {
		return "", fmt.Errorf("create workspaces dir: %w", err)
	}

	workspaceDir := existingPath
	if strings.TrimSpace(workspaceDir) == "" {
		workspaceDir = filepath.Join(workspacesDir, key)
	}

	if fi, err := os.Stat(workspaceDir); err == nil && fi.IsDir() {
		// Workspace already exists — sync with base branch
		if err := m.syncWorkspace(ctx, workspaceDir, branchName); err != nil {
			return "", fmt.Errorf("sync workspace: %w", err)
		}
		return workspaceDir, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat workspace dir %q: %w", workspaceDir, err)
	}

	if _, err := git.MountWorktree(ctx, m.workingDir, workspaceDir, branchName, baseBranch); err != nil {
		return "", fmt.Errorf("mount worktree: %w", err)
	}

	return workspaceDir, nil
}

// syncWorkspace fetches base branch updates and merges them into the workspace branch.
func (m *WorkspaceManager) syncWorkspace(ctx context.Context, workspaceDir, branchName string) error {
	if err := git.GitRunCmdErr(ctx, workspaceDir, "git", "fetch", "origin"); err != nil {
		log.Warn().Err(err).Str("workspace", workspaceDir).Msg("git fetch failed, proceeding without sync")
		return nil
	}

	baseRef := fmt.Sprintf("origin/%s", "master")
	if err := git.GitRunCmdErr(ctx, workspaceDir, "git", "merge", "--no-edit", baseRef); err != nil {
		log.Warn().Err(err).Str("base", baseRef).Msg("git merge failed, workspace may have conflicts")
		return nil
	}

	log.Info().Str("workspace", workspaceDir).Str("base", baseRef).Msg("workspace synced with base")
	return nil
}

// CleanupWorkspace removes a git worktree.
func (m *WorkspaceManager) CleanupWorkspace(ctx context.Context, workspaceDir string) error {
	if workspaceDir == "" {
		return nil
	}
	if err := git.RemoveWorktree(ctx, m.workingDir, workspaceDir); err != nil {
		log.Warn().Err(err).Str("workspace", workspaceDir).Msg("failed to remove worktree")
		return err
	}
	return nil
}
