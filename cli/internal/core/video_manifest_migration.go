package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const videoManifestESMigrationKey = "video_manifest_es.migrated"

type legacyVideoAttachmentRef struct {
	roomID     string
	attachment *corev1.Attachment
}

// migrateVideoManifestsToES imports legacy SERVER_RUNTIME video.{attachment}
// processing state into durable EVT manifest events. It deliberately leaves the
// old KV keys in place for rollback, using a sentinel to avoid duplicate imports.
func (c *ChattoCore) migrateVideoManifestsToES(ctx context.Context) (retErr error) {
	if err := c.migrateAssetCreationsToES(ctx); err != nil {
		return fmt.Errorf("migrate asset creations: %w", err)
	}
	revision, claimed, err := c.claimRuntimeMigration(ctx, videoManifestESMigrationKey)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	defer func() {
		if retErr != nil {
			c.releaseRuntimeMigrationOnFailure(videoManifestESMigrationKey, revision, retErr)
		}
	}()

	legacyKeys, err := c.listLegacyVideoStateKeys(ctx)
	if err != nil {
		return err
	}

	refs, err := c.indexVideoAttachmentsFromEVT(ctx)
	if err != nil {
		return err
	}
	existing, err := c.indexImportedVideoManifestEvents(ctx)
	if err != nil {
		return err
	}

	imported := 0
	legacyAttachmentIDs := make(map[string]bool, len(legacyKeys))
	for _, key := range legacyKeys {
		attachmentID := strings.TrimPrefix(key, "video.")
		legacyAttachmentIDs[attachmentID] = true
		if existing[attachmentID] {
			continue
		}
		ref := refs[attachmentID]
		if ref == nil || ref.attachment == nil {
			c.logger.Warn("video manifest ES migration: skipping legacy video state with no owning message", "key", key)
			continue
		}

		entry, err := c.storage.serverRuntimeKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("get legacy video state %s: %w", key, err)
		}
		var state corev1.VideoProcessingState
		if err := proto.Unmarshal(entry.Value(), &state); err != nil {
			c.logger.Warn("video manifest ES migration: skipping unparseable legacy state", "key", key, "error", err)
			continue
		}

		sourceStatus := c.attachmentBinaryStatus(ctx, ref.attachment)
		// Only emit a durable SOURCE_MISSING outcome when storage definitively
		// confirms the source binary is gone. Unknown status (S3 unreachable,
		// missing client config, transient errors) is treated as "can't tell"
		// and falls through to skip — a later boot in a properly-configured
		// environment can fill in the manifest.
		sourceDefinitelyMissing := sourceStatus == AttachmentBinaryMissing
		switch state.Status {
		case corev1.VideoStatus_VIDEO_STATUS_COMPLETED:
			thumbnail := c.usableAttachment(ctx, state.ThumbnailAttachment)
			var variants []*corev1.VideoVariant
			for _, variant := range state.Variants {
				if variant == nil || variant.Attachment == nil {
					continue
				}
				if c.attachmentBinaryStatus(ctx, variant.Attachment) != AttachmentBinaryPresent {
					continue
				}
				variants = append(variants, proto.Clone(variant).(*corev1.VideoVariant))
			}
			if len(variants) == 0 {
				if sourceDefinitelyMissing {
					if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
						return err
					}
					imported++
				}
				continue
			}
			if err := c.appendVideoProcessedMigrationEvent(ctx, ref, attachmentID, &state, thumbnail, variants); err != nil {
				return err
			}
			imported++
		case corev1.VideoStatus_VIDEO_STATUS_FAILED:
			failureCode := corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED
			if sourceDefinitelyMissing {
				failureCode = corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING
			}
			if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, failureCode); err != nil {
				return err
			}
			imported++
		case corev1.VideoStatus_VIDEO_STATUS_PENDING, corev1.VideoStatus_VIDEO_STATUS_PROCESSING:
			if sourceDefinitelyMissing {
				if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
					return err
				}
				imported++
			}
		default:
			if sourceDefinitelyMissing {
				if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
					return err
				}
				imported++
			}
		}
	}
	for attachmentID, ref := range refs {
		if legacyAttachmentIDs[attachmentID] || existing[attachmentID] {
			continue
		}
		if ref == nil || ref.attachment == nil {
			continue
		}
		if c.attachmentBinaryStatus(ctx, ref.attachment) != AttachmentBinaryMissing {
			continue
		}
		if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
			return err
		}
		imported++
	}

	if err := c.completeRuntimeMigration(ctx, videoManifestESMigrationKey, revision); err != nil {
		return err
	}
	if imported > 0 {
		c.logger.Info("Imported legacy video processing manifests into EVT", "count", imported)
	}
	return nil
}

