package relay

import (
	"context"
	"testing"
)

func TestParseWorkspaceMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    WorkspaceMode
		wantErr bool
	}{
		{name: "default empty", raw: "", want: WorkspaceModeAuto},
		{name: "on", raw: "on", want: WorkspaceModeOn},
		{name: "off", raw: "off", want: WorkspaceModeOff},
		{name: "auto", raw: "auto", want: WorkspaceModeAuto},
		{name: "uppercase", raw: "AUTO", want: WorkspaceModeAuto},
		{name: "invalid", raw: "maybe", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseWorkspaceMode(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseWorkspaceMode(%q) returned nil error", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWorkspaceMode(%q) error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseWorkspaceMode(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResolveWorkspaceEnabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		mode    string
		isGit   bool
		want    bool
		wantErr bool
	}{
		{name: "on git", mode: "on", isGit: true, want: true},
		{name: "on non git", mode: "on", isGit: false, wantErr: true},
		{name: "off", mode: "off", isGit: true, want: false},
		{name: "auto git", mode: "auto", isGit: true, want: true},
		{name: "auto non git", mode: "auto", isGit: false, want: false},
		{name: "invalid", mode: "invalid", isGit: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, enabled, err := ResolveWorkspaceEnabled(
				ctx,
				tc.mode,
				"/tmp/repo",
				func(context.Context, string) bool { return tc.isGit },
			)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveWorkspaceEnabled(%q) returned nil error", tc.mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWorkspaceEnabled(%q) error: %v", tc.mode, err)
			}
			if enabled != tc.want {
				t.Fatalf("ResolveWorkspaceEnabled(%q) enabled=%t, want %t", tc.mode, enabled, tc.want)
			}
		})
	}
}
