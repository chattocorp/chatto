package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core/subjects"
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

	// Enforce instance-wide user limit at signup as a UX gate so people don't sign up
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

	// Atomically claim login name (prevents race conditions)
	loginKey := userByLoginKey(login)
	_, err = c.storage.serverKV.Create(ctx, loginKey, []byte(userID))
	if err != nil {
		// Login already exists or other error
		return nil, ErrLoginAlreadyTaken
	}

	// Create user entity (without password hash)
	user := &corev1.User{
		Id:          userID,
		Login:       login,
		DisplayName: displayName,
		CreatedAt:   timestamppb.Now(),
	}

	// Write user to KV store (source of truth)
	userData, err := proto.Marshal(user)
	if err != nil {
		// Cleanup: remove login claim
		c.storage.serverKV.Delete(ctx, loginKey)
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	_, err = c.storage.serverKV.Put(ctx, userKey(user.Id), userData)
	if err != nil {
		// Cleanup: remove login claim
		c.storage.serverKV.Delete(ctx, loginKey)
		return nil, fmt.Errorf("failed to store user: %w", err)
	}

	// Store password hash separately if password is provided
	if password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			// Cleanup: remove user and login claim
			c.storage.serverKV.Delete(ctx, userKey(user.Id))
			c.storage.serverKV.Delete(ctx, loginKey)
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}

		_, err = c.storage.serverKV.Put(ctx, userAuthPasswordKey(user.Id), hashedPassword)
		if err != nil {
			// Cleanup: remove user and login claim
			c.storage.serverKV.Delete(ctx, userKey(user.Id))
			c.storage.serverKV.Delete(ctx, loginKey)
			return nil, fmt.Errorf("failed to store password: %w", err)
		}
	}

	// Create encryption key for this user
	// Keys are always created so they exist if encryption is enabled later
	_, err = c.encryption.keyManager.CreateUserKey(ctx, userID)
	if err != nil {
		// Cleanup: remove user, login claim, and password
		c.storage.serverKV.Delete(ctx, userKey(user.Id))
		c.storage.serverKV.Delete(ctx, loginKey)
		c.storage.serverKV.Delete(ctx, userAuthPasswordKey(user.Id))
		return nil, fmt.Errorf("failed to create encryption key: %w", err)
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
	subject := subjects.LiveUserEvent(userID, "created")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish user created event", "error", err, "user_id", userID)
	}

	c.logger.Info("Created user", "id", userID, "login", login)

	return user, nil
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

// rollbackUserCreation undoes the KV writes performed by CreateUser. Best-effort —
// failures are logged but not returned, since the caller is already in an error path.
func (c *ChattoCore) rollbackUserCreation(ctx context.Context, user *corev1.User) {
	c.logger.Warn("rolling back user creation", "user_id", user.Id, "login", user.Login)

	keys := []string{
		userKey(user.Id),
		userByLoginKey(user.Login),
		userAuthPasswordKey(user.Id),
	}
	for _, key := range keys {
		if err := c.storage.serverKV.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			c.logger.Warn("rollback delete failed", "key", key, "error", err)
		}
	}
	if err := c.DeleteUserEncryptionKey(ctx, user.Id); err != nil {
		c.logger.Warn("rollback encryption key delete failed", "user_id", user.Id, "error", err)
	}
}

