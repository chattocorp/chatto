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
	"hmans.de/chatto/pkg/signedurl"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Attachment Operations
// ============================================================================

// GetAttachmentsStore returns the ObjectStore for attachments in a space.
// Uses lazy-loading and caching for efficiency.
func (c *ChattoCore) GetAttachmentsStore(ctx context.Context, spaceID string) (jetstream.ObjectStore, error) {
	return c.storage.serverAttachments, nil
}

// UploadAttachment uploads a file as an attachment and returns the attachment metadata.
// For images, it extracts dimensions. Thumbnails are generated on-the-fly via transforms.
// The storage backend (NATS or S3) is determined by configuration.
func (c *ChattoCore) UploadAttachment(
	ctx context.Context,
	spaceID string,
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
		s3Key := S3KeySpaceAttachment(spaceID, attachmentID)
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
		store, err := c.GetAttachmentsStore(ctx, spaceID)
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

	c.logger.Debug("Uploaded attachment",
		"attachment_id", attachmentID,
		"space_id", spaceID,
		"room_id", roomID,
		"filename", filename,
		"content_type", contentType,
		"size", size,
		"storage_backend", c.config.Assets.StorageBackend,
	)

	return &corev1.Attachment{
		Id:          attachmentID,
		SpaceId:     spaceID,
		RoomId:      roomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        size,
		Width:       width,
		Height:      height,
		Storage:     storage,
	}, nil
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
func (c *ChattoCore) GetAttachment(ctx context.Context, spaceID, attachmentID string) (io.Reader, *jetstream.ObjectInfo, error) {
	store, err := c.GetAttachmentsStore(ctx, spaceID)
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

// GetAttachmentFromAnyBackend retrieves an attachment by probing both NATS and S3 backends.
// It tries NATS first (for backwards compatibility with existing attachments), then S3.
// Returns a reader for the attachment content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetAttachmentFromAnyBackend(ctx context.Context, spaceID, attachmentID string) (io.Reader, *AttachmentInfo, error) {
	// Try NATS first (backwards compatibility)
	reader, info, err := c.GetAttachment(ctx, spaceID, attachmentID)
	if err == nil {
		return reader, &AttachmentInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
			Filename:    info.Headers.Get("Filename"),
			RoomID:      info.Headers.Get("Room-Id"),
		}, nil
	}

	// If NATS failed and S3 is configured, try S3
	if c.s3Client != nil {
		s3Key := S3KeySpaceAttachment(spaceID, attachmentID)
		s3Reader, s3Info, s3Err := c.GetS3Attachment(ctx, s3Key)
		if s3Err == nil {
			return s3Reader, s3Info, nil
		}
		// Log S3 error but return the original NATS error
		c.logger.Debug("Attachment not found in either backend",
			"space_id", spaceID,
			"attachment_id", attachmentID,
			"nats_error", err,
			"s3_error", s3Err)
	}

	return nil, nil, err
}

// DeleteAttachment deletes a NATS attachment and its cached resizes.
// This is the legacy path for attachments stored in NATS ObjectStore.
// Use DeleteAttachmentFromStorage for attachments with known storage type.
func (c *ChattoCore) DeleteAttachment(ctx context.Context, spaceID, attachmentID string) error {
	store, err := c.GetAttachmentsStore(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get attachments store: %w", err)
	}

	err = store.Delete(ctx, attachmentID)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}

	// Delete any cached resizes for this attachment (best-effort, don't fail on error)
	deletedCount, cacheErr := c.DeleteCachedResizesForAttachment(ctx, spaceID, attachmentID)
	if cacheErr != nil {
		c.logger.Warn("Failed to delete cached resizes for attachment",
			"attachment_id", attachmentID,
			"space_id", spaceID,
			"error", cacheErr)
	} else if deletedCount > 0 {
		c.logger.Debug("Deleted cached resizes for attachment",
			"attachment_id", attachmentID,
			"space_id", spaceID,
			"deleted_count", deletedCount)
	}

	c.logger.Debug("Deleted attachment", "attachment_id", attachmentID, "space_id", spaceID)

	return nil
}

