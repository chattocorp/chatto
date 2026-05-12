package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DefaultAutoJoinRoom describes a room created automatically by CreateSpace.
type DefaultAutoJoinRoom struct {
	Name        string
	Description string
}

// DefaultAutoJoinRooms is the list of rooms created automatically by every
// CreateSpace call. Each is created with auto_join=true so new space members
// are joined to them on space-join.
var DefaultAutoJoinRooms = []DefaultAutoJoinRoom{
	{Name: "announcements", Description: "Announcements and News"},
	{Name: "general", Description: "General discussion"},
}

// validateSpaceName validates that a space name is non-empty, has no leading/trailing whitespace,
// and does not exceed the maximum length.
func validateSpaceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("space name cannot be empty")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("space name cannot have leading or trailing whitespace")
	}
	if len(name) > MaxSpaceNameLength {
		return ErrSpaceNameTooLong
	}
	return nil
}

// ============================================================================
// Space Operations
// ============================================================================

// storeSpaceAndCreateStream marshals a space, stores it in KV, creates its event stream,
// and eagerly initializes all space-level KV buckets and object stores.
// If atomic is true, uses Create (fails if exists); otherwise uses Put (upsert).
// Returns true if the space was created, false if it already existed (only relevant when atomic=true).
func (c *ChattoCore) storeSpaceAndCreateStream(ctx context.Context, space *corev1.Space, atomic bool) (bool, error) {
	spaceData, err := proto.Marshal(space)
	if err != nil {
		return false, fmt.Errorf("failed to marshal space: %w", err)
	}

	if atomic {
		_, err = c.storage.serverKV.Create(ctx, spaceKey(space.Id), spaceData)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				return false, nil // Already exists, not an error
			}
			return false, fmt.Errorf("failed to store space: %w", err)
		}
	} else {
		_, err = c.storage.serverKV.Put(ctx, spaceKey(space.Id), spaceData)
		if err != nil {
			return false, fmt.Errorf("failed to store space: %w", err)
		}
	}

	return true, nil
}

// CreateSpace creates a new space.
// KV store is written first, then an event is published for audit trail (best-effort).
func (c *ChattoCore) CreateSpace(ctx context.Context, actorID string, name string, description string) (*corev1.Space, error) {
	// Validate and sanitize name
	if err := validateSpaceName(name); err != nil {
		return nil, err
	}

	// Validate description length
	if len(description) > MaxDescriptionLength {
		return nil, ErrDescriptionTooLong
	}

	space := &corev1.Space{
		Id:          NewSpaceID(),
		Name:        name,
		Description: description,
	}

	if _, err := c.storeSpaceAndCreateStream(ctx, space, false); err != nil {
		return nil, err
	}

	// Create default roles (owner, moderator, everyone)
	if err := c.CreateDefaultRoles(ctx); err != nil {
		return nil, fmt.Errorf("failed to create default roles: %w", err)
	}

	// Auto-join creator to any rooms flagged auto_join. Server "membership"
	// is implicit post-consolidation; there's no separate join step.
	c.AutoJoinDefaultRooms(ctx, space.Id, actorID)

	// Assign owner role to creator (SystemActorID bypasses permission check - bootstrap mode)
	if err := c.AssignServerRole(ctx, SystemActorID, actorID, RoleOwner); err != nil {
		return nil, fmt.Errorf("failed to assign owner role to creator: %w", err)
	}

	// Create and publish audit event (best-effort). Goes out on the
	// actor's user-scoped subject — the GraphQL gateway intentionally
	// drops ServerCreatedEvent (the server can't be created via the API
	// anymore), so this publish is for telemetry / future consumers only.
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_ServerCreated{
			ServerCreated: &corev1.ServerCreatedEvent{
				ServerId:    space.Id,
				Name:        space.Name,
				Description: space.Description,
			},
		},
	})
	subject := subjects.LiveUserEvent(actorID, "server_created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish server created event", "error", err, "id", space.Id)
	}

	c.logger.Info("Created space", "id", space.Id, "name", space.Name)

	return space, nil
}

// GetSpace retrieves a space from the INSTANCE KV bucket.
func (c *ChattoCore) GetSpace(ctx context.Context, space_id string) (*corev1.Space, error) {
	entry, err := c.storage.serverKV.Get(ctx, spaceKey(space_id))
	if err != nil {
		return nil, fmt.Errorf("space not found: %w", err)
	}

	space := &corev1.Space{}
	if err := proto.Unmarshal(entry.Value(), space); err != nil {
		return nil, fmt.Errorf("failed to unmarshal space: %w", err)
	}

	return space, nil
}

// ListSpaces retrieves all spaces from the INSTANCE KV bucket.
func (c *ChattoCore) ListSpaces(ctx context.Context) ([]*corev1.Space, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "space.*")
	if err != nil {
		return []*corev1.Space{}, nil
	}

	var spaces []*corev1.Space
	for key := range keyLister.Keys() {
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get space %s: %w", key, err)
		}

		space := &corev1.Space{}
		if err := proto.Unmarshal(entry.Value(), space); err != nil {
			return nil, fmt.Errorf("failed to unmarshal space %s: %w", key, err)
		}

		spaces = append(spaces, space)
	}

	return spaces, nil
}

// ============================================================================
// Space-Scoped User Cleanup
// ============================================================================