// GetUser retrieves a user from the INSTANCE KV bucket.
func (c *ChattoCore) GetUser(ctx context.Context, userID string) (*corev1.User, error) {
	entry, err := c.storage.serverKV.Get(ctx, userKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	user := &corev1.User{}
	if err := proto.Unmarshal(entry.Value(), user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return user, nil
}

// GetUsers retrieves multiple users by ID from the INSTANCE KV bucket.
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

	// Fetch all users concurrently (NATS KV doesn't have multi-get)
	userMap := make(map[string]*corev1.User, len(uniqueIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range uniqueIDs {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()
			user, err := c.GetUser(ctx, userID)
			if err == nil {
				mu.Lock()
				userMap[userID] = user
				mu.Unlock()
			}
			// Silently ignore not-found errors (user may have been deleted)
		}(id)
	}
	wg.Wait()

	// Return in original order (nil for not-found users)
	result := make([]*corev1.User, len(userIDs))
	for i, id := range userIDs {
		result[i] = userMap[id] // nil if not found
	}

	return result, nil
}

// GetUserByLogin retrieves a user by their login name using the login index.
func (c *ChattoCore) GetUserByLogin(ctx context.Context, login string) (*corev1.User, error) {
	loginKey := userByLoginKey(login)
	entry, err := c.storage.serverKV.Get(ctx, loginKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by login: %w", err)
	}

	userID := string(entry.Value())
	return c.GetUser(ctx, userID)
}

// SetPasswordHash hashes and stores a password for a user.
// Password hashes are stored separately from user profile data and are not published to event streams.
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

	// Store password hash in separate KV key
	_, err = c.storage.serverKV.Put(ctx, userAuthPasswordKey(userID), hashedPassword)
	if err != nil {
		return fmt.Errorf("failed to store password: %w", err)
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
	entry, err := c.storage.serverKV.Get(ctx, userAuthPasswordKey(user.Id))
	if err != nil {
		// No password set (OAuth-only user) - run dummy bcrypt to match timing
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, fmt.Errorf("password not set for this user")
	}

	err = bcrypt.CompareHashAndPassword(entry.Value(), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

// UploadUserAvatar processes an image (resizes to 256x256 max, converts to WebP),
// uploads it to the object store (NATS or S3), and returns the asset reference.
// If the user already has an avatar, the old one is deleted after successful upload.
func (c *ChattoCore) UploadUserAvatar(ctx context.Context, userID string, reader io.Reader) (*corev1.Asset, error) {
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
	var asset *corev1.Asset

	if c.ShouldUseS3() {
		// Upload to S3 - use the same assetID as NATS would use for the key
		// The S3 path is constructed from the assetID for consistency
		s3Key := S3KeyServerAsset(assetID)
		_, err := c.s3Client.PutObjectFromBytes(ctx, s3Key, webpData, "image/webp")
		if err != nil {
			return nil, fmt.Errorf("failed to upload avatar to S3: %w", err)
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
		asset = &corev1.Asset{
			Asset: &corev1.Asset_Nats{
				Nats: &corev1.NATSAsset{
					Key: assetID,
				},
			},
		}
		c.logger.Info("Uploaded avatar", "user_id", userID, "size", info.Size)
	}

	// Delete old avatar now that new one is successfully uploaded
	if oldAvatar != nil {
		c.deleteAsset(ctx, oldAvatar, "avatar", userID)
	}

	return asset, nil
}

// SetUserAvatar stores the user's avatar asset reference in a separate KV key.
// This avoids overwriting the entire user record when the avatar changes.
func (c *ChattoCore) SetUserAvatar(ctx context.Context, userID string, asset *corev1.Asset) error {
	// Verify user exists
	_, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Marshal and store the asset at the scoped key
	assetData, err := proto.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal avatar asset: %w", err)
	}

	_, err = c.storage.serverKV.Put(ctx, userAvatarKey(userID), assetData)
	if err != nil {
		return fmt.Errorf("failed to store avatar: %w", err)
	}

	c.logger.Info("Updated user avatar", "user_id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return nil
}

// GetUserAvatar retrieves a user's avatar asset reference from the KV store.
// Returns nil if the user has no avatar set.
func (c *ChattoCore) GetUserAvatar(ctx context.Context, userID string) (*corev1.Asset, error) {
	entry, err := c.storage.serverKV.Get(ctx, userAvatarKey(userID))
	if err != nil {
		// No avatar set is not an error
		return nil, nil
	}

	asset := &corev1.Asset{}
	if err := proto.Unmarshal(entry.Value(), asset); err != nil {
		return nil, fmt.Errorf("failed to unmarshal avatar asset: %w", err)
	}

	return asset, nil
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
	c.deleteAsset(ctx, avatar, "avatar", userID)

	// Delete the KV reference
	if err := c.storage.serverKV.Delete(ctx, userAvatarKey(userID)); err != nil {
		return fmt.Errorf("failed to delete avatar reference: %w", err)
	}

	c.logger.Info("Deleted user avatar", "user_id", userID)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return nil
}

// publishUserProfileUpdate publishes a UserProfileUpdatedEvent to the instance stream.
// This allows other users to see profile changes (avatar, display name) in real-time.
func (c *ChattoCore) publishUserProfileUpdate(ctx context.Context, userID string) {
	// Get current user data
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		c.logger.Warn("failed to get user for profile update event", "error", err, "user_id", userID)
		return
	}

	// Get current avatar URL (full resolution for events)
	avatarURL, err := c.GetUserAvatarURL(ctx, userID, nil, nil)
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

	// Publish to live.server.user.{userId}.profile_updated for real-time delivery
	// Profile updates are transient (no need for JetStream storage/replay)
	subject := subjects.LiveUserEvent(userID, "profile_updated")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("failed to publish user profile update event", "error", err, "user_id", userID)
	}
}

// ListUsers retrieves all users from the INSTANCE KV bucket.
// CountUsers returns the total number of users on the server. Key-only scan.
func (c *ChattoCore) CountUsers(ctx context.Context) (int, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "user.*")
	if err != nil {
		return 0, fmt.Errorf("failed to list user keys: %w", err)
	}
	count := 0
	for range keyLister.Keys() {
		count++
	}
	return count, nil
}

func (c *ChattoCore) ListUsers(ctx context.Context) ([]*corev1.User, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "user.*")
	if err != nil {
		return []*corev1.User{}, nil
	}

	var users []*corev1.User
	for key := range keyLister.Keys() {
		entry, err := c.storage.serverKV.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get user %s: %w", key, err)
		}

		user := &corev1.User{}
		if err := proto.Unmarshal(entry.Value(), user); err != nil {
			return nil, fmt.Errorf("failed to unmarshal user %s: %w", key, err)
		}

		users = append(users, user)
	}

	return users, nil
}

