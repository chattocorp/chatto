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
// `Attachment.SpaceId` is left empty on the returned proto — it's
// reserved for pre-ADR-030-Phase-4 records whose S3 objects still live
// at `spaces/{server|DM}/attachments/{id}` and is consulted only by the
// S3-key fallback probe (`attachmentS3KeyCandidates`).
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

	// Write the canonical Attachment record. The asset HTTP handler
	// authorizes downloads off this record, so a failure here would
	// strand the uploaded binary unreachable behind a 404. Best-effort
	// delete the just-uploaded binary before surfacing the error, so
	// we don't leak orphans into storage.
	if err := c.recordAttachment(ctx, attachment); err != nil {
		if cleanupErr := c.deleteAttachmentBinary(ctx, attachmentID, storage); cleanupErr != nil {
			c.logger.Warn("Failed to clean up attachment binary after metadata write failure",
				"attachment_id", attachmentID,
				"error", cleanupErr)
		}
		return nil, fmt.Errorf("failed to record attachment metadata: %w", err)
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

// attachmentS3Key returns the preferred S3 key for an attachment. New
// attachments (with no SpaceId) use the kind-less layout; legacy
// attachments stay at their pre-ADR-030-Phase-4 layout
// (`spaces/{server|DM}/attachments/{id}`).
func attachmentS3Key(spaceID, attachmentID string) string {
	if spaceID == "" {
		return S3KeyAttachment(attachmentID)
	}
	return S3KeySpaceAttachment(spaceID, attachmentID)
}

// attachmentS3KeyCandidates returns the S3 key to try first plus any
// fallbacks. We probe across layouts so layout mismatches between caller
// hint and actual storage (e.g. a legacy in-flight video processing
// request handled by a Phase-4 binary, or a new variant attached to a
// legacy parent video) resolve transparently.
func attachmentS3KeyCandidates(spaceID, attachmentID string) []string {
	if spaceID == "" {
		// Prefer the new layout but fall back to known legacy spaceIDs.
		return []string{
			S3KeyAttachment(attachmentID),
			S3KeySpaceAttachment(ServerSpaceID, attachmentID),
			S3KeySpaceAttachment(DMSpaceID, attachmentID),
		}
	}
	// Prefer the requested legacy layout, fall back to the new one.
	return []string{
		S3KeySpaceAttachment(spaceID, attachmentID),
		S3KeyAttachment(attachmentID),
	}
}

// GetAttachmentFromAnyBackend retrieves an attachment by probing both NATS and S3 backends.
// It tries NATS first (for backwards compatibility with existing attachments), then S3.
// An empty spaceID selects the kind-less S3 layout used for new uploads;
// a non-empty spaceID selects the legacy `spaces/{spaceId}/...` layout.
// Returns a reader for the attachment content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetAttachmentFromAnyBackend(ctx context.Context, spaceID, attachmentID string) (io.Reader, *AttachmentInfo, error) {
	// Try NATS first (backwards compatibility)
	reader, info, err := c.GetAttachment(ctx, attachmentID)
	if err == nil {
		return reader, &AttachmentInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
			Filename:    info.Headers.Get("Filename"),
			RoomID:      info.Headers.Get("Room-Id"),
		}, nil
	}

	// If NATS failed and S3 is configured, try S3. Probe across layouts
	// so legacy-vs-new mismatches in the caller's hint resolve cleanly.
	if c.s3Client != nil {
		var lastS3Err error
		for _, s3Key := range attachmentS3KeyCandidates(spaceID, attachmentID) {
			s3Reader, s3Info, s3Err := c.GetS3Attachment(ctx, s3Key)
			if s3Err == nil {
				return s3Reader, s3Info, nil
			}
			lastS3Err = s3Err
		}
		c.logger.Debug("Attachment not found in either backend",
			"space_id", spaceID,
			"attachment_id", attachmentID,
			"nats_error", err,
			"s3_error", lastS3Err)
	}

	return nil, nil, err
}

// ============================================================================
// Attachment Records
// ============================================================================
//
// Attachment metadata records live in SERVER_BODIES alongside the
// message bodies that reference them. They share the bucket because:
//
//   - The lifecycle is the same: an attachment is born with the body
//     that posts it and dies when that body is deleted (incl. GDPR).
//   - The write profile is the same (per-message churn), so we don't
//     have to provision another replicated stream for an essentially
//     identical workload.
//
// Two key shapes coexist in this bucket — they don't overlap because
// body keys have two tokens and attachment-record keys have three:
//
//   - {userId}.{bodyId}                       → marshaled MessageBody
//   - attachment.{roomId}.{attachmentId}      → marshaled Attachment
//
// Room ID lives in the key so the per-room attachment sidebar can
// prefix-filter (`attachment.{roomId}.*`); by-ID lookup uses a
// server-side wildcard filter (`attachment.*.{attachmentId}`).
//
// TODO: rename SERVER_BODIES → SERVER_CONTENT now that it hosts more
// than bodies. Pending a separate migration since renaming a bucket
// requires copying every key.

