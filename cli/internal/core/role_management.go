package core

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

type RoleCatalog struct {
	Roles                []RoleWithPermissions
	ViewerCanManageRoles bool
	ViewerCanAssignRoles bool
}

type RoleDetails struct {
	Role                 *RoleWithPermissions
	ViewerCanManageRoles bool
	ViewerCanAssignRoles bool
}

type AdminRoleInput struct {
	RequiresShieldMode bool
	Name               string
	DisplayName        string
	Description        string
	Pingable           *bool
}

type AdminRoleUpdateInput struct {
	RequiresShieldMode *bool
	Name               string
	DisplayName        *string
	Description        *string
	Pingable           *bool
}

func (c *ChattoCore) ListServerRolesForUser(ctx context.Context, actorID string) (*RoleCatalog, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	canAssign, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return &RoleCatalog{
		Roles:                roles,
		ViewerCanManageRoles: canManage,
		ViewerCanAssignRoles: canAssign,
	}, nil
}

// AdminListServerRoles adds aggregate counts only for managers and assigners.
// Ordinary public role catalog reads retain their lightweight response.
func (c *ChattoCore) AdminListServerRoles(ctx context.Context, actorID string) (*RoleCatalog, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	if err := c.waitForCurrentRoleCatalogState(ctx); err != nil {
		return nil, err
	}
	var result *RoleCatalog
	err := c.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		catalog, err := c.ListServerRolesForUser(readCtx, actorID)
		if err != nil {
			return err
		}
		if catalog.ViewerCanManageRoles || catalog.ViewerCanAssignRoles {
			counts := c.rbacModel.roleCounts(c.userModel.activeHumanIDs())
			for i := range catalog.Roles {
				count := counts[catalog.Roles[i].Name]
				catalog.Roles[i].Counts = &count
			}
		}
		result = catalog
		return nil
	})
	return result, err
}

// waitForCurrentRoleCatalogState includes account lifecycle facts as well as
// RBAC facts, so a count refresh on another replica cannot miss new members.
func (c *ChattoCore) waitForCurrentRoleCatalogState(ctx context.Context) error {
	if active, ok := ctx.Value(serverContentViewReadContextKey{}).(serverContentViewReadContext); ok && active.view == c.contentView {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, c.EventPublisher, "role catalog", c.contentView.projector,
		evtstream.RBACSubjectFilter(), evtstream.UserSubjectFilter())
}

func (c *ChattoCore) GetServerRoleDetails(ctx context.Context, actorID, roleName string) (*RoleDetails, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	if roleName == "" {
		return nil, fmt.Errorf("%w: role name is required", ErrInvalidArgument)
	}
	role, err := c.GetServerRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	canAssign, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	details := &RoleDetails{
		Role:                 role,
		ViewerCanManageRoles: canManage,
		ViewerCanAssignRoles: canAssign,
	}
	return details, nil
}

func (c *ChattoCore) AdminCreateServerRole(ctx context.Context, actorID string, input AdminRoleInput) (*RoleWithPermissions, error) {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	pingable := false
	if input.Pingable != nil {
		pingable = *input.Pingable
	}
	return c.createServerRole(ctx, actorID, input.Name, input.DisplayName, input.Description, pingable, input.RequiresShieldMode, func() error { return c.requireCanManageAdminRoles(ctx, actorID) })
}

func (c *ChattoCore) AdminUpdateServerRole(ctx context.Context, actorID string, input AdminRoleUpdateInput) (*RoleWithPermissions, error) {
	return c.updateServerRole(ctx, actorID, input, func() error { return c.requireCanManageAdminRoles(ctx, actorID) })
}

