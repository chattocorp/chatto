package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/linkpreview"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// GetLinkPreview fetches link preview metadata for a URL.
// Results are cached server-side. Returns nil if the URL cannot be previewed.
func (c *ChattoCore) GetLinkPreview(ctx context.Context, url string) (*corev1.LinkPreview, error) {
	// Check cache first
	cached, err := c.linkPreviewCache.Get(ctx, url)
	if errors.Is(err, linkpreview.ErrCachedFailure) {
		// Negative cache hit - URL previously failed, don't re-fetch
		return nil, nil
	}
	if err != nil {
		c.logger.Warn("Failed to get cached link preview", "url", url, "error", err)
		// Continue to fetch - don't fail on cache errors
	}
	if cached != nil {
		if err := c.markCachedLegacyLinkPreviewPublic(ctx, cached); err != nil {
			c.logger.Warn("Failed to preserve cached legacy link-preview image", "asset_id", cached.GetImageAssetId(), "error", err)
		}
		return cached, nil
	}

	// Fetch the preview
	result, err := c.linkPreviewFetcher.Fetch(ctx, url)
	if err != nil {
		// Cache the failure to avoid repeated fetches
		_ = c.linkPreviewCache.SetFailure(ctx, url, err.Error())
		if errors.Is(err, linkpreview.ErrUnavailable) {
			return nil, nil
		}
		return nil, err
	}

	preview := result.ToProto(url)
	if err := validateLinkPreview(preview); err != nil {
		_ = c.linkPreviewCache.SetFailure(ctx, url, err.Error())
		c.logger.Warn("Discarding invalid fetched link preview", "url", url, "error", err)
		return nil, nil
	}

	// Cache the result
	if err := c.linkPreviewCache.Set(ctx, url, preview); err != nil {
		c.logger.Warn("Failed to cache link preview", "url", url, "error", err)
	}

	return preview, nil
}

// markCachedLegacyLinkPreviewPublic preserves a pre-namespace link-preview
// image that is still referenced by the server's runtime cache but has not yet
// appeared in durable message history. The cached server-issued AssetRecord
// must bind one exact canonical flat NATS key; private declarations and metadata
// always win. Only object metadata is updated—the object body is never opened.
func (c *ChattoCore) markCachedLegacyLinkPreviewPublic(ctx context.Context, preview *corev1.LinkPreview) error {
	if preview == nil || c.assetModel == nil || c.assetModel.assets.Projection() == nil {
		return nil
	}
	asset := preview.GetImageAsset()
	assetID := preview.GetImageAssetId()
	if asset == nil || assetID == "" || asset.GetId() != assetID {
		return nil
	}
	natsAsset := asset.GetNats()
	if natsAsset == nil || natsAsset.GetKey() != assetID {
		return nil
	}
	logicalID, namespaced, ok := serverAssetRequestKey(natsAsset.GetKey())
	if !ok || namespaced || logicalID != assetID {
		return nil
	}
	assetState := c.assetModel.AssetState(assetID)
	if assetState.Creation != nil || assetState.Deleted {
		return nil
	}

	info, err := c.storage.serverAssets.GetInfo(ctx, assetID)
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil
		}
		return fmt.Errorf("inspect legacy preview object metadata: %w", err)
	}
	if info.Headers.Get("Room-Id") != "" || info.Headers.Get("Upload-Id") != "" ||
		info.Headers.Get(ServerAssetVisibilityHeader) == ServerAssetVisibilityPublic {
		return nil
	}

	headers := maps.Clone(info.Headers)
	if headers == nil {
		headers = make(nats.Header)
	}
	headers.Set(ServerAssetVisibilityHeader, ServerAssetVisibilityPublic)
	headers.Set(ServerAssetVisibilityNUIDHeader, info.NUID)
	headers.Set(ServerAssetVisibilityDigestHeader, info.Digest)
	if err := c.storage.serverAssets.UpdateMeta(ctx, assetID, jetstream.ObjectMeta{
		Name:        assetID,
		Description: info.Description,
		Headers:     headers,
		Metadata:    maps.Clone(info.Metadata),
	}); err != nil {
		return fmt.Errorf("mark legacy preview object public: %w", err)
	}
	updated, err := c.storage.serverAssets.GetInfo(ctx, assetID)
	if err != nil {
		return fmt.Errorf("verify legacy preview object metadata: %w", err)
	}
	if updated.Headers.Get("Room-Id") != "" || updated.Headers.Get("Upload-Id") != "" ||
		!serverAssetVisibilityMarkerMatches(updated) {
		return fmt.Errorf("legacy preview object generation changed during metadata update")
	}
	return nil
}

