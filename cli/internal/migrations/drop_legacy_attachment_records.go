package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

// DropLegacyAttachmentRecords deletes the standalone
// `attachment.{roomId}.{attachmentId}` entries that older versions of
// Chatto wrote into `SERVER_BODIES`.
//
// # Why
//
// A prior `BackfillAttachmentRecords` migration (since deleted) wrote
// a copy of every `Attachment` proto into a per-attachment record in
// `SERVER_BODIES` so the asset HTTP handler could do an O(1) by-ID
// lookup. The signed
// attachment locator URL scheme (ADR-032) made that redundant: the URL
// itself carries the room ID and source-of-truth pointer, so the
// handler resolves attachments via the owning `MessageBody` or
// `VideoProcessingState` directly. The previous PR stopped reading
// and writing these records; `BackfillAttachmentLocatorData` copies
// the variant/thumbnail data they held into `VideoProcessingState`.
//
// After this migration runs, the records are no longer referenced
// by anything and can be dropped wholesale.
//
// # Ordering
//
// Must run *after* `BackfillAttachmentLocatorData` (which reads these
// records to populate `VideoProcessingState` entries). `RunAll`
// enforces the ordering.
//
// # Idempotency
//
// Safe to re-run. The sentinel
// (`legacy_attachment_records.dropped`) in SERVER_RUNTIME short-circuits
// repeat boots. Even without the sentinel, deletion of an already-deleted
// key is a silent no-op via `ErrKeyNotFound`.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration. The sentinel makes the migration cheap
// to re-run, so there is no urgency.
func DropLegacyAttachmentRecords(
	ctx context.Context,
	bodiesKV, runtimeKV jetstream.KeyValue,
	logger *log.Logger,
) error {
	const flagKey = "legacy_attachment_records.dropped"

	if entry, err := runtimeKV.Get(ctx, flagKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get sentinel: %w", err)
	}

	lister, err := bodiesKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			if _, putErr := runtimeKV.Put(ctx, flagKey, []byte("1")); putErr != nil {
				return fmt.Errorf("set sentinel on empty bucket: %w", putErr)
			}
			return nil
		}
		return fmt.Errorf("list keys: %w", err)
	}

	var recordKeys []string
	for key := range lister.Keys() {
		if strings.HasPrefix(key, "attachment.") {
			recordKeys = append(recordKeys, key)
		}
	}

	deleted := 0
	for _, key := range recordKeys {
		if err := bodiesKV.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("delete %s: %w", key, err)
		}
		deleted++
	}

	if _, err := runtimeKV.Put(ctx, flagKey, []byte("1")); err != nil {
		return fmt.Errorf("set sentinel: %w", err)
	}

	if deleted > 0 {
		logger.Info("legacy_attachment_records: swept legacy standalone records",
			"deleted", deleted)
	}
	return nil
}
