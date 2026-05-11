package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DefaultAutoJoinRoom describes a room created automatically by CreateSpace.
type DefaultAutoJoinRoom struct {
	Name        string
	Description string
}

// DefaultAutoJoinRooms is the list of rooms created automatically by every
// CreateSpace call. Each is created with auto_join=true so new space members
// are joined to them on space-join.
var DefaultAutoJoinRooms = []DefaultAutoJoinRoom{
	{Name: "announcements", Description: "Announcements and News"},
	{Name: "general", Description: "General discussion"},
}

// validateSpaceName validates that a space name is non-empty, has no leading/trailing whitespace,
// and does not exceed the maximum length.
func validateSpaceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("space name cannot be empty")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("space name cannot have leading or trailing whitespace")
	}
	if len(name) > MaxSpaceNameLength {
		return ErrSpaceNameTooLong
	}
	return nil
}

// ============================================================================
// Space Operations
// ============================================================================

// storeSpaceAndCreateStream marshals a space, stores it in KV, creates its event stream,
// and eagerly initializes all space-level KV buckets and object stores.
// If atomic is true, uses Create (fails if exists); otherwise uses Put (upsert).
// Returns true if the space was created, false if it already existed (only relevant when atomic=true).
func (c *ChattoCore) storeSpaceAndCreateStream(ctx context.Context, space *corev1.Space, atomic bool) (bool, error) {
	spaceData, err := proto.Marshal(space)
	if err != nil {
		return false, fmt.Errorf("failed to marshal space: %w", err)
	}

	if atomic {
		_, err = c.storage.serverKV.Create(ctx, spaceKey(space.Id), spaceData)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				return false, nil // Already exists, not an error
			}
			return false, fmt.Errorf("failed to store space: %w", err)
		}
	} else {
		_, err = c.storage.serverKV.Put(ctx, spaceKey(space.Id), spaceData)
		if err != nil {
			return false, fmt.Errorf("failed to store space: %w", err)
		}
	}

	return true, nil
}

// CreateSpace creates a new space.
// KV store is written first, then an event is published for audit trail (best-effort).
func (c *ChattoCore) CreateSpace(ctx context.Context, actorID string, name string, description string) (*corev1.Space, error) {
	// Validate and sanitize name
	if err := validateSpaceName(name); err != nil {
		return nil, err
	}

	// Validate description length
	if len(description) > MaxDescriptionLength {
		return nil, ErrDescriptionTooLong
	}

	space := &corev1.Space{
		Id:          NewSpaceID(),
		Name:        name,
		Description: description,
	}

	if _, err := c.storeSpaceAndCreateStream(ctx, space, false); err != nil {
		return nil, err
	}

	// Create default roles (owner, moderator, everyone)
	if err := c.CreateDefaultRoles(ctx, space.Id); err != nil {
		return nil, fmt.Errorf("failed to create default roles: %w", err)
	}

	// Auto-join creator to any rooms flagged auto_join. Server "membership"
	// is implicit post-consolidation; there's no separate join step.
	c.AutoJoinDefaultRooms(ctx, space.Id, actorID)

	// Assign owner role to creator (SystemActorID bypasses permission check - bootstrap mode)
	if err := c.AssignServerRole(ctx, SystemActorID, actorID, RoleOwner); err != nil {
		return nil, fmt.Errorf("failed to assign owner role to creator: %w", err)
	}

	// Create and publish audit event (best-effort)
	// SpaceCreated goes to INSTANCE stream for instance-wide visibility
	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_SpaceCreated{
			SpaceCreated: &corev1.SpaceCreatedEvent{
				SpaceId:     space.Id,
				Name:        space.Name,
				Description: space.Description,
			},
		},
	})
	subject := subjects.LiveInstanceUserEvent(actorID, "space_created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish space created event", "error", err, "space_id", space.Id)
	}

	c.logger.Info("Created space", "id", space.Id, "name", space.Name)

	return space, nil
}

