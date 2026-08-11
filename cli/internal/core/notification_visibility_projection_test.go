package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func TestNotificationVisibilityProjectionRetainsExactBoundaryWhenCurrentStateAdvances(t *testing.T) {
	p := NewNotificationVisibilityProjection()
	created := &corev1.Event{Id: "create", CreatedAt: timestamppb.Now(), Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	loss := &corev1.Event{Id: "loss", CreatedAt: timestamppb.Now(), Event: &corev1.Event_RoomUniversalChanged{RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
		RoomId: "R1", Universal: false,
	}}}
	regain := &corev1.Event{Id: "regain", CreatedAt: timestamppb.Now(), Event: &corev1.Event_RoomUniversalChanged{RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
		RoomId: "R1", Universal: true,
	}}}
	for seq, event := range []*corev1.Event{created, loss, regain} {
		if err := p.Apply(event, uint64(seq+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", seq+1, err)
		}
	}

	lossState, err := p.Boundary(2, time.Now())
	if err != nil {
		t.Fatalf("Boundary loss: %v", err)
	}
	lossRoom, ok := lossState.rooms.Catalog.Get("R1")
	if !ok || lossRoom.GetUniversal() {
		t.Fatalf("loss boundary room = (%+v, %v), want non-universal", lossRoom, ok)
	}
	regainState, err := p.Boundary(3, time.Now())
	if err != nil {
		t.Fatalf("Boundary regain: %v", err)
	}
	regainRoom, ok := regainState.rooms.Catalog.Get("R1")
	if !ok || !regainRoom.GetUniversal() {
		t.Fatalf("regain boundary room = (%+v, %v), want universal", regainRoom, ok)
	}
}

func TestNotificationVisibilityProjectionCompactsManyPendingBoundariesOverLargeState(t *testing.T) {
	p := NewNotificationVisibilityProjection()
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
	const members = 2_000
	for i := 0; i < members; i++ {
		userID := fmt.Sprintf("U%04d", i)
		joined := &corev1.Event{Id: "join-" + userID, ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}}}
		if err := p.Apply(joined, uint64(i+2)); err != nil {
			t.Fatalf("Apply join %d: %v", i, err)
		}
	}

	const pendingBoundaries = 500
	firstBoundary := uint64(members + 2)
	for i := 0; i < pendingBoundaries; i++ {
		event := &corev1.Event{Id: fmt.Sprintf("universal-%d", i), Event: &corev1.Event_RoomUniversalChanged{RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
			RoomId: "R1", Universal: i%2 == 1,
		}}}
		if err := p.Apply(event, firstBoundary+uint64(i)); err != nil {
			t.Fatalf("Apply boundary %d: %v", i, err)
		}
	}

	p.mu.RLock()
	checkpointBytes := len(p.checkpoint)
	deltaCount := len(p.deltas)
	boundaryCount := len(p.boundaries)
	deltaBytes := 0
	for _, delta := range p.deltas {
		deltaBytes += proto.Size(delta.event)
	}
	p.mu.RUnlock()
	if checkpointBytes == 0 || boundaryCount != pendingBoundaries || deltaCount != pendingBoundaries-1 {
		t.Fatalf("retained state = checkpoint %d bytes, %d boundaries, %d deltas", checkpointBytes, boundaryCount, deltaCount)
	}
	if total := checkpointBytes + deltaBytes; total >= checkpointBytes*4 {
		t.Fatalf("compact journal = %d bytes for %d boundaries over %d-byte state; appears to retain repeated full snapshots", total, pendingBoundaries, checkpointBytes)
	}

	lastSequence := firstBoundary + pendingBoundaries - 1
	last, err := p.Boundary(lastSequence, time.Now())
	if err != nil {
		t.Fatalf("Boundary last: %v", err)
	}
	room, ok := last.rooms.Catalog.Get("R1")
	if !ok || !room.GetUniversal() {
		t.Fatalf("last boundary room = (%+v, %v), want universal", room, ok)
	}
}

