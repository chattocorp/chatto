package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Key Helpers
// ============================================================================

// reactionKey returns the KV key for a reaction.
// Pattern: {messageEventId}.{emojiName}.{userId}
// The emoji is stored as its name (e.g., "thumbsup") for NATS KV compatibility.
func reactionKey(messageEventID, emojiName, userID string) string {
	return fmt.Sprintf("%s.%s.%s", messageEventID, emojiName, userID)
}

// reactionKeyPrefix returns the key prefix for all reactions on a message.
// Pattern: {messageEventId}.
func reactionKeyPrefix(messageEventID string) string {
	return fmt.Sprintf("%s.", messageEventID)
}

// parseReactionKey parses a reaction key into its components.
// Returns messageEventID, emojiName, userID, and an error if parsing fails.
func parseReactionKey(key string) (string, string, string, error) {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid reaction key format: %s", key)
	}

	return parts[0], parts[1], parts[2], nil
}

// ============================================================================
// Reactions API
// ============================================================================

// AddReaction adds an emoji reaction to a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was added, false if it already existed.
// Publishes a durable ReactionAddedEvent after successful OCC write.
func (c *ChattoCore) AddReaction(ctx context.Context, kind RoomKind, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	// Block reactions in archived rooms.
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.Archived {
		return false, ErrRoomArchived
	}

	event := newReactionAddedEvent(userID, roomID, messageEventID, emojiName)
	added, err := c.publishReactionMutation(ctx, kind, roomID, messageEventID, emojiName, userID, event)
	if err != nil {
		return false, fmt.Errorf("failed to add reaction: %w", err)
	}
	if !added {
		return false, nil
	}

	c.logger.Debug("Reaction added",
		"kind", kind,
		"room_id", roomID,
		"message_event_id", messageEventID,
		"emoji_name", emojiName,
		"user_id", userID,
	)

	return true, nil
}

// RemoveReaction removes an emoji reaction from a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was removed, false if it didn't exist.
// Publishes a durable ReactionRemovedEvent after successful OCC write.
func (c *ChattoCore) RemoveReaction(ctx context.Context, kind RoomKind, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	event := newReactionRemovedEvent(userID, roomID, messageEventID, emojiName)
	removed, err := c.publishReactionMutation(ctx, kind, roomID, messageEventID, emojiName, userID, event)
	if err != nil {
		return false, fmt.Errorf("failed to remove reaction: %w", err)
	}
	if !removed {
		return false, nil
	}

	c.logger.Debug("Reaction removed",
		"kind", kind,
		"room_id", roomID,
		"message_event_id", messageEventID,
		"emoji_name", emojiName,
		"user_id", userID,
	)

	return true, nil
}

// ReactionSummary represents aggregated reactions for a message.
type ReactionSummary struct {
	Emoji   string
	UserIDs []string
}

// GetReactions returns all reactions for a message, aggregated by emoji shortcode name.
// Returns a slice of ReactionSummary, each containing the shortcode name and list of user IDs.
// Results are ordered by the time each emoji was first added (earliest first).
func (c *ChattoCore) GetReactions(ctx context.Context, messageEventID string) ([]ReactionSummary, error) {
	return c.Reactions.Reactions(messageEventID), nil
}

// GetReactionsBatch returns reactions for multiple messages in a single pass.
// Returns a map from messageEventID to sorted ReactionSummary slices.
func (c *ChattoCore) GetReactionsBatch(ctx context.Context, eventIDs []string) (map[string][]ReactionSummary, error) {
	if len(eventIDs) == 0 {
		return make(map[string][]ReactionSummary), nil
	}
	return c.Reactions.ReactionsBatch(eventIDs), nil
}

// ============================================================================
// Event Publishing
// ============================================================================

const maxReactionMutationRetries = 5

func newReactionAddedEvent(userID, roomID, messageEventID, emoji string) *corev1.Event {
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{
				RoomId:         roomID,
				MessageEventId: messageEventID,
				Emoji:          emoji,
			},
		},
	})
}

func newReactionRemovedEvent(userID, roomID, messageEventID, emoji string) *corev1.Event {
	return newEvent(userID, &corev1.Event{
		Event: &corev1.Event_ReactionRemoved{
			ReactionRemoved: &corev1.ReactionRemovedEvent{
				RoomId:         roomID,
				MessageEventId: messageEventID,
				Emoji:          emoji,
			},
		},
	})
}

func (c *ChattoCore) publishReactionMutation(ctx context.Context, kind RoomKind, roomID, messageEventID, emoji, userID string, event *corev1.Event) (bool, error) {
	add := event.GetReactionAdded() != nil
	remove := event.GetReactionRemoved() != nil
	if !add && !remove {
		return false, fmt.Errorf("unsupported reaction event %T", event.GetEvent())
	}

	agg := events.RoomAggregate(roomID)
	publishSubject := agg.SubjectFor(event)
	occFilter := agg.AllEventsFilter()

	for attempt := 0; attempt < maxReactionMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, occFilter)
		if err != nil {
			return false, fmt.Errorf("read OCC filter seq: %w", err)
		}
		if err := c.ReactionsProjector.WaitForSeq(ctx, filterSeq); err != nil {
			return false, fmt.Errorf("wait for reactions projection: %w", err)
		}

		exists := c.Reactions.HasReaction(messageEventID, emoji, userID)
		if add && exists {
			return false, nil
		}
		if remove && !exists {
			return false, nil
		}

		seq, err := c.EventPublisher.AppendAtFilter(ctx, publishSubject, event, occFilter, filterSeq)
		if err == nil {
			if err := c.ReactionsProjector.WaitForSeq(ctx, seq); err != nil {
				return false, fmt.Errorf("wait for reactions projection: %w", err)
			}
			return true, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return false, err
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return false, fmt.Errorf("reaction OCC retry exhausted after %d attempts: %w", maxReactionMutationRetries, events.ErrConflict)
}
