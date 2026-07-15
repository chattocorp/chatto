package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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

	_, err := c.appendOIDCRoleSyncBatch(ctx, userID, providerID, func() ([]events.BatchEntry, error) {
		// This check intentionally runs inside the user + RBAC OCC retry loop.
		// A callback may have resolved an identity just before it is disconnected;
		// it must not recreate that provider's source after the unlink batch.
		providerLinked := false
		for _, identity := range c.Users.ExternalIdentities(userID) {
			if identity.ProviderID == providerID {
				providerLinked = true
				break
			}
		}
		return c.oidcRoleClaimSyncEntries(userID, providerID, provider.OIDCRoleClaimModeOrDefault(), enabled && providerLinked, desired), nil
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
	if enabled {
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
			providers := c.RBAC.OIDCProvidersForRole(userID, roleName)
			event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleGranted{
				RbacOidcRoleGranted: &corev1.RbacOIDCRoleGrantedEvent{UserId: userID, RoleName: roleName, ProviderId: providerID},
			}})
			entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
			if !c.RBAC.HasManualRole(userID, roleName) && len(providers) == 0 {
				entries = append(entries, compatibilityRoleAssignedEntry(SystemActorID, userID, roleName))
			}
		}
	}
	if !enabled || mode == config.OIDCRoleClaimModeReconcile {
		for _, roleName := range current {
			if enabled {
				if _, wanted := desired[roleName]; wanted && c.RBAC.RoleExists(roleName) {
					continue
				}
			}
			providers := c.RBAC.OIDCProvidersForRole(userID, roleName)
			event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleRevoked{
				RbacOidcRoleRevoked: &corev1.RbacOIDCRoleRevokedEvent{UserId: userID, RoleName: roleName, ProviderId: providerID},
			}})
			entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
			if !c.RBAC.HasManualRole(userID, roleName) && len(providers) == 1 {
				entries = append(entries, compatibilityRoleRevokedEntry(SystemActorID, userID, roleName))
			}
		}
	}
	return entries
}

// appendOIDCRoleSyncBatch protects only the target user and RBAC aggregate
// tails. The user-scoped synchronization fact supplies the second atomic OCC
// guard without making unrelated EVT traffic contend with interactive login.
func (c *ChattoCore) appendOIDCRoleSyncBatch(ctx context.Context, userID, providerID string, build func() ([]events.BatchEntry, error)) (uint64, error) {
	userFilter := events.UserAggregate(userID).AllEventsFilter()
	rbacFilter := events.RBACSubjectFilter()
	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		userSeq, err := c.EventPublisher.LastSubjectSeq(ctx, userFilter)
		if err != nil {
			return 0, fmt.Errorf("read OIDC role user OCC seq: %w", err)
		}
		if err := c.userModel.waitForUsersCurrent(ctx, "OIDC role target user", userFilter); err != nil {
			return 0, err
		}
		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, rbacFilter)
		if err != nil {
			return 0, fmt.Errorf("read OIDC role RBAC OCC seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(rbacFilter, rbacSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if _, err := c.GetUser(ctx, userID); err != nil {
			return 0, err
		}
		roleEntries, err := build()
		if err != nil {
			return 0, err
		}
		if len(roleEntries) == 0 {
			return 0, nil
		}

		syncEvent := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_UserOidcRolesSynchronized{
			UserOidcRolesSynchronized: &corev1.UserOIDCRolesSynchronizedEvent{UserId: userID, ProviderId: providerID},
		}})
		userSubject := events.UserAggregate(userID).SubjectFor(syncEvent)
		entries := make([]events.BatchEntry, 0, len(roleEntries)+1)
		entries = append(entries, events.BatchEntry{
			Subject: userSubject, Event: syncEvent, HasOCC: true,
			ExpectedSeq: userSeq, FilterSubject: userFilter,
		})
		roleEntries[0].HasOCC = true
		roleEntries[0].ExpectedSeq = rbacSeq
		roleEntries[0].FilterSubject = rbacFilter
		entries = append(entries, roleEntries...)
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(userSubject, seqs[0])); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			last := len(entries) - 1
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
				return 0, fmt.Errorf("wait for RBAC projection: %w", err)
			}
			return seqs[last], nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("OIDC role sync OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}

// ReconcileConfiguredOIDCRoleSources revokes sources whose provider was
// removed or whose role claim was disabled. It runs after projection replay so
// disabling a compromised provider is a fail-closed operator escape hatch.
func (c *ChattoCore) ReconcileConfiguredOIDCRoleSources(ctx context.Context) error {
	enabledProviders := make(map[string]struct{})
	for _, provider := range c.config.AuthProviders {
		if provider.Type == config.AuthProviderTypeOpenIDConnect && strings.TrimSpace(provider.RoleClaim) != "" {
			enabledProviders[strings.TrimSpace(provider.ID)] = struct{}{}
		}
	}
	type providerUser struct{ providerID, userID string }
	stale := make(map[providerUser]struct{})
	for _, assignment := range c.RBAC.OIDCRoleAssignments() {
		if _, enabled := enabledProviders[assignment.providerID]; !enabled {
			stale[providerUser{providerID: assignment.providerID, userID: assignment.userID}] = struct{}{}
		}
	}
	pairs := make([]providerUser, 0, len(stale))
	for pair := range stale {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].providerID != pairs[j].providerID {
			return pairs[i].providerID < pairs[j].providerID
		}
		return pairs[i].userID < pairs[j].userID
	})
	for _, pair := range pairs {
		disabled := config.AuthProviderConfig{ID: pair.providerID, Type: config.AuthProviderTypeOpenIDConnect}
		_, userExists, err := c.Users.GetContext(ctx, pair.userID)
		if err != nil {
			return fmt.Errorf("read disabled OIDC provider %q source user %q: %w", pair.providerID, pair.userID, err)
		}
		if !userExists {
			if _, err := c.appendRBACBatchBuilt(ctx, func() ([]events.BatchEntry, error) {
				return c.oidcRoleClaimSyncEntries(pair.userID, pair.providerID, config.OIDCRoleClaimModeReconcile, false, nil), nil
			}); err != nil {
				return fmt.Errorf("revoke disabled OIDC provider %q sources for deleted user %q: %w", pair.providerID, pair.userID, err)
			}
			continue
		}
		if err := c.SyncOIDCRoleClaims(ctx, pair.userID, disabled, false, nil); err != nil {
			return fmt.Errorf("revoke disabled OIDC provider %q role sources for user %q: %w", pair.providerID, pair.userID, err)
		}
	}
	return nil
}