// DeleteAttachmentFromStorage deletes an attachment based on its storage type.
// Handles both NATS ObjectStore and S3 storage.
func (c *ChattoCore) DeleteAttachmentFromStorage(ctx context.Context, attachment *corev1.Attachment) error {
	// Check storage type - if nil or NATS, use legacy deletion
	if attachment.Storage == nil {
		return c.DeleteAttachment(ctx, attachment.SpaceId, attachment.Id)
	}

	switch storage := attachment.Storage.Asset.(type) {
	case *corev1.Asset_Nats:
		return c.DeleteAttachment(ctx, attachment.SpaceId, attachment.Id)
	case *corev1.Asset_S3:
		// Delete from S3
		if c.s3Client == nil {
			return fmt.Errorf("S3 client not configured")
		}
		if err := c.s3Client.DeleteObjectFromBucket(ctx, storage.S3.GetBucket(), storage.S3.Key); err != nil {
			return fmt.Errorf("failed to delete S3 attachment: %w", err)
		}
		// Delete any cached resizes (S3 attachments use the S3 key as cache prefix)
		deletedCount, cacheErr := c.DeleteCachedResizesForKey(ctx, "s3", storage.S3.Key)
		if cacheErr != nil {
			c.logger.Warn("Failed to delete cached resizes for S3 attachment",
				"s3_key", storage.S3.Key,
				"error", cacheErr)
		} else if deletedCount > 0 {
			c.logger.Debug("Deleted cached resizes for S3 attachment",
				"s3_key", storage.S3.Key,
				"deleted_count", deletedCount)
		}
		c.logger.Debug("Deleted S3 attachment", "s3_key", storage.S3.Key)
		return nil
	default:
		return c.DeleteAttachment(ctx, attachment.SpaceId, attachment.Id)
	}
}

// TryPresignedAttachmentURL attempts to generate a presigned S3 URL for an attachment.
// Returns the URL string if the attachment exists in S3, or an error if S3 is not
// configured or the attachment is not found in S3.
func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, spaceID, attachmentID string) (string, error) {
	if c.s3Client == nil {
		return "", fmt.Errorf("S3 not configured")
	}

	s3Key := S3KeySpaceAttachment(spaceID, attachmentID)

	// Stat to verify the object exists (presigned URLs don't check existence)
	if _, err := c.s3Client.StatObject(ctx, s3Key); err != nil {
		return "", err
	}

	presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3Key, time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// GetAttachmentURL returns the URL for accessing an attachment.
// For legacy attachments without storage info, defaults to NATS path.
func (c *ChattoCore) GetAttachmentURL(spaceID, attachmentID string) string {
	return c.assetURL(fmt.Sprintf("/assets/space/%s/attachments/%s", spaceID, attachmentID))
}

// GetAttachmentURLFromStorage returns the URL for accessing an attachment.
// The URL is always the standard attachment URL format regardless of storage backend.
// Storage backend is an internal implementation detail that should not leak into URLs.
func (c *ChattoCore) GetAttachmentURLFromStorage(attachment *corev1.Attachment) string {
	// Always use the standard attachment URL - the HTTP handler will determine storage
	return c.GetAttachmentURL(attachment.SpaceId, attachment.Id)
}

// GetTransformedAttachmentURL returns the URL for accessing a transformed version of an attachment.
// The URL includes HMAC signature to prevent parameter tampering.
// Format: /assets/space/{spaceId}/attachments/{attachmentId}/t/{params}.{signature}
// where {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}
func (c *ChattoCore) GetTransformedAttachmentURL(spaceID, attachmentID string, width, height int, fit string) string {
	// Generate signed transform path component
	signedPath := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, spaceID, attachmentID, width, height, fit)

	// Return signed transform URL with base64 path
	return c.assetURL(fmt.Sprintf("/assets/space/%s/attachments/%s/t/%s",
		spaceID, attachmentID, signedPath))
}

// GetTransformedAttachmentURLFromStorage returns the URL for accessing a transformed version of an attachment.
// The URL is always the standard transformed attachment URL format regardless of storage backend.
// Storage backend is an internal implementation detail that should not leak into URLs.
func (c *ChattoCore) GetTransformedAttachmentURLFromStorage(attachment *corev1.Attachment, width, height int, fit string) string {
	// Always use the standard transformed attachment URL - the HTTP handler will determine storage
	return c.GetTransformedAttachmentURL(attachment.SpaceId, attachment.Id, width, height, fit)
}

