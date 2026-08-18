package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationOccurrenceLifecycleUsesStreamFacts(t *testing.T) {
	chattoCore, _ := newTestCore(t)
	chattoCore.SetNotificationAlertHandler(func(context.Context, *corev1.NotificationOccurrence) error {
		return errors.New("hold alert pending for lifecycle assertions")
	})
	startCoreServices(t, chattoCore)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC().Truncate(time.Millisecond)
	input := CreateNotificationOccurrenceInput{
		RecipientID:   "U-notification-recipient",
		SourceEventID: "E-notification-source",
		SourceCreated: now,
		ActorID:       "U-notification-actor",
		Signal: testNotificationSignal(
			corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION,
			"R-notification-room",
			"E-notification-source",
		),
		Mode:           corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_ALERT,
		AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	}

	created, wasCreated, err := model.Create(ctx, input)
	if err != nil || !wasCreated || created == nil {
		t.Fatalf("Create = (%+v, %v, %v), want new occurrence", created, wasCreated, err)
	}
	wantID := notificationOccurrenceID(input.RecipientID, input.SourceEventID, "direct_mention_received")
	if created.GetId() != wantID || created.GetNotificationStreamSequence() == 0 {
		t.Fatalf("created occurrence = %+v, want deterministic ID and stream sequence", created)
	}
	if created.GetAlertExpiresAt() == nil || !NotificationAlertPending(created) {
		t.Fatalf("created delivery state = %+v, want pending alert", created)
	}
	storedSignal, err := chattoCore.storage.notificationStream.GetMsg(ctx, created.GetNotificationStreamSequence())
	if err != nil {
		t.Fatalf("read stored signal: %v", err)
	}
	var signalEvent corev1.NotificationEvent
	if err := proto.Unmarshal(storedSignal.Data, &signalEvent); err != nil {
		t.Fatalf("decode stored signal: %v", err)
	}
	if got := signalEvent.GetSignalled(); signalEvent.GetNotificationId() != wantID || got.GetSourceEventId() != input.SourceEventID || got.GetSignal() == nil {
		t.Fatalf("stored immutable signal = %+v", got)
	}
	duplicate, wasCreated, err := model.Create(ctx, input)
	if err != nil || wasCreated || duplicate.GetId() != wantID {
		t.Fatalf("duplicate Create = (%+v, %v, %v)", duplicate, wasCreated, err)
	}
	read, err := model.MarkRead(ctx, input.RecipientID, wantID)
	if err != nil || !read.GetRead() || read.AlertDelivered == nil || read.GetAlertDelivered() {
		t.Fatalf("MarkRead = (%+v, %v)", read, err)
	}
	deleted, err := model.Delete(ctx, input.RecipientID, wantID)
	if err != nil || !deleted {
		t.Fatalf("Delete = (%v, %v)", deleted, err)
	}
	if _, err := model.Get(ctx, input.RecipientID, wantID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want not found", err)
	}
	if _, err := chattoCore.storage.notificationStream.GetMsg(ctx, created.GetNotificationStreamSequence()); !notificationSignalAlreadyAbsent(err) {
		t.Fatalf("rich signal after delete = %v, want securely deleted", err)
	}
	if recreated, wasCreated, err := model.Create(ctx, input); err != nil || wasCreated || recreated != nil {
		t.Fatalf("Create after tombstone = (%+v, %v, %v), want suppressed", recreated, wasCreated, err)
	}
	model.cleanupDismissedSignals(ctx, input.SourceCreated.Add(notificationTTL+time.Second))
	model.cleanedMu.Lock()
	cleanedCount := len(model.cleaned)
	model.cleanedMu.Unlock()
	if cleanedCount != 0 {
		t.Fatalf("expired secure-delete results = %d, want none", cleanedCount)
	}
}

func TestNotificationIdentitySeparatesSignalKinds(t *testing.T) {
	recipientID, sourceID := "U1", "E1"
	mention := notificationOccurrenceID(recipientID, sourceID, "direct_mention_received")
	reply := notificationOccurrenceID(recipientID, sourceID, "reply_received")
	if mention == reply {
		t.Fatalf("different signal kinds shared ID %q", mention)
	}
}

