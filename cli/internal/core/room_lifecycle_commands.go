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

// createRoom serves trusted callers after authorization. It commits creation,
// group membership and optional defaults atomically, then waits for their views.
func (s *RoomCommandModel) createRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string, opts ...CreateRoomOption) (*readmodel.Room, error) {
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}
	options := collectCreateRoomOptions(opts)
	if kind == KindDM && options.universal {
		return nil, fmt.Errorf("DM rooms cannot be universal")
	}
	if kind == KindDM && options.applyAnnouncementsDefaults {
		return nil, fmt.Errorf("DM rooms cannot use announcements defaults")
	}
	if kind == KindDM && options.threadingMode != evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED {
		return nil, invalidArgument("DM rooms cannot configure threading")
	}
	if kind == KindChannel && options.threadingMode != evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED && !IsValidRoomThreadingMode(options.threadingMode) {
		return nil, invalidArgument("invalid room threading mode")
	}
	threadingMode := normalizedRoomThreadingMode(ProtoKindForRoomKind(kind), options.threadingMode)

	if groupID != "" {
		if _, err := s.rooms.GetRoomGroup(ctx, groupID); err != nil {
			return nil, err
		}
	} else if kind == KindChannel {
		groups, err := s.rooms.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			return nil, fmt.Errorf("lookup default group: %w", err)
		}
		if len(groups) > 0 {
			groupID = groups[0].Id
		}
	}

	name = normalizeRoomName(name)
	room_id := NewRoomID()

	room := &readmodel.Room{
		Id:                   room_id,
		Kind:                 ProtoKindForRoomKind(kind),
		Name:                 name,
		Description:          description,
		GroupId:              groupID,
		Universal:            options.universal,
		ThreadingMode:        threadingMode,
		ShowMembershipEvents: true,
	}

	createdEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomCreated{
			RoomCreated: &evtv1.RoomCreatedEvent{
				RoomId:        room_id,
				Name:          name,
				Description:   description,
				Kind:          ProtoKindForRoomKind(kind),
				Universal:     options.universal,
				ThreadingMode: threadingMode,
			},
		},
	})

	var defaultPermissionEntries []evtstream.BatchEntry
	if options.importContext != nil {
		createdEvent.ImportContext = options.importContext
	}
	if kind == KindChannel && options.applyAnnouncementsDefaults {
		defaultPermissionEntries = rbacSeedEntries(nil, nil, defaultAnnouncementsRoomDecisions(room_id))
	}
	additionalEntries := func(ctx context.Context) ([]evtstream.BatchEntry, error) {
		entries := make([]evtstream.BatchEntry, 0, len(defaultPermissionEntries)+1)
		if groupID != "" {
			groupPosition, err := s.publisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
			if err != nil {
				return nil, fmt.Errorf("read room-group OCC position before room creation: %w", err)
			}
			if !groupPosition.IsZero() {
				if err := s.rooms.waitForGroupLayout(ctx, groupPosition); err != nil {
					return nil, fmt.Errorf("wait for room group before room creation: %w", err)
				}
			}
			if _, ok := s.rooms.roomGroup(groupID); !ok {
				return nil, ErrRoomGroupNotFound
			}
			added := newEvent(actorID, &evtv1.Event{
				Event: &evtv1.Event_RoomAddedToGroup{
					RoomAddedToGroup: &evtv1.RoomAddedToGroupEvent{GroupId: groupID, RoomId: room_id},
				},
			})
			entries = append(entries, evtstream.BatchEntry{
				Subject: evtstream.GroupAggregate(groupID).SubjectFor(added),
				Event:   added,
				Expect:  events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), groupPosition.Seq),
			})
		}
		entries = append(entries, defaultPermissionEntries...)
		if options.importReceipt != nil {
			for _, entry := range entries {
				entry.Event.ImportContext = options.importContext
			}
			entries = append(entries, options.importReceipt(room_id))
		}
		return entries, nil
	}
	seqs, err := s.publishRoomEventWithNameOCCEntries(ctx, name, createdEvent, room_id, additionalEntries)
	if err != nil {
		return nil, err
	}
	createdSeq := seqs[0]

	s.logger.Info("Room created", "kind", kind, "room_id", room_id, "group_id", groupID)

	createdSubject := evtstream.RoomAggregate(room_id).SubjectFor(createdEvent)
	if err := s.rooms.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(createdSubject, createdSeq)); err != nil {
		return nil, err
	}
	if groupID != "" {
		groupEntryIndex := 1
		groupSubject := evtstream.GroupAggregate(groupID).Subject(evtstream.EventRoomAddedToGroup)
		if err := s.rooms.waitForGroupLayout(ctx, events.SubjectPosition(groupSubject, seqs[groupEntryIndex])); err != nil {
			return nil, fmt.Errorf("wait for created room group membership: %w", err)
		}
	}
	if len(defaultPermissionEntries) > 0 {
		last := len(defaultPermissionEntries) - 1
		if err := s.rbac.waitFor(ctx, events.SubjectPosition(defaultPermissionEntries[last].Subject, seqs[len(seqs)-1])); err != nil {
			return nil, fmt.Errorf("wait for channel room defaults: %w", err)
		}
	}
	return room, nil
}

