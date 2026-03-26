package relay

// Config holds the configuration for the relay bot.
type Config struct {
	Relay RelayConfig `mapstructure:"relay"`
}

// RelayConfig holds the relay-specific configuration.
type RelayConfig struct {
	OrchestratorAgent string            `mapstructure:"orchestrator_agent"`
	Telegram          TelegramConfig    `mapstructure:"telegram"`
	Auth              AuthConfig        `mapstructure:"auth"`
	Logger            LoggerConfig      `mapstructure:"logger"`
	WorkingDir        string            `mapstructure:"working_dir"`
	Workspace         WorkspaceConfig   `mapstructure:"workspace"`
	MCP               MCPConfig         `mapstructure:"mcp"`
	InternalMCP       InternalMCPConfig `mapstructure:"internal_mcp"`
}

// TelegramConfig holds the Telegram bot configuration.
type TelegramConfig struct {
	Token        string `mapstructure:"token"`
	WebhookToken string `mapstructure:"webhook_token"`
	WebhookURL   string `mapstructure:"webhook_url"`
	ReceiverMode string `mapstructure:"receiver_mode"`
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

// MCPConfig holds the MCP server configuration.
type MCPConfig struct {
	Address string `mapstructure:"address"`
}

// WorkspaceConfig controls relay Git workspace behavior.
type WorkspaceConfig struct {
	Mode string `mapstructure:"mode"`
}

// InternalMCPConfig contains startup configuration for internal MCP servers.
type InternalMCPConfig struct {
	Servers []string `mapstructure:"servers"`
}
