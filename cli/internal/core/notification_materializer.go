package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const notificationMaterializerPollEvery = 250 * time.Millisecond

// NotificationMaterializer recovers occurrence creation and lifecycle effects
// from durable notification plans, revocations, and source lifecycle facts.
// Every replica may run it: recipient/source KV identity makes overlapping
// replay safe.
type NotificationMaterializer struct {
	core      *ChattoCore
	consumer  *evtstream.IncrementalEffectConsumer
	pollEvery time.Duration
}

func NewNotificationMaterializer(core *ChattoCore) *NotificationMaterializer {
	materializer := &NotificationMaterializer{core: core, pollEvery: notificationMaterializerPollEvery}
	materializer.consumer = evtstream.NewOrderedIncrementalEffectConsumer(
		core.EventPublisher,
		evtstream.EventSubjectFilter(),
		materializer.MaterializeEvent,
	)
	return materializer
}

func (m *NotificationMaterializer) Run(ctx context.Context) error {
	// Full replay waits for current room/account projections so notification
	// recovery can enforce current visibility alongside source-event ordering.
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("wait for projections before notification replay: %w", err)
	}
	if err := m.core.notificationOccurrences.WaitReady(ctx); err != nil {
		return fmt.Errorf("wait for notification index before replay: %w", err)
	}
	ticker := time.NewTicker(m.pollEvery)
	defer ticker.Stop()
	for {
		if err := m.consumer.Consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			m.core.logger.Warn("Notification materialization pass incomplete", "error", err)
		}
		m.deliverPendingAlerts(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
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

func (m *NotificationMaterializer) MaterializeEvent(ctx context.Context, event *corev1.Event) error {
	if event == nil {
		return nil
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_NotificationOccurrencePlanned:
		plan := payload.NotificationOccurrencePlanned
		if plan == nil || len(plan.GetRecipients()) == 0 || plan.GetTarget() == nil {
			return nil
		}
		switch plan.GetSourceKind() {
		case corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_MESSAGE:
			// Independent effect retries may run after a later retraction. Consult
			// current monotonic target state so delayed creation cannot resurrect it.
			if _, retracted, known := m.core.roomModel.latestBody(plan.GetSourceEventId()); known && retracted {
				return nil
			}
		case corev1.NotificationSourceKind_NOTIFICATION_SOURCE_KIND_REACTION:
			// A removed and later re-added reaction has a different source event.
			// Only materialize while this exact add is still projected as active.
			target := plan.GetTarget()
			snapshot := m.core.roomModel.reactionMutationSnapshot(target.GetRoomId(), target.GetEventId(), plan.GetReactionEmoji(), event.GetActorId())
			if !snapshot.Exists || snapshot.SourceEventID != plan.GetSourceEventId() {
				return nil
			}
		default:
			return nil
		}
		// Projection catch-up is complete before replay starts. A missing room is
		// therefore a terminal historical condition (normally a later RoomDeleted
		// fact), not a retryable materialization failure that should poison the
		// globally ordered effect queue.
		if _, err := m.core.FindRoomByID(ctx, plan.GetTarget().GetRoomId()); errors.Is(err, ErrNotFound) {
			return nil
		} else if err != nil {
			return fmt.Errorf("verify notification room: %w", err)
		}
		for _, decision := range plan.GetRecipients() {
			active, err := m.activeRecipient(ctx, decision.GetRecipientId())
			if err != nil {
				return fmt.Errorf("verify notification recipient %s: %w", decision.GetRecipientId(), err)
			}
			if !active {
				continue
			}
			_, _, err = m.core.notificationOccurrences.Create(ctx, CreateNotificationOccurrenceInput{
				RecipientID:   decision.GetRecipientId(),
				SourceEventID: plan.GetSourceEventId(),
				SourceCreated: event.GetCreatedAt().AsTime(),
				ActorID:       event.GetActorId(),
				Target:        plan.GetTarget(),
				Reasons:       decision.GetReasons(),
				EvaluatedAt:   event.GetCreatedAt().AsTime(),
			})
			if err != nil {
				return fmt.Errorf("create occurrence for recipient %s: %w", decision.GetRecipientId(), err)
			}
		}
	case *corev1.Event_NotificationOccurrenceRevoked:
		revocation := payload.NotificationOccurrenceRevoked
		if revocation == nil || revocation.GetRecipientId() == "" || revocation.GetSourceEventId() == "" {
			return nil
		}
		_, err := m.core.notificationOccurrences.RemoveSource(ctx, revocation.GetRecipientId(), revocation.GetSourceEventId(), revocation.GetReason())
		return err
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

func (m *NotificationMaterializer) activeRecipient(ctx context.Context, userID string) (bool, error) {
	_, err := m.core.GetUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
