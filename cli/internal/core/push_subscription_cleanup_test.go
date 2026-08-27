package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
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

func TestPushSubscriptionCleanupDurableWorkerHandsOffInterruptedDelivery(t *testing.T) {
	chatto, _ := newTestCore(t)
	ctx := testContext(t)
	model := chatto.pushSubscriptionCleanup
	consumer, err := chatto.storage.serverEvtStream.Consumer(ctx, pushSubscriptionCleanupConsumerName)
	if err != nil {
		t.Fatalf("load push cleanup consumer: %v", err)
	}

	started := make(chan struct{})
	model.deleteAllFn = func(ctx context.Context, _ string) (int, error) {
		close(started)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	firstWorker, err := events.NewDurableWorker(consumer, model.processDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     1,
		FetchMaxWait:      20 * time.Millisecond,
		RetryDelay:        10 * time.Millisecond,
		AckTimeout:        time.Second,
		HeartbeatInterval: time.Second,
		Logger:            model.logger,
	})
	if err != nil {
		t.Fatalf("configure first push cleanup worker: %v", err)
	}
	firstCtx, cancelFirst := context.WithCancel(ctx)
	firstRun := make(chan error, 1)
	go func() { firstRun <- firstWorker.Run(firstCtx) }()

	userID := "push-cleanup-worker-handoff-user"
	appendPushAccountDeletionFact(t, chatto, userID)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("wait for first cleanup attempt: %v", ctx.Err())
	}
	cancelFirst()
	if err := <-firstRun; err != nil {
		t.Fatalf("stop first push cleanup worker: %v", err)
	}

	model.deleteAllFn = func(context.Context, string) (int, error) { return 1, nil }
	completed := make(chan uint64, 1)
	secondWorker, err := events.NewDurableWorker(consumer, func(ctx context.Context, delivery events.DurableDelivery) error {
		err := model.processDelivery(ctx, delivery)
		if err == nil {
			completed <- delivery.NumDelivered
		}
		return err
	}, events.DurableWorkerOptions{
		MaxConcurrent:     1,
		FetchMaxWait:      20 * time.Millisecond,
		RetryDelay:        10 * time.Millisecond,
		AckTimeout:        time.Second,
		HeartbeatInterval: time.Second,
		Logger:            model.logger,
	})
	if err != nil {
		t.Fatalf("configure second push cleanup worker: %v", err)
	}
	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondRun := make(chan error, 1)
	go func() { secondRun <- secondWorker.Run(secondCtx) }()
	select {
	case deliveries := <-completed:
		if deliveries < 2 {
			t.Fatalf("delivery attempt = %d, want a redelivery", deliveries)
		}
	case <-ctx.Done():
		t.Fatalf("wait for handed-off cleanup: %v", ctx.Err())
	}
	cancelSecond()
	if err := <-secondRun; err != nil {
		t.Fatalf("stop second push cleanup worker: %v", err)
	}
}