// GetTransformedServerAssetURL returns the URL for accessing a transformed version of an instance asset.
// Instance assets include space logos, space banners, and user avatars stored in the instance object store.
// The URL includes HMAC signature to prevent parameter tampering.
// Format: /assets/instance/{key}/t/{params}.{signature}
// where {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}
func (c *ChattoCore) GetTransformedServerAssetURL(key string, width, height int, fit string) string {
	// Generate signed transform path component using "instance" as the first resource ID
	signedPath := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, "instance", key, width, height, fit)

	// Return signed transform URL
	return c.assetURL(fmt.Sprintf("/assets/instance/%s/t/%s", key, signedPath))
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

// DeleteCachedResizesForAttachment deletes all cached resizes for an attachment.
// Returns the number of deleted cache entries and any error encountered.
// Does nothing if the cache is disabled.
func (c *ChattoCore) DeleteCachedResizesForAttachment(ctx context.Context, spaceID, attachmentID string) (int, error) {
	// Cache keys follow the pattern: {spaceId}.{attachmentId}.{paramsHash}
	return c.DeleteCachedResizesForKey(ctx, spaceID, attachmentID)
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
func (c *ChattoCore) GetVideoProcessingState(ctx context.Context, spaceID, attachmentID string) (*corev1.VideoProcessingState, error) {
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
func (c *ChattoCore) SetVideoProcessingState(ctx context.Context, spaceID, attachmentID string, state *corev1.VideoProcessingState) error {
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
func (c *ChattoCore) InitVideoProcessingState(ctx context.Context, spaceID, attachmentID string) error {
	return c.SetVideoProcessingState(ctx, spaceID, attachmentID, &corev1.VideoProcessingState{
		Status: corev1.VideoStatus_VIDEO_STATUS_PENDING,
	})
}

// PublishVideoProcessingRequest publishes a video processing request to NATS.
// Call this AFTER PostMessage, once the messageBodyID is known.
func (c *ChattoCore) PublishVideoProcessingRequest(ctx context.Context, spaceID, roomID, attachmentID, contentType, messageBodyID string) error {
	payload := struct {
		SpaceID       string `json:"space_id"`
		RoomID        string `json:"room_id"`
		AttachmentID  string `json:"attachment_id"`
		ContentType   string `json:"content_type"`
		MessageBodyID string `json:"message_body_id"`
	}{
		SpaceID:       spaceID,
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
		"space_id", spaceID,
		"attachment_id", attachmentID,
	)

	return nil
}

// PublishVideoProcessingCompleted publishes a live event indicating video processing is done.
// The frontend subscription receives this and refreshes the affected message.
func (c *ChattoCore) PublishVideoProcessingCompleted(ctx context.Context, spaceID, roomID, attachmentID, messageBodyID string) error {
	event := newServerEvent("", &corev1.ServerEvent{
		Event: &corev1.ServerEvent_VideoProcessingCompleted{
			VideoProcessingCompleted: &corev1.VideoProcessingCompletedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				AttachmentId:   attachmentID,
				MessageBodyId:  messageBodyID,
				MessageEventId: eventIDFromBodyKey(messageBodyID),
			},
		},
	})

	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "video_processed")
	return c.publishLiveServerEvent(ctx, subject, event)
}

// DeleteAttachmentFromStorageByID deletes an attachment from storage by space ID and attachment ID.
// This probes both NATS and S3 backends. Used by the video service to delete the original
// after successful transcoding.
func (c *ChattoCore) DeleteAttachmentFromStorageByID(ctx context.Context, spaceID, attachmentID string) error {
	// Try NATS first
	err := c.DeleteAttachment(ctx, spaceID, attachmentID)
	if err == nil {
		return nil
	}

	// Try S3
	if c.s3Client != nil {
		s3Key := S3KeySpaceAttachment(spaceID, attachmentID)
		if s3Err := c.s3Client.DeleteObject(ctx, s3Key); s3Err == nil {
			return nil
		}
	}

	return err
}
