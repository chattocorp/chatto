package core

import (
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/config"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

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
	if err := core.SyncProviderGroupRoles(testContext(t), userID, providerID, mappings, ProviderGroups{Groups: groups, Known: true}); err != nil {
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
	user, err := core.CreateUser(ctx, SystemActorID, "group-member", "Group Member", "password123")
	if err != nil {
		t.Fatal(err)
	}

	syncGroups(t, core, user.Id, "company", "chat-mods", "it-ops", "admins")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true, "helper": true, RoleOwner: false})
	if owner, _ := core.rbacModel.assignmentProvider(user.Id, RoleModerator); owner != "company" {
		t.Fatalf("moderator owner = %q, want company", owner)
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
		if event.GetRbacRoleAssigned().GetReason().GetProviderGroups().GetProviderId() != "company" || event.GetActorId() != SystemActorID {
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
	user, err := core.CreateUser(ctx, SystemActorID, "admin-assigned", "Admin Assigned", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}

	// Joining and leaving the mapped group does not take over the admin assignment.
	syncGroups(t, core, user.Id, "company", "chat-mods")
	syncGroups(t, core, user.Id, "company")
	requireRoles(t, core, user.Id, map[string]bool{RoleModerator: true})
	if owner, _ := core.rbacModel.assignmentProvider(user.Id, RoleModerator); owner != "" {
		t.Fatalf("moderator owner = %q, want admin", owner)
	}
}

func TestSyncProviderGroupRolesGrantsAgainAfterAdminRevoke(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "revoked-member", "Revoked Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
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
	user, err := core.CreateUser(ctx, SystemActorID, "unknown-groups", "Unknown Groups", "password123")
	if err != nil {
		t.Fatal(err)
	}
	syncGroups(t, core, user.Id, "company", "chat-mods")
	mappings, _ := config.ParseGroupRoles(testGroupRoles["company"])
	if err := core.SyncProviderGroupRoles(ctx, user.Id, "company", mappings, ProviderGroups{}); err != nil {
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
	user, err := core.CreateUser(ctx, SystemActorID, "two-providers", "Two Providers", "password123")
	if err != nil {
		t.Fatal(err)
	}
	syncGroups(t, core, user.Id, "other", "other-mods")
	// The mapping for the role moves to company. Company does not own the
	// assignment that other created, so it does not revoke it.
	syncGroupsWith(t, core, user.Id, "company", "other-mods:other-role")
	requireRoles(t, core, user.Id, map[string]bool{"other-role": true})
	if owner, _ := core.rbacModel.assignmentProvider(user.Id, "other-role"); owner != "other" {
		t.Fatalf("other-role owner = %q, want other", owner)
	}
}

func TestSyncProviderGroupRolesConcurrentSyncsAssignOnce(t *testing.T) {
	t.Parallel()
	core := setupGroupRolesCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "concurrent-member", "Concurrent Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
	mappings, _ := config.ParseGroupRoles(testGroupRoles["company"])
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := core.SyncProviderGroupRoles(ctx, user.Id, "company", mappings, ProviderGroups{Groups: []string{"chat-mods"}, Known: true}); err != nil {
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
	provider := &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: "company"}}}
	apply := func(seq uint64, reason *evtv1.RbacRoleChangeReason) {
		t.Helper()
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "U1", RoleName: "moderator", Reason: reason}}})
		if err := projection.Apply(event, seq); err != nil {
			t.Fatal(err)
		}
	}
	apply(1, nil)
	apply(2, provider)
	if owner, assigned := projection.AssignmentProvider("U1", "moderator"); !assigned || owner != "" {
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
	if owner, assigned := restored.AssignmentProvider("U2", "helper"); !assigned || owner != "company" {
		t.Fatalf("restored owner = %q, %v; want company", owner, assigned)
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
				var err error
				if admin {
					err = core.DisconnectExternalIdentityAs(ctx, SystemActorID, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true})
				} else {
					err = core.DisconnectExternalIdentity(ctx, user.Id, hash)
				}
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
	user, err := core.CreateUser(ctx, SystemActorID, "reassigned-member", "Reassigned Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
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
	provider := &evtv1.RbacRoleChangeReason{Reason: &evtv1.RbacRoleChangeReason_ProviderGroups{ProviderGroups: &evtv1.RbacProviderGroupsReason{ProviderId: "company"}}}
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
	if owner, assigned := projection.AssignmentProvider("U1", "helper"); !assigned || owner != "" {
		t.Fatalf("owner = %q, %v; want admin assignment after recreation", owner, assigned)
	}
}
