package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxJoinRoomRetries = 5

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

// JoinRoom creates a room membership for a user.
// Idempotent: calling it multiple times with the same parameters is a no-op
// (the projection's Apply is idempotent on already-present (room, user)
// pairs, and we early-out via IsMember).
// Authorization: Caller must verify CanJoinRoom before calling.
//
// ADR-035 phase 6: event-only. Publishes UserJoinedRoomEvent to EVT, writes
// a legacy SERVER_EVENTS copy for migration/import tooling, then WaitForSeq
// on the projections that serve membership and room history reads. The
// room_membership KV bucket is no longer written to (retained as pre-ES
// import evidence).
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	// Verify room exists and is not archived
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}

	membership := &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{
				RoomId: room_id,
			},
		},
	})

	joinSubject := events.RoomAggregate(room_id).SubjectFor(event)
	var seq uint64
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		if c.RoomMembership.IsMember(room_id, user_id) {
			return membership, nil
		}

		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, joinSubject)
		if err != nil {
			return nil, fmt.Errorf("read UserJoinedRoomEvent OCC seq: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.RoomMembershipProjector.WaitForSeq(ctx, expectedSeq); err != nil {
				return nil, fmt.Errorf("wait for membership projection before join: %w", err)
			}
			if c.RoomMembership.IsMember(room_id, user_id) {
				return membership, nil
			}
		}

		seq, err = c.EventPublisher.AppendAt(ctx, joinSubject, event, expectedSeq)
		if err == nil {
			if err := c.RoomMembershipProjector.WaitForSeq(ctx, seq); err != nil {
				return nil, fmt.Errorf("wait for membership projection: %w", err)
			}
			if err := c.RoomTimelineProjector.WaitForSeq(ctx, seq); err != nil {
				return nil, fmt.Errorf("wait for room timeline projection: %w", err)
			}
			break
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, fmt.Errorf("publish UserJoinedRoomEvent: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	if seq == 0 {
		return nil, fmt.Errorf("publish UserJoinedRoomEvent retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
	}

	// Legacy SERVER_EVENTS copy retained for migration/import tooling.
	// Best-effort; failures are logged but don't roll back the join.
	legacySubject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, legacySubject, event); err != nil {
		c.logger.Error("failed to publish UserJoinedRoomEvent (legacy)", "error", err, "user_id", user_id, "room_id", room_id)
	}

	c.logger.Info("Created room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

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

	return membership, nil
}

// LeaveRoom removes a room membership for a user.
// Idempotent: no-op if the user is not a member.
//
// Business rules:
//   - DM conversations are permanent and cannot be left.
//   - Global rooms grant implicit membership to every server member and
//     cannot be left (users can mute them via notification preferences).
//
// ADR-035 phase 6: event-only. Publishes UserLeftRoomEvent, legacy
// mirror, then WaitForSeq on the projections that serve membership
// and room history reads.
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	if kind == KindDM {
		return ErrCannotLeaveDMConversation
	}

	if !c.RoomMembership.IsMember(room_id, user_id) {
		return nil
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{
				RoomId: room_id,
			},
		},
	})

	seq, err := c.RoomMembershipProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, events.RoomAggregate(room_id), event)
	if err != nil {
		return fmt.Errorf("publish UserLeftRoomEvent: %w", err)
	}
	if err := c.RoomTimelineProjector.WaitForSeq(ctx, seq); err != nil {
		return fmt.Errorf("wait for room timeline projection: %w", err)
	}

	legacySubject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, legacySubject, event); err != nil {
		c.logger.Error("failed to publish UserLeftRoomEvent (legacy)", "error", err, "user_id", user_id, "room_id", room_id)
	}

	c.logger.Info("Deleted room membership", "user_id", user_id, "kind", kind, "room_id", room_id)
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

// deleteUserRoomMembershipsInSpace removes all of a user's memberships of
// the given kind. Called when a user is deleted or leaves a space.
// Publishes UserLeftRoomEvent for each affected room, which projections apply.
//
// ADR-035 phase 6: event-only. The list of rooms to leave is read
// from the projection rather than scanning the KV bucket. The kind
// filter is applied via GetRoom to skip rooms of other kinds.
func (c *ChattoCore) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id string, kind RoomKind) error {
	allRoomIDs := c.RoomMembership.Rooms(user_id)
	if len(allRoomIDs) == 0 {
		return nil
	}

	type roomEntry struct {
		roomID string
	}
	var entries []roomEntry
	for _, roomID := range allRoomIDs {
		// Filter by kind: GetRoom returns ErrKeyNotFound when the
		// room exists but is of a different kind.
		if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("lookup room %s during membership cleanup: %w", roomID, err)
		}
		entries = append(entries, roomEntry{roomID: roomID})
	}

	var lastSeq uint64
	for _, entry := range entries {
		event := newEvent(user_id, &corev1.Event{
			Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					RoomId: entry.roomID,
				},
			},
		})

		seq, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(entry.roomID).SubjectFor(event), event)
		if err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent to EVT", "room_id", entry.roomID, "error", err)
			continue
		}
		if seq > lastSeq {
			lastSeq = seq
		}

		subject := subjects.RoomMeta(string(kind), entry.roomID)
		if err := c.publishServerEvent(ctx, subject, event); err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent (legacy)", "room_id", entry.roomID, "error", err)
		}
	}

	if len(entries) > 0 {
		c.logger.Info("Deleted user room memberships", "user_id", user_id, "kind", kind, "count", len(entries))
	}

	if lastSeq > 0 {
		if err := c.RoomMembershipProjector.WaitForSeq(ctx, lastSeq); err != nil {
			return fmt.Errorf("wait for membership projection after membership cleanup: %w", err)
		}
		if err := c.RoomTimelineProjector.WaitForSeq(ctx, lastSeq); err != nil {
			return fmt.Errorf("wait for room timeline projection after membership cleanup: %w", err)
		}
	}
	return nil
}

// GetRoomMembersList returns every user currently a member of the room.
// ADR-035 phase 6: served from the projection.
//
// kind is preserved on the signature for symmetry with the rest of the
// room API; the (roomID, userID) pair is globally unique so kind is
// irrelevant to the lookup.
func (c *ChattoCore) GetRoomMembersList(_ context.Context, _ RoomKind, room_id string) ([]*corev1.RoomMembership, error) {
	userIDs := c.RoomMembership.Members(room_id)
	out := make([]*corev1.RoomMembership, 0, len(userIDs))
	for _, uid := range userIDs {
		out = append(out, &corev1.RoomMembership{
			UserId: uid,
			RoomId: room_id,
		})
	}
	return out, nil
}
