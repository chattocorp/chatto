package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// User Operations
// ============================================================================

// CreateUser creates a new user.
// Uses atomic login claim via kv.Create to prevent race conditions.
// Password is optional - pass empty string for OAuth-only users.
// Note: actorID parameter is retained for future use (e.g., admin-created users) but is not currently used.
func (c *ChattoCore) CreateUser(ctx context.Context, actorID string, login, displayName, password string) (*corev1.User, error) {
	// Trim and validate login (preserve original casing)
	login = strings.TrimSpace(login)
	if err := ValidateLogin(login); err != nil {
		return nil, err
	}

	// Normalize and validate display name
	displayName = NormalizeDisplayName(displayName)
	if utf8.RuneCountInString(displayName) > MaxDisplayNameLength {
		return nil, ErrDisplayNameTooLong
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return nil, err
	}

	// Validate password strength if password is provided
	if password != "" {
		if err := ValidatePassword(password); err != nil {
			return nil, err
		}
	}

	// Check if login is blocked (defense in depth - HTTP layer should check first)
	isBlocked, err := c.configManager.IsUsernameBlocked(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to check blocked usernames: %w", err)
	}
	if isBlocked {
		return nil, ErrUsernameBlocked
	}

	// Enforce server-wide user limit at signup as a UX gate so people don't sign up
	// only to be blocked at verification. The verification check (in addVerifiedEmail)
	// remains the race-safe hard gate.
	if max := c.config.Limits.MaxUsersOrDefault(); max >= 0 {
		count, err := c.CountVerifiedUsers(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to count verified users: %w", err)
		}
		if count >= max {
			return nil, ErrLimitExceeded
		}
	}

	// Generate user ID upfront
	userID := NewUserID()

	now := timestamppb.Now()
	user := &corev1.User{
		Id:          userID,
		Login:       login,
		DisplayName: displayName,
		CreatedAt:   now,
	}

	// Create encryption key for this user. Keys are always created so they
	// exist if encryption is enabled later.
	keyRef, err := c.encryption.keyWrapper.CreateKey(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryption key: %w", err)
	}
	cleanupEncryptionKey := true
	var cleanupContentKeyRefs []string
	defer func() {
		if cleanupEncryptionKey {
			for _, contentKeyRef := range cleanupContentKeyRefs {
				if err := c.encryption.contentKeys.Shred(context.WithoutCancel(ctx), contentKeyRef); err != nil {
					c.logger.Warn("failed to clean up user content key after failed signup", "error", err, "content_key_ref", contentKeyRef)
				}
			}
			c.cleanupCreatedUserEncryptionKey(ctx, keyRef)
		}
	}()

	_, wrappedMessageDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedMessageDEK.GetContentKeyRef())

	piiDEKBytes, wrappedPIIDEK, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		return nil, err
	}
	cleanupContentKeyRefs = append(cleanupContentKeyRefs, wrappedPIIDEK.GetContentKeyRef())

	piiDEK := &userDEK{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: piiDEKBytes}
	agg := events.UserAggregate(userID)
	messageDEKEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
		UserDekGenerated: wrappedMessageDEK,
	}})
	messageDEKEvent.CreatedAt = now
	piiDEKEvent := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
		UserDekGenerated: wrappedPIIDEK,
	}})
	piiDEKEvent.CreatedAt = now
	accountCreated := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: userID},
	}})
	accountCreated.CreatedAt = now
	account := accountCreated.GetUserAccountCreated()
	account.EncryptedLogin, err = encryptUserPIIStringWithDEK(piiDEK, accountCreated.GetId(), userID, events.EventUserAccountCreated, "login", login)
	if err != nil {
		return nil, fmt.Errorf("encrypt login: %w", err)
	}
	account.EncryptedDisplayName, err = encryptUserPIIStringWithDEK(piiDEK, accountCreated.GetId(), userID, events.EventUserAccountCreated, "display_name", displayName)
	if err != nil {
		return nil, fmt.Errorf("encrypt display name: %w", err)
	}

	entries := []events.BatchEntry{{
		Subject: agg.Subject(events.EventUserDEKGenerated),
		Event:   messageDEKEvent,
	}, {
		Subject: agg.Subject(events.EventUserDEKGenerated),
		Event:   piiDEKEvent,
	}, {
		Subject: agg.Subject(events.EventUserAccountCreated),
		Event:   accountCreated,
	}}
	if password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		passwordChanged := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
				UserId:       userID,
				PasswordHash: hashedPassword,
			},
		}})
		passwordChanged.CreatedAt = now
		entries = append(entries, events.BatchEntry{
			Subject: agg.Subject(events.EventUserPasswordHashChanged),
			Event:   passwordChanged,
		})
	}

	_, err = c.appendUserBatch(ctx, userID, entries, events.UserSubjectFilter(), func() error {
		if c.Users.LoginExists(login) {
			return ErrLoginAlreadyTaken
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	cleanupEncryptionKey = false
	if err := c.waitForUserContentKeysCurrent(ctx, userID); err != nil {
		return nil, err
	}

	// Create and publish audit event (best-effort)
	// UserCreated goes to INSTANCE stream
	// The actor is the newly created user (not the caller/system)
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserCreated{
			UserCreated: &corev1.UserCreatedEvent{
				UserId:      userID,
				Login:       login,
				DisplayName: displayName,
			},
		},
	})
	subject := subjects.LiveSyncUserEvent(userID, "created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish user created event", "error", err, "user_id", userID)
	}

	c.logger.Info("Created user", "id", userID, "login", login)

	return user, nil
}

