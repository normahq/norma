package handlers

import (
	"context"

	"github.com/metalagman/norma/internal/apps/relay/agent"
	"github.com/metalagman/norma/internal/apps/relay/session"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"go.uber.org/fx"
)

// Module provides handlers for the relay bot.
var Module = fx.Module("relay_handlers",
	fx.Provide(
		agent.NewBuilder,
		session.NewManager,
		NewStartHandler,
		NewRelayHandler,
		NewCommandHandler,
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
			registerCommandHandler,
			fx.As(new(tgbotkit.Handler)),
			fx.ResultTags(`group:"bot_handlers"`),
		),
	),
	fx.Invoke(WireHandlers),
	fx.Invoke(InitTopicSessions),
)

// WireHandlers connects the start handler to the relay handler.
func WireHandlers(start *StartHandler, relay *RelayHandler) {
	start.SetRelayHandler(relay)
}

// InitTopicSessions restores persisted topic sessions on startup and closes them on shutdown.
func InitTopicSessions(lc fx.Lifecycle, mgr *session.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return mgr.Restore(ctx)
		},
		OnStop: func(ctx context.Context) error {
			mgr.StopAll()
			return nil
		},
	})
}

func registerStartHandler(h *StartHandler) tgbotkit.Handler {
	return h
}

func registerRelayHandler(h *RelayHandler) tgbotkit.Handler {
	return h
}

func registerCommandHandler(h *CommandHandler) tgbotkit.Handler {
	return h
}
