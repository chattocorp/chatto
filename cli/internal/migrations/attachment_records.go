package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// BackfillAttachmentRecords populates standalone Attachment metadata
// records in `SERVER_BODIES` so the asset HTTP handler can authorize
// downloads by attachment ID. Two sources are walked:
//
//  1. Embedded `Attachment` protos inside each `MessageBody` in
//     SERVER_BODIES (covers user-posted attachments).
//  2. Variant + thumbnail attachment IDs referenced by each
//     `VideoProcessingState` in SERVER_RUNTIME (covers the transcoded
//     outputs the video service uploads on the side, which never appear
//     in any MessageBody and therefore aren't reachable from pass 1).
//
// # Why
//
// Attachment metadata used to live exclusively inside `MessageBody`
// records, which meant the only way to answer "what room does this
// attachment belong to?" was to scan every body. The asset HTTP handler
// previously avoided the scan by trusting an unauthenticated URL on the
// S3 fast path — a real authorization bug. This migration plus the new
// `attachment.{roomId}.{attachmentId}` records gives the handler an
// O(1) lookup.
//
// Pass 2 fixes a regression in the original migration: video variants
// and thumbnails uploaded by the video service are never embedded in a
// MessageBody, so they had no records after the v1 migration and were
// served as 404 by the new auth gate.
//
// # Layout
//
// Both record kinds live in `SERVER_BODIES`. Their key shapes don't
// overlap:
//
//	{userId}.{bodyId}                   → MessageBody  (existing)
//	attachment.{roomId}.{attachmentId}  → Attachment   (this migration)
//
// # Idempotency
//
// Safe to re-run. Every record is written via Put, so re-running on
// an already-populated bucket is a series of no-op overwrites with
// identical values. A sentinel key (`attachment_records.backfilled.v2`)
// in SERVER_RUNTIME short-circuits repeat boots. The v2 suffix forces
// instances that already ran the v1 pass to re-run and pick up the
// video-variant records.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration. Operators can verify by inspecting
// SERVER_RUNTIME for the sentinel key.
func BackfillAttachmentRecords(ctx context.Context, bodiesKV, runtimeKV jetstream.KeyValue, logger *log.Logger) error {
	const flagKey = "attachment_records.backfilled.v2"

	if entry, err := runtimeKV.Get(ctx, flagKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get backfill flag: %w", err)
	}

	// Pass 1: scan message bodies, write records for embedded
	// attachments. Returns a map of attachmentID → roomID so pass 2
	// can resolve the room for a video's variant/thumbnail attachments
	// (which only know about themselves via their original's ID).
	roomByAttachmentID, bodiesScanned, indexed, err := backfillFromBodies(ctx, bodiesKV, logger)
	if err != nil {
		return err
	}

	// Pass 2: scan video processing state, write records for variant
	// and thumbnail attachments. Returns a count of additional records
	// for logging.
	videoIndexed, err := backfillFromVideoState(ctx, bodiesKV, runtimeKV, roomByAttachmentID, logger)
	if err != nil {
		return err
	}

	if _, err := runtimeKV.Put(ctx, flagKey, []byte("1")); err != nil {
		return fmt.Errorf("set backfill flag: %w", err)
	}

	if indexed > 0 || videoIndexed > 0 || bodiesScanned > 0 {
		logger.Info("attachment_records migration: indexed attachment metadata records",
			"bodies_scanned", bodiesScanned,
			"attachments_indexed", indexed,
			"video_outputs_indexed", videoIndexed)
	}
	return nil
}

