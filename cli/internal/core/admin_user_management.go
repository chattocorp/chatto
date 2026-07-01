package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type AdminMemberListInput struct {
	Search string
	Limit  int
	Offset int
}

type AdminMemberRoleSummary struct {
	Name        string
	DisplayName string
	Description string
	IsSystem    bool
	Position    int32
	Pingable    bool
}

type AdminMemberRole struct {
	Name              string
	DisplayName       string
	Description       string
	IsSystem          bool
	Position          int32
	Pingable          bool
	Permissions       []Permission
	PermissionDenials []Permission
}

type AdminMember struct {
	ID                     string
	Login                  string
	DisplayName            string
	AvatarURL              string
	Roles                  []string
	CreatedAt              *timestamppb.Timestamp
	Deleted                bool
	HasVerifiedEmail       bool
	VerifiedEmails         []string
	ViewerCanDeleteAccount bool
	LastLoginChange        *time.Time
}

type AdminMemberList struct {
	Users      []AdminMember
	Roles      []AdminMemberRoleSummary
	TotalCount int
	HasMore    bool
}

type AdminMemberDetails struct {
	Member                         *AdminMember
	Roles                          []AdminMemberRole
	AvailablePermissions           []Permission
	ViewerCanAssignRoles           bool
	ViewerCanManageRoles           bool
	ViewerCanManageUserPermissions bool
}

func (c *ChattoCore) ListAdminMembers(ctx context.Context, actorID string, input AdminMemberListInput) (*AdminMemberList, error) {
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	limit, offset := adminMemberPagination(input.Limit, input.Offset)

	members, totalCount, err := c.GetServerMembers(ctx, input.Search, limit, offset)
	if err != nil {
		return nil, err
	}

	users := make([]AdminMember, 0, len(members))
	for _, member := range members {
		user, err := c.GetUser(ctx, member.UserID)
		if err != nil {
			continue
		}
		adminMember, err := c.adminMemberForViewer(ctx, actorID, user, explicitServerRoles(member.Roles))
		if err != nil {
			return nil, err
		}
		users = append(users, *adminMember)
	}

	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}

	return &AdminMemberList{
		Users:      users,
		Roles:      adminMemberRoleSummaries(roles),
		TotalCount: totalCount,
		HasMore:    offset+len(users) < totalCount,
	}, nil
}

func (c *ChattoCore) GetAdminMemberDetails(ctx context.Context, actorID, targetUserID string) (*AdminMemberDetails, error) {
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	if targetUserID == "" {
		return nil, fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}

	user, err := c.GetUser(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	assignedRoles, err := c.GetUserRoles(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	member, err := c.adminMemberForViewer(ctx, actorID, user, assignedRoles)
	if err != nil {
		return nil, err
	}

	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}
	canAssignRoles, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	canManageRoles, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	canManageUserPermissions, err := c.CanManageUserPermissions(ctx, actorID)
	if err != nil {
		return nil, err
	}

	return &AdminMemberDetails{
		Member:                         member,
		Roles:                          adminMemberRoles(roles),
		AvailablePermissions:           c.AllServerPermissions(),
		ViewerCanAssignRoles:           canAssignRoles,
		ViewerCanManageRoles:           canManageRoles,
		ViewerCanManageUserPermissions: canManageUserPermissions,
	}, nil
}

func (c *ChattoCore) BatchGetAdminMembers(ctx context.Context, actorID string, userIDs []string) (*AdminMemberList, error) {
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(userIDs))
	users := make([]AdminMember, 0, len(userIDs))
	for _, userID := range userIDs {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		user, err := c.GetUser(ctx, userID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		assignedRoles, err := c.GetUserRoles(ctx, userID)
		if err != nil {
			return nil, err
		}
		member, err := c.adminMemberForViewer(ctx, actorID, user, assignedRoles)
		if err != nil {
			return nil, err
		}
		users = append(users, *member)
	}

	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}

	return &AdminMemberList{
		Users:      users,
		Roles:      adminMemberRoleSummaries(roles),
		TotalCount: len(users),
	}, nil
}

