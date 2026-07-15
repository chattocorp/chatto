package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func linkOIDCTestIdentity(t *testing.T, chatto *ChattoCore, userID, providerID string) {
	t.Helper()
	if err := chatto.LinkExternalIdentity(t.Context(), providerID, "oidc", "https://"+providerID+".example", userID, userID); err != nil {
		t.Fatalf("LinkExternalIdentity(%q): %v", providerID, err)
	}
}

func oidcTestIssuer(providerID string) string { return "https://" + providerID + ".example" }

func TestChattoCore_SyncOIDCRoleClaimsPreservesIndependentSources(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-role-user", "OIDC Role User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	providerA := config.AuthProviderConfig{
		ID: "oidc-a", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: oidcTestIssuer("oidc-a"),
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleAdmin, RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	providerB := config.AuthProviderConfig{
		ID: "oidc-b", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: oidcTestIssuer("oidc-b"),
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleModerator},
	}
	linkOIDCTestIdentity(t, chatto, user.Id, providerA.ID)
	linkOIDCTestIdentity(t, chatto, user.Id, providerB.ID)
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerA, true, []string{RoleAdmin, "unknown"}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims provider A: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("OIDC provider A should grant admin")
	}
	if err := chatto.AssignServerRoleToExistingUser(ctx, SystemActorID, user.Id, RoleAdmin); err != nil {
		t.Fatalf("manual admin assignment over OIDC source: %v", err)
	}
	if err := chatto.AssignServerRoleToExistingUser(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRoleToExistingUser: %v", err)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, true, []string{RoleModerator}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims provider B: %v", err)
	}

	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerA, true, nil); err != nil {
		t.Fatalf("reconcile provider A empty roles: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("manual admin grant must survive provider A reconciliation")
	}
	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleAdmin); err != nil {
		t.Fatalf("manual admin revoke: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("admin should be gone after its manual source is revoked")
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("manual and provider B moderator grants must survive provider A reconciliation")
	}

	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatalf("operator moderator revoke: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("manual revocation must leave the provider-managed role in place")
	}
	if got := chatto.RBAC.OIDCRolesForProvider(user.Id, providerB.ID); len(got) != 1 || got[0] != RoleModerator {
		t.Fatalf("provider B sources after manual revoke = %v, want moderator", got)
	}
	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleModerator); !errors.Is(err, ErrRoleManagedByIdentityProvider) {
		t.Fatalf("solely OIDC-managed role revoke error = %v, want ErrRoleManagedByIdentityProvider", err)
	}

	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, false, nil); err != nil {
		t.Fatalf("missing configured claim should preserve grants: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("missing claim must preserve OIDC grant")
	}
	providerB.RoleClaim = ""
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, false, nil); err != nil {
		t.Fatalf("disabled role claim cleanup: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("disabled role claim must remove provider-managed grants")
	}
}

func TestChattoCore_SyncOIDCRoleClaimsEmitsSourceTaggedAssignmentEvents(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-shadow", "OIDC Shadow", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc-shadow", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: oidcTestIssuer("oidc-shadow"),
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	linkOIDCTestIdentity(t, chatto, user.Id, provider.ID)
	grantEntries := chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, oidcTestIssuer(provider.ID), config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{RoleModerator: {}})
	if len(grantEntries) != 1 {
		t.Fatalf("grant entries = %d, want one assignment event", len(grantEntries))
	}
	grant := grantEntries[0].Event.GetRbacRoleAssigned()
	if grant == nil || grant.GetSource() != corev1.RbacRoleAssignmentSource_RBAC_ROLE_ASSIGNMENT_SOURCE_OIDC || grant.GetSourceProviderId() != provider.ID {
		t.Fatalf("grant assignment event = %v", grantEntries[0].Event.GetEvent())
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleModerator}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims grant: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) || chatto.RBAC.HasManualRole(user.Id, RoleModerator) {
		t.Fatal("OIDC assignment must not become a manual assignment")
	}
	grantEntries = chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, oidcTestIssuer(provider.ID), config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{RoleModerator: {}})
	if len(grantEntries) != 0 {
		t.Fatalf("idempotent grant entries = %d, want 0", len(grantEntries))
	}

	revokeEntries := chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, oidcTestIssuer(provider.ID), config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{})
	if len(revokeEntries) != 1 {
		t.Fatalf("revoke entries = %d, want one assignment event", len(revokeEntries))
	}
	revoke := revokeEntries[0].Event.GetRbacRoleRevoked()
	if revoke == nil || revoke.GetSource() != corev1.RbacRoleAssignmentSource_RBAC_ROLE_ASSIGNMENT_SOURCE_OIDC || revoke.GetSourceProviderId() != provider.ID {
		t.Fatalf("revoke assignment event = %v", revokeEntries[0].Event.GetEvent())
	}

	projection := NewRBACProjection()
	grantEvent := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleAssigned{
		RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{UserId: user.Id, RoleName: RoleModerator, Source: corev1.RbacRoleAssignmentSource_RBAC_ROLE_ASSIGNMENT_SOURCE_OIDC, SourceProviderId: provider.ID, SourceIssuer: oidcTestIssuer(provider.ID)},
	}})
	if err := projection.Apply(grantEvent, 1); err != nil {
		t.Fatalf("apply OIDC grant: %v", err)
	}
	legacyRevoke := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleRevoked{
		RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{UserId: user.Id, RoleName: RoleModerator},
	}})
	if err := projection.Apply(legacyRevoke, 2); err != nil {
		t.Fatalf("apply legacy revoke: %v", err)
	}
	if projection.HasRole(user.Id, RoleModerator) || len(projection.OIDCRolesForProvider(user.Id, provider.ID)) != 0 {
		t.Fatal("a legacy writer's normal revoke must clear source-aware assignments")
	}
}

