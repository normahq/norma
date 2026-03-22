package handlers

import (
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
)

// Module is the Fx module for handlers.
var Module = fx.Module("relay_handlers",
	fx.Provide(
		fx.Annotate(
			NewStartHandler,
			fx.As(new(Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
		fx.Annotate(
			NewRelayHandler,
			fx.As(new(Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
	),
	fx.Invoke(RegisterHandlers),
)

// Handler is the interface for bot handlers.
type Handler interface {
	Register(registry handlers.RegistryInterface)
}

type handlerParams struct {
	fx.In

	Bot      *runtime.Bot
	Handlers []Handler `group:"bot_handlers"`
}

// RegisterHandlers registers all handlers with the bot.
func RegisterHandlers(params handlerParams) {
	for _, handler := range params.Handlers {
		handler.Register(params.Bot.Handlers())
	}
}

// StartHandlerParams contains the parameters for NewStartHandler.
type StartHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	TgClient   client.ClientWithResponsesInterface
	Auth       RelayAuthParams
}

// RelayAuthParams contains the auth token.
type RelayAuthParams struct {
	fx.In

	AuthToken string `name:"relay_auth_token"`
}

// NewStartHandlerWithParams creates a StartHandler with injected parameters.
func NewStartHandlerWithParams(params StartHandlerParams) *StartHandler {
	return NewStartHandler(params.OwnerStore, params.TgClient, params.Auth.AuthToken)
}

// RelayHandlerParams contains the parameters for NewRelayHandler.
type RelayHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	TgClient   client.ClientWithResponsesInterface
}