func (c *ChattoCore) cleanupCreatedUserEncryptionKey(ctx context.Context, keyRef string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := c.deleteEncryptionKeyOnly(cleanupCtx, keyRef); err != nil {
		c.logger.Warn("failed to clean up user encryption key after failed signup", "error", err, "key_ref", keyRef)
	}
}

// CreateVerifiedUser creates a user and registers an already-verified email for them
// in a single best-effort transaction. If verification fails after the user record is
// written, the user record is rolled back so signup paths don't produce orphan accounts.
//
// Used by signup-completion (post email-link click) and OIDC callbacks, where the email
// has already been proven (via clicking the link or via an OIDC `email_verified` claim).
func (c *ChattoCore) CreateVerifiedUser(ctx context.Context, actorID, login, displayName, password, email string) (*corev1.User, error) {
	user, err := c.CreateUser(ctx, actorID, login, displayName, password)
	if err != nil {
		return nil, err
	}

	if err := c.AddVerifiedEmailDirect(ctx, user.Id, email); err != nil {
		c.rollbackUserCreation(ctx, user)
		return nil, fmt.Errorf("failed to verify email for new user: %w", err)
	}

	return user, nil
}

// rollbackUserCreation undoes the persisted writes performed by CreateUser. Best-effort —
// failures are logged but not returned, since the caller is already in an error path.
func (c *ChattoCore) rollbackUserCreation(ctx context.Context, user *corev1.User) {
	c.logger.Warn("rolling back user creation", "user_id", user.Id, "login", user.Login)
	_ = c.DeleteUser(ctx, "system:rollback", user.Id)
}

