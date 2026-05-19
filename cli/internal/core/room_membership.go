package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomMembershipKey returns the KV key for a room membership.
// Pattern: `room_membership.{kind}.{roomID}.{userID}` where kind is
// "channel" or "dm". Same outer-to-inner scope ordering as roomKey
// (`room.{kind}.{roomID}`): kind, then room, then per-room detail.
func roomMembershipKey(kind RoomKind, room_id, user_id string) string {
	return fmt.Sprintf("room_membership.%s.%s.%s", kind, room_id, user_id)
}

// roomMembershipKeyPrefixForRoom returns the key prefix for listing all
// memberships of a given room. Pattern: `room_membership.{kind}.{roomID}.*`.
// Pure prefix scan — used by room-deletion cleanup and member-list reads.
func roomMembershipKeyPrefixForRoom(kind RoomKind, room_id string) string {
	return fmt.Sprintf("room_membership.%s.%s.*", kind, room_id)
}

// roomMembershipKeyMatchForUser returns the subject filter that matches
// a user's memberships of a given kind. The userID is in the trailing
// position of the key (`room_membership.{kind}.{roomID}.{userID}`), so
// this is an internal-wildcard filter rather than a pure prefix:
// `room_membership.{kind}.*.{userID}`. Server-side filtered by NATS.
func roomMembershipKeyMatchForUser(kind RoomKind, user_id string) string {
	return fmt.Sprintf("room_membership.%s.*.%s", kind, user_id)
}

// roomMembershipKeyMatchForUserAnyKind returns the subject filter that matches
// a user's memberships across all kinds (channel + dm).
// Pattern: `room_membership.*.*.{userID}`.
func roomMembershipKeyMatchForUserAnyKind(user_id string) string {
	return fmt.Sprintf("room_membership.*.*.%s", user_id)
}

// GetRoomMembership retrieves a room membership for a user in a specific room.
func (c *ChattoCore) GetRoomMembership(ctx context.Context, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	key := roomMembershipKey(kind, room_id, user_id)
	data, err := kv.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	var membership corev1.RoomMembership
	if err := proto.Unmarshal(data.Value(), &membership); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room membership data for user %s in room %s: %w", user_id, room_id, err)
	}

	return &membership, nil
}

// RoomMembershipExists checks if a user is a member of a room.
//
// Membership is strictly explicit: a user is a member iff a
// `room_membership` KV record exists. A user with `room.join` who hasn't
// joined is not yet a member.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	_, err := c.GetRoomMembership(ctx, kind, user_id, room_id)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, jetstream.ErrKeyNotFound):
		return false, nil
	default:
		return false, fmt.Errorf("failed to check membership for user %s in room %s: %w", user_id, room_id, err)
	}
}

// JoinRoom creates or updates a room membership for a user.
// This operation is idempotent - calling it multiple times with the same parameters
// will succeed without error, making it safe for distributed systems where the same
// operation might be retried or executed concurrently.
// Authorization: Caller must verify CanJoinRoom before calling.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	// Verify room exists and is not archived
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}

	// Check if this is a new membership (for event publishing)
	exists, err := c.RoomMembershipExists(ctx, kind, user_id, room_id)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing membership: %w", err)
	}
	isNew := !exists

	kv := c.storage.serverConfigKV

	membership := &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}

	data, err := proto.Marshal(membership)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room membership data: %w", err)
	}

	_, err = kv.Put(ctx, roomMembershipKey(kind, room_id, user_id), data)
	if err != nil {
		return nil, fmt.Errorf("failed to create room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Created room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	// Initialize the read marker for new members. For non-empty rooms, mark
	// them caught up to the current last event so existing messages don't
	// surface as unread. For empty rooms, write an empty-string sentinel so
	// the key's presence still distinguishes "member with nothing to read
	// yet" from "no marker at all" (which the lazy-init path treats as a
	// deploy-era upgrade — see GetLastReadEventID).
	if isNew {
		var initEventID string
		if lastID, _, exists, err := c.GetRoomLastEvent(ctx, kind, room_id); err != nil {
			c.logger.Warn("Failed to get room last event during join", "error", err, "room_id", room_id)
		} else if exists {
			initEventID = lastID
		}
		if err := c.SetLastReadEventID(ctx, kind, user_id, room_id, initEventID); err != nil {
			c.logger.Warn("Failed to initialize read marker during join", "error", err, "room_id", room_id)
		}
	}

	// Publish UserJoinedRoomEvent if this is a new membership
	if isNew {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					SpaceId: SpaceIDForKind(kind),
					RoomId:  room_id,
				},
			},
		})

		subject := subjects.RoomMeta(string(kind), room_id)
		if err := c.publishServerEvent(ctx, subject, event); err != nil {
			c.logger.Error("failed to publish UserJoinedRoomEvent", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	return membership, nil
}

// LeaveRoom removes a room membership for a user.
// This operation is idempotent - it will succeed even if the membership doesn't exist.
//
// Business rules:
//   - DM conversations are permanent and cannot be left.
//   - Global rooms grant implicit membership to every server member and
//     cannot be left (users can mute them via notification preferences).
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	// DM conversations are permanent - users cannot leave them
	if kind == KindDM {
		return ErrCannotLeaveDMConversation
	}

	// Check if the membership exists before deletion (for event publishing)
	exists, err := c.RoomMembershipExists(ctx, kind, user_id, room_id)
	if err != nil {
		return fmt.Errorf("failed to check existing membership: %w", err)
	}

	kv := c.storage.serverConfigKV

	err = kv.Delete(ctx, roomMembershipKey(kind, room_id, user_id))
	if err != nil {
		return fmt.Errorf("failed to delete room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Deleted room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	// Publish UserLeftRoomEvent if the membership existed
	if exists {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					SpaceId: SpaceIDForKind(kind),
					RoomId:  room_id,
				},
			},
		})

		subject := subjects.RoomMeta(string(kind), room_id)
		if err := c.publishServerEvent(ctx, subject, event); err != nil {
			c.logger.Error("failed to publish UserLeftRoomEvent", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	return nil
}

// GetUserRoomMemberships retrieves all room memberships for a given user in a specific space.
func (c *ChattoCore) GetUserRoomMemberships(ctx context.Context, kind RoomKind, user_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUser(kind, user_id))
	if err != nil {
		return nil, fmt.Errorf("failed to list room memberships for user %s in space %s: %w", user_id, kind, err)
	}

	return readMembershipsFromKeys(ctx, kv, kl)
}

