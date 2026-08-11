package core

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

func TestMessageNotificationMaterializationMergesReasonsAndReconcilesReadState(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "notify-bob", "Notify Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "notify-alice", "Notify Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, alice.Id, KindChannel, "", "notification-materializer-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{alice.Id, bob.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}

	root, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, bob.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	reply, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, alice.Id, "@notify-bob hello", nil, "", root.Id, nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}
	if seq, err := chattoCore.EventPublisher.LastSubjectSeq(ctx, "evt.notification.>"); err != nil || seq != 0 {
		t.Fatalf("notification-only EVT sequence = (%d, %v), want none", seq, err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, bob.Id, NotificationOccurrenceViewInbox)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("occurrences = %d, want one merged occurrence", len(occurrences))
	}
	occurrence := occurrences[0]
	if occurrence.GetSourceEventId() != reply.Id || occurrence.GetTarget().GetEventId() != reply.Id || occurrence.GetTarget().GetParentEventId() != root.Id {
		t.Fatalf("occurrence target = %+v, source = %q", occurrence.GetTarget(), occurrence.GetSourceEventId())
	}
	wantReasons := map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity{
		corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		corev1.NotificationReason_NOTIFICATION_REASON_REPLY:          corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM:  corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF,
	}
	if len(occurrence.GetReasons()) != len(wantReasons) {
		t.Fatalf("reasons = %+v, want %d", occurrence.GetReasons(), len(wantReasons))
	}
	for _, match := range occurrence.GetReasons() {
		if wantReasons[match.GetReason()] != match.GetIntensity() {
			t.Fatalf("reason %v intensity = %v, want %v", match.GetReason(), match.GetIntensity(), wantReasons[match.GetReason()])
		}
	}

	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, bob.Id, room.Id, reply.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	read, err := chattoCore.NotificationOccurrences().Get(ctx, bob.Id, occurrence.GetId())
	if err != nil {
		t.Fatalf("Get after room read: %v", err)
	}
	if read.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ {
		t.Fatalf("inbox state = %v, want READ", read.GetInboxState())
	}

	if err := chattoCore.DeleteMessage(ctx, alice.Id, KindChannel, room.Id, reply.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, bob.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 0 {
		t.Fatalf("Inbox after retraction = (%v, %v), want empty", occurrences, err)
	}
}

