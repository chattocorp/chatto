package graph

//go:generate go run github.com/99designs/gqlgen generate --verbose

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	"hmans.de/chatto/internal/graph/dataloader"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	core          *core.ChattoCore
	ownersConfig  config.OwnersConfig
	authConfig    config.AuthConfig
	pushConfig    config.PushConfig
	videoConfig   config.VideoConfig
	livekitConfig config.LiveKitConfig
	logger        *log.Logger
	version       string
}

func NewResolver(core *core.ChattoCore, ownersConfig config.OwnersConfig, authConfig config.AuthConfig, pushConfig config.PushConfig, videoConfig config.VideoConfig, livekitConfig config.LiveKitConfig, version string) *Resolver {
	logger := log.WithPrefix("graph.Resolver")

	return &Resolver{
		core:          core,
		ownersConfig:  ownersConfig,
		authConfig:    authConfig,
		pushConfig:    pushConfig,
		videoConfig:   videoConfig,
		livekitConfig: livekitConfig,
		logger:        logger,
		version:       version,
	}
}

// getUser loads a user by ID, using dataloaders if available,
// falling back to direct core call otherwise.
func (r *Resolver) getUser(ctx context.Context, userID string) (*corev1.User, error) {
	if loaders := dataloader.ForContext(ctx); loaders != nil {
		return loaders.GetUser(ctx, userID)
	}
	// Fallback for tests or contexts without dataloaders
	return r.core.GetUser(ctx, userID)
}

// getReactions loads reactions for a message, using the batch dataloader if available.
func (r *Resolver) getReactions(ctx context.Context, eventID string) ([]core.ReactionSummary, error) {
	if loaders := dataloader.ForContext(ctx); loaders != nil {
		return loaders.GetReactions(ctx, eventID)
	}
	return r.core.GetReactions(ctx, eventID)
}

// resolveReactions converts core ReactionSummaries to GraphQL Reactions for the current user.
// Shared by messagePostedEventResolver and messageUpdatedEventResolver.
func (r *Resolver) resolveReactions(ctx context.Context, eventID string) ([]*model.Reaction, error) {
	currentUser := auth.ForContext(ctx)
	if currentUser == nil {
		return []*model.Reaction{}, nil
	}

	summaries, err := r.getReactions(ctx, eventID)
	if err != nil {
		return nil, err
	}

	reactions := make([]*model.Reaction, 0, len(summaries))
	for _, summary := range summaries {
		users := make([]*corev1.User, 0, len(summary.UserIDs))
		hasReacted := false
		for _, userID := range summary.UserIDs {
			if userID == currentUser.Id {
				hasReacted = true
			}
			user, err := r.getUser(ctx, userID)
			if err != nil {
				continue // Skip deleted users
			}
			users = append(users, user)
		}

		reactions = append(reactions, &model.Reaction{
			Emoji:      summary.Emoji,
			Count:      int32(len(users)),
			Users:      users,
			HasReacted: hasReacted,
		})
	}

	return reactions, nil
}

// getMessageBody loads a message body, using per-request caching if available.
// This prevents redundant KV lookups when Body, Attachments, and UpdatedAt
// resolvers all need the same MessageBody for a single message.
func (r *Resolver) getMessageBody(ctx context.Context, spaceID, messageBodyKey string) (*core.DecryptedMessageBody, error) {
	kind := core.KindForSpace(spaceID)
	if loaders := dataloader.ForContext(ctx); loaders != nil {
		return loaders.GetMessageBody(ctx, kind, messageBodyKey)
	}
	// Fallback for tests or contexts without dataloaders
	return r.core.GetFullMessageBody(ctx, kind, messageBodyKey)
}

// resolveMessageBodyKey constructs the internal message body key from an event ID.
// Body keys have the format {userId}.{eventId}. For author operations (edit own,
// delete own), the caller's user ID is the key prefix. For moderator operations,
// we fall back to looking up the event to find the original author.
func (r *Resolver) resolveMessageBodyKey(ctx context.Context, kind core.RoomKind, roomID, eventID string) (string, error) {
	user := auth.ForContext(ctx)
	if user == nil {
		return "", fmt.Errorf("authentication required")
	}

	// Try constructing the body key using the caller's user ID.
	// This is the fast path for author operations (the common case).
	candidateKey := user.Id + "." + eventID
	body, err := r.core.GetFullMessageBody(ctx, kind, candidateKey)
	if err != nil {
		return "", fmt.Errorf("failed to look up message body: %w", err)
	}
	if body != nil {
		return candidateKey, nil
	}

	// Slow path: look up the event to find the actual author's body key.
	// Needed when the caller is not the author (moderator edit/delete).
	event, err := r.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return "", fmt.Errorf("failed to look up event: %w", err)
	}
	if event == nil {
		return "", core.ErrMessageNotFound
	}

	if msg := event.GetMessagePosted(); msg != nil {
		return msg.MessageBodyId, nil
	}

	return "", fmt.Errorf("event %s is not a message event", eventID)
}