func TestNotificationProjectionExpiresOccurrencesAndTombstones(t *testing.T) {
	p := NewNotificationProjection()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.now = func() time.Time { return now }
	occurrence := &corev1.NotificationOccurrence{
		Id:              "N1",
		RecipientId:     "U1",
		SourceEventId:   "E1",
		SourceCreatedAt: timestamp(now.Add(-time.Hour)),
		Signal:          testNotificationSignal(corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_REPLY, "R1", "E1"),
		ExpiresAt:       timestamp(now.Add(time.Minute)),
	}
	if err := p.Apply(notificationSignalledEvent("NE1", occurrence, now.Add(time.Minute)), 7); err != nil {
		t.Fatalf("Apply signal: %v", err)
	}
	if got, ok := p.occurrence("U1", "N1", now); !ok || got.GetNotificationStreamSequence() != 7 {
		t.Fatalf("projected occurrence = (%+v, %v)", got, ok)
	}
	if err := p.Apply(&corev1.NotificationEvent{
		Id: "NE2", RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamp(now), ExpiresAt: timestamp(now.Add(time.Minute)),
		Event: &corev1.NotificationEvent_Removed{Removed: &corev1.NotificationRemoved{}},
	}, 8); err != nil {
		t.Fatalf("Apply dismissal: %v", err)
	}
	if _, ok := p.occurrence("U1", "N1", now); ok {
		t.Fatal("dismissed occurrence remained visible")
	}
	if got := p.pendingPhysicalDeletes(now)["N1"].signalSequence; got != 7 {
		t.Fatalf("pending physical delete sequence = %d, want 7", got)
	}
	now = now.Add(2 * time.Minute)
	if got := p.pendingPhysicalDeletes(now); len(got) != 0 {
		t.Fatalf("expired tombstones = %+v, want none", got)
	}
}

func TestNotificationProjectionKeepsFirstAlertResolution(t *testing.T) {
	p := NewNotificationProjection()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p.now = func() time.Time { return now }
	occurrence := &corev1.NotificationOccurrence{
		Id: "N1", RecipientId: "U1", SourceEventId: "E1", SourceCreatedAt: timestamp(now),
		Signal:    testNotificationSignal(corev1.NotificationPreferenceCategory_NOTIFICATION_PREFERENCE_CATEGORY_DIRECT_MENTION, "R1", "E1"),
		ExpiresAt: timestamp(now.Add(time.Hour)), AlertExpiresAt: timestamp(now.Add(time.Minute)),
	}
	if err := p.Apply(notificationSignalledEvent("signal", occurrence, now.Add(time.Hour)), 1); err != nil {
		t.Fatal(err)
	}
	resolved := func(id string, delivered bool) *corev1.NotificationEvent {
		return &corev1.NotificationEvent{
			Id: id, RecipientId: "U1", NotificationId: "N1", OccurredAt: timestamp(now), ExpiresAt: timestamp(now.Add(time.Hour)),
			Event: &corev1.NotificationEvent_AlertResolved{AlertResolved: &corev1.NotificationAlertResolved{Delivered: delivered}},
		}
	}
	if err := p.Apply(resolved("first", true), 2); err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(resolved("late-contradiction", false), 3); err != nil {
		t.Fatal(err)
	}
	got, ok := p.occurrence("U1", "N1", now)
	if !ok || got.AlertDelivered == nil || !got.GetAlertDelivered() {
		t.Fatalf("terminal alert state = %+v, want first Delivered outcome", got)
	}
}

func notificationSignalledEvent(id string, occurrence *corev1.NotificationOccurrence, expires time.Time) *corev1.NotificationEvent {
	return &corev1.NotificationEvent{
		Id: id, RecipientId: occurrence.GetRecipientId(), NotificationId: occurrence.GetId(), OccurredAt: occurrence.GetSourceCreatedAt(), ExpiresAt: timestamp(expires),
		Event: &corev1.NotificationEvent_Signalled{Signalled: &corev1.NotificationSignalled{
			SourceEventId:        occurrence.GetSourceEventId(),
			SourceCreatedAt:      occurrence.GetSourceCreatedAt(),
			ActorId:              occurrence.GetActorId(),
			Signal:               occurrence.GetSignal(),
			InitiallyRead:        occurrence.GetRead(),
			SourceStreamSequence: occurrence.GetSourceStreamSequence(),
			AttentionLevel:       occurrence.GetAttentionLevel(),
			AlertExpiresAt:       occurrence.GetAlertExpiresAt(),
		}},
	}
}

func TestUnsupportedNotificationSignalDetection(t *testing.T) {
	if !NotificationOccurrenceHasUnsupportedSignal(&corev1.NotificationOccurrence{Signal: testUnsupportedNotificationSignal()}) {
		t.Fatal("future signal was not detected")
	}
	if NotificationOccurrenceHasUnsupportedSignal(&corev1.NotificationOccurrence{Signal: &corev1.NotificationSignal{}}) {
		t.Fatal("empty signal was treated as an unknown future signal")
	}
}