// GetUser retrieves a user from the user projection.
func (c *ChattoCore) GetUser(ctx context.Context, userID string) (*corev1.User, error) {
	if user, ok := c.Users.Get(userID); ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// GetUsers retrieves multiple users by ID from the user projection.
// Returns users in the same order as userIDs. nil entries indicate not-found users.
// More efficient than calling GetUser() in a loop for batched operations.
func (c *ChattoCore) GetUsers(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	if len(userIDs) == 0 {
		return []*corev1.User{}, nil
	}

	// Deduplicate IDs to avoid redundant fetches
	seen := make(map[string]bool, len(userIDs))
	uniqueIDs := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	userMap := make(map[string]*corev1.User, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if user, ok := c.Users.Get(id); ok {
			userMap[id] = user
		}
	}

	// Return in original order (nil for not-found users)
	result := make([]*corev1.User, len(userIDs))
	for i, id := range userIDs {
		result[i] = userMap[id] // nil if not found
	}

	return result, nil
}

// GetUserByLogin retrieves a user by their login name using the login index.
func (c *ChattoCore) GetUserByLogin(ctx context.Context, login string) (*corev1.User, error) {
	if user, ok := c.Users.GetByLogin(login); ok {
		return user, nil
	}
	return nil, ErrNotFound
}

// SetPasswordHash hashes and stores a password for a user.
// Password hashes are stored separately from user profile data in the user event stream.
func (c *ChattoCore) SetPasswordHash(ctx context.Context, userID string, password string) error {
	// Validate password strength
	if err := ValidatePassword(password); err != nil {
		return err
	}

	// Verify user exists
	_, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{
		UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
			UserId:       userID,
			PasswordHash: hashedPassword,
		},
	}})
	if _, err := c.RevokeCookieSessionsForUser(ctx, userID); err != nil {
		return err
	}
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return err
	}
	if err := c.PublishSessionTerminated(ctx, userID, "password_changed"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", userID, "reason", "password_changed", "error", err)
	}
	return nil
}

// VerifyPassword verifies a user's password by login name or email and returns the user if valid.
func (c *ChattoCore) VerifyPassword(ctx context.Context, identifier string, password string) (*corev1.User, error) {
	// Timing attack protection: Always run bcrypt comparison even for non-existent users.
	// Without this, attackers could enumerate valid logins by measuring response times:
	// - Non-existent login: fast return (~1μs)
	// - Real login, wrong password: slow bcrypt check (~100ms)
	// By always running bcrypt, both paths take the same time, preventing user enumeration.
	dummyHash := []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

	// First try to find user by login/username
	user, err := c.GetUserByLogin(ctx, identifier)
	if err != nil {
		// If not found and identifier looks like an email, try email lookup
		if strings.Contains(identifier, "@") {
			user, err = c.GetUserByVerifiedEmail(ctx, identifier)
		}
	}

	if err != nil || user == nil {
		// User doesn't exist - run dummy bcrypt to match timing
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, fmt.Errorf("invalid credentials")
	}

	return c.verifyUserPassword(ctx, user, password, dummyHash)
}

// verifyUserPassword is an internal helper that verifies a password for an already-fetched user.
func (c *ChattoCore) verifyUserPassword(ctx context.Context, user *corev1.User, password string, dummyHash []byte) (*corev1.User, error) {

	// Retrieve password hash from separate KV storage
	passwordHash, ok := c.Users.PasswordHash(user.Id)
	if !ok {
		// No password set (OAuth-only user) - run dummy bcrypt to match timing
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, fmt.Errorf("password not set for this user")
	}

	err := bcrypt.CompareHashAndPassword(passwordHash, []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

// UploadUserAvatar processes an image (resizes to 256x256 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// If the user already has an avatar, the old one is deleted after successful upload.
func (c *ChattoCore) UploadUserAvatar(ctx context.Context, userID string, reader io.Reader) (*corev1.AssetRecord, error) {
	// Verify user exists
	_, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
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
		meta := jetstream.ObjectMeta{
			Name:    assetID,
			Headers: headers,
		}
		info, err := c.storage.serverStore.Put(ctx, meta, bytes.NewReader(webpData))
		if err != nil {
			return nil, fmt.Errorf("failed to upload avatar: %w", err)
		}
		asset.Storage = &corev1.AssetRecord_Nats{
			Nats: &corev1.NATSAsset{
				Key: assetID,
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
	// Verify user exists
	_, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
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
	if asset, ok := c.Users.Avatar(userID); ok {
		return asset, nil
	}
	return nil, nil
}

// DeleteUserAvatar removes a user's avatar from storage (NATS or S3).
// Returns nil if the user has no avatar set.
func (c *ChattoCore) DeleteUserAvatar(ctx context.Context, userID string) error {
	// Verify user exists
	_, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
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

// publishUserProfileUpdate publishes a UserProfileUpdatedEvent to the server stream.
// This allows other users to see profile changes (avatar, display name) in real-time.
func (c *ChattoCore) publishUserProfileUpdate(ctx context.Context, userID string) {
	// Get current user data
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		c.logger.Warn("failed to get user for profile update event", "error", err, "user_id", userID)
		return
	}

	// Get current avatar URL (full resolution for events)
	avatarURL, err := c.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		c.logger.Warn("failed to get avatar URL for profile update event", "error", err, "user_id", userID)
		avatarURL = ""
	}

	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserProfileUpdated{
			UserProfileUpdated: &corev1.UserProfileUpdatedEvent{
				UserId:      userID,
				DisplayName: user.DisplayName,
				AvatarUrl:   avatarURL,
				Login:       user.Login,
			},
		},
	})

	// Publish to live.sync.user.{userId}.profile_updated for real-time delivery.
	// Profile updates are transient (no need for JetStream storage/replay)
	subject := subjects.LiveSyncUserEvent(userID, "profile_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish user profile update event", "error", err, "user_id", userID)
	}
}

