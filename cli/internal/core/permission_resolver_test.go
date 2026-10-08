package core

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// ============================================================================
// HasServerPermission Tests
// ============================================================================

func TestPermissionResolver_HasServerPermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("returns true when user has user.delete-self via everyone role", func(t *testing.T) {
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermUserDeleteSelf)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have user.delete-self via everyone role")
		}
	})

	t.Run("returns true for message.post at server scope by default", func(t *testing.T) {
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have server-scope message.post by default")
		}
	})

	t.Run("returns false when user lacks permission", func(t *testing.T) {
		// Regular user doesn't have admin.view-users
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermAdminUsersView)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if has {
			t.Error("Expected user NOT to have admin.view-users")
		}
	})

}

func TestPermissionResolver_MessageReadInclusionTruthTable(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "read-inclusion", "Read Inclusion", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Only a single user can be denied (ADR-116). Thus the table sets the
	// user's room settings, without everyone server allows to fall back to.
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "read-inclusion", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, permission := range []Permission{PermMessageRead, PermMessageReadInteractions} {
		if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, permission); err != nil {
			t.Fatalf("clear server default %s: %v", permission, err)
		}
	}

	tests := []struct {
		name   string
		broad  PermissionState
		narrow PermissionState
		want   DecisionKind
	}{
		{name: "no decisions", broad: PermissionStateNone, narrow: PermissionStateNone, want: DecisionNone},
		{name: "broad allow", broad: PermissionStateAllow, narrow: PermissionStateNone, want: DecisionAllow},
		{name: "broad allow beats narrow deny", broad: PermissionStateAllow, narrow: PermissionStateDeny, want: DecisionAllow},
		{name: "narrow allow is independent of broad deny", broad: PermissionStateDeny, narrow: PermissionStateAllow, want: DecisionAllow},
		{name: "broad deny does not become narrow deny", broad: PermissionStateDeny, narrow: PermissionStateNone, want: DecisionNone},
		{name: "narrow deny applies without broad allow", broad: PermissionStateNone, narrow: PermissionStateDeny, want: DecisionDeny},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apply := func(permission Permission, state PermissionState) {
				t.Helper()
				var err error
				switch state {
				case PermissionStateAllow:
					err = core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.GetId(), permission)
				case PermissionStateDeny:
					err = core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.GetId(), permission)
				default:
					err = core.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, user.GetId(), permission)
				}
				if err != nil {
					t.Fatalf("set %s to %s: %v", permission, state, err)
				}
			}
			apply(PermMessageRead, test.broad)
			apply(PermMessageReadInteractions, test.narrow)

			got, err := core.PermResolver().Resolve(ctx, user.GetId(), KindChannel, room.Id, PermMessageReadInteractions)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != test.want {
				t.Fatalf("decision = %s, want %s", got, test.want)
			}
		})
	}

	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("grant narrow permission: %v", err)
	}
	if got, err := core.PermResolver().Resolve(ctx, user.GetId(), KindChannel, "", PermMessageRead); err != nil || got != DecisionNone {
		t.Fatalf("narrow permission must not include broad permission: decision = %s, err = %v", got, err)
	}
}

// This test does not call t.Parallel: installTestPermissionInclusion changes
// the package-wide permission catalog.
func TestPermissionResolver_ResolvesExplicitInclusion(t *testing.T) {
	broad, narrow := installTestPermissionInclusion(t)
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "explicit-inclusion", "Explicit Inclusion", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, user.GetId(), broad); err != nil {
		t.Fatalf("grant broad permission: %v", err)
	}
	if err := core.DenyUserPermission(ctx, SystemActorID, user.GetId(), narrow); err != nil {
		t.Fatalf("deny narrow permission: %v", err)
	}
	if got, err := core.PermResolver().Resolve(ctx, user.GetId(), KindChannel, "", narrow); err != nil || got != DecisionAllow {
		t.Fatalf("included decision = %s, %v; want allow", got, err)
	}
}

func TestPermissionResolver_HasServerPermission_MultiRoleDenyWins(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("same-subject denial replaces grant", func(t *testing.T) {
		// Grant the permission to the user. Only a single user can be denied
		// (ADR-116), so the user is the subject.
		err := core.GrantUserPermission(ctx, SystemActorID, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny same permission for the same user (replaces the grant)
		err = core.DenyUserPermission(ctx, SystemActorID, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// User should NOT have the permission (denial replaced grant)
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if has {
			t.Error("Expected denial to replace grant")
		}
	})
}

func TestPermissionResolver_HasSpacePermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	t.Run("returns true when user has permission via space role", func(t *testing.T) {
		// Space admin gets space.manage
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermServerManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space admin to have space.manage")
		}
	})

	t.Run("returns false when user lacks permission at space level", func(t *testing.T) {
		// Create another user who is not a member
		otherUser, _ := core.CreateUser(ctx, "system", "otheruser", "Other User", "password123")

		// Non-member should not have space.manage
		has, err := core.permissionResolver.HasSpacePermission(ctx, otherUser.Id, KindChannel, PermServerManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-member NOT to have space.manage")
		}
	})

	// "instance-only permission returns false at space level" was a dual-tier
	// assertion that no longer applies: post-Phase-5 there's only one tier, so
	// an admin-permission grant on a role propagates everywhere.
}

