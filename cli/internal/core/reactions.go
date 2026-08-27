package core

import (
	"context"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
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

// addReaction adds an emoji reaction to a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was added, false if it already existed.
// Publishes a durable ReactionAddedEvent after successful OCC write.
func (s *ReactionModel) addReaction(ctx context.Context, kind RoomKind, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	// Block reactions in archived rooms.
	room, err := s.core.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.Archived {
		return false, ErrRoomArchived
	}

	messageEventID, err = s.core.canonicalReactionMessageEventID(roomID, messageEventID)
	if err != nil {
		return false, err
	}
	target, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, messageEventID)
	if err != nil {
		return false, fmt.Errorf("resolve reaction notification target: %w", err)
	}
	if target == nil {
		return false, ErrNotFound
	}
	event := newReactionAddedEvent(userID, roomID, messageEventID, emojiName)
	added, sequence, err := s.publishReactionMutation(ctx, kind, roomID, messageEventID, emojiName, userID, event)
	if err != nil {
		return false, fmt.Errorf("failed to add reaction: %w", err)
	}
	if !added {
		return false, nil
	}
	if err := s.core.notificationMaterializer.WaitThrough(ctx, sequence); err != nil {
		s.core.logger.Warn("Notification materialization did not reach the committed reaction before the request completed",
			"room_id", roomID, "message_event_id", messageEventID, "error", err)
	}
	s.core.logger.Debug("Reaction added",
		"kind", kind,
		"room_id", roomID,
		"message_event_id", messageEventID,
		"emoji_name", emojiName,
		"user_id", userID,
	)

	return true, nil
}

// removeReaction removes an emoji reaction from a message.
// Accepts an emoji shortcode name (e.g., "thumbsup", "heart").
// Returns true if the reaction was removed, false if it didn't exist.
// Publishes a durable ReactionRemovedEvent after successful OCC write.
func (s *ReactionModel) removeReaction(ctx context.Context, kind RoomKind, roomID, messageEventID, emojiInput, userID string) (bool, error) {
	emojiName, err := resolveEmojiInput(emojiInput)
	if err != nil {
		return false, err
	}

	messageEventID, err = s.core.canonicalReactionMessageEventID(roomID, messageEventID)
	if err != nil {
		return false, err
	}
	target, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, messageEventID)
	if err != nil {
		return false, fmt.Errorf("resolve reaction notification target: %w", err)
	}
	if target == nil {
		return false, ErrNotFound
	}
	event := newReactionRemovedEvent(userID, roomID, messageEventID, emojiName)
	removed, sequence, err := s.publishReactionMutation(ctx, kind, roomID, messageEventID, emojiName, userID, event)
	if err != nil {
		return false, fmt.Errorf("failed to remove reaction: %w", err)
	}
	if !removed {
		return false, nil
	}
	if err := s.core.notificationMaterializer.WaitThrough(ctx, sequence); err != nil {
		s.core.logger.Warn("Notification cleanup did not reach the committed reaction removal before the request completed",
			"room_id", roomID, "message_event_id", messageEventID, "error", err)
	}
	s.core.logger.Debug("Reaction removed",
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
	messageEventID, _ = c.canonicalReactionMessageEventID("", messageEventID)
	return c.roomModel.reactionsForMessage(messageEventID), nil
}

// GetReactionsBatch returns reactions for multiple messages in a single pass.
// Returns a map from messageEventID to sorted ReactionSummary slices.
func (c *ChattoCore) GetReactionsBatch(ctx context.Context, eventIDs []string) (map[string][]ReactionSummary, error) {
	if len(eventIDs) == 0 {
		return make(map[string][]ReactionSummary), nil
	}
	canonicalEventIDs := make([]string, 0, len(eventIDs))
	requestedByCanonical := make(map[string][]string, len(eventIDs))
	for _, eventID := range eventIDs {
		canonicalID, _ := c.canonicalReactionMessageEventID("", eventID)
		canonicalEventIDs = append(canonicalEventIDs, canonicalID)
		requestedByCanonical[canonicalID] = append(requestedByCanonical[canonicalID], eventID)
	}
	canonicalReactions := c.roomModel.reactionsBatch(canonicalEventIDs)
	result := make(map[string][]ReactionSummary, len(canonicalReactions))
	for canonicalID, summaries := range canonicalReactions {
		for _, requestedID := range requestedByCanonical[canonicalID] {
			result[requestedID] = summaries
		}
	}
	return result, nil
}

