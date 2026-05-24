package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomMembershipKey returns the KV key for a room membership.
// Pattern: `room_membership.{kind}.{roomID}.{userID}` where kind is
// "channel" or "dm". Same outer-to-inner scope ordering as roomKey
// (`room.{kind}.{roomID}`): kind, then room, then per-room detail.
func roomMembershipKey(kind RoomKind, room_id, user_id string) string {
	return fmt.Sprintf("room_membership.%s.%s.%s", kind, room_id, user_id)
}

// roomMembershipKeyMatchForUser returns the subject filter that matches
// a user's memberships of a given kind. The userID is in the trailing
// position of the key (`room_membership.{kind}.{roomID}.{userID}`), so
// this is an internal-wildcard filter rather than a pure prefix:
// `room_membership.{kind}.*.{userID}`. Server-side filtered by NATS.
//
// Used by deleteUserRoomMembershipsInSpace to find the keys it needs to
// delete during account-deletion cleanup. Other callers used to scan the
// bucket here for reads; those now go through the projection.
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
// Reads from the RoomMembership projection (ADR-035 phase 5 cutover).
// kind is ignored — roomID is globally unique, so the (roomID, userID)
// pair fully identifies a membership.
func (c *ChattoCore) GetRoomMembership(ctx context.Context, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	if !c.RoomMembership.IsMember(room_id, user_id) {
		return nil, fmt.Errorf("room membership not found for user %s in room %s: %w", user_id, room_id, jetstream.ErrKeyNotFound)
	}
	return &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}, nil
}

// RoomMembershipExists checks if a user is a member of a room.
// Reads from the RoomMembership projection (ADR-035 phase 5 cutover).
//
// Membership is strictly explicit: a user is a member iff the projection
// has an entry. A user with `room.join` who hasn't joined is not yet a member.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	return c.RoomMembership.IsMember(room_id, user_id), nil
}

