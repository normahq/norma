package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBeadsTrackerExec_ForcesNoDaemon(t *testing.T) {
	callsPath := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("BD_TEST_CALLS", callsPath)

	bdPath := writeFakeBeadsCLI(t, `
calls="${BD_TEST_CALLS:?}"
printf '%s\n' "$*" >> "$calls"
if [[ "$1" != "--no-daemon" ]]; then
  echo "missing --no-daemon" >&2
  exit 1
fi
shift
if [[ "$1" == "list" ]]; then
  echo '[]'
  exit 0
fi
echo "unexpected command: $*" >&2
exit 1
`)

	tracker := &BeadsTracker{BinPath: bdPath, WorkingDir: t.TempDir()}
	status := normaStatusTodo

	got, err := tracker.List(context.Background(), &status)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() len = %d, want 0", len(got))
	}

	lines := readCallLogLines(t, callsPath)
	if len(lines) != 1 {
		t.Fatalf("call count = %d, want 1 (%v)", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "--no-daemon list ") {
		t.Fatalf("first call = %q, want --no-daemon list ...", lines[0])
	}
}

func TestBeadsTrackerExec_StaleDatabaseAutoImportAndRetry(t *testing.T) {
	tempDir := t.TempDir()
	callsPath := filepath.Join(tempDir, "calls.log")
	statePath := filepath.Join(tempDir, "state.flag")
	t.Setenv("BD_TEST_CALLS", callsPath)
	t.Setenv("BD_TEST_STALE_STATE", statePath)

	bdPath := writeFakeBeadsCLI(t, `
calls="${BD_TEST_CALLS:?}"
state="${BD_TEST_STALE_STATE:?}"
printf '%s\n' "$*" >> "$calls"
if [[ "$1" != "--no-daemon" ]]; then
  echo "missing --no-daemon" >&2
  exit 1
fi
shift
case "$1" in
  ready)
    if [[ ! -f "$state" ]]; then
      touch "$state"
      echo "Database out of sync with JSONL. Run 'bd sync --import-only' to fix." >&2
      exit 1
    fi
    echo '[]'
    ;;
  sync)
    if [[ "${2:-}" != "--import-only" ]]; then
      echo "unexpected sync args: $*" >&2
      exit 1
    fi
    echo '{"ok":true}'
    ;;
  *)
    echo "unexpected command: $*" >&2
    exit 1
    ;;
esac
`)

	tracker := &BeadsTracker{BinPath: bdPath, WorkingDir: tempDir}
	got, err := tracker.LeafTasks(context.Background())
	if err != nil {
		t.Fatalf("LeafTasks() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("LeafTasks() len = %d, want 0", len(got))
	}

	lines := readCallLogLines(t, callsPath)
	if len(lines) != 3 {
		t.Fatalf("call count = %d, want 3 (%v)", len(lines), lines)
	}
	if lines[0] != "--no-daemon ready --limit 0 --json --quiet" {
		t.Fatalf("first call = %q", lines[0])
	}
	if lines[1] != "--no-daemon sync --import-only --json --quiet" {
		t.Fatalf("second call = %q", lines[1])
	}
	if lines[2] != "--no-daemon ready --limit 0 --json --quiet" {
		t.Fatalf("third call = %q", lines[2])
	}
}

func TestLeafTasks_ReturnsOnlyExecutableLeaves(t *testing.T) {
	tempDir := t.TempDir()
	callsPath := filepath.Join(tempDir, "calls.log")
	t.Setenv("BD_TEST_CALLS", callsPath)

	bdPath := writeFakeBeadsCLI(t, `
calls="${BD_TEST_CALLS:?}"
printf '%s\n' "$*" >> "$calls"
if [[ "$1" != "--no-daemon" ]]; then
  echo "missing --no-daemon" >&2
  exit 1
fi
shift

if [[ "$1" == "ready" ]]; then
  echo '[{"id":"task-ready","issue_type":"task","title":"Ready Task","description":"Objective: x\nArtifact: y\nVerify: z","status":"open"},{"id":"feature-a","issue_type":"feature","title":"Feature A","description":"","status":"open"},{"id":"task-parent","issue_type":"task","title":"Parent Task","description":"Objective: p\nArtifact: p\nVerify: p","status":"open"}]'
  exit 0
fi

if [[ "$1" == "list" ]]; then
  parent=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "--parent" ]]; then
      parent="$2"
      break
    fi
    shift
  done
  if [[ "$parent" == "task-ready" ]]; then
    echo '[]'
    exit 0
  fi
  if [[ "$parent" == "task-parent" ]]; then
    echo '[{"id":"child-1","issue_type":"task","title":"Child","description":"","status":"open"}]'
    exit 0
  fi
  echo '[]'
  exit 0
fi

echo "unexpected command: $*" >&2
exit 1
`)

	tracker := &BeadsTracker{BinPath: bdPath, WorkingDir: tempDir}
	got, err := tracker.LeafTasks(context.Background())
	if err != nil {
		t.Fatalf("LeafTasks() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("LeafTasks() len = %d, want 1", len(got))
	}
	if got[0].ID != "task-ready" {
		t.Fatalf("LeafTasks()[0].ID = %q, want %q", got[0].ID, "task-ready")
	}

	lines := readCallLogLines(t, callsPath)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "--no-daemon ready --limit 0 --json --quiet") {
		t.Fatalf("expected ready call with --limit 0, calls=%v", lines)
	}
	if strings.Contains(joined, "--parent feature-a") {
		t.Fatalf("feature issue should not be checked for children, calls=%v", lines)
	}
}

func writeFakeBeadsCLI(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "bd")
	content := "#!/usr/bin/env bash\nset -euo pipefail\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake bd script: %v", err)
	}
	return path
}

func readCallLogLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
