package pdca

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/metalagman/norma/internal/adk/agentconfig"
)

const tasksMCPServerName = "norma_tasks"

var resolveExecutablePath = os.Executable

func roleMCPServers(roleName, repoRoot string) (map[string]agentconfig.MCPServerConfig, error) {
	if strings.TrimSpace(roleName) != RolePlan {
		return nil, nil
	}

	trimmedRepoRoot := strings.TrimSpace(repoRoot)
	if trimmedRepoRoot == "" {
		return nil, fmt.Errorf("repo root is required for %q role MCP tasks server", RolePlan)
	}

	absoluteRepoRoot, err := filepath.Abs(trimmedRepoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root path %q: %w", trimmedRepoRoot, err)
	}

	executablePath, err := resolveExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("resolve norma executable path: %w", err)
	}
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" {
		return nil, fmt.Errorf("resolved empty norma executable path")
	}

	return map[string]agentconfig.MCPServerConfig{
		tasksMCPServerName: {
			Type: agentconfig.MCPServerTypeStdio,
			Cmd:  []string{executablePath, "mcp", "tasks", "--repo-root", absoluteRepoRoot},
		},
	}, nil
}
