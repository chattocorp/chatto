package core

import (
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/config"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// testIssuers are the identity issuers of the test providers.
var testIssuers = map[string]string{"company": "https://id.example", "other": "https://other.example"}

// testGroupRoles maps the groups of two test providers to roles. The owner
// pair and the unknown role exercise the defensive skips.
var testGroupRoles = map[string]string{
	"company": "chat-mods:moderator helpers:helper it-ops:helper ghosts:missing-role admins:owner",
	"other":   "other-mods:other-role",
}

// setupGroupRolesCore returns a core with the custom role helper.
func setupGroupRolesCore(t *testing.T) *ChattoCore {
	t.Helper()
	core, _ := setupTestCore(t)
	if _, err := core.CreateServerRole(testContext(t), SystemActorID, "helper", "Helper", ""); err != nil {
		t.Fatal(err)
	}
	return core
}

// newGroupMember creates a user with an identity of each test provider.
func newGroupMember(t *testing.T, core *ChattoCore, login string) *readmodel.User {
	t.Helper()
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, login, login, "password123")
	if err != nil {
		t.Fatal(err)
	}
	for providerID, issuer := range testIssuers {
		if err := core.LinkExternalIdentity(ctx, providerID, config.AuthProviderTypeOpenIDConnect, issuer, login, user.Id); err != nil {
			t.Fatal(err)
		}
	}
	return user
}

func syncGroups(t *testing.T, core *ChattoCore, userID, providerID string, groups ...string) {
	t.Helper()
	syncGroupsWith(t, core, userID, providerID, testGroupRoles[providerID], groups...)
}

func syncGroupsWith(t *testing.T, core *ChattoCore, userID, providerID, groupRoles string, groups ...string) {
	t.Helper()
	mappings, err := config.ParseGroupRoles(groupRoles)
	if err != nil {
		t.Fatal(err)
	}
	source := ProviderRoleSource{ProviderID: providerID, Issuer: testIssuers[providerID]}
	if err := core.SyncProviderGroupRoles(testContext(t), userID, source, mappings, ProviderGroups{Groups: groups, Known: true}); err != nil {
		t.Fatalf("SyncProviderGroupRoles(%v): %v", groups, err)
	}
}

func requireRoles(t *testing.T, core *ChattoCore, userID string, want map[string]bool) {
	t.Helper()
	for roleName, assigned := range want {
		if got := core.rbacModel.hasRole(userID, roleName); got != assigned {
			t.Fatalf("hasRole(%s) = %v, want %v", roleName, got, assigned)
		}
	}
}

func TestSyncProviderGroupRolesGrantsAndRevokesProviderAssignments(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "group-member")

	syncGroups(t, core, user.Id, "company", "chat-mods", "it-ops", "admins")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true, "helper": true, RoleOwner: false})
	if owner, _ := core.rbacModel.assignmentOwner(user.Id, RoleModerator); owner != testIssuers["company"] {
		t.Fatalf("moderator owner = %q, want company issuer", owner)
	}

	// helper stays while one of its groups matches.
	syncGroups(t, core, user.Id, "company", "helpers")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false, "helper": true})

	// A known, empty claim removes every provider assignment.
	syncGroups(t, core, user.Id, "company")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false, "helper": false})

	assignment := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{}}})
	assigned, _, err := core.EventPublisher.SubjectEvents(ctx, rbacSubjectForEvent(assignment))
	if err != nil {
		t.Fatal(err)
	}
	var providerAssignments int
	for _, event := range assigned {
		if event.GetRbacRoleAssigned().GetUserId() != user.Id {
			continue
		}
		reason := event.GetRbacRoleAssigned().GetReason().GetProviderGroups()
		if reason.GetProviderId() != "company" || reason.GetIssuer() != testIssuers["company"] || event.GetActorId() != SystemActorID {
			t.Fatalf("provider assignment reason or actor = %v / %q", event.GetRbacRoleAssigned().GetReason(), event.GetActorId())
		}
		providerAssignments++
	}
	if providerAssignments != 2 {
		t.Fatalf("provider assignments = %d, want 2", providerAssignments)
	}
}

func TestSyncProviderGroupRolesKeepsAdminAssignments(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "admin-assigned")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}

	// Joining and leaving the mapped group does not take over the admin assignment.
	syncGroups(t, core, user.Id, "company", "chat-mods")
	syncGroups(t, core, user.Id, "company")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
	if owner, _ := core.rbacModel.assignmentOwner(user.Id, RoleModerator); owner != "" {
		t.Fatalf("moderator owner = %q, want admin", owner)
	}
}

