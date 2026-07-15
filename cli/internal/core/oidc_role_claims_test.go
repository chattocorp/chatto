package core

import (
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

func TestChattoCore_SyncOIDCRoleClaimsPreservesIndependentSources(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-role-user", "OIDC Role User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	providerA := config.AuthProviderConfig{
		ID: "oidc-a", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleAdmin, RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	providerB := config.AuthProviderConfig{
		ID: "oidc-b", Type: config.AuthProviderTypeOpenIDConnect,
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
	if chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("operator revocation must remove manual and OIDC role sources")
	}
	if got := chatto.RBAC.OIDCRolesForProvider(user.Id, providerB.ID); len(got) != 0 {
		t.Fatalf("provider B sources after operator revoke = %v, want none", got)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, true, []string{RoleModerator}); err != nil {
		t.Fatalf("provider B regrant after operator revoke: %v", err)
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

func TestChattoCore_SyncOIDCRoleClaimsEmitsCompatibilityShadows(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-shadow", "OIDC Shadow", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc-shadow", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	linkOIDCTestIdentity(t, chatto, user.Id, provider.ID)
	grantEntries := chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{RoleModerator: {}})
	if len(grantEntries) != 2 {
		t.Fatalf("grant entries = %d, want source event plus compatibility shadow", len(grantEntries))
	}
	grantShadow := grantEntries[1].Event.GetRbacRoleAssigned()
	if grantShadow == nil || !grantShadow.GetCompatibilityShadow() {
		t.Fatalf("grant compatibility event = %v", grantEntries[1].Event.GetEvent())
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleModerator}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims grant: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) || chatto.RBAC.HasManualRole(user.Id, RoleModerator) {
		t.Fatal("current projection must keep the compatibility shadow out of manual role sources")
	}
	grantEntries = chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{RoleModerator: {}})
	if len(grantEntries) != 0 {
		t.Fatalf("idempotent grant entries = %d, want 0", len(grantEntries))
	}

	revokeEntries := chatto.oidcRoleClaimSyncEntries(user.Id, provider.ID, config.OIDCRoleClaimModeReconcile, true, map[string]struct{}{})
	if len(revokeEntries) != 2 {
		t.Fatalf("revoke entries = %d, want source event plus compatibility shadow", len(revokeEntries))
	}
	shadow := revokeEntries[1].Event.GetRbacRoleRevoked()
	if shadow == nil || !shadow.GetCompatibilityShadow() {
		t.Fatalf("revoke compatibility event = %v", revokeEntries[1].Event.GetEvent())
	}

	projection := NewRBACProjection()
	grant := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleGranted{
		RbacOidcRoleGranted: &corev1.RbacOIDCRoleGrantedEvent{UserId: user.Id, RoleName: RoleModerator, ProviderId: provider.ID},
	}})
	if err := projection.Apply(grant, 1); err != nil {
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

func TestChattoCore_ReconcileConfiguredOIDCRoleSourcesRevokesDisabledProvider(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-disabled", "OIDC Disabled", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc-disabled", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleAdmin},
	}
	linkOIDCTestIdentity(t, chatto, user.Id, provider.ID)
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleAdmin}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims: %v", err)
	}
	chatto.config.AuthProviders = []config.AuthProviderConfig{provider}
	if err := chatto.ReconcileConfiguredOIDCRoleSources(ctx); err != nil {
		t.Fatalf("ReconcileConfiguredOIDCRoleSources enabled: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("enabled provider source must survive startup reconciliation")
	}
	chatto.config.AuthProviders = nil
	if err := chatto.ReconcileConfiguredOIDCRoleSources(ctx); err != nil {
		t.Fatalf("ReconcileConfiguredOIDCRoleSources: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("disabled provider sources must be revoked")
	}

	ghostID := "deleted-oidc-user"
	ghostGrant := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacOidcRoleGranted{
		RbacOidcRoleGranted: &corev1.RbacOIDCRoleGrantedEvent{UserId: ghostID, RoleName: RoleAdmin, ProviderId: provider.ID},
	}})
	if _, err := chatto.appendRBACEvent(ctx, ghostGrant, nil); err != nil {
		t.Fatalf("append stale deleted-user source: %v", err)
	}
	if err := chatto.ReconcileConfiguredOIDCRoleSources(ctx); err != nil {
		t.Fatalf("ReconcileConfiguredOIDCRoleSources deleted user: %v", err)
	}
	if got := chatto.RBAC.OIDCRolesForProvider(ghostID, provider.ID); len(got) != 0 {
		t.Fatalf("deleted-user OIDC sources after reconciliation = %v, want none", got)
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
