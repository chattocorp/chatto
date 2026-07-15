package core

import (
	"context"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// reconcileOIDCRoleSources removes OIDC role sources whose provider ID no
// longer names the same configured issuer. This runs at boot after RBAC has
// replayed, so an operator cannot repoint a provider ID and retain authority
// granted by its previous identity provider.
func (c *ChattoCore) reconcileOIDCRoleSources(ctx context.Context) error {
	issuersByProviderID := make(map[string]string, len(c.config.AuthProviders))
	for _, provider := range c.config.AuthProviders {
		if provider.Type != config.AuthProviderTypeOpenIDConnect {
			continue
		}
		if issuer := config.CanonicalOIDCIssuer(provider.IssuerURL); issuer != "" {
			issuersByProviderID[provider.ID] = issuer
		}
	}

	_, err := c.appendRBACBatchBuilt(ctx, func() ([]events.BatchEntry, error) {
		entries := make([]events.BatchEntry, 0)
		for _, assignment := range c.RBAC.OIDCRoleAssignments() {
			issuer, configured := issuersByProviderID[assignment.providerID]
			if configured && assignment.issuer == issuer {
				continue
			}
			event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleRevoked{
				RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{
					UserId: assignment.userID, RoleName: assignment.roleName,
					Source:           corev1.RbacRoleAssignmentSource_RBAC_ROLE_ASSIGNMENT_SOURCE_OIDC,
					SourceProviderId: assignment.providerID,
					SourceIssuer:     assignment.issuer,
				},
			}})
			entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
		}
		return entries, nil
	})
	return err
}
