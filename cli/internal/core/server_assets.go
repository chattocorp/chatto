package core

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/config"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// assetURL prepends AssetBaseURL to an asset path.
// When AssetBaseURL is empty, returns the path unchanged.
func (c *ChattoCore) assetURL(path string) string {
	if c.AssetBaseURL == "" {
		return path
	}
	return c.AssetBaseURL + path
}

// AssetsConfig returns the assets configuration as an assets.Config.
func (c *ChattoCore) AssetsConfig() assets.Config {
	maxUploadSize := int64(c.config.Assets.MaxUploadSize)
	if maxUploadSize == 0 {
		maxUploadSize = assets.DefaultMaxUploadSize
	}
	return assets.Config{
		MaxUploadSize: maxUploadSize,
	}
}

// ShouldUseS3 returns true if new uploads should be stored in S3.
func (c *ChattoCore) ShouldUseS3() bool {
	return c.config.Assets.StorageBackend == config.StorageBackendS3 && c.s3Client != nil
}

// S3Client returns the S3 client, or nil if S3 is not configured.
func (c *ChattoCore) S3Client() *S3Client {
	return c.s3Client
}

// ServerAssetInfo contains metadata about a server asset.
type ServerAssetInfo struct {
	Size        int64
	ContentType string
}

// PublicServerAssetLocation binds a successful public classification to one
// exact backend object. Its fields stay private so callers cannot manufacture
// a location that bypasses ResolvePublicServerAsset.
type PublicServerAssetLocation struct {
	natsKey    string
	natsNUID   string
	natsDigest string
	s3Key      string
}

func publicNATSServerAssetLocation(key string, info *jetstream.ObjectInfo) (*PublicServerAssetLocation, bool) {
	if key == "" || info == nil || info.NUID == "" || info.Digest == "" {
		return nil, false
	}
	return &PublicServerAssetLocation{
		natsKey:    key,
		natsNUID:   info.NUID,
		natsDigest: info.Digest,
	}, true
}

func serverAssetVisibilityMarkerMatches(info *jetstream.ObjectInfo) bool {
	return info != nil && info.NUID != "" && info.Digest != "" &&
		info.Headers.Get(ServerAssetVisibilityHeader) == ServerAssetVisibilityPublic &&
		info.Headers.Get(ServerAssetVisibilityNUIDHeader) == info.NUID &&
		info.Headers.Get(ServerAssetVisibilityDigestHeader) == info.Digest
}

// serverAssetRequestKey recognizes the explicit public/ namespace used by new
// NATS objects and the flat canonical IDs used by historical NATS objects and
// current S3 instance/ objects. Every other namespace fails closed.
func serverAssetRequestKey(key string) (assetID string, namespaced bool, ok bool) {
	if key == "" || key != strings.TrimSpace(key) || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", false, false
	}
	if strings.HasPrefix(key, PublicServerAssetObjectPrefix) {
		namespaced = true
		assetID = strings.TrimPrefix(key, PublicServerAssetObjectPrefix)
	} else {
		assetID = key
	}
	if len(assetID) != idLength+1 || assetID[0] != 'A' || strings.Contains(assetID, "/") {
		return "", false, false
	}
	for _, ch := range assetID[1:] {
		if !strings.ContainsRune(idAlphabet, ch) {
			return "", false, false
		}
	}
	return assetID, namespaced, true
}

func serverAssetNATSObjectKeys(key string) (logicalID string, namespaced bool, objectKeys []string, ok bool) {
	logicalID, namespaced, ok = serverAssetRequestKey(key)
	if !ok {
		return "", false, nil, false
	}
	if namespaced {
		return logicalID, true, []string{key}, true
	}
	return logicalID, false, []string{PublicServerAssetObjectKey(logicalID), logicalID}, true
}

// IsReservedServerAssetKey rejects private, internal, and unknown namespaces
// before public-route transform parsing or backend probing.
func IsReservedServerAssetKey(key string) bool {
	_, _, ok := serverAssetRequestKey(key)
	return !ok
}

