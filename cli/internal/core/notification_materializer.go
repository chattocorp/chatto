package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	notificationMaterializerPollEvery = 250 * time.Millisecond
	notificationWorkerConsumerName    = "chatto-notification-materializer-v2"
	notificationWorkerAckWait         = time.Minute
	notificationWorkerHeartbeat       = 15 * time.Second
	notificationWorkerRetryDelay      = 10 * time.Second
	notificationWorkerAckTimeout      = 5 * time.Second
	// Notification lifecycle is causal: one shared in-flight delivery keeps a
	// later leave/retraction/removal behind the source it supersedes, including
	// when several Chatto replicas share the consumer.
	notificationWorkerMaxPending    = 1
	notificationWorkKeyPrefix       = "notification_work."
	maxNotificationWorkWriteRetries = 8
)

// NotificationMaterializer consumes existing domain facts and applies their
// notification effects. Exact source-time recipient decisions are staged as
// short-lived RUNTIME_STATE work records before the source fact commits; EVT
// contains no notification-only planning events.
type NotificationMaterializer struct {
	core      *ChattoCore
	pollEvery time.Duration
}

func NewNotificationMaterializer(core *ChattoCore) *NotificationMaterializer {
	return &NotificationMaterializer{core: core, pollEvery: notificationMaterializerPollEvery}
}

func (m *NotificationMaterializer) Run(ctx context.Context) error {
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("wait for projections before notification worker: %w", err)
	}
	if err := m.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification index before worker: %w", err)
	}

	consumer, err := m.createConsumer(ctx)
	if err != nil {
		return err
	}
	worker, err := events.NewDurableWorker(
		consumer,
		m.processDelivery,
		events.DurableWorkerOptions{
			MaxConcurrent:     notificationWorkerMaxPending,
			FetchMaxWait:      time.Second,
			RetryDelay:        notificationWorkerRetryDelay,
			AckTimeout:        notificationWorkerAckTimeout,
			HeartbeatInterval: notificationWorkerHeartbeat,
			Logger:            m.core.logger.WithPrefix("NotificationWorker"),
		},
	)
	if err != nil {
		return fmt.Errorf("configure notification worker: %w", err)
	}

	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(ctx) }()

	ticker := time.NewTicker(m.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case err := <-workerDone:
			if err == nil && ctx.Err() == nil {
				return errors.New("notification worker stopped unexpectedly")
			}
			return err
		case <-ctx.Done():
			return <-workerDone
		case <-ticker.C:
			m.deliverPendingAlerts(ctx)
		}
	}
}

func (m *NotificationMaterializer) createConsumer(ctx context.Context) (jetstream.Consumer, error) {
	consumer, err := m.core.storage.serverEvtStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          notificationWorkerConsumerName,
		Durable:       notificationWorkerConsumerName,
		Description:   "Shared durable queue for Chatto notification materialization",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       notificationWorkerAckWait,
		MaxDeliver:    -1,
		FilterSubjects: []string{
			evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted),
			evtstream.RoomEventTypeFilter(evtstream.EventReactionAdded),
			evtstream.RoomEventTypeFilter(evtstream.EventReactionRemoved),
			evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
			evtstream.RoomEventTypeFilter(evtstream.EventUserLeftRoom),
			evtstream.RoomEventTypeFilter(evtstream.EventRoomMemberRemoved),
			evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
			evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
		},
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   notificationWorkerMaxPending,
		MaxRequestBatch: notificationWorkerMaxPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create notification materializer consumer: %w", err)
	}
	return consumer, nil
}

func (m *NotificationMaterializer) processDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		m.core.logger.Error("Terminating malformed notification delivery", "error", err)
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	position := events.SubjectPosition(delivery.Subject, delivery.StreamSequence)
	if event.GetUserAccountDeleted() != nil {
		if err := m.core.userModel.waitForUsers(ctx, position); err != nil {
			return fmt.Errorf("wait for user projection: %w", err)
		}
	} else if err := m.core.roomModel.waitForLiveEVTEvent(ctx, position, &event); err != nil {
		return fmt.Errorf("wait for room projections: %w", err)
	}
	return m.MaterializeEvent(ctx, &event)
}

