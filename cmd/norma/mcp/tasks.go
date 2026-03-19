package mcpcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/metalagman/norma/internal/apps/tasksmcp"
	"github.com/metalagman/norma/internal/task"
	"github.com/spf13/cobra"
)

var (
	runTasksServer = tasksmcp.Run
	newTracker     = func(repoRoot string) task.Tracker {
		tracker := task.NewBeadsTracker("")
		tracker.WorkingDir = repoRoot
		return tracker
	}
)

// TasksCommand runs the tracker-parity tasks MCP server over stdio.
func TasksCommand() *cobra.Command {
	var repoRoot string

	cmd := &cobra.Command{
		Use:          "tasks",
		Short:        "Run norma task tracker MCP server over stdio",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedRepoRoot := strings.TrimSpace(repoRoot)
			if resolvedRepoRoot == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve current working directory: %w", err)
				}
				resolvedRepoRoot = cwd
			}

			absoluteRepoRoot, err := filepath.Abs(resolvedRepoRoot)
			if err != nil {
				return fmt.Errorf("resolve absolute repo root %q: %w", resolvedRepoRoot, err)
			}

			return runTasksServer(cmd.Context(), newTracker(absoluteRepoRoot))
		},
	}

	cmd.Flags().StringVar(&repoRoot, "repo-root", "", "Repository root for task context resolution (default: current directory)")
	return cmd
}
