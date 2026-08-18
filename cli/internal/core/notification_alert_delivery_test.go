package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

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
