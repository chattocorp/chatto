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
	return c.media().GetAttachmentsStore(ctx)
}

func (c *ChattoCore) UploadAttachment(
	ctx context.Context,
	actorID string,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.media().UploadAttachment(ctx, actorID, roomID, filename, contentType, reader)
}

func (c *ChattoCore) uploadAttachmentBinary(
	ctx context.Context,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.media().uploadAttachmentBinary(ctx, roomID, filename, contentType, reader)
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
	return c.media().UploadDerivativeAttachment(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, reader)
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
	return c.media().UploadDerivativeAttachmentWithDimensions(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, reader, width, height)
}

func (c *ChattoCore) GetAttachment(ctx context.Context, attachmentID string) (io.Reader, *jetstream.ObjectInfo, error) {
	return c.media().GetAttachment(ctx, attachmentID)
}

func (c *ChattoCore) GetS3Attachment(ctx context.Context, s3Key string) (io.ReadCloser, *AttachmentInfo, error) {
	return c.media().GetS3Attachment(ctx, s3Key)
}

func (c *ChattoCore) GetAttachmentReader(ctx context.Context, attachment *corev1.Attachment) (io.Reader, *AttachmentInfo, error) {
	return c.media().GetAttachmentReader(ctx, attachment)
}

func (c *ChattoCore) MessageBodyAttachments(body *corev1.MessageBody) []*corev1.Attachment {
	return c.media().MessageBodyAttachments(body)
}

func (c *ChattoCore) DeleteAttachmentFromStorage(ctx context.Context, attachment *corev1.Attachment) error {
	return c.media().DeleteAttachmentFromStorage(ctx, attachment)
}

func (c *ChattoCore) DeleteVideoDerivativesForAttachment(ctx context.Context, actorID string, kind RoomKind, attachmentID string) {
	c.assetLifecycle().DeleteVideoDerivativesForAttachment(ctx, actorID, attachmentID)
}

func (c *ChattoCore) DeleteMessageOwnedAssetsForUser(ctx context.Context, actorID, userID string) int {
	return c.assetLifecycle().DeleteMessageOwnedAssetsForUser(ctx, actorID, userID)
}

func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, attachment *corev1.Attachment, ttl time.Duration) (string, error) {
	return c.media().TryPresignedAttachmentURL(ctx, attachment, ttl)
}

func (c *ChattoCore) GetStableAttachmentURL(assetID, userID string) string {
	return c.media().GetStableAttachmentURL(assetID, userID)
}

func (c *ChattoCore) GetStableAttachmentAssetURL(assetID, userID string) StableAssetURL {
	return c.media().GetStableAttachmentAssetURL(assetID, userID)
}

func (c *ChattoCore) GetStableTransformedAttachmentURL(assetID, userID string, width, height int, fit string) string {
	return c.media().GetStableTransformedAttachmentURL(assetID, userID, width, height, fit)
}

func (c *ChattoCore) GetStableTransformedAttachmentAssetURL(assetID, userID string, width, height int, fit string) StableAssetURL {
	return c.media().GetStableTransformedAttachmentAssetURL(assetID, userID, width, height, fit)
}

func (c *ChattoCore) GetTransformedServerAssetURL(key string, width, height int, fit string) string {
	return c.media().GetTransformedServerAssetURL(key, width, height, fit)
}

func (c *ChattoCore) ImageCacheEnabled() bool {
	return c.media().ImageCacheEnabled()
}

func (c *ChattoCore) GetCachedResize(ctx context.Context, key string) ([]byte, error) {
	return c.media().GetCachedResize(ctx, key)
}

func (c *ChattoCore) StoreCachedResize(ctx context.Context, key string, data []byte) error {
	return c.media().StoreCachedResize(ctx, key, data)
}

func (c *ChattoCore) DeleteCachedResizesForAttachment(ctx context.Context, attachmentID string) (int, error) {
	return c.media().DeleteCachedResizesForAttachment(ctx, attachmentID)
}

func (c *ChattoCore) DeleteCachedResizesForServerAsset(ctx context.Context, assetID string) (int, error) {
	return c.media().DeleteCachedResizesForServerAsset(ctx, assetID)
}

func (c *ChattoCore) DeleteCachedResizesForKey(ctx context.Context, prefix, assetKey string) (int, error) {
	return c.media().DeleteCachedResizesForKey(ctx, prefix, assetKey)
}

func (c *ChattoCore) ScheduleVideoProcessingForMessageAttachment(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID string, attachment *corev1.Attachment) error {
	return c.assetLifecycle().ScheduleVideoProcessingForMessageAttachment(ctx, actorID, roomID, messageEventID, attachment)
}

func (c *ChattoCore) RecordAssetProcessingStarted(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, assetID string) error {
	return c.assetLifecycle().RecordAssetProcessingStarted(ctx, actorID, roomID, messageEventID, assetID)
}

func (c *ChattoCore) RecoverUnmanifestedVideoAttachments(ctx context.Context) {
	c.assetLifecycle().RecoverUnmanifestedVideoAttachments(ctx)
}

func (c *ChattoCore) PublishAssetProcessing(ctx context.Context, kind RoomKind, roomID string, event *corev1.Event) error {
	return c.assetLifecycle().PublishAssetProcessing(ctx, roomID, event)
}

func (c *ChattoCore) RecordAssetProcessed(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	return c.assetLifecycle().RecordAssetProcessed(ctx, actorID, roomID, messageEventID, attachmentID, durationMs, width, height, thumbnail, variants)
}

func (c *ChattoCore) RecordAssetDeleted(ctx context.Context, actorID string, kind RoomKind, roomID, assetID string) error {
	return c.assetLifecycle().RecordAssetDeleted(ctx, actorID, roomID, assetID)
}

func (c *ChattoCore) RecordAssetProcessingFailed(ctx context.Context, actorID string, kind RoomKind, roomID, messageEventID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	return c.assetLifecycle().RecordAssetProcessingFailed(ctx, actorID, roomID, messageEventID, attachmentID, failureCode)
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
	return c.media().stableAttachmentPathWithAccess(assetID, userID, path, params, expiresAt)
}