// ResolvePublicServerAsset positively classifies an object and binds the
// decision to one exact backend key before the public route performs cache
// access, content reads, or transforms. Unknown objects fail closed.
func (c *ChattoCore) ResolvePublicServerAsset(ctx context.Context, key string) (*PublicServerAssetLocation, bool) {
	assetID, namespaced, ok := serverAssetRequestKey(key)
	if !ok || c.assetModel == nil || c.assetModel.assets.Projection() == nil {
		return nil, false
	}

	// Durable room-scoped declarations take precedence over every public hint,
	// including stale metadata or a colliding current public reference.
	assetState := c.assetModel.AssetState(assetID)
	if assetState.Creation != nil || assetState.Deleted {
		return nil, false
	}

	// The public/ namespace is itself the positive declaration for new NATS
	// objects. Metadata-only inspection still rejects an object misplaced there
	// by a private writer before any content or derivative cache is opened.
	if namespaced {
		info, err := c.storage.serverAssets.GetInfo(ctx, key)
		if err != nil || info == nil || info.Headers.Get("Room-Id") != "" || info.Headers.Get("Upload-Id") != "" {
			return nil, false
		}
		return publicNATSServerAssetLocation(key, info)
	}

	// Canonical-ID URLs remain aliases for new namespaced objects. This keeps
	// stored API references and clients that retained the logical ID working.
	if info, err := c.storage.serverAssets.GetInfo(ctx, PublicServerAssetObjectKey(assetID)); err == nil && info != nil {
		if info.Headers.Get("Room-Id") != "" || info.Headers.Get("Upload-Id") != "" {
			return nil, false
		}
		return publicNATSServerAssetLocation(PublicServerAssetObjectKey(assetID), info)
	}

	// Object metadata is safe to inspect for classification; object content is
	// not opened until this method has returned true.
	var legacyNATSExists bool
	if info, err := c.storage.serverAssets.GetInfo(ctx, assetID); err == nil && info != nil {
		legacyNATSExists = true
		if info.Headers.Get("Room-Id") != "" || info.Headers.Get("Upload-Id") != "" {
			return nil, false
		}
		if serverAssetVisibilityMarkerMatches(info) {
			return publicNATSServerAssetLocation(assetID, info)
		}
	}

	// Historical public objects predate the explicit visibility header. Their
	// durable/current public references provide the positive declaration.
	legacyDeclaredPublic := c.userModel != nil && c.userModel.isPublicAvatarAsset(assetID)
	if c.configModel != nil {
		logo := c.configModel.serverBrandingAsset("logo")
		banner := c.configModel.serverBrandingAsset("banner")
		if assetRecordMatchesKey(logo, assetID) || assetRecordMatchesKey(banner, assetID) {
			legacyDeclaredPublic = true
		}
	}
	if assetState.PublicLinkPreview {
		legacyDeclaredPublic = true
	}
	if legacyDeclaredPublic && legacyNATSExists {
		info, err := c.storage.serverAssets.GetInfo(ctx, assetID)
		if err != nil || info == nil || info.Headers.Get("Room-Id") != "" || info.Headers.Get("Upload-Id") != "" {
			return nil, false
		}
		return publicNATSServerAssetLocation(assetID, info)
	}

	// S3 server assets live exclusively below instance/. Private current and
	// historical attachments use attachments/ or spaces/*/attachments/.
	if c.s3Client != nil {
		s3Key := S3KeyServerAsset(assetID)
		if _, err := c.s3Client.StatObject(ctx, s3Key); err == nil {
			return &PublicServerAssetLocation{s3Key: s3Key}, true
		}
	}
	return nil, false
}

// IsPublicServerAsset reports whether ResolvePublicServerAsset can bind the
// request key to an explicitly public object. Prefer the resolver in delivery
// paths so classification cannot fall through to a different backend object.
func (c *ChattoCore) IsPublicServerAsset(ctx context.Context, key string) bool {
	_, ok := c.ResolvePublicServerAsset(ctx, key)
	return ok
}