// StoreWork writes exact prepared occurrences before their triggering domain
// event is appended. Orphans from a failed append expire at the same absolute
// 90-day boundary as the occurrence they would have created.
func (m *NotificationMaterializer) StoreWork(ctx context.Context, trigger *corev1.Event, work []*corev1.NotificationOccurrence) error {
	if trigger == nil || trigger.GetId() == "" || trigger.GetCreatedAt() == nil || len(work) == 0 {
		return nil
	}
	ttl := trigger.GetCreatedAt().AsTime().UTC().Add(notificationTTL).Sub(time.Now().UTC())
	if ttl <= 0 {
		return nil
	}
	for _, occurrence := range work {
		if occurrence == nil || occurrence.GetRecipientId() == "" || occurrence.GetSourceEventId() == "" {
			return invalidArgument("notification work requires recipient_id and source_event_id")
		}
		data, err := proto.Marshal(occurrence)
		if err != nil {
			return fmt.Errorf("marshal notification work: %w", err)
		}
		if err := m.putWorkWithTTL(ctx, notificationWorkKey(trigger.GetId(), occurrence.GetRecipientId()), data, ttl); err != nil {
			return err
		}
	}
	return nil
}

func (m *NotificationMaterializer) putWorkWithTTL(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	for attempt := 0; attempt < maxNotificationWorkWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if _, err := m.core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(ttl)); err == nil {
				return nil
			} else if !jetstreamutil.IsSequenceConflict(err) {
				return fmt.Errorf("create notification work: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read notification work: %w", err)
		}
		_, err = m.core.js.Publish(
			ctx,
			"$KV.RUNTIME_STATE."+key,
			data,
			jetstream.WithExpectLastSequencePerSubject(entry.Revision()),
			jetstream.WithMsgTTL(ttl),
		)
		if err == nil {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("update notification work: %w", err)
		}
	}
	return fmt.Errorf("write notification work after %d attempts", maxNotificationWorkWriteRetries)
}

func notificationWorkKey(triggerEventID, recipientID string) string {
	return notificationWorkKeyPrefix + triggerEventID + "." + recipientID
}

func notificationWorkFilter(triggerEventID string) string {
	return notificationWorkKeyPrefix + triggerEventID + ".*"
}

func (m *NotificationMaterializer) deliverPendingAlerts(ctx context.Context) {
	if m.core.OnNotificationOccurrenceCreated == nil {
		return
	}
	for {
		occurrence, claimed, err := m.core.notificationOccurrences.ClaimPendingAlert(ctx)
		if err != nil {
			m.core.logger.Warn("Failed to claim notification alert", "error", err)
			return
		}
		if !claimed {
			return
		}
		deliveryErr := m.core.OnNotificationOccurrenceCreated(context.WithoutCancel(ctx), occurrence)
		if err := m.core.notificationOccurrences.CompleteAlertClaim(ctx, occurrence, deliveryErr == nil); err != nil {
			m.core.logger.Warn("Failed to complete notification alert claim", "notification_id", occurrence.GetId(), "error", err)
			return
		}
		if deliveryErr != nil {
			m.core.logger.Warn("Notification alert delivery failed", "notification_id", occurrence.GetId(), "error", deliveryErr)
			return
		}
	}
}