func (c *ChattoCore) listLegacyVideoStateKeys(ctx context.Context) ([]string, error) {
	lister, err := c.storage.serverRuntimeKV.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("list legacy video states: %w", err)
	}
	var keys []string
	for key := range lister.Keys() {
		if strings.HasPrefix(key, "video.") {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (c *ChattoCore) appendVideoProcessedMigrationEvent(ctx context.Context, ref *legacyVideoAttachmentRef, attachmentID string, state *corev1.VideoProcessingState, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	assetVariants := make([]*corev1.AssetVideoVariant, 0, len(variants))
	thumbnailAssetID := ""
	if thumbnailAsset := assetFromAttachment(thumbnail); thumbnailAsset != nil {
		thumbnailAssetID = thumbnailAsset.GetId()
		if err := c.appendDerivativeAssetCreatedMigrationEvent(ctx, ref, thumbnailAsset, attachmentID, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL); err != nil {
			return err
		}
	}
	for _, variant := range variants {
		if variant == nil || variant.GetAttachment() == nil {
			continue
		}
		variantAsset := assetFromAttachment(variant.GetAttachment())
		if err := c.appendDerivativeAssetCreatedMigrationEvent(ctx, ref, variantAsset, attachmentID, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT); err != nil {
			return err
		}
		assetVariants = append(assetVariants, &corev1.AssetVideoVariant{
			Quality: variant.GetQuality(),
			AssetId: variant.GetAttachment().GetId(),
		})
	}
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: attachmentID,
				Video: &corev1.AssetProcessedVideo{
					DurationMs:       state.GetDurationMs(),
					Width:            state.GetWidth(),
					Height:           state.GetHeight(),
					ThumbnailAssetId: thumbnailAssetID,
					Variants:         assetVariants,
				},
			},
		},
	})
	_, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(ref.roomID).SubjectFor(event), event)
	return err
}

func (c *ChattoCore) appendDerivativeAssetCreatedMigrationEvent(ctx context.Context, ref *legacyVideoAttachmentRef, asset *corev1.AssetRecord, sourceAssetID string, role corev1.AssetDerivativeRole) error {
	if asset == nil || asset.GetId() == "" {
		return nil
	}
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset:                   asset,
				RoomId:                  ref.roomID,
				ParentAssetId:           sourceAssetID,
				DerivativeRole:          role,
			},
		},
	})
	_, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(ref.roomID).SubjectFor(event), event)
	return err
}

func (c *ChattoCore) appendVideoFailedMigrationEvent(ctx context.Context, ref *legacyVideoAttachmentRef, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:     attachmentID,
				FailureCode: failureCode,
			},
		},
	})
	_, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(ref.roomID).SubjectFor(event), event)
	return err
}

func (c *ChattoCore) usableAttachment(ctx context.Context, attachment *corev1.Attachment) *corev1.Attachment {
	if attachment == nil || c.attachmentBinaryStatus(ctx, attachment) != AttachmentBinaryPresent {
		return nil
	}
	return proto.Clone(attachment).(*corev1.Attachment)
}

// AttachmentBinaryStatus is the tri-state result of probing an attachment's
// underlying binary. Use this when the absence-vs-can't-tell distinction
// matters — most importantly, when deciding whether to emit a durable
// "source missing" terminal event.
type AttachmentBinaryStatus int

