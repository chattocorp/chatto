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

// ReactionModel returns the operation-level model for user-facing reaction
// mutations. Public transports should authenticate at the edge, pass the actor
// ID here, and let this model own membership, applicable channel-room
// message.read, and message.react checks.
func (c *ChattoCore) ReactionModel() *ReactionModel {
	return c.reactionModel
}

// ReactionModel owns user-facing reaction authorization, event-sourced writes,
// OCC retries, and projection readiness.
type ReactionModel struct {
	core      *ChattoCore
	mutations reactionMutationExecutor
}

// AddReaction adds actorID's reaction to a message. Authorization: actor must
// be a room member and have message.react. Channel-room reactions also require
// message.read; DM membership authorizes the DM read.
func (s *ReactionModel) AddReaction(ctx context.Context, input ReactionMutationInput) (bool, error) {
	return s.mutateAuthorizedReaction(ctx, input, true)
}

// RemoveReaction removes actorID's reaction from a message. Authorization:
// actor must be a room member and have message.react. Channel-room reactions
// also require message.read; DM membership authorizes the DM read.
func (s *ReactionModel) RemoveReaction(ctx context.Context, input ReactionMutationInput) (bool, error) {
	return s.mutateAuthorizedReaction(ctx, input, false)
}

func (s *ReactionModel) authorizeReaction(ctx context.Context, input ReactionMutationInput) (RoomKind, error) {
	if err := validateReactionMutationInput(input); err != nil {
		return KindChannel, err
	}

	room, kind, err := s.core.requireRoomMessageReader(ctx, input.ActorID, input.RoomID)
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

func validateReactionMutationInput(input ReactionMutationInput) error {
	if strings.TrimSpace(input.MessageEventID) == "" {
		return invalidArgument("message_event_id is required")
	}
	if strings.TrimSpace(input.Emoji) == "" {
		return invalidArgument("emoji is required")
	}
	return nil
}
