package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

// ============================================================================
// Attachment Operations
// ============================================================================

// GetAttachmentsStore returns the ObjectStore for attachments.
// Uses lazy-loading and caching for efficiency.
func (c *ChattoCore) GetAttachmentsStore(ctx context.Context) (jetstream.ObjectStore, error) {
	return c.storage.serverAttachments, nil
}

// UploadAttachment uploads a file as an attachment and returns the
// attachment metadata. For images, it extracts dimensions. Thumbnails
// are generated on-the-fly via transforms. The storage backend (NATS or
// S3) is determined by configuration.
//
// New attachments use the kind-less `attachments/{id}` S3 key.
// `Attachment.SpaceId` is left empty (it's a vestigial field).
// `Attachment.MessageBodyId` is also left empty here — the body key is
// set later in PostMessage when the owning MessageBody is written.
func (c *ChattoCore) UploadAttachment(
	ctx context.Context,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	// Generate a unique ID for this attachment
	attachmentID := NewAssetID()

	// Check if this is an image that we should process
	isImage := strings.HasPrefix(contentType, "image/")

	var content []byte
	var size int64
	var width, height int32

	if isImage {
		// Process the image: extract metadata (dimensions)
		result, err := assets.ProcessAttachmentImageWithConfig(reader, c.AssetsConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to process image: %w", err)
		}

		content = result.Original
		size = int64(len(result.Original))
		width = int32(result.Width)
		height = int32(result.Height)
	} else {
		// For non-images, just read the file content.
		// Videos get a higher limit when video processing is enabled.
		assetsCfg := c.AssetsConfig()
		maxSize := assetsCfg.MaxUploadSize
		if strings.HasPrefix(contentType, "video/") && c.VideoMaxUploadSize > 0 {
			maxSize = c.VideoMaxUploadSize
		}

		var err error
		content, err = io.ReadAll(io.LimitReader(reader, maxSize+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read attachment: %w", err)
		}
		if int64(len(content)) > maxSize {
			return nil, fmt.Errorf("attachment exceeds maximum size of %d bytes", maxSize)
		}

		size = int64(len(content))
	}

	// Store the attachment in the appropriate backend
	var storage *corev1.Asset
	if c.ShouldUseS3() {
		// Upload to S3
		s3Key := S3KeyAttachment(attachmentID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, content, contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to upload attachment to S3: %w", err)
		}
		storage = &corev1.Asset{
			Asset: &corev1.Asset_S3{
				S3: &corev1.S3Asset{
					Key:    s3Key,
					Bucket: proto.String(c.s3Client.Bucket()),
				},
			},
		}
		c.logger.Debug("Uploaded attachment to S3",
			"attachment_id", attachmentID,
			"s3_key", s3Key,
		)
	} else {
		// Upload to NATS ObjectStore
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get attachments store: %w", err)
		}

		_, err = store.Put(ctx, jetstream.ObjectMeta{
			Name: attachmentID,
			Headers: map[string][]string{
				"Content-Type": {contentType},
				"Filename":     {filename},
				"Room-Id":      {roomID},
			},
		}, bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("failed to store attachment: %w", err)
		}
		storage = &corev1.Asset{
			Asset: &corev1.Asset_Nats{
				Nats: &corev1.NATSAsset{
					Key: attachmentID,
				},
			},
		}
	}

	attachment := &corev1.Attachment{
		Id:          attachmentID,
		RoomId:      roomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        size,
		Width:       width,
		Height:      height,
		Storage:     storage,
	}

	c.logger.Debug("Uploaded attachment",
		"attachment_id", attachmentID,
		"room_id", roomID,
		"filename", filename,
		"content_type", contentType,
		"size", size,
		"storage_backend", c.config.Assets.StorageBackend,
	)

	return attachment, nil
}

// AttachmentInfo contains metadata about an attachment.
// This is a backend-agnostic wrapper around storage-specific info.
type AttachmentInfo struct {
	Size        int64
	ContentType string
	Filename    string
	RoomID      string
}

// GetAttachment retrieves an attachment by ID from NATS ObjectStore.
// This is the legacy path for attachments stored in NATS.
// Returns a reader for the attachment content and the object info.
func (c *ChattoCore) GetAttachment(ctx context.Context, attachmentID string) (io.Reader, *jetstream.ObjectInfo, error) {
	store, err := c.GetAttachmentsStore(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get attachments store: %w", err)
	}

	result, err := store.Get(ctx, attachmentID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get attachment: %w", err)
	}

	info, err := result.Info()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get attachment info: %w", err)
	}

	return result, info, nil
}

