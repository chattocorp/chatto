package core

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

func TestUnknownPersistedDeliveryModeFailsClosedWithoutStallingMaterializer(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "unknown-mode-author", "Unknown Mode Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "unknown-mode-recipient", "Unknown Mode Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "unknown-mode-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}

	unknown := evtv1.NotificationDeliveryMode(99)
	policyChanged := newEvent(recipient.Id, &evtv1.Event{Event: &evtv1.Event_UserNotificationPolicyChanged{
		UserNotificationPolicyChanged: &evtv1.UserNotificationPolicyChangedEvent{
			UserId: recipient.Id,
			Overrides: &evtv1.NotificationDeliveryModes{
				DirectMentions: &unknown,
			},
		},
	}})
	subject := evtstream.ConfigSubjectAggregate(recipient.Id).SubjectFor(policyChanged)
	seq, err := chattoCore.EventPublisher.AppendEventually(ctx, subject, policyChanged)
	if err != nil {
		t.Fatalf("append future policy event: %v", err)
	}
	if err := chattoCore.configModel.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("wait for future policy event: %v", err)
	}

	first, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@unknown-mode-recipient first", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("materializer stalled on unknown delivery mode: %v", err)
	}
	if got := testNotificationOccurrences(t, chattoCore, recipient.Id); len(got) != 0 {
		t.Fatalf("unknown mode occurrences = %+v, want none for source %s", got, first.GetId())
	}

	if _, err := chattoCore.NotificationPolicy().UpdateNotificationPolicy(ctx, recipient.Id, "",
		&evtv1.NotificationDeliveryModes{DirectMentions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"direct_mentions"}},
	); err != nil {
		t.Fatalf("replace future policy mode: %v", err)
	}
	second, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@unknown-mode-recipient second", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for materializer after known mode: %v", err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id)
	if len(occurrences) != 1 || occurrences[0].GetSourceEventId() != second.GetId() {
		t.Fatalf("occurrences after replacing future mode = %+v, want only source %s", occurrences, second.GetId())
	}
}

