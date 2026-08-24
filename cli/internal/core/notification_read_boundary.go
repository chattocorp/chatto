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
	for attempt := 0; attempt < maxNotificationStateWriteRetries; attempt++ {
		current, err := m.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if revision, err := m.kv.Create(ctx, key, encodeNotificationReadBoundary(next), jetstream.KeyTTL(notificationTTL)); err == nil {
				if err := m.core.notificationBoundaries.waitForRevision(ctx, key, revision); err != nil {
					return notificationReadBoundary{}, err
				}
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
			if err := m.core.notificationBoundaries.waitForRevision(ctx, key, current.Revision()); err != nil {
				return notificationReadBoundary{}, err
			}
			return next, nil
		}
		if revision, err := m.core.updateRuntimeStateWithTTL(ctx, key, encodeNotificationReadBoundary(next), current.Revision(), notificationTTL); err == nil {
			if err := m.core.notificationBoundaries.waitForRevision(ctx, key, revision); err != nil {
				return notificationReadBoundary{}, err
			}
			return next, nil
		} else if !jetstreamutil.IsSequenceConflict(err) {
			return notificationReadBoundary{}, fmt.Errorf("update notification read boundary: %w", err)
		}
	}
	return notificationReadBoundary{}, fmt.Errorf("write notification read boundary after %d attempts", maxNotificationStateWriteRetries)
}

func (m *NotificationOccurrenceModel) notificationReadBoundary(ctx context.Context, userID, roomID, threadRootEventID string) (notificationReadBoundary, bool, error) {
	return m.core.notificationBoundaries.readBoundary(ctx, userID, roomID, threadRootEventID)
}

func (m *NotificationOccurrenceModel) occurrenceCoveredByReadBoundary(ctx context.Context, occurrence *corev1.NotificationOccurrence) (bool, error) {
	if occurrence == nil {
		return false, nil
	}
	message := notificationSignalMessage(occurrence.GetSignal())
	if occurrence.GetSourceStreamSequence() == 0 || message == nil {
		return false, nil
	}
	boundary, exists, err := m.notificationReadBoundary(ctx, occurrence.GetRecipientId(), message.GetRoomId(), message.GetThreadRootEventId())
	if err != nil || !exists {
		return false, err
	}
	return m.occurrenceCoveredByBoundary(occurrence, boundary), nil
}

func (m *NotificationOccurrenceModel) occurrenceCoveredByBoundary(occurrence *corev1.NotificationOccurrence, boundary notificationReadBoundary) bool {
	if occurrence == nil || occurrence.GetSourceStreamSequence() == 0 {
		return false
	}
	message := notificationSignalMessage(occurrence.GetSignal())
	if message == nil {
		return false
	}
	if occurrence.GetSignal().GetReactionReceived() != nil {
		targetEntry, ok := m.core.roomModel.timelineEntry(message.GetEventId())
		return ok && targetEntry.StreamSeq <= boundary.targetSequence && occurrence.GetSourceStreamSequence() <= boundary.observedSequence
	}
	return occurrence.GetSourceStreamSequence() <= boundary.targetSequence
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
