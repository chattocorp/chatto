package core

import (
	"context"
	"strings"
)

// ReactionMutationInput describes one user-facing reaction add/remove
// operation.
type ReactionMutationInput struct {
	ActorID        string
	RoomID         string
	MessageEventID string
	Emoji          string
}

// Reactions returns the operation-level service for user-facing reaction
// mutations. Public transports should authenticate at the edge, pass the actor
// ID here, and let this service own membership and message.react checks.
func (c *ChattoCore) ReactionsService() *ReactionService {
	return c.reactionService
}

// ReactionService owns user-facing reaction mutations. Lower-level ChattoCore
// helpers still perform the event-sourced write and OCC behavior, while this
// service centralizes public API authorization during the GraphQL-to-ConnectRPC
// migration.
type ReactionService struct {
	core *ChattoCore
}

// AddReaction adds actorID's reaction to a message. Authorization: actor must
// be a room member and have message.react in the target room.
func (s *ReactionService) AddReaction(ctx context.Context, input ReactionMutationInput) (bool, error) {
	kind, err := s.authorizeReaction(ctx, input)
	if err != nil {
		return false, err
	}
	return s.core.AddReaction(ctx, kind, input.RoomID, input.MessageEventID, input.Emoji, input.ActorID)
}

// RemoveReaction removes actorID's reaction from a message. Authorization:
// actor must be a room member and have message.react in the target room.
func (s *ReactionService) RemoveReaction(ctx context.Context, input ReactionMutationInput) (bool, error) {
	kind, err := s.authorizeReaction(ctx, input)
	if err != nil {
		return false, err
	}
	return s.core.RemoveReaction(ctx, kind, input.RoomID, input.MessageEventID, input.Emoji, input.ActorID)
}

func (s *ReactionService) authorizeReaction(ctx context.Context, input ReactionMutationInput) (RoomKind, error) {
	if strings.TrimSpace(input.MessageEventID) == "" {
		return KindChannel, invalidArgument("message_event_id is required")
	}
	if strings.TrimSpace(input.Emoji) == "" {
		return KindChannel, invalidArgument("emoji is required")
	}

	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return KindChannel, err
	}

	can, err := s.core.CanReactToMessage(ctx, input.ActorID, kind, room.Id)
	if err != nil {
		return KindChannel, err
	}
	if !can {
		return KindChannel, ErrPermissionDenied
	}
	return kind, nil
}
