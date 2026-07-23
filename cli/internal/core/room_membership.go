package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

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
// Channel rooms marked universal grant effective membership to every server
// member who is currently eligible to join the room. Explicit memberships
// remain the durable state; universal membership is derived at read time.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	if c.RoomMembership.IsMember(room_id, user_id) {
		return true, nil
	}
	if kind != KindChannel {
		return false, nil
	}
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return false, err
	}
	if !room.GetUniversal() {
		return false, nil
	}
	return c.CanJoinRoomAt(ctx, user_id, kind, room_id)
}

// JoinRoom creates a room membership for a user.
// Idempotent: calling it multiple times with the same parameters is a no-op
// (the projection's Apply is idempotent on already-present (room, user)
// pairs, and we early-out via IsMember).
// Authorization: Caller must verify CanJoinRoomAt before calling.
//
// Event-only. Publishes UserJoinedRoomEvent to EVT, then WaitForSeq on the
// projections that serve membership and room history reads.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*corev1.RoomMembership, error) {
	// Verify room exists and is not archived
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}
	if kind == KindChannel && c.roomModel.isRoomBanActive(room_id, user_id, time.Now()) {
		return nil, ErrPermissionDenied
	}

	membership := &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}
	if kind == KindChannel && room.GetUniversal() {
		return membership, nil
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
			if err := c.roomModel.waitForDirectory(ctx, events.SubjectPosition(joinSubject, expectedSeq)); err != nil {
				return nil, fmt.Errorf("wait for room directory projection before join: %w", err)
			}
			if c.RoomMembership.IsMember(room_id, user_id) {
				return membership, nil
			}
		}

		seq, err = c.EventPublisher.AppendAt(ctx, joinSubject, event, expectedSeq)
		if err == nil {
			if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(joinSubject, seq)); err != nil {
				return nil, err
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

	c.logger.Info("Created room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	c.initializeRoomReadMarker(ctx, kind, user_id, room_id)

	return membership, nil
}

// AddMember creates an explicit channel-room membership for another user.
// Authorization: caller must verify room.manage before calling.
//
// The membership transition is still represented by UserJoinedRoomEvent with
// the target user as actor, so existing membership projections and public room
// history remain compatible. A separate moderation event records the manager
// action for audit.
func (c *ChattoCore) AddMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (*corev1.RoomMembership, error) {
	if kind == KindDM {
		return nil, invalidArgument("DM room participants cannot be managed through RoomService")
	}
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	if room.GetUniversal() {
		return nil, invalidArgument("universal room membership cannot be managed explicitly")
	}
	if room.GetArchived() {
		return nil, ErrRoomArchived
	}
	if _, err := c.GetUser(ctx, targetUserID); err != nil {
		return nil, err
	}

	membership := &corev1.RoomMembership{
		UserId: targetUserID,
		RoomId: roomID,
	}

	agg := events.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read room membership add OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.roomModel.waitForDirectory(ctx, events.SubjectPosition(filter, expectedSeq)); err != nil {
				return nil, fmt.Errorf("wait for room directory projection before member add: %w", err)
			}
		}
		if c.RoomMembership.IsMember(roomID, targetUserID) {
			return membership, nil
		}
		if kind == KindChannel && c.roomModel.isRoomBanActive(roomID, targetUserID, time.Now()) {
			return nil, ErrPermissionDenied
		}

		auditEvent := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_RoomMemberAdded{
				RoomMemberAdded: &corev1.RoomMemberAddedEvent{
					RoomId: roomID,
					UserId: targetUserID,
				},
			},
		})
		joinEvent := newEvent(targetUserID, &corev1.Event{
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					RoomId: roomID,
				},
			},
		})

		if err := c.appendRoomMembershipAuditBatch(ctx, roomID, expectedSeq, auditEvent, joinEvent); err == nil {
			c.initializeRoomReadMarker(ctx, kind, targetUserID, roomID)
			c.logger.Info("Added room membership", "actor_id", actorID, "user_id", targetUserID, "kind", kind, "room_id", roomID)
			return membership, nil
		} else if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("publish room member add retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// LeaveRoom removes a room membership for a user.
// Idempotent: no-op if the user is not a member.
//
// Business rules:
//   - DM conversations are permanent and cannot be left.
//   - Universal rooms grant effective membership to join-eligible server
//     members and cannot be left (users can mute them via notification
//     preferences).
//
// ADR-035 phase 6: event-only. Publishes UserLeftRoomEvent, then WaitFor on
// the projections that serve membership and room history reads.
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	if kind == KindDM {
		return ErrCannotLeaveDMConversation
	}

	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) && !c.RoomMembership.IsMember(room_id, user_id) {
			return nil
		}
		return err
	}
	if kind == KindChannel && room.GetUniversal() {
		return ErrCannotLeaveUniversalRoom
	}

	agg := events.RoomAggregate(room_id)
	filter := agg.AllEventsFilter()
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return fmt.Errorf("read room leave OCC tail: %w", err)
		}
		if err := c.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
			return fmt.Errorf("wait for room projections before leave: %w", err)
		}
		if !c.RoomMembership.IsMember(room_id, user_id) {
			return nil
		}

		if err := c.appendRoomLeaveBatch(ctx, kind, room_id, user_id, expectedSeq); err == nil {
			c.logger.Info("Deleted room membership", "user_id", user_id, "kind", kind, "room_id", room_id)
			return nil
		} else if !errors.Is(err, events.ErrConflict) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("publish room leave retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// RemoveMember removes another user's explicit channel-room membership.
// Authorization: caller must verify room.manage before calling.
//
// The public membership transition remains a UserLeftRoomEvent with the target
// user as actor. A separate moderation event records who performed the removal.
func (c *ChattoCore) RemoveMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (bool, error) {
	if kind == KindDM {
		return false, invalidArgument("DM room participants cannot be managed through RoomService")
	}
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.GetUniversal() {
		return false, invalidArgument("universal room membership cannot be managed explicitly")
	}
	if room.GetArchived() {
		return false, ErrRoomArchived
	}
	if _, err := c.GetUser(ctx, targetUserID); err != nil {
		return false, err
	}
	agg := events.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return false, fmt.Errorf("read room membership remove OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
				return false, fmt.Errorf("wait for room directory projection before member remove: %w", err)
			}
		}
		if !c.RoomMembership.IsMember(roomID, targetUserID) {
			return false, nil
		}

		auditEvent := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_RoomMemberRemoved{
				RoomMemberRemoved: &corev1.RoomMemberRemovedEvent{
					RoomId: roomID,
					UserId: targetUserID,
				},
			},
		})

		if err := c.appendRoomLeaveBatch(ctx, kind, roomID, targetUserID, expectedSeq, auditEvent); err == nil {
			c.logger.Info("Removed room membership", "actor_id", actorID, "user_id", targetUserID, "kind", kind, "room_id", roomID)
			return true, nil
		} else if !errors.Is(err, events.ErrConflict) {
			return false, err
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return false, fmt.Errorf("publish room member remove retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

type roomLeaveCallCleanup struct {
	kind        RoomKind
	roomID      string
	userID      string
	callID      string
	endedKeyRef string
}

func (c *ChattoCore) waitForRoomLeaveTail(ctx context.Context, filter string, seq uint64) error {
	if seq == 0 {
		return nil
	}
	pos := events.SubjectPosition(filter, seq)
	if err := c.roomModel.waitForDirectory(ctx, pos); err != nil {
		return err
	}
	if c.CallStateProjector != nil {
		if err := c.CallStateProjector.WaitFor(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChattoCore) appendRoomLeaveBatch(ctx context.Context, kind RoomKind, roomID, userID string, expectedSeq uint64, prefixEvents ...*corev1.Event) error {
	agg := events.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()

	leaveEvent := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{
				RoomId: roomID,
			},
		},
	})

	eventsToAppend := make([]*corev1.Event, 0, len(prefixEvents)+3)
	eventsToAppend = append(eventsToAppend, prefixEvents...)
	eventsToAppend = append(eventsToAppend, leaveEvent)

	var cleanup roomLeaveCallCleanup
	if c.CallState != nil {
		snapshot := c.CallState.RoomSnapshot(roomID)
		if participant, ok := callParticipantByUser(snapshot.Participants, userID); ok {
			callID := participant.CallID
			if callID == "" {
				callID = snapshot.Call.CallID
			}
			eventsToAppend = append(eventsToAppend, newCallParticipantEvent(roomID, userID, callID, false, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
			cleanup = roomLeaveCallCleanup{
				kind:   kind,
				roomID: roomID,
				userID: userID,
				callID: callID,
			}
			if len(snapshot.Participants) == 1 && snapshot.Call.CallID == callID {
				eventsToAppend = append(eventsToAppend, newCallEndedEvent(roomID, userID, callID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
				cleanup.endedKeyRef = snapshot.Call.E2EEKeyRef
			}
		}
	}

	entries := make([]events.BatchEntry, 0, len(eventsToAppend))
	for i, event := range eventsToAppend {
		entry := events.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		}
		if i == 0 {
			entry.ExpectedSeq = expectedSeq
			entry.FilterSubject = filter
			entry.HasOCC = true
		}
		entries = append(entries, entry)
	}

	seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		return fmt.Errorf("publish room leave batch: %w", err)
	}
	pos := events.SubjectPosition(filter, seqs[len(seqs)-1])

	var cleanupErr error
	if cleanup.endedKeyRef != "" {
		if c.callModel == nil || c.callModel.callKeys == nil {
			cleanupErr = fmt.Errorf("call key store is not initialized")
		} else {
			if err := c.callModel.cleanupQueuedCallKey(ctx, cleanup.endedKeyRef); err != nil {
				cleanupErr = fmt.Errorf("shred ended call key: %w", err)
			}
		}
	}

	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
		return err
	}
	if cleanup.callID != "" && c.CallStateProjector != nil {
		if err := c.CallStateProjector.WaitFor(ctx, pos); err != nil {
			return err
		}
	}
	c.removeLiveKitParticipantAfterRoomLeave(ctx, cleanup)
	return cleanupErr
}

func (c *ChattoCore) removeLiveKitParticipantAfterRoomLeave(ctx context.Context, cleanup roomLeaveCallCleanup) {
	if cleanup.callID == "" || c.callModel == nil {
		return
	}
	if err := c.callModel.RemoveLiveKitParticipant(ctx, cleanup.kind, cleanup.roomID, cleanup.callID, cleanup.userID); err != nil {
		c.logger.Warn("Failed to remove room-leaving participant from LiveKit call", "room_id", cleanup.roomID, "call_id", cleanup.callID, "error", err)
	}
}

func (c *ChattoCore) appendRoomMembershipAuditBatch(ctx context.Context, roomID string, expectedSeq uint64, auditEvent, membershipEvent *corev1.Event) error {
	agg := events.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()

	entries := []events.BatchEntry{
		{
			Subject:       agg.SubjectFor(auditEvent),
			Event:         auditEvent,
			ExpectedSeq:   expectedSeq,
			FilterSubject: filter,
			HasOCC:        true,
		},
		{
			Subject: agg.SubjectFor(membershipEvent),
			Event:   membershipEvent,
		},
	}
	seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		return fmt.Errorf("publish room membership audit batch: %w", err)
	}

	lastSubject := entries[len(entries)-1].Subject
	lastSeq := seqs[len(seqs)-1]
	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
		return err
	}
	return nil
}

