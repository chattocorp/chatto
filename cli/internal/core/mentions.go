package core

import (
	"context"
	"errors"
	"regexp"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// mentionRegex matches @username patterns in message text.
// Usernames can contain alphanumeric characters, underscores, hyphens, and dots.
// Dots are only allowed as internal separators (not trailing) to avoid capturing
// sentence punctuation like "Thanks @user." → captures "user" not "user."
// The @ must be preceded by whitespace or start of string to avoid matching emails.
// The pattern is case-insensitive for extraction, but lookup is also case-insensitive.
var mentionRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])@([a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*)`)

// ExtractMentionUsernames extracts all unique @username mentions from a message body.
// Returns a slice of usernames (without the @ prefix) in the order they appear.
// Duplicate mentions are deduplicated.
func ExtractMentionUsernames(body string) []string {
	matches := mentionRegex.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate while preserving order
	seen := make(map[string]bool)
	var usernames []string
	for _, match := range matches {
		username := match[1]
		if !seen[username] {
			seen[username] = true
			usernames = append(usernames, username)
		}
	}
	return usernames
}

// ResolveMentions takes a list of usernames and resolves them to user IDs.
// Invalid usernames are silently ignored.
// Returns a slice of valid user IDs.
func (c *ChattoCore) ResolveMentions(ctx context.Context, usernames []string) ([]string, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	var userIDs []string
	for _, username := range usernames {
		// Look up user by login (case-insensitive). Every authenticated user
		// is implicitly a server member post-#330, so no further gate.
		user, err := c.GetUserByLogin(ctx, username)
		if err != nil {
			continue
		}

		userIDs = append(userIDs, user.Id)
	}

	return userIDs, nil
}

// notifyMentionedUsers creates persistent notifications for all mentioned users.
// This handles both the room-level mention indicator and the bell icon notification.
// This is best-effort - failures are logged but don't affect message posting.
//
// inThread is the thread root event ID when the mention is on a message inside
// a thread, or empty string for room-level messages. The frontend uses this to
// route notification clicks directly into the thread pane.
func (c *ChattoCore) notifyMentionedUsers(ctx context.Context, kind RoomKind, roomID, authorID, eventID, inThread string, mentionedUserIDs []string) {
	for _, mentionedUserID := range mentionedUserIDs {
		// Don't notify the author if they mentioned themselves
		if mentionedUserID == authorID {
			continue
		}

		// Skip if user has muted this room
		level, err := c.GetEffectiveNotificationLevel(ctx, mentionedUserID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for mention check, continuing",
				"user_id", mentionedUserID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}

		// Store persistent mention state in KV (for room-level indicator)
		if err := c.setMentionStatus(ctx, roomID, mentionedUserID); err != nil {
			c.logger.Warn("Failed to set mention status",
				"user_id", mentionedUserID,
				"kind", kind,
				"room_id", roomID,
				"error", err)
		}

		// Publish live mention event for room-level indicator real-time update
		// (Space/room/user names are resolved by GraphQL resolvers)
		mentionEvent := &corev1.Event{
			Id:        NewEventID(),
			ActorId:   authorID,
			CreatedAt: timestamppb.Now(),
			Event: &corev1.Event_MentionNotification{
				MentionNotification: &corev1.MentionNotificationEvent{
					RoomId:            roomID,
					MentionedByUserId: authorID,
				},
			},
		}
		subject := subjects.LiveUserEvent(mentionedUserID, "mentioned")
		if err := c.publishLiveEvent(ctx, subject, mentionEvent); err != nil {
			c.logger.Warn("Failed to publish mention live event",
				"mentioned_user_id", mentionedUserID,
				"error", err)
		}

		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		_, createErr := c.CreateNotification(ctx, mentionedUserID, authorID, &corev1.Notification{
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					RoomId:   roomID,
					EventId:  eventID,
					InThread: inThread,
				},
			},
		})
		if createErr != nil {
			c.logger.Warn("Failed to create mention notification",
				"mentioned_user_id", mentionedUserID,
				"author_id", authorID,
				"kind", kind,
				"room_id", roomID,
				"error", createErr)
		} else {
			c.logger.Debug("Created mention notification",
				"mentioned_user_id", mentionedUserID,
				"author_id", authorID,
				"kind", kind,
				"room_id", roomID)
		}
	}
}

// Mention status KV key pattern: room_mention_status.{userId}.{roomId}
func mentionStatusKey(userID, roomID string) string {
	return "room_mention_status." + userID + "." + roomID
}

// setMentionStatus marks that a user has an unread mention in a room.
// Uses atomic create-if-not-exists so it's idempotent — the first mention
// is preserved until the user reads the room and clears it.
func (c *ChattoCore) setMentionStatus(ctx context.Context, roomID, userID string) error {
	bucket := c.storage.serverRuntimeKV

	key := mentionStatusKey(userID, roomID)

	// Use Create to only set if key doesn't exist - preserves earliest mention
	_, err := bucket.Create(ctx, key, []byte{1})
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Key already exists means there's already an unread mention - that's fine
			return nil
		}
		// Real error - propagate it
		return err
	}
	return nil
}

// HasMention checks if a user has an unread mention in a room.
func (c *ChattoCore) HasMention(ctx context.Context, roomID, userID string) (bool, error) {
	bucket := c.storage.serverRuntimeKV

	key := mentionStatusKey(userID, roomID)
	_, err := bucket.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Key not found means no unread mention
			return false, nil
		}
		// Real error - propagate it
		return false, err
	}
	return true, nil
}

// ClearMentionStatus removes the mention indicator for a user in a room.
// Called when the user visits the room and reads their mentions, or when
// they dismiss a mention notification. Idempotent.
//
// When a flag is actually removed, publishes a MentionStatusClearedEvent on
// the user's live stream so other devices drop the orange dot for the room
// without waiting for the next GraphQL refetch. The Get-then-Delete shape
// keeps idempotent callers from spamming events on no-op clears (the common
// case when entering a room with no pending mention).
func (c *ChattoCore) ClearMentionStatus(ctx context.Context, roomID, userID string) error {
	bucket := c.storage.serverRuntimeKV
	key := mentionStatusKey(userID, roomID)

	if _, err := bucket.Get(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return err
	}

	if err := bucket.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return err
	}

	c.publishMentionStatusClearedEvent(ctx, userID, roomID)
	return nil
}

// publishMentionStatusClearedEvent emits a live event so the user's other
// devices can drop the room's orange dot in real time. Best-effort — a
// publish failure is logged but doesn't fail the clear operation, since the
// KV is already updated and clients will catch up on next refetch.
func (c *ChattoCore) publishMentionStatusClearedEvent(ctx context.Context, userID, roomID string) {
	event := &corev1.Event{
		Id:        NewEventID(),
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MentionStatusCleared{
			MentionStatusCleared: &corev1.MentionStatusClearedEvent{
				RoomId: roomID,
			},
		},
	}

	subject := subjects.LiveUserEvent(userID, "mention_status_cleared")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish mention status cleared event",
			"user_id", userID,
			"room_id", roomID,
			"error", err)
	}
}