func TestPushSubscriptionCleanupReconcilesLateWriteAfterCompletedDeletionDelivery(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-late-write-user"
	endpoint := "https://push.example.com/late-write"
	appendPushAccountDeletionFact(t, chatto, userID)
	if err := chatto.pushSubscriptionCleanup.processDelivery(ctx, pushSubscriptionCleanupDelivery(t, userID, userID)); err != nil {
		t.Fatalf("complete initial deletion delivery: %v", err)
	}
	// Model an already-authorised registration completing after the deletion
	// worker has acknowledged its otherwise successful pass.
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

	if err := chatto.pushSubscriptionCleanup.reconcileDeletedAccountPushState(ctx); err != nil {
		t.Fatalf("reconcile deleted-account push state: %v", err)
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
	appendPushAccountDeletionFact(t, chatto, userID)
	if _, err := chatto.SavePushSubscription(ctx, userID, "https://push.example.com/deleted", "key", "auth", "browser"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SavePushSubscription error = %v, want ErrNotFound", err)
	}
}

func TestPushSubscriptionCleanupPreservesHostAwareEndpointOwner(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-host-aware-user"
	endpoint := "https://push.example.com/host-aware"
	if _, err := chatto.SavePushSubscriptionForClient(
		ctx,
		userID,
		endpoint,
		"key",
		"auth",
		"browser",
		"app.example.com",
	); err != nil {
		t.Fatalf("save host-aware subscription: %v", err)
	}

	if err := chatto.pushSubscriptionCleanup.reconcileDeletedAccountPushState(ctx); err != nil {
		t.Fatalf("reconcile active host-aware push state: %v", err)
	}
	if owned, err := chatto.PushSubscriptionOwnedByUser(ctx, userID, endpoint); err != nil || !owned {
		t.Fatalf("host-aware endpoint ownership after reconciliation = %v, err = %v", owned, err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, pushSubscriptionKey(userID, endpoint)); err != nil {
		t.Fatalf("host-aware subscription after reconciliation: %v", err)
	}

	// Simulate a subscription record disappearing after its owner claim was
	// written. The leased reconciler must repair the orphaned owner.
	if err := chatto.storage.runtimeStateKV.Delete(ctx, pushSubscriptionKey(userID, endpoint)); err != nil {
		t.Fatalf("delete subscription fixture: %v", err)
	}
	if err := chatto.pushSubscriptionCleanup.reconcileDeletedAccountPushState(ctx); err != nil {
		t.Fatalf("reconcile orphaned host-aware owner: %v", err)
	}
	if owned, err := chatto.PushSubscriptionOwnedByUser(ctx, userID, endpoint); err != nil || owned {
		t.Fatalf("orphaned host-aware endpoint ownership after reconciliation = %v, err = %v", owned, err)
	}
}

func TestPushSubscriptionCleanupRepairsOwnerOnlyCrashState(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-owner-only-user"
	endpoint := "https://push.example.com/owner-only"
	appendPushAccountDeletionFact(t, chatto, userID)

	value, err := json.Marshal(pushEndpointOwner{UserID: userID, SubscriptionRevision: 42})
	if err != nil {
		t.Fatalf("marshal owner-only fixture: %v", err)
	}
	ownerKey := pushEndpointOwnerKey(endpoint)
	if _, err := chatto.storage.runtimeStateKV.Put(ctx, ownerKey, value); err != nil {
		t.Fatalf("store owner-only fixture: %v", err)
	}

	if err := chatto.pushSubscriptionCleanup.reconcileDeletedAccountPushState(ctx); err != nil {
		t.Fatalf("reconcile owner-only crash state: %v", err)
	}
	if _, err := chatto.storage.runtimeStateKV.Get(ctx, ownerKey); !isPushRuntimeStateKeyAbsent(err) {
		t.Fatalf("owner-only record remains after reconciliation: %v", err)
	}
}

func TestDeleteAllUserPushSubscriptionsRejectsPartialKeyListing(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := context.Background()
	userID := "push-cleanup-partial-list-user"
	endpoints := []string{
		"https://push.example.com/partial-list-a",
		"https://push.example.com/partial-list-b",
	}
	for _, endpoint := range endpoints {
		if _, err := chatto.SavePushSubscription(ctx, userID, endpoint, "key", "auth", "browser"); err != nil {
			t.Fatalf("save push subscription: %v", err)
		}
	}

	realKV := chatto.storage.runtimeStateKV
	firstEntry, err := realKV.Get(ctx, pushSubscriptionKey(userID, endpoints[0]))
	if err != nil {
		t.Fatalf("get partial-list fixture: %v", err)
	}
	chatto.storage.runtimeStateKV = &partialKeyListingKV{
		KeyValue: realKV,
		filter:   pushSubscriptionKeyFilter(userID),
		entries:  []jetstream.KeyValueEntry{firstEntry},
	}
	if deleted, err := chatto.DeleteAllUserPushSubscriptions(ctx, userID); err == nil || deleted != 0 {
		t.Fatalf("partial listing cleanup = (%d, %v), want (0, error)", deleted, err)
	}
	chatto.storage.runtimeStateKV = realKV
	for _, endpoint := range endpoints {
		if _, err := realKV.Get(ctx, pushSubscriptionKey(userID, endpoint)); err != nil {
			t.Fatalf("subscription %q was deleted from a partial listing: %v", endpoint, err)
		}
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

func appendPushAccountDeletionFact(t *testing.T, chatto *ChattoCore, userID string) {
	t.Helper()
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserAccountDeleted{
		UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: userID},
	}})
	if _, err := chatto.EventPublisher.AppendEventually(context.Background(), evtstream.UserAggregate(userID).SubjectFor(event), event); err != nil {
		t.Fatalf("append account deletion: %v", err)
	}
}

type partialKeyListingKV struct {
	jetstream.KeyValue
	filter  string
	entries []jetstream.KeyValueEntry
}

func (kv *partialKeyListingKV) WatchFiltered(
	_ context.Context,
	filters []string,
	opts ...jetstream.WatchOpt,
) (jetstream.KeyWatcher, error) {
	if len(filters) != 1 || filters[0] != kv.filter {
		return kv.KeyValue.WatchFiltered(context.Background(), filters, opts...)
	}
	updates := make(chan jetstream.KeyValueEntry, len(kv.entries))
	for _, entry := range kv.entries {
		updates <- entry
	}
	close(updates)
	return &staticKeyWatcher{updates: updates}, nil
}

type staticKeyWatcher struct {
	updates <-chan jetstream.KeyValueEntry
}

func (w *staticKeyWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *staticKeyWatcher) Stop() error                             { return nil }
