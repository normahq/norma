package handlers

import (
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/tgbotkit/client"
	"go.uber.org/fx"
)

var Module = fx.Module("relay_handlers",
	fx.Provide(
		fx.Annotate(
			NewStartHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
		fx.Annotate(
			NewRelayHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
	),
	fx.Invoke(WireHandlers),
)

type StartHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	TgClient   client.ClientWithResponsesInterface
	Auth       AuthParams
}

type AuthParams struct {
	fx.In

	AuthToken string `name:"relay_auth_token"`
}

type RelayHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	TgClient   client.ClientWithResponsesInterface
}

type WireHandlersParams struct {
	fx.In

	StartHandler *StartHandler
	RelayHandler *RelayHandler
}

// WireHandlers connects the start handler to the relay handler.
func WireHandlers(params WireHandlersParams) {
	params.StartHandler.SetRelayHandler(params.RelayHandler)
}