func TestPermissionResolver_HasSpacePermission_ServerFallback(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	t.Run("owner gets space-scoped permissions from effective-owner override", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermRoomCreate)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected owner to have room.create via effective-owner override")
		}
	})

	t.Run("non-member does NOT get space-scoped permissions", func(t *testing.T) {
		// Create user who is NOT a space member
		nonMember, _ := core.CreateUser(ctx, "system", "nonmember", "Non Member", "password123")

		// Non-member should NOT get room.create
		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, KindChannel, PermRoomCreate)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-member NOT to have space-scoped permission")
		}
	})

	t.Run("authenticated user gets server-scope message.post by default", func(t *testing.T) {
		// Create user who is NOT a space member
		nonMember, _ := core.CreateUser(ctx, "system", "nonmember2", "Non Member 2", "password123")

		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected authenticated user to have server-scope message.post by default")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_ServerRoleOverride(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("space can override role permissions", func(t *testing.T) {
		// Grant permission to instance-everyone at space level (override)
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// User should have the permission via the space-level override
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermRoomManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space-level override for role to work")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_DMKind(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("DM rooms allow message.post", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected DM rooms to allow message.post")
		}
	})

	t.Run("DM rooms deny server.manage", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermServerManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected DM rooms NOT to allow server.manage")
		}
	})

	t.Run("DM rooms keep room.join outside the scope", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermRoomJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected DM rooms not to resolve room.join")
		}
	})
}

// ============================================================================
// HasRoomPermission Tests
// ============================================================================

func TestPermissionResolver_HasRoomPermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("returns true when user has permission at room level", func(t *testing.T) {
		// Grant permission at room level
		err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleOwner, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have permission at room level")
		}
	})

	t.Run("falls back to space level", func(t *testing.T) {
		// User is space admin, should have space.manage which doesn't apply at room level
		// but room.manage does apply at space and room levels
		err := core.GrantServerPermission(ctx, SystemActorID, RoleOwner, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant space permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermRoomManage)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected fallback to space level to work")
		}
	})
}

func TestPermissionResolver_HasRoomPermission_RoomGrantOverridesAbsentSetGrant(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomoverride1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomoverride1member", "Member", "password123")

	// Clear the group-scope AND server-scope grants for message.react so
	// member starts with no permission at any scope, then verify a per-room
	// override grants it.
	groups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	groupID := groups[0].Id
	if err := core.ClearGroupPermissionState(ctx, SystemActorID, groupID, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("ClearGroupPermissionState: %v", err)
	}
	if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}
	if err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("ClearRoomPermissionState: %v", err)
	}

	// Verify member doesn't have permission with no set grant
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected member NOT to have message.react before room grant")
	}

	// Grant at room level
	err = core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)
	if err != nil {
		t.Fatalf("Failed to grant room permission: %v", err)
	}

	has, err = core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected room grant to give member message.react")
	}
}

func TestPermissionResolver_HasRoomPermission_UserRoomDenyOverridesServerGrant(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomdeny1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomdeny1member", "Member", "password123")
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("baseline: expected the everyone server grant to allow")
	}

	if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessagePost); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}
	has, err = core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected the user's room deny to block the everyone server grant")
	}
}

// TestPermissionResolver_HasRoomPermission_NearestUserSettingDecides checks
// that the user's nearest setting decides between a user allow and a user
// deny.
func TestPermissionResolver_HasRoomPermission_NearestUserSettingDecides(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	group, err := core.CreateRoomGroup(ctx, SystemActorID, "Override Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "general", "General")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	groupScope := PermissionTargetScope{Kind: MatrixScopeGroup, ID: group.Id}
	owner, _ := core.CreateUser(ctx, SystemActorID, "nearest-owner", "Owner", "password123")
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}

	t.Run("room allow replaces a group deny", func(t *testing.T) {
		member, _ := core.CreateUser(ctx, "system", "nearest-room-allow", "Member", "password123")
		if err := core.SetUserPermissionState(ctx, owner.Id, member.Id, groupScope, PermMessagePost, PermissionStateDeny); err != nil {
			t.Fatalf("SetUserPermissionState group deny: %v", err)
		}
		if has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost); err != nil || has {
			t.Fatalf("with group deny = %v, %v; want denied", has, err)
		}
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessagePost); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		if has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost); err != nil || !has {
			t.Fatalf("with room allow = %v, %v; want allowed", has, err)
		}
	})

	t.Run("room deny replaces a group allow", func(t *testing.T) {
		member, _ := core.CreateUser(ctx, "system", "nearest-room-deny", "Member", "password123")
		if err := core.SetUserPermissionState(ctx, owner.Id, member.Id, groupScope, PermMessageManage, PermissionStateAllow); err != nil {
			t.Fatalf("SetUserPermissionState group allow: %v", err)
		}
		if has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageManage); err != nil || !has {
			t.Fatalf("with group allow = %v, %v; want allowed", has, err)
		}
		if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessageManage); err != nil {
			t.Fatalf("DenyUserRoomPermission: %v", err)
		}
		if has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageManage); err != nil || has {
			t.Fatalf("with room deny = %v, %v; want denied", has, err)
		}
	})
}

