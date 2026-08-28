package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
// Issuance enforces the human-account check and the caller's user.delete-self
// permission, so revoking that permission disables self-service deletion: no
// valid confirmation token can be issued or redeemed without it.
func (c *ChattoCore) CreateAccountDeletionToken(ctx context.Context, userID string) (string, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return "", err
	}
	canDeleteSelf, err := c.CanDeleteUser(ctx, userID, userID)
	if err != nil {
		return "", err
	}
	if !canDeleteSelf {
		return "", ErrPermissionDenied
	}
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
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if !user.GetIsBot() {
		if err := c.deleteOwnedBots(ctx, actorID, userID); err != nil {
			return err
		}
	}

	// Post-ADR-030 there are two implicit scopes — channel and DM — and
	// cleanup iterates each kind.
	allKinds := []RoomKind{KindChannel, KindDM}

	// Delete encryption key (crypto-shreds any remaining encrypted data) and
	// record the durable shred signal projections use to tombstone messages
	// before decrypting.
	if err := c.DeleteUserEncryptionKeyAs(ctx, actorID, userID); err != nil {
		_, _, requested, requestErr := c.keyShredding.requestFact(ctx, userID)
		if requestErr != nil {
			return errors.Join(fmt.Errorf("delete user encryption keys: %w", err), fmt.Errorf("confirm durable shredding request: %w", requestErr))
		}
		if !requested {
			return fmt.Errorf("record durable user-key shredding request: %w", err)
		}
		// Once the logical privacy boundary is durable, the worker can safely
		// finish physical deletion and completion after this request returns.
		c.logger.Warn("Physical user-key shredding remains pending", "user_id", userID, "error", err)
	}

	if deleted := c.assetModel.DeleteMessageOwnedAssetsForUser(ctx, actorID, userID); deleted > 0 {
		c.logger.Info("Deleted message-owned assets during user deletion", "user_id", userID, "count", deleted)
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
	for {
		// Owned-bot checks depend on all user aggregates, but not on unrelated
		// EVT facts. The authorization fence still serializes this deletion
		// against bot-owner reassignment and other authorization changes.
		_, err := c.appendUserEvent(ctx, userID, deletedEvent, evtstream.UserSubjectFilter(), func() error {
			if !user.GetIsBot() && len(c.userModel.botIDsOwnedBy(userID)) > 0 {
				return errOwnedBotsRemain
			}
			return nil
		})
		if !errors.Is(err, errOwnedBotsRemain) {
			if err != nil {
				return fmt.Errorf("failed to mark user deleted: %w", err)
			}
			break
		}
		if err := c.deleteOwnedBots(ctx, actorID, userID); err != nil {
			return err
		}
	}
	// Attempt immediate erasure after the durable deletion fact commits. A
	// failure cannot roll the account deletion back; the dedicated durable
	// worker will retry the same idempotent cleanup until it succeeds.
	if _, err := c.DeleteAllUserPushSubscriptions(ctx, userID); err != nil {
		c.logger.Warn("Push-subscription cleanup remains pending after account deletion", "user_id", userID, "error", err)
	}
	if _, err := c.RevokeRuntimeCredentialsForUser(ctx, userID, "account_deleted"); err != nil {
		c.logger.Warn("Failed to revoke runtime credentials during deletion", "user_id", userID, "error", err)
		// Continue - this is best-effort
	}
	if err := c.deleteUserSettings(ctx, userID); err != nil {
		c.logger.Warn("Failed to delete user settings during deletion", "user_id", userID, "error", err)
	}
	if user.GetIsBot() {
		c.credentialUsage.ForgetAll(ctx, userID)
	}

	// Clean per-kind user artifacts AFTER the user projection marks the
	// account deleted, so ServerMemberDeletedEvent refetches already see
	// "Deleted User".
	for _, kind := range allKinds {
		if err := c.CleanupUserState(ctx, userID, kind, true); err != nil {
			c.logger.Warn("Failed to clean up user state during deletion", "user_id", userID, "kind", kind, "error", err)
		}
	}

	// Revoke all role assignments (server-wide, no per-space loop needed).
	if err := c.RevokeAllUserRoles(ctx, actorID, userID); err != nil {
		c.logger.Warn("Failed to revoke user roles during deletion", "user_id", userID, "error", err)
	}

	if err := c.PublishSessionTerminated(ctx, userID, "account_deleted"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", userID, "error", err)
	}

	c.logger.Info("Deleted user account", "id", userID)

	return nil
}

var errOwnedBotsRemain = errors.New("owned bots remain")

func (c *ChattoCore) deleteOwnedBots(ctx context.Context, actorID, ownerUserID string) error {
	for _, botID := range c.userModel.botIDsOwnedBy(ownerUserID) {
		if err := c.DeleteUser(ctx, actorID, botID); err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("delete owned bot: %w", err)
		}
	}
	return nil
}
