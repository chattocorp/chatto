package core

import (
	"context"
	"slices"
	"time"

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

// ProviderRoleSource identifies the auth provider of a group role change.
// Issuer is the identity namespace of the provider's links (the verified OIDC
// issuer URL) and owns the assignments. ProviderID is the configured provider
// ID, recorded for the audit log only, because it can change. Subject is the
// provider subject of the verified identity; group role sync uses it to
// complete legacy links that have no issuer.
type ProviderRoleSource struct {
	ProviderID string
	Issuer     string
	Subject    string
}

func (s ProviderRoleSource) reason() *evtv1.RbacRoleChangeReason {
	return &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{
		ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: s.ProviderID, Issuer: s.Issuer},
	}}
}

// revokedEntry returns a revocation of a role assignment that source owns.
func (s ProviderRoleSource) revokedEntry(userID, roleName string) evtstream.BatchEntry {
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleRevoked{
		RbacRoleRevoked: &evtv1.RbacRoleRevokedEvent{UserId: userID, RoleName: roleName, Reason: s.reason()},
	}})
	return evtstream.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event}
}

// RevokeProviderGroupRoles revokes every assignment of the user that the
// provider with source.Issuer owns, whatever its current mapping. Chatto calls
// it when the user may no longer sign in through the provider, or no longer
// has an identity of it, so the user keeps no authority from the provider
// (FDR-050). Admin and configuration assignments stay.
func (c *ChattoCore) RevokeProviderGroupRoles(ctx context.Context, userID string, source ProviderRoleSource) error {
	if source.Issuer == "" {
		return nil
	}
	_, err := c.appendComputedRBACBatch(ctx, func() ([]evtstream.BatchEntry, error) {
		var entries []evtstream.BatchEntry
		for _, roleName := range c.rbacModel.providerAssignments(userID, source.Issuer) {
			entries = append(entries, source.revokedEntry(userID, roleName))
		}
		return entries, nil
	})
	return err
}

// SyncProviderGroupRoles applies mappings, the group_roles mapping of the
// provider source, to a user after a verified sign-in, identity link, or
// account creation through that provider (FDR-050). It assigns each mapped
// role that the user's groups give and that the user does not have. It
// revokes a mapped role only when the user is in no matching group and the
// provider owns the assignment. Admin and configuration assignments are never
// changed.
//
// The configuration authorizes these changes, so they use the system actor and
// skip the delegated role-assignment limits. All changes commit in one RBAC
// batch with OCC. A conflict recomputes the changes from the current RBAC
// state, so concurrent syncs and admin changes do not lose updates.
//
// Provider assignments must exist only while the user has an identity of the
// provider. A concurrent unlink can commit before this sync, so the sync
// checks the links again after a grant and revokes when none remains.
func (c *ChattoCore) SyncProviderGroupRoles(ctx context.Context, userID string, source ProviderRoleSource, mappings []config.GroupRole, groups ProviderGroups) error {
	if !groups.Known || len(mappings) == 0 || source.Issuer == "" {
		return nil
	}
	if source.Subject != "" {
		if err := c.CompleteLegacyOIDCIdentityLink(ctx, userID, source.ProviderID, source.Issuer, source.Subject); err != nil {
			return err
		}
	}

	// A role is wanted when the user is in at least one of its groups.
	wanted := make(map[string]bool)
	for _, mapping := range mappings {
		wanted[mapping.Role] = wanted[mapping.Role] || slices.Contains(groups.Groups, mapping.Group)
	}
	roleNames := sortedMapKeys(wanted)

	var missingRoles []string
	granted := false
	_, err := c.appendComputedRBACBatch(ctx, func() ([]evtstream.BatchEntry, error) {
		if err := c.requireHumanUser(ctx, userID); err != nil {
			return nil, err
		}
		missingRoles = missingRoles[:0]
		granted = false
		var entries []evtstream.BatchEntry
		for _, roleName := range roleNames {
			if config.IsReservedGroupRole(roleName) {
				continue
			}
			if !c.rbacModel.roleExists(roleName) {
				missingRoles = append(missingRoles, roleName)
				continue
			}
			owner, assigned := c.rbacModel.assignmentOwner(userID, roleName)
			switch {
			case wanted[roleName] && !assigned:
				event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{
					RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: userID, RoleName: roleName, Reason: source.reason()},
				}})
				entries = append(entries, evtstream.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
				granted = true
			case !wanted[roleName] && assigned && owner == source.Issuer:
				entries = append(entries, source.revokedEntry(userID, roleName))
			}
		}
		return entries, nil
	})
	// A grant can commit although the append reports an error, for example
	// when the request ends during the projection wait after the commit. The
	// check is harmless when nothing committed, so it runs in both cases.
	if granted {
		if checkErr := c.revokeProviderGroupRolesWithoutIdentity(ctx, userID, source); err == nil {
			err = checkErr
		}
	}
	if err != nil {
		return err
	}
	for _, roleName := range missingRoles {
		c.logger.Warn("Provider group role does not exist", "provider_id", source.ProviderID, "role", roleName)
	}
	return nil
}

// providerRoleRecheckTimeout bounds the identity check after a group role
// sync or an unlink commits.
const providerRoleRecheckTimeout = 10 * time.Second

// revokeProviderGroupRolesWithoutIdentity revokes the provider's assignments
// when the user has no identity with the provider's issuer. Group role sync
// and identity unlinks both call it after they commit. The second of two
// concurrent writes therefore sees the effect of the first. The check runs to
// its end also when the request is cancelled, because the write it checks is
// already committed.
func (c *ChattoCore) revokeProviderGroupRolesWithoutIdentity(ctx context.Context, userID string, source ProviderRoleSource) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerRoleRecheckTimeout)
	defer cancel()
	if err := c.userModel.waitForUserAuthCurrent(ctx, "provider group roles"); err != nil {
		return err
	}
	if slices.ContainsFunc(c.userModel.externalIdentities(userID), func(identity ExternalIdentity) bool {
		return identity.Issuer == source.Issuer
	}) {
		return nil
	}
	return c.RevokeProviderGroupRoles(ctx, userID, source)
}
