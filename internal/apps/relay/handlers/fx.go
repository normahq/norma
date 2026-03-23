package handlers

import (
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/tgbotkit/client"
	"go.uber.org/fx"
)

// Module provides handlers for the relay bot.
var Module = fx.Module("relay_handlers",
	fx.Provide(
		// Provide concrete types (for WireHandlers)
		NewStartHandler,
		NewRelayHandler,
	),
	fx.Invoke(
		// Wire handlers together first
		WireHandlers,
		// Then register with bot as tgbotkit.Handler interface
		fx.Annotate(
			registerStartHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
		fx.Annotate(
			registerRelayHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
	),
)

// StartHandlerParams contains the parameters for NewStartHandler.
type StartHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	TgClient   client.ClientWithResponsesInterface
	Auth       AuthParams
}

// AuthParams provides the auth token.
type AuthParams struct {
	fx.In

	AuthToken string `name:"relay_auth_token"`
}

// WireHandlersParams contains the parameters for WireHandlers.
type WireHandlersParams struct {
	fx.In

	StartHandler *StartHandler
	RelayHandler *RelayHandler
}

// WireHandlers connects the start handler to the relay handler.
func WireHandlers(params WireHandlersParams) {
	params.StartHandler.SetRelayHandler(params.RelayHandler)
}

// registerStartHandler wraps StartHandler for bot registration.
func registerStartHandler(h *StartHandler) tgbotkit.Handler {
	return h
}

// registerRelayHandler wraps RelayHandler for bot registration.
func registerRelayHandler(h *RelayHandler) tgbotkit.Handler {
	return h
}