// publishRoomEventWithNameOCCEntries publishes a name-claiming room event
// (RoomCreated or RoomUpdated) with cluster-wide name uniqueness enforced via
// JetStream wildcard OCC against `evt.room.>`. The name-claiming event and the
// additional entries commit atomically.
//
// The flow per attempt:
//  1. Read the catalog name-claim snapshot for the desired `name`;
//     if any other room holds it, return ErrRoomNameExists immediately.
//  2. Rebuild the additional entries, so that their expected positions and
//     projection-derived facts describe the same event-log prefix as the
//     commit attempt.
//  3. Publish the event, and the additional entries, with the snapshot's
//     applied evt.room.> seq.
//     The projected state and OCC token describe the same observed
//     event-log prefix.
//  4. JetStream
//     rejects with ErrConflict if any evt.room.> message landed in the
//     read-publish window — backoff briefly and retry.
//
// excludeRoomID is the ID to exclude from the uniqueness check, so that a
// room can keep a name it already holds.
func (s *RoomCommandModel) publishRoomEventWithNameOCCEntries(
	ctx context.Context,
	name string,
	event *evtv1.Event,
	excludeRoomID string,
	buildAdditionalEntries func(context.Context) ([]evtstream.BatchEntry, error),
) ([]uint64, error) {
	// Determine publish subject from the event payload. Room events
	// all target the per-room aggregate subject; this doesn't change
	// across retries.
	var roomID string
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomCreated:
		roomID = e.RoomCreated.GetRoomId()
	case *evtv1.Event_RoomUpdated:
		roomID = e.RoomUpdated.GetRoomId()
	default:
		return nil, fmt.Errorf("publishRoomEventWithNameOCCEntries: unsupported event type %T", e)
	}
	publishSubject := evtstream.RoomAggregate(roomID).SubjectFor(event)
	occFilter := evtstream.RoomSubjectFilter()

	for attempt := range maxRoomNameClaimRetries {
		snapshot := s.rooms.nameClaimSnapshot(name, excludeRoomID)
		if snapshot.ConflictingRoomID != "" {
			return nil, ErrRoomNameExists
		}
		additionalEntries, err := buildAdditionalEntries(ctx)
		if err != nil {
			return nil, err
		}

		var seqs []uint64
		if len(additionalEntries) == 0 {
			var seq uint64
			seq, err = s.publisher.AppendAt(ctx, publishSubject, event, events.ExpectFilterSeq(occFilter, snapshot.Seq))
			seqs = []uint64{seq}
		} else {
			entries := make([]evtstream.BatchEntry, 1, len(additionalEntries)+1)
			entries[0] = evtstream.BatchEntry{
				Subject: publishSubject,
				Event:   event,
				Expect:  events.ExpectFilterSeq(occFilter, snapshot.Seq),
			}
			entries = append(entries, additionalEntries...)
			seqs, err = s.publisher.AppendBatch(ctx, entries)
		}
		if err == nil {
			return seqs, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		if err := s.rooms.waitForDirectoryCurrent(ctx, s.publisher); err != nil {
			return nil, fmt.Errorf("wait for room directory after OCC conflict: %w", err)
		}

		// Filter advanced under us after the snapshot. Backoff briefly
		// and retry — the next attempt reads a fresh projection snapshot.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("room name OCC retry exhausted after %d attempts: %w", maxRoomNameClaimRetries, events.ErrConflict)
}

// archiveRoom serves trusted callers after authorization and waits for room views.
func (s *RoomCommandModel) archiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*readmodel.Room, error) {
	room, err := s.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = true

	archivedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomArchived{
			RoomArchived: &evtv1.RoomArchivedEvent{
				RoomId: roomID,
			},
		},
	})
	pos, err := s.rooms.appendDirectoryEventually(ctx, s.publisher, evtstream.RoomAggregate(roomID), archivedEvent)
	if err != nil {
		return nil, fmt.Errorf("publish RoomArchivedEvent: %w", err)
	}
	if err := s.rooms.waitForTimeline(ctx, pos); err != nil {
		return nil, err
	}

	s.logger.Info("Room archived", "kind", kind, "room_id", roomID)
	return room, nil
}

// unarchiveRoom serves trusted callers after authorization and waits for room views.
func (s *RoomCommandModel) unarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*readmodel.Room, error) {
	room, err := s.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = false

	unarchivedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomUnarchived{
			RoomUnarchived: &evtv1.RoomUnarchivedEvent{
				RoomId: roomID,
			},
		},
	})
	pos, err := s.rooms.appendDirectoryEventually(ctx, s.publisher, evtstream.RoomAggregate(roomID), unarchivedEvent)
	if err != nil {
		return nil, fmt.Errorf("publish RoomUnarchivedEvent: %w", err)
	}
	if err := s.rooms.waitForTimeline(ctx, pos); err != nil {
		return nil, err
	}

	s.logger.Info("Room unarchived", "kind", kind, "room_id", roomID)
	return room, nil
}
