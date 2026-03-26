package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBundled(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "norma.config", want: true},
		{id: "norma.state", want: true},
		{id: "norma.relay", want: true},
		{id: "norma.workspace", want: true},
		{id: "norma.tasks", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			if got := isBundled(tc.id); got != tc.want {
				t.Fatalf("isBundled(%q) = %t, want %t", tc.id, got, tc.want)
			}
		})
	}
}

func TestSelectConfigPath_PrefersAppSpecificFile(t *testing.T) {
	workDir := t.TempDir()
	normaDir := filepath.Join(workDir, ".norma")
	if err := os.MkdirAll(normaDir, 0o755); err != nil {
		t.Fatalf("mkdir .norma: %v", err)
	}
	if err := os.WriteFile(filepath.Join(normaDir, "config.yaml"), []byte("a: b\n"), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(normaDir, "relay.yaml"), []byte("a: c\n"), 0o600); err != nil {
		t.Fatalf("write relay.yaml: %v", err)
	}

	got := selectConfigPath(workDir, "relay")
	want := filepath.Join(normaDir, "relay.yaml")
	if got != want {
		t.Fatalf("selectConfigPath() = %q, want %q", got, want)
	}
}

func TestSelectConfigPath_FallsBackToCoreConfig(t *testing.T) {
	workDir := t.TempDir()
	normaDir := filepath.Join(workDir, ".norma")
	if err := os.MkdirAll(normaDir, 0o755); err != nil {
		t.Fatalf("mkdir .norma: %v", err)
	}
	if err := os.WriteFile(filepath.Join(normaDir, "config.yaml"), []byte("a: b\n"), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	got := selectConfigPath(workDir, "relay")
	want := filepath.Join(normaDir, "config.yaml")
	if got != want {
		t.Fatalf("selectConfigPath() = %q, want %q", got, want)
	}
}