// UpdateSpace updates an existing space.
// KV store is updated first, then an event is published for audit trail (best-effort).
// Authorization: Caller must verify CanAdminSpaceManage before calling.
func (c *ChattoCore) UpdateSpace(ctx context.Context, actorID string, space_id string, name string, description string) (*corev1.Space, error) {
	// Validate and sanitize name
	if err := validateSpaceName(name); err != nil {
		return nil, err
	}

	// Validate description length
	if len(description) > MaxDescriptionLength {
		return nil, ErrDescriptionTooLong
	}

	// Verify space exists
	_, err := c.GetSpace(ctx, space_id)
	if err != nil {
		return nil, fmt.Errorf("space not found: %w", err)
	}

	// Update space entity
	space := &corev1.Space{
		Id:          space_id,
		Name:        name,
		Description: description,
	}

	// Write to KV store (source of truth)
	spaceData, err := proto.Marshal(space)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal space: %w", err)
	}
	_, err = c.storage.serverKV.Put(ctx, spaceKey(space.Id), spaceData)
	if err != nil {
		return nil, fmt.Errorf("failed to update space: %w", err)
	}

	// Publish space update event (delivered to members via server-side filtering)
	c.publishSpaceUpdate(ctx, actorID, space_id, space)

	c.logger.Info("Updated space", "id", space_id, "name", name)

	return space, nil
}

// DeleteSpace deletes a space.
// KV store is updated first, then an event is published for audit trail (best-effort).
// Authorization: Caller must verify CanAdminSpaceDelete before calling.
func (c *ChattoCore) DeleteSpace(ctx context.Context, actorID string, space_id string) error {
	// Verify space exists
	_, err := c.GetSpace(ctx, space_id)
	if err != nil {
		return fmt.Errorf("space not found: %w", err)
	}

	// Delete from KV store (source of truth). All space data lives in the
	// shared SERVER_* buckets, so there's no per-space resource cleanup to
	// do here — the space record itself is the only artifact.
	if err := c.storage.serverKV.Delete(ctx, spaceKey(space_id)); err != nil {
		return fmt.Errorf("failed to delete space: %w", err)
	}

	// Create and publish audit event (best-effort)
	// SpaceDeleted goes to INSTANCE stream for instance-wide visibility
	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_SpaceDeleted{
			SpaceDeleted: &corev1.SpaceDeletedEvent{
				SpaceId: space_id,
			},
		},
	})
	subject := subjects.LiveInstanceUserEvent(actorID, "space_deleted")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish space deleted event", "error", err, "space_id", space_id)
	}

	c.logger.Info("Deleted space", "id", space_id)

	return nil
}

// GetSpace retrieves a space from the INSTANCE KV bucket.
func (c *ChattoCore) GetSpace(ctx context.Context, space_id string) (*corev1.Space, error) {
	entry, err := c.storage.serverKV.Get(ctx, spaceKey(space_id))
	if err != nil {
		return nil, fmt.Errorf("space not found: %w", err)
	}

	space := &corev1.Space{}
	if err := proto.Unmarshal(entry.Value(), space); err != nil {
		return nil, fmt.Errorf("failed to unmarshal space: %w", err)
	}

	return space, nil
}

// CountSpaces returns the number of live (non-deleted) spaces in the INSTANCE KV bucket.
//
// Why this isn't a single stream.Info call: NATS exposes server-side per-subject
// counts via stream.Info(WithSubjectFilter("$KV.INSTANCE.space.*")), but kv.Delete
// writes a 0-byte tombstone message rather than removing the subject, and with the
// default History=1 the tombstone simply replaces the live value. The subject still
// has 1 message, so len(State.Subjects) and NumSubjects both inflate by every
// historical deletion until the tombstone is purged. ListKeysFiltered is the only
// API that filters tombstones (via the KV-Operation: DEL header on each message),
// so it's the only correct option for a live-key count.
//
// This is O(N) in the number of matching subjects (live + tombstoned), but only
// metadata is sent — no values are fetched.
func (c *ChattoCore) CountSpaces(ctx context.Context) (int, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "space.*")
	if err != nil {
		return 0, nil
	}
	count := 0
	for range keyLister.Keys() {
		count++
	}
	return count, nil
}

// ListSpaces retrieves all spaces from the INSTANCE KV bucket.
func (c *ChattoCore) ListSpaces(ctx context.Context) ([]*corev1.Space, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "space.*")
	if err != nil {
		return []*corev1.Space{}, nil
	}

	var spaces []*corev1.Space
	for key := range keyLister.Keys() {
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get space %s: %w", key, err)
		}

		space := &corev1.Space{}
		if err := proto.Unmarshal(entry.Value(), space); err != nil {
			return nil, fmt.Errorf("failed to unmarshal space %s: %w", key, err)
		}

		spaces = append(spaces, space)
	}

	return spaces, nil
}

