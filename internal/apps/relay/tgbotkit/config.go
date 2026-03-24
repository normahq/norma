package tgbotkit

// Config holds the configuration for the Telegram bot.
type Config struct {
	Token        string `mapstructure:"token"`
	WebhookToken string `mapstructure:"webhook_token"`
	WebhookURL   string `mapstructure:"webhook_url"`
	ReceiverMode string `mapstructure:"receiver_mode"`
}