// GetS3Attachment retrieves an attachment from S3 by its key.
// Returns a reader for the attachment content and metadata.
// The caller is responsible for closing the reader.
//
// AttachmentInfo.RoomID is NOT populated here — S3 has no equivalent of
// the NATS `Room-Id` header. Callers that need authorization should
// instead look up the canonical Attachment record via GetAttachmentRecord.
func (c *ChattoCore) GetS3Attachment(ctx context.Context, s3Key string) (io.ReadCloser, *AttachmentInfo, error) {
	if c.s3Client == nil {
		return nil, nil, fmt.Errorf("S3 client not configured")
	}

	reader, info, err := c.s3Client.GetObject(ctx, s3Key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get S3 attachment: %w", err)
	}

	return reader, &AttachmentInfo{
		Size:        info.Size,
		ContentType: info.ContentType,
	}, nil
}

// GetAttachmentReader reads an attachment's binary from whichever
// storage backend its `Storage` field points at. Returns a reader and
// metadata. The caller is responsible for closing the reader if it
// implements io.Closer.
//
// Returns an error if `attachment.Storage` is unset or points at an
// unknown backend kind.
func (c *ChattoCore) GetAttachmentReader(ctx context.Context, attachment *corev1.Attachment) (io.Reader, *AttachmentInfo, error) {
	if attachment == nil || attachment.Storage == nil {
		return nil, nil, fmt.Errorf("attachment has no storage info")
	}
	switch asset := attachment.Storage.Asset.(type) {
	case *corev1.Asset_Nats:
		reader, info, err := c.GetAttachment(ctx, asset.Nats.Key)
		if err != nil {
			return nil, nil, err
		}
		return reader, &AttachmentInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
			Filename:    info.Headers.Get("Filename"),
			RoomID:      info.Headers.Get("Room-Id"),
		}, nil
	case *corev1.Asset_S3:
		if c.s3Client == nil {
			return nil, nil, fmt.Errorf("S3 client not configured")
		}
		return c.GetS3Attachment(ctx, asset.S3.Key)
	default:
		return nil, nil, fmt.Errorf("attachment %s has unknown storage backend", attachment.Id)
	}
}

// FindBodyAttachment fetches the named MessageBody and returns the
// embedded Attachment with the given ID, or (nil, nil) if either is
// missing. The returned Attachment is the in-memory copy from the body
// proto with `MessageBodyId` populated, so callers can use it to
// construct signed URLs directly.
func (c *ChattoCore) FindBodyAttachment(ctx context.Context, bodyKey, attachmentID string) (*corev1.Attachment, error) {
	if bodyKey == "" || attachmentID == "" {
		return nil, nil
	}
	entry, err := c.storage.serverBodiesKV.Get(ctx, bodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get message body: %w", err)
	}
	var body corev1.MessageBody
	if err := proto.Unmarshal(entry.Value(), &body); err != nil {
		return nil, fmt.Errorf("unmarshal message body: %w", err)
	}
	for _, att := range body.Attachments {
		if att.Id == attachmentID {
			if att.MessageBodyId == "" {
				att.MessageBodyId = bodyKey
			}
			return att, nil
		}
	}
	return nil, nil
}

// FindVideoOriginAttachment looks up a variant or thumbnail Attachment
// from the `VideoProcessingState` keyed by the original video's
// attachment ID. Returns (nil, nil) if the state is missing or doesn't
// contain an attachment with the given ID.
func (c *ChattoCore) FindVideoOriginAttachment(ctx context.Context, videoOriginID, attachmentID string) (*corev1.Attachment, error) {
	if videoOriginID == "" || attachmentID == "" {
		return nil, nil
	}
	state, err := c.GetVideoProcessingState(ctx, videoOriginID)
	if err != nil {
		return nil, fmt.Errorf("get video processing state: %w", err)
	}
	if state == nil {
		return nil, nil
	}
	if state.ThumbnailAttachment != nil && state.ThumbnailAttachment.Id == attachmentID {
		return state.ThumbnailAttachment, nil
	}
	for _, v := range state.Variants {
		if v.Attachment != nil && v.Attachment.Id == attachmentID {
			return v.Attachment, nil
		}
	}
	return nil, nil
}

// LookupAttachment resolves any attachment by its URL locator, choosing
// the right source of truth (`MessageBody.Attachments` for body
// attachments, `VideoProcessingState` for variants/thumbnails).
func (c *ChattoCore) LookupAttachment(ctx context.Context, loc signedurl.AttachmentLocator) (*corev1.Attachment, error) {
	if err := loc.Validate(); err != nil {
		return nil, err
	}
	if loc.BodyKey != "" {
		return c.FindBodyAttachment(ctx, loc.BodyKey, loc.AttachmentID)
	}
	return c.FindVideoOriginAttachment(ctx, loc.VideoOrigin, loc.AttachmentID)
}

