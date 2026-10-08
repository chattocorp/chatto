package core

import (
	"errors"
	"slices"
	"testing"
)

func TestGetAccessSummary(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c := f.core
	ctx := testContext(t)

	summary := func(roomID, groupID string) *AccessSummary {
		t.Helper()
		got, err := c.GetAccessSummary(ctx, f.owner, roomID, groupID)
		if err != nil {
			t.Fatalf("GetAccessSummary(%q, %q): %v", roomID, groupID, err)
		}
		return got
	}

	if got := summary(f.general, ""); !got.EveryoneCanList || !got.EveryoneCanJoin || len(got.RolesCanJoin) != 0 {
		t.Fatalf("general = %+v, want open to everyone", got)
	}

	group, err := c.CreateRoomGroup(ctx, f.owner, "Summary group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := c.CreateRoom(ctx, f.owner, KindChannel, group.Id, "summary-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if got := summary(room.Id, ""); got.EveryoneCanList || got.EveryoneCanJoin || len(got.RolesCanList) != 0 || len(got.RolesCanJoin) != 0 {
		t.Fatalf("new room = %+v, want closed to everybody", got)
	}

	if _, err := c.CreateServerRole(ctx, SystemActorID, "engineering", "Engineering", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := c.GrantRoomPermission(ctx, SystemActorID, room.Id, "engineering", PermRoomJoin); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}
	// Joining without reading does not count as access.
	if got := summary(room.Id, ""); len(got.RolesCanJoin) != 0 {
		t.Fatalf("room where engineering can join but not read = %+v, want no roles", got)
	}
	if err := c.GrantRoomPermission(ctx, SystemActorID, room.Id, "engineering", PermMessageRead); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}
	if got := summary(room.Id, ""); got.EveryoneCanJoin || !slices.Equal(got.RolesCanJoin, []string{"engineering"}) || len(got.RolesCanList) != 0 {
		t.Fatalf("room with role allows = %+v, want only engineering can join", got)
	}

	if err := c.GrantGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("GrantGroupPermission: %v", err)
	}
	if got := summary(room.Id, ""); !got.EveryoneCanJoin || got.EveryoneCanRead {
		t.Fatalf("room that everyone can join but not read = %+v", got)
	}
	if err := c.GrantGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("GrantGroupPermission: %v", err)
	}
	if got := summary(room.Id, ""); !got.EveryoneCanJoin || !got.EveryoneCanRead || len(got.RolesCanJoin) != 0 {
		t.Fatalf("room in an open group = %+v, want everyone can join and read", got)
	}
	if got := summary("", group.Id); !got.EveryoneCanJoin || got.EveryoneCanList {
		t.Fatalf("group = %+v, want everyone can join but not list", got)
	}

	if _, err := c.GetAccessSummary(ctx, f.member, room.Id, ""); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member GetAccessSummary error = %v, want ErrPermissionDenied", err)
	}
	if _, err := c.GetAccessSummary(ctx, f.owner, room.Id, group.Id); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("room and group error = %v, want ErrInvalidArgument", err)
	}
}

// TestRoomManagersCanOpenTheirRooms checks the grant limit exception: room.manage
// at a room or room group covers the room permissions that do not need
// privileged mode there, and nothing else.
func TestRoomManagersCanOpenTheirRooms(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c := f.core
	ctx := testContext(t)

	room, err := c.CreateRoom(ctx, f.owner, KindChannel, "", "managed-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	other, err := c.CreateRoom(ctx, f.owner, KindChannel, "", "other-room", "")
	if err != nil {
		t.Fatalf("CreateRoom other: %v", err)
	}
	manager, err := c.CreateUser(ctx, SystemActorID, "room-manager", "room-manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission: %v", err)
	}
	set := func(actorID, roomID string, perm Permission) error {
		return c.SetRolePermissionState(ctx, actorID, RoleEveryone, PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID}, perm, PermissionStateAllow)
	}

	for _, perm := range []Permission{PermRoomList, PermRoomJoin, PermMessageRead, PermMessagePost} {
		if err := set(manager.Id, room.Id, perm); err != nil {
			t.Fatalf("room manager opens %s: %v", perm, err)
		}
	}
	if err := set(manager.Id, room.Id, PermMessageManage); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("room manager allows message.manage: error = %v, want ErrPermissionDenied", err)
	}
	if err := set(manager.Id, other.Id, PermRoomJoin); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("room manager opens another room: error = %v, want ErrPermissionDenied", err)
	}
	if err := set(f.member, room.Id, PermRoomJoin); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("member opens a room: error = %v, want ErrPermissionDenied", err)
	}
	// Admins manage every room, so they can open new rooms and room groups.
	if err := set(f.admin, other.Id, PermRoomJoin); err != nil {
		t.Fatalf("admin opens a room: %v", err)
	}
	group, err := c.CreateRoomGroup(ctx, f.owner, "Managed group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if err := c.SetRolePermissionState(ctx, f.admin, RoleEveryone, PermissionTargetScope{Kind: MatrixScopeGroup, ID: group.Id}, PermRoomJoin, PermissionStateAllow); err != nil {
		t.Fatalf("admin opens a room group: %v", err)
	}
	// The exception covers only rooms and room groups.
	for _, scope := range []PermissionTargetScope{{Kind: MatrixScopeServer}, {Kind: MatrixScopeDM}} {
		if err := c.SetRolePermissionState(ctx, manager.Id, RoleEveryone, scope, PermMessagePost, PermissionStateAllow); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("room manager sets message.post at %s: error = %v, want ErrPermissionDenied", scope.Kind, err)
		}
	}

	// It does not cover user settings, where an allow can lift a deny on a
	// user: an admin muted by an owner cannot unmute themselves in a room.
	if err := c.DenyUserPermission(ctx, SystemActorID, f.admin, PermMessagePost); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, f.admin, f.admin, PermissionTargetScope{Kind: MatrixScopeRoom, ID: other.Id}, PermMessagePost, PermissionStateAllow); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("muted admin allows themselves message.post: error = %v, want ErrPermissionDenied", err)
	}

	tiers, err := c.GetRolePermissionTierMatrix(ctx, manager.Id, room.Id, "")
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix: %v", err)
	}
	if !slices.Contains(tiers.ViewerChangeablePermissions, string(PermRoomJoin)) || slices.Contains(tiers.ViewerChangeablePermissions, string(PermMessageManage)) {
		t.Fatalf("changeable = %v, want room.join but not message.manage", tiers.ViewerChangeablePermissions)
	}
}