// GetPublicServerAsset opens only the exact object previously classified by
// ResolvePublicServerAsset. It never probes a fallback backend or key.
func (c *ChattoCore) GetPublicServerAsset(ctx context.Context, location *PublicServerAssetLocation) (io.Reader, *ServerAssetInfo, error) {
	if location == nil {
		return nil, nil, jetstream.ErrObjectNotFound
	}
	if location.natsKey != "" {
		obj, err := c.storage.serverAssets.Get(ctx, location.natsKey)
		if err != nil {
			return nil, nil, err
		}
		info, err := obj.Info()
		if err != nil || info == nil || info.NUID != location.natsNUID || info.Digest != location.natsDigest {
			_ = obj.Close()
			return nil, nil, jetstream.ErrObjectNotFound
		}
		return obj, &ServerAssetInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
		}, nil
	}
	if location.s3Key != "" && c.s3Client != nil {
		reader, info, err := c.s3Client.GetObject(ctx, location.s3Key)
		if err != nil {
			return nil, nil, err
		}
		return reader, &ServerAssetInfo{Size: info.Size, ContentType: info.ContentType}, nil
	}
	return nil, nil, jetstream.ErrObjectNotFound
}

// ServerAssetRecordFromAnyBackend builds an AssetRecord by probing the
// server-asset backends. It is primarily for legacy ID-only server-scoped
// assets that need to be rehydrated into richer metadata.
func (c *ChattoCore) ServerAssetRecordFromAnyBackend(ctx context.Context, assetID, filename string) (*evtv1.AssetRecord, error) {
	logicalID, namespaced, natsKeys, ok := serverAssetNATSObjectKeys(assetID)
	if !ok {
		return nil, jetstream.ErrObjectNotFound
	}
	var natsErr error
	for _, objectKey := range natsKeys {
		obj, err := c.storage.serverAssets.Get(ctx, objectKey)
		if err != nil {
			natsErr = err
			continue
		}
		if closer, ok := obj.(io.Closer); ok {
			defer closer.Close()
		}
		info, _ := obj.Info()
		contentType := info.Headers.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return &evtv1.AssetRecord{
			Id:          logicalID,
			Filename:    filename,
			ContentType: contentType,
			Size:        int64(info.Size),
			Storage:     &evtv1.AssetRecord_Nats{Nats: &evtv1.NATSAsset{Key: objectKey}},
		}, nil
	}

	if c.s3Client != nil && !namespaced {
		s3Info, s3Err := c.s3Client.StatObject(ctx, S3KeyServerAsset(logicalID))
		if s3Err == nil {
			contentType := s3Info.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			return &evtv1.AssetRecord{
				Id:          logicalID,
				Filename:    filename,
				ContentType: contentType,
				Size:        s3Info.Size,
				Storage: &evtv1.AssetRecord_S3{S3: &evtv1.S3Asset{
					Key:    logicalID,
					Bucket: proto.String(c.s3Client.Bucket()),
				}},
			}, nil
		}
		c.logger.Debug("Server asset record not found in either backend",
			"asset_id", logicalID,
			"nats_error", natsErr,
			"s3_error", s3Err)
	}

	return nil, natsErr
}

