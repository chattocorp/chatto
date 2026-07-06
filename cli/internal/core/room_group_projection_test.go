package core

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func groupCreatedEvent(groupID, name, description string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomGroupCreated{
			RoomGroupCreated: &corev1.RoomGroupCreatedEvent{
				GroupId:     groupID,
				Name:        name,
				Description: description,
			},
		},
	}
}

func groupUpdatedEvent(groupID, name, description string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomGroupUpdated{
			RoomGroupUpdated: &corev1.RoomGroupUpdatedEvent{
				GroupId:     groupID,
				Name:        name,
				Description: description,
			},
		},
	}
}

func groupDeletedEvent(groupID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomGroupDeleted{
			RoomGroupDeleted: &corev1.RoomGroupDeletedEvent{GroupId: groupID},
		},
	}
}

func roomAddedToGroupEvent(groupID, roomID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomAddedToGroup{
			RoomAddedToGroup: &corev1.RoomAddedToGroupEvent{
				GroupId: groupID,
				RoomId:  roomID,
			},
		},
	}
}

func roomRemovedFromGroupEvent(groupID, roomID string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomRemovedFromGroup{
			RoomRemovedFromGroup: &corev1.RoomRemovedFromGroupEvent{
				GroupId: groupID,
				RoomId:  roomID,
			},
		},
	}
}

func roomsReorderedEvent(groupID string, roomIDs []string) *corev1.Event {
	return &corev1.Event{
		Event: &corev1.Event_RoomsInGroupReordered{
			RoomsInGroupReordered: &corev1.RoomsInGroupReorderedEvent{
				GroupId: groupID,
				RoomIds: roomIDs,
			},
		},
	}
}

func TestRoomGroupProjection_FreshState(t *testing.T) {
	p := NewRoomGroupProjection()
	require.False(t, p.Exists("G1"))
	require.Equal(t, 0, p.Count())
	require.Empty(t, p.All())
	require.Equal(t, "", p.GroupForRoom("R1"))
}

func TestRoomGroupProjection_CreateUpdateDelete(t *testing.T) {
	p := NewRoomGroupProjection()

	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Lobby", "default group"), 1))

	got, ok := p.Get("G1")
	require.True(t, ok)
	require.Equal(t, "Lobby", got.Name)
	require.Equal(t, "default group", got.Description)
	require.Empty(t, got.RoomIds)

	require.NoError(t, p.Apply(groupUpdatedEvent("G1", "Main", "renamed"), 2))
	got, _ = p.Get("G1")
	require.Equal(t, "Main", got.Name)
	require.Equal(t, "renamed", got.Description)

	require.NoError(t, p.Apply(groupDeletedEvent("G1"), 3))
	_, ok = p.Get("G1")
	require.False(t, ok)
	require.Equal(t, 0, p.Count())
}

func TestRoomGroupProjection_RoomMembership(t *testing.T) {
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Lobby", ""), 1))

	// Add three rooms in order.
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 2))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R2"), 3))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R3"), 4))

	got, _ := p.Get("G1")
	require.Equal(t, []string{"R1", "R2", "R3"}, got.RoomIds)

	// Remove R2 — order of remaining preserved.
	require.NoError(t, p.Apply(roomRemovedFromGroupEvent("G1", "R2"), 5))
	got, _ = p.Get("G1")
	require.Equal(t, []string{"R1", "R3"}, got.RoomIds)

	// GroupForRoom reflects current membership.
	require.Equal(t, "G1", p.GroupForRoom("R1"))
	require.Equal(t, "", p.GroupForRoom("R2"))
	require.Equal(t, "G1", p.GroupForRoom("R3"))

	// Reorder replaces the ordering wholesale.
	require.NoError(t, p.Apply(roomsReorderedEvent("G1", []string{"R3", "R1"}), 6))
	got, _ = p.Get("G1")
	require.Equal(t, []string{"R3", "R1"}, got.RoomIds)
}