func TestNotificationVisibilityBoundarySurvivesSuccessfulHandlerUntilAckFloor(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	chattoCore, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "notification-ack-boundary-secret",
		Assets:    config.AssetsConfig{SigningSecret: "notification-ack-boundary-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	// Prevent the confirmed-ACK cleanup ticker from racing this explicit
	// handler/redelivery assertion.
	chattoCore.notificationMaterializer.pollEvery = time.Hour
	startCoreServices(t, chattoCore)

	owner, err := chattoCore.CreateUser(ctx, SystemActorID, "ack-boundary-owner", "Ack Boundary Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	member, err := chattoCore.CreateUser(ctx, SystemActorID, "ack-boundary-member", "Ack Boundary Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, owner.Id, KindChannel, "", "ack-boundary-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, owner.Id, "ack boundary target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	postedSequence, err := chattoCore.GetEventSequence(ctx, KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	if _, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID: member.Id, SourceEventID: "ack-boundary-source", SourceCreated: time.Now().UTC(),
		SourceStreamSequence: postedSequence, ActorID: owner.Id,
		Target:         &corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		Reasons:        []*corev1.NotificationReasonMatch{{Reason: corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION, Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE}},
		SkipReadLookup: true,
	}); err != nil || !created {
		t.Fatalf("Create occurrence = (%v, %v)", created, err)
	}

	if _, err := chattoCore.SetRoomUniversal(ctx, owner.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}
	if _, err := chattoCore.SetRoomUniversal(ctx, owner.Id, KindChannel, room.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	losses, _, err := chattoCore.EventPublisher.SubjectEventsWithSubjectsAfter(ctx, evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged), 0)
	if err != nil || len(losses) == 0 {
		t.Fatalf("read loss event = (%d, %v)", len(losses), err)
	}
	loss := losses[len(losses)-1]
	if _, err := chattoCore.notificationMaterializer.visibility.Projection().Boundary(loss.Sequence, time.Now()); err != nil {
		t.Fatalf("boundary after successful handler: %v", err)
	}
	data, err := proto.Marshal(loss.Event)
	if err != nil {
		t.Fatalf("marshal loss event: %v", err)
	}
	// Model DoubleAck failing after a successful handler: JetStream may deliver
	// the same fact again, and that evaluation must still have its exact state.
	if err := chattoCore.notificationMaterializer.processDelivery(ctx, events.DurableDelivery{
		Subject: loss.Subject, Data: data, StreamSequence: loss.Sequence, NumDelivered: 2,
	}); err != nil {
		t.Fatalf("redelivered loss: %v", err)
	}
	if _, err := chattoCore.notificationMaterializer.visibility.Projection().Boundary(loss.Sequence, time.Now()); err != nil {
		t.Fatalf("boundary after redelivered handler: %v", err)
	}
	if err := chattoCore.notificationMaterializer.visibility.Projection().ReleaseThrough(loss.Sequence); err != nil {
		t.Fatalf("ReleaseThrough: %v", err)
	}
	if _, err := chattoCore.notificationMaterializer.visibility.Projection().Boundary(loss.Sequence, time.Now()); err == nil {
		t.Fatal("boundary remained after confirmed acknowledgement floor")
	}
}

func TestNotificationAcknowledgedThroughUsesFullConsumerFloor(t *testing.T) {
	tests := []struct {
		name string
		tail uint64
		info *jetstream.ConsumerInfo
		want uint64
	}{
		{
			name: "idle consumer reaches earlier stream tail",
			tail: 90,
			info: &jetstream.ConsumerInfo{AckFloor: jetstream.SequenceInfo{Stream: 41}},
			want: 90,
		},
		{
			name: "undelivered fact retains confirmed ack floor",
			tail: 90,
			info: &jetstream.ConsumerInfo{AckFloor: jetstream.SequenceInfo{Stream: 41}, NumPending: 1},
			want: 41,
		},
		{
			name: "delivered fact retains confirmed ack floor",
			tail: 90,
			info: &jetstream.ConsumerInfo{AckFloor: jetstream.SequenceInfo{Stream: 41}, NumAckPending: 1},
			want: 41,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := notificationAcknowledgedThrough(test.tail, test.info); got != test.want {
				t.Fatalf("notificationAcknowledgedThrough() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestConfiguredOwnerMaterializationRetriesWithoutLiveFallbackDivergence(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateVerifiedUser(ctx, SystemActorID, "retry-config-owner", "Retry Config Owner", "password", "owner@example.com")
	if err != nil {
		t.Fatalf("CreateVerifiedUser: %v", err)
	}
	chattoCore.config.Owners = config.OwnersConfig{Emails: []string{"owner@example.com"}}

	realAssign := chattoCore.notificationMaterializer.assignConfiguredOwnerRole
	attempts := 0
	chattoCore.notificationMaterializer.assignConfiguredOwnerRole = func(ctx context.Context, userID string) error {
		attempts++
		if attempts == 1 {
			return errors.New("forced transient assignment failure")
		}
		return realAssign(ctx, userID)
	}
	if err := chattoCore.notificationMaterializer.materializeConfiguredOwner(ctx, user.Id); err == nil {
		t.Fatal("first materialization unexpectedly succeeded")
	}
	if owner, err := chattoCore.IsServerOwner(ctx, user.Id); err != nil || owner {
		t.Fatalf("configured email became a live-only owner after failed durable assignment: owner=%v err=%v", owner, err)
	}
	if err := chattoCore.notificationMaterializer.materializeConfiguredOwner(ctx, user.Id); err != nil {
		t.Fatalf("retry configured-owner materialization: %v", err)
	}
	if owner, err := chattoCore.IsServerOwner(ctx, user.Id); err != nil || !owner {
		t.Fatalf("durably materialized owner = %v, err=%v", owner, err)
	}
}

func TestNotificationDurableWorkerMaterializesPreparedRuntimeWork(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "worker-author", "Worker Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "worker-recipient", "Worker Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "notification-worker-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}

	source := newEvent(author.Id, &corev1.Event{Event: &corev1.Event_MessagePosted{
		MessagePosted: &corev1.MessagePostedEvent{RoomId: room.Id},
	}})
	work := newNotificationOccurrenceWork(
		source,
		&corev1.NotificationTarget{RoomId: room.Id, EventId: source.Id},
		[]notificationRecipientDecision{{
			recipientID: recipient.Id,
			reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, source, work); err != nil {
		t.Fatalf("StoreWork: %v", err)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationWorkMarkerKey(source.Id)); err != nil {
		t.Fatalf("notification work marker after StoreWork: %v", err)
	}
	if _, err := chattoCore.EventPublisher.AppendEventually(ctx, evtstream.RoomAggregate(room.Id).SubjectFor(source), source); err != nil {
		t.Fatalf("append source: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox)
		if err == nil && len(occurrences) == 1 && occurrences[0].GetSourceEventId() == source.Id && occurrences[0].GetSourceStreamSequence() != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("durable worker did not materialize occurrence: occurrences=%+v err=%v", occurrences, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationWorkKey(source.Id, recipient.Id)); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("notification work remains after materialization: %v", err)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationWorkMarkerKey(source.Id)); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		t.Fatalf("notification work marker remains after materialization: %v", err)
	}
}

func TestStoreWorkClearsStaleRecipientsWhenRetryNowProducesNoWork(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	source := newEvent("U-work-actor", &corev1.Event{Event: &corev1.Event_ReactionAdded{
		ReactionAdded: &corev1.ReactionAddedEvent{RoomId: "R-work", MessageEventId: "E-target", Emoji: "thumbsup"},
	}})
	work := newNotificationOccurrenceWork(
		source,
		&corev1.NotificationTarget{RoomId: "R-work", EventId: "E-target"},
		[]notificationRecipientDecision{{
			recipientID: "U-work-recipient",
			reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, source, work); err != nil {
		t.Fatalf("StoreWork initial: %v", err)
	}
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, source, nil); err != nil {
		t.Fatalf("StoreWork retry without recipients: %v", err)
	}
	for _, key := range []string{
		notificationWorkMarkerKey(source.GetId()),
		notificationWorkKey(source.GetId(), "U-work-recipient"),
	} {
		if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
			t.Fatalf("stale notification work key %q remains: %v", key, err)
		}
	}
}

func TestNotificationMaterializerConsumerStartsAtCreationBoundary(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)

	consumer, err := chattoCore.notificationMaterializer.createConsumer(ctx)
	if err != nil {
		t.Fatalf("createConsumer: %v", err)
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		t.Fatalf("consumer Info: %v", err)
	}
	if info.Config.DeliverPolicy != jetstream.DeliverNewPolicy {
		t.Fatalf("deliver policy = %v, want DeliverNewPolicy", info.Config.DeliverPolicy)
	}
}

func TestNotificationMaterializerWaitCurrentFencesRelevantEventTail(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	secondIndex := NewNotificationOccurrenceIndex(chattoCore.storage.runtimeStateKV, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- secondIndex.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("second notification index did not stop")
		}
	})
	if err := secondIndex.WaitReady(ctx); err != nil {
		t.Fatalf("second index WaitReady: %v", err)
	}
	event := &corev1.Event{
		Id:        "E-notification-wait-current",
		CreatedAt: timestamppb.Now(),
		ActorId:   SystemActorID,
		Event: &corev1.Event_UserAccountDeleted{UserAccountDeleted: &corev1.UserAccountDeletedEvent{
			UserId: "U-notification-wait-current",
		}},
	}
	sequence, err := chattoCore.EventPublisher.AppendEventually(ctx, evtstream.UserAggregate("U-notification-wait-current").SubjectFor(event), event)
	if err != nil {
		t.Fatalf("append notification-relevant event: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	info, err := chattoCore.notificationMaterializer.consumer.Info(ctx)
	if err != nil {
		t.Fatalf("consumer Info: %v", err)
	}
	if info.AckFloor.Stream < sequence {
		t.Fatalf("notification ack floor = %d, want at least %d", info.AckFloor.Stream, sequence)
	}
	readFence, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationReadFenceKey)
	if err != nil {
		t.Fatalf("get notification read fence: %v", err)
	}
	if len(readFence.Value()) != 8 || binary.BigEndian.Uint64(readFence.Value()) < sequence {
		t.Fatalf("notification read fence = %v, want EVT sequence at least %d", readFence.Value(), sequence)
	}
	chattoCore.notificationOccurrences.index.mu.RLock()
	observedRevision := chattoCore.notificationOccurrences.index.observedRevision
	chattoCore.notificationOccurrences.index.mu.RUnlock()
	if observedRevision < readFence.Revision() {
		t.Fatalf("local index revision = %d, want read fence revision at least %d", observedRevision, readFence.Revision())
	}
	if err := secondIndex.waitForObservedRevision(ctx, readFence.Revision()); err != nil {
		t.Fatalf("second index read fence wait: %v", err)
	}
}

func TestNotificationMaterializerRemovesOccurrencesAfterImplicitMembershipLoss(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		revoke     func(context.Context, *ChattoCore, string, string, string) error
		wantFilter string
	}{
		{
			name: "universal room disabled",
			revoke: func(ctx context.Context, chattoCore *ChattoCore, actorID, roomID, _ string) error {
				_, err := chattoCore.SetRoomUniversal(ctx, actorID, KindChannel, roomID, false)
				return err
			},
			wantFilter: evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged),
		},
		{
			name: "room join permission denied",
			revoke: func(ctx context.Context, chattoCore *ChattoCore, _, roomID, recipientID string) error {
				return chattoCore.DenyUserRoomPermission(ctx, SystemActorID, roomID, recipientID, PermRoomJoin)
			},
			wantFilter: evtstream.RBACEventTypeFilter(evtstream.EventRBACPermissionDenied),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			chattoCore, _ := setupTestCore(t)
			ctx := testContext(t)
			author, err := chattoCore.CreateUser(ctx, SystemActorID, "implicit-author", "Implicit Author", "password")
			if err != nil {
				t.Fatalf("CreateUser author: %v", err)
			}
			recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "implicit-recipient", "Implicit Recipient", "password")
			if err != nil {
				t.Fatalf("CreateUser recipient: %v", err)
			}
			room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "implicit-notification-room", "")
			if err != nil {
				t.Fatalf("CreateRoom: %v", err)
			}
			if _, err := chattoCore.SetRoomUniversal(ctx, author.Id, KindChannel, room.Id, true); err != nil {
				t.Fatalf("SetRoomUniversal true: %v", err)
			}

			message, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@implicit-recipient hello", nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("PostMessage: %v", err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatalf("WaitCurrent after message: %v", err)
			}
			occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox)
			if err != nil || len(occurrences) != 1 || occurrences[0].GetSourceEventId() != message.Id {
				t.Fatalf("occurrences before visibility loss = (%+v, %v), want source %s", occurrences, err, message.Id)
			}

			if err := testCase.revoke(ctx, chattoCore, author.Id, room.Id, recipient.Id); err != nil {
				t.Fatalf("revoke implicit membership: %v", err)
			}
			boundary, err := chattoCore.EventPublisher.LastSubjectPosition(ctx, testCase.wantFilter)
			if err != nil {
				t.Fatalf("read visibility event boundary: %v", err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatalf("WaitCurrent after visibility loss: %v", err)
			}
			if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 0 {
				t.Fatalf("occurrences after visibility loss = (%+v, %v), want empty", occurrences, err)
			}
			visibilityEntry, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationVisibilityBoundaryKey(recipient.Id, room.Id))
			if err != nil {
				t.Fatalf("get visibility boundary: %v", err)
			}
			if len(visibilityEntry.Value()) != 8 || binary.BigEndian.Uint64(visibilityEntry.Value()) < boundary.Seq {
				t.Fatalf("visibility boundary = %v, want sequence at least %d", visibilityEntry.Value(), boundary.Seq)
			}
		})
	}
}

