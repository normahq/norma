package tgbotkit

import (
	"context"

	"github.com/metalagman/appkit/lifecycle"
	"github.com/rs/zerolog"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime"
	"github.com/tgbotkit/runtime/handlers"
	"github.com/tgbotkit/runtime/logger"
	"github.com/tgbotkit/runtime/updatepoller"
	"github.com/tgbotkit/runtime/updatepoller/offsetstore"
	"github.com/tgbotkit/runtime/webhook"
	"go.uber.org/fx"
)

var Module = fx.Module("relay_tgbotkit",
	fx.Provide(
		NewUpdateSource,
		NewBot,
		NewClient,
	),
	fx.Invoke(RegisterHandlers),
	fx.Invoke(func(*runtime.Bot) {
		// Placeholder to ensure bot is created
	}),
)

// NewClient creates a new Telegram API client.
func NewClient(cfg Config) (client.ClientWithResponsesInterface, error) {
	serverURL, err := client.NewServerUrlTelegramBotAPIEndpointSubstituteBotTokenWithYourBotToken(
		client.ServerUrlTelegramBotAPIEndpointSubstituteBotTokenWithYourBotTokenBotTokenVariable(cfg.Token),
	)
	if err != nil {
		return nil, err
	}

	return client.NewClientWithResponses(serverURL)
}

// NewBot creates a new Telegram bot runtime.
func NewBot(
	lc fx.Lifecycle,
	cfg Config,
	client client.ClientWithResponsesInterface,
	updateSource runtime.UpdateSource,
	l zerolog.Logger,
) (*runtime.Bot, error) {
	bot, err := runtime.New(
		runtime.NewOptions(
			cfg.Token,
			runtime.WithUpdateSource(updateSource),
			runtime.WithClient(client),
			runtime.WithLogger(logger.NewZerolog(l)),
		),
	)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := bot.Run(runCtx); err != nil {
					// Using bot logger which is already configured
					bot.Logger().Errorf("bot run failed: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			return nil
		},
	})

	return bot, nil
}

// NewUpdateSource creates a new update source (webhook or polling).
func NewUpdateSource(cfg Config, client client.ClientWithResponsesInterface, l zerolog.Logger) (runtime.UpdateSource, error) {
	// Use webhook if configured
	if cfg.WebhookURL != "" {
		w, err := webhook.New(
			webhook.NewOptions(
				webhook.WithToken(cfg.WebhookToken),
				webhook.WithUrl(cfg.WebhookURL),
				webhook.WithClient(client),
			),
		)
		if err != nil {
			return nil, err
		}

		return w, nil
	}

	// Use long polling as default
	opts := updatepoller.NewOptions(
		client,
		updatepoller.WithOffsetStore(offsetstore.NewInMemoryOffsetStore(0)),
		updatepoller.WithLogger(logger.NewZerolog(l)),
	)

	return updatepoller.NewPoller(opts)
}

// Handler is a local interface for bot handlers.
type Handler interface {
	Register(registry handlers.RegistryInterface)
}

type handlerParams struct {
	fx.In

	Bot      *runtime.Bot
	Handlers []Handler `group:"bot_handlers"`
}

// RegisterHandlers registers all bot handlers.
func RegisterHandlers(params handlerParams) {
	for _, handler := range params.Handlers {
		handler.Register(params.Bot.Handlers())
	}
}

// lifecycleCheck ensures UpdateSource implements lifecycle.Lifecycle.
var _ lifecycle.Lifecycle = (runtime.UpdateSource)(nil)
