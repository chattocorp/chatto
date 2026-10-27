package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

const maxJoinRoomRetries = 5

// GetRoomMembership retrieves a room membership for a user in a specific room.
// Reads explicit membership through RoomModel (ADR-035 phase 5 cutover).
// kind is ignored — roomID is globally unique, so the (roomID, userID)
// pair fully identifies a membership.
func (m *RoomMembershipModel) GetRoomMembership(ctx context.Context, kind RoomKind, user_id, room_id string) (*readmodel.RoomMembership, error) {
	if !m.rooms.hasExplicitRoomMembership(room_id, user_id) {
		return nil, fmt.Errorf("room membership for user %s in room %s: %w", user_id, room_id, ErrNotFound)
	}
	return &readmodel.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}, nil
}

// RoomMembershipExists checks if a user is an effective member of a room.
// AuthorizationModel owns this read because universal channels derive
// membership from join authority. This method is a convenience for
// consumers that already depend on the membership model; it adds no logic.
func (m *RoomMembershipModel) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	return m.policy.RoomMembershipExists(ctx, kind, user_id, room_id)
}

// JoinRoom creates a room membership for a user.
// Idempotent: calling it multiple times with the same parameters is a no-op
// (the projection's Apply is idempotent on already-present (room, user)
// pairs, and we early-out via IsMember).
// Authorization: Caller must verify CanJoinRoomAt before calling.
//
// Event-only. Publishes UserJoinedRoomEvent to EVT, then WaitForSeq on the
// projections that serve membership and room history reads.
func (m *RoomMembershipModel) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*readmodel.RoomMembership, error) {
	// Verify room exists and is not archived
	room, err := m.rooms.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}
	if kind == KindChannel && m.rooms.isRoomBanActive(room_id, user_id, time.Now()) {
		return nil, ErrPermissionDenied
	}

	membership := &readmodel.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}
	if kind == KindChannel && room.GetUniversal() {
		return membership, nil
	}

	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_UserJoinedRoom{
			UserJoinedRoom: &evtv1.UserJoinedRoomEvent{
				RoomId: room_id,
			},
		},
	})

	joinSubject := evtstream.RoomAggregate(room_id).SubjectFor(event)
	var seq uint64
	for attempt := range maxJoinRoomRetries {
		if m.rooms.hasExplicitRoomMembership(room_id, user_id) {
			return membership, nil
		}

		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, joinSubject)
		if err != nil {
			return nil, fmt.Errorf("read UserJoinedRoomEvent OCC seq: %w", err)
		}
		if expectedSeq > 0 {
			if err := m.rooms.waitForDirectory(ctx, events.SubjectPosition(joinSubject, expectedSeq)); err != nil {
				return nil, fmt.Errorf("wait for room directory projection before join: %w", err)
			}
			if m.rooms.hasExplicitRoomMembership(room_id, user_id) {
				return membership, nil
			}
		}

		seq, err = m.publisher.AppendAt(ctx, joinSubject, event, events.ExpectSubjectSeq(expectedSeq))
		if err == nil {
			if err := m.rooms.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(joinSubject, seq)); err != nil {
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

	m.logger.Info("Created room membership", "user_id", user_id, "kind", kind, "room_id", room_id)

	m.initializeRoomReadMarker(ctx, kind, user_id, room_id)

	return membership, nil
}

// AddMember creates an explicit channel-room membership for another user.
// Authorization: caller must verify room.manage before calling.
//
// The membership transition is still represented by UserJoinedRoomEvent with
// the target user as actor, so existing membership projections and public room
// history remain compatible. A separate moderation event records the manager
// action for audit.
func (m *RoomMembershipModel) AddMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (*readmodel.RoomMembership, error) {
	return m.addMember(ctx, actorID, kind, roomID, targetUserID, nil)
}

