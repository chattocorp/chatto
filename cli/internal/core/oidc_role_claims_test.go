package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
)

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
		t.Fatalf("manual moderator revoke: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("provider B's moderator grant must survive manual revoke")
	}
	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleModerator); !errors.Is(err, ErrRoleManagedByOIDC) {
		t.Fatalf("revoke OIDC-only role error = %v, want ErrRoleManagedByOIDC", err)
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
}
