//go:build integration && codex

package codexacpbridge_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/normahq/norma/pkg/runtime/acpagent"
)

const testTimeout = 45 * time.Second

func TestCodexACPProxyIntegration_NewSessionWithMCP(t *testing.T) {
	workingDir := requireCodexEnvironment(t)
	normaBin := buildNormaBinary(t, workingDir)

	client, stderr := newToolACPClient(t, workingDir, normaBin)
	_ = mustInitialize(t, client, stderr)

	mcpServers := []acp.McpServer{{
		Stdio: &acp.McpServerStdio{
			Name:    "test-stdio",
			Command: "echo",
			Args:    []string{"hello"},
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	_, err := client.NewSession(ctx, workingDir, mcpServers)
	if err != nil {
		failWithDetails(t, "session/new failed with mcpServers", err, stderr.String())
	}
}

func TestCodexACPProxyIntegration_InitializeAndNewSession(t *testing.T) {
	workingDir := requireCodexEnvironment(t)
	normaBin := buildNormaBinary(t, workingDir)

	client, stderr := newToolACPClient(t, workingDir, normaBin)
	initResp := mustInitialize(t, client, stderr)
	if initResp.ProtocolVersion != acp.ProtocolVersion(acp.ProtocolVersionNumber) {
		t.Fatalf("initialize protocol version = %d, want %d", initResp.ProtocolVersion, acp.ProtocolVersionNumber)
	}

	mustNewSession(t, client, stderr, workingDir)
}

func TestCodexACPProxyIntegration_CustomName(t *testing.T) {
	workingDir := requireCodexEnvironment(t)
	normaBin := buildNormaBinary(t, workingDir)
	_, wantVersion := probeCodexMCPIdentity(t, workingDir)

	client, stderr := newToolACPClient(t, workingDir, normaBin, "--name", "team-codex")
	initResp := mustInitialize(t, client, stderr)
	if initResp.AgentInfo == nil {
		t.Fatal("initialize agentInfo is nil")
	}
	if initResp.AgentInfo.Name != "team-codex" {
		t.Fatalf("initialize agentInfo.name = %q, want %q", initResp.AgentInfo.Name, "team-codex")
	}
	if initResp.AgentInfo.Version != wantVersion {
		t.Fatalf("initialize agentInfo.version = %q, want %q", initResp.AgentInfo.Version, wantVersion)
	}
}

func TestCodexACPProxyIntegration_DefaultIdentityFromMCP(t *testing.T) {
	workingDir := requireCodexEnvironment(t)
	normaBin := buildNormaBinary(t, workingDir)
	wantName, wantVersion := probeCodexMCPIdentity(t, workingDir)

	client, stderr := newToolACPClient(t, workingDir, normaBin)
	initResp := mustInitialize(t, client, stderr)
	if initResp.AgentInfo == nil {
		t.Fatal("initialize agentInfo is nil")
	}
	if initResp.AgentInfo.Name != wantName {
		t.Fatalf("initialize agentInfo.name = %q, want %q", initResp.AgentInfo.Name, wantName)
	}
	if initResp.AgentInfo.Version != wantVersion {
		t.Fatalf("initialize agentInfo.version = %q, want %q", initResp.AgentInfo.Version, wantVersion)
	}
}

func TestCodexACPProxyIntegration_RejectsPositionalArgs(t *testing.T) {
	workingDir := requireCodexEnvironment(t)
	normaBin := buildNormaBinary(t, workingDir)

	client, stderr := newToolACPClient(t, workingDir, normaBin, "--", "--definitely-invalid-flag")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := client.Initialize(ctx); err == nil {
		failWithDetails(t, "initialize unexpectedly succeeded with rejected positional arg", nil, stderr.String())
	}

	if !strings.Contains(stderr.String(), "accepts 0 arg(s)") {
		failWithDetails(t, "proxy stderr does not indicate positional args are rejected", nil, stderr.String())
	}
}

func requireCodexEnvironment(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("codex"); err != nil {
		t.Fatalf("codex binary not found in PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	helpCmd := exec.CommandContext(ctx, "codex", "mcp-server", "--help")
	var helpOut bytes.Buffer
	helpCmd.Stdout = &helpOut
	helpCmd.Stderr = &helpOut
	if err := helpCmd.Run(); err != nil {
		t.Fatalf("codex mcp-server --help failed: %v | output=%s", err, strings.TrimSpace(helpOut.String()))
	}

	return findRepoRoot(t)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat go.mod failed in %q: %v", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate working dir containing go.mod (started from %q)", dir)
		}
		dir = parent
	}
}

func buildNormaBinary(t *testing.T, workingDir string) string {
	t.Helper()

	binPath := filepath.Join(t.TempDir(), "norma")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/norma")
	cmd.Dir = workingDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build norma binary failed: %v | output=%s", err, strings.TrimSpace(string(out)))
	}
	return binPath
}

func newToolACPClient(t *testing.T, workingDir, normaBin string, args ...string) (*acpagent.Client, *bytes.Buffer) {
	t.Helper()

	command := []string{normaBin, "tool", "codex-acp-bridge"}
	command = append(command, args...)

	var stderr bytes.Buffer
	client, err := acpagent.NewClient(context.Background(), acpagent.ClientConfig{
		Command:    command,
		WorkingDir: workingDir,
		Stderr:     &stderr,
	})
	if err != nil {
		failWithDetails(t, "start proxy acp client failed", err, stderr.String())
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client, &stderr
}

func mustInitialize(t *testing.T, client *acpagent.Client, stderr *bytes.Buffer) acp.InitializeResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	resp, err := client.Initialize(ctx)
	if err != nil {
		failWithDetails(t, "initialize failed", err, stderr.String())
	}
	return resp
}

func mustNewSession(t *testing.T, client *acpagent.Client, stderr *bytes.Buffer, cwd string) acp.NewSessionResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	resp, err := client.NewSession(ctx, cwd, nil)
	if err != nil {
		failWithDetails(t, "session/new failed", err, stderr.String())
	}
	if strings.TrimSpace(string(resp.SessionId)) == "" {
		failWithDetails(t, "session/new returned empty session id", nil, stderr.String())
	}
	return resp
}

func failWithDetails(t *testing.T, heading string, err error, stderr string) {
	t.Helper()

	errText := ""
	if err != nil {
		errText = strings.TrimSpace(err.Error())
	}
	stderrText := strings.TrimSpace(stderr)

	message := heading
	if errText != "" {
		message += ": " + errText
	}
	if stderrText != "" && (errText == "" || !strings.Contains(stderrText, errText)) {
		message += " | stderr: " + stderrText
	}
	t.Fatal(message)
}

func probeCodexMCPIdentity(t *testing.T, workingDir string) (name string, version string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "norma-bridge-test", Version: "v0.0.0"}, nil)
	cmd := exec.CommandContext(ctx, "codex", "mcp-server")
	cmd.Dir = workingDir
	cmd.Stderr = io.Discard

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect codex mcp-server for identity probe failed: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = session.Wait()
	})

	result := session.InitializeResult()
	if result == nil || result.ServerInfo == nil {
		t.Fatal("codex identity probe returned empty initialize serverInfo")
	}
	name = strings.TrimSpace(result.ServerInfo.Name)
	version = strings.TrimSpace(result.ServerInfo.Version)
	if name == "" {
		t.Fatal("codex identity probe returned empty serverInfo.name")
	}
	if version == "" {
		t.Fatal("codex identity probe returned empty serverInfo.version")
	}
	return name, version
}
