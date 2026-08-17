package core

import (
	"context"
	"slices"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationIntensity(ctx, lateMember.Id, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL, corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE); err != nil {
		t.Fatalf("SetServerNotificationIntensity: %v", err)
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
		if mention.GetUserId() == lateMember.Id && mention.GetKind() == corev1.MessageMentionKind_MESSAGE_MENTION_KIND_ALL {
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
	if len(occurrences) != 1 || notificationSignalPolicyKind(occurrences[0].GetSignal()) != corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_ALL {
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
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationIntensity(ctx, recipient.Id, room.Id, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM, corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE); err != nil {
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
	kinds := []corev1.NotificationPolicyKind{
		notificationSignalPolicyKind(occurrences[0].GetSignal()),
		notificationSignalPolicyKind(occurrences[1].GetSignal()),
	}
	slices.Sort(kinds)
	want := []corev1.NotificationPolicyKind{
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION,
		corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM,
	}
	if !slices.Equal(kinds, want) || occurrences[0].GetSourceEventId() != posted.GetId() || occurrences[0].GetId() == occurrences[1].GetId() {
		t.Fatalf("signal kinds/identities = (%v, %+v)", kinds, occurrences)
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