func TestNotificationVisibilityReconciliationUsesEventBoundaryWhenProjectionIsAhead(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "ahead-author", "Ahead Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "ahead-recipient", "Ahead Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "ahead-notification-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.SetRoomUniversal(ctx, author.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}
	beforeLoss, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "before loss", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage before loss: %v", err)
	}
	beforeLossSequence, err := chattoCore.GetEventSequence(ctx, KindChannel, room.Id, beforeLoss.Id)
	if err != nil {
		t.Fatalf("GetEventSequence before loss: %v", err)
	}

	if _, err := chattoCore.SetRoomUniversal(ctx, author.Id, KindChannel, room.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal false: %v", err)
	}
	lossFilter := evtstream.RoomEventTypeFilter(evtstream.EventRoomUniversalChanged)
	lossEvents, _, err := chattoCore.EventPublisher.SubjectEventsWithSubjectsAfter(ctx, lossFilter, 0)
	if err != nil {
		t.Fatalf("read universal events: %v", err)
	}
	if len(lossEvents) == 0 {
		t.Fatal("read universal events: got none")
	}
	loss := lossEvents[len(lossEvents)-1]
	if loss.Event.GetRoomUniversalChanged().GetUniversal() {
		t.Fatalf("latest universal event = true, want loss event")
	}
	if _, err := chattoCore.SetRoomUniversal(ctx, author.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal restore: %v", err)
	}
	afterRegain, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "after regain", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage after regain: %v", err)
	}
	afterRegainSequence, err := chattoCore.GetEventSequence(ctx, KindChannel, room.Id, afterRegain.Id)
	if err != nil {
		t.Fatalf("GetEventSequence after regain: %v", err)
	}
	member, err := chattoCore.RoomMembershipExists(ctx, KindChannel, recipient.Id, room.Id)
	if err != nil || !member {
		t.Fatalf("current restored membership = (%v, %v), want true", member, err)
	}

	createOccurrence := func(sourceID, targetID string, sequence uint64) *corev1.NotificationOccurrence {
		t.Helper()
		occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
			RecipientID:          recipient.Id,
			SourceEventID:        sourceID,
			SourceCreated:        time.Now().UTC(),
			SourceStreamSequence: sequence,
			ActorID:              author.Id,
			Target:               &corev1.NotificationTarget{RoomId: room.Id, EventId: targetID},
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
			SkipReadLookup: true,
		})
		if err != nil || !created {
			t.Fatalf("Create occurrence %s = (%+v, %v, %v), want created", sourceID, occurrence, created, err)
		}
		return occurrence
	}
	preLossOccurrence := createOccurrence("ahead-pre-loss", beforeLoss.Id, beforeLossSequence)
	postRegainOccurrence := createOccurrence("ahead-post-regain", afterRegain.Id, afterRegainSequence)
	// The live worker has already released this acknowledged boundary. Reapply
	// the loss to the dedicated projection to exercise reconciliation against
	// the same retained exact-boundary state while the owning projections remain
	// ahead at the restored value.
	if err := chattoCore.notificationMaterializer.visibility.Projection().Apply(loss.Event, loss.Sequence); err != nil {
		t.Fatalf("recapture loss boundary: %v", err)
	}

	if err := chattoCore.notificationMaterializer.reconcileOccurrenceVisibility(
		ctx,
		recipient.Id,
		room.Id,
		loss.Sequence,
		loss.Event.GetCreatedAt().AsTime(),
	); err != nil {
		t.Fatalf("reconcile loss after projection restored: %v", err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, recipient.Id, preLossOccurrence.Id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pre-loss occurrence error = %v, want not found", err)
	}
	if occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, recipient.Id, postRegainOccurrence.Id); err != nil || occurrence.GetId() != postRegainOccurrence.Id {
		t.Fatalf("post-regain occurrence = (%+v, %v), want preserved", occurrence, err)
	}
}

