package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/pkg/events"
)

const (
	pushSubscriptionCleanupConsumerName       = "chatto-user-push-subscription-cleanup-v1"
	pushSubscriptionCleanupConsumerAckWait    = 2 * time.Minute
	pushSubscriptionCleanupDeliveryHeartbeat  = 30 * time.Second
	pushSubscriptionCleanupRetryDelay         = 30 * time.Second
	pushSubscriptionCleanupAcknowledgeTimeout = 5 * time.Second
	pushSubscriptionCleanupMaxPending         = 16

	pushSubscriptionDeletionFenceKeyPrefix = "push_subscription_account_deleted."
	// The durable account-deletion fact remains the permanent authority. This
	// short-lived fence only covers an already-authorized subscription request
	// that was in flight while its account was deleted.
	pushSubscriptionDeletionFenceTTL   = 24 * time.Hour
	pushSubscriptionReconcileEvery     = 15 * time.Second
	pushSubscriptionReconcileTimeout   = 30 * time.Second
	pushSubscriptionReconcileLeaseTTL  = time.Minute
	pushSubscriptionReconcileLeaseName = "push-subscription-deletion-reconcile"
)

// pushSubscriptionCleanupModel turns the existing UserAccountDeleted domain
// fact into recoverable physical removal of Web Push credentials. The durable
// consumer handles normal crash/retry recovery. A bounded, leased reconciliation
// pass covers the narrower race where an already-authorized registration write
// lands after the deletion delivery has completed.
type pushSubscriptionCleanupModel struct {
	core           *ChattoCore
	worker         *events.DurableWorker
	reconcileLease *lease.Lease
	logger         *log.Logger
	deleteAllFn    func(context.Context, string) (int, error)
}

func newPushSubscriptionCleanupModel(
	ctx context.Context,
	core *ChattoCore,
	reconcileLease *lease.Lease,
	logger *log.Logger,
) (*pushSubscriptionCleanupModel, error) {
	consumer, err := core.storage.serverEvtStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:            pushSubscriptionCleanupConsumerName,
		Durable:         pushSubscriptionCleanupConsumerName,
		Description:     "Shared durable worker for Chatto user push-subscription cleanup",
		DeliverPolicy:   jetstream.DeliverAllPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         pushSubscriptionCleanupConsumerAckWait,
		MaxDeliver:      -1,
		FilterSubject:   evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   pushSubscriptionCleanupMaxPending,
		MaxRequestBatch: pushSubscriptionCleanupMaxPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create user push-subscription cleanup consumer: %w", err)
	}
	model := &pushSubscriptionCleanupModel{
		core:           core,
		reconcileLease: reconcileLease,
		logger:         logger,
		deleteAllFn:    core.DeleteAllUserPushSubscriptions,
	}
	model.worker, err = events.NewDurableWorker(consumer, model.processDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     pushSubscriptionCleanupMaxPending,
		FetchMaxWait:      time.Second,
		RetryDelay:        pushSubscriptionCleanupRetryDelay,
		AckTimeout:        pushSubscriptionCleanupAcknowledgeTimeout,
		HeartbeatInterval: pushSubscriptionCleanupDeliveryHeartbeat,
		Logger:            logger,
	})
	if err != nil {
		return nil, fmt.Errorf("configure user push-subscription cleanup worker: %w", err)
	}
	return model, nil
}

func (m *pushSubscriptionCleanupModel) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return m.worker.Run(gctx) })
	g.Go(func() error {
		m.runReconciler(gctx)
		return nil
	})
	return g.Wait()
}

func (m *pushSubscriptionCleanupModel) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	deleted := event.GetUserAccountDeleted()
	userID, ok := evtstream.ParseUserSubject(delivery.Subject)
	if !ok || deleted == nil || deleted.GetUserId() == "" || userID != deleted.GetUserId() {
		return events.TerminateDelivery(
			"invalid user push-subscription cleanup fact",
			errors.New("account-deletion subject and payload do not match"),
		)
	}
	if err := m.recordDeletionFence(ctx, userID); err != nil {
		return err
	}
	if _, err := m.deleteAllFn(ctx, userID); err != nil {
		return fmt.Errorf("delete push subscriptions for deleted account: %w", err)
	}
	return nil
}

func (m *pushSubscriptionCleanupModel) recordDeletionFence(ctx context.Context, userID string) error {
	_, err := m.core.storage.runtimeStateKV.Create(
		ctx,
		pushSubscriptionDeletionFenceKey(userID),
		[]byte{1},
		jetstream.KeyTTL(pushSubscriptionDeletionFenceTTL),
	)
	if err == nil || jetstreamutil.IsSequenceConflict(err) {
		return nil
	}
	return fmt.Errorf("record push-subscription account-deletion fence: %w", err)
}

func (m *pushSubscriptionCleanupModel) runReconciler(ctx context.Context) {
	ticker := time.NewTicker(pushSubscriptionReconcileEvery)
	defer ticker.Stop()
	for {
		m.tryReconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *pushSubscriptionCleanupModel) tryReconcile(ctx context.Context) {
	reconcileCtx, cancel := context.WithTimeout(ctx, pushSubscriptionReconcileTimeout)
	defer cancel()
	acquired, err := m.reconcileLease.TryRunWithCooldown(reconcileCtx, m.reconcileDeletionFences)
	if err != nil && ctx.Err() == nil {
		m.logger.Warn("Failed to reconcile push subscriptions for deleted accounts", "error", err)
		return
	}
	if acquired {
		m.logger.Debug("Reconciled push subscriptions for recently deleted accounts")
	}
}

func (m *pushSubscriptionCleanupModel) reconcileDeletionFences(ctx context.Context) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, pushSubscriptionDeletionFenceKeyPrefix+">")
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list push-subscription account-deletion fences: %w", err)
	}
	var cleanupErrors []error
	for key := range lister.Keys() {
		userID := strings.TrimPrefix(key, pushSubscriptionDeletionFenceKeyPrefix)
		if userID == "" || userID == key || strings.Contains(userID, ".") {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("invalid push-subscription account-deletion fence key"))
			continue
		}
		// The deletion fence remains for 24 hours so registrations that were
		// already in flight cannot recreate credentials after the durable worker
		// has acknowledged the deletion fact. Avoid repeatedly scanning every
		// endpoint-owner record for fences whose subscriptions are already gone.
		hasSubscriptions, err := m.hasPushSubscriptions(ctx, userID)
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if !hasSubscriptions {
			continue
		}
		if _, err := m.deleteAllFn(ctx, userID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func (m *pushSubscriptionCleanupModel) hasPushSubscriptions(ctx context.Context, userID string) (bool, error) {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, pushSubscriptionKeyFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("list push subscriptions for deleted account: %w", err)
	}
	for range lister.Keys() {
		return true, nil
	}
	return false, nil
}

func pushSubscriptionDeletionFenceKey(userID string) string {
	return pushSubscriptionDeletionFenceKeyPrefix + userID
}