func TestPermissionResolver_HasRoomPermission_IsolationBetweenRooms(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomisoadmin", "Admin", "password123")
	roomA, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "rooma", "Room A")
	roomB, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "roomb", "Room B")

	member, _ := core.CreateUser(ctx, "system", "roomisomember", "Member", "password123")
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	// Deny message.post to the member only in room A.
	if err := core.DenyUserRoomPermission(ctx, SystemActorID, roomA.Id, member.Id, PermMessagePost); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	// Room A: denied
	hasA, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomA.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasA {
		t.Error("Expected member to be denied in room A")
	}

	// Room B: allowed (no room setting, the everyone server grant applies)
	hasB, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomB.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasB {
		t.Error("Expected member to have permission in room B (no override)")
	}
}

func TestPermissionResolver_HasRoomPermission_ServerRoleRoomGrant(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomgrant1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomgrant1member", "Member", "password123")
	// Clear message.react from everyone at space level (no grant)
	core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageReact)

	// Grant message.react to instance-everyone at room level
	core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected role room grant to give permission")
	}
}

func TestPermissionResolver_HasRoomPermission_ClearFallsBackToSpace(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "clearfallbackadmin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "clearfallbackmember", "Member", "password123")
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}
	if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessagePost); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	// Verify denied
	has, _ := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if has {
		t.Fatal("Setup error: expected the user's room deny to block")
	}

	// Clear the user's room setting
	if err := core.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, member.Id, PermMessagePost); err != nil {
		t.Fatalf("ClearUserRoomPermissionState: %v", err)
	}

	// Should fall back to space grant
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected clearing room override to fall back to space grant")
	}
}

func TestPermissionResolver_HasRoomPermission_MultiplePermissionsPerRoom(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "multipermadmin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "multipermmember", "Member", "password123")
	// Grant message.post to everyone only at room level, and deny
	// message.react to the member at room level.
	if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}
	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}
	if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessageReact); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	hasPost, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPost {
		t.Error("Expected message.post to be granted at room level")
	}

	hasReact, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasReact {
		t.Error("Expected message.react to be denied at room level")
	}
}

// ============================================================================
// Per-User Override Contract — user-level grants/denies beat role decisions
// ============================================================================