// ============================================================================
// Space Event Broadcasting
// ============================================================================

// publishSpaceUpdate publishes a SpaceUpdatedEvent to the instance stream.
// The event is published to instance.space.{spaceId}.updated and delivered
// to all space members via server-side authorization filtering in StreamMyLiveEvents.
func (c *ChattoCore) publishSpaceUpdate(ctx context.Context, actorID, spaceID string, space *corev1.Space) {
	// Fetch current logo URL to include in the event (full resolution for events)
	logoURL, err := c.GetSpaceLogoURL(ctx, spaceID, nil, nil)
	if err != nil {
		c.logger.Warn("failed to get logo URL for space update event", "error", err, "space_id", spaceID)
		logoURL = ""
	}

	// Fetch current banner URL to include in the event (full resolution for events)
	bannerURL, err := c.GetSpaceBannerURL(ctx, spaceID, nil, nil)
	if err != nil {
		c.logger.Warn("failed to get banner URL for space update event", "error", err, "space_id", spaceID)
		bannerURL = ""
	}

	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_SpaceUpdated{
			SpaceUpdated: &corev1.SpaceUpdatedEvent{
				SpaceId:     spaceID,
				Name:        space.Name,
				Description: space.Description,
				LogoUrl:     logoURL,
				BannerUrl:   bannerURL,
			},
		},
	})

	subject := subjects.LiveInstanceSpaceEvent(spaceID, "updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish space update event", "error", err, "space_id", spaceID)
	}
}

// ============================================================================
// Space-Scoped User Cleanup
// ============================================================================

// CleanupUserStateInSpace removes a user's per-space artifacts: room
// memberships, notification levels, and (during account deletion) emits a
// SpaceMemberDeletedEvent so clients can re-render messages as "Deleted User".
// Idempotent; safe to call for spaces the user never interacted with.
//
// Post-#330 there's no separate "space membership" record to delete — every
// authenticated user is implicitly a server member.
func (c *ChattoCore) CleanupUserStateInSpace(ctx context.Context, userID, spaceID string, isAccountDeletion bool) error {
	if err := c.deleteUserRoomMembershipsInSpace(ctx, userID, spaceID); err != nil {
		c.logger.Warn("Failed to delete room memberships during cleanup", "user_id", userID, "space_id", spaceID, "error", err)
	}

	if err := c.deleteUserNotificationLevels(ctx, spaceID, userID); err != nil {
		c.logger.Warn("Failed to delete notification levels during cleanup", "user_id", userID, "space_id", spaceID, "error", err)
	}

	if isAccountDeletion {
		memberDeletedEvent := newServerEvent(userID, &corev1.ServerEvent{
			Event: &corev1.ServerEvent_SpaceMemberDeleted{
				SpaceMemberDeleted: &corev1.SpaceMemberDeletedEvent{
					SpaceId: spaceID,
					UserId:  userID,
				},
			},
		})
		subject := subjects.Member("member_deleted")
		if err := c.publishServerEvent(ctx, subject, memberDeletedEvent); err != nil {
			c.logger.Warn("Failed to publish SpaceMemberDeletedEvent", "user_id", userID, "space_id", spaceID, "error", err)
		}
		liveSubject := subjects.LiveMember("member_deleted")
		if err := c.publishLiveServerEvent(ctx, liveSubject, memberDeletedEvent); err != nil {
			c.logger.Warn("Failed to publish live SpaceMemberDeletedEvent", "user_id", userID, "space_id", spaceID, "error", err)
		}
	}

	return nil
}

