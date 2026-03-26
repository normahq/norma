package relay

import (
	"context"
	"fmt"
	"strings"
)

type WorkspaceMode string

const (
	WorkspaceModeOn   WorkspaceMode = "on"
	WorkspaceModeOff  WorkspaceMode = "off"
	WorkspaceModeAuto WorkspaceMode = "auto"
)

func ParseWorkspaceMode(raw string) (WorkspaceMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(WorkspaceModeOn):
		return WorkspaceModeOn, nil
	case string(WorkspaceModeOff):
		return WorkspaceModeOff, nil
	case string(WorkspaceModeAuto):
		return WorkspaceModeAuto, nil
	default:
		return "", fmt.Errorf("invalid relay.workspace.mode %q: expected one of on, off, auto", raw)
	}
}

func ResolveWorkspaceEnabled(
	ctx context.Context,
	modeRaw string,
	workingDir string,
	isGitRepo func(context.Context, string) bool,
) (WorkspaceMode, bool, error) {
	mode, err := ParseWorkspaceMode(modeRaw)
	if err != nil {
		return "", false, err
	}

	switch mode {
	case WorkspaceModeOn:
		return mode, true, nil
	case WorkspaceModeOff:
		return mode, false, nil
	case WorkspaceModeAuto:
		if isGitRepo != nil && isGitRepo(ctx, workingDir) {
			return mode, true, nil
		}
		return mode, false, nil
	default:
		return "", false, fmt.Errorf("unsupported workspace mode %q", mode)
	}
}
