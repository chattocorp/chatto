package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const notificationReadBoundaryKeyPrefix = "notification_read_boundary."

type notificationReadBoundary struct {
	targetSequence   uint64
	observedSequence uint64
}

func notificationReadBoundaryKey(userID, roomID, threadRootEventID string) string {
	key := notificationReadBoundaryKeyPrefix + userID + "." + roomID
	if threadRootEventID != "" {
		key += "." + threadRootEventID
	}
	return key
}

func notificationReadBoundaryFilter(userID string) string {
	return notificationReadBoundaryKeyPrefix + userID + ".>"
}

func encodeNotificationReadBoundary(boundary notificationReadBoundary) []byte {
	value := make([]byte, 16)
	binary.BigEndian.PutUint64(value[:8], boundary.targetSequence)
	binary.BigEndian.PutUint64(value[8:], boundary.observedSequence)
	return value
}

func decodeNotificationReadBoundary(value []byte) (notificationReadBoundary, error) {
	if len(value) != 16 {
		return notificationReadBoundary{}, fmt.Errorf("notification read boundary has invalid length %d", len(value))
	}
	return notificationReadBoundary{
		targetSequence:   binary.BigEndian.Uint64(value[:8]),
		observedSequence: binary.BigEndian.Uint64(value[8:]),
	}, nil
}

// recordNotificationReadBoundary durably records both the timeline item the
// user read through and the EVT horizon visible when they performed the read.
// The second coordinate lets a later read cover reactions to an older message
// without incorrectly treating reactions that arrive afterwards as read.
func (m *NotificationOccurrenceModel) recordNotificationReadBoundary(ctx context.Context, userID, roomID, threadRootEventID, targetEventID string) (notificationReadBoundary, error) {
	entry, ok := m.core.roomModel.timelineEntry(targetEventID)
	if !ok || entry.Event == nil || roomIDOfEvent(entry.Event) != roomID {
		return notificationReadBoundary{}, ErrNotFound
	}
	// Reactions are coverable only through the local reaction projection's
	// applied horizon, not merely because a newer fact exists in EVT but has not
	// yet become observable to this read operation.
	next := notificationReadBoundary{
		targetSequence:   entry.StreamSeq,
		observedSequence: m.core.roomModel.reactions.Projector().Status().LastSeq,
	}
	if next.observedSequence < next.targetSequence {
		next.observedSequence = next.targetSequence
	}
	key := notificationReadBoundaryKey(userID, roomID, threadRootEventID)
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		current, err := m.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if _, err := m.kv.Create(ctx, key, encodeNotificationReadBoundary(next), jetstream.KeyTTL(notificationTTL)); err == nil {
				return next, nil
			} else if !jetstreamutil.IsSequenceConflict(err) {
				return notificationReadBoundary{}, fmt.Errorf("create notification read boundary: %w", err)
			}
			continue
		}
		if err != nil {
			return notificationReadBoundary{}, fmt.Errorf("read notification read boundary: %w", err)
		}
		previous, err := decodeNotificationReadBoundary(current.Value())
		if err != nil {
			return notificationReadBoundary{}, err
		}
		if previous.targetSequence > next.targetSequence {
			next.targetSequence = previous.targetSequence
		}
		if previous.observedSequence > next.observedSequence {
			next.observedSequence = previous.observedSequence
		}
		if previous == next {
			return next, nil
		}
		if _, err := m.core.updateRuntimeStateTokenTTL(ctx, key, encodeNotificationReadBoundary(next), current.Revision(), notificationTTL); err == nil {
			return next, nil
		} else if !jetstreamutil.IsSequenceConflict(err) {
			return notificationReadBoundary{}, fmt.Errorf("update notification read boundary: %w", err)
		}
	}
	return notificationReadBoundary{}, fmt.Errorf("write notification read boundary after %d attempts", maxNotificationUpdateRetries)
}

func (m *NotificationOccurrenceModel) notificationReadBoundary(ctx context.Context, userID, roomID, threadRootEventID string) (notificationReadBoundary, bool, error) {
	entry, err := m.kv.Get(ctx, notificationReadBoundaryKey(userID, roomID, threadRootEventID))
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return notificationReadBoundary{}, false, nil
	}
	if err != nil {
		return notificationReadBoundary{}, false, fmt.Errorf("read notification read boundary: %w", err)
	}
	boundary, err := decodeNotificationReadBoundary(entry.Value())
	return boundary, err == nil, err
}

func (m *NotificationOccurrenceModel) occurrenceCoveredByReadBoundary(ctx context.Context, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil || occurrence.GetSourceStreamSequence() == 0 || occurrence.GetTarget() == nil {
		return false, nil
	}
	target := occurrence.GetTarget()
	boundary, exists, err := m.notificationReadBoundary(ctx, occurrence.GetRecipientId(), target.GetRoomId(), target.GetThreadRootEventId())
	if err != nil || !exists {
		return false, err
	}
	if notificationOccurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REACTION) {
		targetEntry, ok := m.core.roomModel.timelineEntry(target.GetEventId())
		return ok && targetEntry.StreamSeq <= boundary.targetSequence && occurrence.GetSourceStreamSequence() <= boundary.observedSequence, nil
	}
	return occurrence.GetSourceStreamSequence() <= boundary.targetSequence, nil
}

func (m *NotificationOccurrenceModel) purgeNotificationReadBoundaries(ctx context.Context, userID string) error {
	lister, err := m.kv.ListKeysFiltered(ctx, notificationReadBoundaryFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list notification read boundaries: %w", err)
	}
	for key := range lister.Keys() {
		if err := m.core.notificationMaterializer.deleteRuntimeStateKey(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