func TestNotificationVisibilityProjectionBoundaryWorkDoesNotGrowWithMembershipHistory(t *testing.T) {
	p := NewNotificationVisibilityProjection()
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
	const historyEvents = 10_000
	for i := 0; i < historyEvents/2; i++ {
		userID := fmt.Sprintf("U%d", i)
		joined := &corev1.Event{Id: fmt.Sprintf("join-%d", i), ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}}}
		left := &corev1.Event{Id: fmt.Sprintf("left-%d", i), ActorId: userID, Event: &corev1.Event_UserLeftRoom{UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: "R1"}}}
		if err := p.Apply(joined, uint64(2+i*2)); err != nil {
			t.Fatalf("Apply join %d: %v", i, err)
		}
		if err := p.Apply(left, uint64(3+i*2)); err != nil {
			t.Fatalf("Apply leave %d: %v", i, err)
		}
	}
	lossSequence := uint64(historyEvents + 2)
	loss := &corev1.Event{Id: "loss", Event: &corev1.Event_RoomUniversalChanged{RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{RoomId: "R1", Universal: false}}}
	if err := p.Apply(loss, lossSequence); err != nil {
		t.Fatalf("Apply loss after membership history: %v", err)
	}

	p.mu.RLock()
	boundaryCount := len(p.boundaries)
	p.mu.RUnlock()
	if boundaryCount != 1 {
		t.Fatalf("retained boundaries = %d, want 1 independent of %d membership events", boundaryCount, historyEvents)
	}
	if _, err := p.Boundary(lossSequence, time.Now()); err != nil {
		t.Fatalf("Boundary after membership history: %v", err)
	}
}

type notificationVisibilityCapturingSnapshotSource struct {
	request events.ProjectionSnapshotLoadRequest
}

func (s *notificationVisibilityCapturingSnapshotSource) LoadProjectionSnapshot(_ context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	s.request = request
	return events.ProjectionSnapshot{}, nil
}

func TestNotificationVisibilitySnapshotRestoreIsCappedAtWorkerFloor(t *testing.T) {
	projection := NewNotificationVisibilityProjection()
	projection.SetAcknowledgedThrough(41)
	underlying := &notificationVisibilityCapturingSnapshotSource{}
	source := cappedNotificationVisibilitySnapshotSource{source: underlying, projection: projection}
	if _, err := source.LoadProjectionSnapshot(context.Background(), events.ProjectionSnapshotLoadRequest{MaxCutoff: 99}); err != nil {
		t.Fatalf("LoadProjectionSnapshot: %v", err)
	}
	if underlying.request.MaxCutoff != 41 {
		t.Fatalf("snapshot max cutoff = %d, want worker floor 41", underlying.request.MaxCutoff)
	}
}

func TestNotificationVisibilitySnapshotPublicationPreservesSafeGenerationWhilePending(t *testing.T) {
	p := NewNotificationVisibilityProjection()
	p.SetAcknowledgedThrough(1)
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
	if !p.AllowSnapshotPublication(1) {
		t.Fatal("snapshot before pending boundary was rejected")
	}
	loss := &corev1.Event{Id: "loss", Event: &corev1.Event_RoomUniversalChanged{RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
		RoomId: "R1", Universal: false,
	}}}
	if err := p.Apply(loss, 2); err != nil {
		t.Fatalf("Apply visibility loss: %v", err)
	}
	if p.AllowSnapshotPublication(2) {
		t.Fatal("snapshot including an unacknowledged boundary was allowed to rotate the safe generation")
	}
	if !p.AllowSnapshotPublication(1) {
		t.Fatal("older capture before pending boundary should remain publishable")
	}
	if err := p.ReleaseThrough(2); err != nil {
		t.Fatalf("ReleaseThrough: %v", err)
	}
	if !p.AllowSnapshotPublication(2) {
		t.Fatal("snapshot remained blocked after confirmed acknowledgement")
	}
}

func TestNotificationVisibilitySnapshotPublicationUsesFullWorkerFloor(t *testing.T) {
	p := NewNotificationVisibilityProjection()
	p.SetAcknowledgedThrough(1)
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
	// UserJoinedRoom changes visibility state but is not an implicit-loss
	// boundary. A different non-boundary worker delivery can hold AckFloor at
	// the same point, so publication must still use the full shared floor.
	joined := &corev1.Event{Id: "join", ActorId: "U1", Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}}}
	if err := p.Apply(joined, 2); err != nil {
		t.Fatalf("Apply membership delta: %v", err)
	}
	if p.AllowSnapshotPublication(2) {
		t.Fatal("snapshot above non-boundary worker floor was allowed")
	}
	if err := p.ReleaseThrough(2); err != nil {
		t.Fatalf("ReleaseThrough: %v", err)
	}
	if !p.AllowSnapshotPublication(2) {
		t.Fatal("snapshot remained blocked after worker floor advanced")
	}
}
