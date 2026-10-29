package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// findDM returns the existing DM for the caller and participant set.
// The caller is automatically included in the participant set. A missing DM
// returns (nil, false, nil). The caller must be a member of a found DM.
func (s *RoomCommandModel) findDM(ctx context.Context, creatorID string, participantIDs []string) (*readmodel.Room, bool, error) {
	// Ensure creator is in participants
	allParticipants := ensureInList(participantIDs, creatorID)

	if len(allParticipants) < 1 {
		return nil, false, fmt.Errorf("DM requires at least 1 participant")
	}
	if len(allParticipants) > MaxDMParticipants {
		return nil, false, fmt.Errorf("DM conversations are limited to %d participants", MaxDMParticipants)
	}

	roomID := DMRoomID(allParticipants)
	if roomID == "" {
		return nil, false, fmt.Errorf("failed to generate DM room ID")
	}

	// Try to get existing room
	room, err := s.rooms.GetRoom(ctx, KindDM, roomID)
	if err == nil {
		// Room exists - verify caller is a participant
		isMember, err := s.memberships.RoomMembershipExists(ctx, KindDM, creatorID, roomID)
		if err != nil {
			return nil, false, fmt.Errorf("failed to check DM membership: %w", err)
		}
		if !isMember {
			return nil, false, fmt.Errorf("access denied: not a participant in this DM")
		}
		return room, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, fmt.Errorf("failed to check existing DM: %w", err)
	}
	return nil, false, nil
}

// findOrCreateDM finds an existing DM conversation or creates a new one.
// The caller (creatorID) is automatically included in the participant list.
// Returns the room and a boolean indicating whether it was newly created.
//
// Trusted callers must check DM creation policy before calling; public callers
// use StartDM. For existing DMs, the caller must already be a participant.
// For new DMs, all participants are automatically joined to the room.
func (s *RoomCommandModel) findOrCreateDM(ctx context.Context, creatorID string, participantIDs []string) (*readmodel.Room, bool, error) {
	room, found, err := s.findDM(ctx, creatorID, participantIDs)
	if err != nil {
		return nil, false, err
	}
	if found {
		return room, false, nil
	}

	allParticipants := ensureInList(participantIDs, creatorID)
	roomID := DMRoomID(allParticipants)

	// Create new DM room
	room, err = s.createDMRoom(ctx, roomID, allParticipants)
	if err != nil {
		// Another request can create this DM first. The empty-aggregate guard
		// rejects our batch. Catch up locally before checking membership.
		if errors.Is(err, events.ErrConflict) {
			if err := s.rooms.waitForDirectoryCurrent(ctx, s.publisher); err != nil {
				return nil, false, fmt.Errorf("wait for DM after race: %w", err)
			}
			room, found, err = s.findDM(ctx, creatorID, participantIDs)
			if err != nil {
				return nil, false, fmt.Errorf("failed to get DM after race: %w", err)
			}
			if !found {
				return nil, false, fmt.Errorf("DM missing after race: %w", ErrNotFound)
			}
			return room, false, nil
		}
		return nil, false, fmt.Errorf("failed to create DM: %w", err)
	}

	s.logger.Info("Created DM conversation", "room_id", roomID, "participants", len(allParticipants))
	return room, true, nil
}

// createDMRoom creates a new DM room and joins all participants
// atomically via a single AppendBatch — RoomCreatedEvent followed
// by participant UserJoinedRoom events in the same room aggregate, committed
// as one unit. No rollback path is needed: either all events land
// or none do.
//
// Concurrency safety: the first batch entry carries an OCC guard with
// expected sequence 0, so a race where another replica already created
// this DM (same hashed roomID) is rejected with events.ErrConflict
// — the caller (findOrCreateDM) re-fetches.
//
// Post-batch side effects (per-participant read markers) happen after the
// batch acks, since they're outside the durable event log.
func (s *RoomCommandModel) createDMRoom(ctx context.Context, roomID string, participantIDs []string) (*readmodel.Room, error) {
	// Keep stored group conversations readable, but never create new ones.
	if err := validateNewDMParticipants(participantIDs); err != nil {
		return nil, err
	}

	room := &readmodel.Room{
		Id:   roomID,
		Kind: evtv1.RoomKind_ROOM_KIND_DM,
		Name: "", // DMs don't have names - derived from participants in UI
	}

	agg := evtstream.RoomAggregate(roomID)

	// "system" actor reflects that the conversation is created by
	// the platform on the first participant's behalf — DMs have no
	// operator-driven creation flow.
	createdEvent := newEvent("system", &evtv1.Event{
		Event: &evtv1.Event_RoomCreated{
			RoomCreated: &evtv1.RoomCreatedEvent{
				RoomId:      roomID,
				Name:        "",
				Description: "",
				Kind:        evtv1.RoomKind_ROOM_KIND_DM,
			},
		},
	})

	// First entry uses wildcard OCC against the aggregate's full
	// filter — "the entire room aggregate must be empty," not just
	// "no prior RoomCreated event." Preserves the per-aggregate
	// uniqueness guarantee under the per-(agg, event-type) subject
	// shape.
	entries := []evtstream.BatchEntry{
		{
			Subject: agg.SubjectFor(createdEvent),
			Event:   createdEvent,
			Expect:  events.ExpectFilterSeq(agg.AllEventsFilter(), 0),
		},
	}
	for _, pid := range participantIDs {
		joinEvent := newEvent(pid, &evtv1.Event{
			Event: &evtv1.Event_UserJoinedRoom{
				UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID},
			},
		})
		entries = append(entries, evtstream.BatchEntry{
			Subject: agg.SubjectFor(joinEvent),
			Event:   joinEvent,
		})
	}

	seqs, err := s.publisher.AppendBatch(ctx, entries)
	if err != nil {
		// Includes events.ErrConflict on race (room-aggregate OCC
		// rejected because another replica created this DM first).
		return nil, err
	}

	// Wait for the last batch sequence on projections that consume evt.room.>.
	// Reaching the last UserJoinedRoom means the earlier RoomCreated has also
	// landed in the room directory.
	lastSubject := entries[len(entries)-1].Subject
	if err := s.rooms.waitForDirectory(ctx, events.SubjectPosition(lastSubject, seqs[len(seqs)-1])); err != nil {
		s.logger.Warn("DM room directory projection wait failed", "error", err, "room_id", roomID)
	}
	if err := s.rooms.waitForTimeline(ctx, events.SubjectPosition(lastSubject, seqs[len(seqs)-1])); err != nil {
		s.logger.Warn("DM room timeline projection wait failed", "error", err, "room_id", roomID)
	}

	// Per-participant non-batched side effects: initialise the read marker so
	// HasUnread distinguishes a fresh member from a deploy-era user; see
	// GetLastReadEventID.
	for _, pid := range participantIDs {
		if err := s.readState.initializeLastReadEventID(ctx, pid, roomID, ""); err != nil {
			s.logger.Warn("Failed to initialize DM read marker", "error", err, "user_id", pid, "room_id", roomID)
		}
	}

	return room, nil
}

// validateNewDMParticipants applies the product limit only to new conversations.
func validateNewDMParticipants(ids []string) error {
	participants := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		participants[id] = struct{}{}
	}
	if len(participants) > 2 {
		return invalidArgument("New DMs are limited to two participants; use a channel room for group conversations")
	}
	return nil
}
