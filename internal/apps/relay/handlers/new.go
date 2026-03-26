package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/normahq/norma/internal/apps/relay/auth"
	"github.com/normahq/norma/internal/apps/relay/messenger"
	"github.com/normahq/norma/internal/apps/relay/session"
	"github.com/rs/zerolog/log"
	"github.com/tgbotkit/runtime/events"
	"github.com/tgbotkit/runtime/handlers"
	"go.uber.org/fx"
)

// CommandHandler handles /new command to create topic agent sessions.
type CommandHandler struct {
	ownerStore     *auth.OwnerStore
	sessionManager *session.Manager
	messenger      *messenger.Messenger
}

type commandHandlerParams struct {
	fx.In

	OwnerStore     *auth.OwnerStore
	SessionManager *session.Manager
	Messenger      *messenger.Messenger
}

// NewCommandHandler creates a new /new command handler.
func NewCommandHandler(params commandHandlerParams) *CommandHandler {
	return &CommandHandler{
		ownerStore:     params.OwnerStore,
		sessionManager: params.SessionManager,
		messenger:      params.Messenger,
	}
}

// Register registers the handler with the registry.
func (h *CommandHandler) Register(registry handlers.RegistryInterface) {
	registry.OnCommand(h.onCommand)
}

func (h *CommandHandler) onCommand(ctx context.Context, event *events.CommandEvent) error {
	if event.Command != "new" {
		return nil
	}

	chatID := event.Message.Chat.Id
	userID := event.Message.From.Id

	if !h.ownerStore.HasOwner() || !h.ownerStore.IsOwner(userID) {
		if err := h.messenger.SendPlain(ctx, chatID, "Only the bot owner can use this command.", 0); err != nil {
			return err
		}
		return nil
	}

	agentName := strings.TrimSpace(event.Args)
	if agentName == "" {
		if err := h.messenger.SendPlain(ctx, chatID, "Usage: /new <agent_name>\n\nAvailable agents: gemini_agent, opencode_agent, etc.", 0); err != nil {
			return err
		}
		return nil
	}

	log.Info().
		Int64("user_id", userID).
		Int64("chat_id", chatID).
		Str("agent", agentName).
		Msg("Creating new topic with agent")

	sessionID, topicID, err := h.sessionManager.CreateTopicSession(ctx, chatID, agentName)
	if err != nil {
		log.Error().Err(err).Str("agent", agentName).Msg("Failed to create topic with agent")
		if sendErr := h.messenger.SendPlain(ctx, chatID, fmt.Sprintf("Failed to create agent session: %v", err), 0); sendErr != nil {
			return sendErr
		}
		return nil
	}

	welcomeMsg := fmt.Sprintf("🤖 Started new **%s** agent session (%s).", agentName, sessionID)
	if err := h.messenger.SendMarkdown(ctx, chatID, welcomeMsg, topicID); err != nil {
		log.Error().Err(err).Msg("Failed to send welcome message")
		return err
	}

	return nil
}
