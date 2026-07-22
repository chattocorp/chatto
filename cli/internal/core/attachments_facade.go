package core

import (
	"context"
	"io"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

func (c *ChattoCore) GetAttachmentsStore(ctx context.Context) (jetstream.ObjectStore, error) {
	return c.mediaModel.GetAttachmentsStore(ctx)
}

func (c *ChattoCore) UploadAttachment(
	ctx context.Context,
	actorID string,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.mediaModel.UploadAttachment(ctx, actorID, roomID, filename, contentType, reader)
}

func (c *ChattoCore) uploadAttachmentBinary(
	ctx context.Context,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.mediaModel.uploadAttachmentBinary(ctx, roomID, filename, contentType, reader)
}

func (c *ChattoCore) UploadDerivativeAttachment(
	ctx context.Context,
	parentAssetID string,
	derivativeRole corev1.AssetDerivativeRole,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.mediaModel.UploadDerivativeAttachment(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, reader)
}

func (c *ChattoCore) UploadDerivativeAttachmentWithDimensions(
	ctx context.Context,
	parentAssetID string,
	derivativeRole corev1.AssetDerivativeRole,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
	width int32,
	height int32,
) (*corev1.Attachment, error) {
	return c.mediaModel.UploadDerivativeAttachmentWithDimensions(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, reader, width, height)
}

func (c *ChattoCore) GetAttachment(ctx context.Context, attachmentID string) (io.Reader, *jetstream.ObjectInfo, error) {
	return c.mediaModel.GetAttachment(ctx, attachmentID)
}

func (c *ChattoCore) GetS3Attachment(ctx context.Context, s3Key string) (io.ReadCloser, *AttachmentInfo, error) {
	return c.mediaModel.GetS3Attachment(ctx, s3Key)
}

func (c *ChattoCore) GetAttachmentReader(ctx context.Context, attachment *corev1.Attachment) (io.Reader, *AttachmentInfo, error) {
	return c.mediaModel.GetAttachmentReader(ctx, attachment)
}

func (c *ChattoCore) MessageBodyAttachments(body *corev1.MessageBody) []*corev1.Attachment {
	return c.mediaModel.MessageBodyAttachments(body)
}

func (c *ChattoCore) DeleteAttachmentFromStorage(ctx context.Context, attachment *corev1.Attachment) error {
	return c.mediaModel.DeleteAttachmentFromStorage(ctx, attachment)
}

func (c *ChattoCore) DeleteVideoDerivativesForAttachment(ctx context.Context, actorID string, kind RoomKind, attachmentID string) {
	c.assetModel.DeleteVideoDerivativesForAttachment(ctx, actorID, attachmentID)
}

func (c *ChattoCore) DeleteMessageOwnedAssetsForUser(ctx context.Context, actorID, userID string) int {
	return c.assetModel.DeleteMessageOwnedAssetsForUser(ctx, actorID, userID)
}

func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, attachment *corev1.Attachment, ttl time.Duration) (string, error) {
	return c.mediaModel.TryPresignedAttachmentURL(ctx, attachment, ttl)
}

func (c *ChattoCore) GetStableAttachmentURL(assetID, userID string) string {
	return c.mediaModel.GetStableAttachmentURL(assetID, userID)
}

func (c *ChattoCore) GetStableAttachmentAssetURL(assetID, userID string) StableAssetURL {
	return c.mediaModel.GetStableAttachmentAssetURL(assetID, userID)
}

func (c *ChattoCore) GetStableHLSMasterPlaylistAssetURL(assetID, userID string) StableAssetURL {
	return c.mediaModel.GetStableHLSMasterPlaylistAssetURL(assetID, userID)
}

func (c *ChattoCore) GetStableTransformedAttachmentURL(assetID, userID string, width, height int, fit string) string {
	return c.mediaModel.GetStableTransformedAttachmentURL(assetID, userID, width, height, fit)
}

func (c *ChattoCore) GetStableTransformedAttachmentAssetURL(assetID, userID string, width, height int, fit string) StableAssetURL {
	return c.mediaModel.GetStableTransformedAttachmentAssetURL(assetID, userID, width, height, fit)
}

func (c *ChattoCore) GetTransformedServerAssetURL(key string, width, height int, fit string) string {
	return c.mediaModel.GetTransformedServerAssetURL(key, width, height, fit)
}

func (c *ChattoCore) ImageCacheEnabled() bool {
	return c.mediaModel.ImageCacheEnabled()
}

func (c *ChattoCore) GetCachedResize(ctx context.Context, key string) ([]byte, error) {
	return c.mediaModel.GetCachedResize(ctx, key)
}

func (c *ChattoCore) StoreCachedResize(ctx context.Context, key string, data []byte) error {
	return c.mediaModel.StoreCachedResize(ctx, key, data)
}

