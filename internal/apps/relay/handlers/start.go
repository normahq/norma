package handlers

import (
	"context"
	"strings"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
)

// StartHandler handles /start command for owner authentication.
type StartHandler struct {
	ownerStore *auth.OwnerStore
	tgClient   client.ClientWithResponsesInterface
	authToken  string
}

// NewStartHandler creates a new start handler.
func NewStartHandler(ownerStore *auth.OwnerStore, tgClient client.ClientWithResponsesInterface, authToken string) *StartHandler {
	return &StartHandler{
		ownerStore: ownerStore,
		tgClient:   tgClient,
		authToken:  authToken,
	}
}

// Register registers the handler with the registry.
func (h *StartHandler) Register(registry handlers.RegistryInterface) {
	registry.OnCommand(h.onCommand)
}

func (h *StartHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	if event.Command != "start" {
		return nil
	}

	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id
	firstName := event.Message.From.FirstName
	lastName := ""
	if event.Message.From.LastName != nil {
		lastName = *event.Message.From.LastName
	}
	username := ""
	if event.Message.From.Username != nil {
		username = *event.Message.From.Username
	}

	// Parse auth token from args (e.g., /start auth=xxx)
	args := event.Args
	authToken := ""
	if args != "" {
		parts := strings.Fields(args)
		for _, part := range parts {
			if strings.HasPrefix(part, "auth=") {
				authToken = strings.TrimPrefix(part, "auth=")
				break
			}
		}
	}

	// Check if owner is already registered
	if h.ownerStore.HasOwner() {
		if h.ownerStore.IsOwner(userID) {
			return h.sendMessage(chatID, "You are already registered as the bot owner.")
		}
		return h.sendMessage(chatID, "Bot owner is already registered. Only the owner can use this bot.")
	}

	// If no auth token provided, show welcome message
	if authToken == "" {
		return h.sendWelcomeMessage(chatID)
	}

	// Validate auth token
	if authToken != h.authToken {
		log.Warn().Str("provided_token", authToken).Msg("Invalid auth token provided")
		return h.sendMessage(chatID, "Invalid authentication token. Please try again.")
	}

	// Register user as owner
	registered, err := h.ownerStore.RegisterOwner(userID, username, firstName, lastName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to register owner")
		return h.sendMessage(chatID, "Failed to register owner. Please try again.")
	}

	if !registered {
		return h.sendMessage(chatID, "Owner is already registered.")
	}

	log.Info().
		Int64("user_id", userID).
		Str("username", username).
		Str("first_name", firstName).
		Msg("Owner registered successfully")

	return h.sendOwnerRegisteredMessage(chatID, firstName)
}

func (h *StartHandler) sendWelcomeMessage(chatID int64) error {
	text := `Welcome to Norma Relay Bot!

This bot allows you to interact with norma agent workflows via Telegram.

To authenticate as the bot owner, use:
/start auth=<your_owner_token>

Type /help to see available commands.`
	return h.sendMessage(chatID, text)
}

func (h *StartHandler) sendOwnerRegisteredMessage(chatID int64, firstName string) error {
	name := firstName
	if name == "" {
		name = "Owner"
	}

	text := "Congratulations, " + name + "! You are now registered as the bot owner.\n\n"
	text += "Available commands:\n"
	text += "/help - Show available commands\n"
	text += "/status - Show current norma runs\n"
	text += "/run <task-id> - Start a norma run\n"
	text += "/stop <run-id> - Stop a running norma run\n"
	text += "/logs <run-id> [step] - View run logs"

	return h.sendMessage(chatID, text)
}

func (h *StartHandler) sendMessage(chatID int64, text string) error {
	_, err := h.tgClient.SendMessageWithResponse(context.Background(), client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	return err
}
