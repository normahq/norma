package relaycmd

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/metalagman/norma/internal/apps/relay"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed config.yaml
var defaultConfig []byte

const (
	envPrefix         = "NORMA"
	defaultConfigPath = ".norma/config.yaml"
)

// Command builds the `norma relay` command.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Telegram relay bot",
		Long:  "Run a Telegram bot that acts as a relay/proxy to norma agents, enabling starting PDCA loops and receiving notifications.",
	}

	// Add subcommands
	cmd.AddCommand(serveCommand())

	return cmd
}

func serveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start Telegram relay bot",
		Long:  "Start the Telegram relay bot server. A random owner token will be generated and displayed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize viper with default config
			if err := initConfig(); err != nil {
				return fmt.Errorf("init config: %w", err)
			}

			// Unmarshal config from viper
			var cfg relay.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("unmarshal config: %w", err)
			}

			// Validate required fields
			if cfg.Relay.Telegram.Token == "" {
				return fmt.Errorf("telegram token is required\nSet it via:\n  - Environment: NORMA_RELAY_TELEGRAM_TOKEN=<token>\n  - Config file: relay.telegram.token in .norma/config.yaml")
			}

			// Load norma config to get agent configuration
			repoRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			normaCfg, err := loadNormaConfig(repoRoot)
			if err != nil {
				return fmt.Errorf("load norma config: %w", err)
			}

			// Normalize agent aliases (opencode_acp -> generic_acp with cmd)
			executablePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable path: %w", err)
			}
			normaCfg, err = config.NormalizeAgentAliases(normaCfg, executablePath)
			if err != nil {
				return fmt.Errorf("normalize agent aliases: %w", err)
			}

			// Get norma directory
			normaDir, err := getNormaDir(repoRoot)
			if err != nil {
				return fmt.Errorf("get norma dir: %w", err)
			}

			// Always generate a random owner token (one-time use)
			ownerToken, err := auth.GenerateOwnerToken()
			if err != nil {
				return fmt.Errorf("generate owner token: %w", err)
			}

			log.Info().
				Str("owner_token", ownerToken).
				Str("auth_url", fmt.Sprintf("https://t.me/<bot_username>?start=%s", ownerToken)).
				Msg("Relay bot owner token generated")

			// Set owner token in config
			cfg.Relay.Auth.OwnerToken = ownerToken

			// Create and run the relay app with norma config for agent info
			app := relay.App(cfg, normaDir, normaCfg)

			// Setup signal context for graceful shutdown
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			// Start the app
			if err := app.Start(ctx); err != nil {
				return fmt.Errorf("start relay app: %w", err)
			}

			log.Info().Msg("Relay bot started. Press Ctrl+C to stop.")

			// Wait for shutdown signal
			<-ctx.Done()

			// Stop the app
			if err := app.Stop(context.Background()); err != nil {
				return fmt.Errorf("stop relay app: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func initConfig() error {
	// Reset viper to avoid conflicts
	viper.Reset()

	// 1. Load embedded default config FIRST
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(bytes.NewBuffer(defaultConfig)); err != nil {
		return fmt.Errorf("read default config: %w", err)
	}

	// 2. Set up env var support AFTER loading defaults
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	return nil
}

func loadNormaConfig(repoRoot string) (config.Config, error) {
	path := resolveConfigPath(repoRoot, viper.GetString("config"))
	rawConfig, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.Config{}, nil
		}
		return config.Config{}, fmt.Errorf("read config bytes: %w", err)
	}

	expanded, err := config.ExpandEnv(string(rawConfig))
	if err != nil {
		return config.Config{}, fmt.Errorf("expand env vars in config: %w", err)
	}

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(expanded)); err != nil {
		return config.Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return config.Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func resolveConfigPath(repoRoot, configuredPath string) string {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		path = defaultConfigPath
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return path
}

func getNormaDir(repoRoot string) (string, error) {
	return filepath.Join(repoRoot, ".norma"), nil
}