const (
	// AttachmentBinaryPresent means storage definitively returned the object.
	AttachmentBinaryPresent AttachmentBinaryStatus = iota
	// AttachmentBinaryMissing means storage definitively said "not there"
	// (S3 NoSuchKey / 404, NATS ObjectStore ErrObjectNotFound). Safe to
	// treat as a permanent terminal state.
	AttachmentBinaryMissing
	// AttachmentBinaryUnknown means the probe failed for a reason that
	// isn't "not found" — auth, network, missing client config, etc. The
	// binary might still exist; callers must NOT publish missing-source
	// events on this status, only skip / retry later.
	AttachmentBinaryUnknown
)

// attachmentBinaryStatus probes storage for the attachment and classifies
// the result. The intent is to let callers distinguish "we know it's gone"
// from "we couldn't reach storage" so that one-shot migrations don't burn
// SOURCE_MISSING events into EVT every time someone boots against
// unreachable S3.
func (c *ChattoCore) attachmentBinaryStatus(ctx context.Context, attachment *corev1.Attachment) AttachmentBinaryStatus {
	reader, _, err := c.GetAttachmentReader(ctx, attachment)
	if err == nil {
		if closer, ok := reader.(io.Closer); ok {
			_ = closer.Close()
		}
		return AttachmentBinaryPresent
	}
	if errors.Is(err, jetstream.ErrObjectNotFound) || IsNoSuchKeyError(err) {
		return AttachmentBinaryMissing
	}
	return AttachmentBinaryUnknown
}

func (c *ChattoCore) indexVideoAttachmentsFromEVT(ctx context.Context) (map[string]*legacyVideoAttachmentRef, error) {
	out := make(map[string]*legacyVideoAttachmentRef)
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_created"}, func(event *corev1.Event) {
		declared := event.GetAssetCreated()
		roomID := assetCreatedRoomID(declared)
		// Only original message attachments are candidates for legacy video
		// state — skip derivatives (thumbnails / transcoded variants), which
		// carry a parent_asset_id and have no standalone legacy state.
		if declared == nil || roomID == "" || declared.GetParentAssetId() != "" {
			return
		}
		att := attachmentFromAsset(declared.GetAsset())
		if att == nil {
			return
		}
		if !strings.HasPrefix(att.GetContentType(), "video/") && att.GetContentType() != "image/gif" {
			return
		}
		out[att.GetId()] = &legacyVideoAttachmentRef{
			roomID:     roomID,
			attachment: proto.Clone(att).(*corev1.Attachment),
		}
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ChattoCore) indexImportedVideoManifestEvents(ctx context.Context) (map[string]bool, error) {
	out := make(map[string]bool)
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_processing_succeeded", "evt.room.*.asset_processing_failed"}, func(event *corev1.Event) {
		if succeeded := event.GetAssetProcessingSucceeded(); succeeded != nil {
			out[succeeded.GetAssetId()] = true
		}
		if failed := event.GetAssetProcessingFailed(); failed != nil {
			out[failed.GetAssetId()] = true
		}
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *ChattoCore) scanEVT(ctx context.Context, filters []string, handle func(*corev1.Event)) error {
	consumer, err := c.storage.serverEvtStream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    filters,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create temporary EVT scan consumer: %w", err)
	}
	defer c.storage.serverEvtStream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return fmt.Errorf("get temporary EVT scan consumer info: %w", err)
	}
	if info.NumPending == 0 {
		return nil
	}
	msgs, err := consumer.Fetch(int(info.NumPending), jetstream.FetchMaxWait(60*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return fmt.Errorf("fetch temporary EVT scan messages: %w", err)
	}
	if msgs == nil {
		return nil
	}
	for msg := range msgs.Messages() {
		var event corev1.Event
		if err := proto.Unmarshal(msg.Data(), &event); err != nil {
			c.logger.Warn("temporary EVT scan: skipping unparseable event", "subject", msg.Subject(), "error", err)
			continue
		}
		handle(&event)
	}
	return nil
}
