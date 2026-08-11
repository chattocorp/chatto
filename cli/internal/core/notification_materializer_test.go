package core

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

	if err := chattoCore.notificationMaterializer.MaterializeEvent(ctx, source); err != nil {
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

	occurrence, created, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   reader.Id,
		SourceEventID: "E-late-materialization",
		SourceCreated: posted.GetCreatedAt().AsTime(),
		ActorID:       author.Id,
		Target:        &corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
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

func TestReactionRemovalSkipsPromptMaterializedLaterReadd(t *testing.T) {
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
	later := create("E-reaction-after-removal", 0)

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
		t.Fatalf("prompt-materialized later re-add was removed: %v", err)
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
	err = chattoCore.notificationMaterializer.MaterializeEvent(ctx, source)
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
	err = chattoCore.notificationMaterializer.MaterializeEvent(ctx, source)
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
	if err := chattoCore.notificationMaterializer.MaterializeEvent(ctx, posted); err != nil {
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
	if err := chattoCore.notificationMaterializer.MaterializeEvent(ctx, addEvent); err != nil {
		t.Fatalf("retry reaction materialization: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox)
	if err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences after delayed removed-reaction retry = (%+v, %v), want empty", occurrences, err)
	}
}