// GetAllUserRoomMemberships retrieves all of a user's room memberships across
// every kind (channel + dm). The post-pivot data layer is a single
// SERVER_CONFIG bucket, so the kind segment is the only thing that scoped a
// listing by space; callers that don't care about that distinction (e.g. the
// unified live-event subscription) use this.
func (c *ChattoCore) GetAllUserRoomMemberships(ctx context.Context, user_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUserAnyKind(user_id))
	if err != nil {
		return nil, fmt.Errorf("failed to list room memberships for user %s: %w", user_id, err)
	}

	return readMembershipsFromKeys(ctx, kv, kl)
}

func readMembershipsFromKeys(ctx context.Context, kv jetstream.KeyValue, kl jetstream.KeyLister) ([]*corev1.RoomMembership, error) {
	var memberships []*corev1.RoomMembership
	for key := range kl.Keys() {
		data, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get room membership data for key %s: %w", key, err)
		}

		var membership corev1.RoomMembership
		if err := proto.Unmarshal(data.Value(), &membership); err != nil {
			return nil, fmt.Errorf("failed to unmarshal room membership data for key %s: %w", key, err)
		}

		memberships = append(memberships, &membership)
	}
	return memberships, nil
}

// deleteUserRoomMembershipsInSpace deletes all room memberships for a user in a specific space.
// This is called when a user leaves a space (or their account is deleted) to clean up room memberships.
// It also publishes UserLeftRoomEvent for each room so clients can update their member lists.
func (c *ChattoCore) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id string, kind RoomKind) error {
	kv := c.storage.serverConfigKV

	// List the user's memberships in this space's kind. Key format
	// post-#330 phase 4b: `room_membership.{kind}.{room_id}.{user_id}`.
	// userID is the trailing segment, so this is an internal-wildcard
	// filter rather than a pure prefix.
	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUser(kind, user_id))
	if err != nil {
		// No keys found is fine - user may not be in any rooms
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("failed to list room memberships for user %s in space %s: %w", user_id, kind, err)
	}

	// Collect keys and extract room IDs
	type keyAndRoom struct {
		key    string
		roomID string
	}
	var entries []keyAndRoom
	for key := range kl.Keys() {
		// Extract room ID from key: room_membership.{kind}.{room_id}.{user_id}
		parts := strings.Split(key, ".")
		if len(parts) == 4 {
			entries = append(entries, keyAndRoom{key: key, roomID: parts[2]})
		}
	}

	// Delete each room membership and publish events
	for _, entry := range entries {
		if err := kv.Delete(ctx, entry.key); err != nil {
			c.logger.Warn("Failed to delete room membership", "key", entry.key, "error", err)
			continue
		}

		// Publish UserLeftRoomEvent so clients can update their member lists
		event := newEvent(user_id, &corev1.Event{
			Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					SpaceId: SpaceIDForKind(kind),
					RoomId:  entry.roomID,
				},
			},
		})
		subject := subjects.RoomMeta(string(kind), entry.roomID)
		if err := c.publishServerEvent(ctx, subject, event); err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent", "room_id", entry.roomID, "error", err)
		}
	}

	if len(entries) > 0 {
		c.logger.Info("Deleted user room memberships", "user_id", user_id, "kind", kind, "count", len(entries))
	}

	return nil
}

// GetRoomMembersList retrieves all user memberships for a given room.
func (c *ChattoCore) GetRoomMembersList(ctx context.Context, kind RoomKind, room_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	// List room memberships of the kind that lives in this space's bucket.
	// Key format: `room_membership.{kind}.{userID}.{roomID}`.
	kl, err := kv.ListKeysFiltered(ctx, fmt.Sprintf("room_membership.%s.>", kind))
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return []*corev1.RoomMembership{}, nil
		}
		return nil, fmt.Errorf("failed to list room membership keys in space %s: %w", kind, err)
	}

	var memberships []*corev1.RoomMembership

	for key := range kl.Keys() {
		data, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get room membership data for key %s: %w", key, err)
		}

		var membership corev1.RoomMembership
		if err := proto.Unmarshal(data.Value(), &membership); err != nil {
			return nil, fmt.Errorf("failed to unmarshal room membership data for key %s: %w", key, err)
		}

		// Filter by room_id
		if membership.RoomId == room_id {
			memberships = append(memberships, &membership)
		}
	}

	return memberships, nil
}
