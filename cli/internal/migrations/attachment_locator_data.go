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

// BackfillAttachmentLocatorData embeds full `Attachment` protos into
// `VideoProcessingState.ThumbnailAttachment` and `Variants[i].Attachment`
// on instances that were running before the signed-attachment-locator
// URL scheme landed.
//
// # Why
//
// The locator URL handler dispatches variant/thumbnail lookups to
// `VideoProcessingState`, which on pre-locator instances only carried
// string attachment IDs (no `Storage` info, no filename, no dimensions).
// This migration copies the full `Attachment` protos in from the
// pre-existing standalone `attachment.{roomId}.{attachmentId}` records
// in SERVER_BODIES (originally written by the `BackfillAttachmentRecords`
// migration in a prior release; the migration itself has been deleted,
// but the records it produced are still on disk and serve as the data
// source for this pass). After it runs, VPS is self-contained and the
// standalone records are no longer load-bearing — `DropLegacyAttachmentRecords`
// then sweeps them.
//
// Body attachments do not need a migration: the URL resolver patches
// `MessageBody.Attachments[i].MessageBodyId` on read for legacy bodies
// that predate the field, and `PostMessage` stamps it for new ones.
//
// # Idempotency
//
// Safe to re-run. Each rewrite is a Put with the same content if the
// data is already present. A sentinel
// (`attachment_locator_data.backfilled`) in SERVER_RUNTIME short-circuits
// repeat boots.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration. Operators can verify by inspecting the
// sentinel key in SERVER_RUNTIME.
func BackfillAttachmentLocatorData(
	ctx context.Context,
	bodiesKV, runtimeKV jetstream.KeyValue,
	logger *log.Logger,
) error {
	const flagKey = "attachment_locator_data.backfilled"

	if entry, err := runtimeKV.Get(ctx, flagKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get backfill flag: %w", err)
	}

	if err := backfillVideoProcessingAttachments(ctx, bodiesKV, runtimeKV, logger); err != nil {
		return fmt.Errorf("video processing state: %w", err)
	}

	if _, err := runtimeKV.Put(ctx, flagKey, []byte("1")); err != nil {
		return fmt.Errorf("set backfill flag: %w", err)
	}
	return nil
}

// backfillVideoProcessingAttachments walks every VideoProcessingState
// and embeds the full Attachment proto for the thumbnail and each
// variant by reading from the pre-existing standalone
// `attachment.{roomId}.{attachmentId}` records (populated by an earlier
// migration). Skips entries that already have all attachments embedded.
func backfillVideoProcessingAttachments(
	ctx context.Context,
	bodiesKV, runtimeKV jetstream.KeyValue,
	logger *log.Logger,
) error {
	lister, err := runtimeKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list runtime keys: %w", err)
	}

	var vpsKeys []string
	for key := range lister.Keys() {
		if strings.HasPrefix(key, "video.") {
			vpsKeys = append(vpsKeys, key)
		}
	}

	embedded := 0
	for _, key := range vpsKeys {
		entry, err := runtimeKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("get vps %s: %w", key, err)
		}

		var state corev1.VideoProcessingState
		if err := proto.Unmarshal(entry.Value(), &state); err != nil {
			logger.Warn("attachment_locator_data: skipping unparseable VPS",
				"key", key, "error", err)
			continue
		}

		needsRewrite := false
		if state.ThumbnailAttachmentId != "" && state.ThumbnailAttachment == nil {
			if att := lookupAttachmentRecord(ctx, bodiesKV, state.ThumbnailAttachmentId, logger); att != nil {
				state.ThumbnailAttachment = att
				needsRewrite = true
			}
		}
		for _, v := range state.Variants {
			if v == nil || v.AttachmentId == "" || v.Attachment != nil {
				continue
			}
			if att := lookupAttachmentRecord(ctx, bodiesKV, v.AttachmentId, logger); att != nil {
				v.Attachment = att
				needsRewrite = true
			}
		}

		if !needsRewrite {
			continue
		}

		newData, err := proto.Marshal(&state)
		if err != nil {
			return fmt.Errorf("marshal updated vps %s: %w", key, err)
		}
		if _, err := runtimeKV.Update(ctx, key, newData, entry.Revision()); err != nil {
			logger.Warn("attachment_locator_data: skipping vps that changed under us",
				"key", key, "error", err)
			continue
		}
		embedded++
	}

	if embedded > 0 {
		logger.Info("attachment_locator_data: embedded attachment protos in VideoProcessingState entries",
			"entries_rewritten", embedded)
	}
	return nil
}

// lookupAttachmentRecord finds the standalone `attachment.*.{attachmentId}`
// record (written by an earlier migration) and returns the embedded
// Attachment proto. Returns nil if the record isn't found or can't be
// parsed — the caller falls back to leaving the field unembedded.
func lookupAttachmentRecord(ctx context.Context, bodiesKV jetstream.KeyValue, attachmentID string, logger *log.Logger) *corev1.Attachment {
	lister, err := bodiesKV.ListKeysFiltered(ctx, "attachment.*."+attachmentID)
	if err != nil {
		if !errors.Is(err, jetstream.ErrNoKeysFound) {
			logger.Warn("attachment_locator_data: failed to filter for attachment record",
				"attachment_id", attachmentID, "error", err)
		}
		return nil
	}
	for key := range lister.Keys() {
		entry, err := bodiesKV.Get(ctx, key)
		if err != nil {
			continue
		}
		var att corev1.Attachment
		if err := proto.Unmarshal(entry.Value(), &att); err != nil {
			logger.Warn("attachment_locator_data: unparseable attachment record",
				"key", key, "error", err)
			continue
		}
		return &att
	}
	return nil
}