// DeleteAttachmentFromStorage deletes an attachment's binary and its
// cached resizes. Requires `Storage` to be populated.
func (c *ChattoCore) DeleteAttachmentFromStorage(ctx context.Context, attachment *corev1.Attachment) error {
	if attachment == nil || attachment.Storage == nil {
		return fmt.Errorf("attachment has no storage info")
	}

	switch storage := attachment.Storage.Asset.(type) {
	case *corev1.Asset_Nats:
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return fmt.Errorf("failed to get attachments store: %w", err)
		}
		if err := store.Delete(ctx, storage.Nats.Key); err != nil {
			return fmt.Errorf("failed to delete attachment from NATS: %w", err)
		}
		c.logger.Debug("Deleted NATS attachment", "attachment_id", attachment.Id, "key", storage.Nats.Key)
	case *corev1.Asset_S3:
		if c.s3Client == nil {
			return fmt.Errorf("S3 client not configured")
		}
		if err := c.s3Client.DeleteObjectFromBucket(ctx, storage.S3.GetBucket(), storage.S3.Key); err != nil {
			return fmt.Errorf("failed to delete S3 attachment: %w", err)
		}
		c.logger.Debug("Deleted S3 attachment", "attachment_id", attachment.Id, "s3_key", storage.S3.Key)
	default:
		return fmt.Errorf("attachment %s has unknown storage backend", attachment.Id)
	}

	deletedCount, cacheErr := c.DeleteCachedResizesForAttachment(ctx, attachment.Id)
	if cacheErr != nil {
		c.logger.Warn("Failed to delete cached resizes for attachment",
			"attachment_id", attachment.Id,
			"error", cacheErr)
	} else if deletedCount > 0 {
		c.logger.Debug("Deleted cached resizes for attachment",
			"attachment_id", attachment.Id,
			"deleted_count", deletedCount)
	}

	return nil
}

// TryPresignedAttachmentURL generates a presigned S3 URL for an
// attachment. Returns an error if S3 is not configured or the attachment
// is not stored in S3 (e.g. it's a NATS-stored legacy attachment, in
// which case the caller should fall back to GetAttachmentReader).
//
// Authorization is the caller's responsibility.
func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, attachment *corev1.Attachment) (string, error) {
	if c.s3Client == nil {
		return "", fmt.Errorf("S3 not configured")
	}
	if attachment == nil || attachment.Storage == nil {
		return "", fmt.Errorf("attachment has no storage info")
	}
	s3, ok := attachment.Storage.Asset.(*corev1.Asset_S3)
	if !ok {
		return "", fmt.Errorf("attachment %s is not stored in S3", attachment.Id)
	}
	presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3.S3.Key, time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

// AttachmentSignResource is the first resource component fed to the
// signed-URL signer for attachment transform URLs (after the locator).
// Stable so existing signatures continue to verify across deployments.
const AttachmentSignResource = "attachment"

// GetAttachmentURL returns the URL for accessing the binary identified
// by the locator. The URL embeds the locator as a signed payload, so
// the HTTP handler can authorize and serve without a separate index
// lookup.
//
// Returns an empty string if the locator is invalid (which would
// indicate a programmer error — locators come from trusted resolver
// code, not user input).
func (c *ChattoCore) GetAttachmentURL(loc signedurl.AttachmentLocator) string {
	signed, err := signedurl.SignedAttachmentLocator(c.config.Assets.SigningSecret, loc)
	if err != nil {
		c.logger.Warn("Failed to sign attachment locator", "error", err, "locator", loc)
		return ""
	}
	return c.assetURL(fmt.Sprintf("/assets/attachments/%s", signed))
}

// GetTransformedAttachmentURL returns the URL for a transformed version
// of the attachment identified by the locator. The transform parameters
// are signed separately so the same locator can drive multiple
// transforms without re-signing.
//
//	/assets/attachments/{signed-locator}/t/{params}.{signature}
//
// {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}.
func (c *ChattoCore) GetTransformedAttachmentURL(loc signedurl.AttachmentLocator, width, height int, fit string) string {
	signedLoc, err := signedurl.SignedAttachmentLocator(c.config.Assets.SigningSecret, loc)
	if err != nil {
		c.logger.Warn("Failed to sign attachment locator", "error", err, "locator", loc)
		return ""
	}
	signedTransform := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, AttachmentSignResource, signedLoc, width, height, fit)
	return c.assetURL(fmt.Sprintf("/assets/attachments/%s/t/%s", signedLoc, signedTransform))
}