// CanonicalReactionMessageEventID returns the original thread reply event ID
// when messageEventID identifies a channel echo. Unknown IDs are returned
// unchanged so legacy callers get the same behavior as direct projection reads.
func (c *ChattoCore) CanonicalReactionMessageEventID(roomID, messageEventID string) string {
	canonicalID, err := c.canonicalReactionMessageEventID(roomID, messageEventID)
	if err != nil {
		return messageEventID
	}
	return canonicalID
}

// ChannelEchoEventID returns the visible room-timeline echo for an original
// thread reply. The boolean is false when the reply is not currently echoed.
func (c *ChattoCore) ChannelEchoEventID(messageEventID string) (string, bool) {
	if c == nil || !c.roomModel.hasTimeline() {
		return "", false
	}
	return c.roomModel.channelEchoEventID(messageEventID)
}

// LinkedChannelEchoEventID returns a linked non-hidden echo even after the
// canonical reply retraction has turned that echo into a tombstone.
func (c *ChattoCore) LinkedChannelEchoEventID(messageEventID string) (string, bool) {
	if c == nil || !c.roomModel.hasTimeline() {
		return "", false
	}
	return c.roomModel.linkedChannelEchoEventID(messageEventID)
}

// IsHiddenChannelEcho reports whether an echo row was directly retracted while
// its canonical thread reply remains visible. Such rows disappear from the
// room projection instead of rendering as deleted-message tombstones.
func (c *ChattoCore) IsHiddenChannelEcho(messageEventID string) bool {
	return c != nil && c.roomModel.hasTimeline() && c.roomModel.isHiddenEcho(messageEventID)
}

func (c *ChattoCore) canonicalReactionMessageEventID(roomID, messageEventID string) (string, error) {
	if strings.TrimSpace(messageEventID) == "" {
		return messageEventID, nil
	}
	if c == nil || !c.roomModel.hasTimeline() {
		return messageEventID, nil
	}
	entry, ok := c.roomModel.timelineEntry(messageEventID)
	if !ok || entry == nil || entry.Event == nil {
		return messageEventID, nil
	}
	if roomID != "" && roomIDOfEvent(entry.Event) != roomID {
		return "", ErrMessageNotFound
	}
	posted := entry.Event.GetMessagePosted()
	if posted == nil || posted.GetEchoOfEventId() == "" {
		return messageEventID, nil
	}
	originalID := posted.GetEchoOfEventId()
	if roomID != "" {
		if originalEntry, ok := c.roomModel.timelineEntry(originalID); ok && originalEntry != nil && originalEntry.Event != nil && roomIDOfEvent(originalEntry.Event) != roomID {
			return "", ErrMessageNotFound
		}
	}
	return originalID, nil
}

// ============================================================================
// Event Publishing
// ============================================================================

type reactionMutationExecutor interface {
	ExecuteMutation(
		context.Context,
		events.MutationBoundary,
		func(context.Context, events.MutationAttempt) ([]evtstream.MutationEntry, error),
	) (events.MutationResult, error)
}

func (s *ReactionModel) executeMutation(
	ctx context.Context,
	boundary events.MutationBoundary,
	decide func(context.Context, events.MutationAttempt) ([]evtstream.MutationEntry, error),
) (events.MutationResult, error) {
	executor := s.mutations
	if executor == nil {
		executor = s.core.EventPublisher
	}
	return executor.ExecuteMutation(ctx, boundary, decide)
}

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