func TestPermissionResolver_UserLevelOverrides(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("user-level deny suspends a user despite role grants", func(t *testing.T) {
		// The classic suspension use case: deny a perm directly to this
		// user, and no role they have can re-grant it.
		mod, _ := core.CreateUser(ctx, SystemActorID, "user-deny-mod", "Mod", "password123")
		if err := core.AssignServerRole(ctx, SystemActorID, mod.Id, RoleModerator); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		if err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost); err != nil {
			t.Fatalf("GrantServerPermission: %v", err)
		}
		has, _ := core.HasServerPermission(ctx, mod.Id, PermMessagePost)
		if !has {
			t.Fatal("baseline: moderator should have message.post")
		}
		// Suspend posting by user-deny.
		if err := core.DenyUserPermission(ctx, SystemActorID, mod.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		has, _ = core.HasServerPermission(ctx, mod.Id, PermMessagePost)
		if has {
			t.Error("expected user-deny to suspend the moderator's message.post")
		}
	})

	t.Run("owner override beats user-level deny", func(t *testing.T) {
		owner, _ := core.CreateUser(ctx, SystemActorID, "user-deny-owner", "Owner", "password123")
		if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		if err := core.DenyUserPermission(ctx, SystemActorID, owner.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		has, _ := core.HasServerPermission(ctx, owner.Id, PermMessagePost)
		if !has {
			t.Error("expected owner override to beat user-deny for message.post")
		}
	})

	t.Run("user-level grant gives a single user a permission no role grants them", func(t *testing.T) {
		// The classic "give this one user admin powers on room X without
		// inventing a role" use case.
		user, _ := core.CreateUser(ctx, SystemActorID, "user-grant-bob", "Bob", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "general", "General")

		// Without a grant, bob can't delete-any in this room.
		has, _ := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessageManage)
		if has {
			t.Fatal("baseline: bob should not have delete-any")
		}

		// Grant directly on the user, at room scope.
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermMessageManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessageManage)
		if !has {
			t.Error("expected user-level room grant to give bob delete-any in this room")
		}

		// Other rooms unaffected.
		other, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "other", "Other")
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, other.Id, PermMessageManage)
		if has {
			t.Error("user-level room grant should not leak to other rooms")
		}
	})

	t.Run("user-level group deny blocks the user in the group's rooms", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, SystemActorID, "user-group-deny", "User", "password123")
		other, _ := core.CreateUser(ctx, SystemActorID, "user-group-other", "Other", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "private", "Private")
		owner, _ := core.CreateUser(ctx, SystemActorID, "user-group-owner", "Owner", "password123")
		if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		groupScope := PermissionTargetScope{Kind: MatrixScopeGroup, ID: room.GroupId}
		if err := core.SetUserPermissionState(ctx, owner.Id, user.Id, groupScope, PermMessagePost, PermissionStateDeny); err != nil {
			t.Fatalf("SetUserPermissionState group deny: %v", err)
		}
		has, _ := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if has {
			t.Error("expected the user's group deny to block the everyone server allow")
		}
		// Other users keep the everyone server allow.
		has, _ = core.permissionResolver.HasRoomPermission(ctx, other.Id, KindChannel, room.Id, PermMessagePost)
		if !has {
			t.Error("a user-level group deny must not affect other users")
		}
	})

	t.Run("nearest user-level room grant replaces the same user's server deny", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, SystemActorID, "user-nearest-override", "Nearest Override", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "nearest-override", "Nearest Override")
		if err := core.DenyUserPermission(ctx, SystemActorID, user.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermMessagePost); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if !has {
			t.Error("expected the user's room allow to replace that user's server deny")
		}
	})

	t.Run("DM user grant overrides inherited state", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx2 := testContext(t)
		admin, _ := c.CreateUser(ctx2, SystemActorID, "dm-scope-admin", "Admin", "password123")
		if err := c.AssignOwnerRole(ctx2, admin.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		user, _ := c.CreateUser(ctx2, SystemActorID, "dm-boundary-user", "User", "password123")
		dmRoomID := "R_dm_boundary_user_test"
		if err := c.SetUserPermissionState(ctx2, admin.Id, user.Id, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageManage, PermissionStateAllow); err != nil {
			t.Fatalf("SetUserPermissionState: %v", err)
		}
		has, _ := c.permissionResolver.HasRoomPermission(ctx2, user.Id, KindDM, dmRoomID, PermMessageManage)
		if !has {
			t.Error("expected the DM user grant to allow message.manage")
		}
	})

	t.Run("owner override applies only to DM permissions", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx2 := testContext(t)
		owner, _ := c.CreateUser(ctx2, SystemActorID, "dm-boundary-owner", "Owner", "password123")
		if err := c.AssignOwnerRole(ctx2, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		// Sanity: owner has server-scope permissions via the effective-owner override.
		has, _ := c.HasServerPermission(ctx2, owner.Id, PermMessagePost)
		if !has {
			t.Fatal("baseline: owner should resolve allow for message.post")
		}
		dmRoomID := "R_dm_boundary_owner_test"
		for _, perm := range []Permission{PermMessageManage} {
			has, _ := c.permissionResolver.HasRoomPermission(ctx2, owner.Id, KindDM, dmRoomID, perm)
			if !has {
				t.Errorf("expected owner override to allow %s", perm)
			}
		}
		for _, perm := range []Permission{PermRoomManage, PermRoomMemberRemove} {
			has, _ := c.permissionResolver.HasRoomPermission(ctx2, owner.Id, KindDM, dmRoomID, perm)
			if has {
				t.Errorf("expected %s to remain outside the DM scope", perm)
			}
		}
	})

	t.Run("clear restores normal role-based resolution", func(t *testing.T) {
		// Use a fresh core so prior subtests' state can't contaminate this one.
		c, _ := setupTestCore(t)
		c2ctx := testContext(t)
		user, _ := c.CreateUser(c2ctx, SystemActorID, "clear-user", "User", "password123")
		has, _ := c.HasServerPermission(c2ctx, user.Id, PermUserDeleteSelf)
		if !has {
			t.Fatal("baseline: user should have user.delete-self via everyone")
		}
		_ = c.DenyUserPermission(c2ctx, SystemActorID, user.Id, PermUserDeleteSelf)
		has, _ = c.HasServerPermission(c2ctx, user.Id, PermUserDeleteSelf)
		if has {
			t.Fatal("expected user-deny to take effect")
		}
		if err := c.ClearUserPermissionState(c2ctx, SystemActorID, user.Id, PermUserDeleteSelf); err != nil {
			t.Fatalf("ClearUserPermissionState: %v", err)
		}
		has, _ = c.HasServerPermission(c2ctx, user.Id, PermUserDeleteSelf)
		if !has {
			t.Error("expected clear to restore default-allow")
		}
	})
}

// ============================================================================
// DM Permission Contract locks down what the unified walker resolves in a DM
// room. All message permissions use the DM-to-Server hierarchy. Room
// permissions stay outside the DM scope.
// ============================================================================