// GetUserAvatarURL returns the URL for a user's avatar.
// If width and height are provided (non-nil), returns a URL to a resized version.
// Returns empty string if no avatar is set.
func (c *ChattoCore) GetUserAvatarURL(ctx context.Context, userID string, width, height *int) (string, error) {
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
	switch asset := avatar.Asset.(type) {
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

// ============================================================================
// Login Validation
// ============================================================================

// ErrLoginAlreadyTaken is returned when the login name is already taken.
var ErrLoginAlreadyTaken = fmt.Errorf("login name is already taken")

// ErrUsernameBlocked is returned when the login name is in the blocked list.
var ErrUsernameBlocked = fmt.Errorf("this username is not available")

// CheckLoginExists checks if a login name is already taken.
func (c *ChattoCore) CheckLoginExists(ctx context.Context, login string) (bool, error) {
	login = strings.TrimSpace(login)
	loginKey := userByLoginKey(login)
	_, err := c.storage.serverKV.Get(ctx, loginKey)
	if err != nil {
		return false, nil
	}
	return true, nil
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

	// Update display name
	user.DisplayName = displayName

	// Write updated user to KV store
	userData, err := proto.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	_, err = c.storage.serverKV.Put(ctx, userKey(userID), userData)
	if err != nil {
		return nil, fmt.Errorf("failed to store user: %w", err)
	}

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

	// Case-only change (e.g., "alice" → "Alice") — same KV key, just update the proto record.
	// No cooldown, no blocked-name check (the name itself hasn't changed), no index swap needed.
	if strings.EqualFold(user.Login, newLogin) {
		user.Login = newLogin
		userData, err := proto.Marshal(user)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal user: %w", err)
		}
		_, err = c.storage.serverKV.Put(ctx, userKey(userID), userData)
		if err != nil {
			return nil, fmt.Errorf("failed to store user: %w", err)
		}

		c.logger.Info("Updated user login casing", "id", userID, "new_login", newLogin)
		c.publishUserProfileUpdate(ctx, userID)
		return user, nil
	}

	// Check blocked list
	isBlocked, err := c.configManager.IsUsernameBlocked(ctx, newLogin)
	if err != nil {
		return nil, fmt.Errorf("failed to check blocked usernames: %w", err)
	}
	if isBlocked {
		return nil, ErrUsernameBlocked
	}

	// Check cooldown (skipped on admin path)
	if enforceCooldown {
		lastChange, err := c.GetLastLoginChange(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to check login change cooldown: %w", err)
		}
		if !lastChange.IsZero() && time.Since(lastChange) < LoginChangeCooldown {
			return nil, ErrLoginChangeCooldown
		}
	}

	// Atomic index swap: claim new login first
	oldLogin := user.Login
	oldLoginKey := userByLoginKey(oldLogin)
	newLoginKey := userByLoginKey(newLogin)

	_, err = c.storage.serverKV.Create(ctx, newLoginKey, []byte(userID))
	if err != nil {
		return nil, ErrLoginAlreadyTaken
	}

	// Update user record
	user.Login = newLogin
	userData, err := proto.Marshal(user)
	if err != nil {
		// Rollback: remove new login claim
		c.storage.serverKV.Delete(ctx, newLoginKey)
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	_, err = c.storage.serverKV.Put(ctx, userKey(userID), userData)
	if err != nil {
		// Rollback: remove new login claim
		c.storage.serverKV.Delete(ctx, newLoginKey)
		return nil, fmt.Errorf("failed to store user: %w", err)
	}

	// Delete old login index (best-effort)
	if deleteErr := c.storage.serverKV.Delete(ctx, oldLoginKey); deleteErr != nil {
		c.logger.Warn("Failed to delete old login index", "error", deleteErr, "old_login", oldLogin)
	}

	// Record change timestamp for cooldown (skipped on admin path)
	if enforceCooldown {
		now := time.Now().Format(time.RFC3339)
		if _, putErr := c.storage.serverKV.Put(ctx, userLoginChangedAtKey(userID), []byte(now)); putErr != nil {
			c.logger.Warn("Failed to record login change timestamp", "error", putErr, "user_id", userID)
		}
	}

	c.logger.Info("Updated user login", "id", userID, "new_login", newLogin)

	// Publish profile update event
	c.publishUserProfileUpdate(ctx, userID)

	return user, nil
}

// GetLastLoginChange returns when the user last changed their login.
// Returns zero time if the user has never changed their login.
func (c *ChattoCore) GetLastLoginChange(ctx context.Context, userID string) (time.Time, error) {
	entry, err := c.storage.serverKV.Get(ctx, userLoginChangedAtKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("failed to get last login change: %w", err)
	}

	t, err := time.Parse(time.RFC3339, string(entry.Value()))
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse login change timestamp: %w", err)
	}

	return t, nil
}

// ClearLoginChangeCooldown removes the cooldown timestamp for a user, allowing
// them to immediately change their login again. Idempotent — clearing an
// already-clear cooldown is a no-op.
// Authorization: Caller must verify admin privileges.
func (c *ChattoCore) ClearLoginChangeCooldown(ctx context.Context, userID string) error {
	err := c.storage.serverKV.Delete(ctx, userLoginChangedAtKey(userID))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear login change cooldown: %w", err)
	}
	c.logger.Info("Cleared user login change cooldown", "id", userID)
	c.publishUserProfileUpdate(ctx, userID)
	return nil
}