func TestBadgeReactionAddsOnlyUnreadAttentionUntilRoomRead(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-reaction-author", "Badge Reaction Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	reactor, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-reaction-reactor", "Badge Reaction Reactor", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-reaction-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{author.Id, reactor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "react here", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, author.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("mark initial message read: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, author.Id, room.Id, notificationTestSignalReaction,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	); err != nil {
		t.Fatalf("set Badge policy: %v", err)
	}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction = (%v, %v)", added, err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge materialization: %v", err)
	}
	if occurrences := testNotificationOccurrences(t, chattoCore, author.Id); len(occurrences) != 0 {
		t.Fatalf("Badge occurrences = %+v, want none", occurrences)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || !unread {
		t.Fatalf("room unread after Badge reaction = (%v, %v), want (true, nil)", unread, err)
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, author.Id, room.Id, notificationTestSignalReaction,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable future Badge reactions: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || !unread {
		t.Fatalf("existing Badge after policy change = (%v, %v), want (true, nil)", unread, err)
	}
	if removed, err := chattoCore.ReactionModel().RemoveReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !removed {
		t.Fatalf("RemoveReaction = (%v, %v)", removed, err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge reaction removal: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || unread {
		t.Fatalf("room unread after reaction removal = (%v, %v), want (false, nil)", unread, err)
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, author.Id, room.Id, notificationTestSignalReaction,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	); err != nil {
		t.Fatalf("restore Badge policy: %v", err)
	}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction again = (%v, %v)", added, err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for replacement Badge reaction: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || !unread {
		t.Fatalf("room unread after replacement Badge = (%v, %v), want (true, nil)", unread, err)
	}
	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, author.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("mark Badge reaction read: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || unread {
		t.Fatalf("room unread after read boundary = (%v, %v), want (false, nil)", unread, err)
	}
	if removed, err := chattoCore.ReactionModel().RemoveReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !removed {
		t.Fatalf("RemoveReaction after read = (%v, %v)", removed, err)
	}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction after read = (%v, %v)", added, err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge after read: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || !unread {
		t.Fatalf("later Badge after read boundary = (%v, %v), want (true, nil)", unread, err)
	}
	if err := chattoCore.LeaveRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("leave Badge room: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge visibility loss: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("rejoin Badge room: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, author.Id, room.Id); err != nil || unread {
		t.Fatalf("old Badge after visibility regain = (%v, %v), want (false, nil)", unread, err)
	}
}

func TestBadgeMarkerRejectsAnOlderReplicaWrite(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-monotonic-recipient", "Badge Monotonic Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-monotonic-author", "Badge Monotonic Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-monotonic-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "Badge source", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	input := CreateNotificationOccurrenceInput{
		RecipientID:          recipient.Id,
		SourceEventID:        posted.Id,
		SourceCreated:        time.Now(),
		ActorID:              author.Id,
		Signal:               testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
		Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
		SourceStreamSequence: 100,
	}
	if changed, err := chattoCore.notificationOccurrences.recordNotificationUnreadMarker(ctx, input); err != nil || !changed {
		t.Fatalf("record newer Badge marker = (%v, %v), want (true, nil)", changed, err)
	}
	input.SourceStreamSequence = 99
	if changed, err := chattoCore.notificationOccurrences.recordNotificationUnreadMarker(ctx, input); err != nil || changed {
		t.Fatalf("record older Badge marker = (%v, %v), want (false, nil)", changed, err)
	}
	marker, _, exists, err := chattoCore.notificationBoundaries.unreadMarker(ctx, notificationReadBoundaryScope{
		userID: recipient.Id, roomID: room.Id,
	})
	if err != nil || !exists || marker.GetSourceStreamSequence() != 100 {
		t.Fatalf("stored Badge marker = (%+v, %v, %v), want sequence 100", marker, exists, err)
	}
}

func TestActiveBadgeMarkerAdvanceDoesNotRequestAnotherInvalidation(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-stable-recipient", "Badge Stable Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-stable-author", "Badge Stable Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-stable-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, recipient.Id, room.Id, notificationTestSignalRoomMessage,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable automatic room Badge: %v", err)
	}

	post := func(body string) (*evtv1.Event, uint64) {
		t.Helper()
		posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, body, nil, "", "", nil, false)
		if err != nil {
			t.Fatal(err)
		}
		entry, exists := chattoCore.roomModel.timelineEntry(posted.Id)
		if !exists || entry.StreamSeq == 0 {
			t.Fatalf("source timeline entry = (%+v, %v)", entry, exists)
		}
		return posted, entry.StreamSeq
	}
	input := func(posted *evtv1.Event, sequence uint64) CreateNotificationOccurrenceInput {
		return CreateNotificationOccurrenceInput{
			RecipientID: recipient.Id, SourceEventID: posted.Id, ActorID: author.Id,
			SourceCreated:        posted.GetCreatedAt().AsTime(),
			Signal:               testNotificationSignal(notificationTestSignalRoomMessage, room.Id, posted.Id),
			Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
			SourceStreamSequence: sequence,
		}
	}

	first, firstSequence := post("first Badge source")
	firstWrite, err := chattoCore.notificationOccurrences.writeNotificationUnreadMarker(ctx, input(first, firstSequence))
	if err != nil || !firstWrite.changed || !firstWrite.notify {
		t.Fatalf("first Badge write = (%+v, %v), want changed and notify", firstWrite, err)
	}
	if err := chattoCore.notificationBoundaries.waitForRevision(ctx, firstWrite.key, firstWrite.revision); err != nil {
		t.Fatal(err)
	}

	second, secondSequence := post("second Badge source")
	secondWrite, err := chattoCore.notificationOccurrences.writeNotificationUnreadMarker(ctx, input(second, secondSequence))
	if err != nil || !secondWrite.changed || secondWrite.notify {
		t.Fatalf("active Badge advance = (%+v, %v), want changed without notify", secondWrite, err)
	}
}

