package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// UpdateUserAvatar uploads and sets an avatar for targetUserID after applying
// the target-aware user and bot management authorization policy.
func (c *ChattoCore) UpdateUserAvatar(ctx context.Context, actorID, targetUserID string, reader io.Reader) (*evtv1.User, error) {
	if _, err := c.requireCanManageUserAvatar(ctx, actorID, targetUserID); err != nil {
		return nil, err
	}

	asset, err := c.storeUserAvatarAsset(ctx, targetUserID, reader)
	if err != nil {
		return nil, err
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_AssetCreated{
		AssetCreated: &evtv1.AssetCreatedEvent{
			Asset:                   asset,
			OriginalBinaryAvailable: true,
			UserId:                  targetUserID,
		},
	}})
	previous, committed, err := c.appendManagedAvatarEvent(ctx, actorID, targetUserID, event, false)
	if err != nil {
		if !committed {
			c.CleanupAsset(ctx, DeprecatedAssetFromAsset(asset))
		} else if previous != nil && previous.GetId() != asset.GetId() {
			c.deleteAsset(ctx, assetStorageFromAsset(previous), "avatar", targetUserID)
		}
		return nil, fmt.Errorf("failed to store avatar: %w", err)
	}
	if previous != nil && previous.GetId() != asset.GetId() {
		c.deleteAsset(ctx, assetStorageFromAsset(previous), "avatar", targetUserID)
	}

	c.logger.Info("Updated user avatar", "actor_id", actorID, "user_id", targetUserID)
	c.publishUserProfileUpdate(ctx, targetUserID)
	return c.GetUser(ctx, targetUserID)
}

// ClearUserAvatar removes the target user's avatar after applying the same
// authorization policy as UpdateUserAvatar. The operation is idempotent.
func (c *ChattoCore) ClearUserAvatar(ctx context.Context, actorID, targetUserID string) (*evtv1.User, error) {
	if _, err := c.requireCanManageUserAvatar(ctx, actorID, targetUserID); err != nil {
		return nil, err
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserAvatarCleared{
		UserAvatarCleared: &evtv1.UserAvatarClearedEvent{UserId: targetUserID},
	}})
	previous, committed, err := c.appendManagedAvatarEvent(ctx, actorID, targetUserID, event, true)
	if err != nil {
		if committed && previous != nil {
			c.deleteAsset(ctx, assetStorageFromAsset(previous), "avatar", targetUserID)
		}
		return nil, fmt.Errorf("failed to delete avatar reference: %w", err)
	}
	if previous == nil {
		return c.GetUser(ctx, targetUserID)
	}
	c.deleteAsset(ctx, assetStorageFromAsset(previous), "avatar", targetUserID)
	c.logger.Info("Deleted user avatar", "actor_id", actorID, "user_id", targetUserID)
	c.publishUserProfileUpdate(ctx, targetUserID)
	return c.GetUser(ctx, targetUserID)
}