// AutoJoinDefaultRooms joins the user to rooms that have auto_join enabled.
// Best-effort: errors are logged but don't cause failure.
func (c *ChattoCore) AutoJoinDefaultRooms(ctx context.Context, spaceID, userID string) {
	// Get all rooms in the space
	rooms, err := c.ListRoomsBySpace(ctx, spaceID)
	if err != nil {
		c.logger.Warn("failed to list rooms for auto-join", "error", err, "space_id", spaceID)
		return
	}

	// Join rooms that have auto_join enabled
	for _, room := range rooms {
		if room.AutoJoin {
			// Use the user as the actor since they are joining (even if automatically)
			_, err := c.JoinRoom(ctx, userID, spaceID, userID, room.Id)
			if err != nil {
				c.logger.Warn("failed to auto-join user to room",
					"error", err,
					"user_id", userID,
					"space_id", spaceID,
					"room_id", room.Id,
					"room_name", room.Name)
			} else {
				c.logger.Info("Auto-joined user to room",
					"user_id", userID,
					"space_id", spaceID,
					"room_id", room.Id,
					"room_name", room.Name)
			}
		}
	}
}

// GetSpaceRoomCount returns the number of rooms in a space.
func (c *ChattoCore) GetSpaceRoomCount(ctx context.Context, spaceID string) (int, error) {
	rooms, err := c.ListRoomsBySpace(ctx, spaceID)
	if err != nil {
		return 0, err
	}
	return len(rooms), nil
}

// GetSpaceAssetCount returns the number of assets (attachments) in a space.
func (c *ChattoCore) GetSpaceAssetCount(ctx context.Context, spaceID string) (int, error) {
	store, err := c.GetAttachmentsStore(ctx, spaceID)
	if err != nil {
		// If the bucket doesn't exist, return 0
		return 0, nil
	}

	// List all objects and count them
	objects, err := store.List(ctx)
	if err != nil {
		// ErrNoObjectsFound means empty bucket, not an error
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list objects for space %s: %w", spaceID, err)
	}

	count := 0
	for range objects {
		count++
	}

	return count, nil
}

// ============================================================================
// Space Logo Operations
// ============================================================================

// UploadSpaceLogo processes an image (resizes to 512x512 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// Note: This only uploads the image; use SetSpaceLogo to atomically update the
// logo reference and clean up the old logo.
func (c *ChattoCore) UploadSpaceLogo(ctx context.Context, spaceID string, reader io.Reader) (*corev1.Asset, error) {
	// Verify space exists
	_, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("space not found: %w", err)
	}

	// Process image: resize and convert to WebP
	webpReader, err := assets.ProcessLogoImageWithConfig(reader, c.AssetsConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to process logo image: %w", err)
	}

	// Read the processed image into bytes (needed for both NATS and S3)
	webpData, err := io.ReadAll(webpReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read processed logo: %w", err)
	}

	// Upload to storage with unique asset ID
	assetID := NewAssetID()
	var asset *corev1.Asset

	if c.ShouldUseS3() {
		// Upload to S3 - use the same assetID as NATS would use for the key
		s3Key := S3KeyServerAsset(assetID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp")
		if err != nil {
			return nil, fmt.Errorf("failed to upload logo to S3: %w", err)
		}
		// Store just the assetID in Key (same as NATS) so URL generation is consistent
		asset = &corev1.Asset{
			Asset: &corev1.Asset_S3{
				S3: &corev1.S3Asset{
					Key:    assetID,
					Bucket: proto.String(c.s3Client.Bucket()),
				},
			},
		}
		c.logger.Info("Uploaded space logo to S3", "space_id", spaceID, "asset_id", assetID, "size", len(webpData))
	} else {
		// Upload to NATS ObjectStore
		headers := nats.Header{}
		headers.Set("Content-Type", "image/webp")
		meta := jetstream.ObjectMeta{
			Name:    assetID,
			Headers: headers,
		}
		info, err := c.storage.serverStore.Put(ctx, meta, bytes.NewReader(webpData))
		if err != nil {
			return nil, fmt.Errorf("failed to upload logo: %w", err)
		}
		asset = &corev1.Asset{
			Asset: &corev1.Asset_Nats{
				Nats: &corev1.NATSAsset{
					Key: assetID,
				},
			},
		}
		c.logger.Info("Uploaded space logo", "space_id", spaceID, "size", info.Size)
	}

	return asset, nil
}