// ListUsers retrieves all users from the user projection.
// CountUsers returns the total number of users on the server.
func (c *ChattoCore) CountUsers(ctx context.Context) (int, error) {
	return c.Users.Count(), nil
}

func (c *ChattoCore) ListUsers(ctx context.Context) ([]*corev1.User, error) {
	return c.Users.Users(), nil
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

	// Get the asset ID (same format for both NATS and S3)
	var assetID string
	switch {
	case avatar.GetNats() != nil:
		assetID = avatar.GetNats().GetKey()
	case avatar.GetS3() != nil:
		assetID = avatar.GetS3().GetKey()
	default:
		return "", fmt.Errorf("unknown asset type")
	}

	// Always use the standard server asset URL format - storage backend is an internal detail
	if width != nil && height != nil {
		if fit == "" {
			fit = "cover"
		}
		return c.GetTransformedServerAssetURL(assetID, *width, *height, fit), nil
	}
	return c.assetURL(fmt.Sprintf("/assets/server/%s", assetID)), nil
}

// ============================================================================
// Login Validation
// ============================================================================

// ErrLoginAlreadyTaken is returned when the login name is already taken.
var ErrLoginAlreadyTaken = fmt.Errorf("login name is already taken")

// ErrUsernameBlocked is returned when the login name is in the blocked list.
var ErrUsernameBlocked = fmt.Errorf("this username is not available")

// CheckLoginExists checks if a login name is already taken.
func (c *ChattoCore) CheckLoginExists(ctx context.Context, login string) (bool, error) {
	return c.Users.LoginExists(login), nil
}

// UpdateUserDisplayName updates a user's display name.
// Authorization: Caller should verify the actor is the user being updated.
func (c *ChattoCore) UpdateUserDisplayName(ctx context.Context, userID, displayName string) (*corev1.User, error) {
	// Normalize and validate display name
	displayName = NormalizeDisplayName(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("display name cannot be empty")
	}
	if utf8.RuneCountInString(displayName) > MaxDisplayNameLength {
		return nil, ErrDisplayNameTooLong
	}
	if err := ValidateDisplayName(displayName); err != nil {
		return nil, err
	}

	// Get current user
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserDisplayNameChanged{
		UserDisplayNameChanged: &corev1.UserDisplayNameChangedEvent{
			UserId: userID,
		},
	}})
	encryptedDisplayName, err := c.encryptUserPIIString(ctx, event.GetId(), userID, events.EventUserDisplayNameChanged, "display_name", displayName)
	if err != nil {
		return nil, fmt.Errorf("encrypt display name: %w", err)
	}
	event.GetUserDisplayNameChanged().EncryptedDisplayName = encryptedDisplayName
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return nil, fmt.Errorf("failed to store user: %w", err)
	}
	user.DisplayName = displayName

	c.logger.Info("Updated user display name", "id", userID, "displayName", displayName)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return user, nil
}

