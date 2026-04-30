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
// Only users who are members of the given space are included in the result.
// Invalid usernames or non-members are silently ignored.
// Returns a slice of valid user IDs.
func (c *ChattoCore) ResolveMentions(ctx context.Context, spaceID string, usernames []string) ([]string, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	var userIDs []string
	for _, username := range usernames {
		// Look up user by login (case-insensitive)
		user, err := c.GetUserByLogin(ctx, username)
		if err != nil {
			// User not found - silently ignore invalid mentions
			continue
		}

		// Verify user is a member of this space
		membership, err := c.GetSpaceMembership(ctx, user.Id, spaceID)
		if err != nil {
			c.logger.Debug("Failed to check space membership for mention",
				"username", username,
				"user_id", user.Id,
				"space_id", spaceID,
				"error", err)
			continue
		}
		if membership == nil {
			// User exists but is not a member of this space - skip
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
func (c *ChattoCore) notifyMentionedUsers(ctx context.Context, spaceID, roomID, authorID, eventID, inThread string, mentionedUserIDs []string) {
	for _, mentionedUserID := range mentionedUserIDs {
		// Don't notify the author if they mentioned themselves
		if mentionedUserID == authorID {
			continue
		}

		// Skip if user has muted this room
		level, err := c.GetEffectiveNotificationLevel(ctx, spaceID, mentionedUserID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for mention check, continuing",
				"user_id", mentionedUserID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}

		// Store persistent mention state in KV (for room-level indicator)
		if err := c.setMentionStatus(ctx, spaceID, roomID, mentionedUserID); err != nil {
			c.logger.Warn("Failed to set mention status",
				"user_id", mentionedUserID,
				"space_id", spaceID,
				"room_id", roomID,
				"error", err)
		}

		// Publish live mention event for room-level indicator real-time update
		// (Space/room/user names are resolved by GraphQL resolvers)
		mentionEvent := &corev1.InstanceEvent{
			Id:        NewEventID(),
			ActorId:   authorID,
			CreatedAt: timestamppb.Now(),
			Event: &corev1.InstanceEvent_MentionNotification{
				MentionNotification: &corev1.MentionNotificationEvent{
					SpaceId:           spaceID,
					RoomId:            roomID,
					MentionedByUserId: authorID,
				},
			},
		}
		subject := subjects.LiveInstanceUserEvent(mentionedUserID, "mentioned")
		if err := c.publishInstanceEvent(ctx, subject, mentionEvent); err != nil {
			c.logger.Warn("Failed to publish mention live event",
				"mentioned_user_id", mentionedUserID,
				"error", err)
		}

		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		_, createErr := c.CreateNotification(ctx, mentionedUserID, authorID, &corev1.Notification{
			Notification: &corev1.Notification_Mention{
				Mention: &corev1.MentionNotification{
					SpaceId:  spaceID,
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
				"space_id", spaceID,
				"room_id", roomID,
				"error", createErr)
		} else {
			c.logger.Debug("Created mention notification",
				"mentioned_user_id", mentionedUserID,
				"author_id", authorID,
				"space_id", spaceID,
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
func (c *ChattoCore) setMentionStatus(ctx context.Context, spaceID, roomID, userID string) error {
	bucket, err := c.getSpaceRuntimeBucket(ctx, spaceID)
	if err != nil {
		return err
	}

	key := mentionStatusKey(userID, roomID)

	// Use Create to only set if key doesn't exist - preserves earliest mention
	_, err = bucket.Create(ctx, key, []byte{1})
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
func (c *ChattoCore) HasMention(ctx context.Context, spaceID, roomID, userID string) (bool, error) {
	bucket, err := c.getSpaceRuntimeBucket(ctx, spaceID)
	if err != nil {
		return false, err
	}

	key := mentionStatusKey(userID, roomID)
	_, err = bucket.Get(ctx, key)
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
// Called when the user visits the room and reads their mentions.
// Idempotent - returns nil if no mention exists.
func (c *ChattoCore) ClearMentionStatus(ctx context.Context, spaceID, roomID, userID string) error {
	bucket, err := c.getSpaceRuntimeBucket(ctx, spaceID)
	if err != nil {
		return err
	}

	key := mentionStatusKey(userID, roomID)
	err = bucket.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return err
	}
	return nil
}