func (c *ChattoCore) initializeRoomReadMarker(ctx context.Context, kind RoomKind, userID, roomID string) {
	// Initialize the read marker for new members. For non-empty rooms, mark
	// them caught up to the current last event so existing messages don't
	// surface as unread. For empty rooms, write an empty-string sentinel so
	// the key's presence still distinguishes "member with nothing to read
	// yet" from "no marker at all" (which the lazy-init path treats as a
	// deploy-era upgrade — see GetLastReadEventID).
	var initEventID string
	if lastID, _, exists, err := c.GetRoomLastEvent(ctx, kind, roomID); err != nil {
		c.logger.Warn("Failed to get room last event during join", "error", err, "room_id", roomID)
	} else if exists {
		initEventID = lastID
	}
	if err := c.SetLastReadEventID(ctx, kind, userID, roomID, initEventID); err != nil {
		c.logger.Warn("Failed to initialize read marker during join", "error", err, "room_id", roomID)
	}
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
	rooms, err := c.ListMemberRooms(ctx, kind, user_id, MemberRoomListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*corev1.RoomMembership, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &corev1.RoomMembership{
			UserId: user_id,
			RoomId: room.Id,
		})
	}
	return out, nil
}

// GetAllUserRoomMemberships retrieves all of a user's room memberships
// across every kind. Reads from the RoomMembership projection
// (ADR-035 phase 5 cutover).
func (c *ChattoCore) GetAllUserRoomMemberships(ctx context.Context, user_id string) ([]*corev1.RoomMembership, error) {
	channelRooms, err := c.ListMemberRooms(ctx, KindChannel, user_id, MemberRoomListOptions{})
	if err != nil {
		return nil, err
	}
	dmRooms, err := c.ListMemberRooms(ctx, KindDM, user_id, MemberRoomListOptions{})
	if err != nil {
		return nil, err
	}
	rooms := append(channelRooms, dmRooms...)
	out := make([]*corev1.RoomMembership, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &corev1.RoomMembership{
			UserId: user_id,
			RoomId: room.Id,
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

	var lastSubject string
	var lastSeq uint64
	for _, entry := range entries {
		agg := events.RoomAggregate(entry.roomID)
		filter := agg.AllEventsFilter()
		published := false
		for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
			expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
			if err != nil {
				c.logger.Warn("Failed to read room leave OCC tail during user cleanup", "room_id", entry.roomID, "error", err)
				break
			}
			if err := c.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
				c.logger.Warn("Failed to wait for room projections during user cleanup", "room_id", entry.roomID, "error", err)
				break
			}
			if err := c.appendRoomLeaveBatch(ctx, kind, entry.roomID, user_id, expectedSeq); err != nil {
				if errors.Is(err, events.ErrConflict) {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
					}
					continue
				}
				c.logger.Warn("Failed to publish UserLeftRoomEvent to EVT", "room_id", entry.roomID, "error", err)
				break
			}
			pos, err := c.EventPublisher.LastSubjectPosition(ctx, filter)
			if err != nil {
				c.logger.Warn("Failed to read room leave position after user cleanup", "room_id", entry.roomID, "error", err)
				break
			}
			if pos.Seq > lastSeq {
				lastSubject = filter
				lastSeq = pos.Seq
			}
			published = true
			break
		}
		if !published {
			c.logger.Warn("Failed to publish UserLeftRoomEvent to EVT after retries", "room_id", entry.roomID)
		}

	}

	if len(entries) > 0 {
		c.logger.Info("Deleted user room memberships", "user_id", user_id, "kind", kind, "count", len(entries))
	}

	if lastSeq > 0 {
		if err := c.roomModel.waitForDirectory(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
			return fmt.Errorf("wait for room directory projection after membership cleanup: %w", err)
		}
		if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
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
func (c *ChattoCore) GetRoomMembersList(ctx context.Context, kind RoomKind, room_id string) ([]*corev1.RoomMembership, error) {
	userIDs := c.RoomMembership.Members(room_id)
	seen := make(map[string]struct{}, len(userIDs))
	out := make([]*corev1.RoomMembership, 0, len(userIDs))
	add := func(uid string) {
		if uid == "" {
			return
		}
		if _, ok := seen[uid]; ok {
			return
		}
		seen[uid] = struct{}{}
		out = append(out, &corev1.RoomMembership{UserId: uid, RoomId: room_id})
	}
	for _, uid := range userIDs {
		add(uid)
	}

	if kind == KindChannel {
		room, err := c.GetRoom(ctx, kind, room_id)
		if err != nil {
			return nil, err
		}
		if room.GetUniversal() {
			users, err := c.ListUsers(ctx)
			if err != nil {
				return nil, err
			}
			for _, user := range users {
				if user == nil || user.GetId() == "" {
					continue
				}
				if _, explicit := seen[user.GetId()]; explicit {
					continue
				}
				canJoin, err := c.CanJoinRoomAt(ctx, user.GetId(), kind, room_id)
				if err != nil {
					return nil, err
				}
				if canJoin {
					add(user.GetId())
				}
			}
		}
	}
	return out, nil
}

func (c *ChattoCore) ListRoomMemberReferences(ctx context.Context, actorID, roomID string) ([]*corev1.User, error) {
	room, kind, err := c.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	return c.roomMemberReferences(ctx, kind, room.GetId())
}

// ListRoomMemberReferencesForList authorizes the public room-member listing.
// Existing members and channel-room managers may list their room. Other
// channel-room nonmembers need both room.list and room.join; DMs retain their
// membership-only privacy boundary.
func (c *ChattoCore) ListRoomMemberReferencesForList(ctx context.Context, actorID, roomID string) ([]*corev1.User, error) {
	return c.listRoomMemberReferencesForRead(ctx, actorID, roomID, true)
}

// ListRoomMemberReferencesForLookup authorizes singular and batch member
// hydration. Existing members and channel-room managers may hydrate rows; DMs
// retain their membership-only privacy boundary.
func (c *ChattoCore) ListRoomMemberReferencesForLookup(ctx context.Context, actorID, roomID string) ([]*corev1.User, error) {
	return c.listRoomMemberReferencesForRead(ctx, actorID, roomID, false)
}

func (c *ChattoCore) listRoomMemberReferencesForRead(ctx context.Context, actorID, roomID string, allowDiscoverableNonmember bool) ([]*corev1.User, error) {
	room, kind, err := c.requireRoomMember(ctx, actorID, roomID)
	if err == nil {
		return c.roomMemberReferences(ctx, kind, room.GetId())
	}
	if !errors.Is(err, ErrNotRoomMember) {
		return nil, err
	}

	room, err = c.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	kind = KindOfRoom(room)
	if kind == KindDM {
		return nil, ErrNotRoomMember
	}
	canManage, err := c.hasRoomPermission(ctx, kind, room.GetId(), actorID, PermRoomManage)
	if err != nil {
		return nil, err
	}
	if canManage {
		return c.roomMemberReferences(ctx, kind, room.GetId())
	}
	if !allowDiscoverableNonmember || room.GetArchived() {
		return nil, ErrNotRoomMember
	}
	canList, err := c.hasRoomPermission(ctx, kind, room.GetId(), actorID, PermRoomList)
	if err != nil {
		return nil, err
	}
	canJoin, err := c.CanJoinRoomAt(ctx, actorID, kind, room.GetId())
	if err != nil {
		return nil, err
	}
	if !canList || !canJoin {
		return nil, ErrPermissionDenied
	}
	return c.roomMemberReferences(ctx, kind, room.GetId())
}

func (c *ChattoCore) roomMemberReferences(ctx context.Context, kind RoomKind, roomID string) ([]*corev1.User, error) {
	memberships, err := c.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, len(memberships))
	for i, membership := range memberships {
		userIDs[i] = membership.GetUserId()
	}
	users := make([]*corev1.User, 0, len(memberships))
	references, err := c.Users.GetReferencesContext(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for i, user := range references {
		if user == nil {
			user = DeletedUserReference(userIDs[i])
		}
		if user != nil {
			users = append(users, user)
		}
	}
	return users, nil
}
