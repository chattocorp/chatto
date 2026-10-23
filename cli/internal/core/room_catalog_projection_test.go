package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func TestRoomCreationMetadataSurvivesUpdatesAndSnapshots(t *testing.T) {
	t.Parallel()
	p := NewRoomDirectoryProjection()
	event := roomCreatedEvent("room", "General", "", evtv1.RoomKind_ROOM_KIND_CHANNEL)
	event.CreatedAt = timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	wantSeconds := event.CreatedAt.Seconds
	require.NoError(t, p.Apply(event, 1))
	event.CreatedAt.Seconds++ // The projection must own its timestamp.
	require.NoError(t, p.Apply(roomUpdatedEvent("room", "Renamed", "Updated"), 2))
	room, ok := p.Catalog.Get("room")
	require.True(t, ok)
	require.Equal(t, wantSeconds, room.GetCreatedAt().GetSeconds())
	room.CreatedAt.Seconds++ // Reads must not mutate the projection either.
	snapshot, err := p.Snapshot()
	require.NoError(t, err)
	restored := NewRoomDirectoryProjection()
	require.NoError(t, restored.Restore(snapshot))
	room, ok = restored.Catalog.Get("room")
	require.True(t, ok)
	require.Equal(t, wantSeconds, room.GetCreatedAt().GetSeconds())
	require.Equal(t, "Renamed", room.GetName())
}

func roomCreatedEvent(roomID, name, description string, kind evtv1.RoomKind) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomCreated{
			RoomCreated: &evtv1.RoomCreatedEvent{
				RoomId:      roomID,
				Name:        name,
				Description: description,
				Kind:        kind,
			},
		},
	}
}

func roomUpdatedEvent(roomID, name, description string) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomUpdated{
			RoomUpdated: &evtv1.RoomUpdatedEvent{
				RoomId:      roomID,
				Name:        name,
				Description: description,
			},
		},
	}
}

func roomArchivedEvent(roomID string) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomArchived{
			RoomArchived: &evtv1.RoomArchivedEvent{RoomId: roomID},
		},
	}
}

func roomUnarchivedEvent(roomID string) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomUnarchived{
			RoomUnarchived: &evtv1.RoomUnarchivedEvent{RoomId: roomID},
		},
	}
}

func roomUniversalChangedEvent(roomID string, universal bool) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &evtv1.RoomUniversalChangedEvent{
				RoomId:    roomID,
				Universal: universal,
			},
		},
	}
}

func roomSlowModeChangedEvent(roomID string, seconds uint32) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomSlowModeChanged{
			RoomSlowModeChanged: &evtv1.RoomSlowModeChangedEvent{
				RoomId:          roomID,
				SlowModeSeconds: seconds,
			},
		},
	}
}

func roomDeletedEvent(roomID string) *evtv1.Event {
	return &evtv1.Event{
		Event: &evtv1.Event_RoomDeleted{
			RoomDeleted: &evtv1.RoomDeletedEvent{RoomId: roomID},
		},
	}
}

func TestRoomCatalogProjection_FreshState(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()
	require.False(t, p.Exists("R1"))
	got, ok := p.Get("R1")
	require.False(t, ok)
	require.Nil(t, got)
	require.Equal(t, 0, p.Count())
	require.Empty(t, p.AllByKind(evtv1.RoomKind_ROOM_KIND_CHANNEL))
}

func TestRoomCatalogProjection_CreateUpdateArchiveDelete(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()

	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "default", evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))

	got, ok := p.Get("R1")
	require.True(t, ok)
	require.Equal(t, "general", got.Name)
	require.Equal(t, "default", got.Description)
	require.Equal(t, evtv1.RoomKind_ROOM_KIND_CHANNEL, got.Kind)
	require.False(t, got.Archived)

	// Update changes name + description together (coarse, like the
	// admin form).
	require.NoError(t, p.Apply(roomUpdatedEvent("R1", "announcements", "updates only"), 2))
	got, _ = p.Get("R1")
	require.Equal(t, "announcements", got.Name)
	require.Equal(t, "updates only", got.Description)
	require.False(t, got.Archived) // archive state untouched

	require.NoError(t, p.Apply(roomArchivedEvent("R1"), 3))
	got, _ = p.Get("R1")
	require.True(t, got.Archived)
	require.Equal(t, "announcements", got.Name) // metadata preserved

	require.NoError(t, p.Apply(roomUnarchivedEvent("R1"), 4))
	got, _ = p.Get("R1")
	require.False(t, got.Archived)

	require.NoError(t, p.Apply(roomUniversalChangedEvent("R1", true), 5))
	got, _ = p.Get("R1")
	require.True(t, got.Universal)

	require.NoError(t, p.Apply(roomUniversalChangedEvent("R1", false), 6))
	got, _ = p.Get("R1")
	require.False(t, got.Universal)

	require.NoError(t, p.Apply(roomSlowModeChangedEvent("R1", 30), 7))
	got, _ = p.Get("R1")
	require.Equal(t, uint32(30), got.SlowModeSeconds)
	require.Nil(t, got.ThreadSlowModeSeconds)
	require.Equal(t, uint32(30), EffectiveThreadSlowModeSeconds(got), "legacy interval applies to both scopes")
	independent := roomSlowModeChangedEvent("R1", 30)
	independent.GetRoomSlowModeChanged().ThreadSlowModeSeconds = new(uint32(0))
	require.NoError(t, p.Apply(independent, 8))
	got, _ = p.Get("R1")
	require.NotNil(t, got.ThreadSlowModeSeconds)
	require.Zero(t, EffectiveThreadSlowModeSeconds(got), "explicit Off must not inherit the root interval")

	require.NoError(t, p.Apply(roomDeletedEvent("R1"), 9))
	_, ok = p.Get("R1")
	require.False(t, ok)
	require.Equal(t, 0, p.Count())
}