// AdminUpdateUserDisplayName updates a user's display name as an admin action.
// Behavior matches UpdateUserDisplayName; this exists as a distinct entry point
// for audit clarity in logs.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) AdminUpdateUserDisplayName(ctx context.Context, userID, displayName string) (*corev1.User, error) {
	user, err := c.UpdateUserDisplayName(ctx, userID, displayName)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Admin updated user display name", "id", userID, "display_name", displayName)
	return user, nil
}

// ============================================================================
// Login Change Operations
// ============================================================================

// userLoginChangedAtKey returns the KV key for tracking when a user last changed their login.
func userLoginChangedAtKey(userID string) string {
	return "user_login_changed_at." + userID
}

// UpdateUserLogin changes a user's login/username with 30-day cooldown enforcement.
// Authorization: Caller should verify the actor is the user being updated.
func (c *ChattoCore) UpdateUserLogin(ctx context.Context, userID, newLogin string) (*corev1.User, error) {
	return c.applyLoginChange(ctx, userID, newLogin, true)
}

// AdminUpdateUserLogin changes a user's login/username, bypassing the cooldown
// check and not advancing the cooldown timestamp. The user retains whatever
// rename allowance they had prior to the admin edit.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) AdminUpdateUserLogin(ctx context.Context, userID, newLogin string) (*corev1.User, error) {
	user, err := c.applyLoginChange(ctx, userID, newLogin, false)
	if err != nil {
		return nil, err
	}
	c.logger.Info("Admin updated user login", "id", userID, "new_login", newLogin)
	return user, nil
}

// applyLoginChange performs the actual login change. When enforceCooldown is
// true, the 30-day cooldown is checked before changing and a new timestamp is
// recorded after a successful change.
func (c *ChattoCore) applyLoginChange(ctx context.Context, userID, newLogin string, enforceCooldown bool) (*corev1.User, error) {
	// Trim and validate (preserve original casing)
	newLogin = strings.TrimSpace(newLogin)
	if err := ValidateLogin(newLogin); err != nil {
		return nil, err
	}

	// Get current user
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Check if unchanged (exact match — case-only changes are allowed)
	if user.Login == newLogin {
		return user, nil // No-op, return current user
	}

	caseOnly := strings.EqualFold(user.Login, newLogin)
	if !caseOnly {
		isBlocked, err := c.configManager.IsUsernameBlocked(ctx, newLogin)
		if err != nil {
			return nil, fmt.Errorf("failed to check blocked usernames: %w", err)
		}
		if isBlocked {
			return nil, ErrUsernameBlocked
		}
	}

	// Check cooldown (skipped on admin path)
	if enforceCooldown && !caseOnly {
		lastChange, err := c.GetLastLoginChange(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check login change cooldown: %w", err)
		}
		if !lastChange.IsZero() && time.Since(lastChange) < LoginChangeCooldown {
			return nil, ErrLoginChangeCooldown
		}
	}

	loginChanged := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserLoginChanged{
		UserLoginChanged: &corev1.UserLoginChangedEvent{
			UserId: userID,
		},
	}})
	encryptedLogin, err := c.encryptUserPIIString(ctx, loginChanged.GetId(), userID, events.EventUserLoginChanged, "login", newLogin)
	if err != nil {
		return nil, fmt.Errorf("encrypt login: %w", err)
	}
	loginChanged.GetUserLoginChanged().EncryptedLogin = encryptedLogin
	agg := events.UserAggregate(userID)
	entries := []events.BatchEntry{{
		Subject: agg.SubjectFor(loginChanged),
		Event:   loginChanged,
	}}
	if enforceCooldown && !caseOnly {
		cooldownStarted := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{
			UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{UserId: userID},
		}})
		cooldownStarted.CreatedAt = loginChanged.GetCreatedAt()
		entries = append(entries, events.BatchEntry{
			Subject: agg.SubjectFor(cooldownStarted),
			Event:   cooldownStarted,
		})
	}
	if _, err := c.appendUserBatch(ctx, userID, entries, events.UserSubjectFilter(), func() error {
		if !caseOnly && c.Users.LoginExists(newLogin) {
			return ErrLoginAlreadyTaken
		}
		return nil
	}); err != nil {
		if errors.Is(err, ErrLoginAlreadyTaken) {
			return nil, ErrLoginAlreadyTaken
		}
		return nil, fmt.Errorf("failed to store user: %w", err)
	}
	user.Login = newLogin

	c.logger.Info("Updated user login", "id", userID, "new_login", newLogin)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return user, nil
}