func TestBadgeMaterializationPipelinesHighFanoutMarkers(t *testing.T) {
	chattoCore, nc := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-fanout-author", "Badge Fanout Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-fanout-room", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chattoCore.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatal(err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "fanout source", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	entry, exists := chattoCore.roomModel.timelineEntry(posted.Id)
	if !exists || entry.StreamSeq == 0 {
		t.Fatalf("source timeline entry = (%+v, %v)", entry, exists)
	}
	invalidationSub, err := nc.SubscribeSync("live.sync.user.*.notification_unread")
	if err != nil {
		t.Fatalf("subscribe to Badge invalidations: %v", err)
	}
	defer invalidationSub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush Badge invalidation subscription: %v", err)
	}
	inputs := make([]CreateNotificationOccurrenceInput, 250)
	for index := range inputs {
		inputs[index] = CreateNotificationOccurrenceInput{
			RecipientID:          fmt.Sprintf("fanout-recipient-%03d", index),
			SourceEventID:        posted.Id,
			SourceCreated:        posted.GetCreatedAt().AsTime(),
			ActorID:              author.Id,
			Signal:               testNotificationSignal(notificationTestSignalRoomMessage, room.Id, posted.Id),
			Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
			SourceStreamSequence: entry.StreamSeq,
		}
	}
	if err := chattoCore.notificationMaterializer.materializeInputs(ctx, inputs, entry.StreamSeq); err != nil {
		t.Fatalf("materialize high-fanout Badge markers: %v", err)
	}
	invalidationsBySubject := make(map[string]int, len(inputs))
	for range inputs {
		message, err := invalidationSub.NextMsg(time.Second)
		if err != nil {
			t.Fatalf("receive high-fanout Badge invalidation: %v", err)
		}
		invalidationsBySubject[message.Subject]++
	}
	for _, input := range inputs {
		marker, _, exists, err := chattoCore.notificationBoundaries.unreadMarker(ctx, notificationReadBoundaryScope{
			userID: input.RecipientID, roomID: room.Id,
		})
		if err != nil || !exists || marker.GetSourceEventId() != posted.Id {
			t.Fatalf("marker for %s = (%+v, %v, %v), want source %s", input.RecipientID, marker, exists, err, posted.Id)
		}
		invalidationSubject := subjects.LiveSyncUserEvent(input.RecipientID, "notification_unread")
		if invalidationsBySubject[invalidationSubject] != 1 {
			t.Fatalf("Badge invalidations for %s = %d, want 1", input.RecipientID, invalidationsBySubject[invalidationSubject])
		}
	}
}

func TestBadgeMarkerIsRemovedWhenRecipientAccountIsDeleted(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-delete-recipient", "Badge Delete Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-delete-author", "Badge Delete Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-delete-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "Badge source", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := chattoCore.notificationOccurrences.recordNotificationUnreadMarker(ctx, CreateNotificationOccurrenceInput{
		RecipientID: recipient.Id, SourceEventID: posted.Id, ActorID: author.Id,
		SourceCreated: time.Now(),
		Signal:        testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
		Mode:          evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, SourceStreamSequence: 100,
	}); err != nil || !changed {
		t.Fatalf("record Badge marker = (%v, %v), want (true, nil)", changed, err)
	}

	if err := chattoCore.DeleteUser(ctx, SystemActorID, recipient.Id); err != nil {
		t.Fatalf("delete Badge recipient: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge account cleanup: %v", err)
	}
	marker, _, exists, err := chattoCore.notificationBoundaries.unreadMarker(ctx, notificationReadBoundaryScope{
		userID: recipient.Id, roomID: room.Id,
	})
	if err != nil || exists || marker != nil {
		t.Fatalf("Badge marker after account deletion = (%+v, %v, %v), want (nil, false, nil)", marker, exists, err)
	}
}

func TestExpiredBadgeSourceDoesNotCreateUnreadMarker(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-expired-recipient", "Badge Expired Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-expired-author", "Badge Expired Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "badge-expired-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, recipient.Id, room.Id, notificationTestSignalRoomMessage,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable ordinary room Badge: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "Expired Badge source", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	chattoCore.notificationOccurrences.now = func() time.Time { return now }
	changed, err := chattoCore.notificationOccurrences.recordNotificationUnreadMarker(ctx, CreateNotificationOccurrenceInput{
		RecipientID: recipient.Id, SourceEventID: posted.Id, ActorID: author.Id,
		SourceCreated: now.Add(-notificationTTL),
		Signal:        testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
		Mode:          evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, SourceStreamSequence: 100,
	})
	if err != nil || changed {
		t.Fatalf("record expired Badge marker = (%v, %v), want (false, nil)", changed, err)
	}
	marker, _, exists, err := chattoCore.notificationBoundaries.unreadMarker(ctx, notificationReadBoundaryScope{
		userID: recipient.Id, roomID: room.Id,
	})
	if err != nil || exists || marker != nil {
		t.Fatalf("expired Badge marker = (%+v, %v, %v), want (nil, false, nil)", marker, exists, err)
	}
}