func TestRoomCatalogProjection_AllByKind(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()

	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))
	require.NoError(t, p.Apply(roomCreatedEvent("R2", "announcements", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 2))
	require.NoError(t, p.Apply(roomCreatedEvent("DM1", "", "", evtv1.RoomKind_ROOM_KIND_DM), 3))

	channels := p.AllByKind(evtv1.RoomKind_ROOM_KIND_CHANNEL)
	require.Len(t, channels, 2)

	dms := p.AllByKind(evtv1.RoomKind_ROOM_KIND_DM)
	require.Len(t, dms, 1)
}

func TestRoomCatalogProjection_Idempotency(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()

	// Same RoomCreated applied twice — second call replaces but result is identical.
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "x", evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "x", evtv1.RoomKind_ROOM_KIND_CHANNEL), 2))

	got, ok := p.Get("R1")
	require.True(t, ok)
	require.Equal(t, "general", got.Name)
	require.Equal(t, 1, p.Count())

	// Update on a non-existent room — no-op.
	require.NoError(t, p.Apply(roomUpdatedEvent("Rmissing", "nope", "nope"), 3))
	require.False(t, p.Exists("Rmissing"))

	// Archive on a non-existent room — no-op.
	require.NoError(t, p.Apply(roomArchivedEvent("Rmissing"), 4))
	require.False(t, p.Exists("Rmissing"))

	// Delete on a non-existent room — no-op (no error).
	require.NoError(t, p.Apply(roomDeletedEvent("Rmissing"), 5))
}

func TestRoomCatalogProjection_GetReturnsClone(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "original", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))

	got, _ := p.Get("R1")
	got.Name = "mutated"

	got2, _ := p.Get("R1")
	require.Equal(t, "original", got2.Name)
}

func TestRoomCatalogProjection_IgnoresMembershipEvents(t *testing.T) {
	t.Parallel()

	// Membership events live on the same evt.room.> subject family but
	// belong to RoomMembershipProjection. RoomCatalogProjection must
	// not react to them.
	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))

	join := &evtv1.Event{
		ActorId: "U1",
		Event:   &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(join, 2))

	leave := &evtv1.Event{
		ActorId: "U1",
		Event:   &evtv1.Event_UserLeftRoom{UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(leave, 3))

	// Catalog state should still reflect only RoomCreated.
	got, ok := p.Get("R1")
	require.True(t, ok)
	require.Equal(t, "general", got.Name)
}

func TestRoomCatalogProjection_NameClaimSnapshotTracksRoomSeq(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 10))

	snapshot := p.NameClaimSnapshot("General", "")
	require.Equal(t, "R1", snapshot.ConflictingRoomID)
	require.Equal(t, uint64(10), snapshot.Seq)

	excluded := p.NameClaimSnapshot("General", "R1")
	require.Empty(t, excluded.ConflictingRoomID)
	require.Equal(t, uint64(10), excluded.Seq)

	join := &evtv1.Event{
		ActorId: "U1",
		Event:   &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(join, 11))

	snapshot = p.NameClaimSnapshot("general", "")
	require.Equal(t, "R1", snapshot.ConflictingRoomID)
	require.Equal(t, uint64(11), snapshot.Seq)

	require.NoError(t, p.Apply(roomCreatedEvent("DM1", "general", "", evtv1.RoomKind_ROOM_KIND_DM), 12))
	snapshot = p.NameClaimSnapshot("general", "")
	require.Equal(t, "R1", snapshot.ConflictingRoomID)
	require.Equal(t, uint64(12), snapshot.Seq)
}

func TestRoomCatalogProjection_NameClaimSnapshotFindsOtherPreexistingCollision(t *testing.T) {
	t.Parallel()

	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "Straße", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 10))
	require.NoError(t, p.Apply(roomCreatedEvent("R2", "STRASSE", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 11))

	snapshot := p.NameClaimSnapshot("strasse", "R1")
	require.Equal(t, "R2", snapshot.ConflictingRoomID)
	require.Equal(t, uint64(11), snapshot.Seq)
}
