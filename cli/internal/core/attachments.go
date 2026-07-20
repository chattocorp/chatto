package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/assets"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

const (
	// ServerAssetVisibilityHeader positively marks legacy flat NATS objects that
	// may be served by the unauthenticated public server-asset route. New public
	// objects use PublicServerAssetObjectPrefix instead.
	ServerAssetVisibilityHeader       = "Chatto-Asset-Visibility"
	ServerAssetVisibilityPublic       = "public"
	ServerAssetVisibilityNUIDHeader   = "Chatto-Asset-Public-NUID"
	ServerAssetVisibilityDigestHeader = "Chatto-Asset-Public-Digest"

	// PublicServerAssetObjectPrefix is the explicit SERVER_ASSETS namespace for
	// newly written unauthenticated public assets. Historical public objects use
	// flat asset-ID keys and remain available through the legacy classifier.
	PublicServerAssetObjectPrefix      = "public/"
	attachmentWriteCompensationTimeout = 5 * time.Second
)

// PublicServerAssetObjectKey returns the NATS object key for a new public
// server asset while keeping its logical asset ID stable.
func PublicServerAssetObjectKey(assetID string) string {
	return PublicServerAssetObjectPrefix + assetID
}

// ServerAssetDeliveryKey returns the storage-aware key used in public server
// asset URLs. New NATS assets carry their explicit public/ object key, while
// S3 and historical records continue to use their logical asset ID.
func ServerAssetDeliveryKey(asset *corev1.AssetRecord) string {
	if asset == nil {
		return ""
	}
	if stored := asset.GetNats(); stored != nil && stored.GetKey() != "" {
		return stored.GetKey()
	}
	if stored := asset.GetS3(); stored != nil && stored.GetKey() != "" {
		return stored.GetKey()
	}
	return asset.GetId()
}

// ============================================================================
// Attachment Operations
// ============================================================================

// GetAttachmentsStore returns the ObjectStore for attachments.
// Uses lazy-loading and caching for efficiency.
func (c *MediaModel) GetAttachmentsStore(ctx context.Context) (jetstream.ObjectStore, error) {
	return c.storage.serverAssets, nil
}

// UploadAttachment uploads a file as an attachment and returns the
// attachment metadata. For images, it extracts dimensions. Thumbnails
// are generated on-the-fly via transforms. The storage backend (NATS or
// S3) is determined by configuration.
//
// UploadAttachment stores a binary in the configured backend, emits a
// durable AssetCreatedEvent into the asset aggregate (so the asset becomes
// a first-class entity from the moment its bytes hit storage), and returns
// the rendered Attachment view for the caller.
//
// `actorID` is the uploader; it's stamped on the AssetCreatedEvent as the
// asset's user_id, distinct from any future message_event_id that might
// claim the asset. The returned Attachment has `MessageBodyId` empty — the
// asset is not (yet) bound to a message; PostMessage references it by id.
func (c *MediaModel) UploadAttachment(
	ctx context.Context,
	actorID string,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	if actorID == "" {
		return nil, fmt.Errorf("upload missing actor id")
	}
	attachment, err := c.uploadAttachmentBinary(ctx, roomID, filename, contentType, reader)
	if err != nil {
		return nil, err
	}
	if err := c.assetLifecycle().RecordUploadedAsset(ctx, actorID, roomID, attachment); err != nil {
		return nil, err
	}

	c.logger.Debug("Uploaded attachment",
		"attachment_id", attachment.GetId(),
		"room_id", roomID,
		"actor_id", actorID,
		"filename", filename,
		"content_type", contentType,
		"size", attachment.GetSize(),
		"storage_backend", c.config.Assets.StorageBackend,
	)

	return attachment, nil
}

