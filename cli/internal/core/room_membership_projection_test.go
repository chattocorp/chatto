package core

import (
	"sort"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func joinEvent(roomID, userID string) *corev1.Event {
	return &corev1.Event{
		ActorId: userID,
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID},
		},
	}
}

func leaveEvent(roomID, userID string) *corev1.Event {
	return &corev1.Event{
		ActorId: userID,
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: roomID},
		},
	}
}

func TestRoomMembershipProjection_JoinLeaveQuery(t *testing.T) {
	p := NewRoomMembershipProjection()

	mustApply(t, p, joinEvent("R1", "U1"))
	mustApply(t, p, joinEvent("R1", "U2"))
	mustApply(t, p, joinEvent("R2", "U1"))

	if !p.IsMember("R1", "U1") {
		t.Error("U1 should be a member of R1")
	}
	if !p.IsMember("R1", "U2") {
		t.Error("U2 should be a member of R1")
	}
	if p.IsMember("R2", "U2") {
		t.Error("U2 should NOT be a member of R2")
	}

	if got := sortedStrings(p.Members("R1")); !equal(got, []string{"U1", "U2"}) {
		t.Errorf("Members(R1) = %v, want [U1 U2]", got)
	}
	if got := sortedStrings(p.Rooms("U1")); !equal(got, []string{"R1", "R2"}) {
		t.Errorf("Rooms(U1) = %v, want [R1 R2]", got)
	}

	mustApply(t, p, leaveEvent("R1", "U1"))
	if p.IsMember("R1", "U1") {
		t.Error("U1 should no longer be a member of R1 after leave")
	}
	if got := sortedStrings(p.Rooms("U1")); !equal(got, []string{"R2"}) {
		t.Errorf("Rooms(U1) after leave = %v, want [R2]", got)
	}
}

func TestRoomMembershipProjection_Idempotency(t *testing.T) {
	p := NewRoomMembershipProjection()

	// Same join applied twice: state must be identical to applying it once.
	mustApply(t, p, joinEvent("R1", "U1"))
	mustApply(t, p, joinEvent("R1", "U1"))

	if got := p.Members("R1"); len(got) != 1 || got[0] != "U1" {
		t.Errorf("Members(R1) after double-join = %v, want [U1]", got)
	}

	// Leave for a non-member: no-op.
	mustApply(t, p, leaveEvent("R1", "U_unknown"))
	if got := p.Members("R1"); len(got) != 1 || got[0] != "U1" {
		t.Errorf("Members(R1) after spurious leave = %v, want [U1]", got)
	}
}

func TestRoomMembershipProjection_EmptyRoomDropped(t *testing.T) {
	// Room should be removed from the index entirely once it has no
	// members, so Members/Rooms don't return stale entries.
	p := NewRoomMembershipProjection()

	mustApply(t, p, joinEvent("R1", "U1"))
	mustApply(t, p, leaveEvent("R1", "U1"))

	if got := p.Members("R1"); len(got) != 0 {
		t.Errorf("Members(R1) after last leave = %v, want empty", got)
	}
	if got := p.Rooms("U1"); len(got) != 0 {
		t.Errorf("Rooms(U1) after last leave = %v, want empty", got)
	}

	rooms, memberships := p.Stats()
	if rooms != 0 || memberships != 0 {
		t.Errorf("Stats = (%d, %d), want (0, 0)", rooms, memberships)
	}
}

func TestRoomMembershipProjection_RoomDeletedDropsAllMembers(t *testing.T) {
	p := NewRoomMembershipProjection()

	mustApply(t, p, joinEvent("R1", "U1"))
	mustApply(t, p, joinEvent("R1", "U2"))
	mustApply(t, p, joinEvent("R2", "U1"))

	deleted := &corev1.Event{
		ActorId: "system",
		Event: &corev1.Event_RoomDeleted{
			RoomDeleted: &corev1.RoomDeletedEvent{RoomId: "R1"},
		},
	}
	mustApply(t, p, deleted)

	if got := p.Members("R1"); len(got) != 0 {
		t.Errorf("Members(R1) after RoomDeleted = %v, want empty", got)
	}
	if got := sortedStrings(p.Rooms("U1")); !equal(got, []string{"R2"}) {
		t.Errorf("Rooms(U1) after RoomDeleted of R1 = %v, want [R2]", got)
	}
	if got := p.Rooms("U2"); len(got) != 0 {
		t.Errorf("Rooms(U2) after RoomDeleted of R1 = %v, want empty (U2 only had R1)", got)
	}

	// Re-applying RoomDeleted is idempotent.
	mustApply(t, p, deleted)
	rooms, memberships := p.Stats()
	if rooms != 1 || memberships != 1 {
		t.Errorf("Stats after RoomDeleted = (%d, %d), want (1, 1)", rooms, memberships)
	}
}

func TestRoomMembershipProjection_MalformedEventsRejected(t *testing.T) {
	p := NewRoomMembershipProjection()

	// Missing room_id.
	noRoom := &corev1.Event{
		ActorId: "U1",
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{},
		},
	}
	if err := p.Apply(noRoom, 1); err == nil {
		t.Error("want error on missing room_id, got nil")
	}

	// Missing actor_id.
	noActor := &corev1.Event{
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
		},
	}
	if err := p.Apply(noActor, 2); err == nil {
		t.Error("want error on missing actor_id, got nil")
	}

	// Unrelated event type: silently ignored.
	other := &corev1.Event{
		ActorId: "U1",
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{RoomId: "R1"},
		},
	}
	if err := p.Apply(other, 3); err != nil {
		t.Errorf("unrelated event type should be ignored, got error: %v", err)
	}
}

// ---- helpers ----

func mustApply(t *testing.T, p *RoomMembershipProjection, e *corev1.Event) {
	t.Helper()
	if err := p.Apply(e, 0); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