func TestPermissionResolver_DMContract(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	regular, _ := core.CreateUser(ctx, "system", "dmcontract-regular", "Regular", "password123")
	moderator, _ := core.CreateUser(ctx, "system", "dmcontract-mod", "Moderator", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, moderator.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	// Synthetic DM room ID — the walker doesn't care about room existence,
	// only about whether room-scope permission facts exist for it (we set none).
	dmRoomID := "R_dm_contract_test"

	// Each row encodes the expected resolution for the given persona at
	// room scope in a DM. Asserts the new contract — change requires a
	// deliberate review.
	type expected struct {
		regular   bool
		moderator bool
	}
	cases := []struct {
		perm Permission
		want expected
		why  string
	}{
		// Room permissions do not apply to direct messages.
		{PermRoomManage, expected{false, false}, "DM rooms can't be managed channel-style"},
		{PermRoomMemberRemove, expected{false, false}, "DM participants can't be removed"},
		{PermRoomCreate, expected{false, false}, "DMs use FindOrCreateDM"},
		{PermRoomJoin, expected{false, false}, "DM membership uses dedicated rules"},

		// Message permissions resolve through the DM and Server tiers.
		{PermMessageManage, expected{false, true}, "moderators inherit message management"},
		{PermMessageEcho, expected{true, true}, "DM thread replies can echo to the conversation"},
		{PermMessagePostInThread, expected{true, true}, "DM threads use Enabled behavior"},
		{PermMessagePost, expected{true, true}, "core DM capability"},
		{PermMessageAttach, expected{true, true}, "core DM capability"},
		{PermMessageReact, expected{true, true}, "core DM capability"},
	}

	for _, tc := range cases {
		t.Run(string(tc.perm), func(t *testing.T) {
			gotRegular, err := core.permissionResolver.HasRoomPermission(ctx, regular.Id, KindDM, dmRoomID, tc.perm)
			if err != nil {
				t.Fatalf("regular HasRoomPermission: %v", err)
			}
			if gotRegular != tc.want.regular {
				t.Errorf("regular: HasRoomPermission(%s) = %v, want %v (%s)", tc.perm, gotRegular, tc.want.regular, tc.why)
			}
			gotMod, err := core.permissionResolver.HasRoomPermission(ctx, moderator.Id, KindDM, dmRoomID, tc.perm)
			if err != nil {
				t.Fatalf("moderator HasRoomPermission: %v", err)
			}
			if gotMod != tc.want.moderator {
				t.Errorf("moderator: HasRoomPermission(%s) = %v, want %v (%s)", tc.perm, gotMod, tc.want.moderator, tc.why)
			}
		})
	}
}

func TestPermissionResolver_DMAttachInheritsAndOverridesServer(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	regular, _ := core.CreateUser(ctx, "system", "dmattachdeny", "DM Attach Deny", "password123")
	dmRoomID := "R_dm_attach_deny_test"

	if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageAttach); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}

	got, err := core.permissionResolver.HasRoomPermission(ctx, regular.Id, KindDM, dmRoomID, PermMessageAttach)
	if err != nil {
		t.Fatalf("HasRoomPermission before deny: %v", err)
	}
	if got {
		t.Fatal("message.attach should be absent after the inherited Server grant is cleared")
	}
	admin, _ := core.CreateUser(ctx, SystemActorID, "dmattach-admin", "Admin", "password123")
	if err := core.AssignOwnerRole(ctx, admin.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	if err := core.SetUserPermissionState(ctx, admin.Id, regular.Id, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageAttach, PermissionStateAllow); err != nil {
		t.Fatalf("SetUserPermissionState allow: %v", err)
	}
	got, err = core.permissionResolver.HasRoomPermission(ctx, regular.Id, KindDM, dmRoomID, PermMessageAttach)
	if err != nil || !got {
		t.Fatalf("DM override result = %v, %v; want allow", got, err)
	}
	if err := core.SetUserPermissionState(ctx, admin.Id, regular.Id, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageAttach, PermissionStateNone); err != nil {
		t.Fatalf("SetUserPermissionState clear: %v", err)
	}

	// Only a single user can be denied (ADR-116).
	if err := core.DenyUserPermission(ctx, SystemActorID, regular.Id, PermMessageAttach); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}

	got, err = core.permissionResolver.HasRoomPermission(ctx, regular.Id, KindDM, dmRoomID, PermMessageAttach)
	if err != nil {
		t.Fatalf("HasRoomPermission after deny: %v", err)
	}
	if got {
		t.Fatal("explicit Server deny should apply when the DM override is absent")
	}
}

// ============================================================================
// Room/group/server scope tests for nearest-scope permission resolution.
// ============================================================================

func TestPermissionResolver_RoomOverridesServer(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "hieradmin", "Admin User", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "General", "General chat")

	member, _ := core.CreateUser(ctx, "system", "hiermember", "Member User", "password123")

	t.Run("room grant overrides server deny on the same subject", func(t *testing.T) {
		// Only a single user can be denied (ADR-116).
		if err := core.DenyUserPermission(ctx, SystemActorID, member.Id, PermMessageReact); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessageReact); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if !has {
			t.Error("expected room grant to override server deny for the same subject")
		}
	})

	t.Run("user room deny overrides an everyone server grant", func(t *testing.T) {
		if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("GrantServerPermission: %v", err)
		}
		if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserRoomPermission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if has {
			t.Error("expected the user's room deny to override the everyone server grant")
		}
	})

	t.Run("server grant + server deny on the same subject: deny wins (grant probed first, but only one was set)", func(t *testing.T) {
		// Sanity check on the within-subject probe order: Grant and Deny
		// shouldn't coexist on the same subject/scope in practice (a deny
		// clears any matching grant and vice versa), but cover the rare race.
		newUser, _ := core.CreateUser(ctx, "system", "graceuser", "Grace", "password123")

		if err := core.GrantUserPermission(ctx, SystemActorID, newUser.Id, PermMessagePost); err != nil {
			t.Fatalf("GrantUserPermission: %v", err)
		}
		if err := core.DenyUserPermission(ctx, SystemActorID, newUser.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}

		// Deny operation clears the prior grant, so only the deny remains in the projection.
		has, err := core.permissionResolver.HasSpacePermission(ctx, newUser.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission: %v", err)
		}
		if has {
			t.Error("expected deny on the user to block message.post")
		}
	})
}

// ============================================================================
// Instance Authority Tests
// ============================================================================

func TestPermissionResolver_ServerAuthority(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "authadmin", "Admin User", "password123")

	member, _ := core.CreateUser(ctx, "system", "authmember", "Member User", "password123")
	t.Run("instance grant applies for space member", func(t *testing.T) {
		// Grant at instance level for instance-everyone
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant server permission: %v", err)
		}

		// Instance grant should apply (no space-level decision)
		has, err := core.permissionResolver.HasSpacePermission(ctx, member.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected instance grant to apply for space member")
		}
	})
}

