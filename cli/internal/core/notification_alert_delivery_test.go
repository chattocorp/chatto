package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"hmans.de/chatto/internal/notificationstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationStreamAndAlertConsumerConfiguration(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	info, err := chattoCore.storage.notificationStream.Info(ctx)
	if err != nil {
		t.Fatalf("notification stream info: %v", err)
	}
	if info.Config.Name != notificationstream.StreamName || info.Config.Storage != jetstream.FileStorage ||
		info.Config.MaxAge != notificationTTL+notificationPhysicalCleanupGrace || !info.Config.AllowMsgTTL {
		t.Fatalf("notification stream config = %+v", info.Config)
	}
	consumer, err := chattoCore.storage.notificationStream.Consumer(ctx, notificationAlertConsumerName)
	if err != nil {
		t.Fatalf("notification alert consumer: %v", err)
	}
	consumerInfo, err := consumer.Info(ctx)
	if err != nil || consumerInfo.Config.FilterSubject != notificationstream.SignalledSubject || consumerInfo.Config.AckPolicy != jetstream.AckExplicitPolicy {
		t.Fatalf("notification consumer info = (%+v, %v)", consumerInfo, err)
	}
}

func TestNotificationAlertEligibleRejectsUnsupportedFutureSignal(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	eligible, err := chattoCore.NotificationAlertEligible(testContext(t), &corev1.NotificationOccurrence{Signal: testUnsupportedNotificationSignal()})
	if eligible || !errors.Is(err, ErrUnsupportedNotificationSignal) {
		t.Fatalf("NotificationAlertEligible = (%v, %v), want false and unsupported-signal error", eligible, err)
	}
}

func TestNotificationSoundEligibleAllowsInAppNotificationWithoutPush(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "sound-recipient", "Sound Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "sound-author", "Sound Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "sound-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, recipient.Id,
		notificationTestSignalDirectMention,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "hello @sound-recipient", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	waitForNotificationMaterializer(t, chattoCore)
	wantID := notificationOccurrenceID(recipient.Id, posted.GetId(), "direct_mention_received")
	occurrence, err := chattoCore.NotificationOccurrences().Get(ctx, recipient.Id, wantID)
	if err != nil {
		t.Fatalf("Get occurrence: %v", err)
	}
	soundEligible, err := chattoCore.NotificationSoundEligible(ctx, occurrence)
	if err != nil || !soundEligible {
		t.Fatalf("NotificationSoundEligible = (%v, %v), want true, nil", soundEligible, err)
	}
	pushEligible, err := chattoCore.NotificationAlertEligible(ctx, occurrence)
	if err != nil || pushEligible {
		t.Fatalf("NotificationAlertEligible = (%v, %v), want false, nil", pushEligible, err)
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, recipient.Id,
		notificationTestSignalDirectMention,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable notification policy: %v", err)
	}
	soundEligible, err = chattoCore.NotificationSoundEligible(ctx, occurrence)
	if err != nil || soundEligible {
		t.Fatalf("NotificationSoundEligible after Off = (%v, %v), want false, nil", soundEligible, err)
	}
}

func TestNotificationAlertWorkerConsumesSignalledEvent(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	delivered := make(chan *corev1.NotificationOccurrence, 1)
	chattoCore.SetNotificationAlertHandler(func(_ context.Context, occurrence *corev1.NotificationOccurrence) error {
		delivered <- occurrence
		return nil
	})
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	alice, err := chattoCore.CreateUser(ctx, SystemActorID, "stream-alert-alice", "Stream Alert Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := chattoCore.CreateUser(ctx, SystemActorID, "stream-alert-bob", "Stream Alert Bob", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, _, err := chattoCore.FindOrCreateDM(ctx, alice.Id, []string{bob.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindDM, room.Id, bob.Id, "stream alert", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	select {
	case occurrence := <-delivered:
		if occurrence.GetRecipientId() != alice.Id || occurrence.GetSourceEventId() != posted.GetId() {
			t.Fatalf("delivered occurrence = %+v", occurrence)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification alert was not delivered from NOTIFICATIONS")
	}
	wantID := notificationOccurrenceID(alice.Id, posted.GetId(), "direct_message_received")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		occurrence, getErr := chattoCore.NotificationOccurrences().Get(ctx, alice.Id, wantID)
		if getErr == nil && occurrence.AlertDelivered != nil && occurrence.GetAlertDelivered() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("notification alert did not reach terminal delivered state")
}

func TestNotificationAlertWorkerUsesRoomGroupPolicy(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	delivered := make(chan *corev1.NotificationOccurrence, 1)
	chattoCore.SetNotificationAlertHandler(func(_ context.Context, occurrence *corev1.NotificationOccurrence) error {
		delivered <- occurrence
		return nil
	})
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	recipient, err := chattoCore.CreateUser(ctx, SystemActorID, "group-alert-recipient", "Group Alert Recipient", "password")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "group-alert-author", "Group Alert Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "Group Alerts", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "group-alert-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{recipient.Id, author.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetServerNotificationMode(ctx, recipient.Id,
		notificationTestSignalDirectMention,
		corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("SetServerNotificationMode: %v", err)
	}
	if _, err := chattoCore.NotificationPolicy().UpdateScopedNotificationPolicy(ctx, recipient.Id,
		NotificationPolicyScope{Kind: NotificationPolicyScopeRoomGroup, ID: group.Id},
		&corev1.NotificationDeliveryModes{DirectMentions: corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()},
		&fieldmaskpb.FieldMask{Paths: []string{"direct_mentions"}},
	); err != nil {
		t.Fatalf("UpdateScopedNotificationPolicy: %v", err)
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "@group-alert-recipient hello", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	select {
	case occurrence := <-delivered:
		if occurrence.GetRecipientId() != recipient.Id || occurrence.GetSourceEventId() != posted.GetId() {
			t.Fatalf("delivered group-policy occurrence = %+v", occurrence)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("group-policy alert was not delivered")
	}
}