func (c *ChattoCore) requireCanViewAdminMembers(ctx context.Context, actorID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canView, err := c.CanAdminUsersView(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check admin.view-users: %w", err)
	}
	if !canView {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) AdminAssignServerRole(ctx context.Context, actorID, targetUserID, roleName string) error {
	if err := c.requireCanAssignAdminRole(ctx, actorID, targetUserID, roleName); err != nil {
		return err
	}
	return c.AssignServerRoleToExistingUser(ctx, actorID, targetUserID, roleName)
}

func (c *ChattoCore) AdminRevokeServerRole(ctx context.Context, actorID, targetUserID, roleName string) error {
	if err := c.requireCanAssignAdminRole(ctx, actorID, targetUserID, roleName); err != nil {
		return err
	}
	if actorID == targetUserID && (roleName == RoleOwner || roleName == RoleAdmin) {
		return ErrCannotRevokeSelfAdmin
	}
	return c.RevokeServerRoleFromExistingUser(ctx, actorID, targetUserID, roleName)
}

func (c *ChattoCore) requireCanAssignAdminRole(ctx context.Context, actorID, targetUserID, roleName string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	if targetUserID == "" {
		return fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}
	if roleName == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalidArgument)
	}
	canAssign, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check role.assign: %w", err)
	}
	if !canAssign {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) adminMemberForViewer(ctx context.Context, actorID string, user *corev1.User, roles []string) (*AdminMember, error) {
	avatarURL, err := c.GetUserAvatarURL(ctx, user.GetId(), nil, nil, "")
	if err != nil {
		return nil, err
	}

	member := &AdminMember{
		ID:          user.GetId(),
		Login:       user.GetLogin(),
		DisplayName: user.GetDisplayName(),
		AvatarURL:   avatarURL,
		Roles:       roles,
		CreatedAt:   user.GetCreatedAt(),
		Deleted:     user.GetDeleted(),
	}

	if canViewEmails, err := c.canViewAdminMemberEmails(ctx, actorID, user.GetId()); err != nil {
		return nil, err
	} else if canViewEmails {
		hasVerifiedEmail, err := c.HasVerifiedEmail(ctx, user.GetId())
		if err != nil {
			return nil, err
		}
		verifiedEmails, err := c.GetVerifiedEmails(ctx, user.GetId())
		if err != nil {
			return nil, err
		}
		member.HasVerifiedEmail = hasVerifiedEmail
		member.VerifiedEmails = make([]string, 0, len(verifiedEmails))
		for _, email := range verifiedEmails {
			member.VerifiedEmails = append(member.VerifiedEmails, email.Email)
		}
	}

	if actorID != user.GetId() {
		viewerCanDeleteAccount, err := c.CanDeleteUser(ctx, actorID, user.GetId())
		if err != nil {
			return nil, err
		}
		member.ViewerCanDeleteAccount = viewerCanDeleteAccount
	}

	if canViewLastLoginChange, err := c.canViewAdminMemberLastLoginChange(ctx, actorID, user.GetId()); err != nil {
		return nil, err
	} else if canViewLastLoginChange {
		lastLoginChange, err := c.GetLastLoginChange(ctx, user.GetId())
		if err != nil {
			return nil, err
		}
		if !lastLoginChange.IsZero() {
			member.LastLoginChange = &lastLoginChange
		}
	}

	return member, nil
}

func (c *ChattoCore) canViewAdminMemberEmails(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return true, nil
	}
	return c.CanAdminUsersView(ctx, actorID)
}

func (c *ChattoCore) canViewAdminMemberLastLoginChange(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return true, nil
	}
	isOwner, err := c.IsServerOwner(ctx, actorID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}
	return c.IsServerAdmin(ctx, actorID)
}

func adminMemberPagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func explicitServerRoles(roles []string) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		if role != RoleEveryone {
			out = append(out, role)
		}
	}
	return out
}

func adminMemberRoleSummaries(roles []RoleWithPermissions) []AdminMemberRoleSummary {
	out := make([]AdminMemberRoleSummary, 0, len(roles))
	for _, role := range roles {
		out = append(out, AdminMemberRoleSummary{
			Name:        role.Name,
			DisplayName: role.DisplayName,
			Description: role.Description,
			IsSystem:    role.IsSystem,
			Position:    role.Position,
			Pingable:    role.Pingable,
		})
	}
	return out
}

func adminMemberRoles(roles []RoleWithPermissions) []AdminMemberRole {
	out := make([]AdminMemberRole, 0, len(roles))
	for _, role := range roles {
		out = append(out, AdminMemberRole{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Description:       role.Description,
			IsSystem:          role.IsSystem,
			Position:          role.Position,
			Pingable:          role.Pingable,
			Permissions:       append([]Permission{}, role.Permissions...),
			PermissionDenials: append([]Permission{}, role.PermissionDenials...),
		})
	}
	return out
}
