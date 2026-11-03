package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	ID          string
	Login       string
	DisplayName string
	// Bio is the user's public biography. Empty means unset.
	Bio string
	// SharedTimezone is public only while the user enables time-zone sharing.
	SharedTimezone              string
	AvatarURL                   string
	Roles                       []string
	CreatedAt                   *timestamppb.Timestamp
	Deleted                     bool
	Deactivated                 bool
	ViewerCanChangeDeactivation bool
	IsBot                       bool
	// BotOwnerUserID identifies the human owner of a bot.
	BotOwnerUserID   string
	HasVerifiedEmail bool
	VerifiedEmails   []string
	// PrimaryVerifiedEmail is empty when no primary address is visible.
	PrimaryVerifiedEmail   string
	ViewerCanDeleteAccount bool
	LastLoginChange        *time.Time
	// UsernameCooldownVisible distinguishes a hidden cooldown from no recorded change.
	UsernameCooldownVisible bool
	CustomStatus            *evtv1.CustomUserStatus
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
	AssignableRoleNames            []string
	RevocableRoleNames             []string
}

// AdminMemberIDPage selects members without hydrating administrative fields.
type AdminMemberIDPage struct {
	UserIDs    []string
	TotalCount int
	HasMore    bool
}

// ListAdminMembers returns ordered IDs for an authorized admin directory read.
// Hydration is owned by BatchGetAdminMembers and checks authorization again.
func (c *ChattoCore) ListAdminMembers(ctx context.Context, actorID string, input AdminMemberListInput) (*AdminMemberIDPage, error) {
	if err := c.requireCanViewAdminMembers(ctx, actorID); err != nil {
		return nil, err
	}
	limit, offset := adminMemberPagination(input.Limit, input.Offset)

	allUsers, err := c.userModel.adminDirectoryCandidates(ctx, input.Search)
	if err != nil {
		return nil, err
	}

	members, totalCount := serverMemberUserPage(allUsers, input.Search, limit, offset)
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.GetId())
	}

	return &AdminMemberIDPage{
		UserIDs:    ids,
		TotalCount: totalCount,
		HasMore:    offset+len(ids) < totalCount,
	}, nil
}

func (c *ChattoCore) GetAdminMemberDetails(ctx context.Context, actorID, targetUserID string) (*AdminMemberDetails, error) {
	member, err := c.ReadAdminUser(ctx, actorID, targetUserID)
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
	if member.IsBot {
		canAssignRoles = false
		canManageRoles = false
		canManageUserPermissions = false
		member.ViewerCanDeleteAccount = false
	}

	assignableRoleNames := make([]string, 0, len(roles))
	revocableRoleNames := make([]string, 0, len(roles))
	if canAssignRoles {
		assignableRoleNames, revocableRoleNames, err = c.roleAssignmentLimits(ctx, actorID, targetUserID, roles)
		if err != nil {
			return nil, err
		}
	}

	return &AdminMemberDetails{
		Member:                         member,
		Roles:                          adminMemberRoles(roles),
		AvailablePermissions:           c.AllServerPermissions(),
		ViewerCanAssignRoles:           canAssignRoles,
		ViewerCanManageRoles:           canManageRoles,
		ViewerCanManageUserPermissions: canManageUserPermissions,
		AssignableRoleNames:            assignableRoleNames,
		RevocableRoleNames:             revocableRoleNames,
	}, nil
}

// UserRoleAssignments contains explicit human-account roles and advisory action
// limits. It excludes private account fields and the implicit everyone role.
type UserRoleAssignments struct {
	UserID              string
	RoleNames           []string
	AssignableRoleNames []string
	RevocableRoleNames  []string
}

// GetUserRoleAssignments checks role.assign independently of admin.view-users.
// Each call reads current state; commands must still recheck their own policy.
func (c *ChattoCore) GetUserRoleAssignments(ctx context.Context, actorID, targetUserID string) (*UserRoleAssignments, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	allowed, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	if targetUserID == "" {
		return nil, fmt.Errorf("%w: target user ID is required", ErrInvalidArgument)
	}
	// Assignment reads need lifecycle metadata, not an encrypted profile.
	isBot, _, exists := c.userModel.isBotAndOwner(targetUserID)
	if !exists {
		return nil, ErrNotFound
	}
	if isBot {
		return nil, ErrHumanAccountRequired
	}
	assigned, err := c.GetUserRoles(ctx, targetUserID)
	if err != nil {
		return nil, err
	}
	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}
	assignable, revocable, err := c.roleAssignmentLimits(ctx, actorID, targetUserID, roles)
	if err != nil {
		return nil, err
	}
	return &UserRoleAssignments{
		UserID: targetUserID, RoleNames: assigned,
		AssignableRoleNames: assignable, RevocableRoleNames: revocable,
	}, nil
}