// attachmentRecordKey returns the SERVER_BODIES key for an attachment
// metadata record. The "attachment." literal prefix keeps these keys
// out of the way of body keys (`{userId}.{bodyId}`).
func attachmentRecordKey(roomID, attachmentID string) string {
	return "attachment." + roomID + "." + attachmentID
}

// recordAttachment stores the canonical Attachment metadata record.
func (c *ChattoCore) recordAttachment(ctx context.Context, attachment *corev1.Attachment) error {
	if attachment == nil || attachment.Id == "" || attachment.RoomId == "" {
		return fmt.Errorf("recordAttachment: missing id or room id")
	}
	data, err := proto.Marshal(attachment)
	if err != nil {
		return fmt.Errorf("marshal attachment: %w", err)
	}
	if _, err := c.storage.serverBodiesKV.Put(ctx, attachmentRecordKey(attachment.RoomId, attachment.Id), data); err != nil {
		return fmt.Errorf("write attachment record: %w", err)
	}
	return nil
}

// GetAttachmentRecord returns the canonical Attachment metadata for the
// given ID, or (nil, nil) if no record exists. Uses a server-side
// wildcard filter on the key (`attachment.*.{attachmentId}`).
func (c *ChattoCore) GetAttachmentRecord(ctx context.Context, attachmentID string) (*corev1.Attachment, error) {
	if attachmentID == "" {
		return nil, nil
	}
	lister, err := c.storage.serverBodiesKV.ListKeysFiltered(ctx, "attachment.*."+attachmentID)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("filter attachment keys: %w", err)
	}
	for key := range lister.Keys() {
		entry, err := c.storage.serverBodiesKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get attachment record: %w", err)
		}
		var att corev1.Attachment
		if err := proto.Unmarshal(entry.Value(), &att); err != nil {
			return nil, fmt.Errorf("unmarshal attachment record: %w", err)
		}
		return &att, nil
	}
	return nil, nil
}

// ListRoomAttachments returns every Attachment metadata record posted
// to the given room. Used by the per-room attachment sidebar. Reads
// every matching record into memory — adequate for the expected
// sidebar size; rooms with huge attachment counts will eventually want
// a pagination cursor.
func (c *ChattoCore) ListRoomAttachments(ctx context.Context, roomID string) ([]*corev1.Attachment, error) {
	if roomID == "" {
		return nil, nil
	}
	lister, err := c.storage.serverBodiesKV.ListKeysFiltered(ctx, "attachment."+roomID+".*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("list room attachments: %w", err)
	}
	var out []*corev1.Attachment
	for key := range lister.Keys() {
		entry, err := c.storage.serverBodiesKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get attachment record: %w", err)
		}
		var att corev1.Attachment
		if err := proto.Unmarshal(entry.Value(), &att); err != nil {
			c.logger.Warn("Skipping unparseable attachment record", "key", key, "error", err)
			continue
		}
		out = append(out, &att)
	}
	return out, nil
}

// deleteAttachmentBinary removes the storage object for a freshly
// uploaded attachment. Used to clean up after a metadata-write failure
// in UploadAttachment so we don't strand orphans in storage.
func (c *ChattoCore) deleteAttachmentBinary(ctx context.Context, attachmentID string, storage *corev1.Asset) error {
	if storage == nil {
		return nil
	}
	switch asset := storage.Asset.(type) {
	case *corev1.Asset_S3:
		if c.s3Client == nil {
			return nil
		}
		return c.s3Client.DeleteObject(ctx, asset.S3.Key)
	case *corev1.Asset_Nats:
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return err
		}
		return store.Delete(ctx, attachmentID)
	}
	return nil
}

