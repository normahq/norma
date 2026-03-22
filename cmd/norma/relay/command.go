package relaycmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/metalagman/norma/internal/apps/relay"
	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
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
	var token string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start Telegram relay bot",
		Long:  "Start the Telegram relay bot server. A random owner token will be generated and displayed.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Create relay config
			cfg := relay.Config{
				Telegram: relay.TelegramConfig{
					Token: token,
				},
				Auth: relay.AuthConfig{
					OwnerToken: ownerToken,
				},
				Logger: relay.LoggerConfig{
					Level:  "info",
					Pretty: true,
				},
			}

			// Create and run the relay app
			app := relay.App(cfg)

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

	cmd.Flags().StringVar(&token, "token", "", "Telegram bot token (or set NORMA_RELAY_TELEGRAM_TOKEN env var)")

	return cmd
}
