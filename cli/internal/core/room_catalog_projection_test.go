package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func roomCreatedEvent(roomID, name, description string, kind corev1.RoomKind) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      roomID,
				Name:        name,
				Description: description,
				Kind:        kind,
			},
		},
	}
}

func roomUpdatedEvent(roomID, name, description string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{
				RoomId:      roomID,
				Name:        name,
				Description: description,
			},
		},
	}
}

func roomArchivedEvent(roomID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomArchived{
			RoomArchived: &corev1.RoomArchivedEvent{RoomId: roomID},
		},
	}
}

func roomUnarchivedEvent(roomID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomUnarchived{
			RoomUnarchived: &corev1.RoomUnarchivedEvent{RoomId: roomID},
		},
	}
}

func roomUniversalChangedEvent(roomID string, universal bool) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
				RoomId:    roomID,
				Universal: universal,
			},
		},
	}
}

func roomDeletedEvent(roomID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomDeleted{
			RoomDeleted: &corev1.RoomDeletedEvent{RoomId: roomID},
		},
	}
}

func TestRoomCatalogProjection_FreshState(t *testing.T) {
	p := NewRoomCatalogProjection()
	require.False(t, p.Exists("R1"))
	got, ok := p.Get("R1")
	require.False(t, ok)
	require.Nil(t, got)
	require.Equal(t, 0, p.Count())
	require.Empty(t, p.AllByKind(corev1.RoomKind_ROOM_KIND_CHANNEL))
}

func TestRoomCatalogProjection_CreateUpdateArchiveDelete(t *testing.T) {
	p := NewRoomCatalogProjection()

	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "default", corev1.RoomKind_ROOM_KIND_CHANNEL), 1))

	got, ok := p.Get("R1")
	require.True(t, ok)
	require.Equal(t, "general", got.Name)
	require.Equal(t, "default", got.Description)
	require.Equal(t, corev1.RoomKind_ROOM_KIND_CHANNEL, got.Kind)
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

	require.NoError(t, p.Apply(roomDeletedEvent("R1"), 7))
	_, ok = p.Get("R1")
	require.False(t, ok)
	require.Equal(t, 0, p.Count())
}

func TestRoomCatalogProjection_AllByKind(t *testing.T) {
	p := NewRoomCatalogProjection()

	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", corev1.RoomKind_ROOM_KIND_CHANNEL), 1))
	require.NoError(t, p.Apply(roomCreatedEvent("R2", "announcements", "", corev1.RoomKind_ROOM_KIND_CHANNEL), 2))
	require.NoError(t, p.Apply(roomCreatedEvent("DM1", "", "", corev1.RoomKind_ROOM_KIND_DM), 3))

	channels := p.AllByKind(corev1.RoomKind_ROOM_KIND_CHANNEL)
	require.Len(t, channels, 2)

	dms := p.AllByKind(corev1.RoomKind_ROOM_KIND_DM)
	require.Len(t, dms, 1)
}

func TestRoomCatalogProjection_Idempotency(t *testing.T) {
	p := NewRoomCatalogProjection()

	// Same RoomCreated applied twice — second call replaces but result is identical.
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "x", corev1.RoomKind_ROOM_KIND_CHANNEL), 1))
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "x", corev1.RoomKind_ROOM_KIND_CHANNEL), 2))

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
	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "original", "", corev1.RoomKind_ROOM_KIND_CHANNEL), 1))

	got, _ := p.Get("R1")
	got.Name = "mutated"

	got2, _ := p.Get("R1")
	require.Equal(t, "original", got2.Name)
}

func TestRoomCatalogProjection_IgnoresMembershipEvents(t *testing.T) {
	// Membership events live on the same evt.room.> subject family but
	// belong to RoomMembershipProjection. RoomCatalogProjection must
	// not react to them.
	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", corev1.RoomKind_ROOM_KIND_CHANNEL), 1))

	join := &corev1.Event{
		ActorId: "U1",
		Event:   &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(join, 2))

	leave := &corev1.Event{
		ActorId: "U1",
		Event:   &corev1.Event_UserLeftRoom{UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(leave, 3))

	// Catalog state should still reflect only RoomCreated.
	got, ok := p.Get("R1")
	require.True(t, ok)
	require.Equal(t, "general", got.Name)
}

func TestRoomCatalogProjection_NameClaimSnapshotTracksRoomSeq(t *testing.T) {
	p := NewRoomCatalogProjection()
	require.NoError(t, p.Apply(roomCreatedEvent("R1", "general", "", corev1.RoomKind_ROOM_KIND_CHANNEL), 10))

	snapshot := p.NameClaimSnapshot("General")
	require.Equal(t, "R1", snapshot.OwnerRoomID)
	require.Equal(t, uint64(10), snapshot.Seq)

	join := &corev1.Event{
		ActorId: "U1",
		Event:   &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}},
	}
	require.NoError(t, p.Apply(join, 11))

	snapshot = p.NameClaimSnapshot("general")
	require.Equal(t, "R1", snapshot.OwnerRoomID)
	require.Equal(t, uint64(11), snapshot.Seq)

	require.NoError(t, p.Apply(roomCreatedEvent("DM1", "general", "", corev1.RoomKind_ROOM_KIND_DM), 12))
	snapshot = p.NameClaimSnapshot("general")
	require.Equal(t, "R1", snapshot.OwnerRoomID)
	require.Equal(t, uint64(12), snapshot.Seq)
}