func TestContentAuthorizationReadsOneServerContentViewGeneration(t *testing.T) {
	t.Parallel()

	tests := map[string]func(context.Context, *ChattoCore) error{
		"permission resolution": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.PermResolver().Resolve(ctx, SystemActorID, KindChannel, "", PermMessagePost)
			return err
		},
		"owner check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.IsServerOwner(ctx, SystemActorID)
			return err
		},
		"DM creation check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.CanStartDM(ctx, "missing-user")
			return err
		},
		"admin capability check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.HasAnyAdminPermission(ctx, SystemActorID)
			return err
		},
		"effective membership check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.RoomMembershipExists(ctx, KindDM, SystemActorID, "missing-room")
			return err
		},
		"room ban and RBAC check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.CanJoinRoomAt(ctx, SystemActorID, KindChannel, "missing-room")
			return err
		},
		"message and interaction check": func(ctx context.Context, core *ChattoCore) error {
			_, err := core.CanReadMessage(ctx, SystemActorID, KindChannel, "missing-room", "missing-message")
			return err
		},
	}
	for name, check := range tests {
		t.Run(name, func(t *testing.T) {
			chattoCore, _ := setupTestCore(t)
			entered := make(chan struct{})
			release := make(chan struct{})
			readDone := make(chan error, 1)
			go func() {
				readDone <- chattoCore.contentView.Read(func(uint64) error {
					close(entered)
					<-release
					return nil
				})
			}()
			<-entered

			checked := make(chan error, 1)
			go func() {
				checked <- check(t.Context(), chattoCore)
			}()
			select {
			case err := <-checked:
				t.Fatalf("authorization crossed an active content-view transaction: %v", err)
			case <-time.After(10 * time.Millisecond):
			}
			close(release)
			if err := <-readDone; err != nil {
				t.Fatal(err)
			}
			if err := <-checked; err != nil && !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
		})
	}
}

func TestDenyingMessagePostAlsoStopsThreadReplies(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user := createPermissionEditUser(t, core, ctx, "muted-member")
	roomID := createPermissionEditRoom(t, core, ctx, "muted-room")
	if err := core.DenyUserPermission(ctx, SystemActorID, user, PermMessagePost); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}

	// message.post includes thread replies, and no separate everyone default
	// allows message.post-in-thread.
	for _, perm := range []Permission{PermMessagePost, PermMessagePostInThread, PermMessagePostInInteractions} {
		allowed, err := core.hasRoomPermission(ctx, KindChannel, roomID, user, perm)
		if err != nil {
			t.Fatalf("hasRoomPermission %s: %v", perm, err)
		}
		if allowed {
			t.Errorf("%s allowed for a member who is denied message.post", perm)
		}
	}
}

