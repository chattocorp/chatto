package core

import (
	"context"
	"slices"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

	unknown := corev1.NotificationDeliveryMode(99)
	policyChanged := newEvent(recipient.Id, &corev1.Event{Event: &corev1.Event_UserNotificationPolicyChanged{
		UserNotificationPolicyChanged: &corev1.UserNotificationPolicyChangedEvent{
			UserId: recipient.Id,
			Overrides: &corev1.NotificationDeliveryModes{
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
		&corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
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
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, lateMember.Id, notificationTestSignalAll, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION); err != nil {
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
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(ctx, recipient.Id, room.Id, notificationTestSignalFollowedRoom, corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION); err != nil {
		t.Fatal(err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@signal-recipient two causes", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := chattoCore.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	occurrences := testNotificationOccurrences(t, chattoCore, recipient.Id)
	if len(occurrences) != 2 {
		t.Fatalf("occurrences = %+v, want two exact signals", occurrences)
	}
	kinds := []notificationTestSignalKind{
		notificationTestSignalKind(notificationSignalIdentity(occurrences[0].GetSignal())),
		notificationTestSignalKind(notificationSignalIdentity(occurrences[1].GetSignal())),
	}
	slices.Sort(kinds)
	want := []notificationTestSignalKind{
		notificationTestSignalDirectMention,
		notificationTestSignalFollowedRoom,
	}
	if !slices.Equal(kinds, want) || occurrences[0].GetSourceEventId() != posted.GetId() || occurrences[0].GetId() == occurrences[1].GetId() {
		t.Fatalf("signal kinds/identities = (%v, %+v)", kinds, occurrences)
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
		t.Fatalf("GrantUserRoomPermission message.read.interactions: %v", err)
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