// LocatorForBodyAttachment builds the URL locator for an attachment
// embedded in a MessageBody. `bodyKey` defaults to attachment.MessageBodyId
// when empty.
func LocatorForBodyAttachment(attachment *corev1.Attachment, bodyKey string) signedurl.AttachmentLocator {
	if bodyKey == "" {
		bodyKey = attachment.MessageBodyId
	}
	return signedurl.AttachmentLocator{
		RoomID:       attachment.RoomId,
		BodyKey:      bodyKey,
		AttachmentID: attachment.Id,
	}
}

// LocatorForVideoOriginAttachment builds the URL locator for a video
// variant or thumbnail attachment owned by a VideoProcessingState
// keyed by `videoOriginID` (the original video's attachment ID).
func LocatorForVideoOriginAttachment(roomID, videoOriginID, attachmentID string) signedurl.AttachmentLocator {
	return signedurl.AttachmentLocator{
		RoomID:       roomID,
		VideoOrigin:  videoOriginID,
		AttachmentID: attachmentID,
	}
}

// GetTransformedServerAssetURL returns the URL for accessing a transformed version of an server asset.
// Server assets include space logos, space banners, and user avatars stored in the server object store.
// The URL includes HMAC signature to prevent parameter tampering.
// Format: /assets/server/{key}/t/{params}.{signature}
// where {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}
func (c *ChattoCore) GetTransformedServerAssetURL(key string, width, height int, fit string) string {
	// Generate signed transform path component using "server" as the first resource ID
	signedPath := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, "server", key, width, height, fit)

	// Return signed transform URL
	return c.assetURL(fmt.Sprintf("/assets/server/%s/t/%s", key, signedPath))
}

// ============================================================================
// Image Cache Operations
// ============================================================================

// ImageCacheEnabled returns whether the image resize cache is enabled.
func (c *ChattoCore) ImageCacheEnabled() bool {
	return c.storage.imageCacheStore != nil
}

// ImageCacheKey generates a cache key for a resized image.
// Format: {spaceId}.{attachmentId}.{paramsHash}
// Uses NATS subject notation (dots as separators).
func ImageCacheKey(spaceID, attachmentID string, width, height int, fit string) string {
	params := fmt.Sprintf("%dx%d_%s", width, height, fit)
	hash := sha256.Sum256([]byte(params))
	return fmt.Sprintf("%s.%s.%x", spaceID, attachmentID, hash[:8])
}

// GetCachedResize retrieves a cached resized image.
// Returns nil, nil if the cache is disabled or the key is not found.
func (c *ChattoCore) GetCachedResize(ctx context.Context, key string) ([]byte, error) {
	if c.storage.imageCacheStore == nil {
		return nil, nil
	}

	result, err := c.storage.imageCacheStore.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get cached resize: %w", err)
	}

	data, err := io.ReadAll(result)
	if err != nil {
		return nil, fmt.Errorf("failed to read cached resize: %w", err)
	}

	return data, nil
}

// StoreCachedResize stores a resized image in the cache.
// Does nothing if the cache is disabled.
func (c *ChattoCore) StoreCachedResize(ctx context.Context, key string, data []byte) error {
	if c.storage.imageCacheStore == nil {
		return nil
	}

	_, err := c.storage.imageCacheStore.Put(ctx, jetstream.ObjectMeta{
		Name: key,
		Headers: map[string][]string{
			"Content-Type": {"image/webp"},
		},
	}, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to store cached resize: %w", err)
	}

	return nil
}

// DeleteCachedResizesForAttachment deletes all cached resizes for an
// attachment. Returns the number of deleted cache entries and any error
// encountered. Does nothing if the cache is disabled. Pre-ADR-030-Phase-4
// cache entries written under a {server|DM} prefix are not cleaned up
// and are left to age out — the transform-URL signer always uses the
// kind-less prefix now, so no lookups land on them.
func (c *ChattoCore) DeleteCachedResizesForAttachment(ctx context.Context, attachmentID string) (int, error) {
	return c.DeleteCachedResizesForKey(ctx, AttachmentSignResource, attachmentID)
}