// HydrateLinkPreviewImageAsset ensures a posted LinkPreview carries the
// server-issued AssetRecord for its preview image. Clients only send
// image_asset_id for compatibility; the backend rehydrates the storage pointer
// from the server-side preview cache or by probing known backends.
func (c *ChattoCore) HydrateLinkPreviewImageAsset(ctx context.Context, preview *corev1.LinkPreview) error {
	if preview == nil {
		return nil
	}

	imageAsset := preview.GetImageAsset()
	imageAssetID := preview.GetImageAssetId()
	if imageAsset != nil {
		if imageAsset.GetId() == "" {
			return fmt.Errorf("link preview image asset record is missing id")
		}
		if imageAssetID != "" && imageAssetID != imageAsset.GetId() {
			return fmt.Errorf("link preview image asset id mismatch")
		}
		if imageAssetID == "" {
			id := imageAsset.GetId()
			preview.ImageAssetId = &id
		}
		return nil
	}
	if imageAssetID == "" {
		return nil
	}

	if cached, err := c.linkPreviewCache.Get(ctx, preview.GetUrl()); err == nil && cached != nil {
		if cachedAsset := cached.GetImageAsset(); cachedAsset != nil && cachedAsset.GetId() == imageAssetID {
			preview.ImageAsset = proto.Clone(cachedAsset).(*corev1.AssetRecord)
			return nil
		}
	} else if err != nil && !errors.Is(err, linkpreview.ErrCachedFailure) {
		c.logger.Debug("Failed to hydrate link preview image asset from cache", "url", preview.GetUrl(), "error", err)
	}

	asset, err := c.ServerAssetRecordFromAnyBackend(ctx, imageAssetID, "link-preview.webp")
	if err != nil {
		return fmt.Errorf("hydrate link preview image asset: %w", err)
	}
	preview.ImageAsset = asset
	return nil
}

func (c *ChattoCore) storeLinkPreviewImage(ctx context.Context, assetID string, data []byte, contentType string) (*corev1.AssetRecord, error) {
	asset := &corev1.AssetRecord{
		Id:          assetID,
		Filename:    "link-preview.webp",
		ContentType: contentType,
		Size:        int64(len(data)),
	}
	if c.ShouldUseS3() {
		s3Key := S3KeyServerAsset(assetID)
		if _, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, data, contentType); err != nil {
			return nil, fmt.Errorf("upload link preview image to S3: %w", err)
		}
		asset.Storage = &corev1.AssetRecord_S3{S3: &corev1.S3Asset{
			Key:    assetID,
			Bucket: proto.String(c.s3Client.Bucket()),
		}}
		c.logger.Debug("Stored link preview image in S3", "asset_id", assetID, "size", len(data))
		return asset, nil
	}

	objectKey := PublicServerAssetObjectKey(assetID)
	meta := jetstream.ObjectMeta{
		Name: objectKey,
		Headers: map[string][]string{
			"Content-Type": {contentType},
		},
	}
	info, err := c.storage.serverAssets.Put(ctx, meta, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("upload link preview image to SERVER_ASSETS: %w", err)
	}
	asset.Size = int64(info.Size)
	asset.Storage = &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: objectKey}}
	c.logger.Debug("Stored link preview image in SERVER_ASSETS", "asset_id", assetID, "size", len(data))
	return asset, nil
}