// CleanupUserStateInSpace removes a user's per-space artifacts: room
// memberships, notification levels, and (during account deletion) emits a
// SpaceMemberDeletedEvent so clients can re-render messages as "Deleted User".
// Idempotent; safe to call for spaces the user never interacted with.
//
// Post-#330 there's no separate "space membership" record to delete — every
// authenticated user is implicitly a server member.
func (c *ChattoCore) CleanupUserStateInSpace(ctx context.Context, userID, spaceID string, isAccountDeletion bool) error {
	if err := c.deleteUserRoomMembershipsInSpace(ctx, userID, spaceID); err != nil {
		c.logger.Warn("Failed to delete room memberships during cleanup", "user_id", userID, "space_id", spaceID, "error", err)
	}

	if err := c.deleteUserNotificationLevels(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete notification levels during cleanup", "user_id", userID, "space_id", spaceID, "error", err)
	}

	if isAccountDeletion {
		memberDeletedEvent := newEvent(userID, &corev1.Event{
			Event: &corev1.Event_SpaceMemberDeleted{
				SpaceMemberDeleted: &corev1.SpaceMemberDeletedEvent{
					SpaceId: spaceID,
					UserId:  userID,
				},
			},
		})
		// SERVER_EVENTS' RePublish forwards the persisted event onto
		// live.server.member.deleted automatically — no manual live
		// publish needed.
		subject := subjects.Member("member_deleted")
		if err := c.publishServerEvent(ctx, subject, memberDeletedEvent); err != nil {
			c.logger.Warn("Failed to publish SpaceMemberDeletedEvent", "user_id", userID, "space_id", spaceID, "error", err)
		}
	}

	return nil
}

// AutoJoinDefaultRooms joins the user to rooms that have auto_join enabled.
// Best-effort: errors are logged but don't cause failure.
func (c *ChattoCore) AutoJoinDefaultRooms(ctx context.Context, spaceID, userID string) {
	// Get all rooms in the space
	rooms, err := c.ListRooms(ctx, KindForSpace(spaceID))
	if err != nil {
		c.logger.Warn("failed to list rooms for auto-join", "error", err, "space_id", spaceID)
		return
	}

	// Join rooms that have auto_join enabled
	for _, room := range rooms {
		if room.AutoJoin {
			// Use the user as the actor since they are joining (even if automatically)
			_, err := c.JoinRoom(ctx, userID, spaceID, userID, room.Id)
			if err != nil {
				c.logger.Warn("failed to auto-join user to room",
					"error", err,
					"user_id", userID,
					"space_id", spaceID,
					"room_id", room.Id,
					"room_name", room.Name)
			} else {
				c.logger.Info("Auto-joined user to room",
					"user_id", userID,
					"space_id", spaceID,
					"room_id", room.Id,
					"room_name", room.Name)
			}
		}
	}
}

// GetSpaceRoomCount returns the number of rooms in a space.
func (c *ChattoCore) GetSpaceRoomCount(ctx context.Context, spaceID string) (int, error) {
	rooms, err := c.ListRooms(ctx, KindForSpace(spaceID))
	if err != nil {
		return 0, err
	}
	return len(rooms), nil
}

// GetSpaceAssetCount returns the number of assets (attachments) in a space.
func (c *ChattoCore) GetSpaceAssetCount(ctx context.Context, spaceID string) (int, error) {
	store, err := c.GetAttachmentsStore(ctx)
	if err != nil {
		// If the bucket doesn't exist, return 0
		return 0, nil
	}

	// List all objects and count them
	objects, err := store.List(ctx)
	if err != nil {
		// ErrNoObjectsFound means empty bucket, not an error
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list objects for space %s: %w", spaceID, err)
	}

	count := 0
	for range objects {
		count++
	}

	return count, nil
}

// ============================================================================
// Space Member Listing (for management UI)
// ============================================================================

// SpaceMemberWithRoles represents a space member with their assigned roles.
type SpaceMemberWithRoles struct {
	UserID string
	Roles  []string
}

// GetSpaceMembers retrieves space members with optional search and pagination.
// Search matches against login and displayName (case-insensitive partial match).
// Returns members, total count (matching search), and error.
//
// Post-#330 every authenticated user is implicitly a server member, so this
// iterates the full user list rather than the (retired) space-membership
// records. The `spaceID` parameter is retained for the API shape but is no
// longer load-bearing.
func (c *ChattoCore) GetSpaceMembers(ctx context.Context, spaceID string, search string, limit, offset int) ([]SpaceMemberWithRoles, int, error) {
	type memberWithUser struct {
		member SpaceMemberWithRoles
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
		return []SpaceMemberWithRoles{}, 0, nil
	}

	// Normalize search term for case-insensitive matching
	searchLower := strings.ToLower(strings.TrimSpace(search))

	// Filter and build results
	var matches []memberWithUser
	for _, userID := range userIDs {
		// Get user data
		user, err := c.GetUser(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user for space member listing", "user_id", userID, "error", err)
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

		// Get user's roles (caller is iterating space members so virtual
		// "everyone" applies — prepend it explicitly).
		assigned, err := c.GetUserRoles(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user roles for space member listing", "user_id", userID, "error", err)
			assigned = nil
		}
		roles := append([]string{RoleEveryone}, assigned...)

		matches = append(matches, memberWithUser{
			member: SpaceMemberWithRoles{
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
		return []SpaceMemberWithRoles{}, totalCount, nil
	}
	matches = matches[offset:]
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	// Extract SpaceMemberWithRoles from sorted results
	result := make([]SpaceMemberWithRoles, len(matches))
	for i, m := range matches {
		result[i] = m.member
	}

	return result, totalCount, nil
}