// DeleteCachedResizesForKey deletes all cached resizes for a given prefix and asset key.
// Returns the number of deleted cache entries and any error encountered.
// Does nothing if the cache is disabled.
func (c *ChattoCore) DeleteCachedResizesForKey(ctx context.Context, prefix, assetKey string) (int, error) {
	if c.storage.imageCacheStore == nil {
		return 0, nil
	}

	// Cache keys follow the pattern: {prefix}.{assetKey}.{paramsHash}
	// We need to find and delete all keys that start with {prefix}.{assetKey}.
	keyPrefix := fmt.Sprintf("%s.%s.", prefix, assetKey)

	// List all objects in the cache
	objects, err := c.storage.imageCacheStore.List(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list cache objects: %w", err)
	}

	// Find and delete objects matching our prefix
	deleted := 0
	for _, info := range objects {
		if strings.HasPrefix(info.Name, keyPrefix) {
			if err := c.storage.imageCacheStore.Delete(ctx, info.Name); err != nil {
				// Log but continue deleting other entries
				c.logger.Warn("Failed to delete cached resize",
					"cache_key", info.Name,
					"error", err)
			} else {
				deleted++
			}
		}
	}

	return deleted, nil
}

// ============================================================================
// Video Processing State
// ============================================================================

// videoProcessingKey returns the RUNTIME KV key for a video's processing state.
func videoProcessingKey(attachmentID string) string {
	return "video." + attachmentID
}

// GetVideoProcessingState retrieves the processing state for a video attachment.
// Returns nil, nil if no processing state exists for this attachment.
func (c *ChattoCore) GetVideoProcessingState(ctx context.Context, attachmentID string) (*corev1.VideoProcessingState, error) {
	bucket := c.storage.serverRuntimeKV

	entry, err := bucket.Get(ctx, videoProcessingKey(attachmentID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get video processing state: %w", err)
	}

	state := &corev1.VideoProcessingState{}
	if err := proto.Unmarshal(entry.Value(), state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal video processing state: %w", err)
	}

	return state, nil
}

// SetVideoProcessingState stores the processing state for a video attachment.
func (c *ChattoCore) SetVideoProcessingState(ctx context.Context, attachmentID string, state *corev1.VideoProcessingState) error {
	bucket := c.storage.serverRuntimeKV

	data, err := proto.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal video processing state: %w", err)
	}

	if _, err := bucket.Put(ctx, videoProcessingKey(attachmentID), data); err != nil {
		return fmt.Errorf("failed to store video processing state: %w", err)
	}

	return nil
}

// SubjectVideoProcess is the NATS subject for video processing requests.
const SubjectVideoProcess = "chatto.video.process"

// InitVideoProcessingState creates the initial PENDING state for a video attachment.
// Call this BEFORE PostMessage so that the subscription-delivered event already has
// videoProcessing data when the frontend resolves it.
func (c *ChattoCore) InitVideoProcessingState(ctx context.Context, attachmentID string) error {
	return c.SetVideoProcessingState(ctx, attachmentID, &corev1.VideoProcessingState{
		Status: corev1.VideoStatus_VIDEO_STATUS_PENDING,
	})
}

// PublishVideoProcessingRequest publishes a video processing request to NATS.
// Call this AFTER PostMessage, once the messageBodyID is known. The video
// service consumes this subject via a transient (non-JetStream) subscription,
// so the payload format can evolve freely.
func (c *ChattoCore) PublishVideoProcessingRequest(ctx context.Context, roomID, attachmentID, contentType, messageBodyID string) error {
	payload := struct {
		RoomID        string `json:"room_id"`
		AttachmentID  string `json:"attachment_id"`
		ContentType   string `json:"content_type"`
		MessageBodyID string `json:"message_body_id"`
	}{
		RoomID:        roomID,
		AttachmentID:  attachmentID,
		ContentType:   contentType,
		MessageBodyID: messageBodyID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal processing request: %w", err)
	}

	if err := c.nc.Publish(SubjectVideoProcess, data); err != nil {
		return fmt.Errorf("failed to publish processing request: %w", err)
	}

	c.logger.Debug("Requested video processing",
		"attachment_id", attachmentID,
	)

	return nil
}

// PublishVideoProcessingCompleted publishes a live event indicating video processing is done.
// The frontend subscription receives this and refreshes the affected message.
func (c *ChattoCore) PublishVideoProcessingCompleted(ctx context.Context, kind RoomKind, roomID, attachmentID, messageBodyID string) error {
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_VideoProcessingCompleted{
			VideoProcessingCompleted: &corev1.VideoProcessingCompletedEvent{
				RoomId:         roomID,
				AttachmentId:   attachmentID,
				MessageBodyId:  messageBodyID,
				MessageEventId: eventIDFromBodyKey(messageBodyID),
			},
		},
	})

	subject := subjects.LiveRoomEvent(string(kind), roomID, "video_processed")
	return c.publishLiveServerEvent(ctx, subject, event)
}

