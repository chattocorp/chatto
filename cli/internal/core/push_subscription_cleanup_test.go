package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func TestPushSubscriptionCleanupDeliveryRetriesPartialFailure(t *testing.T) {
	chatto, _ := setupTestCore(t)
	model := chatto.pushSubscriptionCleanup
	userID := "push-cleanup-retry-user"
	delivery := pushSubscriptionCleanupDelivery(t, userID, userID)

	calls := 0
	model.deleteAllFn = func(context.Context, string) (int, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("injected partial cleanup failure")
		}
		return 1, nil
	}
	if err := model.processDelivery(context.Background(), delivery); err == nil {
		t.Fatal("first cleanup delivery unexpectedly succeeded")
	}
	if _, err := chatto.storage.runtimeStateKV.Get(context.Background(), pushSubscriptionDeletionFenceKey(userID)); err != nil {
		t.Fatalf("cleanup fence was not durable before the failed physical cleanup: %v", err)
	}
	if err := model.processDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("redelivered cleanup: %v", err)
	}
	if calls != 2 {
		t.Fatalf("cleanup calls = %d, want 2", calls)
	}
}

func TestPushSubscriptionCleanupDeliveryRejectsMismatchedSubject(t *testing.T) {
	chatto, _ := setupTestCore(t)
	delivery := pushSubscriptionCleanupDelivery(t, "push-cleanup-subject-user", "push-cleanup-payload-user")
	if err := chatto.pushSubscriptionCleanup.processDelivery(context.Background(), delivery); err == nil {
		t.Fatal("mismatched cleanup delivery unexpectedly succeeded")
	}
}

func TestSavePushSubscriptionRejectsDeletedAccountFence(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-fenced-user"
	if err := chatto.pushSubscriptionCleanup.recordDeletionFence(ctx, userID); err != nil {
		t.Fatalf("record deletion fence: %v", err)
	}
	if _, err := chatto.SavePushSubscription(ctx, userID, "https://push.example.com/fenced", "key", "auth", "browser"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SavePushSubscription error = %v, want ErrNotFound", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, pushSubscriptionKey(userID, "https://push.example.com/fenced")); !isPushRuntimeStateKeyAbsent(err) {
		t.Fatalf("fenced subscription was stored: %v", err)
	}
}

func TestPushSubscriptionCleanupReconcilesLateWriteAfterDeletionDelivery(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-late-write-user"
	endpoint := "https://push.example.com/late-write"
	if err := chatto.pushSubscriptionCleanup.recordDeletionFence(ctx, userID); err != nil {
		t.Fatalf("record deletion fence: %v", err)
	}
	subscription := &corev1.PushSubscription{
		Endpoint:  endpoint,
		P256Dh:    "key",
		Auth:      "auth",
		CreatedAt: timestamppb.New(time.Now()),
		UserAgent: "browser",
	}
	data, err := proto.Marshal(subscription)
	if err != nil {
		t.Fatalf("marshal late subscription: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Put(ctx, pushSubscriptionKey(userID, endpoint), data); err != nil {
		t.Fatalf("store late subscription: %v", err)
	}
	if err := chatto.claimPushEndpointOwnership(ctx, userID, endpoint); err != nil {
		t.Fatalf("claim late endpoint ownership: %v", err)
	}

	if err := chatto.pushSubscriptionCleanup.reconcileDeletionFences(ctx); err != nil {
		t.Fatalf("reconcile deletion fences: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, pushSubscriptionKey(userID, endpoint)); !isPushRuntimeStateKeyAbsent(err) {
		t.Fatalf("late subscription remains after reconciliation: %v", err)
	}
	if owned, err := chatto.PushSubscriptionOwnedByUser(ctx, userID, endpoint); err != nil || owned {
		t.Fatalf("late endpoint ownership after reconciliation = %v, err = %v", owned, err)
	}
}

func TestSavePushSubscriptionRejectsCommittedAccountDeletion(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-deleted-user"
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserAccountDeleted{
		UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: userID},
	}})
	if _, err := chatto.EventPublisher.AppendEventually(ctx, evtstream.UserAggregate(userID).SubjectFor(event), event); err != nil {
		t.Fatalf("append account deletion: %v", err)
	}
	if _, err := chatto.SavePushSubscription(ctx, userID, "https://push.example.com/deleted", "key", "auth", "browser"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SavePushSubscription error = %v, want ErrNotFound", err)
	}
}

func TestDeleteUserImmediatelyRemovesPushCredentials(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	user, err := chatto.CreateUser(ctx, SystemActorID, "push-cleanup-delete-user", "Push Cleanup Delete User", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	endpoint := "https://push.example.com/account-deletion"
	if _, err := chatto.SavePushSubscription(ctx, user.GetId(), endpoint, "key", "auth", "browser"); err != nil {
		t.Fatalf("save push subscription: %v", err)
	}

	if err := chatto.DeleteUser(ctx, SystemActorID, user.GetId()); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, pushSubscriptionKey(user.GetId(), endpoint)); !isPushRuntimeStateKeyAbsent(err) {
		t.Fatalf("push subscription remains after account deletion: %v", err)
	}
	if owned, err := chatto.PushSubscriptionOwnedByUser(ctx, user.GetId(), endpoint); err != nil || owned {
		t.Fatalf("push endpoint ownership after account deletion = %v, err = %v", owned, err)
	}
}

func pushSubscriptionCleanupDelivery(t *testing.T, subjectUserID, payloadUserID string) events.DurableDelivery {
	t.Helper()
	event := newEvent(payloadUserID, &corev1.Event{Event: &corev1.Event_UserAccountDeleted{
		UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: payloadUserID},
	}})
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal cleanup delivery: %v", err)
	}
	return events.DurableDelivery{
		Subject:        evtstream.UserAggregate(subjectUserID).SubjectFor(event),
		Data:           data,
		StreamSequence: 1,
	}
}
