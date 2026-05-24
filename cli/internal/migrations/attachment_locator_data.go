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

// BackfillAttachmentLocatorData populates the fields the
// signed-attachment-locator URL scheme depends on for instances that
// were running before the locator change landed:
//
//  1. `MessageBody.Attachments[i].MessageBodyId` — the body's KV key,
//     stamped onto each embedded attachment so the GraphQL URL resolver
//     can build a locator without parent-context plumbing. Bodies are
//     unmarshaled, scanned, and re-marshaled only when at least one
//     attachment is missing the field.
//
//  2. `VideoProcessingState.ThumbnailAttachment` and `Variants[i].Attachment` —
//     full Attachment protos embedded into the VPS so it becomes the
//     self-contained source of truth for variant/thumbnail metadata.
//     The full protos are pulled from the pre-existing standalone
//     `attachment.{roomId}.{attachmentId}` records in SERVER_BODIES
//     (populated by `BackfillAttachmentRecords` in a prior release).
//
// # Why
//
// The signed-attachment-locator scheme replaces the per-attachment
// SERVER_BODIES index with a URL-encoded payload. The HTTP handler
// no longer scans a record bucket; it decodes the locator, authorizes
// against the room ID in the payload, then fetches the source proto
// from MessageBody (for body attachments) or VideoProcessingState (for
// variants/thumbnails). That requires both source-of-truth records to
// carry enough data — hence the backfill.
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

	if err := backfillBodyAttachmentBodyIDs(ctx, bodiesKV, logger); err != nil {
		return fmt.Errorf("body attachments: %w", err)
	}
	if err := backfillVideoProcessingAttachments(ctx, bodiesKV, runtimeKV, logger); err != nil {
		return fmt.Errorf("video processing state: %w", err)
	}

	if _, err := runtimeKV.Put(ctx, flagKey, []byte("1")); err != nil {
		return fmt.Errorf("set backfill flag: %w", err)
	}
	return nil
}

// backfillBodyAttachmentBodyIDs walks every MessageBody and stamps each
// embedded attachment's `message_body_id` field with the body's KV key
// where it isn't already set. Skips bodies that already have all
// attachments stamped (zero rewrites in steady state).
func backfillBodyAttachmentBodyIDs(ctx context.Context, bodiesKV jetstream.KeyValue, logger *log.Logger) error {
	lister, err := bodiesKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list keys: %w", err)
	}

	var bodyKeys []string
	for key := range lister.Keys() {
		// Skip the legacy attachment.* index keys; we only want MessageBody entries.
		if strings.HasPrefix(key, "attachment.") {
			continue
		}
		bodyKeys = append(bodyKeys, key)
	}

	stamped := 0
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
			logger.Warn("attachment_locator_data: skipping unparseable message body",
				"key", key, "error", err)
			continue
		}

		needsRewrite := false
		for _, att := range body.Attachments {
			if att != nil && att.MessageBodyId == "" {
				att.MessageBodyId = key
				needsRewrite = true
			}
		}
		if !needsRewrite {
			continue
		}

		newData, err := proto.Marshal(&body)
		if err != nil {
			return fmt.Errorf("marshal updated body %s: %w", key, err)
		}
		if _, err := bodiesKV.Update(ctx, key, newData, entry.Revision()); err != nil {
			// On concurrent write we just skip — next boot's run will pick it up if still needed.
			logger.Warn("attachment_locator_data: skipping body that changed under us",
				"key", key, "error", err)
			continue
		}
		stamped++
	}

	if stamped > 0 {
		logger.Info("attachment_locator_data: stamped message_body_id on body attachments",
			"bodies_rewritten", stamped)
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
