package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DefaultGlobalRoom describes a channel that ships with a fresh
// deployment. The bootstrap owner is auto-joined; other users join
// explicitly via `joinRoom` (or the room directory's "Join all").
type DefaultGlobalRoom struct {
	Name        string
	Description string
}

// DefaultGlobalRooms is the list of channel rooms seeded on a fresh
// deployment. Each lands in the seed "Lobby" group.
var DefaultGlobalRooms = []DefaultGlobalRoom{
	{Name: "announcements", Description: "Announcements and News"},
	{Name: "general", Description: "General discussion"},
}

// ============================================================================
// Per-Kind User Cleanup
// ============================================================================

// CleanupUserState removes a user's per-kind artifacts: room memberships,
// notification levels, and (during account deletion) emits a
// SpaceMemberDeletedEvent so clients can re-render messages as "Deleted User".
// Idempotent; safe to call for kinds the user never interacted with.
//
// Post-#330 there's no separate "space membership" record to delete — every
// authenticated user is implicitly a server member.
func (c *ChattoCore) CleanupUserState(ctx context.Context, userID string, kind RoomKind, isAccountDeletion bool) error {
	if err := c.deleteUserRoomMembershipsInSpace(ctx, userID, kind); err != nil {
		c.logger.Warn("Failed to delete room memberships during cleanup", "user_id", userID, "kind", kind, "error", err)
	}

	if err := c.deleteUserNotificationLevels(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete notification levels during cleanup", "user_id", userID, "kind", kind, "error", err)
	}

	if isAccountDeletion {
		memberDeletedEvent := newEvent(userID, &corev1.Event{
			Event: &corev1.Event_SpaceMemberDeleted{
				SpaceMemberDeleted: &corev1.SpaceMemberDeletedEvent{
					UserId: userID,
				},
			},
		})
		// SERVER_EVENTS' RePublish forwards the persisted event onto
		// live.server.member.deleted automatically — no manual live
		// publish needed.
		subject := subjects.Member("member_deleted")
		if err := c.publishServerEvent(ctx, subject, memberDeletedEvent); err != nil {
			c.logger.Warn("Failed to publish SpaceMemberDeletedEvent", "user_id", userID, "kind", kind, "error", err)
		}
	}

	return nil
}

// GetChannelRoomCount returns the number of channel rooms on the server.
func (c *ChattoCore) GetChannelRoomCount(ctx context.Context) (int, error) {
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return 0, err
	}
	return len(rooms), nil
}

// GetAssetCount returns the number of assets (attachments) on the server.
func (c *ChattoCore) GetAssetCount(ctx context.Context) (int, error) {
	store, err := c.GetAttachmentsStore(ctx)
	if err != nil {
		// If the bucket doesn't exist, return 0
		return 0, nil
	}

	objects, err := store.List(ctx)
	if err != nil {
		// ErrNoObjectsFound means empty bucket, not an error
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list objects: %w", err)
	}

	count := 0
	for range objects {
		count++
	}

	return count, nil
}

// ============================================================================
// Server Member Listing (for management UI)
// ============================================================================

// ServerMemberWithRoles represents a server member with their assigned roles.
type ServerMemberWithRoles struct {
	UserID string
	Roles  []string
}

// GetServerMembers retrieves server members with optional search and pagination.
// Search matches against login and displayName (case-insensitive partial match).
// Returns members, total count (matching search), and error.
//
// Post-#330 every authenticated user is implicitly a server member, so this
// iterates the full user list rather than space-membership records.
func (c *ChattoCore) GetServerMembers(ctx context.Context, search string, limit, offset int) ([]ServerMemberWithRoles, int, error) {
	type memberWithUser struct {
		member ServerMemberWithRoles
		user   *corev1.User
	}

	allUsers, err := c.ListUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	userIDs := make([]string, 0, len(allUsers))
	for _, u := range allUsers {
		userIDs = append(userIDs, u.Id)
	}

	if len(userIDs) == 0 {
		return []ServerMemberWithRoles{}, 0, nil
	}

	// Normalize search term for case-insensitive matching
	searchLower := strings.ToLower(strings.TrimSpace(search))

	// Filter and build results
	var matches []memberWithUser
	for _, userID := range userIDs {
		// Get user data
		user, err := c.GetUser(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user for server member listing", "user_id", userID, "error", err)
			continue // Skip users we can't fetch
		}

		// Apply search filter if provided
		if searchLower != "" {
			loginMatch := strings.Contains(strings.ToLower(user.Login), searchLower)
			displayNameMatch := strings.Contains(strings.ToLower(user.DisplayName), searchLower)
			if !loginMatch && !displayNameMatch {
				continue // Doesn't match search
			}
		}

		// Get user's roles (caller is iterating server members so virtual
		// "everyone" applies — prepend it explicitly).
		assigned, err := c.GetUserRoles(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user roles for server member listing", "user_id", userID, "error", err)
			assigned = nil
		}
		roles := append([]string{RoleEveryone}, assigned...)

		matches = append(matches, memberWithUser{
			member: ServerMemberWithRoles{
				UserID: userID,
				Roles:  roles,
			},
			user: user,
		})
	}

	// Sort by created_at (oldest first), with null values sorted to end by login
	sort.Slice(matches, func(i, j int) bool {
		// Both null: sort alphabetically by login
		if matches[i].user.CreatedAt == nil && matches[j].user.CreatedAt == nil {
			return strings.ToLower(matches[i].user.Login) < strings.ToLower(matches[j].user.Login)
		}
		// Null timestamps sort to the end
		if matches[i].user.CreatedAt == nil {
			return false
		}
		if matches[j].user.CreatedAt == nil {
			return true
		}
		// Both have timestamps: sort by time (oldest first)
		return matches[i].user.CreatedAt.AsTime().Before(matches[j].user.CreatedAt.AsTime())
	})

	// Get total count before pagination
	totalCount := len(matches)

	// Apply pagination
	if offset >= len(matches) {
		return []ServerMemberWithRoles{}, totalCount, nil
	}
	matches = matches[offset:]
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	// Extract ServerMemberWithRoles from sorted results
	result := make([]ServerMemberWithRoles, len(matches))
	for i, m := range matches {
		result[i] = m.member
	}

	return result, totalCount, nil
}