func (c *ChattoCore) DeleteCachedResizesForAttachment(ctx context.Context, attachmentID string) (int, error) {
	return c.mediaModel.DeleteCachedResizesForAttachment(ctx, attachmentID)
}

func (c *ChattoCore) DeleteCachedResizesForServerAsset(ctx context.Context, assetID string) (int, error) {
	return c.mediaModel.DeleteCachedResizesForServerAsset(ctx, assetID)
}

func (c *ChattoCore) DeleteCachedResizesForKey(ctx context.Context, prefix, assetKey string) (int, error) {
	return c.mediaModel.DeleteCachedResizesForKey(ctx, prefix, assetKey)
}

func (c *ChattoCore) ScheduleVideoProcessingForMessageAttachment(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID string, attachment *corev1.Attachment) error {
	return c.assetModel.ScheduleVideoProcessingForMessageAttachment(ctx, actorID, roomID, messageEventID, attachment)
}

func (c *ChattoCore) RecordAssetProcessingStarted(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, assetID string) error {
	return c.assetModel.RecordAssetProcessingStarted(ctx, actorID, roomID, messageEventID, assetID)
}

func (c *ChattoCore) RecoverUnmanifestedVideoAttachments(ctx context.Context) {
	c.assetModel.RecoverUnmanifestedVideoAttachments(ctx)
}

func (c *ChattoCore) PublishAssetProcessing(ctx context.Context, kind RoomKind, roomID string, event *corev1.Event) error {
	return c.assetModel.PublishAssetProcessing(ctx, roomID, event)
}

func (c *ChattoCore) RecordAssetProcessed(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	return c.assetModel.RecordAssetProcessed(ctx, actorID, roomID, messageEventID, attachmentID, durationMs, width, height, thumbnail, variants)
}

func (c *ChattoCore) RecordAssetProcessedWithHLS(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant, hls *corev1.AssetProcessedHLS) error {
	return c.assetModel.RecordAssetProcessedWithHLS(ctx, actorID, roomID, messageEventID, attachmentID, durationMs, width, height, thumbnail, variants, hls)
}

func (c *ChattoCore) RecordAssetDeleted(ctx context.Context, actorID string, kind RoomKind, roomID, assetID string) error {
	return c.assetModel.RecordAssetDeleted(ctx, actorID, roomID, assetID)
}

func (c *ChattoCore) RecordAssetProcessingFailed(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	return c.assetModel.RecordAssetProcessingFailed(ctx, actorID, roomID, messageEventID, attachmentID, failureCode)
}

// AssetEventTimelineTarget resolves the current room timeline row affected by
// a durable asset lifecycle event. Processing events carry their owning
// message directly. Deletions recover ownership from the room timeline's
// durable message-to-asset index, including a processed derivative referenced
// by an original message asset's manifest.
func (c *ChattoCore) AssetEventTimelineTarget(event *corev1.Event) (roomID, messageEventID string, ok bool) {
	assetID := assetIDOfLifecycleEvent(event)
	if assetID == "" {
		return "", "", false
	}
	roomID, ok = c.Assets.AssetRoomID(assetID)
	if !ok {
		return "", "", false
	}
	switch payload := event.GetEvent().(type) {
	case *corev1.Event_AssetProcessingStarted:
		messageEventID = payload.AssetProcessingStarted.GetMessageEventId()
	case *corev1.Event_AssetProcessingSucceeded:
		messageEventID = payload.AssetProcessingSucceeded.GetMessageEventId()
	case *corev1.Event_AssetProcessingFailed:
		messageEventID = payload.AssetProcessingFailed.GetMessageEventId()
	case *corev1.Event_AssetDeleted:
		if ownerRoomID, ownerMessageEventID, found := c.RoomTimeline.AssetMessageOwner(assetID); found {
			return ownerRoomID, ownerMessageEventID, true
		}
		for _, owner := range c.RoomTimeline.MessageAssetOwners() {
			manifest, found := c.Assets.VideoAttachmentManifest(owner.AssetID)
			if !found || manifest == nil || manifest.Succeeded == nil || manifest.Succeeded.GetVideo() == nil {
				continue
			}
			video := manifest.Succeeded.GetVideo()
			if video.GetThumbnailAssetId() == assetID {
				return owner.RoomID, owner.MessageEventID, true
			}
			for _, variant := range video.GetVariants() {
				if variant.GetAssetId() == assetID {
					return owner.RoomID, owner.MessageEventID, true
				}
			}
		}
	default:
		return "", "", false
	}
	return roomID, messageEventID, messageEventID != ""
}

func (c *ChattoCore) stableAttachmentPathWithAccess(assetID, userID, path string, params *signedurl.TransformParams, expiresAt time.Time) string {
	return c.mediaModel.stableAttachmentPathWithAccess(assetID, userID, path, params, expiresAt)
}
