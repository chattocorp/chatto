package graph

//go:generate go run github.com/99designs/gqlgen generate --verbose

import (
	"context"
	"errors"
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

// resolveOptionalUser is a tolerant variant of getUser for resolvers backing
// nullable `actor` / `sender` / `target` fields: a deleted user returns
// (nil, nil) instead of (nil, ErrNotFound), so the field resolves to null
// without erroring up and blanking a non-null enclosing list. Reserve true
// errors for infrastructure faults (KV unreachable, decode failure, etc.) —
// see graphql.md "Nullability Must Match Resolver Failure Mode."
func (r *Resolver) resolveOptionalUser(ctx context.Context, userID string) (*corev1.User, error) {
	if userID == "" {
		return nil, nil
	}
	user, err := r.getUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// getReactions loads reactions for a message, using the batch dataloader if available.
func (r *Resolver) getReactions(ctx context.Context, eventID string) ([]core.ReactionSummary, error) {
	if loaders := dataloader.ForContext(ctx); loaders != nil {
		return loaders.GetReactions(ctx, eventID)
	}
	return r.core.GetReactions(ctx, eventID)
}

// resolveReactions returns Core reaction summaries for the GraphQL ReactionSummary type.
// Derived fields such as count, users(first:), and hasReacted are resolved on
// ReactionSummary itself so the API shape can stay preview-oriented without a wrapper model.
func (r *Resolver) resolveReactions(ctx context.Context, eventID string) ([]*core.ReactionSummary, error) {
	currentUser := auth.ForContext(ctx)
	if currentUser == nil {
		return []*core.ReactionSummary{}, nil
	}

	summaries, err := r.getReactions(ctx, eventID)
	if err != nil {
		return nil, err
	}

	reactions := make([]*core.ReactionSummary, 0, len(summaries))
	for _, summary := range summaries {
		reactions = append(reactions, &core.ReactionSummary{
			Emoji:   summary.Emoji,
			UserIDs: append([]string(nil), summary.UserIDs...),
		})
	}

	return reactions, nil
}

func messagePostedPayload(obj *model.MessagePostedEvent) *corev1.MessagePostedEvent {
	if obj == nil {
		return nil
	}
	return obj.Payload
}

// messagePostedEventID returns the durable message event ID from the GraphQL
// event wrapper. MessagePostedEvent itself is payload-only.
func messagePostedEventID(obj *model.MessagePostedEvent) string {
	if obj == nil || obj.Envelope == nil {
		return ""
	}
	return obj.Envelope.GetId()
}

// bodyKeyForLookup uses the canonical envelope event ID. The core body lookup
// still accepts this through its legacy "body key" parameter name.
func bodyKeyForLookup(obj *model.MessagePostedEvent) string {
	return messagePostedEventID(obj)
}

// getMessageBody loads a message body, using per-request caching if available.
// This prevents redundant KV lookups when Body, Attachments, and UpdatedAt
// resolvers all need the same MessageBody for a single message.
func (r *Resolver) getMessageBody(ctx context.Context, kind core.RoomKind, messageBodyKey string) (*core.DecryptedMessageBody, error) {
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
		return event.GetId(), nil
	}

	return "", fmt.Errorf("event %s is not a message event", eventID)
}
