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
			corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION,
			"R-notification-room",
			"E-notification-source",
		),
		Intensity:      corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		SkipReadLookup: true,
	}

	created, wasCreated, err := model.Create(ctx, input)
	if err != nil || !wasCreated || created == nil {
		t.Fatalf("Create = (%+v, %v, %v), want new occurrence", created, wasCreated, err)
	}
	wantID := notificationOccurrenceID(input.RecipientID, input.SourceEventID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION)
	if created.GetId() != wantID || created.GetNotificationStreamSequence() == 0 {
		t.Fatalf("created occurrence = %+v, want deterministic ID and stream sequence", created)
	}
	if created.GetIntensity() != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT ||
		created.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
		t.Fatalf("created delivery state = (%v, %v)", created.GetIntensity(), created.GetAlertState())
	}
	storedSignal, err := chattoCore.storage.notificationStream.GetMsg(ctx, created.GetNotificationStreamSequence())
	if err != nil {
		t.Fatalf("read stored signal: %v", err)
	}
	var signalEvent corev1.NotificationEvent
	if err := proto.Unmarshal(storedSignal.Data, &signalEvent); err != nil {
		t.Fatalf("decode stored signal: %v", err)
	}
	if got := signalEvent.GetSignalled(); got.GetNotificationId() != wantID || got.GetSourceEventId() != input.SourceEventID || got.GetSignal() == nil {
		t.Fatalf("stored immutable signal = %+v", got)
	}
	duplicate, wasCreated, err := model.Create(ctx, input)
	if err != nil || wasCreated || duplicate.GetId() != wantID {
		t.Fatalf("duplicate Create = (%+v, %v, %v)", duplicate, wasCreated, err)
	}
	read, err := model.MarkRead(ctx, input.RecipientID, wantID)
	if err != nil || read.GetInboxState() != corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ ||
		read.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("MarkRead = (%+v, %v)", read, err)
	}
	deleted, err := model.Delete(ctx, input.RecipientID, wantID, corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED)
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
	mention := notificationOccurrenceID(recipientID, sourceID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION)
	reply := notificationOccurrenceID(recipientID, sourceID, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY)
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
		Signal:          testNotificationSignal(corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY, "R1", "E1"),
		ExpiresAt:       timestamp(now.Add(time.Minute)),
	}
	if err := p.Apply(notificationSignalledEvent("NE1", occurrence, now.Add(time.Minute)), 7); err != nil {
		t.Fatalf("Apply signal: %v", err)
	}
	if got, ok := p.occurrence("U1", "N1", now); !ok || got.GetNotificationStreamSequence() != 7 {
		t.Fatalf("projected occurrence = (%+v, %v)", got, ok)
	}
	if err := p.Apply(&corev1.NotificationEvent{
		Id: "NE2", RecipientId: "U1", OccurredAt: timestamp(now), ExpiresAt: timestamp(now.Add(time.Minute)),
		Event: &corev1.NotificationEvent_Dismissed{Dismissed: &corev1.NotificationDismissed{NotificationId: "N1", SignalStreamSequence: 7}},
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

func notificationSignalledEvent(id string, occurrence *corev1.NotificationOccurrence, expires time.Time) *corev1.NotificationEvent {
	return &corev1.NotificationEvent{
		Id: id, RecipientId: occurrence.GetRecipientId(), OccurredAt: occurrence.GetSourceCreatedAt(), ExpiresAt: timestamp(expires),
		Event: &corev1.NotificationEvent_Signalled{Signalled: &corev1.NotificationSignalled{
			NotificationId:       occurrence.GetId(),
			SourceEventId:        occurrence.GetSourceEventId(),
			SourceCreatedAt:      occurrence.GetSourceCreatedAt(),
			ActorId:              occurrence.GetActorId(),
			Signal:               occurrence.GetSignal(),
			Intensity:            occurrence.GetIntensity(),
			InitialInboxState:    corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD,
			EvaluatedAt:          occurrence.GetSourceCreatedAt(),
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
