package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRequireAuth(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("authenticated user succeeds", func(t *testing.T) {
		user, err := requireAuth(env.authContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("unauthenticated returns error", func(t *testing.T) {
		_, err := requireAuth(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireSelf(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("accessing own data succeeds", func(t *testing.T) {
		user, err := requireSelf(env.authContext(), env.testUser.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("accessing other user's data fails", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		_, err = requireSelf(env.authContext(), otherUser.Id)
		if !errors.Is(err, ErrNotSelf) {
			t.Errorf("Expected ErrNotSelf, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireSelf(env.unauthContext(), env.testUser.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireSpaceMember(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("space member succeeds", func(t *testing.T) {
		user, err := requireSpaceMember(env.authContext(), env.core, core.KindChannel)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireSpaceMember(env.unauthContext(), env.core, core.KindChannel)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireRoomMember(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("room member succeeds", func(t *testing.T) {
		user, err := requireRoomMember(env.authContext(), env.core, core.KindChannel, env.testRoom.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("space member but not room member fails", func(t *testing.T) {
		// Create a user who is a space member but not room member
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember", "Space Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		_, err = requireRoomMember(env.authContextForUser(spaceMember), env.core, core.KindChannel, env.testRoom.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("not even a space member fails", func(t *testing.T) {
		// Create a user who is not a member of the space at all
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		_, err = requireRoomMember(env.authContextForUser(outsider), env.core, core.KindChannel, env.testRoom.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireRoomMember(env.unauthContext(), env.core, core.KindChannel, env.testRoom.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireInstanceAdmin(t *testing.T) {
	adminEmail := "admin@example.com"
	env := setupTestResolverWithAdmin(t, []string{adminEmail})

	t.Run("config admin with verified email succeeds", func(t *testing.T) {
		// Note: Username "admin" is blocked by default, so we use "adminuser"
		admin, err := env.core.CreateUser(env.ctx, "system", "adminuser", "Admin", "password123")
		if err != nil {
			t.Fatalf("Failed to create admin: %v", err)
		}
		// Verify the admin email so they are recognized as config admin
		if err := env.core.AddVerifiedEmailDirect(env.ctx, admin.Id, adminEmail); err != nil {
			t.Fatalf("Failed to verify admin email: %v", err)
		}

		user, err := requireInstanceAdmin(env.authContextForUser(admin), env.core)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != admin.Id {
			t.Errorf("Expected user ID %s, got %s", admin.Id, user.Id)
		}
	})

	t.Run("RBAC admin succeeds", func(t *testing.T) {
		rbacAdmin, err := env.core.CreateUser(env.ctx, "system", "rbacadmin", "RBACAdmin", "password123")
		if err != nil {
			t.Fatalf("Failed to create RBAC admin: %v", err)
		}

		// Assign admin role via RBAC
		if err := env.core.AssignInstanceAdminRole(env.ctx, rbacAdmin.Id); err != nil {
			t.Fatalf("Failed to assign admin role: %v", err)
		}

		user, err := requireInstanceAdmin(env.authContextForUser(rbacAdmin), env.core)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != rbacAdmin.Id {
			t.Errorf("Expected user ID %s, got %s", rbacAdmin.Id, user.Id)
		}
	})

	t.Run("non-admin fails", func(t *testing.T) {
		// Create a second user who is NOT an admin (the first user from setupTestResolver
		// is auto-promoted to instance owner, so we need a fresh user)
		regularUser, err := env.core.CreateUser(env.ctx, "system", "regularuser", "Regular User", "password123")
		if err != nil {
			t.Fatalf("Failed to create regular user: %v", err)
		}

		_, err = requireInstanceAdmin(env.authContextForUser(regularUser), env.core)
		if !errors.Is(err, ErrNotInstanceAdmin) {
			t.Errorf("Expected ErrNotInstanceAdmin, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireInstanceAdmin(env.unauthContext(), env.core)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireInstancePermission(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("everyone has default permissions", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "member", "Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Everyone should have dm.view by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMView)
		if err != nil {
			t.Errorf("Expected user to have dm.view, got error: %v", err)
		}

		// Everyone should have dm.write by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMWrite)
		if err != nil {
			t.Errorf("Expected user to have dm.write, got error: %v", err)
		}
	})

	t.Run("everyone denial blocks permission", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "denied", "Denied", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Deny dm.write for everyone role
		if err := env.core.DenyInstancePermission(env.ctx, core.RoleEveryone, core.PermDMWrite); err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Permission should be denied
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMWrite)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("Expected ErrPermissionDenied (everyone role denial), got %v", err)
		}
	})

	t.Run("admin role grants admin permission", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "granted", "Granted", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Members don't have admin by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermAdminAccess)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("Expected ErrPermissionDenied for admin, got %v", err)
		}

		// Assign admin role
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleAdmin); err != nil {
			t.Fatalf("Failed to assign admin role: %v", err)
		}

		// Should now have access
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermAdminAccess)
		if err != nil {
			t.Errorf("Expected admin to work after role assignment, got error: %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireInstancePermission(env.unauthContext(), env.core, core.PermDMView)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

// TestGrantUserPermission_Authorization covers the gating on the new
// per-user grant/deny mutations. They use requireUserPermissionTarget:
// caller needs role.manage AND must strictly outrank the target. No
// self-bypass — self-grant would be a privilege boundary change, and
// the strict-outrank step fails on self by definition.
func TestGrantUserPermission_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// testUser starts as owner (created first in setupTestResolver).
	// Create three other users at different ranks for the matrix.
	regular := env.createVerifiedUser(t, "ugrant-regular", "Regular", "password123")
	moderator := env.createVerifiedUser(t, "ugrant-moderator", "Moderator", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, moderator.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderator: %v", err)
	}
	admin := env.createVerifiedUser(t, "ugrant-admin", "Admin", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	t.Run("regular user cannot grant permissions to anyone", func(t *testing.T) {
		_, err := mutation.GrantUserPermission(env.authContextForUser(regular), model.GrantUserPermissionInput{
			UserID:     moderator.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("moderator without role.manage cannot grant", func(t *testing.T) {
		// Moderator's default permissions don't include role.manage.
		_, err := mutation.GrantUserPermission(env.authContextForUser(moderator), model.GrantUserPermissionInput{
			UserID:     regular.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied (moderator lacks role.manage), got %v", err)
		}
	})

	t.Run("admin can grant to a lower-ranked user", func(t *testing.T) {
		_, err := mutation.GrantUserPermission(env.authContextForUser(admin), model.GrantUserPermissionInput{
			UserID:     regular.Id,
			Permission: string(core.PermMessageDeleteAny),
		})
		if err != nil {
			t.Errorf("expected admin grant to succeed, got %v", err)
		}
		// Verify the grant landed.
		decision, _ := env.core.ResolveUserPermission(env.ctx, regular.Id, core.KindChannel, "", core.PermMessageDeleteAny)
		if decision != core.DecisionAllow {
			t.Errorf("expected DecisionAllow after grant, got %s", decision)
		}
	})

	t.Run("admin cannot grant to a peer admin (peer-deny)", func(t *testing.T) {
		peerAdmin := env.createVerifiedUser(t, "ugrant-peer-admin", "Peer", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, peerAdmin.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		_, err := mutation.GrantUserPermission(env.authContextForUser(admin), model.GrantUserPermissionInput{
			UserID:     peerAdmin.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected peer-admin grant to be denied, got %v", err)
		}
	})

	t.Run("admin cannot grant to owner (admin doesn't outrank owner)", func(t *testing.T) {
		_, err := mutation.GrantUserPermission(env.authContextForUser(admin), model.GrantUserPermissionInput{
			UserID:     env.testUser.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected admin→owner grant to be denied, got %v", err)
		}
	})

	t.Run("self-grant is denied for every rank (privilege boundary change, no self-bypass)", func(t *testing.T) {
		// Self-grant is the original C-1 escalation vector: every authenticated
		// caller, regardless of rank, was able to attach arbitrary permissions
		// to themselves and bypass the security model. The new helper has no
		// self-bypass; the strict-outrank step (OutranksUser(self, self) =
		// false) denies on the rank side regardless of which permission the
		// caller holds.
		callers := []struct {
			name string
			user *corev1.User
		}{
			{"regular", regular},
			{"moderator", moderator},
			{"admin", admin},
			{"owner", env.testUser},
		}
		for _, c := range callers {
			_, err := mutation.GrantUserPermission(env.authContextForUser(c.user), model.GrantUserPermissionInput{
				UserID:     c.user.Id,
				Permission: string(core.PermDMView),
			})
			if !errors.Is(err, core.ErrPermissionDenied) {
				t.Errorf("%s self-grant: expected ErrPermissionDenied, got %v", c.name, err)
			}
		}
	})

	t.Run("denyUserPermission and clearUserPermissionState share the same gate", func(t *testing.T) {
		// Moderator should be denied for both, same as grant.
		_, err := mutation.DenyUserPermission(env.authContextForUser(moderator), model.DenyUserPermissionInput{
			UserID:     regular.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("denyUserPermission: expected ErrPermissionDenied, got %v", err)
		}
		_, err = mutation.ClearUserPermissionState(env.authContextForUser(moderator), model.ClearUserPermissionStateInput{
			UserID:     regular.Id,
			Permission: string(core.PermMessagePost),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("clearUserPermissionState: expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can deny then clear a permission on a lower-ranked user", func(t *testing.T) {
		// End-to-end roundtrip via GraphQL.
		_, err := mutation.DenyUserPermission(env.authContextForUser(admin), model.DenyUserPermissionInput{
			UserID:     regular.Id,
			Permission: string(core.PermDMWrite),
		})
		if err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		decision, _ := env.core.ResolveUserPermission(env.ctx, regular.Id, core.KindChannel, "", core.PermDMWrite)
		if decision != core.DecisionDeny {
			t.Fatalf("expected DecisionDeny after admin deny, got %s", decision)
		}

		_, err = mutation.ClearUserPermissionState(env.authContextForUser(admin), model.ClearUserPermissionStateInput{
			UserID:     regular.Id,
			Permission: string(core.PermDMWrite),
		})
		if err != nil {
			t.Fatalf("ClearUserPermissionState: %v", err)
		}
		decision, _ = env.core.ResolveUserPermission(env.ctx, regular.Id, core.KindChannel, "", core.PermDMWrite)
		if decision != core.DecisionAllow {
			t.Errorf("expected DecisionAllow after clear (everyone default), got %s", decision)
		}
	})

	t.Run("room-scoped user grant lands at the room and not server-wide", func(t *testing.T) {
		// Fresh user to avoid state from prior subtests.
		fresh := env.createVerifiedUser(t, "ugrant-fresh-room", "Fresh", "password123")
		room, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "ugrant-room", "Room")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		roomID := room.Id
		_, err = mutation.GrantUserPermission(env.authContextForUser(admin), model.GrantUserPermissionInput{
			UserID:     fresh.Id,
			Permission: string(core.PermMessageEditAny),
			RoomID:     &roomID,
		})
		if err != nil {
			t.Fatalf("GrantUserPermission (room): %v", err)
		}
		// Room-scoped: allow in this room.
		decision, _ := env.core.ResolveUserPermission(env.ctx, fresh.Id, core.KindChannel, roomID, core.PermMessageEditAny)
		if decision != core.DecisionAllow {
			t.Errorf("expected DecisionAllow in granted room, got %s", decision)
		}
		// Other room: no effect.
		other, _ := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "ugrant-other", "Other")
		decision, _ = env.core.ResolveUserPermission(env.ctx, fresh.Id, core.KindChannel, other.Id, core.PermMessageEditAny)
		if decision == core.DecisionAllow {
			t.Errorf("expected room-scoped grant not to leak to other rooms, got %s", decision)
		}
	})
}

// TestRequireUserAdminTarget covers the two-step "permission AND rank" gate
// for targeted user mutations (issue #435). The critical regression case is
// the rank-only bug: a moderator outranks regular members but does NOT have
// role.assign, so a moderator must be denied — even though hierarchy alone
// would allow it.
func TestRequireUserAdminTarget(t *testing.T) {
	env := setupTestResolver(t)

	regular := env.createVerifiedUser(t, "regular", "Regular", "password123")
	moderator := env.createVerifiedUser(t, "moderator", "Moderator", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, moderator.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole moderator: %v", err)
	}
	admin := env.createVerifiedUser(t, "adminuser", "Admin", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	t.Run("self is always allowed", func(t *testing.T) {
		if err := env.resolver.requireUserAdminTarget(env.ctx, regular.Id, regular.Id); err != nil {
			t.Errorf("self should be allowed, got %v", err)
		}
	})

	t.Run("moderator without role.assign cannot target lower-ranked user", func(t *testing.T) {
		// This is the #435 regression: rank-only gating would allow this.
		// The new "permission AND rank" gate must deny it.
		err := env.resolver.requireUserAdminTarget(env.ctx, moderator.Id, regular.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("moderator without role.assign should be denied, got %v", err)
		}
	})

	t.Run("admin with role.assign can target lower-ranked user", func(t *testing.T) {
		if err := env.resolver.requireUserAdminTarget(env.ctx, admin.Id, regular.Id); err != nil {
			t.Errorf("admin should be allowed, got %v", err)
		}
	})

	t.Run("admin cannot target peer admin (rank check)", func(t *testing.T) {
		admin2 := env.createVerifiedUser(t, "adminuser2", "Admin Two", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin2.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole admin2: %v", err)
		}
		err := env.resolver.requireUserAdminTarget(env.ctx, admin.Id, admin2.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("peer admins should not be able to target each other, got %v", err)
		}
	})

	t.Run("regular user without permissions cannot target anyone else", func(t *testing.T) {
		other := env.createVerifiedUser(t, "other", "Other", "password123")
		err := env.resolver.requireUserAdminTarget(env.ctx, regular.Id, other.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("regular user should be denied, got %v", err)
		}
	})
}
