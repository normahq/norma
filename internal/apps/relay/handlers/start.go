package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
)

// StartHandler handles /start command for owner authentication.
type StartHandler struct {
	ownerStore   *auth.OwnerStore
	tgClient     client.ClientWithResponsesInterface
	authToken    string
	relayHandler *RelayHandler
}

// NewStartHandler creates a new start handler.
func NewStartHandler(params StartHandlerParams) *StartHandler {
	return &StartHandler{
		ownerStore: params.OwnerStore,
		tgClient:   params.TgClient,
		authToken:  params.Auth.AuthToken,
	}
}

// Register registers the handler with the registry.
func (h *StartHandler) Register(registry handlers.RegistryInterface) {
	registry.OnCommand(h.onCommand)
}

// SetRelayHandler sets the relay handler for owner activation.
func (h *StartHandler) SetRelayHandler(relayHandler *RelayHandler) {
	h.relayHandler = relayHandler
}

func (h *StartHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	if event.Command != "start" {
		return nil
	}

	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id

	authToken := parseAuthToken(event.Args)

	log.Debug().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Str("token", authToken).
		Msg("Start command received")

	// Check if owner is already registered
	if h.ownerStore.HasOwner() {
		if h.ownerStore.IsOwner(userID) {
			// Re-activate relay for existing owner
			if h.relayHandler != nil {
				h.relayHandler.SetOwner(ctx, userID, chatID)
				log.Info().Int64("user_id", userID).Msg("Relay re-activated for existing owner")
			}
			return h.sendMessage(ctx, chatID, "You are already registered as the bot owner. Relay mode is active.")
		}
		return h.sendMessage(ctx, chatID, "Bot owner is already registered. Only the owner can use this bot.")
	}

	// If no auth token provided, show welcome message
	if authToken == "" {
		return h.sendWelcomeMessage(ctx, chatID)
	}

	// Validate auth token
	if authToken != h.authToken {
		log.Warn().Str("provided_token", authToken).Msg("Invalid auth token provided")
		return h.sendMessage(ctx, chatID, "Invalid authentication token. Please try again.")
	}

	// Extract user info
	userInfo := extractUserInfo(event.Message.From)

	// Check if the chat supports topics (forum supergroup).
	var hasTopicsEnabled bool
	if event.Message.Chat.IsForum != nil {
		hasTopicsEnabled = *event.Message.Chat.IsForum
	}

	// Register user as owner.
	registered, err := h.ownerStore.RegisterOwner(userID, userInfo.username, userInfo.firstName, userInfo.lastName, hasTopicsEnabled)
	if err != nil {
		log.Error().Err(err).Int64("user_id", userID).Msg("Failed to register owner")
		return h.sendMessage(ctx, chatID, "Failed to register owner. Please try again.")
	}

	if !registered {
		return h.sendMessage(ctx, chatID, "Owner is already registered.")
	}

	log.Info().
		Int64("user_id", userID).
		Str("username", userInfo.username).
		Str("first_name", userInfo.firstName).
		Msg("Owner registered successfully")

	return h.sendOwnerRegisteredMessage(ctx, userID, chatID, userInfo.firstName)
}

func parseAuthToken(args string) string {
	// Deep link format: /start <payload> - args is the payload directly.
	return strings.TrimSpace(args)
}

type userInfo struct {
	username  string
	firstName string
	lastName  string
}

func extractUserInfo(from *client.User) userInfo {
	info := userInfo{
		firstName: from.FirstName,
	}
	if from.Username != nil {
		info.username = *from.Username
	}
	if from.LastName != nil {
		info.lastName = *from.LastName
	}
	return info
}

func (h *StartHandler) sendWelcomeMessage(ctx context.Context, chatID int64) error {
	text := `Welcome to Norma Relay Bot!

This bot allows you to interact with norma agent workflows via Telegram.

To authenticate as the bot owner, send:
/start <your_owner_token>

Or open: https://t.me/<bot_username>?start=<owner_token>`
	return h.sendMessage(ctx, chatID, text)
}

func (h *StartHandler) sendOwnerRegisteredMessage(ctx context.Context, ownerID, chatID int64, firstName string) error {
	name := firstName
	if name == "" {
		name = "Owner"
	}

	log.Info().
		Int64("owner_id", ownerID).
		Int64("chat_id", chatID).
		Msg("Setting owner on relay handler")

	// Activate relay mode.
	if h.relayHandler != nil {
		h.relayHandler.SetOwner(ctx, ownerID, chatID)
		log.Info().Msg("Relay handler SetOwner called successfully")
	} else {
		log.Error().Msg("relayHandler is nil, cannot set owner")
	}

	text := fmt.Sprintf("Congratulations, %s! You are now registered as the bot owner.\n\nRelay mode is active. Send me messages and I will forward them to the agent.", name)

	return h.sendMessage(ctx, chatID, text)
}

func (h *StartHandler) sendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := h.tgClient.SendMessageWithResponse(ctx, client.SendMessageJSONRequestBody{
		ChatId: chatID,
		Text:   text,
	})
	if err != nil {
		return fmt.Errorf("sending message to chat %d: %w", chatID, err)
	}
	return nil
}