// backfillFromBodies walks SERVER_BODIES and writes a metadata record
// for every attachment embedded in a MessageBody. Returns the
// attachmentID → roomID map (used by pass 2) plus counters.
func backfillFromBodies(
	ctx context.Context,
	bodiesKV jetstream.KeyValue,
	logger *log.Logger,
) (roomByAttachmentID map[string]string, bodiesScanned, indexed int, err error) {
	roomByAttachmentID = map[string]string{}

	lister, err := bodiesKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return roomByAttachmentID, 0, 0, nil
		}
		return nil, 0, 0, fmt.Errorf("list message body keys: %w", err)
	}

	// Collect keys first so writes don't reshape the iterator's view of
	// the bucket. Also lets us pre-filter to body-shape keys
	// (`{userId}.{bodyId}` — two segments) and ignore the attachment
	// records we may be writing in this very pass.
	var bodyKeys []string
	for key := range lister.Keys() {
		if strings.HasPrefix(key, "attachment.") {
			continue
		}
		bodyKeys = append(bodyKeys, key)
	}

	for _, key := range bodyKeys {
		entry, err := bodiesKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, 0, 0, fmt.Errorf("get message body %s: %w", key, err)
		}

		var body corev1.MessageBody
		if err := proto.Unmarshal(entry.Value(), &body); err != nil {
			logger.Warn("attachment_records: skipping unparseable message body",
				"key", key, "error", err)
			continue
		}

		for _, att := range body.Attachments {
			if att == nil || att.Id == "" || att.RoomId == "" {
				continue
			}
			roomByAttachmentID[att.Id] = att.RoomId
			recordKey := "attachment." + att.RoomId + "." + att.Id
			marshaled, err := proto.Marshal(att)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("marshal attachment record for %s: %w", att.Id, err)
			}
			if _, err := bodiesKV.Put(ctx, recordKey, marshaled); err != nil {
				return nil, 0, 0, fmt.Errorf("write attachment record %s: %w", recordKey, err)
			}
			indexed++
		}
	}

	return roomByAttachmentID, len(bodyKeys), indexed, nil
}

// backfillFromVideoState walks SERVER_RUNTIME `video.*` keys and
// writes minimal Attachment records for every variant and thumbnail
// attachment referenced by a VideoProcessingState. The room ID is
// inherited from the original video attachment (which the body pass
// already indexed). Existing video pipelines uploaded variants and
// thumbnails via UploadAttachment without ever embedding them in a
// MessageBody, so they have no other on-disk source we can use.
func backfillFromVideoState(
	ctx context.Context,
	bodiesKV, runtimeKV jetstream.KeyValue,
	roomByAttachmentID map[string]string,
	logger *log.Logger,
) (int, error) {
	lister, err := runtimeKV.ListKeysFiltered(ctx, "video.*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("list video state keys: %w", err)
	}

	var videoKeys []string
	for k := range lister.Keys() {
		videoKeys = append(videoKeys, k)
	}

	indexed := 0
	for _, key := range videoKeys {
		entry, err := runtimeKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return 0, fmt.Errorf("get video state %s: %w", key, err)
		}

		var state corev1.VideoProcessingState
		if err := proto.Unmarshal(entry.Value(), &state); err != nil {
			logger.Warn("attachment_records: skipping unparseable video state",
				"key", key, "error", err)
			continue
		}

		// The key shape is `video.{originalAttachmentID}` — pull the
		// original ID back out so we can ask the body-pass map what
		// room it lived in.
		originalID := strings.TrimPrefix(key, "video.")
		roomID := roomByAttachmentID[originalID]
		if roomID == "" {
			// Original wasn't found in any body — most likely GDPR-
			// deleted. Without a room we can't authorize the outputs;
			// they remain unreachable. Acceptable in this degenerate
			// case (the user content driving access is gone too).
			logger.Warn("attachment_records: no room found for video outputs",
				"original_attachment_id", originalID)
			continue
		}

		referenced := make([]string, 0, len(state.Variants)+1)
		if state.ThumbnailAttachmentId != "" {
			referenced = append(referenced, state.ThumbnailAttachmentId)
		}
		for _, v := range state.Variants {
			if v != nil && v.AttachmentId != "" {
				referenced = append(referenced, v.AttachmentId)
			}
		}

		for _, attID := range referenced {
			// A minimal record is enough to gate authorization: the
			// HTTP handler only reads `RoomId`. Storage / filename /
			// dimensions stay zero-valued for these legacy outputs;
			// new uploads going forward write the full proto via
			// UploadAttachment, so the gap is bounded in time.
			rec := &corev1.Attachment{Id: attID, RoomId: roomID}
			marshaled, err := proto.Marshal(rec)
			if err != nil {
				return 0, fmt.Errorf("marshal video output record for %s: %w", attID, err)
			}
			recordKey := "attachment." + roomID + "." + attID
			if _, err := bodiesKV.Put(ctx, recordKey, marshaled); err != nil {
				return 0, fmt.Errorf("write video output record %s: %w", recordKey, err)
			}
			indexed++
		}
	}

	return indexed, nil
}
