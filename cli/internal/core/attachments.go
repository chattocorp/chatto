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
	"hmans.de/chatto/internal/events"
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
// `Attachment.MessageBodyId` is left empty here — the body key is set
// later in PostMessage when the owning MessageBody is written.
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
	var storage *corev1.AssetStorage
	if c.ShouldUseS3() {
		// Upload to S3
		s3Key := S3KeyAttachment(attachmentID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, content, contentType)
		if err != nil {
			return nil, fmt.Errorf("failed to upload attachment to S3: %w", err)
		}
		storage = &corev1.AssetStorage{
			Asset: &corev1.AssetStorage_S3{
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
		storage = &corev1.AssetStorage{
			Asset: &corev1.AssetStorage_Nats{
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
// When `Storage` is nil, falls back to probing known backend layouts
// for the binary by attachment ID — this handles pre-locator video
// variants and thumbnails whose backfilled Attachment protos came from
// minimal standalone records that lacked a `Storage` field.
func (c *ChattoCore) GetAttachmentReader(ctx context.Context, attachment *corev1.Attachment) (io.Reader, *AttachmentInfo, error) {
	if attachment == nil {
		return nil, nil, fmt.Errorf("attachment is nil")
	}
	if attachment.Storage == nil {
		return c.probeAttachmentReaderByID(ctx, attachment.Id)
	}
	switch asset := attachment.Storage.Asset.(type) {
	case *corev1.AssetStorage_Nats:
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
	case *corev1.AssetStorage_S3:
		if c.s3Client == nil {
			return nil, nil, fmt.Errorf("S3 client not configured")
		}
		return c.GetS3Attachment(ctx, asset.S3.Key)
	default:
		return nil, nil, fmt.Errorf("attachment %s has unknown storage backend", attachment.Id)
	}
}

// probeAttachmentReaderByID is the fallback when an Attachment's
// `Storage` field isn't populated. Tries NATS ObjectStore first (where
// pre-S3 uploads landed), then S3 across the post-Phase-4 kind-less
// key and the pre-Phase-4 server/DM-prefixed legacy keys. Whichever
// backend has the binary wins.
//
// This is load-bearing for pre-locator video variants and thumbnails:
// `BackfillAttachmentRecords` (long-since deleted) wrote minimal
// `Attachment{Id, RoomId}` records for those, and
// `BackfillAttachmentLocatorData` copied those minimal records into
// legacy `VideoProcessingState.{ThumbnailAttachment, Variants[i].Attachment}`.
// The EVT manifest migration can import those protos; they have no Storage,
// so we probe.
func (c *ChattoCore) probeAttachmentReaderByID(ctx context.Context, attachmentID string) (io.Reader, *AttachmentInfo, error) {
	reader, natsInfo, err := c.GetAttachment(ctx, attachmentID)
	if err == nil {
		return reader, &AttachmentInfo{
			Size:        int64(natsInfo.Size),
			ContentType: natsInfo.Headers.Get("Content-Type"),
			Filename:    natsInfo.Headers.Get("Filename"),
			RoomID:      natsInfo.Headers.Get("Room-Id"),
		}, nil
	}
	if c.s3Client != nil {
		for _, s3Key := range legacyAttachmentS3KeyCandidates(attachmentID) {
			s3Reader, s3Info, s3Err := c.GetS3Attachment(ctx, s3Key)
			if s3Err == nil {
				return s3Reader, s3Info, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("attachment not found: %s", attachmentID)
}

// legacyAttachmentS3KeyCandidates returns the S3 keys to try when we
// don't know an attachment's storage layout: post-Phase-4 first, then
// the wire-frozen pre-Phase-4 server/DM prefixes.
func legacyAttachmentS3KeyCandidates(attachmentID string) []string {
	return []string{
		S3KeyAttachment(attachmentID),
		"spaces/server/attachments/" + attachmentID,
		"spaces/DM/attachments/" + attachmentID,
	}
}

func assetFromAttachment(attachment *corev1.Attachment) *corev1.Asset {
	if attachment == nil {
		return nil
	}
	asset := &corev1.Asset{
		Id:          attachment.GetId(),
		Filename:    attachment.GetFilename(),
		ContentType: attachment.GetContentType(),
		Size:        attachment.GetSize(),
	}
	applyAssetStorageFromAttachmentStorage(asset, attachment.GetStorage())
	applyAssetMetadataFromAttachment(asset, attachment)
	return asset
}

func attachmentFromAsset(asset *corev1.Asset) *corev1.Attachment {
	if asset == nil {
		return nil
	}
	width, height := assetDimensions(asset)
	return &corev1.Attachment{
		Id:          asset.GetId(),
		Filename:    asset.GetFilename(),
		ContentType: asset.GetContentType(),
		Size:        asset.GetSize(),
		Width:       width,
		Height:      height,
		Storage:     assetStorageFromAsset(asset),
	}
}

func applyAssetStorageFromAttachmentStorage(asset *corev1.Asset, storage *corev1.AssetStorage) {
	if asset == nil || storage == nil {
		return
	}
	switch stored := storage.GetAsset().(type) {
	case *corev1.AssetStorage_Nats:
		if stored.Nats != nil {
			asset.Storage = &corev1.Asset_Nats{Nats: proto.Clone(stored.Nats).(*corev1.NATSAsset)}
		}
	case *corev1.AssetStorage_S3:
		if stored.S3 != nil {
			asset.Storage = &corev1.Asset_S3{S3: proto.Clone(stored.S3).(*corev1.S3Asset)}
		}
	}
}

func assetStorageFromAsset(asset *corev1.Asset) *corev1.AssetStorage {
	if asset == nil {
		return nil
	}
	switch {
	case asset.GetNats() != nil:
		return &corev1.AssetStorage{
			Asset: &corev1.AssetStorage_Nats{Nats: proto.Clone(asset.GetNats()).(*corev1.NATSAsset)},
		}
	case asset.GetS3() != nil:
		return &corev1.AssetStorage{
			Asset: &corev1.AssetStorage_S3{S3: proto.Clone(asset.GetS3()).(*corev1.S3Asset)},
		}
	default:
		return nil
	}
}

func applyAssetMetadataFromAttachment(asset *corev1.Asset, attachment *corev1.Attachment) {
	if asset == nil || attachment == nil || (attachment.GetWidth() == 0 && attachment.GetHeight() == 0) {
		return
	}
	asset.Width = attachment.GetWidth()
	asset.Height = attachment.GetHeight()
}

func assetDimensions(asset *corev1.Asset) (int32, int32) {
	if asset == nil {
		return 0, 0
	}
	return asset.GetWidth(), asset.GetHeight()
}

func cloneAssetStorage(storage *corev1.AssetStorage) *corev1.AssetStorage {
	if storage == nil {
		return nil
	}
	return proto.Clone(storage).(*corev1.AssetStorage)
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
	// Post-#597, the body lives embedded on the event. bodyKey is now
	// the message's event_id (or the legacy {userId}.{eventId} compound
	// key — eventIDFromBodyKey normalizes both).
	eventID := eventIDFromBodyKey(bodyKey)
	body, retracted, ok := c.RoomTimeline.LatestBody(eventID)
	if !ok || retracted || body == nil {
		return nil, nil
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
// from the durable video manifest keyed by the original video's attachment
// ID. Returns (nil, nil) if the manifest is missing or doesn't contain an
// attachment with the given ID.
func (c *ChattoCore) FindVideoOriginAttachment(ctx context.Context, videoOriginID, attachmentID string) (*corev1.Attachment, error) {
	if videoOriginID == "" || attachmentID == "" {
		return nil, nil
	}
	manifest, ok := c.RoomTimeline.VideoAttachmentManifest(videoOriginID)
	if !ok || manifest == nil || manifest.Succeeded == nil {
		return nil, nil
	}
	video := manifest.Succeeded.GetVideo()
	if video == nil {
		return nil, nil
	}
	if video.GetThumbnailAssetId() == attachmentID {
		if declared, ok := c.RoomTimeline.AssetCreation(attachmentID); ok {
			return attachmentFromAsset(declared.GetAsset()), nil
		}
	}
	for _, v := range video.Variants {
		if v.GetAssetId() == attachmentID {
			if declared, ok := c.RoomTimeline.AssetCreation(attachmentID); ok {
				return attachmentFromAsset(declared.GetAsset()), nil
			}
		}
	}
	return nil, nil
}

// LookupAttachment resolves any attachment by its URL locator, choosing
// the right source of truth (`MessageBody.Attachments` for body attachments,
// projected video manifests for variants/thumbnails).
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
	case *corev1.AssetStorage_Nats:
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return fmt.Errorf("failed to get attachments store: %w", err)
		}
		if err := store.Delete(ctx, storage.Nats.Key); err != nil {
			return fmt.Errorf("failed to delete attachment from NATS: %w", err)
		}
		c.logger.Debug("Deleted NATS attachment", "attachment_id", attachment.Id, "key", storage.Nats.Key)
	case *corev1.AssetStorage_S3:
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

// DeleteVideoDerivativesForAttachment deletes generated thumbnail/variant
// binaries for a processed video attachment. The durable manifest remains in
// EVT for audit/replay; deletion makes future signed URLs resolve to 404.
func (c *ChattoCore) DeleteVideoDerivativesForAttachment(ctx context.Context, attachmentID string) {
	manifest, ok := c.RoomTimeline.VideoAttachmentManifest(attachmentID)
	if !ok || manifest == nil || manifest.Succeeded == nil {
		return
	}
	video := manifest.Succeeded.GetVideo()
	if video == nil {
		return
	}
	if declared, ok := c.RoomTimeline.AssetCreation(video.GetThumbnailAssetId()); ok {
		thumbnail := attachmentFromAsset(declared.GetAsset())
		if err := c.DeleteAttachmentFromStorage(ctx, thumbnail); err != nil {
			c.logger.Warn("Failed to delete video thumbnail derivative",
				"attachment_id", thumbnail.GetId(),
				"origin_attachment_id", attachmentID,
				"error", err)
		}
	}
	for _, variant := range video.Variants {
		if variant.GetAssetId() == "" {
			continue
		}
		declared, ok := c.RoomTimeline.AssetCreation(variant.GetAssetId())
		if !ok {
			continue
		}
		attachment := attachmentFromAsset(declared.GetAsset())
		if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			c.logger.Warn("Failed to delete video variant derivative",
				"attachment_id", attachment.GetId(),
				"origin_attachment_id", attachmentID,
				"error", err)
		}
	}
}

// TryPresignedAttachmentURL generates a presigned S3 URL for an
// attachment. Returns an error if S3 isn't configured, the attachment
// isn't stored in S3 (e.g. NATS), or no S3 key can be found (in any of
// which cases the caller should fall back to GetAttachmentReader).
//
// When `Storage` is nil, falls back to stat-probing known S3 key
// layouts. See `GetAttachmentReader` for why this exists.
//
// Authorization is the caller's responsibility.
func (c *ChattoCore) TryPresignedAttachmentURL(ctx context.Context, attachment *corev1.Attachment) (string, error) {
	if c.s3Client == nil {
		return "", fmt.Errorf("S3 not configured")
	}
	if attachment == nil {
		return "", fmt.Errorf("attachment is nil")
	}
	if attachment.Storage == nil {
		return c.probePresignedAttachmentURL(ctx, attachment.Id)
	}
	s3, ok := attachment.Storage.Asset.(*corev1.AssetStorage_S3)
	if !ok {
		return "", fmt.Errorf("attachment %s is not stored in S3", attachment.Id)
	}
	presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3.S3.Key, time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

// probePresignedAttachmentURL is the fallback when an Attachment's
// `Storage` field isn't populated and the binary might live in S3.
// Stats the known key layouts; presigns the first hit.
func (c *ChattoCore) probePresignedAttachmentURL(ctx context.Context, attachmentID string) (string, error) {
	for _, s3Key := range legacyAttachmentS3KeyCandidates(attachmentID) {
		if _, err := c.s3Client.StatObject(ctx, s3Key); err != nil {
			continue
		}
		presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3Key, time.Hour)
		if err != nil {
			return "", fmt.Errorf("failed to generate presigned URL: %w", err)
		}
		return presignedURL.String(), nil
	}
	return "", fmt.Errorf("attachment %s not found in S3", attachmentID)
}

// AttachmentSignResource is the first resource component fed to the
// signed-URL signer for attachment transform URLs (after the locator).
// Stable so existing signatures continue to verify across deployments.
const AttachmentSignResource = "attachment"

// AttachmentURLTTL is how long an attachment URL stays valid after it's
// signed. Short on purpose: the signed locator is a standalone capability
// (no session/bearer check at the asset endpoint — see ADR-032 and
// authorization.md), so a leaked URL grants access for the full TTL. We
// keep it just long enough for an in-flight render to complete; the
// frontend regenerates URLs by re-resolving GraphQL when needed.
//
// We treat this as a stopgap for cross-origin remote-server <img>
// loading rather than a real cross-origin auth design. See the
// "Attachment URL Authorization" section of authorization.md for the
// trade-off in detail.
const AttachmentURLTTL = 5 * time.Minute

// GetAttachmentURL returns the URL for accessing the binary identified
// by the locator, signed for `userID` with a `AttachmentURLTTL`-bounded
// expiry. The URL itself is the capability: the handler trusts the
// signed claims (signature + expiry + room-membership check) and does
// not require a session cookie or bearer header. This is what lets
// cross-origin <img> tags work for remote-server attachments.
//
// Returns an empty string if `userID` is empty or the locator is
// otherwise invalid (a programmer error — locators come from trusted
// resolver code, not user input).
func (c *ChattoCore) GetAttachmentURL(loc signedurl.AttachmentLocator, userID string) string {
	loc.UserID = userID
	loc.ExpiresAt = time.Now().Add(AttachmentURLTTL).Unix()
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
//
// `userID` and `AttachmentURLTTL`-bounded expiry are baked into the
// signed locator — see GetAttachmentURL.
func (c *ChattoCore) GetTransformedAttachmentURL(loc signedurl.AttachmentLocator, userID string, width, height int, fit string) string {
	loc.UserID = userID
	loc.ExpiresAt = time.Now().Add(AttachmentURLTTL).Unix()
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
// when empty. UserID + ExpiresAt are filled in by GetAttachmentURL /
// GetTransformedAttachmentURL at signing time.
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
// variant or thumbnail attachment owned by a projected
// AssetProcessingSucceededEvent keyed by `videoOriginID` (the original
// video's attachment ID). UserID + ExpiresAt are filled in at signing time
// — see LocatorForBodyAttachment.
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

// videoProcessingKey returns the legacy SERVER_RUNTIME key for a video's
// processing state. New writes are process-local only; durable manifests
// live in EVT.
func videoProcessingKey(attachmentID string) string {
	return "video." + attachmentID
}

// GetVideoProcessingState retrieves the processing state for a video attachment.
// Returns nil, nil if no transient processing state exists for this attachment.
// Completed/failed manifests are source-of-truth in EVT and resolved via
// RoomTimeline.VideoAttachmentManifest.
func (c *ChattoCore) GetVideoProcessingState(ctx context.Context, attachmentID string) (*corev1.VideoProcessingState, error) {
	c.videoStateMu.RLock()
	defer c.videoStateMu.RUnlock()
	state := c.videoStates[attachmentID]
	if state == nil {
		return nil, nil
	}
	return proto.Clone(state).(*corev1.VideoProcessingState), nil
}

// SetVideoProcessingState stores transient per-process processing progress for
// a video attachment. It deliberately does not write SERVER_RUNTIME/RUNTIME_STATE;
// durable completed/failed outcomes are AssetProcessing* events in EVT.
func (c *ChattoCore) SetVideoProcessingState(ctx context.Context, attachmentID string, state *corev1.VideoProcessingState) error {
	c.videoStateMu.Lock()
	defer c.videoStateMu.Unlock()
	if state == nil {
		delete(c.videoStates, attachmentID)
		return nil
	}
	c.videoStates[attachmentID] = proto.Clone(state).(*corev1.VideoProcessingState)
	return nil
}

// ClearVideoProcessingState removes transient in-memory processing progress.
func (c *ChattoCore) ClearVideoProcessingState(attachmentID string) {
	c.videoStateMu.Lock()
	defer c.videoStateMu.Unlock()
	delete(c.videoStates, attachmentID)
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

// RecoverUnmanifestedVideoAttachments replays durable message attachments into
// the video worker queue when they have no completed/failed manifest yet. If
// the original binary is already gone, it records a durable unavailable state.
func (c *ChattoCore) RecoverUnmanifestedVideoAttachments(ctx context.Context) {
	for _, req := range c.RoomTimeline.UnmanifestedVideoAttachments() {
		if req.Attachment == nil {
			continue
		}
		if state, _ := c.GetVideoProcessingState(ctx, req.Attachment.GetId()); state != nil {
			continue
		}
		kind, err := c.FindRoomKind(ctx, req.RoomID)
		if err != nil {
			c.logger.Warn("Failed to resolve room kind for video recovery", "room_id", req.RoomID, "error", err)
			continue
		}
		if !c.attachmentBinaryAvailable(ctx, req.Attachment) {
			if err := c.RecordAssetProcessingFailed(ctx, kind, req.RoomID, req.Attachment.GetId(), corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
				c.logger.Warn("Failed to record missing original during video recovery", "attachment_id", req.Attachment.GetId(), "error", err)
			}
			continue
		}
		if err := c.InitVideoProcessingState(ctx, req.Attachment.GetId()); err != nil {
			c.logger.Warn("Failed to init recovered video processing state", "attachment_id", req.Attachment.GetId(), "error", err)
			continue
		}
		if err := c.PublishVideoProcessingRequest(ctx, req.RoomID, req.Attachment.GetId(), req.Attachment.GetContentType(), req.MessageEventID); err != nil {
			c.logger.Warn("Failed to queue recovered video processing request", "attachment_id", req.Attachment.GetId(), "error", err)
		}
	}
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

// PublishAssetProcessing appends a durable asset-processing event to
// EVT and mirrors the same payload onto the legacy live room subject.
func (c *ChattoCore) PublishAssetProcessing(ctx context.Context, kind RoomKind, roomID string, event *corev1.Event) error {
	if roomID == "" {
		return fmt.Errorf("asset processing event missing room id")
	}
	agg := events.RoomAggregate(roomID)
	if _, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, event); err != nil {
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	subject := subjects.LiveRoomEvent(string(kind), roomID, events.EventTypeOf(event))
	if err := c.publishLiveServerEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish asset processing live mirror", "error", err)
	}
	return nil
}

// RecordAssetCreated records the durable content identity and message parent for
// an uploaded asset. Processing outcomes reference this creation by asset id.
// Asset creation is projection state, so it is not mirrored to the public
// live.server subscription stream.
func (c *ChattoCore) RecordAssetCreated(ctx context.Context, _ RoomKind, roomID, messageEventID string, attachment *corev1.Attachment) error {
	if roomID == "" || messageEventID == "" || attachment == nil || attachment.GetId() == "" {
		return fmt.Errorf("asset creation missing room, message, or asset id")
	}
	declaredAttachment := proto.Clone(attachment).(*corev1.Attachment)
	if declaredAttachment.MessageBodyId == "" {
		declaredAttachment.MessageBodyId = messageEventID
	}
	asset := assetFromAttachment(declaredAttachment)
	asset.Parent = &corev1.Asset_Message{
		Message: &corev1.MessageAssetParent{
			RoomId:         roomID,
			MessageEventId: messageEventID,
		},
	}
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				BinaryAvailable: true,
				Asset:           asset,
			},
		},
	})
	agg := events.RoomAggregate(roomID)
	if _, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, agg, event); err != nil {
		return fmt.Errorf("publish asset creation event: %w", err)
	}
	return nil
}

// RecordAssetProcessed builds and publishes a durable processed-video
// manifest for an original video attachment.
func (c *ChattoCore) RecordAssetProcessed(ctx context.Context, kind RoomKind, roomID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	assetVariants := make([]*corev1.AssetVideoVariant, 0, len(variants))
	thumbnailAssetID := ""
	if thumbnailAsset := assetFromAttachment(thumbnail); thumbnailAsset != nil {
		thumbnailAssetID = thumbnailAsset.GetId()
		thumbnailAsset.Parent = &corev1.Asset_Derivative{
			Derivative: &corev1.AssetDerivativeParent{
				AssetId: attachmentID,
				Role:    "thumbnail",
			},
		}
		if err := c.recordDerivativeAssetCreated(ctx, roomID, thumbnailAsset); err != nil {
			return err
		}
	}
	for _, variant := range variants {
		if variant == nil || variant.GetAttachment() == nil {
			continue
		}
		variantAsset := assetFromAttachment(variant.GetAttachment())
		variantAsset.Parent = &corev1.Asset_Derivative{
			Derivative: &corev1.AssetDerivativeParent{
				AssetId: attachmentID,
				Role:    "video_variant",
			},
		}
		if err := c.recordDerivativeAssetCreated(ctx, roomID, variantAsset); err != nil {
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
						DurationMs:       durationMs,
						Width:            width,
						Height:           height,
						ThumbnailAssetId: thumbnailAssetID,
						Variants:         assetVariants,
					},
				},
			},
		},
	})
	return c.PublishAssetProcessing(ctx, kind, roomID, event)
}

func (c *ChattoCore) recordDerivativeAssetCreated(ctx context.Context, roomID string, asset *corev1.Asset) error {
	if roomID == "" || asset == nil || asset.GetId() == "" {
		return fmt.Errorf("derivative asset creation missing room or asset id")
	}
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				BinaryAvailable: true,
				Asset:           asset,
			},
		},
	})
	if _, err := c.RoomTimelineProjector.AppendEventuallyAndWait(ctx, c.EventPublisher, events.RoomAggregate(roomID), event); err != nil {
		return fmt.Errorf("publish derivative asset creation event: %w", err)
	}
	return nil
}

// RecordAssetProcessingFailed builds and publishes a durable failed
// video-processing outcome.
func (c *ChattoCore) RecordAssetProcessingFailed(ctx context.Context, kind RoomKind, roomID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	event := newEvent("", &corev1.Event{
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:     attachmentID,
				FailureCode: failureCode,
			},
		},
	})
	return c.PublishAssetProcessing(ctx, kind, roomID, event)
}
