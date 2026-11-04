package core

import (
	"context"
	"fmt"
	"time"
)

// ListAdminVerifiedEmails reads private addresses without hydrating profiles,
// roles, or editor data. It requires admin.view-users.
func (c *ChattoCore) ListAdminVerifiedEmails(ctx context.Context, actorID, userID string) ([]VerifiedEmail, error) {
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	if err := c.requireExistingAdminUser(userID); err != nil {
		return nil, err
	}
	return c.GetVerifiedEmails(ctx, userID)
}

// GetAdminUsernameCooldown reads private username-change history. A nil
// timestamp means no change is recorded. Hidden history returns an access error.
func (c *ChattoCore) GetAdminUsernameCooldown(ctx context.Context, actorID, userID string) (*time.Time, error) {
	// Check both permissions before resolving the target's existence.
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	allowed, err := c.canViewAdminMemberLastLoginChange(ctx, actorID, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	if err := c.requireExistingAdminUser(userID); err != nil {
		return nil, err
	}
	changedAt, err := c.GetLastLoginChange(ctx, userID)
	if err != nil || changedAt.IsZero() {
		return nil, err
	}
	return &changedAt, nil
}

// requireExistingAdminUser uses lifecycle metadata without decrypting a public
// profile. Callers must check the read's permissions before this lookup.
func (c *ChattoCore) requireExistingAdminUser(userID string) error {
	if userID == "" {
		return fmt.Errorf("%w: user ID is required", ErrInvalidArgument)
	}
	if _, _, exists := c.userModel.isBotAndOwner(userID); !exists {
		return ErrNotFound
	}
	return nil
}
