package tgbotkit

import (
	"fmt"
	"strings"

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

	return bot, nil
}

// NewUpdateSource creates a new update source (webhook or polling).
func NewUpdateSource(cfg Config, client client.ClientWithResponsesInterface, l zerolog.Logger) (runtime.UpdateSource, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.ReceiverMode))
	if mode == "" {
		if strings.TrimSpace(cfg.WebhookURL) != "" {
			mode = "webhook"
		} else {
			mode = "polling"
		}
	}

	switch mode {
	case "webhook":
		if strings.TrimSpace(cfg.WebhookURL) == "" {
			return nil, fmt.Errorf("receiver_mode=webhook requires relay.telegram.webhook_url")
		}
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
	case "polling":
		opts := updatepoller.NewOptions(
			client,
			updatepoller.WithOffsetStore(offsetstore.NewInMemoryOffsetStore(0)),
			updatepoller.WithLogger(logger.NewZerolog(l)),
		)
		return updatepoller.NewPoller(opts)
	default:
		return nil, fmt.Errorf("unsupported relay.telegram.receiver_mode %q (expected polling or webhook)", mode)
	}
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
