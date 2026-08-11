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
	for attempt := 0; attempt < maxNotificationWorkWriteRetries; attempt++ {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			if _, err := m.core.storage.runtimeStateKV.Create(ctx, key, value, jetstream.KeyTTL(notificationTTL)); err == nil {
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
			return nil
		}
		if _, err := m.core.updateRuntimeStateTokenTTL(ctx, key, value, entry.Revision(), notificationTTL); err == nil {
			return nil
		} else if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("update notification visibility boundary: %w", err)
		}
	}
	return fmt.Errorf("write notification visibility boundary after %d attempts", maxNotificationWorkWriteRetries)
}

func (m *NotificationMaterializer) sourceAfterVisibilityBoundary(ctx context.Context, userID, roomID string, sequence uint64) (bool, error) {
	if sequence == 0 {
		return false, nil
	}
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, notificationVisibilityBoundaryKey(userID, roomID))
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read notification visibility boundary: %w", err)
	}
	if len(entry.Value()) != 8 {
		return false, fmt.Errorf("notification visibility boundary has invalid length %d", len(entry.Value()))
	}
	return sequence > binary.BigEndian.Uint64(entry.Value()), nil
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
