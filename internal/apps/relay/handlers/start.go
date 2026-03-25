package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/metalagman/norma/internal/apps/relay/auth"
	"github.com/metalagman/norma/internal/apps/relay/messenger"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/client"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
)

// StartHandler handles /start command for owner authentication.
type StartHandler struct {
	ownerStore   *auth.OwnerStore
	messenger    *messenger.Messenger
	authToken    string
	relayHandler *RelayHandler
}

// StartHandlerParams provides dependencies for StartHandler.
type StartHandlerParams struct {
	fx.In

	OwnerStore *auth.OwnerStore
	Messenger  *messenger.Messenger
	AuthToken  string `name:"relay_auth_token"`
}

// NewStartHandler creates a new start handler.
func NewStartHandler(params StartHandlerParams) *StartHandler {
	return &StartHandler{
		ownerStore: params.OwnerStore,
		messenger:  params.Messenger,
		authToken:  params.AuthToken,
	}
}

// SetRelayHandler sets the relay handler (needed for circular dependency).
func (h *StartHandler) SetRelayHandler(rh *RelayHandler) {
	h.relayHandler = rh
}

// Register registers the handler with the registry.
func (h *StartHandler) Register(registry handlers.RegistryInterface) {
	registry.OnCommand(h.onCommand)
}

func (h *StartHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	if event.Command != "start" {
		return nil
	}

	if event.Message.Chat.Type != "private" {
		return nil
	}

	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id
	authToken := strings.TrimSpace(event.Args)

	log.Debug().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Msg("Start command received")

	if h.ownerStore.HasOwner() {
		if h.ownerStore.IsOwner(userID) {
			if h.relayHandler != nil {
				h.relayHandler.SetOwner(userID, chatID)
				log.Info().Int64("user_id", userID).Msg("Relay re-activated for existing owner")
			}
			if err := h.messenger.SendPlain(ctx, chatID, "You are already registered as the bot owner. Relay mode is active.", 0); err != nil {
				return err
			}
			return nil
		}
		if err := h.messenger.SendPlain(ctx, chatID, "Bot owner is already registered. Only the owner can use this bot.", 0); err != nil {
			return err
		}
		return nil
	}

	if authToken == "" {
		if err := h.sendWelcomeMessage(ctx, chatID); err != nil {
			return err
		}
		return nil
	}

	if authToken != h.authToken {
		log.Warn().Msg("Invalid auth token provided")
		if err := h.messenger.SendPlain(ctx, chatID, "Invalid authentication token. Please try again.", 0); err != nil {
			return err
		}
		return nil
	}

	info := extractUserInfo(event.Message.From)

	var hasTopicsEnabled bool
	if event.Message.Chat.IsForum != nil {
		hasTopicsEnabled = *event.Message.Chat.IsForum
	}

	registered, err := h.ownerStore.RegisterOwner(userID, info.username, info.firstName, info.lastName, hasTopicsEnabled)
	if err != nil {
		log.Error().Err(err).Int64("user_id", userID).Msg("Failed to register owner")
		if sendErr := h.messenger.SendPlain(ctx, chatID, "Failed to register owner. Please try again.", 0); sendErr != nil {
			return sendErr
		}
		return nil
	}

	if !registered {
		if err := h.messenger.SendPlain(ctx, chatID, "Owner is already registered.", 0); err != nil {
			return err
		}
		return nil
	}

	log.Info().
		Int64("user_id", userID).
		Str("username", info.username).
		Msg("Owner registered successfully")

	if err := h.sendOwnerRegisteredMessage(ctx, userID, chatID, info.firstName); err != nil {
		return err
	}
	return nil
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
	return h.messenger.SendPlain(ctx, chatID, "Welcome to Norma Relay Bot!\n\nTo authenticate, send /start <your_owner_token>", 0)
}

func (h *StartHandler) sendOwnerRegisteredMessage(ctx context.Context, ownerID, chatID int64, firstName string) error {
	name := firstName
	if name == "" {
		name = "Owner"
	}

	if h.relayHandler != nil {
		h.relayHandler.SetOwner(ownerID, chatID)
	} else {
		log.Error().Msg("relayHandler is nil, cannot set owner")
	}

	text := fmt.Sprintf("Congratulations, %s! You are now registered as the bot owner.\n\nRelay mode is active.", name)
	return h.messenger.SendPlain(ctx, chatID, text, 0)
}
