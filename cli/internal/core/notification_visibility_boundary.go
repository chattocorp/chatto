package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/jetstreamutil"
)

const notificationVisibilityKeyPrefix = "notification_visibility_boundary."
const maxNotificationStateWriteRetries = 8

func notificationVisibilityBoundaryKey(userID, roomID string) string {
	return notificationVisibilityKeyPrefix + userID + "." + roomID
}

func notificationVisibilityBoundaryFilter(userID string) string {
	return notificationVisibilityKeyPrefix + userID + ".*"
}

func (m *NotificationMaterializer) recordVisibilityBoundary(ctx context.Context, userID, roomID string, sequence uint64) error {
	if userID == "" || roomID == "" || sequence == 0 {
		return nil
	}
	key := notificationVisibilityBoundaryKey(userID, roomID)
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, sequence)
	for attempt := 0; attempt < maxNotificationStateWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if revision, err := m.core.storage.runtimeStateKV.Create(ctx, key, value, jetstream.KeyTTL(notificationTTL)); err == nil {
				if err := m.core.notificationBoundaries.waitForRevision(ctx, key, revision); err != nil {
					return err
				}
				return nil
			} else if !jetstreamutil.IsSequenceConflict(err) {
				return fmt.Errorf("create notification visibility boundary: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read notification visibility boundary: %w", err)
		}
		if len(entry.Value()) != 8 {
			return fmt.Errorf("notification visibility boundary has invalid length %d", len(entry.Value()))
		}
		if binary.BigEndian.Uint64(entry.Value()) >= sequence {
			return m.core.notificationBoundaries.waitForRevision(ctx, key, entry.Revision())
		}
		if revision, err := m.core.updateRuntimeStateWithTTL(ctx, key, value, entry.Revision(), notificationTTL); err == nil {
			if err := m.core.notificationBoundaries.waitForRevision(ctx, key, revision); err != nil {
				return err
			}
			return nil
		} else if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("update notification visibility boundary: %w", err)
		}
	}
	return fmt.Errorf("write notification visibility boundary after %d attempts", maxNotificationStateWriteRetries)
}

func (m *NotificationMaterializer) sourceAfterVisibilityBoundary(ctx context.Context, userID, roomID string, sequence uint64) (bool, error) {
	if sequence == 0 {
		return false, nil
	}
	boundary, exists, err := m.core.notificationBoundaries.visibilityBoundary(ctx, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("read notification visibility boundary: %w", err)
	}
	if !exists {
		return true, nil
	}
	return sequence > boundary, nil
}

func (m *NotificationMaterializer) purgeVisibilityBoundaries(ctx context.Context, userID string) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, notificationVisibilityBoundaryFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list notification visibility boundaries: %w", err)
	}
	for key := range lister.Keys() {
		if err := m.deleteRuntimeStateKey(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (m *NotificationMaterializer) deleteRuntimeStateKey(ctx context.Context, key string) error {
	for attempt := 0; attempt < maxNotificationStateWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read runtime-state key for deletion: %w", err)
		}
		err = m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		if err == nil || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("delete runtime-state key: %w", err)
		}
	}
	return fmt.Errorf("delete runtime-state key after %d attempts", maxNotificationStateWriteRetries)
}
