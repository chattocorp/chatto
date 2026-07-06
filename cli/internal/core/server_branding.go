package core

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Historical INSTANCE KV keys for server branding.
const (
	serverLogoKey   = "instance.logo"
	serverBannerKey = "instance.banner"
)

// UploadServerLogo processes a logo image (resize + WebP) and uploads the
// bytes to the object store. Returns the asset reference. Use SetServerLogo
// to atomically swap the server's logo pointer (and clean up the prior
// asset).
func (c *ChattoCore) UploadServerLogo(ctx context.Context, reader io.Reader) (*corev1.AssetRecord, error) {
	webpReader, err := assets.ProcessLogoImageWithConfig(reader, c.AssetsConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to process logo image: %w", err)
	}
	webpData, err := io.ReadAll(webpReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read processed logo: %w", err)
	}
	return c.uploadServerAsset(ctx, webpData, "logo")
}

// UploadServerBanner processes a banner image (resize + WebP) and uploads
// the bytes to the object store. Returns the asset reference. Use
// SetServerBanner to atomically swap the server's banner pointer (and
// clean up the prior asset).
//
// Banners double as the OG link-preview image, so they're processed at the
// canonical 1200x630 OG aspect rather than the older 4:3 sidebar shape.
func (c *ChattoCore) UploadServerBanner(ctx context.Context, reader io.Reader) (*corev1.AssetRecord, error) {
	webpReader, err := assets.ProcessLinkPreviewImageWithConfig(reader, c.AssetsConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to process banner image: %w", err)
	}
	webpData, err := io.ReadAll(webpReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read processed banner: %w", err)
	}
	return c.uploadServerAsset(ctx, webpData, "banner")
}

// uploadServerAsset routes processed image bytes to NATS or S3 based on
// configuration and returns the resulting asset reference. Used by the
// server-level logo and banner upload paths.
func (c *ChattoCore) uploadServerAsset(ctx context.Context, webpData []byte, kind string) (*corev1.AssetRecord, error) {
	assetID := NewAssetID()
	asset := &corev1.AssetRecord{
		Id:          assetID,
		Filename:    kind + ".webp",
		ContentType: "image/webp",
		Size:        int64(len(webpData)),
	}

	if c.ShouldUseS3() {
		s3Key := S3KeyServerAsset(assetID)
		if _, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp"); err != nil {
			return nil, fmt.Errorf("failed to upload %s to S3: %w", kind, err)
		}
		c.logger.Info("Uploaded server "+kind+" to S3", "asset_id", assetID, "size", len(webpData))
		asset.Storage = &corev1.AssetRecord_S3{S3: &corev1.S3Asset{
			Key:    assetID,
			Bucket: proto.String(c.s3Client.Bucket()),
		}}
		return asset, nil
	}

	headers := nats.Header{}
	headers.Set("Content-Type", "image/webp")
	meta := jetstream.ObjectMeta{
		Name:    assetID,
		Headers: headers,
	}
	info, err := c.storage.serverAssets.Put(ctx, meta, bytes.NewReader(webpData))
	if err != nil {
		return nil, fmt.Errorf("failed to upload %s: %w", kind, err)
	}
	c.logger.Info("Uploaded server "+kind, "asset_id", assetID, "size", info.Size)
	asset.Size = int64(info.Size)
	asset.Storage = &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: assetID}}
	return asset, nil
}

// SetServerLogo atomically points the instance at a new logo asset using
// optimistic locking, and cleans up the prior asset on success.
func (c *ChattoCore) SetServerLogo(ctx context.Context, actorID string, asset *corev1.AssetRecord) error {
	return c.setServerBrandingAsset(ctx, actorID, "logo", asset)
}

// SetServerBanner atomically points the instance at a new banner asset
// using optimistic locking, and cleans up the prior asset on success.
func (c *ChattoCore) SetServerBanner(ctx context.Context, actorID string, asset *corev1.AssetRecord) error {
	return c.setServerBrandingAsset(ctx, actorID, "banner", asset)
}