// mutateAuthorizedReaction evaluates membership, message.react, room state,
// message identity, and reaction limits inside the room aggregate's mutation
// boundary. A concurrent room change makes JetStream reject the attempt and
// rerun the complete decision. Authorization from other aggregates is checked
// at request time and does not retroactively cancel a conflict-free attempt.
func (s *ReactionModel) mutateAuthorizedReaction(ctx context.Context, input ReactionMutationInput, add bool) (bool, error) {
	if err := validateReactionMutationInput(input); err != nil {
		return false, err
	}
	emojiName, err := resolveEmojiInput(input.Emoji)
	if err != nil {
		return false, err
	}

	var event *corev1.Event
	if add {
		event = newReactionAddedEvent(input.ActorID, input.RoomID, input.MessageEventID, emojiName)
	} else {
		event = newReactionRemovedEvent(input.ActorID, input.RoomID, input.MessageEventID, emojiName)
	}
	agg := evtstream.RoomAggregate(input.RoomID)
	publishSubject := agg.SubjectFor(event)
	committedKind := KindChannel
	committedMessageEventID := input.MessageEventID
	result, err := s.executeMutation(ctx, events.AtSubject(agg.AllEventsFilter()), func(ctx context.Context, _ events.MutationAttempt) ([]evtstream.MutationEntry, error) {
		kind, err := s.prepareAuthorizedReactionAttempt(ctx, input)
		if err != nil {
			return nil, err
		}
		if add {
			room, err := s.core.GetRoom(ctx, kind, input.RoomID)
			if err != nil {
				return nil, err
			}
			if room.Archived {
				return nil, ErrRoomArchived
			}
		}

		messageEventID, err := s.core.canonicalReactionMessageEventID(input.RoomID, input.MessageEventID)
		if err != nil {
			return nil, err
		}
		if reaction := event.GetReactionAdded(); reaction != nil {
			reaction.MessageEventId = messageEventID
		} else {
			event.GetReactionRemoved().MessageEventId = messageEventID
		}

		snapshot := s.core.roomModel.reactionMutationSnapshot(input.RoomID, messageEventID, emojiName, input.ActorID)
		if add {
			if snapshot.Exists {
				return nil, nil
			}
			if snapshot.UserReactionCount >= MaxReactionsPerUserPerMessage {
				return nil, ErrReactionLimitExceeded
			}
		} else if !snapshot.Exists {
			return nil, nil
		}

		committedKind = kind
		committedMessageEventID = messageEventID
		entries := []evtstream.MutationEntry{{Subject: publishSubject, Event: event}}
		target, err := s.core.GetRoomEventByEventID(ctx, kind, input.RoomID, messageEventID)
		if err != nil {
			return nil, fmt.Errorf("resolve reaction notification target: %w", err)
		}
		if target == nil {
			return nil, ErrNotFound
		}
		return entries, nil
	})
	if err != nil {
		verb := "remove"
		if add {
			verb = "add"
		}
		return false, fmt.Errorf("failed to %s reaction: %w", verb, err)
	}
	if !result.Committed {
		return false, nil
	}
	if len(result.Sequences) == 0 {
		return false, fmt.Errorf("reaction mutation committed without a sequence")
	}
	if err := s.core.roomModel.waitForReactions(ctx, events.SubjectPosition(publishSubject, result.Sequences[0])); err != nil {
		return false, fmt.Errorf("wait for reactions projection: %w", err)
	}
	if err := s.core.notificationMaterializer.WaitThrough(ctx, result.Sequences[0]); err != nil {
		s.core.logger.Warn("Notification effect did not reach the committed reaction mutation before the request completed",
			"room_id", input.RoomID, "message_event_id", committedMessageEventID, "error", err)
	}
	action := "removed"
	if add {
		action = "added"
	}
	s.core.logger.Debug("Reaction "+action,
		"kind", committedKind,
		"room_id", input.RoomID,
		"message_event_id", committedMessageEventID,
		"emoji_name", emojiName,
		"user_id", input.ActorID,
		"mutation_attempts", result.Attempts,
		"mutation_conflicts", result.Conflicts,
	)
	return true, nil
}

