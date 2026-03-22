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
	runTasksServer     = tasksmcp.Run
	runTasksServerHTTP = tasksmcp.RunHTTP
	newTracker         = func(repoRoot string) task.Tracker {
		tracker := task.NewBeadsTracker("")
		tracker.WorkingDir = repoRoot
		return tracker
	}
)

// TasksCommand runs the tracker-parity tasks MCP server.
// By default it runs over stdio; use --http to run over HTTP.
func TasksCommand() *cobra.Command {
	var (
		repoRoot string
		httpMode bool
		httpAddr string
	)

	cmd := &cobra.Command{
		Use:          "tasks",
		Short:        "Run norma task tracker MCP server",
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

			tracker := newTracker(absoluteRepoRoot)

			if httpMode {
				addr := strings.TrimSpace(httpAddr)
				if addr == "" {
					addr = "localhost:8080"
				}
				return runTasksServerHTTP(cmd.Context(), tracker, addr)
			}

			return runTasksServer(cmd.Context(), tracker)
		},
	}

	cmd.Flags().StringVar(&repoRoot, "repo-root", "", "Repository root for task context resolution (default: current directory)")
	cmd.Flags().BoolVar(&httpMode, "http", false, "Run over HTTP instead of stdio")
	cmd.Flags().StringVar(&httpAddr, "addr", "localhost:8080", "HTTP listen address (host:port)")
	return cmd
}
