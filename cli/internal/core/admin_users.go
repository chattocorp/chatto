package core

import (
	"context"
	"fmt"
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// AdminUserView is the core representation returned by operator user
// administration workflows.
type AdminUserView struct {
	User           *corev1.User
	RoleNames      []string
	VerifiedEmails []VerifiedEmail
}

// AdminUserList is a paginated operator user listing.
type AdminUserList struct {
	Users      []*AdminUserView
	TotalCount int
	HasMore    bool
}

// AdminCreateUserRequest describes one operator-created user.
type AdminCreateUserRequest struct {
	Login         string
	DisplayName   string
	Password      string
	VerifiedEmail string
	RoleNames     []string
}

// AdminUpdateUserRequest describes one operator profile update.
type AdminUpdateUserRequest struct {
	UserID      string
	Login       *string
	DisplayName *string
}

// AdminListUsers returns users with their admin-facing role and verified-email
// data hydrated.
func (c *ChattoCore) AdminListUsers(ctx context.Context, search string, limit, offset int) (*AdminUserList, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		return nil, ErrInvalidArgument
	}

	members, totalCount, err := c.GetServerMembers(ctx, search, limit, offset)
	if err != nil {
		return nil, err
	}
	users := make([]*AdminUserView, 0, len(members))
	for _, member := range members {
		user, err := c.AdminGetUser(ctx, member.UserID)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return &AdminUserList{
		Users:      users,
		TotalCount: totalCount,
		HasMore:    offset+len(users) < totalCount,
	}, nil
}

// AdminGetUser returns a hydrated operator view for one user ID.
func (c *ChattoCore) AdminGetUser(ctx context.Context, userID string) (*AdminUserView, error) {
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := c.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	emails, err := c.GetVerifiedEmails(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AdminUserView{
		User:           user,
		RoleNames:      append([]string(nil), roles...),
		VerifiedEmails: append([]VerifiedEmail(nil), emails...),
	}, nil
}

// AdminGetUserByLogin returns a hydrated operator view for one login.
func (c *ChattoCore) AdminGetUserByLogin(ctx context.Context, login string) (*AdminUserView, error) {
	user, err := c.GetUserByLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return c.AdminGetUser(ctx, user.GetId())
}

// AdminCreateUser creates a user and applies optional operator-managed email
// and role state. If any post-create step fails, it compensates by deleting the
// just-created account.
func (c *ChattoCore) AdminCreateUser(ctx context.Context, req AdminCreateUserRequest) (*AdminUserView, error) {
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(req.Login)
	}
	if email := strings.TrimSpace(req.VerifiedEmail); email != "" {
		claimed, err := c.IsEmailClaimed(ctx, email)
		if err != nil {
			return nil, err
		}
		if claimed {
			return nil, ErrEmailAlreadyVerified
		}
	}
	for _, roleName := range req.RoleNames {
		if roleName == RoleEveryone {
			return nil, ErrImplicitRole
		}
		if !c.RBAC.RoleExists(roleName) {
			return nil, ErrRoleNotFound
		}
	}

	user, err := c.CreateUser(ctx, SystemActorID, req.Login, displayName, req.Password)
	if err != nil {
		return nil, err
	}
	if email := strings.TrimSpace(req.VerifiedEmail); email != "" {
		if err := c.AddVerifiedEmailDirectAs(ctx, SystemActorID, user.GetId(), email); err != nil {
			c.rollbackUserCreation(ctx, user)
			return nil, fmt.Errorf("failed to add verified email for new user: %w", err)
		}
	}
	for _, roleName := range req.RoleNames {
		if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, user.GetId(), roleName); err != nil {
			c.rollbackUserCreation(ctx, user)
			return nil, fmt.Errorf("failed to assign role to new user: %w", err)
		}
	}

	adminUser, err := c.AdminGetUser(ctx, user.GetId())
	if err != nil {
		c.rollbackUserCreation(ctx, user)
		return nil, err
	}
	return adminUser, nil
}

// AdminUpdateUser updates admin-managed profile fields and returns the hydrated
// user view.
func (c *ChattoCore) AdminUpdateUser(ctx context.Context, req AdminUpdateUserRequest) (*AdminUserView, error) {
	if req.Login == nil && req.DisplayName == nil {
		return nil, ErrInvalidArgument
	}
	user, err := c.AdminUpdateUserProfile(ctx, req.UserID, req.Login, req.DisplayName)
	if err != nil {
		return nil, err
	}
	return c.AdminGetUser(ctx, user.GetId())
}

// AdminSetUserPassword sets a user's password as the system actor.
func (c *ChattoCore) AdminSetUserPassword(ctx context.Context, userID, password string) (*AdminUserView, error) {
	if err := c.SetPasswordHashAs(ctx, SystemActorID, userID, password); err != nil {
		return nil, err
	}
	return c.AdminGetUser(ctx, userID)
}

// AdminDeleteUser permanently deletes a user as the system actor.
func (c *ChattoCore) AdminDeleteUser(ctx context.Context, userID string) error {
	return c.DeleteUser(ctx, SystemActorID, userID)
}

// AdminAddUserVerifiedEmail adds an already-verified email to a user.
func (c *ChattoCore) AdminAddUserVerifiedEmail(ctx context.Context, userID, email string) (*AdminUserView, error) {
	if err := c.AddVerifiedEmailDirectAs(ctx, SystemActorID, userID, email); err != nil {
		return nil, err
	}
	return c.AdminGetUser(ctx, userID)
}

// AdminAssignUserRole assigns a server role to an existing user.
func (c *ChattoCore) AdminAssignUserRole(ctx context.Context, userID, roleName string) (*AdminUserView, error) {
	if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, roleName); err != nil {
		return nil, err
	}
	return c.AdminGetUser(ctx, userID)
}

// AdminRevokeUserRole revokes a server role from an existing user.
func (c *ChattoCore) AdminRevokeUserRole(ctx context.Context, userID, roleName string) (*AdminUserView, error) {
	if err := c.RevokeServerRoleFromExistingUser(ctx, SystemActorID, userID, roleName); err != nil {
		return nil, err
	}
	return c.AdminGetUser(ctx, userID)
}
