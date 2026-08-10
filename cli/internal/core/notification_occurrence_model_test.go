package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationOccurrenceLifecycleAndDeterministicIdentity(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC().Truncate(time.Millisecond)
	model.now = func() time.Time { return now }

	input := CreateNotificationOccurrenceInput{
		RecipientID:   "U-notification-recipient",
		SourceEventID: "E-notification-source",
		SourceCreated: now.Add(-24 * time.Hour),
		ActorID:       "U-notification-actor",
		Target: &corev1.NotificationTarget{
			RoomId:  "R-notification-room",
			EventId: "E-notification-source",
		},
		Reasons: []*corev1.NotificationReasonMatch{
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			},
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
			},
			{
				Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
				Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
			},
		},
		SkipReadLookup: true,
	}

	created, wasCreated, err := model.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !wasCreated || created == nil {
		t.Fatalf("Create = (%v, %v), want a new occurrence", created, wasCreated)
	}
	if created.GetId() != notificationOccurrenceID(input.RecipientID, input.SourceEventID) {
		t.Fatalf("id = %q, want deterministic identity", created.GetId())
	}
	if got := created.GetStrongestIntensity(); got != corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
		t.Fatalf("strongest intensity = %v, want ALERT", got)
	}
	if got := len(created.GetReasons()); got != 2 {
		t.Fatalf("reasons = %d, want two deduplicated reasons", got)
	}
	originalExpiry := created.GetExpiresAt().AsTime()

	claim, claimed, err := model.ClaimPendingAlert(ctx)
	if err != nil || !claimed || claim.GetId() != created.GetId() {
		t.Fatalf("ClaimPendingAlert = (%v, %v, %v), want created occurrence", claim, claimed, err)
	}
	renewed, renewedClaim, err := model.RenewAlertClaim(ctx, claim)
	if err != nil || !renewedClaim || !renewed.GetAlertClaimedUntil().AsTime().After(claim.GetAlertClaimedUntil().AsTime()) {
		t.Fatalf("RenewAlertClaim = (%v, %v, %v), want extended exact claim", renewed, renewedClaim, err)
	}
	claim = renewed
	if err := model.CompleteAlertClaim(ctx, claim, false); err != nil {
		t.Fatalf("CompleteAlertClaim failed delivery: %v", err)
	}
	if retry, retryClaimed, err := model.ClaimPendingAlert(ctx); err != nil || retryClaimed || retry != nil {
		t.Fatalf("immediate retry ClaimPendingAlert = (%v, %v, %v), want paced retry", retry, retryClaimed, err)
	}
	now = now.Add(notificationAlertRetryDelay + time.Millisecond)
	claim, claimed, err = model.ClaimPendingAlert(ctx)
	if err != nil || !claimed {
		t.Fatalf("retry ClaimPendingAlert = (%v, %v, %v), want retry", claim, claimed, err)
	}
	if err := model.CompleteAlertClaim(ctx, claim, true); err != nil {
		t.Fatalf("CompleteAlertClaim delivered: %v", err)
	}
	if claim, claimed, err := model.ClaimPendingAlert(ctx); err != nil || claimed || claim != nil {
		t.Fatalf("ClaimPendingAlert after delivery = (%v, %v, %v), want none", claim, claimed, err)
	}

	duplicate, wasCreated, err := model.Create(ctx, input)
	if err != nil {
		t.Fatalf("duplicate Create: %v", err)
	}
	if wasCreated || duplicate.GetId() != created.GetId() {
		t.Fatalf("duplicate Create = (%v, %v), want existing occurrence", duplicate, wasCreated)
	}

	read := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
	saved := true
	updated, err := model.Update(ctx, input.RecipientID, created.GetId(), UpdateNotificationOccurrenceInput{
		InboxState: &read,
		Saved:      &saved,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.GetInboxState() != read || !updated.GetSaved() {
		t.Fatalf("Update = %+v, want read and saved", updated)
	}
	if !updated.GetExpiresAt().AsTime().Equal(originalExpiry) {
		t.Fatalf("expiry changed from %v to %v", originalExpiry, updated.GetExpiresAt().AsTime())
	}

	inboxGroups, err := model.Groups(ctx, input.RecipientID, NotificationOccurrenceViewInbox)
	if err != nil || len(inboxGroups) != 1 {
		t.Fatalf("Inbox groups = (%v, %v), want one", inboxGroups, err)
	}
	done := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE
	if _, err := model.UpdateGroup(ctx, input.RecipientID, inboxGroups[0].ID, NotificationOccurrenceViewInbox, UpdateNotificationOccurrenceInput{InboxState: &done}); err != nil {
		t.Fatalf("UpdateGroup to Done: %v", err)
	}
	if groups, err := model.Groups(ctx, input.RecipientID, NotificationOccurrenceViewInbox); err != nil || len(groups) != 0 {
		t.Fatalf("Inbox groups after Done = (%v, %v), want empty", groups, err)
	}
	if groups, err := model.Groups(ctx, input.RecipientID, NotificationOccurrenceViewDone); err != nil || len(groups) != 1 {
		t.Fatalf("Done groups = (%v, %v), want one", groups, err)
	}
	if groups, err := model.Groups(ctx, input.RecipientID, NotificationOccurrenceViewSaved); err != nil || len(groups) != 1 {
		t.Fatalf("Saved groups = (%v, %v), want one", groups, err)
	}

	deleted, err := model.Delete(ctx, input.RecipientID, created.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED)
	if err != nil || !deleted {
		t.Fatalf("Delete = (%v, %v), want true", deleted, err)
	}
	if _, err := model.Get(ctx, input.RecipientID, created.GetId()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get deleted occurrence = %v, want ErrNotFound", err)
	}
	if recreated, wasCreated, err := model.Create(ctx, input); err != nil || wasCreated || recreated != nil {
		t.Fatalf("Create after tombstone = (%v, %v, %v), want nil, false, nil", recreated, wasCreated, err)
	}
}

func TestNotificationOccurrenceReadCancelsPendingAlert(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	model := chattoCore.NotificationOccurrences()
	now := time.Now().UTC()
	model.now = func() time.Time { return now }
	created, _, err := model.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-read-alert-recipient",
		SourceEventID: "E-read-alert-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-read-alert", EventId: "E-read-alert-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	read := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ
	updated, err := model.Update(ctx, created.GetRecipientId(), created.GetId(), UpdateNotificationOccurrenceInput{InboxState: &read})
	if err != nil {
		t.Fatalf("Update read: %v", err)
	}
	if updated.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED {
		t.Fatalf("alert state = %v, want SILENCED", updated.GetAlertState())
	}
	if claim, claimed, err := model.ClaimPendingAlert(ctx); err != nil || claimed || claim != nil {
		t.Fatalf("ClaimPendingAlert after read = (%v, %v, %v), want none", claim, claimed, err)
	}
}

func TestNotificationOccurrenceIndexConvergesAcrossReplicas(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	second := NewNotificationOccurrenceModel(chattoCore, chattoCore.storage.runtimeStateKV, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- second.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("second notification index did not stop")
		}
	})
	if err := second.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	sourceTime := time.Now().UTC()
	created, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-replica-recipient",
		SourceEventID: "E-replica-source",
		SourceCreated: sourceTime,
		Target: &corev1.NotificationTarget{
			RoomId:  "R-replica-room",
			EventId: "E-replica-source",
		},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	entry, err := chattoCore.storage.runtimeStateKV.Get(ctx, notificationOccurrenceKey("U-replica-recipient", "E-replica-source"))
	if err != nil {
		t.Fatalf("Get KV entry: %v", err)
	}
	if err := second.WaitForSourceRevision(ctx, created.GetRecipientId(), created.GetSourceEventId(), entry.Revision()); err != nil {
		t.Fatalf("wait for source revision on second model: %v", err)
	}
	got, exists, err := second.index.occurrenceByID(ctx, "U-replica-recipient", created.GetId())
	if err != nil || !exists || got.occurrence.GetId() != created.GetId() {
		t.Fatalf("second index occurrence = (%v, %v, %v)", got.occurrence, exists, err)
	}
}