func TestNotificationMaterializerSkipsFactsOutsideRetentionWindow(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	source := &corev1.Event{
		Id:        "E-expired-notification-source",
		CreatedAt: timestamppb.New(time.Now().UTC().Add(-notificationTTL - time.Hour)),
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId: "R-expired-notification-source",
		}},
	}
	markerKey := notificationWorkMarkerKey(source.GetId())
	if _, err := chattoCore.storage.runtimeStateKV.Create(ctx, markerKey, nil); err != nil {
		t.Fatalf("create stale marker: %v", err)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Create(ctx, notificationWorkKey(source.GetId(), "U-stale"), []byte("not protobuf")); err != nil {
		t.Fatalf("create stale work: %v", err)
	}

	if err := chattoCore.notificationMaterializer.materializeEvent(ctx, source, 1, true); err != nil {
		t.Fatalf("MaterializeEvent: %v", err)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, markerKey); err != nil {
		t.Fatalf("expired delivery unexpectedly inspected or deleted work: %v", err)
	}
}

func TestReactionRemovalFromLegacyWriterRemovesV2Occurrence(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "legacy-remove-author", "Legacy Remove Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reactor, err := chattoCore.CreateUser(ctx, SystemActorID, "legacy-remove-reactor", "Legacy Remove Reactor", "password")
	if err != nil {
		t.Fatalf("CreateUser reactor: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "legacy-reaction-removal-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, reactor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "react here", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	input := ReactionMutationInput{ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup"}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, input); err != nil || !added {
		t.Fatalf("AddReaction = (%v, %v), want added", added, err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox)
	if err != nil || len(occurrences) != 1 {
		t.Fatalf("reaction occurrences = (%d, %v), want one", len(occurrences), err)
	}
	if occurrences[0].GetReactionEmoji() != "thumbsup" {
		t.Fatalf("reaction emoji = %q, want thumbsup", occurrences[0].GetReactionEmoji())
	}

	// Simulate an old replica: append the existing domain fact without any
	// Notifications 2.0 runtime work or marker.
	removed := newReactionRemovedEvent(reactor.Id, room.Id, posted.Id, "thumbsup")
	if _, err := chattoCore.EventPublisher.AppendEventually(ctx, evtstream.RoomAggregate(room.Id).SubjectFor(removed), removed); err != nil {
		t.Fatalf("append legacy removal: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		occurrences, err = chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox)
		if err == nil && len(occurrences) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("legacy removal left occurrence visible: occurrences=%+v err=%v", occurrences, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLateNotificationOccurrenceStartsReadWhenCursorAlreadyCoversTarget(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "late-author", "Late Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "late-reader", "Late Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "late-notification-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, reader.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "already read", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	postedEntry, ok := chattoCore.roomModel.timelineEntry(posted.Id)
	if !ok {
		t.Fatal("posted message missing from timeline")
	}

	occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   reader.Id,
		SourceEventID: posted.Id,
		// Coverage is causal, not timestamp-based. A skewed future timestamp must
		// not turn an already-covered source back into an unread notification.
		SourceCreated:        posted.GetCreatedAt().AsTime().Add(time.Hour),
		SourceStreamSequence: postedEntry.StreamSeq,
		ActorID:              author.Id,
		Target:               &corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
	})
	if err != nil || !created {
		t.Fatalf("Create late occurrence = (%v, %v, %v)", occurrence, created, err)
	}
	if occurrence.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ {
		t.Fatalf("late occurrence state = %v, want READ", occurrence.GetInboxState())
	}
	if occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("late occurrence alert state = %v, want SILENCED", occurrence.GetAlertState())
	}
}

func TestDuplicateMaterializationPreservesExplicitMarkUnread(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "race-author", "Race Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "race-reader", "Race Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "race-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, reader.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "race target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	postedEntry, ok := chattoCore.roomModel.timelineEntry(posted.Id)
	if !ok {
		t.Fatal("posted message missing from timeline")
	}
	input := CreateNotificationOccurrenceInput{
		RecipientID:          reader.Id,
		SourceEventID:        posted.Id,
		SourceCreated:        posted.GetCreatedAt().AsTime(),
		SourceStreamSequence: postedEntry.StreamSeq,
		ActorID:              author.Id,
		Target:               &corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	}
	created, wasCreated, err := chattoCore.NotificationOccurrences().Create(ctx, input)
	if err != nil || !wasCreated || created.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
		t.Fatalf("Create missed-race occurrence = (%v, %v, %v)", created, wasCreated, err)
	}
	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	// An explicit Mark unread is user-owned triage and must survive durable
	// source redelivery.
	unread := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD
	if _, err := chattoCore.NotificationOccurrences().Update(ctx, reader.Id, created.GetId(), UpdateNotificationOccurrenceInput{InboxState: &unread}); err != nil {
		t.Fatalf("restore stale unread occurrence: %v", err)
	}
	input.SkipReadLookup = false
	reconciled, wasCreated, err := chattoCore.NotificationOccurrences().Create(ctx, input)
	if err != nil || wasCreated {
		t.Fatalf("duplicate Create = (%v, %v, %v)", reconciled, wasCreated, err)
	}
	if reconciled.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
		t.Fatalf("duplicate state = %v, want UNREAD", reconciled.GetInboxState())
	}
	if reconciled.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("duplicate alert state = %v, want SILENCED", reconciled.GetAlertState())
	}
}

func TestReactionRemovalPreservesCausallyLaterReadd(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	now := time.Now().UTC()
	create := func(sourceID string, sequence uint64) *corev1.NotificationOccurrence {
		t.Helper()
		occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
			RecipientID:          "U-reaction-recipient",
			SourceEventID:        sourceID,
			SourceCreated:        now,
			SourceStreamSequence: sequence,
			ActorID:              "U-reaction-actor",
			Target:               &corev1.NotificationTarget{RoomId: "R-reaction", EventId: "E-message"},
			ReactionEmoji:        "thumbsup",
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
			SkipReadLookup: true,
		})
		if err != nil || !created {
			t.Fatalf("Create(%s) = (%v, %v, %v)", sourceID, occurrence, created, err)
		}
		return occurrence
	}
	older := create("E-reaction-before-removal", 100)
	later := create("E-reaction-after-removal", 300)

	removed, err := chattoCore.NotificationOccurrences().RemoveReaction(
		ctx,
		"U-reaction-recipient",
		"R-reaction",
		"E-message",
		"U-reaction-actor",
		"thumbsup",
		200,
	)
	if err != nil || removed != 1 {
		t.Fatalf("RemoveReaction = (%d, %v), want one", removed, err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, "U-reaction-recipient", older.GetId()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("older reaction occurrence remains: %v", err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, "U-reaction-recipient", later.GetId()); err != nil {
		t.Fatalf("causally later re-add was removed: %v", err)
	}
}

func TestVisibilityBoundaryRejectsDelayedSourceAfterRejoin(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chattoCore.CreateUser(ctx, SystemActorID, "boundary-owner", "Boundary Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	member, err := chattoCore.CreateUser(ctx, SystemActorID, "boundary-member", "Boundary Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, owner.Id, KindChannel, "", "boundary-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := chattoCore.LeaveRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	boundaryEntry, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationVisibilityBoundaryKey(member.Id, room.Id))
	if err != nil {
		t.Fatalf("read visibility boundary: %v", err)
	}
	boundarySequence := binary.BigEndian.Uint64(boundaryEntry.Value())
	if boundarySequence == 0 {
		t.Fatal("visibility boundary sequence is zero")
	}
	if _, err := chattoCore.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("rejoin room: %v", err)
	}

	makeSource := func(id string) (*corev1.Event, *corev1.NotificationOccurrence) {
		t.Helper()
		source := newEvent(owner.Id, &corev1.Event{Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: room.Id},
		}})
		source.Id = id
		work := newNotificationOccurrenceWork(
			source,
			&corev1.NotificationTarget{RoomId: room.Id, EventId: source.Id},
			[]notificationRecipientDecision{{
				recipientID: member.Id,
				reasons: []*corev1.NotificationReasonMatch{{
					Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
					Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
				}},
			}},
		)
		return source, work[0]
	}
	older, olderWork := makeSource("E-before-leave")
	if err := chattoCore.notificationMaterializer.materializeOccurrence(ctx, older, olderWork, boundarySequence); err != nil {
		t.Fatalf("materialize older source: %v", err)
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, member.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 0 {
		t.Fatalf("older occurrences = (%v, %v), want none", occurrences, err)
	}

	newer, newerWork := makeSource("E-after-rejoin")
	if err := chattoCore.notificationMaterializer.materializeOccurrence(ctx, newer, newerWork, boundarySequence+1); err != nil {
		t.Fatalf("materialize newer source: %v", err)
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, member.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 1 || occurrences[0].GetSourceEventId() != newer.GetId() {
		t.Fatalf("newer occurrences = (%v, %v), want newer source", occurrences, err)
	}
}

func TestHistoricalLeaveReplayDoesNotRemoveNotificationsAfterRejoin(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chattoCore.CreateUser(ctx, SystemActorID, "replay-owner", "Replay Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	member, err := chattoCore.CreateUser(ctx, SystemActorID, "replay-member", "Replay Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, owner.Id, KindChannel, "", "replay-membership-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	leftAt := time.Now().UTC()
	if err := chattoCore.LeaveRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("rejoin room: %v", err)
	}
	olderOccurrence, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:          member.Id,
		SourceEventID:        "E-before-leave",
		SourceCreated:        leftAt.Add(-time.Second),
		ActorID:              owner.Id,
		SourceStreamSequence: 100,
		Target:               &corev1.NotificationTarget{RoomId: room.Id, EventId: "E-before-leave"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create older occurrence: %v", err)
	}
	occurrence, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:          member.Id,
		SourceEventID:        "E-after-rejoin",
		SourceCreated:        leftAt.Add(time.Second),
		ActorID:              owner.Id,
		SourceStreamSequence: 300,
		Target:               &corev1.NotificationTarget{RoomId: room.Id, EventId: "E-after-rejoin"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create occurrence: %v", err)
	}
	if isMember, err := chattoCore.RoomMembershipExists(ctx, KindChannel, member.Id, room.Id); err != nil || !isMember {
		t.Fatalf("membership after rejoin = %v, %v", isMember, err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, member.Id, occurrence.GetId()); err != nil {
		t.Fatalf("notification before historical leave replay: %v", err)
	}

	err = chattoCore.notificationMaterializer.materializeEvent(ctx, &corev1.Event{
		Id:        "E-old-leave",
		ActorId:   member.Id,
		CreatedAt: timestamppb.New(leftAt),
		Event: &corev1.Event_UserLeftRoom{UserLeftRoom: &corev1.UserLeftRoomEvent{
			RoomId: room.Id,
		}},
	}, 200, true)
	if err != nil {
		t.Fatalf("replay historical leave: %v", err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, member.Id, occurrence.GetId()); err != nil {
		t.Fatalf("notification after historical leave replay: %v", err)
	}
	if _, err := chattoCore.NotificationOccurrences().Get(ctx, member.Id, olderOccurrence.GetId()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("older notification after historical leave replay = %v, want not found", err)
	}
}

func TestHistoricalNotificationReplaySkipsDeletedRecipient(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "deleted-notification-recipient", "Deleted Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "deleted-notification-actor", "Notification Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	if err := chattoCore.DeleteUser(ctx, recipient.Id, recipient.Id); err != nil {
		t.Fatalf("DeleteUser recipient: %v", err)
	}

	source := &corev1.Event{
		Id:        "E-before-account-deletion",
		ActorId:   actor.Id,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId: "R-deleted-recipient",
		}},
	}
	work := newNotificationOccurrenceWork(
		source,
		&corev1.NotificationTarget{RoomId: "R-deleted-recipient", EventId: source.GetId()},
		[]notificationRecipientDecision{{
			recipientID: recipient.Id,
			reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, source, work); err != nil {
		t.Fatalf("StoreWork: %v", err)
	}
	err = chattoCore.notificationMaterializer.materializeEvent(ctx, source, 1, true)
	if err != nil {
		t.Fatalf("replay notification source after account deletion: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox)
	if err != nil {
		t.Fatalf("List occurrences: %v", err)
	}
	if len(occurrences) != 0 {
		t.Fatalf("occurrences after deleted-recipient replay = %d, want 0", len(occurrences))
	}
}

func TestHistoricalNotificationReplaySkipsDeletedRoom(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "deleted-room-recipient", "Deleted Room Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "deleted-room-actor", "Deleted Room Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}

	source := &corev1.Event{
		Id:        "E-in-deleted-room",
		ActorId:   actor.Id,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId: "R-already-deleted",
		}},
	}
	work := newNotificationOccurrenceWork(
		source,
		&corev1.NotificationTarget{RoomId: "R-already-deleted", EventId: source.GetId()},
		[]notificationRecipientDecision{{
			recipientID: recipient.Id,
			reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, source, work); err != nil {
		t.Fatalf("StoreWork: %v", err)
	}
	err = chattoCore.notificationMaterializer.materializeEvent(ctx, source, 1, true)
	if err != nil {
		t.Fatalf("replay notification source after room deletion: %v", err)
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences after deleted-room replay = (%v, %v), want none", occurrences, err)
	}
}

func TestDelayedMessageNotificationRetryDoesNotOutrunRetraction(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "retracted-notification-author", "Retracted Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "retracted-notification-recipient", "Retracted Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "retracted-notification-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, recipient.Id, KindChannel, recipient.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@retracted-notification-recipient hello", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := chattoCore.NotificationOccurrences().PurgeUser(ctx, recipient.Id); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	if err := chattoCore.DeleteMessage(ctx, author.Id, KindChannel, room.Id, posted.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	work := newNotificationOccurrenceWork(
		posted,
		&corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		[]notificationRecipientDecision{{
			recipientID: recipient.Id,
			reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
			}},
		}},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, posted, work); err != nil {
		t.Fatalf("StoreWork: %v", err)
	}
	if err := chattoCore.notificationMaterializer.materializeEvent(ctx, posted, 100, true); err != nil {
		t.Fatalf("retry message materialization: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, recipient.Id, NotificationOccurrenceViewInbox)
	if err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences after delayed retracted retry = (%+v, %v), want empty", occurrences, err)
	}
}

func TestDelayedReactionNotificationRetryDoesNotOutrunRemoval(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "removed-reaction-author", "Reaction Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reactor, err := chattoCore.CreateUser(ctx, SystemActorID, "removed-reaction-actor", "Reaction Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser reactor: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "removed-reaction-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, reactor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "react here", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	input := ReactionMutationInput{ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup"}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, input); err != nil || !added {
		t.Fatalf("AddReaction = (%v, %v), want added", added, err)
	}
	snapshot := chattoCore.roomModel.reactionMutationSnapshot(room.Id, posted.Id, "thumbsup", reactor.Id)
	if snapshot.SourceEventID == "" {
		t.Fatal("reaction source event ID is empty")
	}
	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 1 {
		t.Fatalf("reaction notification occurrences = (%d, %v), want one", len(occurrences), err)
	}
	if _, err := chattoCore.NotificationOccurrences().PurgeUser(ctx, author.Id); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	if removed, err := chattoCore.ReactionModel().RemoveReaction(ctx, input); err != nil || !removed {
		t.Fatalf("RemoveReaction = (%v, %v), want removed", removed, err)
	}
	addEvent := newReactionAddedEvent(reactor.Id, room.Id, posted.Id, "thumbsup")
	addEvent.Id = snapshot.SourceEventID
	addEvent.CreatedAt = timestamppb.Now()
	decision := notificationRecipientDecision{
		recipientID: author.Id,
		reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
	}
	work := newNotificationOccurrenceWork(
		addEvent,
		&corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		[]notificationRecipientDecision{decision},
	)
	if err := chattoCore.notificationMaterializer.StoreWork(ctx, addEvent, work); err != nil {
		t.Fatalf("StoreWork: %v", err)
	}
	if err := chattoCore.notificationMaterializer.materializeEvent(ctx, addEvent, 100, true); err != nil {
		t.Fatalf("retry reaction materialization: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox)
	if err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences after delayed removed-reaction retry = (%+v, %v), want empty", occurrences, err)
	}
}
