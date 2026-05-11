package core

import (
	"context"
	"testing"
)

// TestPermissionExplainer_AgreesWithHas asserts that for every fixture and every
// applicable permission, Has*Permission and Explain*Permission produce the same
// bool result. This is the safety net guaranteeing the visitor walker can't drift
// from the bool path: both share the same walk*Permission function, but a future
// refactor that breaks one must break the other equally.
//
// It also asserts trace structure: when bool is true the first entry is allow;
// when bool is false but trace is non-empty, the first entry is deny.
func TestPermissionExplainer_AgreesWithHas(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Three subjects with distinct role configurations:
	//   regular: just everyone
	//   adminUser: instance admin role
	//   denyUser: custom instance role denying space.list
	regular, _ := core.CreateUser(ctx, SystemActorID, "regular", "Regular", "password123")
	adminUser, _ := core.CreateUser(ctx, SystemActorID, "adminuser", "Admin User", "password123")
	if err := core.AssignInstanceRole(ctx, SystemActorID, adminUser.Id, RoleAdmin); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	denyUser, _ := core.CreateUser(ctx, SystemActorID, "denyuser", "Deny User", "password123")
	if _, err := core.CreateInstanceRole(ctx, "denytest", "Deny dm.view", "Test deny role"); err != nil {
		t.Fatalf("create deny role: %v", err)
	}
	if err := core.DenyInstancePermission(ctx, "denytest", PermDMView); err != nil {
		t.Fatalf("deny perm: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, denyUser.Id, "denytest"); err != nil {
		t.Fatalf("assign deny role: %v", err)
	}

	// A space owned by adminUser, with an extra member (regular) and a non-member (denyUser).
	space, _ := core.CreateSpace(ctx, adminUser.Id, "Test Space", "")

	// A room in the space; adminUser is auto-member of all rooms (creator).
	room, err := core.CreateRoom(ctx, adminUser.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, err := core.JoinRoom(ctx, regular.Id, space.Id, regular.Id, room.Id); err != nil {
		t.Fatalf("regular joins room: %v", err)
	}

	// Room-level override: deny message.post for the everyone space role in this
	// room. Higher-rank roles (owner) should still post via the hierarchy walk.
	if err := core.DenyRoomPermission(ctx, room.Id, "everyone", PermMessagePost); err != nil {
		t.Fatalf("deny room perm: %v", err)
	}

	subjects := []struct {
		name string
		id   string
	}{
		{"regular", regular.Id},
		{"adminUser", adminUser.Id},
		{"denyUser", denyUser.Id},
	}

	t.Run("instance scope", func(t *testing.T) {
		for _, s := range subjects {
			s := s
			t.Run(s.name, func(t *testing.T) {
				for _, meta := range PermissionsForScope(ScopeServer) {
					assertAgreement(t, ctx, core, s.id, "", "", meta.Permission, ScopeServer)
				}
			})
		}
	})

	t.Run("space scope", func(t *testing.T) {
		for _, s := range subjects {
			s := s
			t.Run(s.name, func(t *testing.T) {
				for _, meta := range PermissionsForScope(ScopeSpace) {
					assertAgreement(t, ctx, core, s.id, space.Id, "", meta.Permission, ScopeSpace)
				}
			})
		}
	})

	t.Run("room scope", func(t *testing.T) {
		for _, s := range subjects {
			s := s
			t.Run(s.name, func(t *testing.T) {
				for _, meta := range PermissionsForScope(ScopeRoom) {
					assertAgreement(t, ctx, core, s.id, space.Id, room.Id, meta.Permission, ScopeRoom)
				}
			})
		}
	})

	t.Run("ExplainAllPermissions matches scope", func(t *testing.T) {
		for _, s := range subjects {
			s := s
			t.Run(s.name+"/instance", func(t *testing.T) {
				exps, err := core.permissionResolver.ExplainAllPermissions(ctx, s.id, "", "")
				if err != nil {
					t.Fatalf("ExplainAllPermissions: %v", err)
				}
				if got, want := len(exps), len(PermissionsForScope(ScopeServer)); got != want {
					t.Errorf("instance: got %d explanations, want %d", got, want)
				}
			})
			t.Run(s.name+"/space", func(t *testing.T) {
				exps, err := core.permissionResolver.ExplainAllPermissions(ctx, s.id, space.Id, "")
				if err != nil {
					t.Fatalf("ExplainAllPermissions: %v", err)
				}
				if got, want := len(exps), len(PermissionsForScope(ScopeSpace)); got != want {
					t.Errorf("space: got %d explanations, want %d", got, want)
				}
			})
			t.Run(s.name+"/room", func(t *testing.T) {
				exps, err := core.permissionResolver.ExplainAllPermissions(ctx, s.id, space.Id, room.Id)
				if err != nil {
					t.Fatalf("ExplainAllPermissions: %v", err)
				}
				if got, want := len(exps), len(PermissionsForScope(ScopeRoom)); got != want {
					t.Errorf("room: got %d explanations, want %d", got, want)
				}
			})
		}
	})

	t.Run("roomID without spaceID is an error", func(t *testing.T) {
		if _, err := core.permissionResolver.ExplainAllPermissions(ctx, regular.Id, "", room.Id); err == nil {
			t.Error("expected error for roomID without spaceID")
		}
	})
}

// assertAgreement verifies Has*Permission and Explain*Permission produce
// consistent results for a single (user, scope, permission) tuple.
func assertAgreement(
	t *testing.T,
	ctx context.Context,
	core *ChattoCore,
	userID, spaceID, roomID string,
	perm Permission,
	scope PermissionScope,
) {
	t.Helper()

	var (
		hasResult bool
		hasErr    error
		exp       PermissionExplanation
		expErr    error
	)
	switch scope {
	case ScopeServer:
		hasResult, hasErr = core.permissionResolver.HasInstancePermission(ctx, userID, perm)
		exp, expErr = core.permissionResolver.ExplainInstancePermission(ctx, userID, perm)
	case ScopeSpace:
		hasResult, hasErr = core.permissionResolver.HasSpacePermission(ctx, userID, spaceID, perm)
		exp, expErr = core.permissionResolver.ExplainSpacePermission(ctx, userID, spaceID, perm)
	case ScopeRoom:
		hasResult, hasErr = core.permissionResolver.HasRoomPermission(ctx, userID, spaceID, roomID, perm)
		exp, expErr = core.permissionResolver.ExplainRoomPermission(ctx, userID, spaceID, roomID, perm)
	default:
		t.Fatalf("unknown scope %v", scope)
	}

	if (hasErr == nil) != (expErr == nil) {
		t.Errorf("perm %s: error mismatch — has=%v explain=%v", perm, hasErr, expErr)
		return
	}
	if hasErr != nil {
		return
	}

	expectedAllow := exp.State == DecisionAllow
	if hasResult != expectedAllow {
		t.Errorf("perm %s (%s/%s/%s): Has=%v but Explain.State=%s (decidedAt=%s by=%s)",
			perm, userID, spaceID, roomID, hasResult, exp.State, exp.DecidedAt, exp.DecidedByRole)
	}

	// Trace structure invariants.
	if hasResult {
		if len(exp.Trace) == 0 {
			t.Errorf("perm %s: Has=true but trace is empty", perm)
		} else if exp.Trace[0].Decision != DecisionAllow {
			t.Errorf("perm %s: Has=true but first trace entry is %s (expected allow)", perm, exp.Trace[0].Decision)
		}
	} else if len(exp.Trace) > 0 && exp.Trace[0].Decision != DecisionDeny {
		t.Errorf("perm %s: Has=false but first trace entry is %s (expected deny when trace non-empty)", perm, exp.Trace[0].Decision)
	}

	// State / DecidedAt / DecidedByRole must match the first trace entry.
	if len(exp.Trace) > 0 {
		first := exp.Trace[0]
		if exp.State != first.Decision {
			t.Errorf("perm %s: State=%s but first trace decision=%s", perm, exp.State, first.Decision)
		}
		if exp.DecidedAt != first.Level {
			t.Errorf("perm %s: DecidedAt=%s but first trace level=%s", perm, exp.DecidedAt, first.Level)
		}
		if exp.DecidedByRole != first.RoleName {
			t.Errorf("perm %s: DecidedByRole=%s but first trace role=%s", perm, exp.DecidedByRole, first.RoleName)
		}
	} else {
		if exp.State != DecisionNone {
			t.Errorf("perm %s: empty trace but State=%s (expected none)", perm, exp.State)
		}
	}
}
