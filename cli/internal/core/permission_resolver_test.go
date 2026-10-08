package core

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	// everyone can deny only below server scope (ADR-116). Thus the table sets
	// everyone at room scope, without server settings to fall back to.
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
					err = core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, permission)
				case PermissionStateDeny:
					err = core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, permission)
				default:
					err = core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, permission)
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
		// Grant the permission to the user. everyone cannot deny at server
		// scope (ADR-116), so the user is the subject.
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

func TestPermissionResolver_HasRoomPermission_RoomDenialOverridesSpaceGrant(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomdeny1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomdeny1member", "Member", "password123")
	// Ensure message.post is granted at space level
	core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)

	// Deny at room level
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected room denial to block space grant")
	}
}

func TestPermissionResolver_HasRoomPermission_NearestDecisionForSameRole(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// everyone can deny only below server scope (ADR-116), so the broader
	// deny is at room-group scope.
	group, err := core.CreateRoomGroup(ctx, SystemActorID, "Override Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "general", "General")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	member, _ := core.CreateUser(ctx, "system", "roomoverridemember", "Member", "password123")
	if err := core.DenyGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("DenyGroupPermission: %v", err)
	}
	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected room grant to override group deny for the same role")
	}
}

func TestPermissionResolver_HasRoomPermission_ConflictingRoles(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "conflictroleadmin", "Admin", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "conflictrolemember", "Member", "password123")
	// Create a custom role.
	core.CreateServerRole(ctx, SystemActorID, "poster", "Poster", "Can post")

	// Grant message.post to poster role at room level
	core.GrantRoomPermission(ctx, SystemActorID, room.Id, "poster", PermMessagePost)

	// Deny message.post for everyone role at room level
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

	// Assign poster role to member (member now has: everyone + poster)
	core.AssignServerRole(ctx, admin.Id, member.Id, "poster")

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected poster role grant to override the everyone baseline denial")
	}
}

func TestPermissionResolver_HasRoomPermission_EveryoneDenyBlocksLessSpecificNamedAllow(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, _ := core.CreateUser(ctx, SystemActorID, "scoped-baseline-owner", "Owner", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole owner: %v", err)
	}
	room, _ := core.CreateRoom(ctx, owner.Id, KindChannel, "", "scoped-baseline", "Scoped Baseline")
	admin, _ := core.CreateUser(ctx, SystemActorID, "scoped-baseline-admin", "Admin", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleAdmin, PermRoomList); err != nil {
		t.Fatalf("GrantServerPermission admin: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission everyone: %v", err)
	}

	has, err := core.permissionResolver.HasRoomPermission(ctx, admin.Id, KindChannel, room.Id, PermRoomList)
	if err != nil {
		t.Fatalf("HasRoomPermission: %v", err)
	}
	if has {
		t.Error("expected room everyone deny to block the less-specific admin server allow")
	}

	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleAdmin, PermRoomList); err != nil {
		t.Fatalf("GrantRoomPermission admin: %v", err)
	}
	has, err = core.permissionResolver.HasRoomPermission(ctx, admin.Id, KindChannel, room.Id, PermRoomList)
	if err != nil {
		t.Fatalf("HasRoomPermission after room allow: %v", err)
	}
	if !has {
		t.Error("expected same-scope admin room allow to override the everyone room deny")
	}
}

func TestPermissionResolver_HasRoomPermission_IsolationBetweenRooms(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomisoadmin", "Admin", "password123")
	roomA, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "rooma", "Room A")
	roomB, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "roomb", "Room B")

	member, _ := core.CreateUser(ctx, "system", "roomisomember", "Member", "password123")
	// Ensure message.post is granted at space level for everyone
	core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)

	// Deny message.post only in room A
	core.DenyRoomPermission(ctx, SystemActorID, roomA.Id, RoleEveryone, PermMessagePost)

	// Room A: denied
	hasA, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomA.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasA {
		t.Error("Expected member to be denied in room A")
	}

	// Room B: allowed (no room override, falls back to space grant)
	hasB, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomB.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasB {
		t.Error("Expected member to have permission in room B (no override)")
	}
}

