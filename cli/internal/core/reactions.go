package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
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

// resolveReactionTarget returns the canonical event ID for storing reactions.
// If the target message is an echo, returns the original thread reply's event ID
// so that reactions are shared between the echo and the original.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) resolveReactionTarget(ctx context.Context, spaceID, roomID, messageEventID string) (string, error) {
	event, err := c.GetRoomEventByEventID(ctx, spaceID, roomID, messageEventID)
	if err != nil {
		return "", fmt.Errorf("failed to look up message: %w", err)
	}
	if event == nil {
		return messageEventID, nil
	}

	if msg := event.GetMessagePosted(); msg != nil && msg.EchoOfEventId != "" {
		return msg.EchoOfEventId, nil
	}

	return messageEventID, nil
}

// AddReaction adds an emoji reaction to a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was added, false if it already existed.
// Publishes a ReactionAddedEvent after successful KV write.
// If the target message is an echo, the reaction is stored against the original message.
func (c *ChattoCore) AddReaction(ctx context.Context, spaceID, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	// Resolve echo → original so reactions are shared
	canonicalEventID, err := c.resolveReactionTarget(ctx, spaceID, roomID, messageEventID)
	if err != nil {
		return false, err
	}

	kv, err := c.getSpaceReactionsKV(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get reactions bucket: %w", err)
	}

	key := reactionKey(canonicalEventID, emojiName, userID)

	// Store timestamp as value for ordering reactions by time added
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, uint64(time.Now().UnixNano()))

	_, err = kv.Create(ctx, key, timestamp)
	if errors.Is(err, jetstream.ErrKeyExists) {
		return false, nil // Already reacted
	}
	if err != nil {
		return false, fmt.Errorf("failed to add reaction: %w", err)
	}

	// Publish event with the canonical (original) event ID so both
	// channel view and thread view can match and refetch reactions.
	c.publishReactionAddedEvent(ctx, spaceID, roomID, canonicalEventID, emojiName, userID)

	c.logger.Debug("Reaction added",
		"space_id", spaceID,
		"room_id", roomID,
		"message_event_id", canonicalEventID,
		"emoji_name", emojiName,
		"user_id", userID,
	)

	return true, nil
}

