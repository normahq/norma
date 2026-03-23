package relay

import (
	"os"

	"github.com/ipfans/fxlogger"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/handlers"
	"github.com/metalagman/norma/internal/apps/relay/tgbotkit"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

// App creates a new fx.App for the relay bot.
func App(cfg Config, normaDir string, normaCfg config.Config) *fx.App {
	return fx.New(
		fx.WithLogger(
			fxlogger.WithZerolog(
				log.Logger.With().Str("component", "relay").Logger(),
			),
		),
		Module(cfg, normaDir, normaCfg),
	)
}

// Module returns the fx.Module for the relay bot.
func Module(cfg Config, normaDir string, normaCfg config.Config) fx.Option {
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
			logger,
			normaCfg,
		),
		// Provide auth token with named injection
		fx.Provide(
			fx.Annotate(
				func() string { return cfg.Auth.OwnerToken },
				fx.ResultTags(`name:"relay_auth_token"`),
			),
		),
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