// TestPermissionResolver_AdditiveRoles covers the subject rules (ADR-116): a
// deny as the user's nearest setting decides; otherwise any allow of the
// user, a role, or everyone at an applicable scope allows; otherwise there is
// no access. Stored role denies, everyone included, have no effect.
func TestPermissionResolver_AdditiveRoles(t *testing.T) {
	t.Parallel()

	const perm = PermMessageAttach
	type fixture struct {
		c       *ChattoCore
		ctx     context.Context
		user    string
		roomID  string
		groupID string
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, f fixture) error
		want  bool
	}{
		{"no setting means no access", func(t *testing.T, f fixture) error { return nil }, false},
		{"user deny beats a more specific role allow", func(t *testing.T, f fixture) error {
			if err := f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, "helper", perm); err != nil {
				return err
			}
			return f.c.DenyUserPermission(f.ctx, SystemActorID, f.user, perm)
		}, false},
		{"user deny beats a more specific everyone allow", func(t *testing.T, f fixture) error {
			if err := f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.DenyUserPermission(f.ctx, SystemActorID, f.user, perm)
		}, false},
		{"nearest user deny beats the user's server allow", func(t *testing.T, f fixture) error {
			if err := f.c.GrantUserPermission(f.ctx, SystemActorID, f.user, perm); err != nil {
				return err
			}
			return f.c.DenyUserRoomPermission(f.ctx, SystemActorID, f.roomID, f.user, perm)
		}, false},
		{"nearest user allow beats the user's server deny", func(t *testing.T, f fixture) error {
			if err := f.c.DenyUserPermission(f.ctx, SystemActorID, f.user, perm); err != nil {
				return err
			}
			return f.c.GrantUserRoomPermission(f.ctx, SystemActorID, f.roomID, f.user, perm)
		}, true},
		{"user server allow applies despite a stored everyone room deny", func(t *testing.T, f fixture) error {
			appendStoredRoleDeny(t, f.c, f.ctx, ScopeRoom, f.roomID, RoleEveryone, perm)
			return f.c.GrantUserPermission(f.ctx, SystemActorID, f.user, perm)
		}, true},
		{"role server allow applies despite a stored everyone room deny", func(t *testing.T, f fixture) error {
			appendStoredRoleDeny(t, f.c, f.ctx, ScopeRoom, f.roomID, RoleEveryone, perm)
			return f.c.GrantServerPermission(f.ctx, SystemActorID, "helper", perm)
		}, true},
		{"role room allow applies despite a stored everyone group deny", func(t *testing.T, f fixture) error {
			appendStoredRoleDeny(t, f.c, f.ctx, ScopeGroup, f.groupID, RoleEveryone, perm)
			return f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, "helper", perm)
		}, true},
		{"everyone server allow applies despite a stored role room deny", func(t *testing.T, f fixture) error {
			appendStoredRoleDeny(t, f.c, f.ctx, ScopeRoom, f.roomID, "helper", perm)
			return f.c.GrantServerPermission(f.ctx, SystemActorID, RoleEveryone, perm)
		}, true},
		{"a stored everyone deny alone gives no access", func(t *testing.T, f fixture) error {
			appendStoredRoleDeny(t, f.c, f.ctx, ScopeRoom, f.roomID, RoleEveryone, perm)
			return nil
		}, false},
		{"everyone server allow applies without other settings", func(t *testing.T, f fixture) error {
			return f.c.GrantServerPermission(f.ctx, SystemActorID, RoleEveryone, perm)
		}, true},
		{"everyone group allow applies in the group's rooms", func(t *testing.T, f fixture) error {
			return f.c.GrantGroupPermission(f.ctx, SystemActorID, f.groupID, RoleEveryone, perm)
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			user := createPermissionEditUser(t, c, ctx, "additive-user")
			roomID := createPermissionEditRoom(t, c, ctx, "additive-room")
			room, err := c.GetRoom(ctx, KindChannel, roomID)
			if err != nil {
				t.Fatalf("GetRoom: %v", err)
			}
			if _, err := c.CreateServerRole(ctx, SystemActorID, "helper", "Helper", "", false); err != nil {
				t.Fatalf("CreateServerRole: %v", err)
			}
			if err := c.AssignServerRole(ctx, SystemActorID, user, "helper"); err != nil {
				t.Fatalf("AssignServerRole: %v", err)
			}
			// Start without the default everyone allow.
			if err := c.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, perm); err != nil {
				t.Fatalf("ClearServerPermissionState: %v", err)
			}
			if err := tt.setup(t, fixture{c: c, ctx: ctx, user: user, roomID: roomID, groupID: room.GetGroupId()}); err != nil {
				t.Fatalf("setup: %v", err)
			}
			got, err := c.permissionResolver.HasRoomPermission(ctx, user, KindChannel, roomID, perm)
			if err != nil {
				t.Fatalf("HasRoomPermission: %v", err)
			}
			if got != tt.want {
				t.Fatalf("HasRoomPermission = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRolesOnlyGrant checks that roles, everyone included, only grant
// (ADR-116): role deny writes are rejected at every scope, and role denies
// that an earlier version stored have no effect on resolution, role grids,
// and role matrices, but show in the startup summary.
func TestRolesOnlyGrant(t *testing.T) {
	t.Parallel()

	const perm = PermMessageAttach
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	member := createPermissionEditUser(t, c, ctx, "roles-only-grant-member")
	denied := createPermissionEditUser(t, c, ctx, "roles-only-grant-denied")
	roomID := createPermissionEditRoom(t, c, ctx, "roles-only-grant-room")
	room, err := c.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	groupID := room.GetGroupId()
	if groupID == "" {
		t.Fatal("test room has no room group")
	}
	if _, err := c.CreateServerRole(ctx, SystemActorID, "helper", "Helper", "", false); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, member, "helper"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	owner := createPermissionEditUser(t, c, ctx, "roles-only-grant-owner")
	if err := c.AssignOwnerRole(ctx, owner); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}

	t.Run("role deny writes are rejected", func(t *testing.T) {
		targets := []struct {
			scope   PermissionTargetScope
			core    PermissionScope
			scopeID string
		}{
			{PermissionTargetScope{Kind: MatrixScopeServer}, ScopeServer, ""},
			{PermissionTargetScope{Kind: MatrixScopeGroup, ID: groupID}, ScopeGroup, groupID},
			{PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID}, ScopeRoom, roomID},
			{PermissionTargetScope{Kind: MatrixScopeDM}, ScopeDM, ""},
		}
		for _, roleName := range []string{"helper", RoleEveryone} {
			for _, target := range targets {
				before := c.rbacModel.decision(target.core, target.scopeID, roleName, perm)
				if err := c.SetRolePermissionState(ctx, SystemActorID, roleName, target.scope, perm, PermissionStateDeny); !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("%s deny at %s: error = %v, want ErrInvalidArgument", roleName, target.scope.Kind, err)
				}
				if after := c.rbacModel.decision(target.core, target.scopeID, roleName, perm); after != before {
					t.Fatalf("%s decision at %s changed from %s to %s", roleName, target.scope.Kind, before, after)
				}
			}
		}
		// A role allow is still accepted.
		if err := c.SetRolePermissionState(ctx, owner, "helper", PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID}, PermMessageManage, PermissionStateAllow); err != nil {
			t.Fatalf("helper allow at room: %v", err)
		}
		if got := c.rbacModel.decision(ScopeRoom, roomID, "helper", PermMessageManage); got != DecisionAllow {
			t.Fatalf("helper room decision = %s, want allow", got)
		}
	})

	// Store denies as an earlier version could.
	appendStoredRoleDeny(t, c, ctx, ScopeRoom, roomID, RoleEveryone, perm)
	appendStoredRoleDeny(t, c, ctx, ScopeGroup, groupID, RoleEveryone, perm)
	appendStoredRoleDeny(t, c, ctx, ScopeServer, "", "helper", perm)
	if got := c.rbacModel.decision(ScopeRoom, roomID, RoleEveryone, perm); got != DecisionDeny {
		t.Fatalf("stored everyone room decision = %s, want deny", got)
	}

	t.Run("stored role denies have no effect", func(t *testing.T) {
		// The everyone server allow still reaches the room, for a role holder
		// and for a member without roles.
		for _, userID := range []string{member, denied} {
			if got, err := c.ResolveUserPermission(ctx, userID, KindChannel, roomID, perm); err != nil || got != DecisionAllow {
				t.Fatalf("resolve %s in room = %s, %v; want allow", userID, got, err)
			}
		}
		if got, err := c.ResolveUserPermission(ctx, member, KindChannel, "", perm); err != nil || got != DecisionAllow {
			t.Fatalf("resolve role holder at server = %s, %v; want allow", got, err)
		}
		// A user deny still restricts.
		if err := c.DenyUserRoomPermission(ctx, SystemActorID, roomID, denied, perm); err != nil {
			t.Fatalf("DenyUserRoomPermission: %v", err)
		}
		if got, err := c.ResolveUserPermission(ctx, denied, KindChannel, roomID, perm); err != nil || got != DecisionDeny {
			t.Fatalf("resolve denied user in room = %s, %v; want deny", got, err)
		}
	})

	t.Run("the startup summary counts stored role denies", func(t *testing.T) {
		summary := c.ignoredRoleDenies()
		if summary.count != 3 {
			t.Fatalf("ignored role denies = %d, want 3", summary.count)
		}
		if !slices.Equal(summary.roomIDs, []string{roomID}) {
			t.Fatalf("affected rooms = %v, want [%s]", summary.roomIDs, roomID)
		}
		if !slices.Equal(summary.groupIDs, []string{groupID}) {
			t.Fatalf("affected groups = %v, want [%s]", summary.groupIDs, groupID)
		}
	})

	t.Run("role grids show no deny", func(t *testing.T) {
		for _, tier := range []struct {
			scope           PermissionScope
			roomID, groupID string
		}{{ScopeServer, "", ""}, {ScopeGroup, "", groupID}, {ScopeRoom, roomID, ""}} {
			tiers, err := c.buildTierRoles(ctx, tier.scope, tier.roomID, tier.groupID)
			if err != nil {
				t.Fatalf("buildTierRoles %s: %v", tier.scope, err)
			}
			found := 0
			for _, role := range tiers.Roles {
				if role.RoleName != RoleEveryone && role.RoleName != "helper" {
					continue
				}
				found++
				if !slices.Contains(role.EffectiveAllows, string(perm)) {
					t.Errorf("%s tier: %s effective allows %v, want %s", tier.scope, role.RoleName, role.EffectiveAllows, perm)
				}
				if tier.scope != ScopeServer && slices.Contains(role.Override.Permissions, string(perm)) {
					t.Errorf("%s tier: %s override %v shows a grant that was not set", tier.scope, role.RoleName, role.Override.Permissions)
				}
			}
			if found != 2 {
				t.Fatalf("%s tier: found %d of the roles everyone and helper", tier.scope, found)
			}
		}
	})

	t.Run("role matrices show no deny", func(t *testing.T) {
		for _, roleName := range []string{RoleEveryone, "helper"} {
			matrix, err := c.GetRolePermissionMatrix(ctx, owner, roleName)
			if err != nil {
				t.Fatalf("GetRolePermissionMatrix %s: %v", roleName, err)
			}
			checked := 0
			for _, cell := range matrix.Cells {
				if cell.Permission != string(perm) {
					continue
				}
				if cell.Override == MatrixDecisionDeny {
					t.Errorf("%s cell %s: override is deny", roleName, cell.ScopeID)
				}
				switch cell.ScopeID {
				case "server", "group:" + groupID, "room:" + roomID:
					checked++
					if cell.Effective != MatrixDecisionAllow {
						t.Errorf("%s cell %s: effective = %s, want allow from the everyone server allow", roleName, cell.ScopeID, cell.Effective)
					}
				}
			}
			if checked != 3 {
				t.Fatalf("%s matrix: checked %d of the server, group, and room cells", roleName, checked)
			}
		}
	})
}

