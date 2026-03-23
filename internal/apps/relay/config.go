package relay

// Config holds the configuration for the relay bot.
type Config struct {
	Relay RelayConfig `mapstructure:"relay"`
}

// RelayConfig holds the relay-specific configuration.
type RelayConfig struct {
	Telegram TelegramConfig `mapstructure:"telegram"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Logger   LoggerConfig   `mapstructure:"logger"`
	WorkingDir string       `mapstructure:"working_dir"`
}

// TelegramConfig holds the Telegram bot configuration.
type TelegramConfig struct {
	Token        string `mapstructure:"token"`
	WebhookToken string `mapstructure:"webhook_token"`
	WebhookURL   string `mapstructure:"webhook_url"`
}

// AuthConfig holds the authentication configuration.
type AuthConfig struct {
	OwnerToken string `mapstructure:"owner_token"`
	OwnerID    int64  `mapstructure:"owner_id"`
}

// LoggerConfig holds the logger configuration.
type LoggerConfig struct {
	Level  string `mapstructure:"level"`
	Pretty bool   `mapstructure:"pretty"`
}
