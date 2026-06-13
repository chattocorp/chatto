package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	MentionHandleAll  = "all"
	MentionHandleHere = "here"

	LargeMentionNotificationThreshold = 10
)

// MentionConfirmationRequiredError is returned when a message would notify a
// large number of people and the caller has not explicitly confirmed the send.
type MentionConfirmationRequiredError struct {
	RecipientCount int
}

func (e *MentionConfirmationRequiredError) Error() string {
	return fmt.Sprintf("mention confirmation required for %d recipients", e.RecipientCount)
}

// IsVirtualMentionHandle reports whether a handle is owned by Chatto rather
// than by a user or role. Handles are matched case-insensitively.
func IsVirtualMentionHandle(handle string) bool {
	switch strings.ToLower(handle) {
	case MentionHandleAll, MentionHandleHere:
		return true
	default:
		return false
	}
}

func (c *ChattoCore) loginConflictsWithMentionHandle(login string) bool {
	normalized := strings.ToLower(login)
	return IsVirtualMentionHandle(normalized) || c.RBAC.RoleExists(normalized)
}

func (c *ChattoCore) roleNameConflictsWithMentionHandle(roleName string) bool {
	normalized := strings.ToLower(roleName)
	if IsVirtualMentionHandle(normalized) {
		return true
	}
	return c.Users.LoginExists(roleName)
}

func (c *ChattoCore) requireLoginMentionHandleAvailable(login string) error {
	availability := c.mentionables.Availability(login, nil)
	if availability.Available {
		return nil
	}
	if availability.OwnerKind == mentionableOwnerUser {
		return ErrLoginAlreadyTaken
	}
	return ErrUsernameBlocked
}

func (c *ChattoCore) requireRoleMentionHandleAvailable(roleName string) error {
	if c.mentionables.Availability(roleName, nil).Available {
		return nil
	}
	return ErrRoleAlreadyExists
}

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

// ResolveRoomMentions resolves @handles in a message to concrete room-member
// user IDs. Handles share one namespace across users, roles, and virtual
// room-scoped broadcasts:
//   - @all: every current room member
//   - @here: current room members whose presence is not OFFLINE
//   - @pingable-role: current room members explicitly assigned that server role
//   - @user: that user, if they are a current room member
//
// Invalid handles are silently ignored, matching existing @user behavior.
func (c *ChattoCore) ResolveRoomMentions(ctx context.Context, kind RoomKind, roomID string, handles []string) ([]string, error) {
	if len(handles) == 0 {
		return nil, nil
	}

	members, err := c.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	roomMembers := make(map[string]struct{}, len(members))
	for _, member := range members {
		if member != nil && member.UserId != "" {
			roomMembers[member.UserId] = struct{}{}
		}
	}

	seen := make(map[string]struct{})
	userIDs := make([]string, 0, len(handles))
	add := func(userID string) {
		if userID == "" {
			return
		}
		if _, ok := roomMembers[userID]; !ok {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	addMembers := func(candidates []string) {
		for _, userID := range candidates {
			add(userID)
		}
	}

	for _, handle := range handles {
		normalized := strings.ToLower(handle)
		switch normalized {
		case MentionHandleAll:
			for _, member := range members {
				if member != nil {
					add(member.UserId)
				}
			}
			continue
		case MentionHandleHere:
			for _, member := range members {
				if member == nil {
					continue
				}
				status, err := c.GetUserPresence(ctx, member.UserId)
				if err != nil {
					c.logger.Warn("Failed to get presence for @here mention",
						"user_id", member.UserId,
						"room_id", roomID,
						"error", err)
					continue
				}
				if status != PresenceStatusOffline {
					add(member.UserId)
				}
			}
			continue
		case RoleEveryone:
			// The implicit RBAC everyone role is intentionally not a mention
			// handle. Use @all for room-wide broadcast semantics.
			continue
		}

		if role, ok := c.RBAC.GetRole(normalized); ok {
			if !role.GetPingable() {
				continue
			}
			roleUsers, err := c.GetRoleUsers(ctx, normalized)
			if err != nil {
				if err != ErrRoleNotFound {
					c.logger.Warn("Failed to resolve role mention",
						"role", normalized,
						"room_id", roomID,
						"error", err)
				}
				continue
			}
			addMembers(roleUsers)
			continue
		}

		user, err := c.GetUserByLogin(ctx, handle)
		if err != nil {
			continue
		}
		add(user.Id)
	}

	return userIDs, nil
}

func (c *ChattoCore) mentionNotificationRecipientCount(ctx context.Context, roomID, authorID string, mentionedUserIDs []string) (int, error) {
	count := 0
	for _, mentionedUserID := range mentionedUserIDs {
		if mentionedUserID == "" || mentionedUserID == authorID {
			continue
		}
		level, err := c.GetEffectiveNotificationLevel(ctx, mentionedUserID, roomID)
		if err != nil {
			return 0, err
		}
		if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}
		count++
	}
	return count, nil
}

// MentionNotificationRecipientCountForBody returns how many people would
// receive a notification from the @mentions in body after room membership,
// author/self, dedupe, and room-mute filtering are applied.
func (c *ChattoCore) MentionNotificationRecipientCountForBody(ctx context.Context, kind RoomKind, roomID, authorID, body string) (int, error) {
	handles := ExtractMentionUsernames(body)
	mentionedUserIDs, err := c.ResolveRoomMentions(ctx, kind, roomID, handles)
	if err != nil {
		return 0, err
	}
	return c.mentionNotificationRecipientCount(ctx, roomID, authorID, mentionedUserIDs)
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
