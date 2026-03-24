package handlers

import (
	"context"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"go.uber.org/fx"
)

// Module provides handlers for the relay bot.
var Module = fx.Module("relay_handlers",
	fx.Provide(
		NewStartHandler,
		NewRelayHandler,
		NewTopicSessionManager,
		NewNewHandler,
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
		fx.Annotate(
			registerNewHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
	),
	fx.Invoke(WireHandlers),
	fx.Invoke(InitExistingOwner),
	fx.Invoke(InitTopicSessions),
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

// InitExistingOwnerParams contains the parameters for InitExistingOwner.
type InitExistingOwnerParams struct {
	fx.In

	LC           fx.Lifecycle
	OwnerStore   *auth.OwnerStore
	RelayHandler *RelayHandler
}

// InitExistingOwner initializes the relay handler with existing owner if any.
func InitExistingOwner(params InitExistingOwnerParams) {
	if params.OwnerStore.HasOwner() {
		owner := params.OwnerStore.GetOwner()
		// Initialize with owner ID only, chatID will be set when first message arrives.
		params.RelayHandler.InitOwner(context.Background(), owner.UserID)
		log.Info().Int64("owner_id", owner.UserID).Msg("Initialized relay with existing owner")
	}

	params.LC.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			params.RelayHandler.Stop()
			return nil
		},
	})
}

// InitTopicSessionsParams contains parameters for topic session lifecycle.
type InitTopicSessionsParams struct {
	fx.In

	LC             fx.Lifecycle
	SessionManager *TopicSessionManager
}

// InitTopicSessions restores persisted topic sessions on startup and closes them on shutdown.
func InitTopicSessions(params InitTopicSessionsParams) {
	params.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return params.SessionManager.Restore(ctx)
		},
		OnStop: func(ctx context.Context) error {
			params.SessionManager.StopAll()
			return nil
		},
	})
}

// registerStartHandler wraps StartHandler for bot registration.
func registerStartHandler(h *StartHandler) tgbotkit.Handler {
	return h
}

// registerRelayHandler wraps RelayHandler for bot registration.
func registerRelayHandler(h *RelayHandler) tgbotkit.Handler {
	return h
}

// registerNewHandler wraps NewHandler for bot registration.
func registerNewHandler(h *NewHandler) tgbotkit.Handler {
	return h
}
