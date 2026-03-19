package plancmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/metalagman/norma/internal/adk/agentconfig"
	"github.com/metalagman/norma/internal/config"
)

func TestPlannerMCPServersAddsTasksServerAndMergesConfigured(t *testing.T) {
	prev := resolvePlannerExecutablePath
	resolvePlannerExecutablePath = func() (string, error) { return "/tmp/norma", nil }
	t.Cleanup(func() { resolvePlannerExecutablePath = prev })

	configured := map[string]config.MCPServerConfig{
		"existing": {
			Type: agentconfig.MCPServerTypeStdio,
			Cmd:  []string{"echo", "ok"},
		},
	}

	servers, err := plannerMCPServers("./repo", configured)
	if err != nil {
		t.Fatalf("plannerMCPServers() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d, want 2", len(servers))
	}

	existing, ok := servers["existing"]
	if !ok {
		t.Fatalf("servers missing configured entry %q", "existing")
	}
	if existing.Type != agentconfig.MCPServerTypeStdio {
		t.Fatalf("existing.Type = %q, want %q", existing.Type, agentconfig.MCPServerTypeStdio)
	}
	if len(existing.Cmd) != 2 || existing.Cmd[0] != "echo" || existing.Cmd[1] != "ok" {
		t.Fatalf("existing.Cmd = %v, want %v", existing.Cmd, []string{"echo", "ok"})
	}

	tasksCfg, ok := servers[plannerTasksMCPName]
	if !ok {
		t.Fatalf("servers missing planner tasks MCP entry %q", plannerTasksMCPName)
	}
	if tasksCfg.Type != agentconfig.MCPServerTypeStdio {
		t.Fatalf("tasksCfg.Type = %q, want %q", tasksCfg.Type, agentconfig.MCPServerTypeStdio)
	}
	if len(tasksCfg.Cmd) != 5 {
		t.Fatalf("len(tasksCfg.Cmd) = %d, want 5", len(tasksCfg.Cmd))
	}
	if tasksCfg.Cmd[0] != "/tmp/norma" || tasksCfg.Cmd[1] != "mcp" || tasksCfg.Cmd[2] != "tasks" || tasksCfg.Cmd[3] != "--repo-root" {
		t.Fatalf("tasksCfg.Cmd = %v, want executable + mcp tasks --repo-root", tasksCfg.Cmd)
	}
	if !filepath.IsAbs(tasksCfg.Cmd[4]) {
		t.Fatalf("tasksCfg.Cmd[4] = %q, want absolute repo root", tasksCfg.Cmd[4])
	}
}

func TestPlannerMCPServersRequiresRepoRoot(t *testing.T) {
	_, err := plannerMCPServers("   ", nil)
	if err == nil {
		t.Fatal("plannerMCPServers() error = nil, want non-nil")
	}
}

func TestPlannerMCPServersExecutableError(t *testing.T) {
	prev := resolvePlannerExecutablePath
	resolvePlannerExecutablePath = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { resolvePlannerExecutablePath = prev })

	_, err := plannerMCPServers(".", nil)
	if err == nil {
		t.Fatal("plannerMCPServers() error = nil, want non-nil")
	}
}
