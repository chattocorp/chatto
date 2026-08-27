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
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// UploadUserAvatar processes an image (resizes to 256x256 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// If the user already has an avatar, the old one is deleted after successful upload.
func (c *ChattoCore) UploadUserAvatar(ctx context.Context, userID string, reader io.Reader) (*corev1.AssetRecord, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}

	// Capture old avatar reference for cleanup after successful upload
	oldAvatar, _ := c.GetUserAvatar(ctx, userID)

	// Process image: resize and convert to WebP
	webpReader, err := assets.ProcessAvatarImageWithConfig(reader, c.AssetsConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to process avatar image: %w", err)
	}

	// Read the processed image into bytes (needed for both NATS and S3)
	webpData, err := io.ReadAll(webpReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read processed avatar: %w", err)
	}

	// Upload to storage with unique asset ID
	assetID := NewAssetID()
	asset := &corev1.AssetRecord{
		Id:          assetID,
		Filename:    "avatar.webp",
		ContentType: "image/webp",
		Size:        int64(len(webpData)),
	}

	if c.ShouldUseS3() {
		// Upload to S3 - use the same assetID as NATS would use for the key
		// The S3 path is constructed from the assetID for consistency
		s3Key := S3KeyServerAsset(assetID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp")
		if err != nil {
			return nil, fmt.Errorf("failed to upload avatar to S3: %w", err)
		}
		// Store just the assetID in Key (same as NATS) so URL generation is consistent
		asset.Storage = &corev1.AssetRecord_S3{
			S3: &corev1.S3Asset{
				Key:    assetID,
				Bucket: proto.String(c.s3Client.Bucket()),
			},
		}
		c.logger.Info("Uploaded avatar to S3", "user_id", userID, "asset_id", assetID, "size", len(webpData))
	} else {
		// Upload to NATS ObjectStore
		headers := nats.Header{}
		headers.Set("Content-Type", "image/webp")
		objectKey := PublicServerAssetObjectKey(assetID)
		meta := jetstream.ObjectMeta{
			Name:    objectKey,
			Headers: headers,
		}
		info, err := c.storage.serverAssets.Put(ctx, meta, bytes.NewReader(webpData))
		if err != nil {
			return nil, fmt.Errorf("failed to upload avatar: %w", err)
		}
		asset.Storage = &corev1.AssetRecord_Nats{
			Nats: &corev1.NATSAsset{
				Key: objectKey,
			},
		}
		c.logger.Info("Uploaded avatar", "user_id", userID, "size", info.Size)
	}

	// Delete old avatar now that new one is successfully uploaded
	if oldAvatar != nil {
		c.deleteAsset(ctx, assetStorageFromAsset(oldAvatar), "avatar", userID)
	}

	return asset, nil
}

// SetUserAvatar stores the user's avatar asset reference through the user aggregate.
func (c *ChattoCore) SetUserAvatar(ctx context.Context, userID string, asset *corev1.AssetRecord) error {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return err
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_AssetCreated{
		AssetCreated: &corev1.AssetCreatedEvent{
			Asset:                   asset,
			OriginalBinaryAvailable: true,
			UserId:                  userID,
		},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return fmt.Errorf("failed to store avatar: %w", err)
	}

	c.logger.Info("Updated user avatar", "user_id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return nil
}

// GetUserAvatar retrieves a user's avatar asset reference from the user projection.
// Returns nil if the user has no avatar set.
func (c *ChattoCore) GetUserAvatar(ctx context.Context, userID string) (*corev1.AssetRecord, error) {
	if asset, ok := c.userModel.avatar(userID); ok {
		return asset, nil
	}
	return nil, nil
}

// DeleteUserAvatar removes a user's avatar from storage (NATS or S3).
// Returns nil if the user has no avatar set.
func (c *ChattoCore) DeleteUserAvatar(ctx context.Context, userID string) error {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return err
	}

	// Get current avatar to delete the file from storage
	avatar, err := c.GetUserAvatar(ctx, userID)
	if err != nil {
		return err
	}

	// If no avatar, nothing to do
	if avatar == nil {
		return nil
	}

	// Delete the asset from storage (NATS or S3)
	c.deleteAsset(ctx, assetStorageFromAsset(avatar), "avatar", userID)

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserAvatarCleared{
		UserAvatarCleared: &corev1.UserAvatarClearedEvent{UserId: userID},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return fmt.Errorf("failed to delete avatar reference: %w", err)
	}

	c.logger.Info("Deleted user avatar", "user_id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return nil
}

func (c *ChattoCore) RecordUserAssetDeleted(ctx context.Context, actorID, userID, assetID string) error {
	if userID == "" || assetID == "" {
		return fmt.Errorf("user asset deletion missing user or asset id")
	}
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: assetID},
		},
	})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return fmt.Errorf("failed to record user asset deletion: %w", err)
	}
	return nil
}

// GetUserAvatarURL returns the URL for a user's avatar.
// If width and height are provided (non-nil), returns a URL to a resized version.
// Returns empty string if no avatar is set.
func (c *ChattoCore) GetUserAvatarURL(ctx context.Context, userID string, width, height *int, fit string) (string, error) {
	avatar, err := c.GetUserAvatar(ctx, userID)
	if err != nil {
		return "", err
	}

	// No avatar set
	if avatar == nil {
		return "", nil
	}

	assetKey := ServerAssetDeliveryKey(avatar)
	if assetKey == "" {
		return "", fmt.Errorf("unknown asset type")
	}

	// Always use the standard server asset URL format - storage backend is an internal detail
	if width != nil && height != nil {
		if fit == "" {
			fit = "cover"
		}
		return c.GetTransformedServerAssetURL(assetKey, *width, *height, fit), nil
	}
	return c.assetURL(fmt.Sprintf("/assets/server/%s", assetKey)), nil
}
