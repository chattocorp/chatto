package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Instance branding lives on the instance, not the underlying primary-space
// record. The asset bytes still live in the same NATS object store / S3
// bucket as space assets — only the KV pointer is keyed at the instance
// level via these constants.
const (
	serverLogoKey   = "instance.logo"
	serverBannerKey = "instance.banner"
)

// UploadServerLogo processes a logo image (resize + WebP) and uploads the
// bytes to the object store. Returns the asset reference. Use SetServerLogo
// to atomically swap the instance's logo pointer (and clean up the prior
// asset).
func (c *ChattoCore) UploadServerLogo(ctx context.Context, reader io.Reader) (*corev1.Asset, error) {
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
// SetServerBanner to atomically swap the instance's banner pointer (and
// clean up the prior asset).
//
// Banners double as the OG link-preview image, so they're processed at the
// canonical 1200x630 OG aspect rather than the older 4:3 sidebar shape.
func (c *ChattoCore) UploadServerBanner(ctx context.Context, reader io.Reader) (*corev1.Asset, error) {
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
// instance-level logo and banner upload paths.
func (c *ChattoCore) uploadServerAsset(ctx context.Context, webpData []byte, kind string) (*corev1.Asset, error) {
	assetID := NewAssetID()

	if c.ShouldUseS3() {
		s3Key := S3KeyServerAsset(assetID)
		if _, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp"); err != nil {
			return nil, fmt.Errorf("failed to upload %s to S3: %w", kind, err)
		}
		c.logger.Info("Uploaded instance "+kind+" to S3", "asset_id", assetID, "size", len(webpData))
		return &corev1.Asset{
			Asset: &corev1.Asset_S3{
				S3: &corev1.S3Asset{
					Key:    assetID,
					Bucket: proto.String(c.s3Client.Bucket()),
				},
			},
		}, nil
	}

	headers := nats.Header{}
	headers.Set("Content-Type", "image/webp")
	meta := jetstream.ObjectMeta{
		Name:    assetID,
		Headers: headers,
	}
	info, err := c.storage.serverStore.Put(ctx, meta, bytes.NewReader(webpData))
	if err != nil {
		return nil, fmt.Errorf("failed to upload %s: %w", kind, err)
	}
	c.logger.Info("Uploaded instance "+kind, "asset_id", assetID, "size", info.Size)
	return &corev1.Asset{
		Asset: &corev1.Asset_Nats{
			Nats: &corev1.NATSAsset{
				Key: assetID,
			},
		},
	}, nil
}

// SetServerLogo atomically points the instance at a new logo asset using
// optimistic locking, and cleans up the prior asset on success.
func (c *ChattoCore) SetServerLogo(ctx context.Context, actorID string, asset *corev1.Asset) error {
	return c.setServerBrandingAsset(ctx, actorID, serverLogoKey, "logo", asset)
}

// SetServerBanner atomically points the instance at a new banner asset
// using optimistic locking, and cleans up the prior asset on success.
func (c *ChattoCore) SetServerBanner(ctx context.Context, actorID string, asset *corev1.Asset) error {
	return c.setServerBrandingAsset(ctx, actorID, serverBannerKey, "banner", asset)
}

// setServerBrandingAsset is the shared OCC swap implementation backing
// SetServerLogo / SetServerBanner. Publishes ServerUpdatedEvent on
// success so subscribers can refetch the updated branding.
func (c *ChattoCore) setServerBrandingAsset(ctx context.Context, actorID, key, kind string, asset *corev1.Asset) error {
	const maxRetries = 5

	assetData, err := proto.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal %s asset: %w", kind, err)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		var revision uint64
		var oldAsset *corev1.Asset

		entry, err := c.storage.serverKV.Get(ctx, key)
		if err == nil {
			revision = entry.Revision()
			oldAsset = &corev1.Asset{}
			if unmarshalErr := proto.Unmarshal(entry.Value(), oldAsset); unmarshalErr != nil {
				c.logger.Warn("Failed to unmarshal old "+kind+" asset", "error", unmarshalErr)
				oldAsset = nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to get current %s: %w", kind, err)
		}

		var updateErr error
		if revision == 0 {
			_, updateErr = c.storage.serverKV.Create(ctx, key, assetData)
		} else {
			_, updateErr = c.storage.serverKV.Update(ctx, key, assetData, revision)
		}

		if updateErr == nil {
			if oldAsset != nil {
				c.deleteAsset(ctx, oldAsset, kind, "instance")
			}
			c.PublishServerBrandingUpdate(ctx, actorID)
			c.logger.Info("Updated instance "+kind, "asset_id", assetIDFromAsset(asset))
			return nil
		}

		if errors.Is(updateErr, jetstream.ErrKeyExists) {
			c.logger.Debug(kind+" update revision conflict, retrying", "attempt", attempt+1)
			continue
		}

		return fmt.Errorf("failed to store %s: %w", kind, updateErr)
	}

	return fmt.Errorf("failed to update %s after %d retries due to concurrent modifications", kind, maxRetries)
}

// GetServerLogo returns the asset reference for the instance's current
// logo, or (nil, nil) if no logo is set.
func (c *ChattoCore) GetServerLogo(ctx context.Context) (*corev1.Asset, error) {
	return c.getServerBrandingAsset(ctx, serverLogoKey, "logo")
}

// GetServerBanner returns the asset reference for the instance's current
// banner, or (nil, nil) if no banner is set.
func (c *ChattoCore) GetServerBanner(ctx context.Context) (*corev1.Asset, error) {
	return c.getServerBrandingAsset(ctx, serverBannerKey, "banner")
}

func (c *ChattoCore) getServerBrandingAsset(ctx context.Context, key, kind string) (*corev1.Asset, error) {
	entry, err := c.storage.serverKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get %s: %w", kind, err)
	}
	asset := &corev1.Asset{}
	if err := proto.Unmarshal(entry.Value(), asset); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s asset: %w", kind, err)
	}
	return asset, nil
}

