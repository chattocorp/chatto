package core

import (
	"context"
	"slices"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// ProviderGroups is the groups claim of one verified provider sign-in.
// Known is false when the provider sent no readable groups claim; group role
// sync then changes nothing. A known claim can be empty.
type ProviderGroups struct {
	Groups []string
	Known  bool
}

// providerGroupsReason is the change reason for group role sync by providerID.
func providerGroupsReason(providerID string) *evtv1.RbacRoleChangeReason {
	return &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{
		ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: providerID},
	}}
}

// providerRoleRevokedEntry returns a revocation of a provider-owned role.
func providerRoleRevokedEntry(userID, roleName, providerID string) evtstream.BatchEntry {
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleRevoked{
		RbacRoleRevoked: &evtv1.RbacRoleRevokedEvent{UserId: userID, RoleName: roleName, Reason: providerGroupsReason(providerID)},
	}})
	return evtstream.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event}
}

// RevokeProviderGroupRoles revokes every assignment of the user that the auth
// provider providerID owns, whatever its current mapping. Chatto calls it when
// the user may no longer sign in through the provider, or no longer has an
// identity of it, so the user keeps no authority from the provider (FDR-050).
// Admin and configuration assignments stay.
func (c *ChattoCore) RevokeProviderGroupRoles(ctx context.Context, userID, providerID string) error {
	_, err := c.appendComputedRBACBatch(ctx, func() ([]evtstream.BatchEntry, error) {
		var entries []evtstream.BatchEntry
		for _, roleName := range c.rbacModel.providerAssignments(userID, providerID) {
			entries = append(entries, providerRoleRevokedEntry(userID, roleName, providerID))
		}
		return entries, nil
	})
	return err
}

// SyncProviderGroupRoles applies mappings, the group_roles mapping of the
// provider providerID, to a user after a verified sign-in, identity link, or account creation through
// that provider (FDR-050). It assigns each mapped role that the user's groups
// give and that the user does not have. It revokes a mapped role only when the
// user is in no matching group and this provider owns the assignment.
// Admin and configuration assignments are never changed.
//
// The configuration authorizes these changes, so they use the system actor and
// skip the delegated role-assignment limits. All changes commit in one RBAC
// batch with OCC. A conflict recomputes the changes from the current RBAC
// state, so concurrent syncs and admin changes do not lose updates.
func (c *ChattoCore) SyncProviderGroupRoles(ctx context.Context, userID, providerID string, mappings []config.GroupRole, groups ProviderGroups) error {
	if !groups.Known || len(mappings) == 0 {
		return nil
	}

	// A role is wanted when the user is in at least one of its groups.
	wanted := make(map[string]bool)
	for _, mapping := range mappings {
		wanted[mapping.Role] = wanted[mapping.Role] || slices.Contains(groups.Groups, mapping.Group)
	}
	roleNames := sortedMapKeys(wanted)
	reason := providerGroupsReason(providerID)

	var missingRoles []string
	if _, err := c.appendComputedRBACBatch(ctx, func() ([]evtstream.BatchEntry, error) {
		if err := c.requireHumanUser(ctx, userID); err != nil {
			return nil, err
		}
		missingRoles = missingRoles[:0]
		var entries []evtstream.BatchEntry
		for _, roleName := range roleNames {
			if config.IsReservedGroupRole(roleName) {
				continue
			}
			if !c.rbacModel.roleExists(roleName) {
				missingRoles = append(missingRoles, roleName)
				continue
			}
			owner, assigned := c.rbacModel.assignmentProvider(userID, roleName)
			switch {
			case wanted[roleName] && !assigned:
				event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{
					RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: userID, RoleName: roleName, Reason: reason},
				}})
				entries = append(entries, evtstream.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
			case !wanted[roleName] && assigned && owner == providerID:
				entries = append(entries, providerRoleRevokedEntry(userID, roleName, providerID))
			}
		}
		return entries, nil
	}); err != nil {
		return err
	}
	for _, roleName := range missingRoles {
		c.logger.Warn("Provider group role does not exist", "provider_id", providerID, "role", roleName)
	}
	return nil
}
