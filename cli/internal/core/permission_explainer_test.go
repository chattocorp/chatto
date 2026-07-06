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
// It also asserts trace structure under the deny-wins model: if any trace
// entry is a deny, the explanation state is deny; otherwise any allow trace
// produces allow.
func TestPermissionExplainer_AgreesWithHas(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Three subjects with distinct role configurations:
	//   regular: just everyone
	//   adminUser: admin role
	//   denyUser: custom role denying message.post
	regular, _ := core.CreateUser(ctx, SystemActorID, "regular", "Regular", "password123")
	adminUser, _ := core.CreateUser(ctx, SystemActorID, "adminuser", "Admin User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, adminUser.Id, RoleAdmin); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	denyUser, _ := core.CreateUser(ctx, SystemActorID, "denyuser", "Deny User", "password123")
	if _, err := core.CreateServerRole(ctx, SystemActorID, "denytest", "Deny message.post", "Test deny role"); err != nil {
		t.Fatalf("create deny role: %v", err)
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, "denytest", PermMessagePost); err != nil {
		t.Fatalf("deny perm: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, denyUser.Id, "denytest"); err != nil {
		t.Fatalf("assign deny role: %v", err)
	}

	// A space owned by adminUser, with an extra member (regular) and a non-member (denyUser).

	// A room in the space; adminUser is auto-member of all rooms (creator).
	room, err := core.CreateRoom(ctx, adminUser.Id, KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, err := core.JoinRoom(ctx, regular.Id, KindChannel, regular.Id, room.Id); err != nil {
		t.Fatalf("regular joins room: %v", err)
	}

	// Room-level override: deny message.post for the everyone role in this room.
	// Owners should still post via the effective-owner override.
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, "everyone", PermMessagePost); err != nil {
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

	t.Run("room scope", func(t *testing.T) {
		for _, s := range subjects {
			s := s
			t.Run(s.name, func(t *testing.T) {
				for _, meta := range PermissionsForScope(ScopeRoom) {
					assertAgreement(t, ctx, core, s.id, LegacyServerSpaceID, room.Id, meta.Permission, ScopeRoom)
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
			t.Run(s.name+"/room", func(t *testing.T) {
				exps, err := core.permissionResolver.ExplainAllPermissions(ctx, s.id, KindChannel, room.Id)
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
		hasResult, hasErr = core.permissionResolver.HasServerPermission(ctx, userID, perm)
		exp, expErr = core.permissionResolver.ExplainServerPermission(ctx, userID, perm)
	case ScopeRoom:
		hasResult, hasErr = core.permissionResolver.HasRoomPermission(ctx, userID, RoomKindFromLegacySpaceID(spaceID), roomID, perm)
		exp, expErr = core.permissionResolver.ExplainRoomPermission(ctx, userID, RoomKindFromLegacySpaceID(spaceID), roomID, perm)
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

	// State / DecidedAt / DecidedByRole must match the winning trace entry.
	if len(exp.Trace) > 0 {
		winning := exp.Trace[0]
		for _, entry := range exp.Trace {
			if entry.Decision == DecisionDeny {
				winning = entry
			}
		}
		if exp.State != winning.Decision {
			t.Errorf("perm %s: State=%s but winning trace decision=%s", perm, exp.State, winning.Decision)
		}
		if exp.DecidedAt != winning.Level {
			t.Errorf("perm %s: DecidedAt=%s but winning trace level=%s", perm, exp.DecidedAt, winning.Level)
		}
		if exp.DecidedByRole != winning.RoleName {
			t.Errorf("perm %s: DecidedByRole=%s but winning trace role=%s", perm, exp.DecidedByRole, winning.RoleName)
		}
	} else {
		if exp.State != DecisionNone {
			t.Errorf("perm %s: empty trace but State=%s (expected none)", perm, exp.State)
		}
	}
}

// TestPermissionExplainer_UserLevelTrace asserts that the explainer surfaces
// user-level grants and denies in the trace, attributed to the user (subject
// = userID). Without this, the inspector UI would silently miss user-level
// overrides applied via grantUserPermission / denyUserPermission.
func TestPermissionExplainer_UserLevelTrace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, SystemActorID, "explainer-user-level", "User", "password123")

	t.Run("server-level user grant appears in trace", func(t *testing.T) {
		if err := core.GrantUserPermission(ctx, SystemActorID, user.Id, PermAdminUsersView); err != nil {
			t.Fatalf("GrantUserPermission: %v", err)
		}
		exp, err := core.permissionResolver.ExplainServerPermission(ctx, user.Id, PermAdminUsersView)
		if err != nil {
			t.Fatalf("ExplainServerPermission: %v", err)
		}
		if exp.State != DecisionAllow {
			t.Errorf("expected DecisionAllow, got %s", exp.State)
		}
		// First trace entry should be the user-level grant.
		if len(exp.Trace) == 0 {
			t.Fatal("expected non-empty trace")
		}
		if exp.Trace[0].RoleName != user.Id {
			t.Errorf("expected first trace entry attributed to user %s, got %s", user.Id, exp.Trace[0].RoleName)
		}
		if exp.Trace[0].Decision != DecisionAllow {
			t.Errorf("expected first trace decision Allow, got %s", exp.Trace[0].Decision)
		}
	})

	t.Run("server-level user deny appears in trace", func(t *testing.T) {
		other, _ := core.CreateUser(ctx, SystemActorID, "explainer-user-deny", "Other", "password123")
		if err := core.DenyUserPermission(ctx, SystemActorID, other.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		exp, err := core.permissionResolver.ExplainServerPermission(ctx, other.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("ExplainServerPermission: %v", err)
		}
		if exp.State != DecisionDeny {
			t.Errorf("expected DecisionDeny, got %s", exp.State)
		}
		if len(exp.Trace) == 0 || exp.Trace[0].RoleName != other.Id || exp.Trace[0].Decision != DecisionDeny {
			t.Errorf("expected first trace entry to be user-level deny on %s, got %+v", other.Id, exp.Trace)
		}
	})

	t.Run("room-scoped user grant appears in trace at LevelRoom", func(t *testing.T) {
		roomUser, _ := core.CreateUser(ctx, SystemActorID, "explainer-room-user", "Room User", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "explainer-room", "Room")
		if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, roomUser.Id, PermMessageManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		exp, err := core.permissionResolver.ExplainRoomPermission(ctx, roomUser.Id, KindChannel, room.Id, PermMessageManage)
		if err != nil {
			t.Fatalf("ExplainRoomPermission: %v", err)
		}
		if exp.State != DecisionAllow {
			t.Errorf("expected DecisionAllow, got %s", exp.State)
		}
		if len(exp.Trace) == 0 || exp.Trace[0].Level != LevelRoom {
			t.Errorf("expected first trace entry at LevelRoom, got %+v", exp.Trace)
		}
	})
}
