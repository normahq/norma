package acpcmd

import (
	"context"
	"io"

	"github.com/metalagman/norma/internal/apps/acpdump"
)

func runACPInfo(
	ctx context.Context,
	repoRoot string,
	command []string,
	sessionModel string,
	component string,
	startMsg string,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
) error {
	return acpdump.Run(ctx, acpdump.RunConfig{
		Command:      command,
		WorkingDir:   repoRoot,
		SessionModel: sessionModel,
		Component:    component,
		StartMessage: startMsg,
		JSONOutput:   jsonOutput,
		Stdout:       stdout,
		Stderr:       stderr,
	})
}