// SetSpaceLogo atomically stores the space's logo asset reference using optimistic locking.
// It uses KV revision-based updates to prevent race conditions when multiple uploads occur
// concurrently. After a successful update, the old logo is deleted from the object store.
func (c *ChattoCore) SetSpaceLogo(ctx context.Context, actorID string, spaceID string, asset *corev1.Asset) error {
	const maxRetries = 5

	// Verify space exists
	space, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("space not found: %w", err)
	}

	// Marshal the new asset
	assetData, err := proto.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal logo asset: %w", err)
	}

	key := spaceLogoKey(spaceID)

	// Optimistic locking loop
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry (if any) with its revision
		var revision uint64
		var oldAsset *corev1.Asset

		entry, err := c.storage.serverKV.Get(ctx, key)
		if err == nil {
			// Key exists - get revision and unmarshal old asset for cleanup
			revision = entry.Revision()
			oldAsset = &corev1.Asset{}
			if unmarshalErr := proto.Unmarshal(entry.Value(), oldAsset); unmarshalErr != nil {
				c.logger.Warn("Failed to unmarshal old logo asset", "error", unmarshalErr)
				oldAsset = nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to get current logo: %w", err)
		}
		// If ErrKeyNotFound, revision stays 0 and oldAsset stays nil

		// Try atomic update
		var updateErr error
		if revision == 0 {
			// No existing key - use Create for atomic insert
			_, updateErr = c.storage.serverKV.Create(ctx, key, assetData)
		} else {
			// Existing key - use Update with revision check
			_, updateErr = c.storage.serverKV.Update(ctx, key, assetData, revision)
		}

		if updateErr == nil {
			// Success! Now clean up the old logo (NATS or S3)
			if oldAsset != nil {
				c.deleteAsset(ctx, oldAsset, "logo", spaceID)
			}

			// Publish space update event (delivered to members via server-side filtering)
			c.publishSpaceUpdate(ctx, actorID, spaceID, space)

			c.logger.Info("Updated space logo", "space_id", spaceID)
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(updateErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Logo update revision conflict, retrying", "space_id", spaceID, "attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to store logo: %w", updateErr)
	}

	return fmt.Errorf("failed to update logo after %d retries due to concurrent modifications", maxRetries)
}

// GetSpaceLogo retrieves a space's logo asset reference from the KV store.
// Returns (nil, nil) if the space has no logo set.
func (c *ChattoCore) GetSpaceLogo(ctx context.Context, spaceID string) (*corev1.Asset, error) {
	entry, err := c.storage.serverKV.Get(ctx, spaceLogoKey(spaceID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // No logo set is not an error
		}
		return nil, fmt.Errorf("failed to get logo: %w", err)
	}

	asset := &corev1.Asset{}
	if err := proto.Unmarshal(entry.Value(), asset); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logo asset: %w", err)
	}

	return asset, nil
}

// GetSpaceLogoURL returns the URL for a space's logo.
// If width and height are provided (non-nil), returns a URL to a resized version.
// Returns empty string if no logo is set.
func (c *ChattoCore) GetSpaceLogoURL(ctx context.Context, spaceID string, width, height *int) (string, error) {
	logo, err := c.GetSpaceLogo(ctx, spaceID)
	if err != nil {
		return "", err
	}

	// No logo set
	if logo == nil {
		return "", nil
	}

	// Get the asset ID (same format for both NATS and S3)
	var assetID string
	switch asset := logo.Asset.(type) {
	case *corev1.Asset_Nats:
		assetID = asset.Nats.Key
	case *corev1.Asset_S3:
		assetID = asset.S3.Key
	default:
		return "", fmt.Errorf("unknown asset type")
	}

	// Always use the standard instance asset URL format - storage backend is an internal detail
	if width != nil && height != nil {
		return c.GetTransformedServerAssetURL(assetID, *width, *height, "cover"), nil
	}
	return c.assetURL(fmt.Sprintf("/assets/instance/%s", assetID)), nil
}

