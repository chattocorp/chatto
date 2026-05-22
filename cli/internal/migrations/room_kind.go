package migrations

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// BackfillRoomKind populates the new Room.kind enum on existing room
// records that predate the field. Rooms are stored in SERVER_CONFIG
// under keys "room.channel.{roomId}" and "room.dm.{roomId}", and the
// kind segment of the key is the authoritative source for backfill.
//
// # Why
//
// ADR-030 Phase 2 introduced Room.kind as the canonical kind
// discriminator. Pre-existing rooms still have the field zero-valued
// (ROOM_KIND_UNSPECIFIED). Backfilling at boot lets all read sites
// assume `room.Kind != UNSPECIFIED` instead of falling back to
// `KindForSpace(room.SpaceId)` on every access.
//
// # Idempotency
//
// Safe to re-run. Rooms with `Kind` already set are skipped without a
// write. A crash mid-migration just leaves unmigrated rooms to be
// picked up on the next boot.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND the wire-compat fallback in
// `KindOfRoom` has been removed.
func BackfillRoomKind(ctx context.Context, configKV jetstream.KeyValue, logger *log.Logger) error {
	keyLister, err := configKV.ListKeysFiltered(ctx, "room.channel.*", "room.dm.*")
	if err != nil {
		// "no keys match" is the steady-state empty case, not a failure.
		return nil
	}

	converted := 0
	for key := range keyLister.Keys() {
		// Derive the wanted kind from the key prefix; that's the
		// authoritative source even when the proto field is unset.
		var wantKind corev1.RoomKind
		switch {
		case strings.HasPrefix(key, "room.channel."):
			wantKind = corev1.RoomKind_ROOM_KIND_CHANNEL
		case strings.HasPrefix(key, "room.dm."):
			wantKind = corev1.RoomKind_ROOM_KIND_DM
		default:
			continue
		}

		entry, err := configKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			logger.Warn("room_kind backfill: get failed", "key", key, "error", err)
			continue
		}

		var room corev1.Room
		if err := proto.Unmarshal(entry.Value(), &room); err != nil {
			logger.Warn("room_kind backfill: unmarshal failed", "key", key, "error", err)
			continue
		}

		if room.Kind == wantKind {
			continue
		}

		room.Kind = wantKind
		data, err := proto.Marshal(&room)
		if err != nil {
			logger.Warn("room_kind backfill: marshal failed", "key", key, "error", err)
			continue
		}

		if _, err := configKV.Update(ctx, key, data, entry.Revision()); err != nil {
			// Another writer raced us; the new writer will have set Kind
			// itself, so this room is fine — move on.
			logger.Debug("room_kind backfill: update raced, skipping", "key", key, "error", err)
			continue
		}
		converted++
	}

	if converted > 0 {
		logger.Info("room_kind backfill: populated Room.kind on legacy records", "count", converted)
	}
	return nil
}