// uploadAttachmentBinary writes the upload's bytes to the configured backend
// (NATS object store or S3) and returns the rendered Attachment proto. No
// event is emitted — callers are responsible for emitting AssetCreatedEvent
// with the right owner shape (user upload vs derivative).
func (c *MediaModel) uploadAttachmentBinary(
	ctx context.Context,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	attachmentID := NewAssetID()

	isImage := strings.HasPrefix(contentType, "image/")

	var content []byte
	var size int64
	var width, height int32

	if isImage {
		result, err := assets.ProcessAttachmentImageWithConfig(reader, c.AssetsConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to process image: %w", err)
		}
		content = result.Original
		size = int64(len(result.Original))
		width = int32(result.Width)
		height = int32(result.Height)
	} else {
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

	var storage *corev1.DeprecatedAsset
	if c.ShouldUseS3() {
		s3Key := S3KeyAttachment(attachmentID)
		if _, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, content, contentType); err != nil {
			return nil, fmt.Errorf("failed to upload attachment to S3: %w", err)
		}
		storage = &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_S3{
				S3: &corev1.S3Asset{
					Key:    s3Key,
					Bucket: proto.String(c.s3Client.Bucket()),
				},
			},
		}
	} else {
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get attachments store: %w", err)
		}
		if _, err := store.Put(ctx, jetstream.ObjectMeta{
			Name: attachmentID,
			Headers: map[string][]string{
				"Content-Type": {contentType},
				"Filename":     {filename},
				"Room-Id":      {roomID},
			},
		}, bytes.NewReader(content)); err != nil {
			return nil, fmt.Errorf("failed to store attachment: %w", err)
		}
		storage = &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_Nats{
				Nats: &corev1.NATSAsset{Key: attachmentID},
			},
		}
	}

	return &corev1.Attachment{
		Id:          attachmentID,
		RoomId:      roomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        size,
		Width:       width,
		Height:      height,
		Storage:     storage,
	}, nil
}

// UploadDerivativeAttachment is the worker-side variant of UploadAttachment.
// It writes bytes through the same storage path and emits AssetCreatedEvent
// with parent_asset_id + derivative_role already set, so the projection
// knows this asset is a child of `parentAssetID` (thumbnails, transcoded
// video variants, etc.). Always attributed to SystemActorID — derivatives
// are produced by workers, not user actions.
func (c *MediaModel) UploadDerivativeAttachment(
	ctx context.Context,
	parentAssetID string,
	derivativeRole corev1.AssetDerivativeRole,
	roomID string,
	filename string,
	contentType string,
	reader io.Reader,
) (*corev1.Attachment, error) {
	return c.UploadDerivativeAttachmentWithDimensions(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, reader, 0, 0)
}

// UploadDerivativeAttachmentWithDimensions is UploadDerivativeAttachment with
// explicit media dimensions supplied by the worker for generated non-image
// derivatives such as transcoded video variants.
func (c *MediaModel) UploadDerivativeAttachmentWithDimensions(
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
	attachment, err := c.uploadAttachmentBinary(ctx, roomID, filename, contentType, reader)
	if err != nil {
		return nil, err
	}
	if width > 0 && height > 0 {
		attachment.Width = width
		attachment.Height = height
	}
	if err := c.assetLifecycle().RecordDerivativeAsset(ctx, parentAssetID, derivativeRole, roomID, attachment); err != nil {
		if errors.Is(err, ErrAssetCommitUnknown) {
			// Preserve the storage handle so the worker can attempt prompt cleanup.
			return attachment, err
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attachmentWriteCompensationTimeout)
		defer cancel()
		if cleanupErr := c.DeleteAttachmentFromStorage(cleanupCtx, attachment); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("delete unrecorded derivative binary: %w", cleanupErr))
		}
		return nil, err
	}
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

type StableAssetURL struct {
	URL       string
	ExpiresAt time.Time
}

