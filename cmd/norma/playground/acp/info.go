package acpcmd

import (
	"context"
	"io"

	"github.com/normahq/norma/internal/apps/acpdump"
	"github.com/rs/zerolog/log"
)

func runACPInfo(
	ctx context.Context,
	workingDir string,
	command []string,
	sessionModel string,
	component string,
	startMsg string,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
) error {
	if component != "" {
		ctx = log.Ctx(ctx).With().Str("component", component).Logger().WithContext(ctx)
	}

	return acpdump.Run(ctx, acpdump.RunConfig{
		Command:      command,
		WorkingDir:   workingDir,
		SessionModel: sessionModel,
		StartMessage: startMsg,
		JSONOutput:   jsonOutput,
		Stdout:       stdout,
		Stderr:       stderr,
	})
}