func TestSyncProviderGroupRolesGrantsAgainAfterAdminRevoke(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "revoked-member")
	syncGroups(t, core, user.Id, "company", "chat-mods")
	if err := core.RevokeServerRole(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false})
	syncGroups(t, core, user.Id, "company", "chat-mods")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
}

func TestSyncProviderGroupRolesChangesNothingWithoutClaim(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "unknown-groups")
	syncGroups(t, core, user.Id, "company", "chat-mods")
	mappings, _ := config.ParseGroupRoles(testGroupRoles["company"])
	if err := core.SyncProviderGroupRoles(ctx, user.Id, ProviderRoleSource{ProviderID: "company", Issuer: testIssuers["company"]}, mappings, ProviderGroups{}); err != nil {
		t.Fatal(err)
	}
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
}

func TestSyncProviderGroupRolesRevokesOnlyOwnProviderAssignments(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	if _, err := core.CreateServerRole(ctx, SystemActorID, "other-role", "Other Role", ""); err != nil {
		t.Fatal(err)
	}
	user := newGroupMember(t, core, "two-providers")
	syncGroups(t, core, user.Id, "other", "other-mods")
	// The mapping for the role moves to company. Company does not own the
	// assignment that other created, so it does not revoke it.
	syncGroupsWith(t, core, user.Id, "company", "other-mods:other-role")
	requireRoles(t, core, user.Id, map[string]bool{"other-role": true})
	if owner, _ := core.rbacModel.assignmentOwner(user.Id, "other-role"); owner != testIssuers["other"] {
		t.Fatalf("other-role owner = %q, want other issuer", owner)
	}
}

func TestSyncProviderGroupRolesConcurrentSyncsAssignOnce(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "concurrent-member")
	mappings, _ := config.ParseGroupRoles(testGroupRoles["company"])
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := core.SyncProviderGroupRoles(ctx, user.Id, ProviderRoleSource{ProviderID: "company", Issuer: testIssuers["company"]}, mappings, ProviderGroups{Groups: []string{"chat-mods"}, Known: true}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	assignment := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{}}})
	assigned, _, err := core.EventPublisher.SubjectEvents(ctx, rbacSubjectForEvent(assignment))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, event := range assigned {
		if event.GetRbacRoleAssigned().GetUserId() == user.Id {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("assignment events = %d, want 1", count)
	}
}

func TestAssignConfiguredOwnerRoleRecordsReason(t *testing.T) {
	t.Parallel()
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "configured-owner", "Configured Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := core.AssignConfiguredOwnerRole(ctx, user.Id); err != nil {
		t.Fatal(err)
	}
	assignment := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{}}})
	assigned, _, err := core.EventPublisher.SubjectEvents(ctx, rbacSubjectForEvent(assignment))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range assigned {
		if event.GetRbacRoleAssigned().GetUserId() == user.Id && event.GetRbacRoleAssigned().GetReason().GetConfiguredOwner() != nil {
			return
		}
	}
	t.Fatal("configured owner assignment has no configured-owner reason")
}

func TestRBACProjectionKeepsFirstAssignmentOwner(t *testing.T) {
	t.Parallel()
	projection := NewRBACProjection()
	provider := &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: "company", Issuer: "https://id.example"}}}
	apply := func(seq uint64, reason *evtv1.RbacRoleChangeReason) {
		t.Helper()
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U1", RoleName: "moderator", Reason: reason}}})
		if err := projection.Apply(event, seq); err != nil {
			t.Fatal(err)
		}
	}
	apply(1, nil)
	apply(2, provider)
	if owner, assigned := projection.AssignmentOwner("U1", "moderator"); !assigned || owner != "" {
		t.Fatalf("owner = %q, %v; want admin assignment", owner, assigned)
	}

	restored := NewRBACProjection()
	other := NewRBACProjection()
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U2", RoleName: "helper", Reason: provider}}})
	if err := other.Apply(event, 1); err != nil {
		t.Fatal(err)
	}
	data, err := other.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Restore(data); err != nil {
		t.Fatal(err)
	}
	if owner, assigned := restored.AssignmentOwner("U2", "helper"); !assigned || owner != "https://id.example" {
		t.Fatalf("restored owner = %q, %v; want company issuer", owner, assigned)
	}
}

// The configuration check names the reserved roles without importing core.
func TestReservedGroupRolesMatchCoreRoles(t *testing.T) {
	t.Parallel()
	if !config.IsReservedGroupRole(RoleOwner) || !config.IsReservedGroupRole(RoleEveryone) || config.IsReservedGroupRole(RoleModerator) {
		t.Fatal("config reserved group roles do not match core role names")
	}
}

