package task

import "go.uber.org/fx"

// Module is the Fx module for task management.
var Module = fx.Module("task",
	fx.Provide(
		func() *BeadsTracker {
			return NewBeadsTracker("")
		},
	),
)