// DeleteSpaceLogo removes a space's logo from both the KV store and object store.
// Uses optimistic locking to prevent race conditions with concurrent uploads.
func (c *ChattoCore) DeleteSpaceLogo(ctx context.Context, actorID string, spaceID string) error {
	const maxRetries = 5

	// Verify space exists
	space, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("space not found: %w", err)
	}

	key := spaceLogoKey(spaceID)

	// Optimistic locking loop
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry with revision
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				// No logo to delete
				return nil
			}
			return fmt.Errorf("failed to get current logo: %w", err)
		}

		revision := entry.Revision()

		// Unmarshal the asset for cleanup
		logo := &corev1.Asset{}
		if unmarshalErr := proto.Unmarshal(entry.Value(), logo); unmarshalErr != nil {
			c.logger.Warn("Failed to unmarshal logo asset for deletion", "error", unmarshalErr)
			// Continue with deletion anyway - the KV entry is corrupted
		}

		// Try atomic delete with revision check
		deleteErr := c.storage.serverKV.Delete(ctx, key, jetstream.LastRevision(revision))
		if deleteErr == nil {
			// Success! Now clean up the storage asset (NATS or S3)
			c.deleteAsset(ctx, logo, "logo", spaceID)

			// Publish space update event (delivered to members via server-side filtering)
			c.publishSpaceUpdate(ctx, actorID, spaceID, space)

			c.logger.Info("Deleted space logo", "space_id", spaceID)
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(deleteErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Logo delete revision conflict, retrying", "space_id", spaceID, "attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to delete logo: %w", deleteErr)
	}

	return fmt.Errorf("failed to delete logo after %d retries due to concurrent modifications", maxRetries)
}

// ============================================================================
// Space Banner Operations
// ============================================================================

// UploadSpaceBanner processes an image (resizes to 768x576 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// Note: This only uploads the image; use SetSpaceBanner to atomically update the
// banner reference and clean up the old banner.
func (c *ChattoCore) UploadSpaceBanner(ctx context.Context, spaceID string, reader io.Reader) (*corev1.Asset, error) {
	// Verify space exists
	_, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("space not found: %w", err)
	}

	// Process image: resize and convert to WebP
	webpReader, err := assets.ProcessBannerImageWithConfig(reader, c.AssetsConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to process banner image: %w", err)
	}

	// Read the processed image into bytes (needed for both NATS and S3)
	webpData, err := io.ReadAll(webpReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read processed banner: %w", err)
	}

	// Upload to storage with unique asset ID
	assetID := NewAssetID()
	var asset *corev1.Asset

	if c.ShouldUseS3() {
		// Upload to S3 - use the same assetID as NATS would use for the key
		s3Key := S3KeyServerAsset(assetID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp")
		if err != nil {
			return nil, fmt.Errorf("failed to upload banner to S3: %w", err)
		}
		// Store just the assetID in Key (same as NATS) so URL generation is consistent
		asset = &corev1.Asset{
			Asset: &corev1.Asset_S3{
				S3: &corev1.S3Asset{
					Key:    assetID,
					Bucket: proto.String(c.s3Client.Bucket()),
				},
			},
		}
		c.logger.Info("Uploaded space banner to S3", "space_id", spaceID, "asset_id", assetID, "size", len(webpData))
	} else {
		// Upload to NATS ObjectStore
		headers := nats.Header{}
		headers.Set("Content-Type", "image/webp")
		meta := jetstream.ObjectMeta{
			Name:    assetID,
			Headers: headers,
		}
		info, err := c.storage.serverStore.Put(ctx, meta, bytes.NewReader(webpData))
		if err != nil {
			return nil, fmt.Errorf("failed to upload banner: %w", err)
		}
		asset = &corev1.Asset{
			Asset: &corev1.Asset_Nats{
				Nats: &corev1.NATSAsset{
					Key: assetID,
				},
			},
		}
		c.logger.Info("Uploaded space banner", "space_id", spaceID, "size", info.Size)
	}

	return asset, nil
}