func TestBadgeThreadReplyRollsUpAndClearsAtThreadBoundary(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	rootAuthor, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-thread-root", "Badge Thread Root", "password")
	if err != nil {
		t.Fatal(err)
	}
	replyAuthor, err := chattoCore.CreateUser(ctx, SystemActorID, "badge-thread-reply", "Badge Thread Reply", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, rootAuthor.Id, KindChannel, "", "badge-thread-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{rootAuthor.Id, replyAuthor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	root, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, rootAuthor.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, rootAuthor.Id, room.Id, root.Id); err != nil {
		t.Fatalf("mark root read: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, rootAuthor.Id, room.Id, notificationTestSignalFollowedThread,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	); err != nil {
		t.Fatalf("set Badge policy: %v", err)
	}
	reply, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, replyAuthor.Id, "thread reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for Badge materialization: %v", err)
	}
	if occurrences := testNotificationOccurrences(t, chattoCore, rootAuthor.Id); len(occurrences) != 0 {
		t.Fatalf("Badge occurrences = %+v, want none", occurrences)
	}
	if unread, err := chattoCore.notificationOccurrences.HasNotificationUnread(ctx, rootAuthor.Id, room.Id, root.Id); err != nil || !unread {
		t.Fatalf("thread Badge unread = (%v, %v), want (true, nil)", unread, err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, rootAuthor.Id, room.Id); err != nil || !unread {
		t.Fatalf("parent room Badge unread = (%v, %v), want (true, nil)", unread, err)
	}
	if _, err := chattoCore.ReadState().MarkThreadAsRead(ctx, rootAuthor.Id, room.Id, root.Id, reply.Id); err != nil {
		t.Fatalf("mark thread read: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, rootAuthor.Id, room.Id); err != nil || unread {
		t.Fatalf("parent room unread after thread read = (%v, %v), want (false, nil)", unread, err)
	}
}

func TestMessageMentionFactsRecomputeAfterOCCConflict(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "notify-retry-author", "Notify Retry Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	lateMember, err := chattoCore.CreateUser(ctx, SystemActorID, "notify-retry-member", "Notify Retry Member", "password")
	if err != nil {
		t.Fatalf("CreateUser late member: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "notification-retry-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, lateMember.Id, notificationTestSignalAll, evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}

	preparedAttempts := 0
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@all retry recipients", nil, "", "", nil, false,
		withPostMessageAttemptPrepared(func(attemptCtx context.Context) error {
			preparedAttempts++
			if preparedAttempts != 1 {
				return nil
			}
			_, err := chattoCore.JoinRoom(attemptCtx, lateMember.Id, KindChannel, lateMember.Id, room.Id)
			return err
		}),
	)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if preparedAttempts < 2 || !slices.Contains(posted.GetMessagePosted().GetMentionedUserIds(), lateMember.Id) {
		t.Fatalf("retry attempts/recipients = (%d, %v)", preparedAttempts, posted.GetMessagePosted().GetMentionedUserIds())
	}
	foundLateMember := false
	for _, mention := range posted.GetMessagePosted().GetMentions() {
		if mention.GetUserId() == lateMember.Id && mention.GetAll() != nil {
			foundLateMember = true
		}
	}
	if !foundLateMember {
		t.Fatalf("resolved mention facts = %+v, want @all fact for late member", posted.GetMessagePosted().GetMentions())
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, lateMember.Id)
	if len(occurrences) != 1 || notificationTestSignalKind(notificationSignalIdentity(occurrences[0].GetSignal())) != notificationTestSignalAll {
		t.Fatalf("late member occurrences = %+v", occurrences)
	}
}

func TestOneSourceFactProducesIndependentSignalsPerCause(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "signal-author", "Signal Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "signal-recipient", "Signal Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "signal-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(ctx, recipient.Id, room.Id, notificationTestSignalRoomMessage, evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION); err != nil {
		t.Fatal(err)
	}
	// The deprecated followed-room compatibility slot remains inert.
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(ctx, recipient.Id, room.Id, notificationTestSignalFollowedRoom, evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION); err != nil {
		t.Fatal(err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@all @signal-recipient three causes", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id)
	if len(occurrences) != 3 {
		t.Fatalf("occurrences = %+v, want three exact signals", occurrences)
	}
	kinds := []notificationTestSignalKind{
		notificationTestSignalKind(notificationSignalIdentity(occurrences[0].GetSignal())),
		notificationTestSignalKind(notificationSignalIdentity(occurrences[1].GetSignal())),
		notificationTestSignalKind(notificationSignalIdentity(occurrences[2].GetSignal())),
	}
	slices.Sort(kinds)
	want := []notificationTestSignalKind{
		notificationTestSignalAll,
		notificationTestSignalDirectMention,
		notificationTestSignalRoomMessage,
	}
	if !slices.Equal(kinds, want) || occurrences[0].GetSourceEventId() != posted.GetId() || occurrences[0].GetId() == occurrences[1].GetId() {
		t.Fatalf("signal kinds/identities = (%v, %+v)", kinds, occurrences)
	}
}

func TestRoomMessageOutputHonoursMessageReadVisibilityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		slug string
		mode evtv1.NotificationDeliveryMode
	}{
		{name: "Badge", slug: "badge", mode: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE},
		{name: "Notification", slug: "notification", mode: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chattoCore, _ := setupTestCore(t)
			ctx := testContext(t)
			author, err := chattoCore.CreateUser(ctx, SystemActorID, "read-author-"+tc.slug, "Read Author "+tc.name, "password")
			if err != nil {
				t.Fatal(err)
			}
			recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "read-recipient-"+tc.slug, "Read Recipient "+tc.name, "password")
			if err != nil {
				t.Fatal(err)
			}
			room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "read-room-"+tc.slug, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, userID := range []string{author.Id, recipient.Id} {
				if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
				ctx, recipient.Id, room.Id, notificationTestSignalRoomMessage, tc.mode,
			); err != nil {
				t.Fatal(err)
			}

			if err := chattoCore.DenyUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermMessageRead); err != nil {
				t.Fatal(err)
			}
			if _, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "hidden before source", nil, "", "", nil, false); err != nil {
				t.Fatal(err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatal(err)
			}
			if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || unread {
				t.Fatalf("Badge while message.read denied at source = (%v, %v), want (false, nil)", unread, err)
			}
			if occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id); len(occurrences) != 0 {
				t.Fatalf("occurrences while message.read denied at source = %+v, want none", occurrences)
			}

			if err := chattoCore.GrantUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermMessageRead); err != nil {
				t.Fatal(err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatal(err)
			}
			if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || unread {
				t.Fatalf("hidden Badge after permission regain = (%v, %v), want (false, nil)", unread, err)
			}
			if occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id); len(occurrences) != 0 {
				t.Fatalf("hidden occurrences after permission regain = %+v, want none", occurrences)
			}

			if _, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "visible source", nil, "", "", nil, false); err != nil {
				t.Fatal(err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatal(err)
			}
			if tc.mode == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
				if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || !unread {
					t.Fatalf("visible Badge = (%v, %v), want (true, nil)", unread, err)
				}
			} else if occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id); len(occurrences) != 1 {
				t.Fatalf("visible occurrences = %+v, want one", occurrences)
			}

			if err := chattoCore.DenyUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermMessageRead); err != nil {
				t.Fatal(err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatal(err)
			}
			if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || unread {
				t.Fatalf("Badge after message.read loss = (%v, %v), want (false, nil)", unread, err)
			}
			if occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id); len(occurrences) != 0 {
				t.Fatalf("occurrences after message.read loss = %+v, want none", occurrences)
			}

			if err := chattoCore.GrantUserRoomPermission(ctx, SystemActorID, room.Id, recipient.Id, PermMessageRead); err != nil {
				t.Fatal(err)
			}
			if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
				t.Fatal(err)
			}
			if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || unread {
				t.Fatalf("old Badge after message.read regain = (%v, %v), want (false, nil)", unread, err)
			}
			if occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id); len(occurrences) != 0 {
				t.Fatalf("old occurrences after message.read regain = %+v, want none", occurrences)
			}
		})
	}
}

