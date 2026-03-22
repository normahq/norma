package relay

import (
	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

// App creates a new fx.App for the relay bot.
func App(cfg Config) *fx.App {
	return fx.New(
		fx.WithLogger(
			fxlogger.WithZerolog(
				log.Logger.With().Str("component", "relay").Logger(),
			),
		),
		Module(cfg),
	)
}

// Module returns the fx.Module for the relay bot.
func Module(cfg Config) fx.Option {
	return fx.Module("relay",
		fx.Supply(
			cfg.Telegram,
			cfg.Logger,
		),
		tgbotkit.Module,
	)
}