func (c *ChattoCore) requireCanManageUserAvatar(ctx context.Context, actorID, targetUserID string) (*evtv1.User, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	if targetUserID == "" {
		return nil, invalidArgument("target user ID is required")
	}
	if !isCanonicalUserID(targetUserID) {
		return nil, invalidArgument("target user ID is invalid")
	}
	target, err := c.GetUser(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	if actorID == targetUserID {
		return target, nil
	}
	actor, err := c.GetUser(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if actor.GetIsBot() {
		return nil, ErrPermissionDenied
	}
	canManageAccounts, err := c.CanManageUserAccounts(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("check user.manage-accounts: %w", err)
	}
	if !target.GetIsBot() {
		if !canManageAccounts {
			return nil, ErrPermissionDenied
		}
		return target, nil
	}
	if target.GetBotOwnerUserId() == actorID || canManageAccounts {
		return target, nil
	}
	canManageBots, err := c.CanManageBots(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("check bot.manage: %w", err)
	}
	if !canManageBots {
		return nil, ErrPermissionDenied
	}
	return target, nil
}

// appendManagedAvatarEvent commits one avatar fact against the target user and
// the global authorization fence. It returns the avatar that was current at
// the successful OCC attempt. When skipIfMissing is true, an absent avatar is
// an authorized no-op and no event is appended.
func (c *ChattoCore) appendManagedAvatarEvent(ctx context.Context, actorID, targetUserID string, event *evtv1.Event, skipIfMissing bool) (*evtv1.AssetRecord, bool, error) {
	filter := evtstream.UserAggregate(targetUserID).AllEventsFilter()
	subject := evtstream.UserAggregate(targetUserID).SubjectFor(event)
	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("read authorization fence seq: %w", err)
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, false, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return nil, false, fmt.Errorf("wait for user projection: %w", err)
		}
		if err := c.userModel.waitForUserAuthCurrent(ctx, "avatar mutation"); err != nil {
			return nil, false, fmt.Errorf("wait for user auth projection: %w", err)
		}
		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
		if err != nil {
			return nil, false, fmt.Errorf("read RBAC projection position: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(evtstream.RBACSubjectFilter(), rbacSeq)); err != nil {
			return nil, false, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if _, err := c.requireCanManageUserAvatar(ctx, actorID, targetUserID); err != nil {
			return nil, false, err
		}
		previous, _ := c.GetUserAvatar(ctx, targetUserID)
		if skipIfMissing && previous == nil {
			return nil, false, nil
		}
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
			Subject: subject, Event: event, HasOCC: true, ExpectedSeq: filterSeq, FilterSubject: filter,
		}}, authorizationSeq)
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
				return previous, true, fmt.Errorf("wait for user projection: %w", err)
			}
			return previous, true, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, false, err
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, false, fmt.Errorf("avatar mutation OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

// UploadUserAvatar processes an image (resizes to 256x256 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// If the user already has an avatar, the old one is deleted after successful upload.
func (c *ChattoCore) UploadUserAvatar(ctx context.Context, userID string, reader io.Reader) (*evtv1.AssetRecord, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}

	// Capture old avatar reference for cleanup after successful upload
	oldAvatar, _ := c.GetUserAvatar(ctx, userID)

	asset, err := c.storeUserAvatarAsset(ctx, userID, reader)
	if err != nil {
		return nil, err
	}

	// Preserve the legacy two-step helper's replacement cleanup behavior.
	if oldAvatar != nil {
		c.deleteAsset(ctx, assetStorageFromAsset(oldAvatar), "avatar", userID)
	}

	return asset, nil
}

func (c *ChattoCore) storeUserAvatarAsset(ctx context.Context, userID string, reader io.Reader) (*evtv1.AssetRecord, error) {
	if _, err := c.GetUser(ctx, userID); err != nil {
		return nil, err
	}
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
	asset := &evtv1.AssetRecord{
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
		asset.Storage = &evtv1.AssetRecord_S3{
			S3: &evtv1.S3Asset{
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
		asset.Storage = &evtv1.AssetRecord_Nats{
			Nats: &evtv1.NATSAsset{
				Key: objectKey,
			},
		}
		c.logger.Info("Uploaded avatar", "user_id", userID, "size", info.Size)
	}

	return asset, nil
}

// SetUserAvatar stores the user's avatar asset reference through the user aggregate.
func (c *ChattoCore) SetUserAvatar(ctx context.Context, userID string, asset *evtv1.AssetRecord) error {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return err
	}

	event := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_AssetCreated{
		AssetCreated: &evtv1.AssetCreatedEvent{
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
func (c *ChattoCore) GetUserAvatar(ctx context.Context, userID string) (*evtv1.AssetRecord, error) {
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

	event := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserAvatarCleared{
		UserAvatarCleared: &evtv1.UserAvatarClearedEvent{UserId: userID},
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
	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_AssetDeleted{
			AssetDeleted: &evtv1.AssetDeletedEvent{AssetId: assetID},
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