// prepareAuthorizedReactionAttempt makes each projection read current to the
// authorization and room facts used by one mutation attempt before evaluating
// the authoritative operation-level gate.
func (s *ReactionModel) prepareAuthorizedReactionAttempt(ctx context.Context, input ReactionMutationInput) (RoomKind, error) {
	roomPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(input.RoomID).AllEventsFilter())
	if err != nil {
		return KindChannel, fmt.Errorf("read room reaction tail: %w", err)
	}
	groupPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return KindChannel, fmt.Errorf("read room-group authorization tail: %w", err)
	}
	rbacPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return KindChannel, fmt.Errorf("read RBAC authorization tail: %w", err)
	}
	userPosition, err := s.core.EventPublisher.LastSubjectPosition(ctx, evtstream.UserAggregate(input.ActorID).AllEventsFilter())
	if err != nil {
		return KindChannel, fmt.Errorf("read actor authorization tail: %w", err)
	}

	if !roomPosition.IsZero() {
		if err := s.core.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
			return KindChannel, fmt.Errorf("wait for room membership projection: %w", err)
		}
		if err := s.core.roomModel.waitForTimeline(ctx, roomPosition); err != nil {
			return KindChannel, fmt.Errorf("wait for room timeline projection: %w", err)
		}
		if err := s.core.roomModel.waitForReactions(ctx, roomPosition); err != nil {
			return KindChannel, fmt.Errorf("wait for reaction projection: %w", err)
		}
	}
	if err := s.core.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
		return KindChannel, fmt.Errorf("wait for room-group authorization projection: %w", err)
	}
	if err := s.core.rbacModel.waitFor(ctx, rbacPosition); err != nil {
		return KindChannel, fmt.Errorf("wait for RBAC authorization projection: %w", err)
	}
	if err := s.core.userModel.waitForUsers(ctx, userPosition); err != nil {
		return KindChannel, fmt.Errorf("wait for actor authorization projection: %w", err)
	}
	return s.authorizeReaction(ctx, input)
}

func (s *ReactionModel) publishReactionMutation(ctx context.Context, kind RoomKind, roomID, messageEventID, emoji, userID string, event *corev1.Event) (bool, uint64, error) {
	add := event.GetReactionAdded() != nil
	remove := event.GetReactionRemoved() != nil
	if !add && !remove {
		return false, 0, fmt.Errorf("unsupported reaction event %T", event.GetEvent())
	}

	agg := evtstream.RoomAggregate(roomID)
	publishSubject := agg.SubjectFor(event)
	occFilter := agg.AllEventsFilter()

	result, err := s.executeMutation(ctx, events.AtSubject(occFilter), func(ctx context.Context, attempt events.MutationAttempt) ([]evtstream.MutationEntry, error) {
		if attempt.ExpectedSequence > 0 {
			if err := s.core.roomModel.waitForReactions(ctx, events.SubjectPosition(occFilter, attempt.ExpectedSequence)); err != nil {
				return nil, fmt.Errorf("wait for current reactions projection: %w", err)
			}
		}
		snapshot := s.core.roomModel.reactionMutationSnapshot(roomID, messageEventID, emoji, userID)
		if add {
			if snapshot.Exists {
				return nil, nil
			}
			if snapshot.UserReactionCount >= MaxReactionsPerUserPerMessage {
				return nil, ErrReactionLimitExceeded
			}
		} else if !snapshot.Exists {
			return nil, nil
		}
		return []evtstream.MutationEntry{{Subject: publishSubject, Event: event}}, nil
	})
	if err != nil {
		return false, 0, err
	}
	if !result.Committed {
		return false, 0, nil
	}
	if len(result.Sequences) == 0 {
		return false, 0, fmt.Errorf("reaction mutation committed without a sequence")
	}
	if err := s.core.roomModel.waitForReactions(ctx, events.SubjectPosition(publishSubject, result.Sequences[0])); err != nil {
		return false, 0, fmt.Errorf("wait for reactions projection: %w", err)
	}
	return true, result.Sequences[0], nil
}