func TestUnlinkingLastProviderIdentityRevokesProviderRoles(t *testing.T) {
	t.Parallel()
	for _, admin := range []bool{false, true} {
		t.Run(map[bool]string{false: "self", true: "operator"}[admin], func(t *testing.T) {
			t.Parallel()
			core := setupGroupRolesCore(t)
			ctx := testContext(t)
			user, err := core.CreateUser(ctx, SystemActorID, "unlinking-member", "Unlinking Member", "password123")
			if err != nil {
				t.Fatal(err)
			}
			for _, subject := range []string{"first", "second"} {
				if err := core.LinkExternalIdentity(ctx, "company", config.AuthProviderTypeOpenIDConnect, "https://id.example", subject, user.Id); err != nil {
					t.Fatal(err)
				}
			}
			syncGroups(t, core, user.Id, "company", "chat-mods")
			if err := core.AssignServerRole(ctx, SystemActorID, user.Id, "helper"); err != nil {
				t.Fatal(err)
			}
			unlink := func(subject string) {
				t.Helper()
				hash := externalIdentityHash("https://id.example", subject)
				actorID := user.Id
				if admin {
					actorID = SystemActorID
				}
				err := core.DisconnectExternalIdentityAs(ctx, actorID, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true})
				if err != nil {
					t.Fatalf("unlink %s: %v", subject, err)
				}
			}
			// Another identity of the provider remains, so the role stays.
			unlink("first")
			requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true, "helper": true})
			// The last identity goes, so the provider role goes. The admin role stays.
			unlink("second")
			requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false, "helper": true})
		})
	}
}

func TestAdminReassignmentAfterRevokeIsAdminOwned(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user := newGroupMember(t, core, "reassigned-member")
	syncGroups(t, core, user.Id, "company", "chat-mods")
	if err := core.RevokeServerRole(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}
	syncGroups(t, core, user.Id, "company")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
}

func TestRBACProjectionRoleDeletionClearsAssignmentOwner(t *testing.T) {
	t.Parallel()
	projection := NewRBACProjection()
	provider := &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: "company", Issuer: "https://id.example"}}}
	events := []*evtv1.Event{
		{Event: &evtv1.Event_RbacRoleCreated{RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: "helper"}}},
		{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U1", RoleName: "helper", Reason: provider}}},
		{Event: &evtv1.Event_RbacRoleDeleted{RbacRoleDeleted: &evtv1.RbacRoleDeletedEvent{RoleName: "helper"}}},
		{Event: &evtv1.Event_RbacRoleCreated{RbacRoleCreated: &evtv1.RbacRoleCreatedEvent{RoleName: "helper"}}},
		{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U1", RoleName: "helper"}}},
	}
	for i, event := range events {
		if err := projection.Apply(newEvent(SystemActorID, event), uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if owner, assigned := projection.AssignmentOwner("U1", "helper"); !assigned || owner != "" {
		t.Fatalf("owner = %q, %v; want admin assignment after recreation", owner, assigned)
	}
}

// An identity linked under an older provider ID still owns the roles that the
// provider gives under its current ID, because ownership follows the issuer.
// Admin assignments never match.
func TestUnlinkingLegacyProviderIDIdentityRevokesOnlyProviderRoles(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "legacy-member", "Legacy Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
	// The legacy single-provider OIDC configuration linked identities as "oidc".
	if err := core.LinkExternalIdentity(ctx, "oidc", config.AuthProviderTypeOpenIDConnect, testIssuers["company"], "legacy", user.Id); err != nil {
		t.Fatal(err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, "helper"); err != nil {
		t.Fatal(err)
	}
	syncGroups(t, core, user.Id, "company", "chat-mods")
	if err := core.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, externalIdentityHash(testIssuers["company"], "legacy"), ExternalIdentitySignInPolicy{PasswordLoginEnabled: true}); err != nil {
		t.Fatal(err)
	}
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false, "helper": true})
}

// A sync that grants a role after the identity was unlinked revokes it again.
func TestSyncProviderGroupRolesWithoutIdentityRevokesGrant(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "unlinked-sync", "Unlinked Sync", "password123")
	if err != nil {
		t.Fatal(err)
	}
	syncGroups(t, core, user.Id, "company", "chat-mods")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false})
}

func TestRBACProjectionEmptyIssuerOwnsNothing(t *testing.T) {
	t.Parallel()
	projection := NewRBACProjection()
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U1", RoleName: "admin"}}})
	if err := projection.Apply(event, 1); err != nil {
		t.Fatal(err)
	}
	if roles := projection.ProviderAssignments("U1", ""); len(roles) != 0 {
		t.Fatalf("ProviderAssignments with empty issuer = %v, want none", roles)
	}
}

