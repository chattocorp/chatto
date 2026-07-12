package core

import (
	"context"
	"sort"
	"strings"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// SyncOIDCRoleClaims synchronizes durable role sources for one verified OIDC
// identity. Claim values are never persisted; only accepted role names and the
// configured provider ID become RBAC facts.
//
// A disabled role claim removes this provider's sources on the next successful
// login. A missing or malformed enabled claim leaves sources intact.
func (c *ChattoCore) SyncOIDCRoleClaims(ctx context.Context, userID string, provider config.AuthProviderConfig, claimPresent bool, claimRoles []string) error {
	providerID := strings.TrimSpace(provider.ID)
	if providerID == "" || provider.Type != config.AuthProviderTypeOpenIDConnect {
		return nil
	}
	enabled := strings.TrimSpace(provider.RoleClaim) != ""
	if enabled && !claimPresent {
		return nil
	}

	allowed := make(map[string]struct{}, len(provider.RoleClaimAllowedRoles))
	wildcard := false
	for _, roleName := range provider.RoleClaimAllowedRoles {
		roleName = strings.TrimSpace(roleName)
		if roleName == "*" {
			wildcard = true
			continue
		}
		allowed[roleName] = struct{}{}
	}
	desired := make(map[string]struct{})
	if enabled {
		for _, roleName := range claimRoles {
			roleName = strings.TrimSpace(roleName)
			if roleName == "" || roleName == RoleEveryone {
				continue
			}
			if wildcard {
				desired[roleName] = struct{}{}
				continue
			}
			if _, ok := allowed[roleName]; ok {
				desired[roleName] = struct{}{}
			}
		}
	}

	_, err := c.appendRBACBatchWithUserCheck(ctx, userID, func() ([]events.BatchEntry, error) {
		return c.oidcRoleClaimSyncEntries(userID, providerID, provider.OIDCRoleClaimModeOrDefault(), enabled, desired), nil
	})
	return err
}

func (c *ChattoCore) oidcRoleClaimSyncEntries(userID, providerID, mode string, enabled bool, desired map[string]struct{}) []events.BatchEntry {
	current := c.RBAC.OIDCRolesForProvider(userID, providerID)
	entries := make([]events.BatchEntry, 0, len(desired)+len(current))
	desiredRoles := make([]string, 0, len(desired))
	for roleName := range desired {
		desiredRoles = append(desiredRoles, roleName)
	}
	sort.Strings(desiredRoles)
	for _, roleName := range desiredRoles {
		if !c.RBAC.RoleExists(roleName) {
			continue
		}
		found := false
		for _, existing := range current {
			if existing == roleName {
				found = true
				break
			}
		}
		if found {
			continue
		}
		event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleGranted{
			RbacOidcRoleGranted: &corev1.RbacOIDCRoleGrantedEvent{UserId: userID, RoleName: roleName, ProviderId: providerID},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}
	if !enabled || mode == config.OIDCRoleClaimModeReconcile {
		for _, roleName := range current {
			if enabled {
				if _, wanted := desired[roleName]; wanted && c.RBAC.RoleExists(roleName) {
					continue
				}
			}
			event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleRevoked{
				RbacOidcRoleRevoked: &corev1.RbacOIDCRoleRevokedEvent{UserId: userID, RoleName: roleName, ProviderId: providerID},
			}})
			entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
		}
	}
	return entries
}