// GetLastLoginChange returns when the user last changed their login.
// Returns zero time if the user has never changed their login.
func (c *ChattoCore) GetLastLoginChange(ctx context.Context, userID string) (time.Time, error) {
	return c.Users.LoginChangedAt(userID), nil
}

// ClearLoginChangeCooldown removes the cooldown timestamp for a user, allowing
// them to immediately change their login again. Idempotent — clearing an
// already-clear cooldown is a no-op.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) ClearLoginChangeCooldown(ctx context.Context, userID string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserLoginCooldownCleared{
		UserLoginCooldownCleared: &corev1.UserLoginCooldownClearedEvent{UserId: userID},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", nil); err != nil {
		return fmt.Errorf("failed to clear login change cooldown: %w", err)
	}
	c.logger.Info("Cleared user login change cooldown", "id", userID)
	c.publishUserProfileUpdate(ctx, userID)
	return nil
}

// ============================================================================
// Account Deletion Token Operations
// ============================================================================

const accountDeletionTokenKeyPrefix = "account_deletion_token."

// accountDeletionTokenKey returns the HMAC-derived KV key for an account deletion token.
func (c *ChattoCore) accountDeletionTokenKey(token string) string {
	return c.runtimeTokenKey(accountDeletionTokenKeyPrefix, token)
}

// AccountDeletionTokenTTL is how long an account deletion token is valid.
const AccountDeletionTokenTTL = 15 * time.Minute

// AccountDeletionToken represents a token used to confirm account deletion.
type AccountDeletionToken struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAccountDeletionToken generates a confirmation token for account deletion.
// The token is stored in RUNTIME_STATE and must be provided to DeleteUser within the TTL.
func (c *ChattoCore) CreateAccountDeletionToken(ctx context.Context, userID string) (string, error) {
	token := NewAccountDeletionToken()
	createdAt := time.Now()

	tokenData := AccountDeletionToken{
		UserID:    userID,
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, c.accountDeletionTokenKey(token), data, jetstream.KeyTTL(AccountDeletionTokenTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store account deletion token: %w", err)
	}

	if err := c.recordAccountDeletionConfirmationIssued(ctx, userID, createdAt); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.accountDeletionTokenKey(token))
		return "", err
	}

	c.logger.Debug("Created account deletion token", "user_id", userID)
	return token, nil
}

// ValidateAccountDeletionToken validates a token and ensures it belongs to the user.
// If valid, the token is consumed (deleted) to prevent reuse.
// Returns an error if the token is invalid, expired, or doesn't belong to the user.
func (c *ChattoCore) ValidateAccountDeletionToken(ctx context.Context, token, userID string) error {
	key := c.accountDeletionTokenKey(token)

	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to get account deletion token: %w", err)
	}

	var tokenData AccountDeletionToken
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Check if token has expired
	if time.Since(tokenData.CreatedAt) > AccountDeletionTokenTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key) // Clean up expired token
		return ErrTokenExpired
	}

	// Check if token belongs to the user
	if tokenData.UserID != userID {
		return ErrPermissionDenied
	}

	// Consume the token (delete it)
	if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil {
		c.logger.Warn("Failed to delete consumed account deletion token", "error", err)
		// Continue anyway - the token was valid
	}

	return nil
}