// A legacy link with only a subject hash gets its issuer at the first sync,
// so it keeps the provider roles and an unlink revokes them.
func TestSyncProviderGroupRolesCompletesLegacyHashOnlyLink(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "hash-only", "Hash Only", "password123")
	if err != nil {
		t.Fatal(err)
	}
	hash := externalIdentityHash(testIssuers["company"], "hash-only")
	legacy := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{
		UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: user.Id, SubjectHash: hash},
	}})
	if _, err := core.appendUserEvent(ctx, user.Id, legacy, "", nil); err != nil {
		t.Fatal(err)
	}
	mappings, _ := config.ParseGroupRoles(testGroupRoles["company"])
	source := ProviderRoleSource{ProviderID: "company", Issuer: testIssuers["company"], Subject: "hash-only"}
	for range 2 {
		if err := core.SyncProviderGroupRoles(ctx, user.Id, source, mappings, ProviderGroups{Groups: []string{"chat-mods"}, Known: true}); err != nil {
			t.Fatal(err)
		}
		requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
	}
	if err := core.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true}); err != nil {
		t.Fatal(err)
	}
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: false})
}

// Roles that existed before an upgrade to provider group roles have
// reason-less assignment events. No provider path removes them: not a sync
// without mappings, not a sync with a known claim that matches no group, not
// a denied sign-in, and not an unlink of the last provider identity. This
// covers servers that already use OIDC, also with legacy hash-only links.
func TestProviderGroupRolesKeepPreUpgradeAssignments(t *testing.T) {
	t.Parallel()
	for _, legacyLink := range []bool{false, true} {
		t.Run(map[bool]string{false: "issuer link", true: "hash-only link"}[legacyLink], func(t *testing.T) {
			t.Parallel()
			core := setupGroupRolesCore(t)
			ctx := testContext(t)
			user, err := core.CreateUser(ctx, SystemActorID, "pre-upgrade", "Pre Upgrade", "password123")
			if err != nil {
				t.Fatal(err)
			}
			issuer := testIssuers["company"]
			hash := externalIdentityHash(issuer, "pre-upgrade")
			if legacyLink {
				legacy := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{
					UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: user.Id, SubjectHash: hash},
				}})
				if _, err := core.appendUserEvent(ctx, user.Id, legacy, "", nil); err != nil {
					t.Fatal(err)
				}
			} else if err := core.LinkExternalIdentity(ctx, "company", config.AuthProviderTypeOpenIDConnect, issuer, "pre-upgrade", user.Id); err != nil {
				t.Fatal(err)
			}
			// Assignment events in the shape that older server versions wrote.
			for _, roleName := range []string{RoleModerator, "helper"} {
				event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{
					RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: user.Id, RoleName: roleName},
				}})
				if _, err := core.appendRoleAssignmentEvent(ctx, user.Id, true, event, nil); err != nil {
					t.Fatal(err)
				}
			}
			preUpgrade := map[string]bool{RoleModerator: true, "helper": true}
			requireRoles(t, core, user.Id, preUpgrade)

			source := ProviderRoleSource{ProviderID: "company", Issuer: issuer, Subject: "pre-upgrade"}
			mappings, err := config.ParseGroupRoles(testGroupRoles["company"])
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct {
				name     string
				mappings []config.GroupRole
				groups   ProviderGroups
			}{
				{"sign-in without group_roles", nil, ProviderGroups{Known: true}},
				{"sign-in in no mapped group", mappings, ProviderGroups{Known: true}},
				{"sign-in in mapped groups", mappings, ProviderGroups{Groups: []string{"chat-mods", "helpers"}, Known: true}},
				{"sign-in after leaving mapped groups", mappings, ProviderGroups{Groups: []string{"unrelated"}, Known: true}},
			} {
				if err := core.SyncProviderGroupRoles(ctx, user.Id, source, step.mappings, step.groups); err != nil {
					t.Fatalf("%s: %v", step.name, err)
				}
				requireRoles(t, core, user.Id, preUpgrade)
			}
			for _, roleName := range []string{RoleModerator, "helper"} {
				if owner, _ := core.rbacModel.assignmentOwner(user.Id, roleName); owner != "" {
					t.Fatalf("%s owner = %q, want admin", roleName, owner)
				}
			}

			// allowed_groups denies a sign-in.
			if err := core.RevokeProviderGroupRoles(ctx, user.Id, source); err != nil {
				t.Fatal(err)
			}
			requireRoles(t, core, user.Id, preUpgrade)

			if err := core.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true}); err != nil {
				t.Fatal(err)
			}
			requireRoles(t, core, user.Id, preUpgrade)
		})
	}
}