// ============================================================================
// Account Deletion Token Operations
// ============================================================================

// accountDeletionTokenKey returns the KV key for an account deletion token.
func accountDeletionTokenKey(token string) string {
	return "account_deletion_token." + token
}

// AccountDeletionTokenTTL is how long an account deletion token is valid.
const AccountDeletionTokenTTL = 15 * time.Minute

// AccountDeletionToken represents a token used to confirm account deletion.
type AccountDeletionToken struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAccountDeletionToken generates a confirmation token for account deletion.
// The token is stored in KV and must be provided to DeleteUser within the TTL.
func (c *ChattoCore) CreateAccountDeletionToken(ctx context.Context, userID string) (string, error) {
	token := NewAccountDeletionToken()

	tokenData := AccountDeletionToken{
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.storage.serverKV.Put(ctx, accountDeletionTokenKey(token), data)
	if err != nil {
		return "", fmt.Errorf("failed to store account deletion token: %w", err)
	}

	c.logger.Debug("Created account deletion token", "user_id", userID)
	return token, nil
}

// ValidateAccountDeletionToken validates a token and ensures it belongs to the user.
// If valid, the token is consumed (deleted) to prevent reuse.
// Returns an error if the token is invalid, expired, or doesn't belong to the user.
func (c *ChattoCore) ValidateAccountDeletionToken(ctx context.Context, token, userID string) error {
	key := accountDeletionTokenKey(token)

	entry, err := c.storage.serverKV.Get(ctx, key)
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
		c.storage.serverKV.Delete(ctx, key) // Clean up expired token
		return ErrTokenExpired
	}

	// Check if token belongs to the user
	if tokenData.UserID != userID {
		return ErrPermissionDenied
	}

	// Consume the token (delete it)
	if err := c.storage.serverKV.Delete(ctx, key); err != nil {
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

	// Get verified emails before deletion (for index cleanup)
	verifiedEmails, err := c.GetVerifiedEmails(ctx, userID)
	if err != nil {
		c.logger.Warn("Failed to get verified emails for deletion", "user_id", userID, "error", err)
		verifiedEmails = []VerifiedEmail{} // Continue anyway
	}

	// Iterate every space (typically just the primary server-space plus the
	// DM system space) to find user artifacts that need cleaning up. Space
	// membership records no longer exist post-#330 — every authenticated
	// user is implicitly a server member, so we work from the space list
	// directly.
	allSpaces, err := c.ListSpaces(ctx)
	if err != nil {
		c.logger.Warn("Failed to list spaces for deletion", "user_id", userID, "error", err)
		allSpaces = nil
	}

	// Delete all message bodies authored by this user
	for _, space := range allSpaces {
		deleted, err := c.deleteUserMessageBodiesInSpace(ctx, userID, space.Id)
		if err != nil {
			c.logger.Warn("Failed to delete message bodies in space", "user_id", userID, "space_id", space.Id, "error", err)
		} else if deleted > 0 {
			c.logger.Info("Deleted message bodies during user deletion", "user_id", userID, "space_id", space.Id, "count", deleted)
		}
	}

	// Delete encryption key (crypto-shreds any remaining encrypted data)
	if err := c.DeleteUserEncryptionKey(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete encryption key", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}

	// Delete push notification subscriptions
	if _, err := c.DeleteAllUserPushSubscriptions(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete push subscriptions", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}

	// Delete avatar from object store if it exists
	avatar, _ := c.GetUserAvatar(ctx, userID)
	if avatar != nil {
		if natsAsset := avatar.GetNats(); natsAsset != nil {
			if err := c.storage.serverStore.Delete(ctx, natsAsset.Key); err != nil {
				c.logger.Warn("Failed to delete avatar from object store", "user_id", userID, "key", natsAsset.Key, "error", err)
			}
		}
	}

	// Delete email index entries
	for _, email := range verifiedEmails {
		emailKey := userByEmailKey(email.Email)
		if err := c.storage.serverKV.Delete(ctx, emailKey); err != nil {
			c.logger.Warn("Failed to delete email index", "user_id", userID, "email", email.Email, "error", err)
		}
	}

	// Delete user KV entries BEFORE leaving spaces.
	// This ensures that when SpaceMemberDeletedEvent is published and clients refetch,
	// the user record is already gone and they see "Deleted User".
	keysToDelete := []string{
		userKey(userID),               // user profile
		userAuthPasswordKey(userID),   // password hash
		userAvatarKey(userID),         // avatar reference
		userVerifiedEmailsKey(userID), // verified emails list
		userByLoginKey(user.Login),    // login index
		userPreferencesKey(userID),    // user preferences
	}

	for _, key := range keysToDelete {
		if err := c.storage.serverKV.Delete(ctx, key); err != nil {
			// Log but don't fail - some keys may not exist (e.g., no password for OAuth users)
			c.logger.Debug("Failed to delete key during user deletion", "key", key, "error", err)
		}
	}

	// Clean per-space user artifacts AFTER the user record is deleted, so
	// the SpaceMemberDeletedEvent triggered inside lands when client refetches
	// already see "Deleted User".
	for _, space := range allSpaces {
		if err := c.CleanupUserStateInSpace(ctx, userID, space.Id, true); err != nil {
			c.logger.Warn("Failed to clean up user state in space during deletion", "user_id", userID, "space_id", space.Id, "error", err)
		}
	}

	// Revoke all role assignments (server-wide, no per-space loop needed).
	if err := c.RevokeAllUserRoles(ctx, userID); err != nil {
		c.logger.Warn("Failed to revoke user roles during deletion", "user_id", userID, "error", err)
	}

	// Publish instance-level UserDeletedEvent for audit logging and admin UI updates
	serverEvent := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserDeleted{
			UserDeleted: &corev1.UserDeletedEvent{
				UserId: userID,
			},
		},
	})
	serverSubject := subjects.LiveUserEvent(userID, "user_deleted")
	if err := c.publishLiveEvent(ctx, serverSubject, serverEvent); err != nil {
		c.logger.Warn("Failed to publish UserDeletedEvent", "user_id", userID, "error", err)
	}

	c.logger.Info("Deleted user account", "id", userID, "login", user.Login)

	return nil
}