// JoinRoom creates or updates a room membership for a user.
// This operation is idempotent - calling it multiple times with the same parameters
// will succeed without error, making it safe for distributed systems where the same
// operation might be retried or executed concurrently.
// Authorization: Caller must verify CanJoinRoom before calling.
//
// Dual-write (ADR-035 phase 4): when this is a new membership, a
// UserJoinedRoomEvent is appended to EVT *before* the KV write,
// then a legacy event is published to the room's meta subject (still
// the source of live updates for the frontend's myEvents subscription).
// Reads have been cut over to the projection (phase 5) — the isNew
// check uses RoomMembership.IsMember.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	// Verify room exists and is not archived
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}

	// Idempotency check via projection. There's a tiny race window if two
	// callers IsMember-check before either publishes — both would emit a
	// duplicate UserJoinedRoom. That's fine: the projection's Apply is
	// idempotent on already-present (room, user) pairs.
	isNew := !c.RoomMembership.IsMember(room_id, user_id)

	var seq uint64
	if isNew {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					RoomId: room_id,
				},
			},
		})

		seq, err = c.EventPublisher.Append(ctx, events.RoomAggregate(room_id).Subject(), event)
		if err != nil {
			return nil, fmt.Errorf("publish UserJoinedRoomEvent: %w", err)
		}

		// Legacy publish — feeds live.server.> for the frontend's
		// myEvents subscription. Best-effort; failures are logged but
		// don't roll back the join.
		legacySubject := subjects.RoomMeta(string(kind), room_id)
		if err := c.publishServerEvent(ctx, legacySubject, event); err != nil {
			c.logger.Error("failed to publish UserJoinedRoomEvent (legacy)", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	// KV write stays in place during dual-write. EVT is the source
	// of truth for reads (projection-backed); KV is kept up to date so
	// backups and any unmigrated read paths remain coherent.
	kv := c.storage.serverConfigKV
	membership := &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}
	data, err := proto.Marshal(membership)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room membership data: %w", err)
	}
	if _, err := kv.Put(ctx, roomMembershipKey(kind, room_id, user_id), data); err != nil {
		return nil, fmt.Errorf("failed to create room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Created room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	if isNew {
		// Initialize the read marker for new members. For non-empty rooms, mark
		// them caught up to the current last event so existing messages don't
		// surface as unread. For empty rooms, write an empty-string sentinel so
		// the key's presence still distinguishes "member with nothing to read
		// yet" from "no marker at all" (which the lazy-init path treats as a
		// deploy-era upgrade — see GetLastReadEventID).
		var initEventID string
		if lastID, _, exists, err := c.GetRoomLastEvent(ctx, kind, room_id); err != nil {
			c.logger.Warn("Failed to get room last event during join", "error", err, "room_id", room_id)
		} else if exists {
			initEventID = lastID
		}
		if err := c.SetLastReadEventID(ctx, kind, user_id, room_id, initEventID); err != nil {
			c.logger.Warn("Failed to initialize read marker during join", "error", err, "room_id", room_id)
		}

		// Read-your-writes: ensure the projection has applied our event
		// before returning so the caller's next IsMember/Members read is
		// consistent.
		if err := c.RoomMembershipProjector.WaitForSeq(ctx, seq); err != nil {
			return nil, fmt.Errorf("wait for projection: %w", err)
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
//
// Dual-write (ADR-035 phase 4) — same shape as JoinRoom: publish to
// EVT first, then KV delete, then legacy publish, then
// WaitForSeq. Idempotent on the no-op path (user wasn't a member).
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	// DM conversations are permanent - users cannot leave them
	if kind == KindDM {
		return ErrCannotLeaveDMConversation
	}

	// Read membership state from the projection (ADR-035 phase 5).
	wasMember := c.RoomMembership.IsMember(room_id, user_id)

	var seq uint64
	if wasMember {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					RoomId: room_id,
				},
			},
		})

		var err error
		seq, err = c.EventPublisher.Append(ctx, events.RoomAggregate(room_id).Subject(), event)
		if err != nil {
			return fmt.Errorf("publish UserLeftRoomEvent: %w", err)
		}

		legacySubject := subjects.RoomMeta(string(kind), room_id)
		if err := c.publishServerEvent(ctx, legacySubject, event); err != nil {
			c.logger.Error("failed to publish UserLeftRoomEvent (legacy)", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	// KV delete stays in place during dual-write. Idempotent: deleting a
	// non-existent key is fine.
	kv := c.storage.serverConfigKV
	if err := kv.Delete(ctx, roomMembershipKey(kind, room_id, user_id)); err != nil {
		return fmt.Errorf("failed to delete room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Deleted room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	if wasMember {
		// Read-your-writes: projection must reflect our event before we return.
		if err := c.RoomMembershipProjector.WaitForSeq(ctx, seq); err != nil {
			return fmt.Errorf("wait for projection: %w", err)
		}
	}

	return nil
}

// GetUserRoomMemberships retrieves all room memberships for a given user of a
// given kind. The projection (ADR-035 phase 5) doesn't track kind, so the
// caller's set of roomIDs is filtered against the Room KV via GetRoom.
// This is O(N) lookups in the user's room count — acceptable for the
// resolvers that use it (each user has a bounded number of rooms).
//
// Once a RoomKind projection lands (or kind moves into the Room proto so
// a kind check is local), this can become a single projection read.
func (c *ChattoCore) GetUserRoomMemberships(ctx context.Context, kind RoomKind, user_id string) ([]*corev1.RoomMembership, error) {
	roomIDs := c.RoomMembership.Rooms(user_id)
	out := make([]*corev1.RoomMembership, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		// Probe the Room KV at the requested kind. If the room exists
		// under that kind, include the membership.
		if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("lookup room %s: %w", roomID, err)
		}
		out = append(out, &corev1.RoomMembership{
			UserId: user_id,
			RoomId: roomID,
		})
	}
	return out, nil
}

// GetAllUserRoomMemberships retrieves all of a user's room memberships
// across every kind. Reads from the RoomMembership projection
// (ADR-035 phase 5 cutover).
func (c *ChattoCore) GetAllUserRoomMemberships(ctx context.Context, user_id string) ([]*corev1.RoomMembership, error) {
	roomIDs := c.RoomMembership.Rooms(user_id)
	out := make([]*corev1.RoomMembership, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		out = append(out, &corev1.RoomMembership{
			UserId: user_id,
			RoomId: roomID,
		})
	}
	return out, nil
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

	// 4 dual-write: EVT publish first, then KV delete, then the
	// legacy publish for live myEvents delivery.
	for _, entry := range entries {
		event := newEvent(user_id, &corev1.Event{
			Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					RoomId: entry.roomID,
				},
			},
		})

		if _, err := c.EventPublisher.Append(ctx, events.RoomAggregate(entry.roomID).Subject(), event); err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent to EVT", "room_id", entry.roomID, "error", err)
		}

		if err := kv.Delete(ctx, entry.key); err != nil {
			c.logger.Warn("Failed to delete room membership", "key", entry.key, "error", err)
			continue
		}

		subject := subjects.RoomMeta(string(kind), entry.roomID)
		if err := c.publishServerEvent(ctx, subject, event); err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent (legacy)", "room_id", entry.roomID, "error", err)
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
