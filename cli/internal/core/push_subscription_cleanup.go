package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	pushSubscriptionCleanupConsumerName       = "chatto-user-push-subscription-cleanup-v1"
	pushSubscriptionCleanupConsumerAckWait    = 2 * time.Minute
	pushSubscriptionCleanupDeliveryHeartbeat  = 30 * time.Second
	pushSubscriptionCleanupRetryDelay         = 30 * time.Second
	pushSubscriptionCleanupAcknowledgeTimeout = 5 * time.Second
	pushSubscriptionCleanupMaxPending         = 16

	pushSubscriptionReconcileEvery     = 15 * time.Second
	pushSubscriptionReconcileLeaseTTL  = time.Minute
	pushSubscriptionReconcileLeaseName = "push-subscription-deletion-reconcile"
)

// pushSubscriptionCleanupModel turns the existing UserAccountDeleted domain
// fact into recoverable physical removal of Web Push credentials. The durable
// consumer handles the normal path. One leased startup/periodic pass scans the
// current subscription and owner keyspaces once, using the permanent EVT fact
// as its fence, to repair late writes, old partial failures, and orphaned owner
// records without replaying a global scan for every deleted account.
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
	if _, err := m.deleteAllFn(ctx, userID); err != nil {
		return fmt.Errorf("delete push subscriptions for deleted account: %w", err)
	}
	return nil
}

func (m *pushSubscriptionCleanupModel) runReconciler(ctx context.Context) {
	err := m.reconcileLease.Run(ctx, func(leaderCtx context.Context) error {
		for {
			if err := m.reconcileDeletedAccountPushState(leaderCtx); err != nil && leaderCtx.Err() == nil {
				m.logger.Warn("Failed to reconcile push subscriptions for deleted accounts", "error", err)
			} else if err == nil {
				m.logger.Debug("Reconciled push subscriptions and endpoint owners")
			}
			timer := time.NewTimer(pushSubscriptionReconcileEvery)
			select {
			case <-leaderCtx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil
			case <-timer.C:
			}
		}
	})
	if err != nil && ctx.Err() == nil {
		m.logger.Warn("Push-subscription reconciliation lease stopped", "error", err)
	}
}

func (m *pushSubscriptionCleanupModel) reconcileDeletedAccountPushState(ctx context.Context) error {
	deletedAccounts := make(map[string]bool)
	cleanedAccounts := make(map[string]bool)
	ownerReconciledAccounts := make(map[string]bool)
	var cleanupErrors []error

	subscriptionKeys, err := listPushRuntimeStateKeys(ctx, m.core.storage.runtimeStateKV, "push_subscription.>")
	if err != nil {
		return fmt.Errorf("list push subscriptions during account reconciliation: %w", err)
	}
	for _, key := range subscriptionKeys {
		userID := extractUserIDFromPushKey(key)
		if userID == "" {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("invalid push subscription key during account reconciliation"))
			continue
		}
		deleted, err := m.accountDeleted(ctx, userID, deletedAccounts)
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if !deleted || cleanedAccounts[userID] {
			continue
		}
		if _, err := m.deleteAllFn(ctx, userID); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("reconcile deleted account push subscriptions: %w", err))
			continue
		}
		cleanedAccounts[userID] = true
	}

	ownerKeys, err := listPushRuntimeStateKeys(ctx, m.core.storage.runtimeStateKV, pushEndpointOwnerKeyPrefix+">")
	if err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("list push endpoint owners during account reconciliation: %w", err))
		return errors.Join(cleanupErrors...)
	}
	for _, key := range ownerKeys {
		userID, ownerRevision, remove, err := m.inspectPushEndpointOwner(ctx, key, deletedAccounts)
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if !remove {
			continue
		}
		if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(ownerRevision)); err != nil && !isPushRuntimeStateKeyAbsent(err) {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete push endpoint owner during account reconciliation: %w", err))
			continue
		}
		if userID == "" || ownerReconciledAccounts[userID] {
			continue
		}
		deleted, err := m.accountDeleted(ctx, userID, deletedAccounts)
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if !deleted {
			continue
		}
		ownerReconciledAccounts[userID] = true
		if _, err := m.deleteAllFn(ctx, userID); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("reconcile deleted account after owner removal: %w", err))
			continue
		}
		cleanedAccounts[userID] = true
	}

	return errors.Join(cleanupErrors...)
}

// inspectPushEndpointOwner reports whether an owner record should be removed.
// It treats malformed, orphaned, stale-revision, and deleted-account owners as
// invalid. The owner key contains only a hash, so its corresponding subscription
// key is reconstructed from the stored user ID and the short hash prefix.
func (m *pushSubscriptionCleanupModel) inspectPushEndpointOwner(
	ctx context.Context,
	key string,
	deletedAccounts map[string]bool,
) (userID string, revision uint64, remove bool, err error) {
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
	if isPushRuntimeStateKeyAbsent(err) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("get push endpoint owner during account reconciliation: %w", err)
	}
	var owner pushEndpointOwner
	if err := json.Unmarshal(entry.Value(), &owner); err != nil || owner.UserID == "" || owner.SubscriptionRevision == 0 {
		return "", entry.Revision(), true, nil
	}

	fullHash := strings.TrimPrefix(key, pushEndpointOwnerKeyPrefix)
	if fullHash == key || len(fullHash) != sha256HexLength {
		return owner.UserID, entry.Revision(), true, nil
	}
	if _, err := hex.DecodeString(fullHash); err != nil {
		return owner.UserID, entry.Revision(), true, nil
	}
	subscriptionKey := "push_subscription." + owner.UserID + "." + fullHash[:shortPushEndpointHashLength]
	subscriptionEntry, err := m.core.storage.runtimeStateKV.Get(ctx, subscriptionKey)
	if isPushRuntimeStateKeyAbsent(err) {
		return owner.UserID, entry.Revision(), true, nil
	}
	if err != nil {
		return owner.UserID, entry.Revision(), false, fmt.Errorf("get push subscription for endpoint owner reconciliation: %w", err)
	}
	var subscription corev1.PushSubscription
	if err := proto.Unmarshal(subscriptionEntry.Value(), &subscription); err != nil {
		return owner.UserID, entry.Revision(), true, nil
	}
	if subscriptionEntry.Revision() != owner.SubscriptionRevision || pushEndpointOwnerKey(subscription.GetEndpoint()) != key {
		return owner.UserID, entry.Revision(), true, nil
	}
	deleted, err := m.accountDeleted(ctx, owner.UserID, deletedAccounts)
	if err != nil {
		return owner.UserID, entry.Revision(), false, err
	}
	return owner.UserID, entry.Revision(), deleted, nil
}

func (m *pushSubscriptionCleanupModel) accountDeleted(
	ctx context.Context,
	userID string,
	cache map[string]bool,
) (bool, error) {
	if deleted, ok := cache[userID]; ok {
		return deleted, nil
	}
	subject := evtstream.UserAggregate(userID).Subject(evtstream.EventUserAccountDeleted)
	sequence, err := m.core.EventPublisher.LastSubjectSeq(ctx, subject)
	if err != nil {
		return false, fmt.Errorf("check account-deletion fact during push reconciliation: %w", err)
	}
	deleted := sequence > 0
	cache[userID] = deleted
	return deleted, nil
}

const (
	sha256HexLength             = 64
	shortPushEndpointHashLength = 16
)