// SetSpaceBanner atomically stores the space's banner asset reference using optimistic locking.
// It uses KV revision-based updates to prevent race conditions when multiple uploads occur
// concurrently. After a successful update, the old banner is deleted from the object store.
func (c *ChattoCore) SetSpaceBanner(ctx context.Context, actorID string, spaceID string, asset *corev1.Asset) error {
	const maxRetries = 5

	// Verify space exists
	space, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("space not found: %w", err)
	}

	// Marshal the new asset
	assetData, err := proto.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal banner asset: %w", err)
	}

	key := spaceBannerKey(spaceID)

	// Optimistic locking loop
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry (if any) with its revision
		var revision uint64
		var oldAsset *corev1.Asset

		entry, err := c.storage.serverKV.Get(ctx, key)
		if err == nil {
			// Key exists - get revision and unmarshal old asset for cleanup
			revision = entry.Revision()
			oldAsset = &corev1.Asset{}
			if unmarshalErr := proto.Unmarshal(entry.Value(), oldAsset); unmarshalErr != nil {
				c.logger.Warn("Failed to unmarshal old banner asset", "error", unmarshalErr)
				oldAsset = nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to get current banner: %w", err)
		}
		// If ErrKeyNotFound, revision stays 0 and oldAsset stays nil

		// Try atomic update
		var updateErr error
		if revision == 0 {
			// No existing key - use Create for atomic insert
			_, updateErr = c.storage.serverKV.Create(ctx, key, assetData)
		} else {
			// Existing key - use Update with revision check
			_, updateErr = c.storage.serverKV.Update(ctx, key, assetData, revision)
		}

		if updateErr == nil {
			// Success! Now clean up the old banner
			if oldAsset != nil {
				c.deleteAsset(ctx, oldAsset, "banner", spaceID)
			}

			// Publish space update event (delivered to members via server-side filtering)
			c.publishSpaceUpdate(ctx, actorID, spaceID, space)

			c.logger.Info("Updated space banner", "space_id", spaceID)
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(updateErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Banner update revision conflict, retrying", "space_id", spaceID, "attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to store banner: %w", updateErr)
	}

	return fmt.Errorf("failed to update banner after %d retries due to concurrent modifications", maxRetries)
}

// GetSpaceBanner retrieves a space's banner asset reference from the KV store.
// Returns (nil, nil) if the space has no banner set.
func (c *ChattoCore) GetSpaceBanner(ctx context.Context, spaceID string) (*corev1.Asset, error) {
	entry, err := c.storage.serverKV.Get(ctx, spaceBannerKey(spaceID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // No banner set is not an error
		}
		return nil, fmt.Errorf("failed to get banner: %w", err)
	}

	asset := &corev1.Asset{}
	if err := proto.Unmarshal(entry.Value(), asset); err != nil {
		return nil, fmt.Errorf("failed to unmarshal banner asset: %w", err)
	}

	return asset, nil
}

// GetSpaceBannerURL returns the URL for a space's banner.
// If width and height are provided (non-nil), returns a URL to a resized version.
// Returns empty string if no banner is set.
func (c *ChattoCore) GetSpaceBannerURL(ctx context.Context, spaceID string, width, height *int) (string, error) {
	banner, err := c.GetSpaceBanner(ctx, spaceID)
	if err != nil {
		return "", err
	}

	// No banner set
	if banner == nil {
		return "", nil
	}

	// Get the asset ID (same format for both NATS and S3)
	var assetID string
	switch asset := banner.Asset.(type) {
	case *corev1.Asset_Nats:
		assetID = asset.Nats.Key
	case *corev1.Asset_S3:
		assetID = asset.S3.Key
	default:
		return "", fmt.Errorf("unknown asset type")
	}

	// Always use the standard instance asset URL format - storage backend is an internal detail
	if width != nil && height != nil {
		return c.GetTransformedServerAssetURL(assetID, *width, *height, "cover"), nil
	}
	return c.assetURL(fmt.Sprintf("/assets/instance/%s", assetID)), nil
}

// DeleteSpaceBanner removes a space's banner from both the KV store and object store.
// Uses optimistic locking to prevent race conditions with concurrent uploads.
func (c *ChattoCore) DeleteSpaceBanner(ctx context.Context, actorID string, spaceID string) error {
	const maxRetries = 5

	// Verify space exists
	space, err := c.GetSpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("space not found: %w", err)
	}

	key := spaceBannerKey(spaceID)

	// Optimistic locking loop
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry with revision
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				// No banner to delete
				return nil
			}
			return fmt.Errorf("failed to get current banner: %w", err)
		}

		revision := entry.Revision()

		// Unmarshal the asset for cleanup
		banner := &corev1.Asset{}
		if unmarshalErr := proto.Unmarshal(entry.Value(), banner); unmarshalErr != nil {
			c.logger.Warn("Failed to unmarshal banner asset for deletion", "error", unmarshalErr)
			// Continue with deletion anyway - the KV entry is corrupted
		}

		// Try atomic delete with revision check
		deleteErr := c.storage.serverKV.Delete(ctx, key, jetstream.LastRevision(revision))
		if deleteErr == nil {
			// Success! Now clean up the storage asset (NATS or S3)
			c.deleteAsset(ctx, banner, "banner", spaceID)

			// Publish space update event (delivered to members via server-side filtering)
			c.publishSpaceUpdate(ctx, actorID, spaceID, space)

			c.logger.Info("Deleted space banner", "space_id", spaceID)
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(deleteErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Banner delete revision conflict, retrying", "space_id", spaceID, "attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to delete banner: %w", deleteErr)
	}

	return fmt.Errorf("failed to delete banner after %d retries due to concurrent modifications", maxRetries)
}

