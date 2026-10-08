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
	if got := summary(room.Id, ""); got.EveryoneCanJoin || !slices.Equal(got.RolesCanJoin, []string{"engineering"}) || len(got.RolesCanList) != 0 {
		t.Fatalf("room with a role allow = %+v, want only engineering can join", got)
	}

	if err := c.GrantGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("GrantGroupPermission: %v", err)
	}
	if got := summary(room.Id, ""); !got.EveryoneCanJoin || len(got.RolesCanJoin) != 0 {
		t.Fatalf("room in an open group = %+v, want everyone can join", got)
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