func TestChattoCore_SyncOIDCRoleClaimsWildcardIncludesOwner(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-owner", "OIDC Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc-owner", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: oidcTestIssuer("oidc-owner"),
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{"*"},
	}
	linkOIDCTestIdentity(t, chatto, user.Id, provider.ID)
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleOwner, RoleEveryone, "does-not-exist"}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleOwner) {
		t.Fatal("wildcard should allow owner")
	}
	if chatto.RBAC.HasRole(user.Id, RoleEveryone) {
		t.Fatal("implicit everyone must never become an explicit OIDC role")
	}
}

func TestChattoCore_SyncOIDCRoleClaimsDoesNotRestoreDeletedRole(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-deleted-role", "OIDC Deleted Role", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := chatto.CreateServerRole(ctx, SystemActorID, "idp-editor", "IdP editor", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: oidcTestIssuer("oidc"),
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{"idp-editor"}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	linkOIDCTestIdentity(t, chatto, user.Id, provider.ID)
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{"idp-editor"}); err != nil {
		t.Fatalf("initial SyncOIDCRoleClaims: %v", err)
	}
	if err := chatto.DeleteServerRole(ctx, SystemActorID, "idp-editor"); err != nil {
		t.Fatalf("DeleteServerRole: %v", err)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{"idp-editor"}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims after delete: %v", err)
	}
	if got := chatto.RBAC.OIDCRolesForProvider(user.Id, provider.ID); len(got) != 0 {
		t.Fatalf("OIDC roles after deletion = %v, want none", got)
	}
	if _, err := chatto.CreateServerRole(ctx, SystemActorID, "idp-editor", "IdP editor", ""); err != nil {
		t.Fatalf("recreate role: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, "idp-editor") {
		t.Fatal("recreating a deleted role must not restore an old OIDC assignment")
	}
}

func TestChattoCore_ReconcileOIDCRoleSourcesRevokesReplacedIssuer(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "issuer-replacement", "Issuer Replacement", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	oldProvider := config.AuthProviderConfig{
		ID: "company-sso", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: "https://login.old.example/",
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleOwner},
	}
	if err := chatto.LinkExternalIdentity(ctx, oldProvider.ID, oldProvider.Type, "https://login.old.example", "subject", user.Id); err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, oldProvider, true, []string{RoleOwner}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims old issuer: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleOwner) {
		t.Fatal("old issuer should initially grant owner")
	}

	newProvider := oldProvider
	newProvider.IssuerURL = "https://login.new.example"
	chatto.config.AuthProviders = []config.AuthProviderConfig{newProvider}
	if err := chatto.reconcileOIDCRoleSources(ctx); err != nil {
		t.Fatalf("reconcileOIDCRoleSources: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleOwner) {
		t.Fatal("a reused provider ID must not retain roles from its previous issuer")
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, newProvider, true, []string{RoleOwner}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims new issuer: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleOwner) {
		t.Fatal("an old issuer identity must not synchronize roles for the replacement issuer")
	}
}

func TestChattoCore_DisconnectExternalIdentityRevokesOIDCRoleSources(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-disconnect", "OIDC Disconnect", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chatto.LinkExternalIdentity(ctx, "oidc", "oidc", "https://issuer.example", "subject", user.Id); err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc", Type: config.AuthProviderTypeOpenIDConnect,
		IssuerURL: "https://issuer.example",
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleAdmin}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleAdmin}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims: %v", err)
	}
	identities, err := chatto.ExternalIdentitiesForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("ExternalIdentitiesForUser: %v", err)
	}
	if err := chatto.DisconnectExternalIdentity(ctx, user.Id, identities[0].SubjectHash); err != nil {
		t.Fatalf("DisconnectExternalIdentity: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("disconnecting an OIDC identity must revoke its managed role sources")
	}
	if got := chatto.RBAC.OIDCRolesForProvider(user.Id, provider.ID); len(got) != 0 {
		t.Fatalf("OIDC roles after disconnect = %v, want none", got)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleAdmin}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims after disconnect: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("a callback that completes after disconnect must not recreate its OIDC role source")
	}
}
