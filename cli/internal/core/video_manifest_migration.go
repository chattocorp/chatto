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
	roomID         string
	messageEventID string
	attachment     *corev1.Attachment
}

// migrateVideoManifestsToES imports legacy SERVER_RUNTIME video.{attachment}
// processing state into durable EVT manifest events. It deliberately leaves the
// old KV keys in place for rollback, using a sentinel to avoid duplicate imports.
func (c *ChattoCore) migrateVideoManifestsToES(ctx context.Context) error {
	if err := c.migrateAssetCreationsToES(ctx); err != nil {
		return fmt.Errorf("migrate asset creations: %w", err)
	}
	if entry, err := c.storage.serverRuntimeKV.Get(ctx, videoManifestESMigrationKey); err == nil && entry != nil {
		return nil
	} else if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("get sentinel: %w", err)
	}

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

		sourceAvailable := c.attachmentBinaryAvailable(ctx, ref.attachment)
		switch state.Status {
		case corev1.VideoStatus_VIDEO_STATUS_COMPLETED:
			thumbnail := c.usableAttachment(ctx, state.ThumbnailAttachment)
			var variants []*corev1.VideoVariant
			for _, variant := range state.Variants {
				if variant == nil || variant.Attachment == nil || !c.attachmentBinaryAvailable(ctx, variant.Attachment) {
					continue
				}
				variants = append(variants, proto.Clone(variant).(*corev1.VideoVariant))
			}
			if len(variants) == 0 {
				if !sourceAvailable {
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
			if !sourceAvailable {
				failureCode = corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING
			}
			if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, failureCode); err != nil {
				return err
			}
			imported++
		case corev1.VideoStatus_VIDEO_STATUS_PENDING, corev1.VideoStatus_VIDEO_STATUS_PROCESSING:
			if !sourceAvailable {
				if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
					return err
				}
				imported++
			}
		default:
			if !sourceAvailable {
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
		if ref == nil || ref.attachment == nil || c.attachmentBinaryAvailable(ctx, ref.attachment) {
			continue
		}
		if err := c.appendVideoFailedMigrationEvent(ctx, ref, attachmentID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
			return err
		}
		imported++
	}

	if _, err := c.storage.serverRuntimeKV.Put(ctx, videoManifestESMigrationKey, []byte("1")); err != nil {
		return fmt.Errorf("set sentinel: %w", err)
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
		thumbnailAsset.Parent = &corev1.Asset_Asset{
			Asset: &corev1.AssetDerivativeParent{
				AssetId: attachmentID,
				Role:    "thumbnail",
			},
		}
		if err := c.appendDerivativeAssetCreatedMigrationEvent(ctx, ref, thumbnailAsset); err != nil {
			return err
		}
	}
	for _, variant := range variants {
		if variant == nil || variant.GetAttachment() == nil {
			continue
		}
		variantAsset := assetFromAttachment(variant.GetAttachment())
		variantAsset.Parent = &corev1.Asset_Asset{
			Asset: &corev1.AssetDerivativeParent{
				AssetId: attachmentID,
				Role:    "video_variant",
				Variant: variant.GetQuality(),
			},
		}
		if err := c.appendDerivativeAssetCreatedMigrationEvent(ctx, ref, variantAsset); err != nil {
			return err
		}
		assetVariants = append(assetVariants, &corev1.AssetVideoVariant{
			Quality: variant.GetQuality(),
			AssetId: variant.GetAttachment().GetId(),
		})
	}
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: attachmentID,
				Result: &corev1.AssetProcessingSucceededEvent_Video{
					Video: &corev1.AssetProcessedVideo{
						DurationMs:       state.GetDurationMs(),
						Width:            state.GetWidth(),
						Height:           state.GetHeight(),
						ThumbnailAssetId: thumbnailAssetID,
						Variants:         assetVariants,
					},
				},
			},
		},
	})
	_, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(ref.roomID).SubjectFor(event), event)
	return err
}

func (c *ChattoCore) appendDerivativeAssetCreatedMigrationEvent(ctx context.Context, ref *legacyVideoAttachmentRef, asset *corev1.Asset) error {
	if asset == nil || asset.GetId() == "" {
		return nil
	}
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				SourceAvailable: true,
				Asset:           asset,
			},
		},
	})
	_, err := c.EventPublisher.AppendEventually(ctx, events.RoomAggregate(ref.roomID).SubjectFor(event), event)
	return err
}

func (c *ChattoCore) appendVideoFailedMigrationEvent(ctx context.Context, ref *legacyVideoAttachmentRef, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	event := newEvent("", &corev1.Event{
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
	if attachment == nil || !c.attachmentBinaryAvailable(ctx, attachment) {
		return nil
	}
	return proto.Clone(attachment).(*corev1.Attachment)
}

func (c *ChattoCore) attachmentBinaryAvailable(ctx context.Context, attachment *corev1.Attachment) bool {
	reader, _, err := c.GetAttachmentReader(ctx, attachment)
	if err != nil {
		return false
	}
	if closer, ok := reader.(io.Closer); ok {
		_ = closer.Close()
	}
	return true
}

func (c *ChattoCore) indexVideoAttachmentsFromEVT(ctx context.Context) (map[string]*legacyVideoAttachmentRef, error) {
	out := make(map[string]*legacyVideoAttachmentRef)
	if err := c.scanEVT(ctx, []string{"evt.room.*.asset_created"}, func(event *corev1.Event) {
		declared := event.GetAssetCreated()
		roomID := assetCreatedRoomID(declared)
		messageEventID := assetCreatedMessageEventID(declared)
		if declared == nil || roomID == "" || messageEventID == "" {
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
			roomID:         roomID,
			messageEventID: messageEventID,
			attachment:     proto.Clone(att).(*corev1.Attachment),
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
