package core

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// SetPasswordHash hashes and stores a password for a user.
// Password hashes are stored separately from user profile data in the user event stream.
func (c *ChattoCore) SetPasswordHash(ctx context.Context, userID string, password string) error {
	return c.SetPasswordHashAs(ctx, userID, userID, password)
}

// SetPasswordHashAs hashes and stores a password for a user with explicit
// actor attribution. Operator/admin flows should pass SystemActorID.
func (c *ChattoCore) SetPasswordHashAs(ctx context.Context, actorID, userID string, password string) error {
	return c.setPasswordHash(ctx, actorID, userID, password, true, nil)
}

// SetInitialPasswordHash adds the first password credential for a passwordless
// account. It refuses to overwrite an existing password.
func (c *ChattoCore) SetInitialPasswordHash(ctx context.Context, userID string, password string) error {
	return c.setPasswordHash(ctx, userID, userID, password, false, func() error {
		if _, hasPassword := c.userModel.passwordHash(userID); hasPassword {
			return ErrPasswordAlreadySet
		}
		return nil
	})
}

// SetOwnPassword sets or changes a user's own password. Existing password
// credentials require current password proof; adding the first password keeps
// existing runtime credentials valid so SSO-only users are not logged out.
func (c *ChattoCore) SetOwnPassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	hasPassword, err := c.HasPassword(ctx, userID)
	if err != nil {
		return err
	}
	if !hasPassword {
		return c.SetInitialPasswordHash(ctx, userID, newPassword)
	}
	if currentPassword == "" {
		return ErrCurrentPasswordRequired
	}
	if err := c.VerifyUserPassword(ctx, userID, currentPassword); err != nil {
		return err
	}
	return c.setPasswordHash(ctx, userID, userID, newPassword, true, func() error {
		return c.verifyUserPasswordCurrent(ctx, userID, currentPassword)
	})
}

// AdminSetUserPasswordAuthorized sets a password for the target account without requiring
// the old password. Changing another account requires the same admin-management
// gate used by admin identity changes.
func (c *ChattoCore) AdminSetUserPasswordAuthorized(ctx context.Context, actorID, targetUserID, password string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	if targetUserID == "" {
		return fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}
	if actorID == targetUserID {
		return ErrAdminCannotSetOwnPassword
	}
	canManage, err := c.CanManageUserAccounts(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check user.manage-accounts: %w", err)
	}
	if !canManage {
		return ErrPermissionDenied
	}
	return c.setPasswordHash(ctx, actorID, targetUserID, password, true, nil)
}

func (c *ChattoCore) HasPassword(ctx context.Context, userID string) (bool, error) {
	if err := c.userModel.waitForUsersCurrent(ctx, "user password", evtstream.UserAggregate(userID).AllEventsFilter()); err != nil {
		return false, err
	}
	_, ok, err := c.userModel.user(ctx, userID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, ErrNotFound
	}
	_, hasPassword := c.userModel.passwordHash(userID)
	return hasPassword, nil
}

func (c *ChattoCore) VerifyUserPassword(ctx context.Context, userID, password string) error {
	if err := c.userModel.waitForUsersCurrent(ctx, "user password", evtstream.UserAggregate(userID).AllEventsFilter()); err != nil {
		return err
	}
	return c.verifyUserPasswordCurrent(ctx, userID, password)
}

func (c *ChattoCore) verifyUserPasswordCurrent(ctx context.Context, userID, password string) error {
	_, ok, err := c.userModel.user(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	passwordHash, ok := c.userModel.passwordHash(userID)
	if !ok {
		return ErrCurrentPasswordRequired
	}
	if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(password)); err != nil {
		return ErrCurrentPasswordInvalid
	}
	return nil
}

func (c *ChattoCore) setPasswordHash(ctx context.Context, actorID, userID string, password string, revokeCredentials bool, check func() error) error {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return err
	}
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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserPasswordHashChanged{
		UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{
			UserId:                      userID,
			PasswordHash:                hashedPassword,
			PreserveExistingCredentials: !revokeCredentials,
		},
	}})
	if _, err := c.appendUserEvent(ctx, userID, event, "", func() error {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		if check != nil {
			return check()
		}
		return nil
	}); err != nil {
		return err
	}
	if !revokeCredentials {
		return nil
	}
	if _, err := c.RevokeRuntimeCredentialsForUser(ctx, userID, "password_changed"); err != nil {
		c.logger.Warn("Failed to clean up runtime credentials after password change", "user_id", userID, "error", err)
	}
	if err := c.PublishSessionTerminated(ctx, userID, "password_changed"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", userID, "reason", "password_changed", "error", err)
	}
	return nil
}

// VerifyPassword verifies a user's password by login name or email and returns the user if valid.
func (c *ChattoCore) VerifyPassword(ctx context.Context, identifier string, password string) (*evtv1.User, error) {
	user, _, err := c.VerifyPasswordWithAuthGeneration(ctx, identifier, password)
	return user, err
}

// VerifyPasswordWithAuthGeneration verifies a password and returns the user
// auth generation that was current when the password hash was checked.
func (c *ChattoCore) VerifyPasswordWithAuthGeneration(ctx context.Context, identifier string, password string) (*evtv1.User, uint64, error) {
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
		return nil, 0, fmt.Errorf("invalid credentials")
	}
	if user.GetIsBot() {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, 0, fmt.Errorf("invalid credentials")
	}

	return c.verifyUserPassword(ctx, user, password, dummyHash)
}

// verifyUserPassword is an internal helper that verifies a password for an already-fetched user.
func (c *ChattoCore) verifyUserPassword(ctx context.Context, user *evtv1.User, password string, dummyHash []byte) (*evtv1.User, uint64, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		return nil, 0, err
	}

	// Retrieve password hash from the user projection.
	passwordHash, ok := c.userModel.passwordHash(user.Id)
	if !ok {
		// No password set (OAuth-only user) - run dummy bcrypt to match timing
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, 0, fmt.Errorf("password not set for this user")
	}

	err = bcrypt.CompareHashAndPassword(passwordHash, []byte(password))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid credentials")
	}

	return user, authGeneration, nil
}