// RemoveReaction removes an emoji reaction from a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was removed, false if it didn't exist.
// Publishes a ReactionRemovedEvent after successful KV delete.
// If the target message is an echo, the reaction is removed from the original message.
func (c *ChattoCore) RemoveReaction(ctx context.Context, spaceID, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	// Resolve echo → original so reactions are shared
	canonicalEventID, err := c.resolveReactionTarget(ctx, spaceID, roomID, messageEventID)
	if err != nil {
		return false, err
	}

	kv, err := c.getSpaceReactionsKV(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get reactions bucket: %w", err)
	}

	key := reactionKey(canonicalEventID, emojiName, userID)

	// Check if the reaction exists first (KV Delete doesn't error on non-existent keys)
	_, err = kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil // Reaction didn't exist
	}
	if err != nil {
		return false, fmt.Errorf("failed to check reaction: %w", err)
	}

	// Delete the reaction
	err = kv.Delete(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to remove reaction: %w", err)
	}

	// Publish event with the canonical (original) event ID
	c.publishReactionRemovedEvent(ctx, spaceID, roomID, canonicalEventID, emojiName, userID)

	c.logger.Debug("Reaction removed",
		"space_id", spaceID,
		"room_id", roomID,
		"message_event_id", canonicalEventID,
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
func (c *ChattoCore) GetReactions(ctx context.Context, spaceID, messageEventID string) ([]ReactionSummary, error) {
	result, err := c.GetReactionsBatch(ctx, spaceID, []string{messageEventID})
	if err != nil {
		return nil, err
	}
	return result[messageEventID], nil
}

// GetReactionsBatch returns reactions for multiple messages in a single pass.
// Uses ListKeysFiltered with per-message subject filters to avoid scanning the entire bucket.
// Returns a map from messageEventID to sorted ReactionSummary slices.
func (c *ChattoCore) GetReactionsBatch(ctx context.Context, spaceID string, eventIDs []string) (map[string][]ReactionSummary, error) {
	if len(eventIDs) == 0 {
		return make(map[string][]ReactionSummary), nil
	}

	kv, err := c.getSpaceReactionsKV(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reactions bucket: %w", err)
	}

	// Build NATS subject filters for all event IDs (e.g., "eventId1.>", "eventId2.>")
	filters := make([]string, len(eventIDs))
	for i, eventID := range eventIDs {
		filters[i] = eventID + ".>"
	}

	lister, err := kv.ListKeysFiltered(ctx, filters...)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return make(map[string][]ReactionSummary), nil
		}
		return nil, fmt.Errorf("failed to list reaction keys: %w", err)
	}

	// Collect all matching keys
	var matchedKeys []string
	for key := range lister.Keys() {
		matchedKeys = append(matchedKeys, key)
	}

	// Parse keys and fetch timestamps, grouped by event ID
	type emojiData struct {
		userIDs       []string
		earliestNanos uint64
	}
	// eventID -> emojiName -> emojiData
	reactionsByEvent := make(map[string]map[string]*emojiData)

	for _, key := range matchedKeys {
		eventID, emojiName, userID, err := parseReactionKey(key)
		if err != nil {
			c.logger.Warn("Invalid reaction key", "key", key, "error", err)
			continue
		}

		entry, err := kv.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Failed to get reaction entry", "key", key, "error", err)
			continue
		}

		// Parse timestamp from value (8-byte big-endian uint64)
		var timestamp uint64
		if len(entry.Value()) >= 8 {
			timestamp = binary.BigEndian.Uint64(entry.Value())
		}

		if reactionsByEvent[eventID] == nil {
			reactionsByEvent[eventID] = make(map[string]*emojiData)
		}

		data, exists := reactionsByEvent[eventID][emojiName]
		if !exists {
			data = &emojiData{earliestNanos: timestamp}
			reactionsByEvent[eventID][emojiName] = data
		} else if timestamp > 0 && (data.earliestNanos == 0 || timestamp < data.earliestNanos) {
			data.earliestNanos = timestamp
		}
		data.userIDs = append(data.userIDs, userID)
	}

	// Convert to result map with sorted summaries per event
	result := make(map[string][]ReactionSummary, len(reactionsByEvent))
	for eventID, byEmoji := range reactionsByEvent {
		type sortableReaction struct {
			summary       ReactionSummary
			earliestNanos uint64
		}
		sortable := make([]sortableReaction, 0, len(byEmoji))

		for emojiName, data := range byEmoji {
			sortable = append(sortable, sortableReaction{
				summary: ReactionSummary{
					Emoji:   emojiName,
					UserIDs: data.userIDs,
				},
				earliestNanos: data.earliestNanos,
			})
		}

		slices.SortFunc(sortable, func(a, b sortableReaction) int {
			if a.earliestNanos < b.earliestNanos {
				return -1
			}
			if a.earliestNanos > b.earliestNanos {
				return 1
			}
			return strings.Compare(a.summary.Emoji, b.summary.Emoji)
		})

		summaries := make([]ReactionSummary, len(sortable))
		for i, s := range sortable {
			summaries[i] = s.summary
		}
		result[eventID] = summaries
	}

	return result, nil
}

// ============================================================================
// Event Publishing
// ============================================================================

// publishReactionAddedEvent publishes a ReactionAddedEvent directly to the live subject space.
// Reactions are transient UI updates that don't need JetStream storage - the KV bucket is the source of truth.
func (c *ChattoCore) publishReactionAddedEvent(ctx context.Context, spaceID, roomID, messageEventID, emoji, userID string) {
	event := newSpaceEvent(userID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				MessageEventId: messageEventID,
				Emoji:          emoji,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "reaction_added")
	if err := c.publishLiveSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish reaction added event", "error", err)
	}
}

// publishReactionRemovedEvent publishes a ReactionRemovedEvent directly to the live subject space.
// Reactions are transient UI updates that don't need JetStream storage - the KV bucket is the source of truth.
func (c *ChattoCore) publishReactionRemovedEvent(ctx context.Context, spaceID, roomID, messageEventID, emoji, userID string) {
	event := newSpaceEvent(userID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_ReactionRemoved{
			ReactionRemoved: &corev1.ReactionRemovedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				MessageEventId: messageEventID,
				Emoji:          emoji,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "reaction_removed")
	if err := c.publishLiveSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish reaction removed event", "error", err)
	}
}
