package agent

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
)

// DefaultPermissionHandler auto-denies permission requests with no options.
func DefaultPermissionHandler(_ context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	if len(req.Options) > 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(req.Options[0].OptionId),
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.NewRequestPermissionOutcomeCancelled(),
	}, nil
}