// DeleteUser permanently deletes a user account and all associated data.
// This performs GDPR-compliant deletion including removal of message bodies.
// Authorization: Caller must verify CanDeleteUser(actorID, userID) before calling.
func (c *ChattoCore) DeleteUser(ctx context.Context, actorID, userID string) error {
	// Get the user first to get their login for index cleanup
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Post-ADR-030 there are two implicit scopes — channel and DM — and
	// cleanup iterates each kind.
	allKinds := []RoomKind{KindChannel, KindDM}

	// Delete all message bodies authored by this user
	for _, kind := range allKinds {
		deleted, err := c.deleteUserMessageBodiesInSpace(ctx, userID, kind)
		if err != nil {
			c.logger.Warn("Failed to delete message bodies", "user_id", userID, "kind", kind, "error", err)
		} else if deleted > 0 {
			c.logger.Info("Deleted message bodies during user deletion", "user_id", userID, "kind", kind, "count", deleted)
		}
	}

	// Delete encryption key (crypto-shreds any remaining encrypted data) and
	// record the durable shred signal projections use to tombstone messages
	// before decrypting.
	if err := c.DeleteUserEncryptionKeyAs(ctx, actorID, userID); err != nil {
		c.logger.Warn("Failed to delete encryption key", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}

	if deleted := c.DeleteMessageOwnedAssetsForUser(ctx, actorID, userID); deleted > 0 {
		c.logger.Info("Deleted message-owned assets during user deletion", "user_id", userID, "count", deleted)
	}

	// Delete push notification subscriptions
	if _, err := c.DeleteAllUserPushSubscriptions(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete push subscriptions", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}
	if _, err := c.RevokeCookieSessionsForUser(ctx, userID); err != nil {
		c.logger.Warn("Failed to revoke cookie sessions during deletion", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}

	// Delete avatar from object store if it exists
	avatar, _ := c.GetUserAvatar(ctx, userID)
	if avatar != nil {
		if err := c.RecordUserAssetDeleted(ctx, actorID, userID, avatar.GetId()); err != nil {
			c.logger.Warn("Failed to publish avatar asset deletion event", "user_id", userID, "asset_id", avatar.GetId(), "error", err)
		}
		c.deleteAsset(ctx, assetStorageFromAsset(avatar), "avatar", userID)
	}

	deletedEvent := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserAccountDeleted{
		UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: userID},
	}})
	if _, err := c.appendUserEvent(ctx, userID, deletedEvent, "", nil); err != nil {
		return fmt.Errorf("failed to mark user deleted: %w", err)
	}
	if err := c.deleteUserSettings(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete user settings during deletion", "user_id", userID, "error", err)
	}

	// Clean per-kind user artifacts AFTER the user projection marks the
	// account deleted, so SpaceMemberDeletedEvent refetches already see
	// "Deleted User".
	for _, kind := range allKinds {
		if err := c.CleanupUserState(ctx, userID, kind, true); err != nil {
			c.logger.Warn("Failed to clean up user state during deletion", "user_id", userID, "kind", kind, "error", err)
		}
	}

	// Revoke all role assignments (server-wide, no per-space loop needed).
	if err := c.RevokeAllUserRoles(ctx, userID); err != nil {
		c.logger.Warn("Failed to revoke user roles during deletion", "user_id", userID, "error", err)
	}

	// Publish server-level UserDeletedEvent for audit logging and admin UI updates
	serverEvent := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserDeleted{
			UserDeleted: &corev1.UserDeletedEvent{
				UserId: userID,
			},
		},
	})
	serverSubject := subjects.LiveSyncUserEvent(userID, "user_deleted")
	if err := c.publishLiveEvent(ctx, serverSubject, serverEvent); err != nil {
		c.logger.Warn("Failed to publish UserDeletedEvent", "user_id", userID, "error", err)
	}
	if err := c.PublishSessionTerminated(ctx, userID, "account_deleted"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", userID, "error", err)
	}

	c.logger.Info("Deleted user account", "id", userID, "login", user.Login)

	return nil
}
