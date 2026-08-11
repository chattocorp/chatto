package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func notificationOccurrenceFilter(userID string) string {
	if userID == "" {
		return notificationOccurrenceWatchFilter
	}
	return notificationOccurrenceKeyPrefix + userID + ".*"
}

// storedOccurrenceEntries reads the authoritative KV state. It is reserved
// for cross-replica handshakes and causally ordered lifecycle cleanup; normal
// hot list/count reads continue to use the process-wide watcher index.
func (m *NotificationOccurrenceModel) storedOccurrenceEntries(ctx context.Context, userID string) ([]notificationOccurrenceIndexEntry, error) {
	lister, err := m.kv.ListKeysFiltered(ctx, notificationOccurrenceFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list notification occurrences: %w", err)
	}
	entries := make([]notificationOccurrenceIndexEntry, 0)
	for key := range lister.Keys() {
		if _, _, ok := parseNotificationOccurrenceKey(key); !ok {
			// The watcher prefix also contains internal ordering markers such as
			// the cross-replica read fence. They are not occurrence records.
			continue
		}
		entry, err := m.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read notification occurrence %s: %w", key, err)
		}
		var occurrence corev1.NotificationOccurrence
		if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
			return nil, fmt.Errorf("decode notification occurrence %s: %w", key, err)
		}
		if userID != "" && occurrence.GetRecipientId() != userID {
			return nil, fmt.Errorf("notification occurrence %s has mismatched recipient", key)
		}
		entries = append(entries, notificationOccurrenceIndexEntry{
			key:        key,
			revision:   entry.Revision(),
			occurrence: &occurrence,
		})
	}
	return entries, nil
}

func (m *NotificationOccurrenceModel) storedOccurrenceBySource(ctx context.Context, userID, sourceEventID string) (notificationOccurrenceIndexEntry, bool, error) {
	key := notificationOccurrenceKey(userID, sourceEventID)
	entry, err := m.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return notificationOccurrenceIndexEntry{}, false, nil
	}
	if err != nil {
		return notificationOccurrenceIndexEntry{}, false, fmt.Errorf("read notification occurrence: %w", err)
	}
	var occurrence corev1.NotificationOccurrence
	if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
		return notificationOccurrenceIndexEntry{}, false, fmt.Errorf("decode notification occurrence: %w", err)
	}
	if occurrence.GetRecipientId() != userID || occurrence.GetSourceEventId() != sourceEventID {
		return notificationOccurrenceIndexEntry{}, false, fmt.Errorf("notification occurrence key does not match payload")
	}
	return notificationOccurrenceIndexEntry{key: key, revision: entry.Revision(), occurrence: &occurrence}, true, nil
}

func (m *NotificationOccurrenceModel) deleteStoredOccurrence(ctx context.Context, userID, sourceEventID string, reason corev1.NotificationRemovalReason) (*corev1.NotificationOccurrence, bool, error) {
	if reason == corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		reason = corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED
	}
	for attempt := 0; attempt < maxNotificationUpdateRetries; attempt++ {
		entry, exists, err := m.storedOccurrenceBySource(ctx, userID, sourceEventID)
		if err != nil || !exists {
			return nil, false, err
		}
		if entry.occurrence.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
			return entry.occurrence, false, nil
		}
		now := m.now().UTC()
		tombstone := &corev1.NotificationOccurrence{
			Id:              entry.occurrence.GetId(),
			RecipientId:     entry.occurrence.GetRecipientId(),
			SourceEventId:   entry.occurrence.GetSourceEventId(),
			SourceCreatedAt: entry.occurrence.GetSourceCreatedAt(),
			InboxState:      corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE,
			UpdatedAt:       timestamppb.New(now),
			ExpiresAt:       entry.occurrence.GetExpiresAt(),
			RemovalReason:   reason,
			RemovedAt:       timestamppb.New(now),
			AlertState:      corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_NOT_APPLICABLE,
		}
		written, err := m.updateAtRevision(ctx, entry, tombstone)
		if jetstreamutil.IsSequenceConflict(err) {
			continue
		}
		return written, err == nil, err
	}
	return nil, false, fmt.Errorf("notification occurrence delete failed after %d retries", maxNotificationUpdateRetries)
}
