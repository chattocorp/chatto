package core

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrNotificationAlertSuppressed tells the durable worker that a last-moment
// transport check intentionally suppressed delivery and should be terminal.
var ErrNotificationAlertSuppressed = errors.New("notification alert suppressed")

// ErrUnsupportedNotificationSignal preserves work for a newer signal shape
// until a binary that understands it can safely process it.
var ErrUnsupportedNotificationSignal = errors.New("unsupported notification signal")

const (
	notificationAlertConsumerName = "chatto-notification-alert-delivery-v1"
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
func (c *ChattoCore) SetNotificationAlertHandler(handler func(context.Context, *notificationv1.NotificationOccurrence) error) {
	c.notificationAlertHandler = handler
}

func (d *notificationAlertDelivery) initialize(ctx context.Context) error {
	consumer, err := evtstream.CreateEffectConsumer(ctx, d.core.storage.notificationStream, evtstream.EffectConsumerConfig{
		Name:           notificationAlertConsumerName,
		Description:    "Shared durable worker for Chatto notification alerts",
		FilterSubjects: []string{notificationstream.SignalledSubject},
		AckWait:        notificationAlertAckWait,
		MaxAckPending:  notificationAlertMaxPending,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
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
	worker, err := evtstream.NewEffectWorker(d.consumer, d.processDelivery, evtstream.EffectWorkerOptions{
		MaxConcurrent:     notificationAlertMaxPending,
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
	var event notificationv1.NotificationEvent
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		return events.TerminateDelivery("invalid notification event", err)
	}
	signalled := event.GetSignalled()
	if event.GetId() == "" || event.GetRecipientId() == "" || event.GetNotificationId() == "" || signalled == nil {
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
	current, err := d.core.notificationOccurrences.Get(ctx, event.GetRecipientId(), event.GetNotificationId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !NotificationAlertPending(current) {
		return nil
	}
	deadline := NotificationAlertDeadline(current)
	if deadline.IsZero() || !d.core.notificationOccurrences.now().UTC().Before(deadline) {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, false)
	}
	eligible, err := d.core.NotificationAlertEligible(ctx, current)
	if err != nil {
		return err
	}
	if !eligible || d.core.notificationAlertHandler == nil {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, false)
	}
	if err := d.core.notificationAlertHandler(ctx, current); err != nil {
		if errors.Is(err, ErrNotificationAlertSuppressed) {
			return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, false)
		}
		return err
	}
	return d.core.notificationOccurrences.completeAlertDelivery(ctx, current, true)
}

// reconcileExpired closes PENDING alert state if its immutable source signal
// was removed or unavailable before the durable consumer processed it.
func (d *notificationAlertDelivery) reconcileExpired(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		now := d.core.notificationOccurrences.now().UTC()
		for _, occurrence := range d.core.notificationOccurrences.projection.Projection().allOccurrences(now) {
			if !NotificationAlertPending(occurrence) {
				continue
			}
			deadline := NotificationAlertDeadline(occurrence)
			if !deadline.IsZero() && now.Before(deadline) {
				continue
			}
			if err := d.core.notificationOccurrences.completeAlertDelivery(ctx, occurrence, false); err != nil {
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

// NotificationSoundEligible fences notification materialization and policy,
// then checks the exact unread occurrence and current target visibility. Local
// sound remains a best-effort live effect and is not durable delivery work.
func (c *ChattoCore) NotificationSoundEligible(ctx context.Context, occurrence *notificationv1.NotificationOccurrence) (bool, error) {
	if occurrence == nil {
		return false, nil
	}
	if NotificationOccurrenceHasUnsupportedSignal(occurrence) {
		return false, ErrUnsupportedNotificationSignal
	}
	if err := c.notificationMaterializer.WaitCurrent(ctx); err != nil {
		return false, fmt.Errorf("fence notification materializer before sound: %w", err)
	}
	if err := c.waitForCurrentNotificationPolicy(ctx); err != nil {
		return false, err
	}
	current, err := c.notificationOccurrences.deliveryCurrent(ctx, occurrence)
	if err != nil || current == nil || current.GetRead() {
		return false, err
	}
	if !c.notificationAlertDelivery.currentPolicyAllowsSound(current) {
		return false, nil
	}
	presence, err := c.GetUserPresence(ctx, current.GetRecipientId())
	if err != nil {
		return false, fmt.Errorf("read notification recipient presence: %w", err)
	}
	if presence == PresenceStatusDoNotDisturb {
		return false, nil
	}
	visible, err := c.notificationOccurrences.VisibleOccurrences(ctx, current.GetRecipientId(), []*notificationv1.NotificationOccurrence{current})
	if err != nil {
		return false, fmt.Errorf("revalidate notification visibility: %w", err)
	}
	return len(visible) == 1, nil
}

// NotificationAlertEligible fences notification materialization and policy,
// then checks the exact unread occurrence and current target visibility. Push
// transports call it again immediately before contacting their provider.
func (c *ChattoCore) NotificationAlertEligible(ctx context.Context, occurrence *notificationv1.NotificationOccurrence) (bool, error) {
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
	deadline := NotificationAlertDeadline(occurrence)
	if deadline.IsZero() || !c.notificationOccurrences.now().UTC().Before(deadline) {
		return false, nil
	}
	if !c.notificationAlertDelivery.currentPolicyAllowsAlert(occurrence) {
		return false, nil
	}
	presence, err := c.GetUserPresence(ctx, occurrence.GetRecipientId())
	if err != nil {
		return false, fmt.Errorf("read notification recipient presence: %w", err)
	}
	if presence == PresenceStatusDoNotDisturb {
		return false, nil
	}
	visible, err := c.notificationOccurrences.VisibleOccurrences(ctx, occurrence.GetRecipientId(), []*notificationv1.NotificationOccurrence{occurrence})
	if err != nil {
		return false, fmt.Errorf("revalidate notification visibility: %w", err)
	}
	return len(visible) == 1, nil
}

func (d *notificationAlertDelivery) currentPolicyAllowsAlert(occurrence *notificationv1.NotificationOccurrence) bool {
	message := notificationSignalMessage(occurrence.GetSignal())
	if message == nil || notificationSignalIdentity(occurrence.GetSignal()) == "" {
		return false
	}
	return d.core.GetEffectiveNotificationModeForSignal(occurrence.GetRecipientId(), message.GetRoomId(), occurrence.GetSignal()) == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION
}

func (d *notificationAlertDelivery) currentPolicyAllowsSound(occurrence *notificationv1.NotificationOccurrence) bool {
	message := notificationSignalMessage(occurrence.GetSignal())
	if message == nil || notificationSignalIdentity(occurrence.GetSignal()) == "" {
		return false
	}
	mode := d.core.GetEffectiveNotificationModeForSignal(occurrence.GetRecipientId(), message.GetRoomId(), occurrence.GetSignal())
	return notificationModeProducesOccurrence(mode)
}