func TestRoomGroupProjection_Idempotency(t *testing.T) {
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Lobby", ""), 1))

	// Double-add: second is a no-op.
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 2))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 3))
	got, _ := p.Get("G1")
	require.Equal(t, []string{"R1"}, got.RoomIds)

	// Remove non-member: no-op.
	require.NoError(t, p.Apply(roomRemovedFromGroupEvent("G1", "Runknown"), 4))
	got, _ = p.Get("G1")
	require.Equal(t, []string{"R1"}, got.RoomIds)

	// Events against an unknown group: no-op (no panic).
	require.NoError(t, p.Apply(roomAddedToGroupEvent("Gunknown", "Rwhatever"), 5))
	require.NoError(t, p.Apply(roomRemovedFromGroupEvent("Gunknown", "Rwhatever"), 6))
	require.NoError(t, p.Apply(groupUpdatedEvent("Gunknown", "x", "y"), 7))
	require.NoError(t, p.Apply(groupDeletedEvent("Gunknown"), 8))
	require.False(t, p.Exists("Gunknown"))
}

func TestRoomGroupProjection_CrossGroupMove(t *testing.T) {
	// The move-room operation is two events: removed-from-source +
	// added-to-target. Projection should reflect that R1 is no longer
	// in G1 and is now in G2.
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Source", ""), 1))
	require.NoError(t, p.Apply(groupCreatedEvent("G2", "Target", ""), 2))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 3))

	require.Equal(t, "G1", p.GroupForRoom("R1"))

	// Move R1 from G1 to G2.
	require.NoError(t, p.Apply(roomRemovedFromGroupEvent("G1", "R1"), 4))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G2", "R1"), 5))

	require.Equal(t, "G2", p.GroupForRoom("R1"))

	g1, _ := p.Get("G1")
	require.Empty(t, g1.RoomIds)

	g2, _ := p.Get("G2")
	require.Equal(t, []string{"R1"}, g2.RoomIds)
}

func TestRoomGroupProjection_GetReturnsClone(t *testing.T) {
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Lobby", ""), 1))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 2))

	got, _ := p.Get("G1")
	got.Name = "mutated"
	got.RoomIds[0] = "Rmutated"
	got.RoomIds = append(got.RoomIds, "Rextra")

	got2, _ := p.Get("G1")
	require.Equal(t, "Lobby", got2.Name)
	require.Equal(t, []string{"R1"}, got2.RoomIds)
}

func TestRoomGroupProjection_UnrelatedEventsIgnored(t *testing.T) {
	p := NewRoomGroupProjection()

	// A non-group event under no particular filter — projection
	// shouldn't react, and shouldn't panic.
	other := &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{RoomId: "R1", Name: "general"},
		},
	}
	require.NoError(t, p.Apply(other, 1))
	require.Equal(t, 0, p.Count())
}

func TestRoomGroupProjection_MoveSnapshotTracksIgnoredGroupSeq(t *testing.T) {
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Target", ""), 10))

	ignoredGroupEvent := &corev1.Event{}
	require.NoError(t, p.Apply(ignoredGroupEvent, 11))

	snapshot := p.MoveSnapshot("R1", "G1")
	require.True(t, snapshot.TargetExists)
	require.Equal(t, uint64(11), snapshot.Seq)
}

func TestRoomGroupProjection_MoveSnapshotTracksGroupSeq(t *testing.T) {
	p := NewRoomGroupProjection()
	require.NoError(t, p.Apply(groupCreatedEvent("G1", "Source", ""), 10))
	require.NoError(t, p.Apply(groupCreatedEvent("G2", "Target", ""), 11))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G1", "R1"), 12))

	snapshot := p.MoveSnapshot("R1", "G2")
	require.True(t, snapshot.TargetExists)
	require.Equal(t, "G1", snapshot.SourceGroupID)
	require.Equal(t, uint64(12), snapshot.Seq)

	require.NoError(t, p.Apply(roomRemovedFromGroupEvent("G1", "R1"), 13))
	require.NoError(t, p.Apply(roomAddedToGroupEvent("G2", "R1"), 14))

	snapshot = p.MoveSnapshot("R1", "G2")
	require.True(t, snapshot.TargetExists)
	require.Equal(t, "G2", snapshot.SourceGroupID)
	require.Equal(t, uint64(14), snapshot.Seq)

	snapshot = p.MoveSnapshot("R1", "Gmissing")
	require.False(t, snapshot.TargetExists)
	require.Equal(t, "G2", snapshot.SourceGroupID)
	require.Equal(t, uint64(14), snapshot.Seq)
}