// updateServerRole applies the sparse edit as one batch. Rebuild the changes
// and check the actor's authority again after a concurrent RBAC edit.
func (c *ChattoCore) updateServerRole(ctx context.Context, actorID string, input AdminRoleUpdateInput, authorize func() error) (*RoleWithPermissions, error) {
	name := input.Name
	_, err := c.appendComputedRBACBatch(ctx, func() ([]evtstream.BatchEntry, error) {
		if authorize != nil {
			if err := c.authorizeAtStableInputs(ctx, authorize); err != nil {
				return nil, err
			}
		}
		if input.DisplayName == nil && input.Description == nil && input.Pingable == nil && input.RequiresShieldMode == nil {
			return nil, fmt.Errorf("%w: provide at least one role field to update", ErrInvalidArgument)
		}
		existing, ok := c.rbacModel.role(name)
		if !ok {
			return nil, ErrRoleNotFound
		}
		display, description := existing.DisplayName, existing.Description
		if input.DisplayName != nil {
			display = *input.DisplayName
		}
		if input.Description != nil {
			description = *input.Description
		}
		if err := validateRoleMetadata(display, description); err != nil {
			return nil, err
		}
		var entries []evtstream.BatchEntry
		add := func(e *evtv1.Event) {
			e = newEvent(actorID, e)
			entries = append(entries, evtstream.BatchEntry{Subject: rbacSubjectForEvent(e), Event: e})
		}
		if display != existing.DisplayName {
			add(&evtv1.Event{Event: &evtv1.Event_RbacRoleDisplayNameChanged{
				RbacRoleDisplayNameChanged: &evtv1.RbacRoleDisplayNameChangedEvent{RoleName: name, DisplayName: display},
			}})
		}
		if description != existing.Description {
			add(&evtv1.Event{Event: &evtv1.Event_RbacRoleDescriptionChanged{
				RbacRoleDescriptionChanged: &evtv1.RbacRoleDescriptionChangedEvent{RoleName: name, Description: description},
			}})
		}
		if input.Pingable != nil && *input.Pingable != existing.Pingable {
			add(&evtv1.Event{Event: &evtv1.Event_RbacRolePingableChanged{
				RbacRolePingableChanged: &evtv1.RbacRolePingableChangedEvent{RoleName: name, Pingable: *input.Pingable},
			}})
		}
		if input.RequiresShieldMode != nil && *input.RequiresShieldMode != existing.RequiresShieldMode {
			if name == RoleOwner || name == RoleEveryone {
				return nil, fmt.Errorf("%w: fixed role protection", ErrInvalidArgument)
			}
			add(&evtv1.Event{Event: &evtv1.Event_RbacRoleProtectionChanged{
				RbacRoleProtectionChanged: &evtv1.RbacRoleProtectionChangedEvent{RoleName: name, RequiresShieldMode: *input.RequiresShieldMode},
			}})
		}
		return entries, nil
	})
	if err != nil {
		return nil, err
	}
	return c.GetServerRole(ctx, name)
}

func (c *ChattoCore) AdminDeleteServerRole(ctx context.Context, actorID, roleName string) error {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return err
	}
	if roleName == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalidArgument)
	}
	return c.DeleteServerRole(ctx, actorID, roleName)
}

func (c *ChattoCore) AdminReorderServerRoles(ctx context.Context, actorID string, roleNames []string) ([]RoleWithPermissions, error) {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	if roleNames == nil {
		roleNames = []string{}
	}
	return c.ReorderServerRoles(ctx, actorID, roleNames)
}

func (c *ChattoCore) requireCanManageAdminRoles(ctx context.Context, actorID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if !canManage {
		return ErrPermissionDenied
	}
	return nil
}

// RoleMemberPage selects assignments before any user or profile hydration.
// IDs are sorted, but separate requests do not share a fixed snapshot.
type RoleMemberPage struct {
	UserIDs    []string
	TotalCount int
	HasMore    bool
}

// ListServerRoleMembers gates every page with role.assign. The implicit
// everyone role has no explicit assignments. Limits default to 20 and cap at 100.
func (c *ChattoCore) ListServerRoleMembers(ctx context.Context, actorID, roleName string, limit, offset int) (*RoleMemberPage, error) {
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
	if roleName == "" || limit < 0 || offset < 0 {
		return nil, fmt.Errorf("%w: invalid role member page", ErrInvalidArgument)
	}
	if limit == 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ids, err := c.GetRoleUsers(ctx, roleName)
	if err != nil {
		return nil, err
	}
	result := &RoleMemberPage{TotalCount: len(ids)}
	if offset >= len(ids) {
		return result, nil
	}
	end := offset + min(limit, len(ids)-offset)
	result.UserIDs = ids[offset:end]
	result.HasMore = end < len(ids)
	return result, nil
}