func TestNotificationOccurrenceIndexPrunesExpiredRecordsWithoutKVDeleteEvent(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	now := time.Now().UTC()
	created, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   "U-expired-recipient",
		SourceEventID: "E-expired-source",
		SourceCreated: now,
		Target:        &corev1.NotificationTarget{RoomId: "R-expired-room", EventId: "E-expired-source"},
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_REPLY,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	key := notificationOccurrenceKey(created.GetRecipientId(), created.GetSourceEventId())
	entry, err := chattoCore.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get occurrence KV: %v", err)
	}
	expired := proto.Clone(created).(*corev1.NotificationOccurrence)
	expired.ExpiresAt = timestamppb.New(now.Add(-time.Second))
	data, err := proto.Marshal(expired)
	if err != nil {
		t.Fatalf("marshal expired occurrence: %v", err)
	}
	revision, err := chattoCore.storage.runtimeStateKV.Update(ctx, key, data, entry.Revision())
	if err != nil {
		t.Fatalf("write stale expired occurrence: %v", err)
	}
	if err := chattoCore.NotificationOccurrences().index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for stale expiry revision: %v", err)
	}

	if occurrences, err := chattoCore.NotificationOccurrences().List(ctx, created.GetRecipientId(), NotificationOccurrenceViewInbox); err != nil || len(occurrences) != 0 {
		t.Fatalf("List expired occurrences = (%v, %v), want empty", occurrences, err)
	}
	if _, exists, err := chattoCore.NotificationOccurrences().index.occurrenceBySource(ctx, created.GetRecipientId(), created.GetSourceEventId()); err != nil || exists {
		t.Fatalf("expired index entry exists=%v, err=%v", exists, err)
	}
	chattoCore.NotificationOccurrences().index.mu.RLock()
	_, retainsRevisionFence := chattoCore.NotificationOccurrences().index.keyRevisions[key]
	chattoCore.NotificationOccurrences().index.mu.RUnlock()
	if retainsRevisionFence {
		t.Fatal("expired index entry retained its revision fence")
	}
	if err := chattoCore.NotificationOccurrences().index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for pruned expiry revision: %v", err)
	}
}
