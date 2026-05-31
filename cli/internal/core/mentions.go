package core

import (
	"context"
	"regexp"

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

		// Publish live mention event for room-level indicator real-time update
		// (Space/room/user names are resolved by GraphQL resolvers)
		mentionEvent := newLiveEvent(authorID, &corev1.LiveEvent{
			Event: &corev1.LiveEvent_MentionNotification{
				MentionNotification: &corev1.MentionNotificationEvent{
					RoomId:            roomID,
					MentionedByUserId: authorID,
				},
			},
		})
		subject := subjects.LiveSyncUserEvent(mentionedUserID, "mentioned")
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
