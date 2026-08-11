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
// deployment. Universal rooms are available to every join-eligible
// server member without writing explicit memberships.
type DefaultGlobalRoom struct {
	Name        string
	Description string
	Universal   bool
}

// DefaultGlobalRooms is the list of channel rooms seeded on a fresh
// deployment. Each lands in the seed "Lobby" group. Normal rooms inherit the
// server-tier everyone defaults; the "announcements" room is universal and
// adds a room-tier everyone/message.post denial and admin/message.post grant.
var DefaultGlobalRooms = []DefaultGlobalRoom{
	{Name: AnnouncementsRoomName, Description: "Announcements and news", Universal: true},
	{Name: "general", Description: "General discussion"},
}

// SeedDefaultRooms creates the default channel rooms
// (`announcements`, `general`) on a fresh server. Idempotent — returns
// nil without creating anything once any channel room exists, so an
// operator can delete the defaults without seeing them reappear on the
// next boot (as long as at least one channel room remains).
//
// Relies on `ensureChannelRoomsAreInAGroup` having run first so the
// seed "Lobby" group is already in the layout; `CreateRoom` with an
// empty groupID then auto-routes each new room into it.
func (c *ChattoCore) SeedDefaultRooms(ctx context.Context) error {
	existing, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}

	for _, r := range DefaultGlobalRooms {
		options := []CreateRoomOption{WithUniversalRoom(r.Universal)}
		if r.Name == AnnouncementsRoomName {
			options = append(options, WithAnnouncementsRoomDefaults())
		}
		if _, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", r.Name, r.Description, options...); err != nil {
			if errors.Is(err, ErrRoomNameExists) {
				continue
			}
			return fmt.Errorf("create default room %q: %w", r.Name, err)
		}
		c.logger.Info("Seeded default channel room", "name", r.Name)
	}
	return nil
}

// ============================================================================
// Per-Kind User Cleanup
// ============================================================================

// CleanupUserState removes a user's per-kind artifacts: room memberships and,
// during account deletion, emits a live
// ServerMemberDeletedEvent so clients can re-render messages as "Deleted User".
// Idempotent; safe to call for kinds the user never interacted with.
//
// Post-#330 there's no separate "space membership" record to delete — every
// authenticated user is implicitly a server member.
func (c *ChattoCore) CleanupUserState(ctx context.Context, userID string, kind RoomKind, isAccountDeletion bool) error {
	if err := c.deleteUserRoomMembershipsInSpace(ctx, userID, kind); err != nil {
		c.logger.Warn("Failed to delete room memberships during cleanup", "user_id", userID, "kind", kind, "error", err)
	}

	if isAccountDeletion {
		memberDeletedEvent := newLiveEvent(userID, &corev1.LiveEvent{
			Event: &corev1.LiveEvent_ServerMemberDeleted{
				ServerMemberDeleted: &corev1.ServerMemberDeletedEvent{
					UserId: userID,
				},
			},
		})
		subject := subjects.LiveSyncMember("member_deleted")
		if err := c.publishLiveEvent(ctx, subject, memberDeletedEvent); err != nil {
			c.logger.Warn("Failed to publish ServerMemberDeletedEvent", "user_id", userID, "kind", kind, "error", err)
		}
	}

	return nil
}

// GetAssetCount returns the number of assets (attachments) on the server.
func (c *ChattoCore) GetAssetCount(ctx context.Context) (int, error) {
	store, err := c.mediaModel.GetAttachmentsStore(ctx)
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
	User   *corev1.User
	Roles  []string
}

// GetServerMembers retrieves server members with optional search and pagination.
// Search matches against login and displayName (case-insensitive partial match).
// Returns members, total count (matching search), and error.
//
// Post-#330 every authenticated user is implicitly a server member, so this
// iterates the full user list rather than space-membership records.
func (c *ChattoCore) GetServerMembers(ctx context.Context, search string, limit, offset int) ([]ServerMemberWithRoles, int, error) {
	allUsers, err := c.ListUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	matches, totalCount := serverMemberUserPage(allUsers, search, limit, offset)
	result := make([]ServerMemberWithRoles, 0, len(matches))
	for _, user := range matches {
		// The caller is iterating server members, so the virtual "everyone"
		// role applies. Prepend it to the selected page's explicit assignments.
		assigned, err := c.GetUserRoles(ctx, user.GetId())
		if err != nil {
			c.logger.Warn("Failed to get user roles for server member listing", "user_id", user.GetId(), "error", err)
			assigned = nil
		}
		result = append(result, ServerMemberWithRoles{
			UserID: user.GetId(),
			User:   user,
			Roles:  append([]string{RoleEveryone}, assigned...),
		})
	}

	return result, totalCount, nil
}

func serverMemberUserPage(allUsers []*corev1.User, search string, limit, offset int) ([]*corev1.User, int) {
	searchLower := strings.ToLower(strings.TrimSpace(search))
	matches := make([]*corev1.User, 0, len(allUsers))
	for _, user := range allUsers {
		if searchLower != "" {
			loginMatch := strings.Contains(strings.ToLower(user.GetLogin()), searchLower)
			displayNameMatch := strings.Contains(strings.ToLower(user.GetDisplayName()), searchLower)
			if !loginMatch && !displayNameMatch {
				continue
			}
		}
		matches = append(matches, user)
	}

	// Sort by created_at (oldest first), with null values sorted to end by login
	sort.Slice(matches, func(i, j int) bool {
		// Both null: sort alphabetically by login
		if matches[i].GetCreatedAt() == nil && matches[j].GetCreatedAt() == nil {
			return strings.ToLower(matches[i].GetLogin()) < strings.ToLower(matches[j].GetLogin())
		}
		// Null timestamps sort to the end
		if matches[i].GetCreatedAt() == nil {
			return false
		}
		if matches[j].GetCreatedAt() == nil {
			return true
		}
		// Both have timestamps: sort by time (oldest first)
		return matches[i].GetCreatedAt().AsTime().Before(matches[j].GetCreatedAt().AsTime())
	})

	// Get total count before pagination
	totalCount := len(matches)

	// Apply pagination
	if offset >= len(matches) {
		return []*corev1.User{}, totalCount
	}
	matches = matches[offset:]
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, totalCount
}
