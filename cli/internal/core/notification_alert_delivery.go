package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrNotificationAlertSuppressed tells the queue worker that a last-moment
// transport check intentionally suppressed delivery and should be terminal.
var ErrNotificationAlertSuppressed = errors.New("notification alert suppressed")

const (
	notificationQueueStreamName   = "NOTIFICATIONS_QUEUE"
	notificationAlertSubject      = "notifications.alert"
	notificationAlertConsumerName = "chatto-notification-alert-delivery-v1"
	notificationAlertMaxPending   = 16
	notificationAlertAckWait      = time.Minute
	notificationAlertRetryDelay   = 30 * time.Second
	notificationAlertAckTimeout   = 5 * time.Second
	notificationAlertHeartbeat    = 15 * time.Second
	notificationAlertDeliveryTTL  = 2 * time.Minute
)

// notificationAlertDelivery owns the application queue and durable consumer
// that turn persisted occurrences into optional provider side effects.
type notificationAlertDelivery struct {
	core     *ChattoCore
	consumer jetstream.Consumer
}

func newNotificationAlertDelivery(core *ChattoCore) *notificationAlertDelivery {
	return &notificationAlertDelivery{core: core}
}

// SetNotificationAlertHandler enables alert work and configures its provider
// transport. It must be called during process setup, before ChattoCore.Run.
func (c *ChattoCore) SetNotificationAlertHandler(handler func(context.Context, *corev1.NotificationOccurrence) error) {
	c.notificationAlertHandler = handler
	c.notificationAlertsEnabled = handler != nil
}

func (d *notificationAlertDelivery) initialize(ctx context.Context) error {
	consumer, err := d.core.storage.notificationQueue.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          notificationAlertConsumerName,
		Durable:       notificationAlertConsumerName,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       notificationAlertAckWait,
		MaxDeliver:    -1,
		FilterSubject: notificationAlertSubject,
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
		MaxAckPending: notificationAlertMaxPending,
	})
	if err != nil {
		return fmt.Errorf("create notification alert consumer: %w", err)
	}
	d.consumer = consumer
	return nil
}

func (d *notificationAlertDelivery) run(ctx context.Context) error {
	if err := d.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification index before alert delivery: %w", err)
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
	return worker.Run(ctx)
}

// Enqueue publishes only an opaque occurrence coordinate. Message-ID
// deduplication covers source-worker redelivery; the occurrence's terminal
// alert state is the durable idempotency fence after queue acknowledgement.
func (d *notificationAlertDelivery) enqueue(ctx context.Context, occurrence *corev1.NotificationOccurrence) error {
	if !d.core.notificationAlertsEnabled || occurrence == nil ||
		occurrence.GetAlertState() != corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING {
		return nil
	}
	data, err := proto.Marshal(&corev1.NotificationAlertJob{
		RecipientId:    occurrence.GetRecipientId(),
		SourceEventId:  occurrence.GetSourceEventId(),
		NotificationId: occurrence.GetId(),
	})
	if err != nil {
		return fmt.Errorf("encode notification alert job: %w", err)
	}
	if _, err := d.core.js.Publish(ctx, notificationAlertSubject, data, jetstream.WithMsgID(occurrence.GetId())); err != nil {
		return fmt.Errorf("enqueue notification alert: %w", err)
	}
	return nil
}

func (d *notificationAlertDelivery) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var job corev1.NotificationAlertJob
	if err := proto.Unmarshal(delivery.Data, &job); err != nil {
		return events.TerminateDelivery("invalid notification alert job", err)
	}
	if job.GetRecipientId() == "" || job.GetSourceEventId() == "" || job.GetNotificationId() == "" {
		return events.TerminateDelivery("invalid notification alert coordinate", errors.New("required coordinate is empty"))
	}
	if time.Since(delivery.PublishedAt) > notificationAlertDeliveryTTL {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}

	occurrence, err := d.core.notificationOccurrences.Get(ctx, job.GetRecipientId(), job.GetNotificationId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if occurrence.GetSourceEventId() != job.GetSourceEventId() {
		return nil
	}
	eligible, err := d.core.NotificationAlertEligible(ctx, occurrence)
	if err != nil {
		return err
	}
	if !eligible {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}
	if d.core.suppressesNotificationAlertsForPresence(ctx, job.GetRecipientId()) {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}
	if d.core.notificationAlertHandler == nil {
		return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
	}
	if err := d.core.notificationAlertHandler(ctx, occurrence); err != nil {
		if errors.Is(err, ErrNotificationAlertSuppressed) {
			return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_SILENCED)
		}
		return err
	}
	return d.core.notificationOccurrences.completeAlertDelivery(ctx, &job, corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_DELIVERED)
}

// NotificationAlertEligible fences notification materialization and policy,
// then checks the exact unread occurrence and current target visibility. Push
// transports call it again immediately before contacting their provider.
func (c *ChattoCore) NotificationAlertEligible(ctx context.Context, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil {
		return false, nil
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
	roomID := occurrence.GetTarget().GetRoomId()
	for _, match := range occurrence.GetReasons() {
		if d.core.GetEffectiveNotificationIntensity(occurrence.GetRecipientId(), roomID, match.GetReason()) == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT {
			return true
		}
	}
	return false
}
