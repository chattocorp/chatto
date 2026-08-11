package core

import (
	"errors"
	"testing"
	"time"

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
	planEvents, _, err := chattoCore.EventPublisher.SubjectEvents(
		ctx,
		evtstream.NotificationAggregate(reply.Id).Subject(evtstream.EventNotificationOccurrencePlanned),
	)
	if err != nil || len(planEvents) != 1 {
		t.Fatalf("notification plan events = (%d, %v), want one", len(planEvents), err)
	}
	plan := planEvents[0].GetNotificationOccurrencePlanned()
	if plan.GetSourceEventId() != reply.Id || plan.GetTarget().GetEventId() != reply.Id || len(plan.GetRecipients()) != 1 {
		t.Fatalf("notification plan = %+v, want reply source and one recipient", plan)
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
		RecipientID:   member.Id,
		SourceEventID: "E-before-leave",
		SourceCreated: leftAt.Add(-time.Second),
		ActorID:       owner.Id,
		Target:        &corev1.NotificationTarget{RoomId: room.Id, EventId: "E-before-leave"},
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
		RecipientID:   member.Id,
		SourceEventID: "E-after-rejoin",
		SourceCreated: leftAt.Add(time.Second),
		ActorID:       owner.Id,
		Target:        &corev1.NotificationTarget{RoomId: room.Id, EventId: "E-after-rejoin"},
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

	err = chattoCore.notificationMaterializer.MaterializeEvent(ctx, &corev1.Event{
		Id:        "E-old-leave",
		ActorId:   member.Id,
		CreatedAt: timestamppb.New(leftAt),
		Event: &corev1.Event_UserLeftRoom{UserLeftRoom: &corev1.UserLeftRoomEvent{
			RoomId: room.Id,
		}},
	})
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
	plan := newNotificationOccurrencePlan(
		source,
		corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_MESSAGE,
		&corev1.NotificationTarget{RoomId: "R-deleted-recipient", EventId: source.GetId()},
		[]*corev1.NotificationRecipientDecision{{
			RecipientId: recipient.Id,
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
		"",
	)
	err = chattoCore.notificationMaterializer.MaterializeEvent(ctx, plan)
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
	plan := newNotificationOccurrencePlan(
		source,
		corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_MESSAGE,
		&corev1.NotificationTarget{RoomId: "R-already-deleted", EventId: source.GetId()},
		[]*corev1.NotificationRecipientDecision{{
			RecipientId: recipient.Id,
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			}},
		}},
		"",
	)
	err = chattoCore.notificationMaterializer.MaterializeEvent(ctx, plan)
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
	plan := newNotificationOccurrencePlan(
		posted,
		corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_MESSAGE,
		&corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		[]*corev1.NotificationRecipientDecision{{
			RecipientId: recipient.Id,
			Reasons: []*corev1.NotificationReasonMatch{{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
			}},
		}},
		"",
	)
	if err := chattoCore.notificationMaterializer.MaterializeEvent(ctx, plan); err != nil {
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
	if _, err := chattoCore.JoinRoom(ctx, reactor.Id, KindChannel, reactor.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
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
	plans, _, err := chattoCore.EventPublisher.SubjectEvents(
		ctx,
		evtstream.NotificationAggregate(snapshot.SourceEventID).Subject(evtstream.EventNotificationOccurrencePlanned),
	)
	if err != nil || len(plans) != 1 {
		t.Fatalf("reaction notification plans = (%d, %v), want one", len(plans), err)
	}
	planRecipients := plans[0].GetNotificationOccurrencePlanned().GetRecipients()
	if len(planRecipients) != 1 || planRecipients[0].GetRecipientId() != author.Id {
		t.Fatalf("reaction notification plan recipients = %+v, want author", planRecipients)
	}
	if _, err := chattoCore.NotificationOccurrences().PurgeUser(ctx, author.Id); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	if removed, err := chattoCore.ReactionModel().RemoveReaction(ctx, input); err != nil || !removed {
		t.Fatalf("RemoveReaction = (%v, %v), want removed", removed, err)
	}
	revocations, _, err := chattoCore.EventPublisher.SubjectEvents(
		ctx,
		evtstream.NotificationAggregate(snapshot.SourceEventID).Subject(evtstream.EventNotificationOccurrenceRevoked),
	)
	if err != nil || len(revocations) != 1 {
		t.Fatalf("reaction notification revocations = (%d, %v), want one", len(revocations), err)
	}
	addEvent := newReactionAddedEvent(reactor.Id, room.Id, posted.Id, "thumbsup")
	addEvent.Id = snapshot.SourceEventID
	addEvent.CreatedAt = timestamppb.Now()
	decision := &corev1.NotificationRecipientDecision{
		RecipientId: author.Id,
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REACTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
	}
	plan := newNotificationOccurrencePlan(
		addEvent,
		corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_REACTION,
		&corev1.NotificationTarget{RoomId: room.Id, EventId: posted.Id},
		[]*corev1.NotificationRecipientDecision{decision},
		"thumbsup",
	)
	if err := chattoCore.notificationMaterializer.MaterializeEvent(ctx, plan); err != nil {
		t.Fatalf("retry reaction materialization: %v", err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id, NotificationOccurrenceViewInbox)
	if err != nil || len(occurrences) != 0 {
		t.Fatalf("occurrences after delayed removed-reaction retry = (%+v, %v), want empty", occurrences, err)
	}
}