// GetServerAssetFromAnyBackend retrieves a server asset by probing both NATS and S3 backends.
// It tries the canonical SERVER_ASSETS NATS object store first, then S3.
// Returns a reader for the asset content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetServerAssetFromAnyBackend(ctx context.Context, assetID string) (io.Reader, *ServerAssetInfo, error) {
	logicalID, namespaced, natsKeys, ok := serverAssetNATSObjectKeys(assetID)
	if !ok {
		return nil, nil, jetstream.ErrObjectNotFound
	}
	var natsErr error
	for _, objectKey := range natsKeys {
		obj, err := c.storage.serverAssets.Get(ctx, objectKey)
		if err != nil {
			natsErr = err
			continue
		}
		info, _ := obj.Info()
		return obj, &ServerAssetInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
		}, nil
	}

	// If NATS failed and S3 is configured, try S3
	if c.s3Client != nil && !namespaced {
		s3Key := S3KeyServerAsset(logicalID)
		reader, s3Info, s3Err := c.s3Client.GetObject(ctx, s3Key)
		if s3Err == nil {
			return reader, &ServerAssetInfo{
				Size:        s3Info.Size,
				ContentType: s3Info.ContentType,
			}, nil
		}
		// Log S3 error but return the original NATS error
		c.logger.Debug("Instance asset not found in either backend",
			"asset_id", logicalID,
			"nats_error", natsErr,
			"s3_error", s3Err)
	}

	return nil, nil, natsErr
}

// CleanupAsset deletes an asset from the server object store.
// Used to clean up orphaned assets when subsequent operations fail.
func (c *ChattoCore) CleanupAsset(ctx context.Context, asset *evtv1.DeprecatedAsset) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.serverAssets.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned asset", "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned asset", "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		s3Key := S3KeyServerAsset(s3Asset.Key)
		if err := c.s3Client.DeleteObjectFromBucket(ctx, s3Asset.GetBucket(), s3Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key)
		}
	}
	c.deleteCachedResizesForServerAsset(ctx, assetIDFromAsset(asset), "orphaned asset", "")
}

// deleteAsset deletes a server asset from its storage backend (NATS or S3).
// This is a helper for cleaning up old assets when they are replaced.
// For S3, the assetID stored in S3Asset.Key is used to construct the full S3 path.
// The assetType and ownerID are used for logging only.
func (c *ChattoCore) deleteAsset(ctx context.Context, asset *evtv1.DeprecatedAsset, assetType, ownerID string) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.serverAssets.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to delete old "+assetType, "owner_id", ownerID, "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Deleted old "+assetType, "owner_id", ownerID, "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		// S3Asset.Key stores just the assetID; construct the full S3 path
		s3Key := S3KeyServerAsset(s3Asset.Key)
		if err := c.s3Client.DeleteObjectFromBucket(ctx, s3Asset.GetBucket(), s3Key); err != nil {
			c.logger.Warn("Failed to delete old S3 "+assetType, "owner_id", ownerID, "asset_id", s3Asset.Key, "s3_key", s3Key, "error", err)
		} else {
			c.logger.Info("Deleted old S3 "+assetType, "owner_id", ownerID, "asset_id", s3Asset.Key, "s3_key", s3Key)
		}
	}
	c.deleteCachedResizesForServerAsset(ctx, assetIDFromAsset(asset), assetType, ownerID)
}

func (c *ChattoCore) deleteCachedResizesForServerAsset(ctx context.Context, assetID, assetType, ownerID string) {
	assetKeys := []string{assetID}
	if logicalID, namespaced, ok := serverAssetRequestKey(assetID); ok {
		if namespaced {
			assetKeys = append(assetKeys, logicalID)
		} else {
			assetKeys = append(assetKeys, PublicServerAssetObjectKey(logicalID))
		}
	}
	deletedCount := 0
	var cacheErr error
	for _, assetKey := range assetKeys {
		count, err := c.mediaModel.DeleteCachedResizesForServerAsset(ctx, assetKey)
		deletedCount += count
		cacheErr = errors.Join(cacheErr, err)
	}
	if cacheErr != nil {
		c.logger.Warn("Failed to delete cached resizes for server asset",
			"asset_id", assetID,
			"asset_type", assetType,
			"owner_id", ownerID,
			"error", cacheErr)
	} else if deletedCount > 0 {
		c.logger.Debug("Deleted cached resizes for server asset",
			"asset_id", assetID,
			"asset_type", assetType,
			"owner_id", ownerID,
			"deleted_count", deletedCount)
	}
}
