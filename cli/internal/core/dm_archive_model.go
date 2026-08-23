package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxDMArchiveMarkerRetries = 5

// DMArchive returns the operation-level model for viewer-specific direct
// message archive state.
func (c *ChattoCore) DMArchive() *DMArchiveModel {
	return c.dmArchiveModel
}

// DMArchiveModel owns viewer-specific DM archive mutations and effective-state
// reads. Its marker stores the latest root-message event ID observed at archive
// time; a later root message therefore makes the marker stale and the DM
// visible again without a write on the message path.
type DMArchiveModel struct {
	core  *ChattoCore
	index *DMArchiveIndex
}

func (m *DMArchiveModel) Run(ctx context.Context) error {
	return m.index.Run(ctx)
}

func (m *DMArchiveModel) WaitReady(ctx context.Context) error {
	return m.index.WaitReady(ctx)
}

func (m *DMArchiveModel) Resync(ctx context.Context) error {
	return m.index.Resync(ctx)
}

func (m *DMArchiveModel) Fence(ctx context.Context) (uint64, error) {
	return m.index.fence(ctx)
}

func (m *DMArchiveModel) RoomIDsChangedAfter(ctx context.Context, userID string, fence uint64) ([]string, error) {
	return m.index.roomIDsChangedAfter(ctx, userID, fence)
}

// IsArchived reports effective viewer state. A missing or stale marker is
// unarchived; only a marker equal to the current latest root-message event ID
// hides the conversation from archive-aware navigation.
func (m *DMArchiveModel) IsArchived(ctx context.Context, userID, roomID string) (bool, error) {
	entry, exists, err := m.index.marker(ctx, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("read DM archive marker: %w", err)
	}
	if !exists {
		return false, nil
	}
	latestEventID, _, hasLatest, err := m.core.GetRoomLastEvent(ctx, KindDM, roomID)
	if err != nil {
		return false, err
	}
	return hasLatest && latestEventID != "" && string(entry.value) == latestEventID, nil
}

// Archive records the latest root-message event ID for one DM viewer. Empty
// DMs have no conversation row to archive and are rejected.
func (m *DMArchiveModel) Archive(ctx context.Context, actorID, roomID string) error {
	room, kind, err := m.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if kind != KindDM {
		return invalidArgument("only direct-message conversations can be archived")
	}
	latestEventID, _, exists, err := m.core.GetRoomLastEvent(ctx, kind, room.GetId())
	if err != nil {
		return err
	}
	if !exists || latestEventID == "" {
		return invalidArgument("a direct-message conversation needs a message before it can be archived")
	}

	key := dmArchiveMarkerKey(actorID, room.GetId())
	for attempt := 0; attempt < maxDMArchiveMarkerRetries; attempt++ {
		entry, markerExists, err := m.index.marker(ctx, actorID, room.GetId())
		if err != nil {
			return fmt.Errorf("read DM archive marker: %w", err)
		}
		if markerExists && string(entry.value) == latestEventID {
			return nil
		}

		var revision uint64
		if markerExists {
			revision, err = m.core.storage.runtimeStateKV.Update(ctx, key, []byte(latestEventID), entry.revision)
		} else {
			revision, err = m.core.storage.runtimeStateKV.Create(ctx, key, []byte(latestEventID))
		}
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := m.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return fmt.Errorf("wait for conflicting DM archive marker: %w", waitErr)
				}
				continue
			}
			return fmt.Errorf("write DM archive marker: %w", err)
		}
		if err := m.index.waitForRevision(ctx, key, revision); err != nil {
			return fmt.Errorf("wait for DM archive marker: %w", err)
		}
		m.notifyChanged(ctx, actorID, room.GetId())
		return nil
	}
	return fmt.Errorf("DM archive marker update failed after %d retries", maxDMArchiveMarkerRetries)
}

// Unarchive deletes the viewer marker. It is idempotent, including when the
// marker is already stale because a later root message arrived.
func (m *DMArchiveModel) Unarchive(ctx context.Context, actorID, roomID string) error {
	room, kind, err := m.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if kind != KindDM {
		return invalidArgument("only direct-message conversations can be unarchived")
	}

	key := dmArchiveMarkerKey(actorID, room.GetId())
	for attempt := 0; attempt < maxDMArchiveMarkerRetries; attempt++ {
		entry, exists, err := m.index.marker(ctx, actorID, room.GetId())
		if err != nil {
			return fmt.Errorf("read DM archive marker: %w", err)
		}
		if !exists {
			return nil
		}
		err = m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.revision))
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return nil
			}
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := m.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return fmt.Errorf("wait for conflicting DM archive marker: %w", waitErr)
				}
				continue
			}
			return fmt.Errorf("delete DM archive marker: %w", err)
		}
		if err := m.index.waitForRevisionAfter(ctx, key, entry.revision); err != nil {
			return fmt.Errorf("wait for deleted DM archive marker: %w", err)
		}
		m.notifyChanged(ctx, actorID, room.GetId())
		return nil
	}
	return fmt.Errorf("DM archive marker deletion failed after %d retries", maxDMArchiveMarkerRetries)
}

// DeleteUserMarkers removes all viewer-owned DM archive state during account
// deletion. It is idempotent and revision-aware so a concurrent writer cannot
// be silently erased on a stale revision.
func (m *DMArchiveModel) DeleteUserMarkers(ctx context.Context, userID string) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, dmArchiveMarkerUserFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list user DM archive markers: %w", err)
	}
	for key := range lister.Keys() {
		for attempt := 0; attempt < maxDMArchiveMarkerRetries; attempt++ {
			entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				break
			}
			if err != nil {
				return fmt.Errorf("read user DM archive marker: %w", err)
			}
			err = m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
			if err == nil || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				break
			}
			if !jetstreamutil.IsSequenceConflict(err) {
				return fmt.Errorf("delete user DM archive marker: %w", err)
			}
			if attempt == maxDMArchiveMarkerRetries-1 {
				return fmt.Errorf("delete user DM archive marker after %d retries", maxDMArchiveMarkerRetries)
			}
		}
	}
	return nil
}

func (m *DMArchiveModel) notifyChanged(ctx context.Context, userID, roomID string) {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_DmArchiveChanged{
			DmArchiveChanged: &corev1.DMArchiveChangedEvent{RoomId: roomID},
		},
	})
	if err := m.core.publishLiveEvent(ctx, subjects.LiveSyncUserEvent(userID, "dm_archive"), event); err != nil {
		m.core.logger.Warn("Failed to publish DM archive change", "user_id", userID, "room_id", roomID, "error", err)
	}
}