// MaterializeEvent applies work for one triggering event, or a lifecycle
// cleanup directly derivable from the event itself. It is safe to call on the
// request path and again from the durable worker.
func (m *NotificationMaterializer) MaterializeEvent(ctx context.Context, event *corev1.Event) error {
	if event == nil {
		return nil
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted, *corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved:
		return m.materializeWork(ctx, event)
	case *corev1.Event_MessageRetracted:
		_, err := m.core.notificationOccurrences.RemoveTarget(ctx, payload.MessageRetracted.GetRoomId(), payload.MessageRetracted.GetEventId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_TARGET_RETRACTED)
		return err
	case *corev1.Event_UserLeftRoom:
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, event.GetActorId(), payload.UserLeftRoom.GetRoomId(), event.GetCreatedAt().AsTime(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_RoomMemberRemoved:
		_, err := m.core.notificationOccurrences.RemoveRoomForUser(ctx, payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId(), event.GetCreatedAt().AsTime(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_RoomDeleted:
		_, err := m.core.notificationOccurrences.RemoveRoom(ctx, payload.RoomDeleted.GetRoomId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return err
	case *corev1.Event_UserAccountDeleted:
		_, err := m.core.notificationOccurrences.PurgeUser(ctx, payload.UserAccountDeleted.GetUserId())
		return err
	}
	return nil
}

func (m *NotificationMaterializer) materializeWork(ctx context.Context, event *corev1.Event) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, notificationWorkFilter(event.GetId()))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list notification work: %w", err)
	}
	for key := range lister.Keys() {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read notification work: %w", err)
		}
		var occurrence corev1.NotificationOccurrence
		if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
			m.core.logger.Error("Discarding invalid notification work", "key", key, "error", err)
			if deleteErr := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); deleteErr != nil &&
				!errors.Is(deleteErr, jetstream.ErrKeyNotFound) && !errors.Is(deleteErr, jetstream.ErrKeyDeleted) {
				return fmt.Errorf("delete invalid notification work: %w", deleteErr)
			}
			continue
		}
		if err := m.materializeOccurrence(ctx, event, &occurrence); err != nil {
			return err
		}
		if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil &&
			!errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
			return fmt.Errorf("delete notification work: %w", err)
		}
	}
	return nil
}

func (m *NotificationMaterializer) materializeOccurrence(ctx context.Context, event *corev1.Event, occurrence *corev1.NotificationOccurrence) error {
	if occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		_, err := m.core.notificationOccurrences.RemoveSource(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId(), occurrence.GetRemovalReason())
		return err
	}
	if occurrence.GetTarget() == nil || len(occurrence.GetReasons()) == 0 {
		return nil
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		if _, retracted, known := m.core.roomModel.latestBody(event.GetId()); known && retracted {
			return nil
		}
	case *corev1.Event_ReactionAdded:
		reaction := payload.ReactionAdded
		snapshot := m.core.roomModel.reactionMutationSnapshot(reaction.GetRoomId(), reaction.GetMessageEventId(), reaction.GetEmoji(), event.GetActorId())
		if !snapshot.Exists || snapshot.SourceEventID != event.GetId() {
			return nil
		}
	default:
		return nil
	}
	visible, err := m.activeVisibleRecipient(ctx, occurrence.GetRecipientId(), occurrence.GetTarget().GetRoomId())
	if err != nil {
		return err
	}
	if !visible {
		return nil
	}
	_, _, err = m.core.notificationOccurrences.Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:   occurrence.GetRecipientId(),
		SourceEventID: occurrence.GetSourceEventId(),
		SourceCreated: occurrence.GetSourceCreatedAt().AsTime(),
		ActorID:       occurrence.GetActorId(),
		Target:        occurrence.GetTarget(),
		Reasons:       occurrence.GetReasons(),
		EvaluatedAt:   occurrence.GetEvaluatedAt().AsTime(),
	})
	if err != nil {
		return fmt.Errorf("create occurrence for recipient %s: %w", occurrence.GetRecipientId(), err)
	}
	return nil
}

func (m *NotificationMaterializer) activeVisibleRecipient(ctx context.Context, userID, roomID string) (bool, error) {
	active, err := m.activeRecipient(ctx, userID)
	if err != nil || !active {
		return active, err
	}
	room, err := m.core.FindRoomByID(ctx, roomID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify notification room: %w", err)
	}
	member, err := m.core.RoomMembershipExists(ctx, KindOfRoom(room), userID, roomID)
	if err != nil {
		return false, fmt.Errorf("verify notification room membership: %w", err)
	}
	return member, nil
}

func (m *NotificationMaterializer) activeRecipient(ctx context.Context, userID string) (bool, error) {
	_, err := m.core.GetUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