// GetServerLogoURL returns the URL for the instance's logo, optionally
// transformed to the given dimensions. Returns empty string when no logo
// is set.
func (c *ChattoCore) GetServerLogoURL(ctx context.Context, width, height *int) (string, error) {
	logo, err := c.GetServerLogo(ctx)
	if err != nil || logo == nil {
		return "", err
	}
	return c.serverAssetURL(logo, width, height), nil
}

// GetServerBannerURL returns the URL for the instance's banner, optionally
// transformed to the given dimensions. Returns empty string when no banner
// is set.
func (c *ChattoCore) GetServerBannerURL(ctx context.Context, width, height *int) (string, error) {
	banner, err := c.GetServerBanner(ctx)
	if err != nil || banner == nil {
		return "", err
	}
	return c.serverAssetURL(banner, width, height), nil
}

// serverAssetURL builds the public URL for an instance-scoped asset,
// optionally with transform parameters.
func (c *ChattoCore) serverAssetURL(asset *corev1.Asset, width, height *int) string {
	assetID := assetIDFromAsset(asset)
	if assetID == "" {
		return ""
	}
	if width != nil && height != nil {
		return c.GetTransformedServerAssetURL(assetID, *width, *height, "cover")
	}
	return c.assetURL(fmt.Sprintf("/assets/instance/%s", assetID))
}

// DeleteServerLogo clears the instance's logo (KV pointer + object-store
// asset). No-op when no logo is set.
func (c *ChattoCore) DeleteServerLogo(ctx context.Context, actorID string) error {
	return c.deleteServerBrandingAsset(ctx, actorID, serverLogoKey, "logo")
}

// DeleteServerBanner clears the instance's banner. No-op when no banner
// is set.
func (c *ChattoCore) DeleteServerBanner(ctx context.Context, actorID string) error {
	return c.deleteServerBrandingAsset(ctx, actorID, serverBannerKey, "banner")
}

func (c *ChattoCore) deleteServerBrandingAsset(ctx context.Context, actorID, key, kind string) error {
	const maxRetries = 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil
			}
			return fmt.Errorf("failed to get current %s: %w", kind, err)
		}

		revision := entry.Revision()
		asset := &corev1.Asset{}
		if unmarshalErr := proto.Unmarshal(entry.Value(), asset); unmarshalErr != nil {
			c.logger.Warn("Failed to unmarshal "+kind+" asset for deletion", "error", unmarshalErr)
		}

		if deleteErr := c.storage.serverKV.Delete(ctx, key, jetstream.LastRevision(revision)); deleteErr == nil {
			c.deleteAsset(ctx, asset, kind, "instance")
			c.PublishServerBrandingUpdate(ctx, actorID)
			c.logger.Info("Deleted instance " + kind)
			return nil
		} else if errors.Is(deleteErr, jetstream.ErrKeyExists) {
			c.logger.Debug(kind+" delete revision conflict, retrying", "attempt", attempt+1)
			continue
		} else {
			return fmt.Errorf("failed to delete %s: %w", kind, deleteErr)
		}
	}

	return fmt.Errorf("failed to delete %s after %d retries due to concurrent modifications", kind, maxRetries)
}

// PublishServerBrandingUpdate publishes a ServerUpdatedEvent carrying the
// current name + logo + banner from instance-scoped storage. Best-effort: a
// publish failure does not roll back the underlying KV change.
func (c *ChattoCore) PublishServerBrandingUpdate(ctx context.Context, actorID string) {
	name := ""
	description := ""
	if cm := c.ConfigManager(); cm != nil {
		if n, err := cm.GetEffectiveInstanceName(ctx); err == nil {
			name = n
		}
		if cfg, _, err := cm.GetInstanceConfig(ctx); err == nil && cfg != nil {
			description = cfg.Description
		}
	}

	logoURL, err := c.GetServerLogoURL(ctx, nil, nil)
	if err != nil {
		c.logger.Warn("failed to get instance logo URL for update event", "error", err)
		logoURL = ""
	}
	bannerURL, err := c.GetServerBannerURL(ctx, nil, nil)
	if err != nil {
		c.logger.Warn("failed to get instance banner URL for update event", "error", err)
		bannerURL = ""
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_ServerUpdated{
			ServerUpdated: &corev1.ServerUpdatedEvent{
				ServerId:    ServerSpaceID,
				Name:        name,
				Description: description,
				LogoUrl:     logoURL,
				BannerUrl:   bannerURL,
			},
		},
	})

	subject := subjects.LiveConfigEvent("server_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish server update event", "error", err)
	}
}

// assetIDFromAsset extracts the asset ID from a NATS- or S3-backed asset
// reference. Returns empty string for unknown asset variants.
func assetIDFromAsset(asset *corev1.Asset) string {
	switch a := asset.Asset.(type) {
	case *corev1.Asset_Nats:
		return a.Nats.Key
	case *corev1.Asset_S3:
		return a.S3.Key
	default:
		return ""
	}
}