// forgetAttachmentRecord removes the attachment metadata record.
// Missing entries are not an error.
func (c *ChattoCore) forgetAttachmentRecord(ctx context.Context, roomID, attachmentID string) error {
	if roomID == "" || attachmentID == "" {
		return nil
	}
	if err := c.storage.serverBodiesKV.Delete(ctx, attachmentRecordKey(roomID, attachmentID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("delete attachment record: %w", err)
	}
	return nil
}

// DeleteAttachment deletes a NATS attachment and its cached resizes.
// This is the legacy path for attachments stored in NATS ObjectStore.
// Use DeleteAttachmentFromStorage for attachments with known storage type.
func (c *ChattoCore) DeleteAttachment(ctx context.Context, spaceID, attachmentID string) error {
	store, err := c.GetAttachmentsStore(ctx)
	if err != nil {
		return fmt.Errorf("failed to get attachments store: %w", err)
	}

	// Resolve the room ID via the canonical record before we delete the
	// underlying object, so we can also drop the metadata entry. If no
	// record exists (legacy orphan), we still proceed with the storage
	// delete — the record cleanup is best-effort and silently no-ops.
	var roomID string
	if rec, recErr := c.GetAttachmentRecord(ctx, attachmentID); recErr == nil && rec != nil {
		roomID = rec.RoomId
	}

	err = store.Delete(ctx, attachmentID)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}

	if recErr := c.forgetAttachmentRecord(ctx, roomID, attachmentID); recErr != nil {
		c.logger.Warn("Failed to forget attachment record",
			"attachment_id", attachmentID,
			"error", recErr)
	}

	// Delete any cached resizes for this attachment (best-effort, don't fail on error)
	deletedCount, cacheErr := c.DeleteCachedResizesForAttachment(ctx, attachmentID)
	if cacheErr != nil {
		c.logger.Warn("Failed to delete cached resizes for attachment",
			"attachment_id", attachmentID,
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
		if recErr := c.forgetAttachmentRecord(ctx, attachment.RoomId, attachment.Id); recErr != nil {
			c.logger.Warn("Failed to forget attachment record",
				"attachment_id", attachment.Id,
				"error", recErr)
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

// TryPresignedAttachmentURL attempts to generate a presigned S3 URL for an
// attachment. Returns the URL string if the attachment exists in S3, or an
// error if S3 is not configured or the attachment is not found in S3.
// Pass an empty spaceID to prefer the post-ADR-030-Phase-4 kind-less layout;
// the helper falls back to the legacy `spaces/{server|DM}/...` keys for
// layout mismatches.
//
// Authorization is the caller's responsibility — the room ID comes from
// the canonical Attachment record (see GetAttachmentRecord), not from S3.
func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, spaceID, attachmentID string) (string, error) {
	if c.s3Client == nil {
		return "", fmt.Errorf("S3 not configured")
	}

	var lastErr error
	for _, s3Key := range attachmentS3KeyCandidates(spaceID, attachmentID) {
		// Stat to verify the object exists (presigned URLs don't check existence)
		if _, err := c.s3Client.StatObject(ctx, s3Key); err != nil {
			lastErr = err
			continue
		}
		presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3Key, time.Hour)
		if err != nil {
			return "", fmt.Errorf("failed to generate presigned URL: %w", err)
		}
		return presignedURL.String(), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("attachment not found")
	}
	return "", lastErr
}

// AttachmentSignResource is the first resource component fed to the
// signed-URL signer for attachment transform URLs. Stable so existing
// signatures continue to verify across deployments.
const AttachmentSignResource = "attachment"

// GetAttachmentURL returns the URL for accessing an attachment. All
// attachments — including those whose S3 objects still live at the
// pre-ADR-030-Phase-4 `spaces/{server|DM}/attachments/{id}` layout —
// are served through the kind-less `/assets/attachments/{id}` URL.
// The HTTP handler probes both storage layouts to resolve the binary.
func (c *ChattoCore) GetAttachmentURL(attachmentID string) string {
	return c.assetURL(fmt.Sprintf("/assets/attachments/%s", attachmentID))
}

// GetAttachmentURLFromStorage returns the URL for accessing an attachment.
// Storage backend is an internal implementation detail that does not leak
// into URLs.
func (c *ChattoCore) GetAttachmentURLFromStorage(attachment *corev1.Attachment) string {
	return c.GetAttachmentURL(attachment.Id)
}

// GetTransformedAttachmentURL returns the URL for accessing a transformed
// version of an attachment. The URL includes an HMAC signature to prevent
// parameter tampering.
//
//	/assets/attachments/{attachmentId}/t/{params}.{signature}
//
// {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}.
func (c *ChattoCore) GetTransformedAttachmentURL(attachmentID string, width, height int, fit string) string {
	signedPath := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, AttachmentSignResource, attachmentID, width, height, fit)
	return c.assetURL(fmt.Sprintf("/assets/attachments/%s/t/%s", attachmentID, signedPath))
}

// GetTransformedAttachmentURLFromStorage returns a transform URL for an
// attachment record.
func (c *ChattoCore) GetTransformedAttachmentURLFromStorage(attachment *corev1.Attachment, width, height int, fit string) string {
	return c.GetTransformedAttachmentURL(attachment.Id, width, height, fit)
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

// DeleteAttachmentFromStorageByID deletes an attachment from storage by
// space ID and attachment ID. This probes both NATS and S3 backends. Used
// by the video service to delete the original after successful transcoding.
// Pass an empty spaceID to prefer the post-ADR-030-Phase-4 kind-less layout;
// the helper probes all known S3 layouts so layout mismatches resolve
// transparently.
func (c *ChattoCore) DeleteAttachmentFromStorageByID(ctx context.Context, spaceID, attachmentID string) error {
	// Try NATS first
	err := c.DeleteAttachment(ctx, spaceID, attachmentID)
	if err == nil {
		return nil
	}

	// Try S3 across known layouts.
	if c.s3Client != nil {
		// Resolve the room ID via the canonical record so we can clean
		// up the metadata entry alongside the storage object.
		var roomID string
		if rec, recErr := c.GetAttachmentRecord(ctx, attachmentID); recErr == nil && rec != nil {
			roomID = rec.RoomId
		}
		for _, s3Key := range attachmentS3KeyCandidates(spaceID, attachmentID) {
			if s3Err := c.s3Client.DeleteObject(ctx, s3Key); s3Err == nil {
				if recErr := c.forgetAttachmentRecord(ctx, roomID, attachmentID); recErr != nil {
					c.logger.Warn("Failed to forget attachment record",
						"attachment_id", attachmentID,
						"error", recErr)
				}
				return nil
			}
		}
	}

	return err
}
