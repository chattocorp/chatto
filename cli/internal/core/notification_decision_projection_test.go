package core

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func TestNotificationDecisionProjectionRetainsExactBoundaryWhenCurrentStateAdvances(t *testing.T) {
	p := NewNotificationDecisionProjection()
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

func TestNotificationDecisionBoundaryRetainsEventTimePolicy(t *testing.T) {
	p := NewNotificationDecisionProjection()
	roomID := "R1"
	userID := "U1"
	roomScope := roomID
	events := []*corev1.Event{
		{Id: "user", Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: userID}}},
		{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "join", ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID}}},
		{Id: "silent", Event: &corev1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &corev1.UserNotificationPolicyChangedEvent{
			UserId: userID, RoomId: &roomScope, Overrides: &corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
		}}},
		{Id: "source", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}}},
		{Id: "off", Event: &corev1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &corev1.UserNotificationPolicyChangedEvent{
			UserId: userID, RoomId: &roomScope, Overrides: &corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
		}}},
		{Id: "later-source", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}}},
	}
	for i, event := range events {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", i+1, err)
		}
	}

	atSource, err := p.Boundary(5, time.Now())
	if err != nil {
		t.Fatalf("Boundary source: %v", err)
	}
	directMentionSignal := testNotificationSignal(notificationTestSignalDirectMention, roomID, "source")
	if got := atSource.effectiveNotificationMode(userID, roomID, directMentionSignal); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION {
		t.Fatalf("source policy = %v, want IN_APP_NOTIFICATION", got)
	}
	atLaterSource, err := p.Boundary(7, time.Now())
	if err != nil {
		t.Fatalf("Boundary later source: %v", err)
	}
	if got := atLaterSource.effectiveNotificationMode(userID, roomID, directMentionSignal); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
		t.Fatalf("later source policy = %v, want OFF", got)
	}
}

func TestNotificationDecisionBoundaryUsesRoomGroupAtSourceSequence(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const (
		roomID = "R1"
		userID = "U1"
		groupA = "G1"
		groupB = "G2"
	)
	events := []*corev1.Event{
		{Id: "user", Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: userID}}},
		{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "join", ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID}}},
		{Id: "group-a", Event: &corev1.Event_RoomGroupCreated{RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: groupA, Name: "A"}}},
		{Id: "group-b", Event: &corev1.Event_RoomGroupCreated{RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: groupB, Name: "B"}}},
		roomAddedToGroupEvent(groupA, roomID),
		{Id: "group-a-off", Event: &corev1.Event_UserRoomGroupNotificationPolicyChanged{UserRoomGroupNotificationPolicyChanged: &corev1.UserRoomGroupNotificationPolicyChangedEvent{
			UserId: userID, RoomGroupId: groupA, Overrides: &corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
		}}},
		{Id: "group-b-alert", Event: &corev1.Event_UserRoomGroupNotificationPolicyChanged{UserRoomGroupNotificationPolicyChanged: &corev1.UserRoomGroupNotificationPolicyChangedEvent{
			UserId: userID, RoomGroupId: groupB, Overrides: &corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()},
		}}},
		{Id: "source-a", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}}},
		roomRemovedFromGroupEvent(groupA, roomID),
		roomAddedToGroupEvent(groupB, roomID),
		{Id: "source-b", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}}},
	}
	for index, event := range events {
		if event.Id == "" {
			event.Id = fmt.Sprintf("layout-%d", index)
		}
		if err := p.Apply(event, uint64(index+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", index+1, err)
		}
	}
	signal := testNotificationSignal(notificationTestSignalDirectMention, roomID, "source")
	atA, err := p.Boundary(9, time.Now())
	if err != nil {
		t.Fatalf("Boundary group A source: %v", err)
	}
	if got := atA.effectiveNotificationMode(userID, roomID, signal); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF {
		t.Fatalf("group A source mode = %v, want OFF", got)
	}
	atB, err := p.Boundary(12, time.Now())
	if err != nil {
		t.Fatalf("Boundary group B source: %v", err)
	}
	if got := atB.effectiveNotificationMode(userID, roomID, signal); got != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION {
		t.Fatalf("group B source mode = %v, want PUSH_NOTIFICATION", got)
	}
}

func TestLegacyMessageMentionIDsDoNotGuessRichMentionCause(t *testing.T) {
	p := NewNotificationDecisionProjection()
	roomID := "R1"
	recipientID := "U1"
	source := &corev1.Event{
		Id: "source", ActorId: "U2", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId: roomID, MentionedUserIds: []string{recipientID},
		}},
	}
	events := []*corev1.Event{
		{Id: "user", Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: recipientID}}},
		{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "join", ActorId: recipientID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID}}},
		source,
	}
	for i, event := range events {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", i+1, err)
		}
	}
	snapshot, err := p.Boundary(4, source.GetCreatedAt().AsTime())
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	decisions, err := (&ChattoCore{}).buildMessageNotificationDecisionsAt(context.Background(), snapshot, source)
	if err != nil {
		t.Fatalf("buildMessageNotificationDecisionsAt: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("legacy decisions = %+v, want no guessed mention cause for %s", decisions, recipientID)
	}
}

