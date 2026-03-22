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
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed config.yaml
var defaultConfig []byte

const envPrefix = "NORMA"

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
			if cfg.Telegram.Token == "" {
				return fmt.Errorf("telegram token is required\nSet it via:\n  - Environment: NORMA_TELEGRAM_TOKEN=<token>\n  - Config file: telegram.token in .norma/config.yaml")
			}

			// Get norma directory
			normaDir, err := getNormaDir()
			if err != nil {
				return fmt.Errorf("get norma dir: %w", err)
			}

			// Always generate a random owner token (one-time use)
			ownerToken, err := auth.GenerateOwnerToken()
			if err != nil {
				return fmt.Errorf("generate owner token: %w", err)
			}
			log.Info().Str("owner_token", ownerToken).Msg("Generated one-time owner token")

			// Display owner token and auth URL instructions using logger
			log.Info().
				Str("owner_token", ownerToken).
				Msg("=== Norma Relay Bot ===")
			log.Info().
				Str("owner_token", ownerToken).
				Msg("To authenticate as owner, open this URL in your browser:")
			log.Info().
				Str("url", fmt.Sprintf("https://t.me/<bot_username>/start?auth=%s", ownerToken)).
				Msg("Auth URL (replace <bot_username> with your bot username from @BotFather)")

			// Set owner token in config
			cfg.Auth.OwnerToken = ownerToken

			// Create and run the relay app
			app := relay.App(cfg, normaDir)

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

func getNormaDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".norma"), nil
}