// roleAssignmentLimits uses the same domain policy as assignment commands and
// member details, including target-specific revocation restrictions.
func (c *ChattoCore) roleAssignmentLimits(ctx context.Context, actorID, targetUserID string, roles []RoleWithPermissions) (assignable, revocable []string, err error) {
	assignable = make([]string, 0, len(roles))
	revocable = make([]string, 0, len(roles))
	for _, role := range roles {
		if role.Name == RoleEveryone {
			continue
		}
		canAssign, err := c.CanAssignRole(ctx, actorID, role.Name)
		if err != nil {
			return nil, nil, err
		}
		if canAssign {
			assignable = append(assignable, role.Name)
		}
		canRevoke, err := c.CanRevokeRoleFromUser(ctx, actorID, targetUserID, role.Name)
		if err != nil {
			return nil, nil, err
		}
		if canRevoke {
			revocable = append(revocable, role.Name)
		}
	}
	return assignable, revocable, nil
}

// ReadAdminUsers hydrates only requested account records. Missing users are
// omitted and duplicates preserve first-seen order. No catalogues are loaded.
func (c *ChattoCore) ReadAdminUsers(ctx context.Context, actorID string, userIDs []string) ([]AdminMember, error) {
	ctx = WithDEKRequestCache(ctx)
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
	return users, nil
}

// BatchGetAdminMembers retains role catalogue output for compatibility clients.
func (c *ChattoCore) BatchGetAdminMembers(ctx context.Context, actorID string, userIDs []string) (*AdminMemberList, error) {
	users, err := c.ReadAdminUsers(ctx, actorID, userIDs)
	if err != nil {
		return nil, err
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
	if isProtectedSelfRoleRevocation(actorID, targetUserID, roleName) {
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

func (c *ChattoCore) adminMemberForViewer(ctx context.Context, actorID string, user *readmodel.User, roles []string) (*AdminMember, error) {
	avatarURL, err := c.GetUserAvatarURL(ctx, user.GetId(), nil, nil, "")
	if err != nil {
		return nil, err
	}

	member := &AdminMember{
		ID:             user.GetId(),
		Login:          user.GetLogin(),
		DisplayName:    user.GetDisplayName(),
		Bio:            user.GetBio(),
		AvatarURL:      avatarURL,
		Roles:          roles,
		CreatedAt:      user.GetCreatedAt(),
		Deleted:        user.GetDeleted(),
		Deactivated:    user.GetDeactivated(),
		IsBot:          user.GetIsBot(),
		BotOwnerUserID: user.GetBotOwnerUserId(),
		CustomStatus:   user.GetCustomStatus(),
	}
	settings, err := c.GetUserSettings(ctx, user.GetId())
	if err != nil {
		return nil, err
	}
	if settings.GetShareTimezone() {
		member.SharedTimezone = settings.GetTimezone()
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
			if email.Primary {
				member.PrimaryVerifiedEmail = email.Email
			}
		}
	}

	if actorID != user.GetId() {
		canChangeDeactivation, err := c.CanChangeUserDeactivation(ctx, actorID, user.GetId())
		if err != nil {
			return nil, err
		}
		member.ViewerCanChangeDeactivation = canChangeDeactivation
		viewerCanDeleteAccount, err := c.CanDeleteUser(ctx, actorID, user.GetId())
		if err != nil {
			return nil, err
		}
		member.ViewerCanDeleteAccount = viewerCanDeleteAccount
	}

	if canViewLastLoginChange, err := c.canViewAdminMemberLastLoginChange(ctx, actorID, user.GetId()); err != nil {
		return nil, err
	} else if canViewLastLoginChange {
		member.UsernameCooldownVisible = true
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
	return c.CanManageUserAccounts(ctx, actorID)
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
