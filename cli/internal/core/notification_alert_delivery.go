package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/notificationstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrNotificationAlertSuppressed tells the durable worker that a last-moment
// transport check intentionally suppressed delivery and should be terminal.
var ErrNotificationAlertSuppressed = errors.New("notification alert suppressed")

// ErrUnsupportedNotificationSignal preserves work for a newer signal shape
// until a binary that understands it can safely process it.
var ErrUnsupportedNotificationSignal = errors.New("unsupported notification signal")

const (
	notificationAlertConsumerName = "chatto-notification-alert-delivery-v2"
	notificationAlertMaxPending   = 16
	notificationAlertAckWait      = time.Minute
	notificationAlertRetryDelay   = 30 * time.Second
	notificationAlertAckTimeout   = 5 * time.Second
	notificationAlertHeartbeat    = 15 * time.Second
	notificationAlertDeliveryTTL  = 2 * time.Minute
)

// notificationAlertDelivery consumes immutable NotificationSignalled facts
// directly from NOTIFICATIONS. The projected occurrence is the idempotency and
// eligibility fence; there is no second notification queue to reconcile.
type notificationAlertDelivery struct {
	core                       *ChattoCore
	consumer                   jetstream.Consumer
	waitForMaterializerCurrent func(context.Context) error
}

func newNotificationAlertDelivery(core *ChattoCore) *notificationAlertDelivery {
	return &notificationAlertDelivery{
		core:                       core,
		waitForMaterializerCurrent: core.notificationMaterializer.WaitCurrent,
	}
}

// SetNotificationAlertHandler enables alert work and configures its provider
// transport. It must be called during process setup, before ChattoCore.Run.
func (c *ChattoCore) SetNotificationAlertHandler(handler func(context.Context, *corev1.NotificationOccurrence) error) {
	c.notificationAlertHandler = handler
}

func (d *notificationAlertDelivery) initialize(ctx context.Context) error {
	consumer, err := d.core.storage.notificationStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:            notificationAlertConsumerName,
		Durable:         notificationAlertConsumerName,
		Description:     "Shared durable worker for Chatto notification alerts",
		DeliverPolicy:   jetstream.DeliverAllPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         notificationAlertAckWait,
		MaxDeliver:      -1,
		FilterSubject:   notificationstream.SignalledSubject,
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   notificationAlertMaxPending,
		MaxRequestBatch: notificationAlertMaxPending,
	})
	if err != nil {
		return fmt.Errorf("create notification alert consumer: %w", err)
	}
	d.consumer = consumer
	return nil
}

func (d *notificationAlertDelivery) run(ctx context.Context) error {
	if err := d.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification projection before alert delivery: %w", err)
	}
	worker, err := events.NewDurableWorker(d.consumer, d.processDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     notificationAlertMaxPending,
		FetchMaxWait:      time.Second,
		RetryDelay:        notificationAlertRetryDelay,
		AckTimeout:        notificationAlertAckTimeout,
		HeartbeatInterval: notificationAlertHeartbeat,
		Logger:            d.core.logger.WithPrefix("NotificationAlertWorker"),
	})
	if err != nil {
		return fmt.Errorf("configure notification alert worker: %w", err)
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return worker.Run(gctx) })
	g.Go(func() error { return d.reconcileExpired(gctx) })
	return g.Wait()
}

func (d *notificationAlertDelivery) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var event corev1.NotificationEvent
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return events.TerminateDelivery("invalid notification event", err)
	}
	signalled := event.GetSignalled()
	if event.GetId() == "" || event.GetRecipientId() == "" || signalled.GetNotificationId() == "" {
		return events.TerminateDelivery("invalid notification signal", errors.New("required notification coordinate is empty"))
	}
	if err := d.core.notificationOccurrences.projection.Projector().WaitFor(ctx, events.SubjectPosition(delivery.Subject, delivery.StreamSequence)); err != nil {
		return fmt.Errorf("wait for notification projection before alert delivery: %w", err)
	}
	// A visibility-loss fact may be queued behind the source materialization.
	// Fence that EVT worker before current-state eligibility is evaluated.
	if err := d.waitForMaterializerCurrent(ctx); err != nil {
		return fmt.Errorf("fence notification materializer before alert delivery: %w", err)
	}
	current, err := d.core.notificationOccurrences.Get(ctx, event.GetRecipientId(), signalled.GetNotificationId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
		return nil
	}
	deadline := NotificationAlertDeadline(current)
	if deadline.IsZero() || !d.core.notificationOccurrences.now().UTC().Before(deadline) {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}
	eligible, err := d.core.NotificationAlertEligible(ctx, current)
	if err != nil {
		return err
	}
	if !eligible || d.core.suppressesNotificationAlertsForPresence(ctx, current.GetRecipientId()) || d.core.notificationAlertHandler == nil {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}
	if err := d.core.notificationAlertHandler(ctx, current); err != nil {
		if errors.Is(err, ErrNotificationAlertSuppressed) {
			return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
		}
		return err
	}
	return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED)
}

// reconcileExpired closes PENDING alert state if its immutable source signal
// was removed or unavailable before the durable consumer processed it.
func (d *notificationAlertDelivery) reconcileExpired(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		now := d.core.notificationOccurrences.now().UTC()
		for _, occurrence := range d.core.notificationOccurrences.projection.Projection().allOccurrences(now) {
			if occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
				continue
			}
			deadline := NotificationAlertDeadline(occurrence)
			if !deadline.IsZero() && now.Before(deadline) {
				continue
			}
			if err := d.core.notificationOccurrences.completeAlertDelivery(ctx, occurrence, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// NotificationAlertEligible fences notification materialization and policy,
// then checks the exact unread occurrence and current target visibility. Push
// transports call it again immediately before contacting their provider.
func (c *ChattoCore) NotificationAlertEligible(ctx context.Context, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil {
		return false, nil
	}
	if NotificationOccurrenceHasUnsupportedSignal(occurrence) {
		return false, ErrUnsupportedNotificationSignal
	}
	if err := c.notificationMaterializer.WaitCurrent(ctx); err != nil {
		return false, fmt.Errorf("fence notification materializer before alert: %w", err)
	}
	if err := c.waitForCurrentNotificationPolicy(ctx); err != nil {
		return false, err
	}
	current, err := c.notificationOccurrences.alertDeliveryCurrent(ctx, occurrence)
	if err != nil || !current {
		return false, err
	}
	if !c.notificationAlertDelivery.currentPolicyAllowsAlert(occurrence) {
		return false, nil
	}
	visible, err := c.notificationOccurrences.VisibleOccurrences(ctx, occurrence.GetRecipientId(), []*corev1.NotificationOccurrence{occurrence})
	if err != nil {
		return false, fmt.Errorf("revalidate notification visibility: %w", err)
	}
	if len(visible) == 0 {
		_, err := c.notificationOccurrences.Delete(ctx, occurrence.GetRecipientId(), occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return false, err
	}
	return true, nil
}

func (d *notificationAlertDelivery) currentPolicyAllowsAlert(occurrence *corev1.NotificationOccurrence) bool {
	message := notificationSignalMessage(occurrence.GetSignal())
	kind := notificationSignalPolicyKind(occurrence.GetSignal())
	if message == nil || kind == corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_UNSPECIFIED {
		return false
	}
	return d.core.GetEffectiveNotificationIntensity(occurrence.GetRecipientId(), message.GetRoomId(), kind) == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT
}