// GetAttachment retrieves an attachment by ID from NATS ObjectStore.
// This is the legacy path for attachments stored in NATS.
// Returns a reader for the attachment content and the object info.
func (c *MediaModel) GetAttachment(ctx context.Context, attachmentID string) (io.Reader, *jetstream.ObjectInfo, error) {
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
func (c *MediaModel) GetS3Attachment(ctx context.Context, s3Key string) (io.ReadCloser, *AttachmentInfo, error) {
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
func (c *MediaModel) GetAttachmentReader(ctx context.Context, attachment *corev1.Attachment) (io.Reader, *AttachmentInfo, error) {
	if attachment == nil {
		return nil, nil, fmt.Errorf("attachment is nil")
	}
	if attachment.Storage == nil {
		return c.probeAttachmentReaderByID(ctx, attachment.Id)
	}
	switch asset := attachment.Storage.Asset.(type) {
	case *corev1.DeprecatedAsset_Nats:
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
	case *corev1.DeprecatedAsset_S3:
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
// This is load-bearing for older video variants and thumbnails whose
// attachment protos have no Storage field, so we probe.
func (c *MediaModel) probeAttachmentReaderByID(ctx context.Context, attachmentID string) (io.Reader, *AttachmentInfo, error) {
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

func assetFromAttachment(attachment *corev1.Attachment) *corev1.AssetRecord {
	if attachment == nil {
		return nil
	}
	asset := &corev1.AssetRecord{
		Id:          attachment.GetId(),
		Filename:    attachment.GetFilename(),
		ContentType: attachment.GetContentType(),
		Size:        attachment.GetSize(),
	}
	applyDeprecatedAssetFromAttachmentStorage(asset, attachment.GetStorage())
	applyAssetMetadataFromAttachment(asset, attachment)
	return asset
}

func attachmentFromAsset(asset *corev1.AssetRecord) *corev1.Attachment {
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

// AttachmentFromAsset converts the Asset model into the message-facing
// Attachment view used by API response mapping and asset URL helpers.
func AttachmentFromAsset(asset *corev1.AssetRecord) *corev1.Attachment {
	return attachmentFromAsset(asset)
}

func assetRecordMatchesKey(asset *corev1.AssetRecord, key string) bool {
	if asset == nil || key == "" {
		return false
	}
	if asset.GetId() == key {
		return true
	}
	if stored := asset.GetNats(); stored != nil && stored.GetKey() == key {
		return true
	}
	return asset.GetS3() != nil && asset.GetS3().GetKey() == key
}

func applyDeprecatedAssetFromAttachmentStorage(asset *corev1.AssetRecord, storage *corev1.DeprecatedAsset) {
	if asset == nil || storage == nil {
		return
	}
	switch stored := storage.GetAsset().(type) {
	case *corev1.DeprecatedAsset_Nats:
		if stored.Nats != nil {
			asset.Storage = &corev1.AssetRecord_Nats{Nats: proto.Clone(stored.Nats).(*corev1.NATSAsset)}
		}
	case *corev1.DeprecatedAsset_S3:
		if stored.S3 != nil {
			asset.Storage = &corev1.AssetRecord_S3{S3: proto.Clone(stored.S3).(*corev1.S3Asset)}
		}
	}
}

func assetStorageFromAsset(asset *corev1.AssetRecord) *corev1.DeprecatedAsset {
	if asset == nil {
		return nil
	}
	switch {
	case asset.GetNats() != nil:
		return &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_Nats{Nats: proto.Clone(asset.GetNats()).(*corev1.NATSAsset)},
		}
	case asset.GetS3() != nil:
		return &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_S3{S3: proto.Clone(asset.GetS3()).(*corev1.S3Asset)},
		}
	default:
		return nil
	}
}

func DeprecatedAssetFromAsset(asset *corev1.AssetRecord) *corev1.DeprecatedAsset {
	return assetStorageFromAsset(asset)
}

func assetFromDeprecatedAsset(storage *corev1.DeprecatedAsset, filename, contentType string) *corev1.AssetRecord {
	if storage == nil {
		return nil
	}
	asset := &corev1.AssetRecord{
		Id:          assetIDFromAsset(storage),
		Filename:    filename,
		ContentType: contentType,
	}
	applyDeprecatedAssetFromAttachmentStorage(asset, storage)
	return asset
}

func applyAssetMetadataFromAttachment(asset *corev1.AssetRecord, attachment *corev1.Attachment) {
	if asset == nil || attachment == nil || (attachment.GetWidth() == 0 && attachment.GetHeight() == 0) {
		return
	}
	asset.Width = attachment.GetWidth()
	asset.Height = attachment.GetHeight()
}

func assetDimensions(asset *corev1.AssetRecord) (int32, int32) {
	if asset == nil {
		return 0, 0
	}
	return asset.GetWidth(), asset.GetHeight()
}

func cloneDeprecatedAsset(storage *corev1.DeprecatedAsset) *corev1.DeprecatedAsset {
	if storage == nil {
		return nil
	}
	return proto.Clone(storage).(*corev1.DeprecatedAsset)
}

func cloneAssetRecord(asset *corev1.AssetRecord) *corev1.AssetRecord {
	if asset == nil {
		return nil
	}
	return proto.Clone(asset).(*corev1.AssetRecord)
}

// MessageBodyAttachments returns the materialised attachments for a
// MessageBody, hydrating from the asset projection when the body uses
// asset_ids (current format) and falling back to the legacy embedded
// attachments slice otherwise. The returned slice preserves the body's
// declared order; missing projection entries are skipped.
func (c *MediaModel) MessageBodyAttachments(body *corev1.MessageBody) []*corev1.Attachment {
	if body == nil {
		return nil
	}
	ids := body.GetAssetIds()
	if len(ids) == 0 {
		return body.GetAttachments()
	}
	out := make([]*corev1.Attachment, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		declared, ok := c.assetLifecycle().AssetCreation(id)
		if !ok {
			continue
		}
		if att := attachmentFromAsset(declared.GetAsset()); att != nil {
			out = append(out, att)
		}
	}
	return out
}

// DeleteAttachmentFromStorage deletes an attachment's binary and its
// cached resizes. When Storage is missing for legacy imported derivatives,
// it falls back to known backend key layouts by attachment ID.
func (c *MediaModel) DeleteAttachmentFromStorage(ctx context.Context, attachment *corev1.Attachment) error {
	if attachment == nil {
		return fmt.Errorf("attachment is nil")
	}

	deleteErr := c.deleteAttachmentBinary(ctx, attachment)
	_, cacheErr := c.DeleteCachedResizesForAttachment(ctx, attachment.Id)
	return errors.Join(deleteErr, cacheErr)
}

func (c *MediaModel) deleteAttachmentBinary(ctx context.Context, attachment *corev1.Attachment) error {
	if attachment.Storage == nil {
		return c.deleteAttachmentBinaryByID(ctx, attachment.Id)
	}
	switch storage := attachment.Storage.Asset.(type) {
	case *corev1.DeprecatedAsset_Nats:
		store, err := c.GetAttachmentsStore(ctx)
		if err != nil {
			return fmt.Errorf("failed to get attachments store: %w", err)
		}
		if err := store.Delete(ctx, storage.Nats.Key); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
			return fmt.Errorf("failed to delete attachment from NATS: %w", err)
		}
		c.logger.Debug("Deleted NATS attachment", "attachment_id", attachment.Id, "key", storage.Nats.Key)
	case *corev1.DeprecatedAsset_S3:
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
	return nil
}

func (c *MediaModel) deleteAttachmentBinaryByID(ctx context.Context, attachmentID string) error {
	if attachmentID == "" {
		return fmt.Errorf("attachment has no id")
	}

	var natsErr error
	store, err := c.GetAttachmentsStore(ctx)
	if err != nil {
		natsErr = fmt.Errorf("failed to get attachments store: %w", err)
	} else if err := store.Delete(ctx, attachmentID); err == nil {
		c.logger.Debug("Deleted NATS attachment", "attachment_id", attachmentID, "key", attachmentID)
		return nil
	} else if errors.Is(err, jetstream.ErrObjectNotFound) {
		natsErr = nil
	} else {
		natsErr = fmt.Errorf("failed to delete attachment from NATS: %w", err)
	}

	if c.s3Client != nil {
		var s3Err error
		found := false
		for _, s3Key := range legacyAttachmentS3KeyCandidates(attachmentID) {
			if _, err := c.s3Client.StatObject(ctx, s3Key); err != nil {
				if !IsNoSuchKeyError(err) {
					s3Err = errors.Join(s3Err, err)
				}
				continue
			}
			found = true
			if err := c.s3Client.DeleteObjectFromBucket(ctx, c.s3Client.Bucket(), s3Key); err != nil {
				s3Err = errors.Join(s3Err, err)
				continue
			}
			c.logger.Debug("Deleted S3 attachment", "attachment_id", attachmentID, "s3_key", s3Key)
			return nil
		}
		if s3Err != nil {
			return fmt.Errorf("failed to delete attachment %s from known backends: nats: %v; s3: %w", attachmentID, natsErr, s3Err)
		}
		if found {
			return fmt.Errorf("failed to delete attachment %s from S3", attachmentID)
		}
	}

	return natsErr
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
func (c *MediaModel) TryPresignedAttachmentURL(ctx context.Context, attachment *corev1.Attachment, ttl time.Duration) (string, error) {
	if c.s3Client == nil {
		return "", fmt.Errorf("S3 not configured")
	}
	if attachment == nil {
		return "", fmt.Errorf("attachment is nil")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("presigned URL TTL must be positive")
	}
	if attachment.Storage == nil {
		return c.probePresignedAttachmentURL(ctx, attachment.Id, ttl)
	}
	s3, ok := attachment.Storage.Asset.(*corev1.DeprecatedAsset_S3)
	if !ok {
		return "", fmt.Errorf("attachment %s is not stored in S3", attachment.Id)
	}
	presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3.S3.Key, ttl)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

// probePresignedAttachmentURL is the fallback when an Attachment's
// `Storage` field isn't populated and the binary might live in S3.
// Stats the known key layouts; presigns the first hit.
func (c *MediaModel) probePresignedAttachmentURL(ctx context.Context, attachmentID string, ttl time.Duration) (string, error) {
	for _, s3Key := range legacyAttachmentS3KeyCandidates(attachmentID) {
		if _, err := c.s3Client.StatObject(ctx, s3Key); err != nil {
			continue
		}
		presignedURL, err := c.s3Client.PresignedGetURL(ctx, s3Key, ttl)
		if err != nil {
			return "", fmt.Errorf("failed to generate presigned URL: %w", err)
		}
		return presignedURL.String(), nil
	}
	return "", fmt.Errorf("attachment %s not found in S3", attachmentID)
}

// AttachmentSignResource binds legacy signed attachment transforms and remains
// a cache-cleanup namespace for derivatives written before stable asset paths.
const AttachmentSignResource = "attachment"

// AttachmentDerivativeCacheResource versions the current attachment image
// encoding profile independently from the stable public URL shape.
const AttachmentDerivativeCacheResource = "attachment-stable-v2"

const attachmentLegacyStableCacheResource = "attachment-stable"

// ServerAssetSignResource is the first resource component fed to the
// signed-URL signer for server asset transform URLs and the cache prefix for
// transformed server assets (avatars, logos, banners, and similar public
// server-scoped images).
const ServerAssetSignResource = "server"

// AssetAccessTicketTTL keeps direct browser/standalone-client asset URLs useful
// across long-lived room views and deferred media playback, without turning
// copied URLs into indefinite bearer links.
const AssetAccessTicketTTL = 24 * time.Hour

// Keep repeated reads stable without extending ticket validity beyond the
// configured TTL. URLs issued within one bucket share an expiry and signature.
const assetAccessTicketIssueBucket = time.Hour

// S3AssetRedirectTTL bounds direct object-store redirects for heavy original
// asset responses. Chatto authorizes the request first, then hands browsers a
// short-lived S3 URL only for cases where proxying the bytes would be costly.
const S3AssetRedirectTTL = 5 * time.Minute

// GetStableAttachmentURL returns the canonical URL for an asset binary. The
// path identifies the asset; the asset-scoped access ticket authorizes the
// viewer so browsers and standalone clients can load the URL directly from the
// owning host without custom headers.
func (c *MediaModel) GetStableAttachmentURL(assetID, userID string) string {
	return c.GetStableAttachmentAssetURL(assetID, userID).URL
}

// GetStableAttachmentAssetURL returns the canonical URL for an asset binary
// together with the exact expiry embedded in its access ticket.
func (c *MediaModel) GetStableAttachmentAssetURL(assetID, userID string) StableAssetURL {
	if assetID == "" || userID == "" {
		return StableAssetURL{}
	}
	expiresAt := c.assetAccessTicketExpiry()
	return StableAssetURL{
		URL:       c.assetURL(c.stableAttachmentPathWithAccess(assetID, userID, "", nil, expiresAt)),
		ExpiresAt: expiresAt,
	}
}

// GetStableHLSMasterPlaylistAssetURL returns the authorised entry point for
// one processed video's HLS generation.
func (c *MediaModel) GetStableHLSMasterPlaylistAssetURL(assetID, userID string) StableAssetURL {
	if assetID == "" || userID == "" {
		return StableAssetURL{}
	}
	expiresAt := c.assetAccessTicketExpiry()
	ticket, err := signedurl.SignedHLSAccessTicket(c.config.Assets.SigningSecret, signedurl.HLSAccessTicket{
		AssetID:   assetID,
		UserID:    userID,
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		c.logger.Warn("Failed to sign HLS access ticket", "error", err, "asset_id", assetID, "user_id", userID)
		return StableAssetURL{}
	}
	values := url.Values{}
	values.Set("access", ticket)
	path := fmt.Sprintf("/assets/hls/%s/master.m3u8?%s", url.PathEscape(assetID), values.Encode())
	return StableAssetURL{URL: c.assetURL(path), ExpiresAt: expiresAt}
}

// GetStableTransformedAttachmentURL returns the canonical URL for a derived
// image form factor. The dimensions are visible in the URL; authorization is a
// scoped access ticket.
func (c *MediaModel) GetStableTransformedAttachmentURL(assetID, userID string, width, height int, fit string) string {
	return c.GetStableTransformedAttachmentAssetURL(assetID, userID, width, height, fit).URL
}

// GetStableTransformedAttachmentAssetURL returns the canonical URL for a
// derived image form factor together with the exact expiry embedded in its
// access ticket.
func (c *MediaModel) GetStableTransformedAttachmentAssetURL(assetID, userID string, width, height int, fit string) StableAssetURL {
	if assetID == "" || userID == "" {
		return StableAssetURL{}
	}
	transformPath := fmt.Sprintf(
		"/assets/files/%s/image/%dx%d/%s",
		url.PathEscape(assetID),
		width,
		height,
		url.PathEscape(fit),
	)
	expiresAt := c.assetAccessTicketExpiry()
	return StableAssetURL{
		URL: c.assetURL(c.stableAttachmentPathWithAccess(assetID, userID, transformPath, &signedurl.TransformParams{
			Width:  width,
			Height: height,
			Fit:    fit,
		}, expiresAt)),
		ExpiresAt: expiresAt,
	}
}

func (c *MediaModel) assetAccessTicketExpiry() time.Time {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	return now().UTC().Truncate(assetAccessTicketIssueBucket).Add(AssetAccessTicketTTL)
}

func (c *MediaModel) stableAttachmentPathWithAccess(assetID, userID, path string, params *signedurl.TransformParams, expiresAt time.Time) string {
	if path == "" {
		path = fmt.Sprintf("/assets/files/%s", url.PathEscape(assetID))
	}
	accessTicket := signedurl.AssetAccessTicket{
		AssetID:   assetID,
		UserID:    userID,
		ExpiresAt: expiresAt.Unix(),
	}
	if params != nil {
		accessTicket.Width = params.Width
		accessTicket.Height = params.Height
		accessTicket.Fit = params.Fit
	}
	ticket, err := signedurl.SignedAssetAccessTicket(c.config.Assets.SigningSecret, accessTicket)
	if err != nil {
		c.logger.Warn("Failed to sign asset access ticket", "error", err, "asset_id", assetID, "user_id", userID)
		return ""
	}
	values := url.Values{}
	values.Set("access", ticket)
	return path + "?" + values.Encode()
}

// GetTransformedServerAssetURL returns the URL for accessing a transformed version of an server asset.
// Server assets include server logos, server banners, and user avatars stored in SERVER_ASSETS.
// The URL includes HMAC signature to prevent parameter tampering.
// Format: /assets/server/{key}/t/{params}.{signature}
// where {params} is base64url-encoded JSON: {"w":width,"h":height,"f":"fit"}
func (c *MediaModel) GetTransformedServerAssetURL(key string, width, height int, fit string) string {
	// Generate signed transform path component using the server asset resource ID.
	signedPath := signedurl.SignedTransformPath(c.config.Assets.SigningSecret, ServerAssetSignResource, key, width, height, fit)

	// Return signed transform URL
	return c.assetURL(fmt.Sprintf("/assets/server/%s/t/%s", key, signedPath))
}

// ============================================================================
// Image Cache Operations
// ============================================================================

// ImageCacheEnabled returns whether the image resize cache is enabled.
func (c *MediaModel) ImageCacheEnabled() bool {
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
func (c *MediaModel) GetCachedResize(ctx context.Context, key string) ([]byte, error) {
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
func (c *MediaModel) StoreCachedResize(ctx context.Context, key string, data []byte) error {
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
// encountered. Does nothing if the cache is disabled. Current and earlier
// stable attachment cache namespaces are removed; older room-kind prefixes
// remain unaddressable and age out through the configured TTL.
func (c *MediaModel) DeleteCachedResizesForAttachment(ctx context.Context, attachmentID string) (int, error) {
	prefixes := []string{
		AttachmentDerivativeCacheResource,
		AttachmentSignResource,
		attachmentLegacyStableCacheResource,
	}
	deleted := 0
	var deleteErr error
	for _, prefix := range prefixes {
		count, err := c.DeleteCachedResizesForKey(ctx, prefix, attachmentID)
		deleted += count
		deleteErr = errors.Join(deleteErr, err)
	}
	return deleted, deleteErr
}

// DeleteCachedResizesForServerAsset deletes all cached resizes for a server
// asset such as a user avatar or server branding image.
func (c *MediaModel) DeleteCachedResizesForServerAsset(ctx context.Context, assetID string) (int, error) {
	return c.DeleteCachedResizesForKey(ctx, ServerAssetSignResource, assetID)
}

// DeleteCachedResizesForKey deletes all cached resizes for a given prefix and asset key.
// Returns the number of deleted cache entries and any error encountered.
// Does nothing if the cache is disabled.
func (c *MediaModel) DeleteCachedResizesForKey(ctx context.Context, prefix, assetKey string) (int, error) {
	if prefix == "" || assetKey == "" {
		return 0, nil
	}
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
	var deleteErr error
	for _, info := range objects {
		if strings.HasPrefix(info.Name, keyPrefix) {
			if err := c.storage.imageCacheStore.Delete(ctx, info.Name); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
				deleteErr = errors.Join(deleteErr, fmt.Errorf("delete cached resize %s: %w", info.Name, err))
			} else {
				deleted++
			}
		}
	}

	return deleted, deleteErr
}

// AttachmentNeedsVideoProcessing returns whether an attachment should enter the
// video processing pipeline. Static GIFs stay image-only; callers that inspect
// the upload bytes can set animatedGIF for GIF-to-MP4 conversion.
func AttachmentNeedsVideoProcessing(attachment *corev1.Attachment, animatedGIF bool) bool {
	if attachment == nil {
		return false
	}
	return strings.HasPrefix(attachment.GetContentType(), "video/") || animatedGIF
}

// videoProcessingKey returns the historical SERVER_RUNTIME key for a video's
// processing state. New processing no longer writes runtime state.
func videoProcessingKey(attachmentID string) string {
	return "video." + attachmentID
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

// attachmentBinaryStatus probes storage for the attachment and classifies the
// result. The intent is to let callers distinguish "we know it's gone" from
// "we couldn't reach storage" so transient S3 or configuration issues do not
// get recorded as durable SOURCE_MISSING events.
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