// TestRoleGridsShowWhatARoleHolderGets checks that role grids use the
// resolver: a role's server allow shows as allowed in a room, and a
// permission that neither the role nor everyone has does not.
func TestRoleGridsShowWhatARoleHolderGets(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := createPermissionEditRoom(t, c, ctx, "role-grid-room")
	if err := c.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageAttach); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessageAttach); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	tiers, err := c.buildTierRoles(ctx, ScopeRoom, roomID, "")
	if err != nil {
		t.Fatalf("buildTierRoles: %v", err)
	}
	found := 0
	for _, role := range tiers.Roles {
		switch role.RoleName {
		case RoleModerator:
			found++
			if !slices.Contains(role.EffectiveAllows, string(PermMessageAttach)) {
				t.Errorf("moderator effective allows in room = %v, want message.attach", role.EffectiveAllows)
			}
			if !slices.Contains(role.InheritedAllows, string(PermMessageAttach)) {
				t.Errorf("moderator inherited allows = %v, want the server allow", role.InheritedAllows)
			}
		case RoleEveryone:
			found++
			if slices.Contains(role.EffectiveAllows, string(PermMessageAttach)) {
				t.Errorf("everyone effective allows in room = %v, want no message.attach", role.EffectiveAllows)
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d of the roles moderator and everyone in the tier grid", found)
	}
}