func TestNotificationOccurrenceInputRetainsRoleMentionNames(t *testing.T) {
	source := &corev1.Event{Id: "source", ActorId: "actor", CreatedAt: timestamppb.Now()}
	message := &corev1.NotificationMessageReference{RoomId: "room", EventId: "source"}
	inputs := newNotificationOccurrenceInputs(source, []notificationRecipientDecision{{
		recipientID: "recipient",
		signal: &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &corev1.RoleMentionReceived{
			Message: message, RoleNames: []string{"moderator", "staff"},
		}}},
		mode: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
	}})
	if len(inputs) != 1 {
		t.Fatalf("inputs = %d, want 1", len(inputs))
	}
	got := inputs[0].Signal.GetRoleMentionReceived().GetRoleNames()
	if !slices.Equal(got, []string{"moderator", "staff"}) {
		t.Fatalf("role names = %v, want source role handles", got)
	}
}

func TestNotificationDecisionBoundaryRetainsEventTimeThreadFollowers(t *testing.T) {
	p := NewNotificationDecisionProjection()
	roomID := "R1"
	threadRootID := "ROOT"
	userID := "U1"
	events := []*corev1.Event{
		{Id: "user", Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: userID}}},
		{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{RoomId: roomID, Kind: corev1.RoomKind_ROOM_KIND_CHANNEL}}},
		{Id: "join", ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID}}},
		{Id: "follow", Event: &corev1.Event_ThreadFollowed{ThreadFollowed: &corev1.ThreadFollowedEvent{UserId: userID, RoomId: roomID, ThreadRootEventId: threadRootID}}},
		{Id: "reply", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID, InThread: threadRootID}}},
		{Id: "unfollow", Event: &corev1.Event_ThreadUnfollowed{ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{UserId: userID, RoomId: roomID, ThreadRootEventId: threadRootID}}},
		{Id: "later-reply", ActorId: "U2", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID, InThread: threadRootID}}},
	}
	for i, event := range events {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply sequence %d: %v", i+1, err)
		}
	}

	atReply, err := p.Boundary(5, time.Now())
	if err != nil {
		t.Fatalf("Boundary reply: %v", err)
	}
	if got := atReply.threadFollowerIDs(roomID, threadRootID); !slices.Equal(got, []string{userID}) {
		t.Fatalf("reply followers = %v, want [%s]", got, userID)
	}
	atLaterReply, err := p.Boundary(7, time.Now())
	if err != nil {
		t.Fatalf("Boundary later reply: %v", err)
	}
	if got := atLaterReply.threadFollowerIDs(roomID, threadRootID); len(got) != 0 {
		t.Fatalf("later reply followers = %v, want none", got)
	}
}

func TestNotificationDecisionProjectionRetainsOnlyIncrementalEventsOverLargeState(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const members = 2_000
	// Model startup with all existing state covered by the durable worker floor.
	// Applying that history builds both the current and lagging projections
	// without retaining a serialized server-wide checkpoint.
	p.SetAcknowledgedThrough(members + 1)
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
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
	deltaCount := len(p.deltas)
	boundaryCount := len(p.boundaries)
	deltaBytes := 0
	for _, delta := range p.deltas {
		deltaBytes += proto.Size(delta.event)
	}
	p.mu.RUnlock()
	if boundaryCount != pendingBoundaries || deltaCount != pendingBoundaries {
		t.Fatalf("retained state = %d boundaries, %d deltas; want %d of each", boundaryCount, deltaCount, pendingBoundaries)
	}
	if deltaBytes > pendingBoundaries*256 {
		t.Fatalf("incremental journal = %d bytes for %d small boundary facts; appears to retain more than source events", deltaBytes, pendingBoundaries)
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

func TestNotificationDecisionProjectionBoundaryWorkDoesNotGrowWithMembershipHistory(t *testing.T) {
	p := NewNotificationDecisionProjection()
	const historyEvents = 10_000
	p.SetAcknowledgedThrough(historyEvents + 1)
	created := &corev1.Event{Id: "create", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Universal: true,
	}}}
	if err := p.Apply(created, 1); err != nil {
		t.Fatalf("Apply room create: %v", err)
	}
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

func BenchmarkNotificationDecisionBoundaryIncrementalAfterLargeState(b *testing.B) {
	p := NewNotificationDecisionProjection()
	const members = 10_000
	p.SetAcknowledgedThrough(members + 1)
	if err := p.Apply(&corev1.Event{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	}}}, 1); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < members; i++ {
		userID := fmt.Sprintf("U%d", i)
		if err := p.Apply(&corev1.Event{Id: "join-" + userID, ActorId: userID, Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"}}}, uint64(i+2)); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sequence := uint64(members + 2 + i)
		event := &corev1.Event{Id: fmt.Sprintf("message-%d", i), ActorId: "author", Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}}}
		if err := p.Apply(event, sequence); err != nil {
			b.Fatal(err)
		}
		if _, err := p.Boundary(sequence, time.Now()); err != nil {
			b.Fatal(err)
		}
		if err := p.ReleaseThrough(sequence); err != nil {
			b.Fatal(err)
		}
	}
}