// addMember rechecks command authorization after each room aggregate catch-up.
func (m *RoomMembershipModel) addMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string, authorize func() error) (*readmodel.RoomMembership, error) {
	if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
		return nil, err
	}
	if kind == KindDM {
		return nil, invalidArgument("DM room participants cannot be managed through RoomService")
	}
	room, err := m.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	if room.GetUniversal() {
		return nil, invalidArgument("universal room membership cannot be managed explicitly")
	}
	if room.GetArchived() {
		return nil, ErrRoomArchived
	}
	if _, err := m.users.GetUser(ctx, targetUserID); err != nil {
		return nil, err
	}

	membership := &readmodel.RoomMembership{
		UserId: targetUserID,
		RoomId: roomID,
	}

	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := range maxJoinRoomRetries {
		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read room membership add OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := m.rooms.waitForDirectory(ctx, events.SubjectPosition(filter, expectedSeq)); err != nil {
				return nil, fmt.Errorf("wait for room directory projection before member add: %w", err)
			}
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return nil, err
		}
		// Room lifecycle changes use the same room aggregate. Check the
		// projected state after catching up to the OCC tail on every attempt.
		room, err := m.rooms.GetRoom(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		if room.GetUniversal() {
			return nil, invalidArgument("universal room membership cannot be managed explicitly")
		}
		if room.GetArchived() {
			return nil, ErrRoomArchived
		}
		if m.rooms.hasExplicitRoomMembership(roomID, targetUserID) {
			return membership, nil
		}
		if kind == KindChannel && m.rooms.isRoomBanActive(roomID, targetUserID, time.Now()) {
			return nil, ErrPermissionDenied
		}

		auditEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomMemberAdded{
				RoomMemberAdded: &evtv1.RoomMemberAddedEvent{
					RoomId: roomID,
					UserId: targetUserID,
				},
			},
		})
		joinEvent := newEvent(targetUserID, &evtv1.Event{
			Event: &evtv1.Event_UserJoinedRoom{
				UserJoinedRoom: &evtv1.UserJoinedRoomEvent{
					RoomId: roomID,
				},
			},
		})

		if err := m.appendRoomMembershipAuditBatch(ctx, roomID, expectedSeq, auditEvent, joinEvent); err == nil {
			m.initializeRoomReadMarker(ctx, kind, targetUserID, roomID)
			m.logger.Info("Added room membership", "actor_id", actorID, "user_id", targetUserID, "kind", kind, "room_id", roomID)
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
func (m *RoomMembershipModel) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	if kind == KindDM {
		return ErrCannotLeaveDMConversation
	}

	room, err := m.rooms.GetRoom(ctx, kind, room_id)
	if err != nil {
		if errors.Is(err, ErrNotFound) && !m.rooms.hasExplicitRoomMembership(room_id, user_id) {
			return nil
		}
		return err
	}
	if kind == KindChannel && room.GetUniversal() {
		return ErrCannotLeaveUniversalRoom
	}

	agg := evtstream.RoomAggregate(room_id)
	filter := agg.AllEventsFilter()
	for attempt := range maxJoinRoomRetries {
		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return fmt.Errorf("read room leave OCC tail: %w", err)
		}
		if err := m.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
			return fmt.Errorf("wait for room projections before leave: %w", err)
		}
		if !m.rooms.hasExplicitRoomMembership(room_id, user_id) {
			return nil
		}

		if err := m.appendRoomLeaveBatch(ctx, kind, room_id, user_id, expectedSeq); err == nil {
			m.logger.Info("Deleted room membership", "user_id", user_id, "kind", kind, "room_id", room_id)
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
func (m *RoomMembershipModel) RemoveMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (bool, error) {
	return m.removeMember(ctx, actorID, kind, roomID, targetUserID, nil)
}

// removeMember rechecks command authorization after each room aggregate catch-up.
func (m *RoomMembershipModel) removeMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string, authorize func() error) (bool, error) {
	if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
		return false, err
	}
	if kind == KindDM {
		return false, invalidArgument("DM room participants cannot be managed through RoomService")
	}
	room, err := m.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if room.GetUniversal() {
		return false, invalidArgument("universal room membership cannot be managed explicitly")
	}
	if authorize == nil && room.GetArchived() {
		return false, ErrRoomArchived
	}
	if _, err := m.users.GetUser(ctx, targetUserID); err != nil {
		return false, err
	}
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := range maxJoinRoomRetries {
		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return false, fmt.Errorf("read room membership remove OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := m.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
				return false, fmt.Errorf("wait for room directory projection before member remove: %w", err)
			}
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return false, err
		}
		if !m.rooms.hasExplicitRoomMembership(roomID, targetUserID) {
			return false, nil
		}

		auditEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomMemberRemoved{
				RoomMemberRemoved: &evtv1.RoomMemberRemovedEvent{
					RoomId: roomID,
					UserId: targetUserID,
				},
			},
		})

		if err := m.appendRoomLeaveBatch(ctx, kind, roomID, targetUserID, expectedSeq, auditEvent); err == nil {
			m.logger.Info("Removed room membership", "actor_id", actorID, "user_id", targetUserID, "kind", kind, "room_id", roomID)
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

func (m *RoomMembershipModel) waitForRoomLeaveTail(ctx context.Context, filter string, seq uint64) error {
	if seq == 0 {
		return nil
	}
	pos := events.SubjectPosition(filter, seq)
	if err := m.rooms.waitForDirectory(ctx, pos); err != nil {
		return err
	}
	if err := m.calls.waitFor(ctx, pos); err != nil {
		return err
	}
	return nil
}

func (m *RoomMembershipModel) appendRoomLeaveBatch(ctx context.Context, kind RoomKind, roomID, userID string, expectedSeq uint64, prefixEvents ...*evtv1.Event) error {
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()

	leaveEvent := newEvent(userID, &evtv1.Event{
		Event: &evtv1.Event_UserLeftRoom{
			UserLeftRoom: &evtv1.UserLeftRoomEvent{
				RoomId: roomID,
			},
		},
	})

	eventsToAppend := make([]*evtv1.Event, 0, len(prefixEvents)+3)
	eventsToAppend = append(eventsToAppend, prefixEvents...)
	eventsToAppend = append(eventsToAppend, leaveEvent)

	var cleanup roomLeaveCallCleanup
	if m.calls != nil {
		snapshot := m.calls.roomSnapshot(roomID)
		if participant, ok := callParticipantByUser(snapshot.Participants, userID); ok {
			callID := participant.CallID
			if callID == "" {
				callID = snapshot.Call.CallID
			}
			eventsToAppend = append(eventsToAppend, newCallParticipantEvent(roomID, userID, callID, false, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
			cleanup = roomLeaveCallCleanup{
				kind:   kind,
				roomID: roomID,
				userID: userID,
				callID: callID,
			}
			if len(snapshot.Participants) == 1 && snapshot.Call.CallID == callID {
				eventsToAppend = append(eventsToAppend, newCallEndedEvent(roomID, userID, callID, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
				cleanup.endedKeyRef = snapshot.Call.E2EEKeyRef
			}
		}
	}

	entries := make([]evtstream.BatchEntry, 0, len(eventsToAppend))
	for i, event := range eventsToAppend {
		entry := evtstream.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		}
		if i == 0 {
			entry.Expect = events.ExpectFilterSeq(filter, expectedSeq)
		}
		entries = append(entries, entry)
	}

	seqs, err := m.publisher.AppendBatch(ctx, entries)
	if err != nil {
		return fmt.Errorf("publish room leave batch: %w", err)
	}
	leaveSequence := seqs[len(prefixEvents)]
	// Record the committed cutoff before a rejoin can make an older delayed
	// source look visible again. Creation itself remains owned exclusively by
	// the causally ordered durable worker.
	if m.notifications != nil {
		if err := m.notifications.recordVisibilityBoundary(ctx, userID, roomID, leaveSequence); err != nil {
			return fmt.Errorf("record notification visibility boundary: %w", err)
		}
	}
	pos := events.SubjectPosition(filter, seqs[len(eventsToAppend)-1])

	cleanupErr := m.calls.cleanupMembershipEndedCallKey(ctx, cleanup.endedKeyRef)

	if err := m.rooms.waitForDirectoryAndTimeline(ctx, pos); err != nil {
		return err
	}
	if cleanup.callID != "" {
		if err := m.calls.waitFor(ctx, pos); err != nil {
			return err
		}
	}
	m.removeLiveKitParticipantAfterRoomLeave(ctx, cleanup)
	return cleanupErr
}

func (m *RoomMembershipModel) removeLiveKitParticipantAfterRoomLeave(ctx context.Context, cleanup roomLeaveCallCleanup) {
	if cleanup.callID == "" || m.calls == nil {
		return
	}
	if err := m.calls.RemoveLiveKitParticipant(ctx, cleanup.kind, cleanup.roomID, cleanup.callID, cleanup.userID); err != nil {
		m.logger.Warn("Failed to remove room-leaving participant from LiveKit call", "room_id", cleanup.roomID, "call_id", cleanup.callID, "error", err)
	}
}

func (m *RoomMembershipModel) appendRoomMembershipAuditBatch(ctx context.Context, roomID string, expectedSeq uint64, auditEvent, membershipEvent *evtv1.Event) error {
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()

	entries := []evtstream.BatchEntry{
		{
			Subject: agg.SubjectFor(auditEvent),
			Event:   auditEvent,
			Expect:  events.ExpectFilterSeq(filter, expectedSeq),
		},
		{
			Subject: agg.SubjectFor(membershipEvent),
			Event:   membershipEvent,
		},
	}
	seqs, err := m.publisher.AppendBatch(ctx, entries)
	if err != nil {
		return fmt.Errorf("publish room membership audit batch: %w", err)
	}

	lastSubject := entries[len(entries)-1].Subject
	lastSeq := seqs[len(seqs)-1]
	if err := m.rooms.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
		return err
	}
	return nil
}

func (m *RoomMembershipModel) initializeRoomReadMarker(ctx context.Context, kind RoomKind, userID, roomID string) {
	// Initialize the read marker for new members. For non-empty rooms, mark
	// them caught up to the current last event so existing messages don't
	// surface as unread. For empty rooms, write an empty-string sentinel so
	// the key's presence still distinguishes "member with nothing to read
	// yet" from "no marker at all" (which the lazy-init path treats as a
	// deploy-era upgrade — see GetLastReadEventID).
	var initEventID string
	if lastID, _, exists, err := m.readState.GetRoomLastEvent(ctx, kind, roomID); err != nil {
		m.logger.Warn("Failed to get room last event during join", "error", err, "room_id", roomID)
	} else if exists {
		initEventID = lastID
	}
	if err := m.readState.initializeLastReadEventID(ctx, userID, roomID, initEventID); err != nil {
		m.logger.Warn("Failed to initialize read marker during join", "error", err, "room_id", roomID)
	}
}

// GetUserRoomMemberships retrieves all room memberships for a given user of a
// given kind. The membership projection doesn't track kind, so ListMemberRooms
// filters the user's projected room IDs through RoomModel's projected room
// catalog. This is O(N) in the user's room count and uses local projection
// reads; each user has a bounded number of rooms.
func (m *RoomMembershipModel) GetUserRoomMemberships(ctx context.Context, kind RoomKind, user_id string) ([]*readmodel.RoomMembership, error) {
	rooms, err := m.ListMemberRooms(ctx, kind, user_id, MemberRoomListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*readmodel.RoomMembership, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, &readmodel.RoomMembership{
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
func (m *RoomMembershipModel) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id string, kind RoomKind) error {
	allRoomIDs := m.rooms.explicitRoomIDsForUser(user_id)
	if len(allRoomIDs) == 0 {
		return nil
	}

	type roomEntry struct {
		roomID string
	}
	var entries []roomEntry
	for _, roomID := range allRoomIDs {
		// Filter by kind: GetRoom returns ErrNotFound when the
		// room exists but is of a different kind.
		if _, err := m.rooms.GetRoom(ctx, kind, roomID); err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return fmt.Errorf("lookup room %s during membership cleanup: %w", roomID, err)
		}
		entries = append(entries, roomEntry{roomID: roomID})
	}

	var lastSubject string
	var lastSeq uint64
	for _, entry := range entries {
		agg := evtstream.RoomAggregate(entry.roomID)
		filter := agg.AllEventsFilter()
		published := false
		for attempt := range maxJoinRoomRetries {
			expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
			if err != nil {
				m.logger.Warn("Failed to read room leave OCC tail during user cleanup", "room_id", entry.roomID, "error", err)
				break
			}
			if err := m.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
				m.logger.Warn("Failed to wait for room projections during user cleanup", "room_id", entry.roomID, "error", err)
				break
			}
			if err := m.appendRoomLeaveBatch(ctx, kind, entry.roomID, user_id, expectedSeq); err != nil {
				if errors.Is(err, events.ErrConflict) {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
					}
					continue
				}
				m.logger.Warn("Failed to publish UserLeftRoomEvent to EVT", "room_id", entry.roomID, "error", err)
				break
			}
			pos, err := m.publisher.LastSubjectPosition(ctx, filter)
			if err != nil {
				m.logger.Warn("Failed to read room leave position after user cleanup", "room_id", entry.roomID, "error", err)
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
			m.logger.Warn("Failed to publish UserLeftRoomEvent to EVT after retries", "room_id", entry.roomID)
		}

	}

	if len(entries) > 0 {
		m.logger.Info("Deleted user room memberships", "user_id", user_id, "kind", kind, "count", len(entries))
	}

	if lastSeq > 0 {
		if err := m.rooms.waitForDirectory(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
			return fmt.Errorf("wait for room directory projection after membership cleanup: %w", err)
		}
		if err := m.rooms.waitForTimeline(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
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
func (m *RoomMembershipModel) GetRoomMembersList(ctx context.Context, kind RoomKind, room_id string) ([]*readmodel.RoomMembership, error) {
	userIDs := m.rooms.explicitRoomMemberIDs(room_id)
	seen := make(map[string]struct{}, len(userIDs))
	out := make([]*readmodel.RoomMembership, 0, len(userIDs))
	add := func(uid string) {
		if uid == "" {
			return
		}
		if _, ok := seen[uid]; ok {
			return
		}
		seen[uid] = struct{}{}
		out = append(out, &readmodel.RoomMembership{UserId: uid, RoomId: room_id})
	}
	for _, uid := range userIDs {
		add(uid)
	}

	if kind == KindChannel {
		room, err := m.rooms.GetRoom(ctx, kind, room_id)
		if err != nil {
			return nil, err
		}
		if room.GetUniversal() {
			for _, userID := range m.users.allActiveIDs() {
				if _, explicit := seen[userID]; explicit {
					continue
				}
				canJoin, err := m.policy.CanJoinRoomAt(ctx, userID, kind, room_id)
				if err != nil {
					return nil, err
				}
				if canJoin {
					add(userID)
				}
			}
		}
	}
	return out, nil
}

func (m *RoomMembershipModel) ListRoomMemberReferences(ctx context.Context, actorID, roomID string) ([]*readmodel.User, error) {
	room, kind, err := m.policy.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	return m.roomMemberReferences(ctx, kind, room.GetId())
}

// ListRoomMemberReferencesForList authorizes the public room-member listing.
// Existing members and channel-room managers may list their room. Other
// channel-room nonmembers need both room.list and room.join; DMs retain their
// membership-only privacy boundary.
func (m *RoomMembershipModel) ListRoomMemberReferencesForList(ctx context.Context, actorID, roomID string) ([]*readmodel.User, error) {
	return m.listRoomMemberReferencesForRead(ctx, actorID, roomID, true)
}

// ListRoomMemberIDsForList authorizes the public room-member listing and
// returns projected membership IDs without hydrating user PII. Deleted or
// unavailable user records can still have a membership ID; callers that return
// user resources must remove IDs that they cannot hydrate.
func (m *RoomMembershipModel) ListRoomMemberIDsForList(ctx context.Context, actorID, roomID string) ([]string, error) {
	return m.listRoomMemberIDsForRead(ctx, actorID, roomID, true)
}

// ListActiveRoomMemberIDs authorizes a membership listing and removes deleted,
// shredded, and unknown users without reading or decrypting user profiles.
func (m *RoomMembershipModel) ListActiveRoomMemberIDs(ctx context.Context, actorID, roomID string) ([]string, error) {
	ids, err := m.ListRoomMemberIDsForList(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	return m.users.activeIDs(ids), nil
}

// ListRoomMemberReferencesForLookup authorizes member hydration for room members
// and channel-room managers. DMs require membership.
func (m *RoomMembershipModel) ListRoomMemberReferencesForLookup(ctx context.Context, actorID, roomID string) ([]*readmodel.User, error) {
	return m.listRoomMemberReferencesForRead(ctx, actorID, roomID, false)
}

// GetRoomMemberReferencesForLookup reads only the requested accounts. Channel
// membership may be read by room members, room managers, account managers, and
// managers of the requested bot. Bot ownership never exposes other members.
// DMs retain their membership-only privacy boundary.
func (m *RoomMembershipModel) GetRoomMemberReferencesForLookup(ctx context.Context, actorID, roomID string, userIDs []string) ([]*readmodel.User, error) {
	room, kind, err := m.policy.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		if !errors.Is(err, ErrNotRoomMember) {
			return nil, err
		}
		room, err = m.rooms.FindRoomByID(ctx, roomID)
		if err != nil {
			return nil, err
		}
		kind = KindOfRoom(room)
		if kind == KindDM {
			return nil, ErrNotRoomMember
		}
		roomManager, err := m.policy.hasRoomPermission(ctx, kind, roomID, actorID, PermRoomManage)
		if err != nil {
			return nil, err
		}
		accountManager, err := m.policy.CanManageUserAccounts(ctx, actorID)
		if err != nil {
			return nil, err
		}
		if !roomManager && !accountManager {
			for _, userID := range userIDs {
				if _, err := m.policy.requireBotManager(ctx, actorID, userID); err != nil {
					if errors.Is(err, ErrNotFound) || errors.Is(err, ErrHumanAccountRequired) {
						return nil, ErrPermissionDenied
					}
					return nil, err
				}
			}
		}
	}
	memberIDs := make([]string, 0, len(userIDs))
	seen := make(map[string]bool, len(userIDs))
	for _, userID := range userIDs {
		if seen[userID] {
			continue
		}
		seen[userID] = true
		joined, err := m.RoomMembershipExists(ctx, kind, userID, roomID)
		if err != nil {
			return nil, err
		}
		if joined {
			memberIDs = append(memberIDs, userID)
		}
	}
	return m.userReferencesForIDs(ctx, memberIDs)
}

func (m *RoomMembershipModel) listRoomMemberReferencesForRead(ctx context.Context, actorID, roomID string, allowDiscoverableNonmember bool) ([]*readmodel.User, error) {
	userIDs, err := m.listRoomMemberIDsForRead(ctx, actorID, roomID, allowDiscoverableNonmember)
	if err != nil {
		return nil, err
	}
	return m.userReferencesForIDs(ctx, userIDs)
}

func (m *RoomMembershipModel) listRoomMemberIDsForRead(ctx context.Context, actorID, roomID string, allowDiscoverableNonmember bool) ([]string, error) {
	room, kind, err := m.policy.requireRoomMember(ctx, actorID, roomID)
	if err == nil {
		return m.roomMemberIDs(ctx, kind, room.GetId())
	}
	if !errors.Is(err, ErrNotRoomMember) {
		return nil, err
	}

	room, err = m.rooms.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	kind = KindOfRoom(room)
	if kind == KindDM {
		return nil, ErrNotRoomMember
	}
	canManage, err := m.policy.hasRoomPermission(ctx, kind, room.GetId(), actorID, PermRoomManage)
	if err != nil {
		return nil, err
	}
	if canManage {
		return m.roomMemberIDs(ctx, kind, room.GetId())
	}
	if !allowDiscoverableNonmember || room.GetArchived() {
		return nil, ErrNotRoomMember
	}
	canList, err := m.policy.hasRoomPermission(ctx, kind, room.GetId(), actorID, PermRoomList)
	if err != nil {
		return nil, err
	}
	canJoin, err := m.policy.CanJoinRoomAt(ctx, actorID, kind, room.GetId())
	if err != nil {
		return nil, err
	}
	if !canList || !canJoin {
		return nil, ErrPermissionDenied
	}
	return m.roomMemberIDs(ctx, kind, room.GetId())
}

func (m *RoomMembershipModel) roomMemberReferences(ctx context.Context, kind RoomKind, roomID string) ([]*readmodel.User, error) {
	userIDs, err := m.roomMemberIDs(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	return m.userReferencesForIDs(ctx, userIDs)
}

func (m *RoomMembershipModel) userReferencesForIDs(ctx context.Context, userIDs []string) ([]*readmodel.User, error) {
	users := make([]*readmodel.User, 0, len(userIDs))
	references, err := m.users.userReferences(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, user := range references {
		// User deletion and room-membership cleanup are separate event-sourced
		// transitions. Omit deleted or unknown references while those projections
		// converge, and for legacy histories that retain orphan memberships.
		if user == nil || user.GetDeleted() {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

func (m *RoomMembershipModel) roomMemberIDs(ctx context.Context, kind RoomKind, roomID string) ([]string, error) {
	memberships, err := m.GetRoomMembersList(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	userIDs := make([]string, 0, len(memberships))
	for _, membership := range memberships {
		if membership.GetUserId() != "" {
			userIDs = append(userIDs, membership.GetUserId())
		}
	}
	return userIDs, nil
}