// setServerBrandingAsset is the shared OCC swap implementation backing
// SetServerLogo / SetServerBanner. Publishes ServerUpdatedEvent on
// success so subscribers can refetch the updated branding.
func (c *ChattoCore) setServerBrandingAsset(ctx context.Context, actorID, kind string, asset *corev1.AssetRecord) error {
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return fmt.Errorf("config model not configured")
	}
	if asset == nil {
		return c.deleteServerBrandingAsset(ctx, actorID, kind)
	}

	var oldAsset *corev1.AssetRecord
	changed := false
	err := c.configManager.model.updateSubject(ctx, ConfigSubjectServer, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		oldAsset = c.projectedServerBrandingAsset(kind)
		if assetRecordsEqual(oldAsset, asset) {
			changed = false
			return nil, nil
		}
		changed = true
		if kind == "logo" {
			return []*corev1.Event{newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerLogoSet{
				ServerLogoSet: &corev1.ServerLogoSetEvent{Asset: cloneAssetRecord(asset)},
			}})}, nil
		}
		return []*corev1.Event{newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerBannerSet{
			ServerBannerSet: &corev1.ServerBannerSetEvent{Asset: cloneAssetRecord(asset)},
		}})}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to store %s: %w", kind, err)
	}
	if !changed {
		return nil
	}
	if oldAsset != nil {
		c.deleteAsset(ctx, assetStorageFromAsset(oldAsset), kind, "server")
	}
	c.PublishServerUpdated(ctx, actorID)
	c.logger.Info("Updated server "+kind, "asset_id", asset.GetId())
	return nil
}

// GetServerLogo returns the asset reference for the server's current
// logo, or (nil, nil) if no logo is set.
func (c *ChattoCore) GetServerLogo(ctx context.Context) (*corev1.AssetRecord, error) {
	return c.getServerBrandingAsset(ctx, "logo")
}

// GetServerBanner returns the asset reference for the server's current
// banner, or (nil, nil) if no banner is set.
func (c *ChattoCore) GetServerBanner(ctx context.Context) (*corev1.AssetRecord, error) {
	return c.getServerBrandingAsset(ctx, "banner")
}

func (c *ChattoCore) getServerBrandingAsset(_ context.Context, kind string) (*corev1.AssetRecord, error) {
	if c.ServerConfig == nil {
		return nil, nil
	}
	return c.projectedServerBrandingAsset(kind), nil
}

func (c *ChattoCore) projectedServerBrandingAsset(kind string) *corev1.AssetRecord {
	if c.ServerConfig == nil {
		return nil
	}
	if kind == "logo" {
		asset, _, _ := c.ServerConfig.ServerLogo()
		return asset
	}
	asset, _, _ := c.ServerConfig.ServerBanner()
	return asset
}

// GetServerLogoURL returns the URL for the server's logo, optionally
// transformed to the given dimensions. Returns empty string when no logo
// is set.
func (c *ChattoCore) GetServerLogoURL(ctx context.Context, width, height *int, fit string) (string, error) {
	logo, err := c.GetServerLogo(ctx)
	if err != nil || logo == nil {
		return "", err
	}
	return c.serverAssetURL(logo, width, height, fit), nil
}

// GetServerBannerURL returns the URL for the server's banner, optionally
// transformed to the given dimensions. Returns empty string when no banner
// is set.
func (c *ChattoCore) GetServerBannerURL(ctx context.Context, width, height *int, fit string) (string, error) {
	banner, err := c.GetServerBanner(ctx)
	if err != nil || banner == nil {
		return "", err
	}
	return c.serverAssetURL(banner, width, height, fit), nil
}

// serverAssetURL builds the public URL for an server-scoped asset,
// optionally with transform parameters.
func (c *ChattoCore) serverAssetURL(asset *corev1.AssetRecord, width, height *int, fit string) string {
	assetID := asset.GetId()
	if assetID == "" {
		return ""
	}
	if width != nil && height != nil {
		if fit == "" {
			fit = "cover"
		}
		return c.GetTransformedServerAssetURL(assetID, *width, *height, fit)
	}
	return c.assetURL(fmt.Sprintf("/assets/server/%s", assetID))
}