func TestPermissionResolver_HasRoomPermission_ServerRoleRoomDenial(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomdeny1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomdeny1member", "Member", "password123")
	// Ensure message.post is granted at space level
	core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)

	// Deny message.post for instance-everyone at room level
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected role room denial to block permission")
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
	// Grant at space level
	core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)

	// Deny at room level
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

	// Verify denied
	has, _ := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if has {
		t.Fatal("Setup error: expected room denial to block")
	}

	// Clear room override
	core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

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
	// Grant message.post at room level, deny message.react at room level
	core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)

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

	t.Run("user-level room grant overrides everyone group baseline deny", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, SystemActorID, "user-room-grant", "User", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "private", "Private")
		groupID := room.GroupId
		if err := core.DenyGroupPermission(ctx, SystemActorID, groupID, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("DenyGroupPermission: %v", err)
		}
		// Without the user-grant, user can't post.
		has, _ := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if has {
			t.Fatal("baseline: user should be denied by everyone-role set deny")
		}
		// User-level room grant.
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermMessagePost); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if !has {
			t.Error("expected user-level room grant to override everyone group baseline deny")
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

	// everyone cannot deny at server scope (ADR-116); a user deny can.
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

func TestPermissionResolver_RoomOverridesServerForSameRole(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "hieradmin", "Admin User", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "General", "General chat")

	member, _ := core.CreateUser(ctx, "system", "hiermember", "Member User", "password123")

	t.Run("room grant overrides server deny on the same subject", func(t *testing.T) {
		// everyone cannot deny at server scope (ADR-116), so the user is the
		// subject.
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

	t.Run("room deny overrides server grant on the same role", func(t *testing.T) {
		if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("GrantServerPermission: %v", err)
		}
		if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if has {
			t.Error("expected room deny to override server grant for the same role")
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
// deny on the user decides; otherwise an allow of the user or a role at the
// same scope as everyone's nearest setting, or a more specific one, allows;
// otherwise everyone's setting decides.
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
		// A server-level allow on one user must not open a room that denies
		// everyone, or a delegated manager could grant access to rooms that
		// they cannot enter themselves.
		{"user allow loses to a more specific everyone deny", func(t *testing.T, f fixture) error {
			if err := f.c.DenyRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.GrantUserPermission(f.ctx, SystemActorID, f.user, perm)
		}, false},
		{"user allow beats everyone deny at the same scope", func(t *testing.T, f fixture) error {
			if err := f.c.DenyRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.GrantUserRoomPermission(f.ctx, SystemActorID, f.roomID, f.user, perm)
		}, true},
		{"user deny beats everyone allow", func(t *testing.T, f fixture) error {
			if err := f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.DenyUserPermission(f.ctx, SystemActorID, f.user, perm)
		}, false},
		{"nearest user setting wins", func(t *testing.T, f fixture) error {
			if err := f.c.DenyUserPermission(f.ctx, SystemActorID, f.user, perm); err != nil {
				return err
			}
			return f.c.GrantUserRoomPermission(f.ctx, SystemActorID, f.roomID, f.user, perm)
		}, true},
		{"role allow beats everyone deny at the same scope", func(t *testing.T, f fixture) error {
			if err := f.c.DenyRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, "helper", perm)
		}, true},
		{"role allow at a room beats everyone deny at the group", func(t *testing.T, f fixture) error {
			if err := f.c.DenyGroupPermission(f.ctx, SystemActorID, f.groupID, RoleEveryone, perm); err != nil {
				return err
			}
			return f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, "helper", perm)
		}, true},
		{"role allow loses to a more specific everyone deny", func(t *testing.T, f fixture) error {
			if err := f.c.GrantServerPermission(f.ctx, SystemActorID, "helper", perm); err != nil {
				return err
			}
			return f.c.DenyRoomPermission(f.ctx, SystemActorID, f.roomID, RoleEveryone, perm)
		}, false},
		{"everyone allow applies without other settings", func(t *testing.T, f fixture) error {
			return f.c.GrantServerPermission(f.ctx, SystemActorID, RoleEveryone, perm)
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

func TestRoleDenyWritesAreRejectedAndStoredRoleDeniesHaveNoEffect(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user := createPermissionEditUser(t, c, ctx, "stored-deny-user")
	if _, err := c.CreateServerRole(ctx, SystemActorID, "suspended", "Suspended", "", false); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, user, "suspended"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	if err := c.SetRolePermissionState(ctx, SystemActorID, "suspended", PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateDeny); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("SetRolePermissionState deny for a role: error = %v, want ErrInvalidArgument", err)
	}

	// A deny stored by an earlier version has no effect.
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacRolePermissionDeniedEvent(ScopeServer, "", "suspended", PermMessagePost),
	}})
	if _, err := c.appendRBACEvent(ctx, event, nil); err != nil {
		t.Fatalf("append stored role deny: %v", err)
	}
	if got := c.rbacModel.decision(ScopeServer, "", "suspended", PermMessagePost); got != DecisionDeny {
		t.Fatalf("stored role decision = %s, want deny", got)
	}
	allowed, err := c.permissionResolver.HasServerPermission(ctx, user, PermMessagePost)
	if err != nil {
		t.Fatalf("HasServerPermission: %v", err)
	}
	if !allowed {
		t.Fatal("a stored role deny restricted a role holder")
	}
	if got := c.ignoredRoleDenyCount(); got != 1 {
		t.Fatalf("ignored role denies = %d, want 1", got)
	}
}

// TestEveryoneDeniesOnlyBelowServerScope checks ADR-116's scope limit for
// everyone denies: a server-scope deny is rejected, and a stored one has no
// effect, while denies at room group, room, and direct-message scope still
// restrict members.
func TestEveryoneDeniesOnlyBelowServerScope(t *testing.T) {
	t.Parallel()

	const perm = PermMessageAttach
	type fixture struct {
		c       *ChattoCore
		ctx     context.Context
		member  string
		roomID  string
		groupID string
	}
	setup := func(t *testing.T) fixture {
		t.Helper()
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		member := createPermissionEditUser(t, c, ctx, "everyone-deny-member")
		roomID := createPermissionEditRoom(t, c, ctx, "everyone-deny-room")
		room, err := c.GetRoom(ctx, KindChannel, roomID)
		if err != nil {
			t.Fatalf("GetRoom: %v", err)
		}
		if room.GetGroupId() == "" {
			t.Fatal("test room has no room group")
		}
		return fixture{c: c, ctx: ctx, member: member, roomID: roomID, groupID: room.GetGroupId()}
	}

	t.Run("server-scope deny writes are rejected", func(t *testing.T) {
		t.Parallel()
		f := setup(t)
		if err := f.c.SetRolePermissionState(f.ctx, SystemActorID, RoleEveryone, PermissionTargetScope{Kind: MatrixScopeServer}, perm, PermissionStateDeny); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("SetRolePermissionState server deny for everyone: error = %v, want ErrInvalidArgument", err)
		}
		if got := f.c.rbacModel.decision(ScopeServer, "", RoleEveryone, perm); got != DecisionAllow {
			t.Fatalf("stored everyone server decision = %s, want the default allow", got)
		}
	})

	for _, scopeKind := range []MatrixScopeKind{MatrixScopeGroup, MatrixScopeRoom, MatrixScopeDM} {
		t.Run(string(scopeKind)+" deny restricts a member", func(t *testing.T) {
			t.Parallel()
			f := setup(t)
			editor := createPermissionEditUser(t, f.c, f.ctx, "everyone-deny-editor")
			if err := f.c.GrantUserPermission(f.ctx, SystemActorID, editor, PermRoleManage); err != nil {
				t.Fatalf("GrantUserPermission: %v", err)
			}
			grantTestRank(t, f.c, f.ctx, editor)
			scope := PermissionTargetScope{Kind: scopeKind}
			kind, roomID := KindChannel, f.roomID
			switch scopeKind {
			case MatrixScopeGroup:
				scope.ID = f.groupID
			case MatrixScopeRoom:
				scope.ID = f.roomID
			case MatrixScopeDM:
				kind, roomID = KindDM, ""
			}
			if got, err := f.c.ResolveUserPermission(f.ctx, f.member, kind, roomID, perm); err != nil || got != DecisionAllow {
				t.Fatalf("before deny = %s, %v; want the default allow", got, err)
			}
			if err := f.c.SetRolePermissionState(f.ctx, editor, RoleEveryone, scope, perm, PermissionStateDeny); err != nil {
				t.Fatalf("SetRolePermissionState deny for everyone: %v", err)
			}
			if got, err := f.c.ResolveUserPermission(f.ctx, f.member, kind, roomID, perm); err != nil || got != DecisionDeny {
				t.Fatalf("after deny = %s, %v; want deny", got, err)
			}
		})
	}

	t.Run("a stored server-scope deny has no effect", func(t *testing.T) {
		t.Parallel()
		f := setup(t)
		// Start without the default everyone allow, then store a deny as an
		// earlier version could.
		if err := f.c.ClearServerPermissionState(f.ctx, SystemActorID, RoleEveryone, perm); err != nil {
			t.Fatalf("ClearServerPermissionState: %v", err)
		}
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{
			RbacPermissionDenied: rbacRolePermissionDeniedEvent(ScopeServer, "", RoleEveryone, perm),
		}})
		if _, err := f.c.appendRBACEvent(f.ctx, event, nil); err != nil {
			t.Fatalf("append stored everyone deny: %v", err)
		}
		if got := f.c.rbacModel.decision(ScopeServer, "", RoleEveryone, perm); got != DecisionDeny {
			t.Fatalf("stored everyone decision = %s, want deny", got)
		}

		// Without allows, the result is the same as with no setting.
		for _, target := range []struct {
			kind   RoomKind
			roomID string
		}{{KindChannel, ""}, {KindChannel, f.roomID}, {KindDM, ""}} {
			if got, err := f.c.ResolveUserPermission(f.ctx, f.member, target.kind, target.roomID, perm); err != nil || got != DecisionNone {
				t.Fatalf("resolve %s %q = %s, %v; want none", target.kind, target.roomID, got, err)
			}
		}

		// Role grids show no server-scope deny for everyone.
		for _, tier := range []struct {
			scope   PermissionScope
			groupID string
		}{{ScopeServer, ""}, {ScopeGroup, f.groupID}} {
			tiers, err := f.c.buildTierRoles(f.ctx, tier.scope, "", tier.groupID)
			if err != nil {
				t.Fatalf("buildTierRoles %s: %v", tier.scope, err)
			}
			for _, role := range tiers.Roles {
				if role.RoleName != RoleEveryone {
					continue
				}
				if slices.Contains(role.Override.PermissionDenials, string(perm)) || slices.Contains(role.InheritedDenials, string(perm)) || slices.Contains(role.EffectiveDenials, string(perm)) {
					t.Fatalf("%s tier shows the stored server deny: override %v, inherited %v, effective %v", tier.scope, role.Override.PermissionDenials, role.InheritedDenials, role.EffectiveDenials)
				}
			}
		}

		// A role allow at room scope applies.
		if _, err := f.c.CreateServerRole(f.ctx, SystemActorID, "helper", "Helper", "", false); err != nil {
			t.Fatalf("CreateServerRole: %v", err)
		}
		if err := f.c.AssignServerRole(f.ctx, SystemActorID, f.member, "helper"); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		if err := f.c.GrantRoomPermission(f.ctx, SystemActorID, f.roomID, "helper", perm); err != nil {
			t.Fatalf("GrantRoomPermission: %v", err)
		}
		if got, err := f.c.ResolveUserPermission(f.ctx, f.member, KindChannel, f.roomID, perm); err != nil || got != DecisionAllow {
			t.Fatalf("room with role allow = %s, %v; want allow", got, err)
		}
	})
}

// TestRoleGridsShowWhatARoleHolderGets checks that role grids use the
// resolver: a server-level role allow does not show as allowed in a room
// that denies everyone.
func TestRoleGridsShowWhatARoleHolderGets(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := createPermissionEditRoom(t, c, ctx, "role-grid-room")
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessageAttach); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}
	if err := c.DenyRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, PermMessageAttach); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}

	tiers, err := c.buildTierRoles(ctx, ScopeRoom, roomID, "")
	if err != nil {
		t.Fatalf("buildTierRoles: %v", err)
	}
	for _, role := range tiers.Roles {
		if role.RoleName != RoleModerator {
			continue
		}
		if !slices.Contains(role.EffectiveDenials, string(PermMessageAttach)) || slices.Contains(role.EffectiveAllows, string(PermMessageAttach)) {
			t.Fatalf("moderator effective in room = allows %v, denials %v; want message.attach denied", role.EffectiveAllows, role.EffectiveDenials)
		}
		if !slices.Contains(role.InheritedAllows, string(PermMessageAttach)) {
			t.Fatalf("moderator inherited allows = %v, want the server allow", role.InheritedAllows)
		}
		return
	}
	t.Fatal("moderator missing from the tier grid")
}
