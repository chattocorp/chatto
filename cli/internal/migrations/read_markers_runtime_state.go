package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	legacyRoomReadPrefix     = "room_read_event."
	legacyThreadOpenedPrefix = "thread_last_opened."
	legacyThreadFollowPrefix = "thread_follow."
)

// MigrateReadMarkersToRuntimeState copies legacy read cursors from
// SERVER_RUNTIME into RUNTIME_STATE.
//
// Legacy keys:
//   - room_read_event.{userId}.{roomId} -> read.room.{userId}.{roomId}
//   - thread_last_opened.{userId}.{roomId}.{threadRootEventId}
//     -> read.thread.{userId}.{roomId}.{threadRootEventId}
//
// The old keys are intentionally retained for rollback and phase-7 cleanup.
// Destination writes use Create so this migration never overwrites newer
// RUNTIME_STATE markers written by a previous boot or a live process.
func MigrateReadMarkersToRuntimeState(ctx context.Context, legacyRuntime, runtimeState jetstream.KeyValue, logger *log.Logger) error {
	roomCopied, roomSkipped, err := copyRuntimeStatePrefix(ctx, legacyRuntime, runtimeState, legacyRoomReadPrefix, "read.room.")
	if err != nil {
		return fmt.Errorf("room read markers: %w", err)
	}
	threadCopied, threadSkipped, err := copyRuntimeStatePrefix(ctx, legacyRuntime, runtimeState, legacyThreadOpenedPrefix, "read.thread.")
	if err != nil {
		return fmt.Errorf("thread read markers: %w", err)
	}

	if roomCopied+roomSkipped+threadCopied+threadSkipped > 0 {
		logger.Info("read marker migration: copied legacy markers into RUNTIME_STATE",
			"room_copied", roomCopied,
			"room_skipped", roomSkipped,
			"thread_copied", threadCopied,
			"thread_skipped", threadSkipped)
	}
	return nil
}

// MigrateThreadFollowsToRuntimeState copies legacy thread follow state from
// SERVER_RUNTIME into RUNTIME_STATE.
//
// Legacy and new keys use the same shape:
//
//	thread_follow.{userId}.{roomId}.{threadRootEventId}
//
// The old keys are intentionally retained for rollback and phase-7 cleanup.
// Destination writes use Create so this migration never overwrites newer
// RUNTIME_STATE follows written by a previous boot or a live process.
func MigrateThreadFollowsToRuntimeState(ctx context.Context, legacyRuntime, runtimeState jetstream.KeyValue, logger *log.Logger) error {
	copied, skipped, err := copyRuntimeStatePrefix(ctx, legacyRuntime, runtimeState, legacyThreadFollowPrefix, legacyThreadFollowPrefix)
	if err != nil {
		return fmt.Errorf("thread follows: %w", err)
	}
	if copied+skipped > 0 {
		logger.Info("thread follow migration: copied legacy follows into RUNTIME_STATE",
			"copied", copied,
			"skipped", skipped)
	}
	return nil
}

func copyRuntimeStatePrefix(ctx context.Context, src, dst jetstream.KeyValue, oldPrefix, newPrefix string) (copied, skipped int, err error) {
	lister, err := src.ListKeysFiltered(ctx, oldPrefix+">")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for key := range lister.Keys() {
		entry, err := src.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return copied, skipped, fmt.Errorf("get %s: %w", key, err)
		}

		suffix := strings.TrimPrefix(key, oldPrefix)
		if suffix == key || suffix == "" {
			skipped++
			continue
		}
		if _, err := dst.Create(ctx, newPrefix+suffix, entry.Value()); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				skipped++
				continue
			}
			return copied, skipped, fmt.Errorf("create %s%s: %w", newPrefix, suffix, err)
		}
		copied++
	}
	return copied, skipped, nil
}