func TestDirectMentionOccurrenceVisibleWithInteractionScopedRead(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "interaction-notify-author", "Interaction Notify Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "interaction-notify-recipient", "Interaction Notify Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.GetId(), KindChannel, "", "interaction-notify-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.GetId(), recipient.GetId()} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	root, err := chattoCore.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "notification root", nil, "", "", nil, true)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	if err := chattoCore.DenyRoomPermission(ctx, SystemActorID, room.GetId(), RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission message.read: %v", err)
	}
	if err := chattoCore.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), recipient.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}
	mention, err := chattoCore.PostMessage(
		ctx, KindChannel, room.GetId(), author.GetId(), "@interaction-notify-recipient please review", nil, root.GetId(), "", nil, false,
	)
	if err != nil {
		t.Fatalf("PostMessage mention: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, recipient.GetId())
	if len(occurrences) != 1 || occurrences[0].GetSourceEventId() != mention.GetId() || !testOccurrenceHasKind(occurrences[0], notificationTestSignalDirectMention) {
		t.Fatalf("interaction mention occurrences = %+v, want exact direct mention", occurrences)
	}
	visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, recipient.GetId(), occurrences)
	if err != nil {
		t.Fatalf("VisibleOccurrences: %v", err)
	}
	if len(visible) != 1 || visible[0].GetId() != occurrences[0].GetId() {
		t.Fatalf("visible interaction occurrences = %+v, want mention occurrence", visible)
	}
	if _, err := chattoCore.CreateServerRole(ctx, SystemActorID, "interaction-observer", "Interaction observer", "Unassigned visibility test role"); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := chattoCore.GrantServerPermission(ctx, SystemActorID, "interaction-observer", PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantServerPermission message.read-interactions: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent after unrelated permission change: %v", err)
	}
	occurrences = testNotificationOccurrences(t, chattoCore, recipient.GetId())
	if len(occurrences) != 1 || occurrences[0].GetId() != visible[0].GetId() {
		t.Fatalf("interaction occurrence after unrelated permission change = %+v, want retained occurrence %s", occurrences, visible[0].GetId())
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx,
		recipient.GetId(),
		room.GetId(),
		notificationTestSignalDirectMention,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	); err != nil {
		t.Fatalf("set direct-mention Badge policy: %v", err)
	}
	if _, err := chattoCore.PostMessage(
		ctx, KindChannel, room.GetId(), author.GetId(), "@interaction-notify-recipient Badge review", nil, root.GetId(), "", nil, false,
	); err != nil {
		t.Fatalf("PostMessage Badge mention: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent after Badge mention: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.GetId(), room.GetId()); err != nil || !unread {
		t.Fatalf("interaction Badge before unrelated permission change = (%v, %v), want (true, nil)", unread, err)
	}
	if _, err := chattoCore.CreateServerRole(ctx, SystemActorID, "interaction-auditor", "Interaction auditor", "Second unassigned visibility test role"); err != nil {
		t.Fatalf("CreateServerRole second role: %v", err)
	}
	if err := chattoCore.GrantServerPermission(ctx, SystemActorID, "interaction-auditor", PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantServerPermission second message.read-interactions: %v", err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("WaitCurrent after second unrelated permission change: %v", err)
	}
	if unread, err := chattoCore.HasUnread(ctx, KindChannel, recipient.GetId(), room.GetId()); err != nil || !unread {
		t.Fatalf("interaction Badge after unrelated permission change = (%v, %v), want (true, nil)", unread, err)
	}
}

func TestDirectMessagesRemainExactOccurrences(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "exact-dm-alice", "Exact DM Alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "exact-dm-bob", "Exact DM Bob", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatal(err)
	}
	first, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "second", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, alice.Id)
	if len(occurrences) != 2 || occurrences[0].GetSourceEventId() != second.GetId() || occurrences[1].GetSourceEventId() != first.GetId() {
		t.Fatalf("DM occurrences = %+v, want one per message", occurrences)
	}
}
