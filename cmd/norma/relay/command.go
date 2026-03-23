package relaycmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/metalagman/norma/internal/apps/relay"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed relay.yaml
var defaultRelayConfig []byte

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
			// 1. Load .env file.
			initDotEnv()

			// 2. Get working directory.
			repoRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// 3. Load shared norma config explicitly.
			path := filepath.Join(repoRoot, ".norma", "config.yaml")
			viper.SetConfigFile(path)
			if err := viper.ReadInConfig(); err != nil {
				return fmt.Errorf("reading shared config file %q: %w", path, err)
			}

			// 4. Merge relay-specific config into global viper.
			if err := initConfig(repoRoot); err != nil {
				return fmt.Errorf("initializing relay config: %w", err)
			}

			// 5. Unmarshal shared norma config from the global viper instance.
			var normaCfg config.Config
			if err := viper.Unmarshal(&normaCfg); err != nil {
				return fmt.Errorf("unmarshalling shared config: %w", err)
			}


			// 3. Unmarshal relay app config from the global viper instance.
			var cfg relay.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("unmarshalling relay config: %w", err)
			}

			// Validate required fields.
			if cfg.Relay.Telegram.Token == "" {
				return fmt.Errorf("telegram token is required\nSet it via:\n  - Environment: NORMA_RELAY_TELEGRAM_TOKEN=<token>\n  - Config file: relay.telegram.token in .norma/config.yaml")
			}

			// Normalize agent aliases (opencode_acp -> generic_acp with cmd).
			executablePath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving executable path: %w", err)
			}
			normaCfg, err = config.NormalizeAgentAliases(normaCfg, executablePath)
			if err != nil {
				return fmt.Errorf("normalizing agent aliases: %w", err)
			}

			// Always generate a random owner token (one-time use).
			ownerToken, err := auth.GenerateOwnerToken()
			if err != nil {
				return fmt.Errorf("generating owner token: %w", err)
			}

			log.Info().
				Str("owner_token", ownerToken).
				Str("auth_url", fmt.Sprintf("https://t.me/<bot_username>?start=%s", ownerToken)).
				Msg("Relay bot owner token generated")

			// Set owner token in config.
			cfg.Relay.Auth.OwnerToken = ownerToken

			// Create and run the relay app with norma config for agent info.
			app := relay.App(cfg, normaCfg)

			// Setup signal context for graceful shutdown.
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)

			defer cancel()

			// Start the app.
			if err := app.Start(ctx); err != nil {
				return fmt.Errorf("starting relay app: %w", err)
			}

			log.Info().Msg("Relay bot started. Press Ctrl+C to stop.")

			// Wait for shutdown signal.
			<-ctx.Done()

			// Stop the app.
			if err := app.Stop(context.Background()); err != nil {
				return fmt.Errorf("stopping relay app: %w", err)
			}

			return nil
		},
	}

	return cmd
}


func initDotEnv() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warn().Err(err).Msg("failed to load .env file")
	}
}

// initConfig loads relay-specific configuration layers.
func initConfig(repoRoot string) error {
	// 1. Merge embedded default relay config.
	viper.SetConfigType("yaml")
	if err := viper.MergeConfig(bytes.NewBuffer(defaultRelayConfig)); err != nil {
		return fmt.Errorf("merging default relay config: %w", err)
	}

	// 2. Load .norma/relay.yaml if it exists.
	relayDir := filepath.Join(repoRoot, ".norma")
	relayPath := filepath.Join(relayDir, "relay.yaml")
	if _, err := os.Stat(relayPath); err == nil {
		viper.SetConfigFile(relayPath)
		if err := viper.MergeInConfig(); err != nil {
			return fmt.Errorf("merging relay config: %w", err)
		}
		log.Debug().Str("path", relayPath).Msg("merged relay config")
	}

	return nil
	}