// ============================================================================
// Space Member Listing (for management UI)
// ============================================================================

// SpaceMemberWithRoles represents a space member with their assigned roles.
type SpaceMemberWithRoles struct {
	UserID string
	Roles  []string
}

// GetSpaceMembers retrieves space members with optional search and pagination.
// Search matches against login and displayName (case-insensitive partial match).
// Returns members, total count (matching search), and error.
//
// Post-#330 every authenticated user is implicitly a server member, so this
// iterates the full user list rather than the (retired) space-membership
// records. The `spaceID` parameter is retained for the API shape but is no
// longer load-bearing.
func (c *ChattoCore) GetSpaceMembers(ctx context.Context, spaceID string, search string, limit, offset int) ([]SpaceMemberWithRoles, int, error) {
	type memberWithUser struct {
		member SpaceMemberWithRoles
		user   *corev1.User
	}

	allUsers, err := c.ListUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	userIDs := make([]string, 0, len(allUsers))
	for _, u := range allUsers {
		userIDs = append(userIDs, u.Id)
	}

	if len(userIDs) == 0 {
		return []SpaceMemberWithRoles{}, 0, nil
	}

	// Normalize search term for case-insensitive matching
	searchLower := strings.ToLower(strings.TrimSpace(search))

	// Filter and build results
	var matches []memberWithUser
	for _, userID := range userIDs {
		// Get user data
		user, err := c.GetUser(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user for space member listing", "user_id", userID, "error", err)
			continue // Skip users we can't fetch
		}

		// Apply search filter if provided
		if searchLower != "" {
			loginMatch := strings.Contains(strings.ToLower(user.Login), searchLower)
			displayNameMatch := strings.Contains(strings.ToLower(user.DisplayName), searchLower)
			if !loginMatch && !displayNameMatch {
				continue // Doesn't match search
			}
		}

		// Get user's roles (caller is iterating space members so virtual
		// "everyone" applies — prepend it explicitly).
		assigned, err := c.GetUserRoles(ctx, userID)
		if err != nil {
			c.logger.Warn("Failed to get user roles for space member listing", "user_id", userID, "error", err)
			assigned = nil
		}
		roles := append([]string{RoleEveryone}, assigned...)

		matches = append(matches, memberWithUser{
			member: SpaceMemberWithRoles{
				UserID: userID,
				Roles:  roles,
			},
			user: user,
		})
	}

	// Sort by created_at (oldest first), with null values sorted to end by login
	sort.Slice(matches, func(i, j int) bool {
		// Both null: sort alphabetically by login
		if matches[i].user.CreatedAt == nil && matches[j].user.CreatedAt == nil {
			return strings.ToLower(matches[i].user.Login) < strings.ToLower(matches[j].user.Login)
		}
		// Null timestamps sort to the end
		if matches[i].user.CreatedAt == nil {
			return false
		}
		if matches[j].user.CreatedAt == nil {
			return true
		}
		// Both have timestamps: sort by time (oldest first)
		return matches[i].user.CreatedAt.AsTime().Before(matches[j].user.CreatedAt.AsTime())
	})

	// Get total count before pagination
	totalCount := len(matches)

	// Apply pagination
	if offset >= len(matches) {
		return []SpaceMemberWithRoles{}, totalCount, nil
	}
	matches = matches[offset:]
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	// Extract SpaceMemberWithRoles from sorted results
	result := make([]SpaceMemberWithRoles, len(matches))
	for i, m := range matches {
		result[i] = m.member
	}

	return result, totalCount, nil
}