type notificationDecisionCapturingSnapshotSource struct {
	request events.ProjectionSnapshotLoadRequest
}

func (s *notificationDecisionCapturingSnapshotSource) LoadProjectionSnapshot(_ context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	s.request = request
	return events.ProjectionSnapshot{}, nil
}

func TestNotificationDecisionSnapshotRestoreIsCappedAtWorkerFloor(t *testing.T) {
	projection := NewNotificationDecisionProjection()
	projection.SetAcknowledgedThrough(41)
	underlying := &notificationDecisionCapturingSnapshotSource{}
	source := cappedNotificationDecisionSnapshotSource{source: underlying, projection: projection}
	if _, err := source.LoadProjectionSnapshot(context.Background(), events.ProjectionSnapshotLoadRequest{MaxCutoff: 99}); err != nil {
		t.Fatalf("LoadProjectionSnapshot: %v", err)
	}
	if underlying.request.MaxCutoff != 41 {
		t.Fatalf("snapshot max cutoff = %d, want worker floor 41", underlying.request.MaxCutoff)
	}
}

func TestNotificationDecisionSnapshotPublicationPreservesSafeGenerationWhilePending(t *testing.T) {
	p := NewNotificationDecisionProjection()
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

func TestNotificationDecisionSnapshotPublicationUsesFullWorkerFloor(t *testing.T) {
	p := NewNotificationDecisionProjection()
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

func TestNotificationDecisionEvaluatorAdvancesAndReleasesStateOnlyDeltas(t *testing.T) {
	p := NewNotificationDecisionProjection()
	p.SetAcknowledgedThrough(1)
	if err := p.Apply(&corev1.Event{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	}}}, 1); err != nil {
		t.Fatalf("Apply room: %v", err)
	}
	if err := p.Apply(&corev1.Event{Id: "join", ActorId: "U1", Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{
		RoomId: "R1",
	}}}, 2); err != nil {
		t.Fatalf("Apply join: %v", err)
	}
	if err := p.AdvanceThrough(2); err != nil {
		t.Fatalf("AdvanceThrough: %v", err)
	}
	if !p.evaluator.membershipExists("U1", "R1") {
		t.Fatal("lagging evaluator did not apply state-only membership delta")
	}
	if err := p.ReleaseThrough(2); err != nil {
		t.Fatalf("ReleaseThrough: %v", err)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.deltas) != 0 {
		t.Fatalf("retained deltas = %d, want none after worker acknowledgement", len(p.deltas))
	}
}

func TestNotificationDecisionEvaluatorPreservesOrderWhenIdleFloorAdvancesAheadOfProjector(t *testing.T) {
	p := NewNotificationDecisionProjection()
	p.SetAcknowledgedThrough(1)
	if err := p.Apply(&corev1.Event{Id: "room", Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	}}}, 1); err != nil {
		t.Fatalf("Apply room: %v", err)
	}
	if err := p.Apply(&corev1.Event{Id: "join", ActorId: "U1", Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{
		RoomId: "R1",
	}}}, 2); err != nil {
		t.Fatalf("Apply pending join: %v", err)
	}

	// Model ReleaseThrough observing an idle filtered consumer at EVT 3 before
	// this projection applies the state-only fact at that sequence.
	p.acknowledgedThrough.Store(3)
	if err := p.Apply(&corev1.Event{Id: "user", Event: &corev1.Event_UserAccountCreated{UserAccountCreated: &corev1.UserAccountCreatedEvent{
		UserId: "U1",
	}}}, 3); err != nil {
		t.Fatalf("Apply acknowledged user: %v", err)
	}
	if !p.evaluator.membershipExists("U1", "R1") {
		t.Fatal("worker-position evaluator skipped the older pending membership delta")
	}
	if _, active := p.evaluator.activeUsers["U1"]; !active {
		t.Fatal("worker-position evaluator did not apply the current acknowledged fact")
	}
}
