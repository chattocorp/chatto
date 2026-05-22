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

// BackfillAttachmentRecords promotes every Attachment proto embedded
// in a MessageBody into a standalone metadata record so the asset HTTP
// handler can authorize downloads by attachment ID without scanning
// every body.
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
// # Layout
//
// Both record kinds live in `SERVER_BODIES`. Their key shapes don't
// overlap:
//
//	{userId}.{bodyId}                   → MessageBody  (existing)
//	attachment.{roomId}.{attachmentId}  → Attachment   (new, this migration)
//
// # Idempotency
//
// Safe to re-run. Every record is written via Put, so re-running on
// an already-populated bucket is a series of no-op overwrites with
// identical values. A sentinel key (`attachment_records.backfilled`)
// in SERVER_RUNTIME short-circuits repeat boots.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration. Operators can verify by inspecting
// SERVER_RUNTIME for the sentinel key.
func BackfillAttachmentRecords(ctx context.Context, bodiesKV, runtimeKV jetstream.KeyValue, logger *log.Logger) error {
	const flagKey = "attachment_records.backfilled"

	if entry, err := runtimeKV.Get(ctx, flagKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get backfill flag: %w", err)
	}

	lister, err := bodiesKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			if _, putErr := runtimeKV.Put(ctx, flagKey, []byte("1")); putErr != nil {
				return fmt.Errorf("set backfill flag on empty bucket: %w", putErr)
			}
			return nil
		}
		return fmt.Errorf("list message body keys: %w", err)
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

	indexed := 0
	for _, key := range bodyKeys {
		entry, err := bodiesKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("get message body %s: %w", key, err)
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
			recordKey := "attachment." + att.RoomId + "." + att.Id
			marshaled, err := proto.Marshal(att)
			if err != nil {
				return fmt.Errorf("marshal attachment record for %s: %w", att.Id, err)
			}
			if _, err := bodiesKV.Put(ctx, recordKey, marshaled); err != nil {
				return fmt.Errorf("write attachment record %s: %w", recordKey, err)
			}
			indexed++
		}
	}

	if _, err := runtimeKV.Put(ctx, flagKey, []byte("1")); err != nil {
		return fmt.Errorf("set backfill flag: %w", err)
	}

	if indexed > 0 || len(bodyKeys) > 0 {
		logger.Info("attachment_records migration: indexed attachments from message bodies",
			"bodies_scanned", len(bodyKeys), "attachments_indexed", indexed)
	}
	return nil
}