// DeleteServerLogo clears the server's logo pointer and object-store asset.
// No-op when no logo is set.
func (c *ChattoCore) DeleteServerLogo(ctx context.Context, actorID string) error {
	return c.deleteServerBrandingAsset(ctx, actorID, "logo")
}

// DeleteServerBanner clears the server's banner. No-op when no banner
// is set.
func (c *ChattoCore) DeleteServerBanner(ctx context.Context, actorID string) error {
	return c.deleteServerBrandingAsset(ctx, actorID, "banner")
}

func (c *ChattoCore) deleteServerBrandingAsset(ctx context.Context, actorID, kind string) error {
	if c.configManager == nil || c.configManager.model == nil || c.ServerConfig == nil {
		return fmt.Errorf("config model not configured")
	}

	var asset *corev1.AssetRecord
	changed := false
	err := c.configManager.model.updateSubject(ctx, ConfigSubjectServer, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		asset = c.projectedServerBrandingAsset(kind)
		if asset == nil {
			changed = false
			return nil, nil
		}
		changed = true
		if kind == "logo" {
			return []*corev1.Event{newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerLogoCleared{
				ServerLogoCleared: &corev1.ServerLogoClearedEvent{},
			}})}, nil
		}
		return []*corev1.Event{newEvent(actorID, &corev1.Event{Event: &corev1.Event_ServerBannerCleared{
			ServerBannerCleared: &corev1.ServerBannerClearedEvent{},
		}})}, nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete %s: %w", kind, err)
	}
	if !changed {
		return nil
	}
	c.deleteAsset(ctx, assetStorageFromAsset(asset), kind, "server")
	c.PublishServerUpdated(ctx, actorID)
	c.logger.Info("Deleted server " + kind)
	return nil
}

// PublishServerUpdated publishes the member-visible server profile/config
// refresh signal. It carries only public presentation fields; clients should
// treat arrival as a refetch trigger for Server.profile.
//
// Best-effort: a publish failure does not roll back the underlying config
// change.
func (c *ChattoCore) PublishServerUpdated(ctx context.Context, actorID string) {
	name := ""
	description := ""
	if cm := c.ConfigManager(); cm != nil {
		if n, err := cm.GetEffectiveServerName(ctx); err == nil {
			name = n
		}
		if cfg, err := cm.GetServerConfig(ctx); err == nil && cfg != nil {
			description = cfg.Description
		}
	}

	logoURL, err := c.GetServerLogoURL(ctx, nil, nil, "")
	if err != nil {
		c.logger.Warn("failed to get instance logo URL for update event", "error", err)
		logoURL = ""
	}
	bannerURL, err := c.GetServerBannerURL(ctx, nil, nil, "")
	if err != nil {
		c.logger.Warn("failed to get instance banner URL for update event", "error", err)
		bannerURL = ""
	}

	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_ServerUpdated{
			ServerUpdated: &corev1.ServerUpdatedEvent{
				ServerId:    LegacyServerSpaceID,
				Name:        name,
				Description: description,
				LogoUrl:     logoURL,
				BannerUrl:   bannerURL,
			},
		},
	})

	subject := subjects.LiveSyncConfigEvent("server_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish server update event", "error", err)
	}
}

// assetIDFromAsset extracts the asset ID from a NATS- or S3-backed asset
// reference. Returns empty string for unknown asset variants.
func assetIDFromAsset(asset *corev1.DeprecatedAsset) string {
	if asset == nil {
		return ""
	}
	switch a := asset.Asset.(type) {
	case *corev1.DeprecatedAsset_Nats:
		return a.Nats.Key
	case *corev1.DeprecatedAsset_S3:
		return a.S3.Key
	default:
		return ""
	}
}

func assetRecordsEqual(a, b *corev1.AssetRecord) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a, b)
}
