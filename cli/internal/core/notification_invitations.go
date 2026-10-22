package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A separate capability-specific consumer preserves the existing message queue
// and prevents older replicas from acknowledging invitation notification work.
const invitationNotificationConsumerName = "chatto-invitation-notifications-v1"

func invitationNotificationSubjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventRoomInvitationCreated),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomInvitationAccepted),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomInvitationDeclined),
	}
}

func (m *NotificationMaterializer) createInvitationConsumer(ctx context.Context) (jetstream.Consumer, error) {
	return evtstream.CreateEffectConsumer(ctx, m.core.storage.serverEvtStream, evtstream.EffectConsumerConfig{
		Name: invitationNotificationConsumerName, Description: "Room invitation notification effects",
		FilterSubjects: invitationNotificationSubjects(), AckWait: notificationWorkerAckWait,
		MaxAckPending: 1, DeliverPolicy: jetstream.DeliverNewPolicy,
	})
}

func (m *NotificationMaterializer) waitInvitationCurrent(ctx context.Context) error {
	var boundary uint64
	for _, subject := range invitationNotificationSubjects() {
		position, err := m.core.EventPublisher.LastSubjectPosition(ctx, subject)
		if err != nil {
			return err
		}
		boundary = max(boundary, position.Seq)
	}
	if boundary == 0 {
		return nil
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.invitationConsumerInfoMu.Lock()
		info, err := m.invitationConsumer.Info(ctx)
		m.invitationConsumerInfoMu.Unlock()
		if err != nil {
			return err
		}
		if info.AckFloor.Stream >= boundary || (info.NumAckPending == 0 && info.Delivered.Stream >= boundary) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *NotificationMaterializer) processInvitationDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var source evtv1.Event
	if err := proto.Unmarshal(delivery.Data, &source); err != nil {
		return events.TerminateDelivery("invalid invitation notification source", err)
	}
	if err := m.core.roomInvitations.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	if err := m.core.waitForCurrentNotificationPolicy(ctx); err != nil {
		return err
	}
	var id, recipientID, roomID string
	switch payload := source.GetEvent().(type) {
	case *evtv1.Event_RoomInvitationCreated:
		id, recipientID, roomID = payload.RoomInvitationCreated.GetInvitationId(), payload.RoomInvitationCreated.GetRecipientId(), payload.RoomInvitationCreated.GetRoomId()
	case *evtv1.Event_RoomInvitationAccepted:
		id, recipientID, roomID = payload.RoomInvitationAccepted.GetInvitationId(), payload.RoomInvitationAccepted.GetRecipientId(), payload.RoomInvitationAccepted.GetRoomId()
	case *evtv1.Event_RoomInvitationDeclined:
		id, recipientID, roomID = payload.RoomInvitationDeclined.GetInvitationId(), payload.RoomInvitationDeclined.GetRecipientId(), payload.RoomInvitationDeclined.GetRoomId()
	default:
		return errUnsupportedNotificationEvent
	}
	if id == "" || recipientID == "" || roomID == "" {
		return events.TerminateDelivery("invalid invitation notification coordinates", errors.New("missing required identifier"))
	}
	if source.GetRoomInvitationCreated() == nil {
		_, err := m.core.notificationOccurrences.removeMatching(ctx, func(occurrence *notificationv1.NotificationOccurrence) bool {
			return occurrence.GetRecipientId() == recipientID && occurrence.GetSignal().GetRoomInvitationReceived().GetInvitationId() == id
		})
		if err != nil {
			return err
		}
	} else {
		invitation, ok := m.core.roomInvitations.projection.Projection().get(id)
		if ok && invitation.pending(m.core.notificationOccurrences.now()) {
			signal := &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomInvitationReceived{RoomInvitationReceived: &notificationv1.RoomInvitationReceived{
				RoomId: roomID, InvitationId: id, ExpiresAt: timestamppb.New(invitation.ExpiresAt),
			}}}
			// Reuse the authoritative read-time visibility fence before creating any
			// attention. Later resolution is checked again on API reads and push delivery.
			candidate := &notificationv1.NotificationOccurrence{RecipientId: recipientID, Signal: signal}
			visible, err := m.core.notificationOccurrences.VisibleOccurrences(ctx, recipientID, []*notificationv1.NotificationOccurrence{candidate})
			if err != nil {
				return err
			}
			if len(visible) != 0 {
				mode := m.core.GetEffectiveNotificationModeForSignal(recipientID, roomID, signal)
				if err := m.core.notificationOccurrences.CreateMany(ctx, []CreateNotificationOccurrenceInput{{
					RecipientID: recipientID, SourceEventID: source.GetId(), SourceCreated: source.GetCreatedAt().AsTime(),
					ActorID: source.GetActorId(), Signal: signal, Mode: mode, SourceStreamSequence: delivery.StreamSequence,
					AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
				}}); err != nil {
					return fmt.Errorf("materialize invitation notification: %w", err)
				}
			}
		}
	}
	m.core.NotifyNotificationUnreadStateChanged(ctx, recipientID, source.GetActorId(), roomID, "")
	return nil
}

// invitationVisible reads current domain projections; the caller owns the
// projection catch-up and content-view barrier. No membership is granted here.
func (m *NotificationOccurrenceModel) invitationVisible(ctx context.Context, recipientID string, signal *notificationv1.RoomInvitationReceived) (bool, error) {
	invitation, ok := m.core.roomInvitations.projection.Projection().get(signal.GetInvitationId())
	if !ok || invitation.RecipientID != recipientID || invitation.RoomID != signal.GetRoomId() || !invitation.pending(m.now()) {
		return false, nil
	}
	member, err := m.core.RoomMembershipExists(ctx, KindChannel, recipientID, invitation.RoomID)
	if err != nil || member {
		return false, err
	}
	err = m.core.roomInvitations.eligible(ctx, invitation.SenderID, recipientID, invitation.RoomID)
	if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrRoomArchived) || errors.Is(err, ErrInvalidArgument) {
		return false, nil
	}
	return err == nil, err
}
