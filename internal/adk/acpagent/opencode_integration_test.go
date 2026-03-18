//go:build integration && opencode

package acpagent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

const opencodeIntegrationTimeout = 90 * time.Second

func TestOpenCodeACPIntegration_InitializeAndNewSession(t *testing.T) {
	repoRoot := requireOpenCodeEnvironment(t)
	client, stderr := newOpenCodeACPClient(t, repoRoot, "acp")

	initResp := mustInitializeACP(t, client, stderr)
	if initResp.ProtocolVersion != acp.ProtocolVersion(acp.ProtocolVersionNumber) {
		t.Fatalf("initialize protocol version = %d, want %d", initResp.ProtocolVersion, acp.ProtocolVersionNumber)
	}
	_ = mustNewACPSession(t, client, stderr, repoRoot)
}

func TestOpenCodeACPIntegration_PromptRoundTrip(t *testing.T) {
	repoRoot := requireOpenCodeEnvironment(t)
	client, stderr := newOpenCodeACPClient(t, repoRoot, "acp")

	_ = mustInitializeACP(t, client, stderr)
	sess := mustNewACPSession(t, client, stderr, repoRoot)

	ctx, cancel := context.WithTimeout(context.Background(), opencodeIntegrationTimeout)
	defer cancel()

	updates, resultCh, err := client.Prompt(ctx, string(sess.SessionId), "Reply with one short word.")
	if err != nil {
		failWithDetails(t, "session/prompt failed to start", err, stderr.String())
	}

	var chunks []string
	for note := range updates {
		if text := strings.TrimSpace(updateText(note.Update)); text != "" {
			chunks = append(chunks, text)
		}
	}
	result := <-resultCh
	if result.Err != nil {
		failWithDetails(t, "session/prompt returned error", result.Err, stderr.String())
	}
	if result.Response.StopReason == "" {
		failWithDetails(t, "session/prompt returned empty stop_reason", nil, stderr.String())
	}
	if len(chunks) == 0 {
		failWithDetails(t, "session/prompt produced no text chunks", nil, stderr.String())
	}
}

func TestOpenCodeACPIntegration_InvalidArgFailsInitialize(t *testing.T) {
	repoRoot := requireOpenCodeEnvironment(t)
	client, stderr := newOpenCodeACPClient(t, repoRoot, "--definitely-invalid-flag", "acp")

	ctx, cancel := context.WithTimeout(context.Background(), opencodeIntegrationTimeout)
	defer cancel()
	if _, err := client.Initialize(ctx); err == nil {
		failWithDetails(t, "initialize unexpectedly succeeded with invalid opencode arg", nil, stderr.String())
	}
}

func requireOpenCodeEnvironment(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("opencode"); err != nil {
		t.Fatalf("opencode binary not found in PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	helpCmd := exec.CommandContext(ctx, "opencode", "acp", "--help")
	var helpOut bytes.Buffer
	helpCmd.Stdout = &helpOut
	helpCmd.Stderr = &helpOut
	if err := helpCmd.Run(); err != nil {
		t.Fatalf("opencode acp --help failed: %v | output=%s", err, strings.TrimSpace(helpOut.String()))
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
			t.Fatalf("could not locate repo root containing go.mod (started from %q)", dir)
		}
		dir = parent
	}
}

func newOpenCodeACPClient(t *testing.T, repoRoot string, commandArgs ...string) (*Client, *bytes.Buffer) {
	t.Helper()

	configureOpenCodeWritableEnv(t)

	command := append([]string{"opencode"}, commandArgs...)
	var stderr bytes.Buffer
	client, err := NewClient(context.Background(), ClientConfig{
		Command:    command,
		WorkingDir: repoRoot,
		Stderr:     &stderr,
	})
	if err != nil {
		failWithDetails(t, "start ACP client failed", err, stderr.String())
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client, &stderr
}

func configureOpenCodeWritableEnv(t *testing.T) {
	t.Helper()

	xdgRoot := t.TempDir()
	xdgData := filepath.Join(xdgRoot, "data")
	xdgState := filepath.Join(xdgRoot, "state")
	xdgCache := filepath.Join(xdgRoot, "cache")
	for _, dir := range []string{xdgData, xdgState, xdgCache} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create dir %q: %v", dir, err)
		}
	}

	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_STATE_HOME", xdgState)
	t.Setenv("XDG_CACHE_HOME", xdgCache)
}

func mustInitializeACP(t *testing.T, client *Client, stderr *bytes.Buffer) acp.InitializeResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), opencodeIntegrationTimeout)
	defer cancel()

	resp, err := client.Initialize(ctx)
	if err != nil {
		maybeSkipOpenCodeIntegration(t, err, stderr.String())
		failWithDetails(t, "initialize failed", err, stderr.String())
	}
	return resp
}

func mustNewACPSession(t *testing.T, client *Client, stderr *bytes.Buffer, cwd string) acp.NewSessionResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), opencodeIntegrationTimeout)
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

func maybeSkipOpenCodeIntegration(t *testing.T, err error, stderr string) {
	t.Helper()

	errText := strings.ToLower(strings.TrimSpace(err.Error()))
	stderrText := strings.ToLower(strings.TrimSpace(stderr))
	combined := errText + "\n" + stderrText

	skipMarkers := []string{
		"peer disconnected before response",
		"failed to start server on port 0",
		"unable to connect. is the computer able to access the url?",
		"service=models.dev error=unable to connect",
	}
	for _, marker := range skipMarkers {
		if strings.Contains(combined, marker) {
			t.Skipf("opencode ACP unavailable in this environment (%s)", marker)
		}
	}
}
