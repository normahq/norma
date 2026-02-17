package agentfactory

import "go.uber.org/fx"

// Module is the Fx module for the agent factory.
var Module = fx.Module("agentfactory",
	fx.Provide(
		NewFactory,
	),
)
