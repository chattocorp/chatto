package core

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// ReadAdminUser returns administrative account data without loading a role
// catalogue or evaluating role-assignment limits. It requires admin.view-users.
func (c *ChattoCore) ReadAdminUser(ctx context.Context, actorID, userID string) (*AdminMember, error) {
	ctx = WithDEKRequestCache(ctx)
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: user ID is required", ErrInvalidArgument)
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return c.hydrateAdminUser(ctx, actorID, user)
}

func (c *ChattoCore) hydrateAdminUser(ctx context.Context, actorID string, user *readmodel.User) (*AdminMember, error) {
	roles, err := c.GetUserRoles(ctx, user.GetId())
	if err != nil {
		return nil, err
	}
	return c.adminMemberForViewer(ctx, actorID, user, roles)
}

// ReadAdminUserByLogin applies the account-read gate before resolving a login.
func (c *ChattoCore) ReadAdminUserByLogin(ctx context.Context, actorID, login string) (*AdminMember, error) {
	ctx = WithDEKRequestCache(ctx)
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	if login == "" {
		return nil, fmt.Errorf("%w: login is required", ErrInvalidArgument)
	}
	user, err := c.GetUserByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	return c.hydrateAdminUser(ctx, actorID, user)
}
