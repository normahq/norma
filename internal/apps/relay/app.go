package relay

import (
	"os"

	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/handlers"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

// App creates a new fx.App for the relay bot.
func App(cfg Config, normaDir string) *fx.App {
	return fx.New(
		fx.WithLogger(
			fxlogger.WithZerolog(
				log.Logger.With().Str("component", "relay").Logger(),
			),
		),
		Module(cfg, normaDir),
	)
}

// Module returns the fx.Module for the relay bot.
func Module(cfg Config, normaDir string) fx.Option {
	// Convert relay config to tgbotkit config
	tgbotkitCfg := tgbotkit.Config{
		Token:        cfg.Telegram.Token,
		WebhookToken: cfg.Telegram.WebhookToken,
		WebhookURL:   cfg.Telegram.WebhookURL,
	}

	// Create logger
	logger := log.Logger.With().Str("component", "relay").Logger()

	return fx.Module("relay",
		fx.Supply(
			tgbotkitCfg,
			cfg.Auth.OwnerToken,
		),
		// Provide zerolog.Logger
		fx.Supply(logger),
		// Provide owner store
		fx.Provide(func() (*auth.OwnerStore, error) {
			// Ensure norma directory exists
			if err := os.MkdirAll(normaDir, 0755); err != nil {
				return nil, err
			}
			return auth.NewOwnerStore(normaDir)
		}),
		tgbotkit.Module,
		handlers.Module,
	)
}
