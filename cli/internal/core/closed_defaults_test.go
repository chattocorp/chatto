package core

import (
	"errors"
	"testing"
)

// These tests use the real seeded defaults. New servers start closed
// (ADR-116): everyone gets no server-scope room access or room content. The
// seeded rooms and Direct messages are opened at their own scopes, and new
// rooms and room groups stay closed until an operator opens them.

// closedDefaultsFixture is a server with the real defaults, seeded rooms, and
// one user for each default role.
type closedDefaultsFixture struct {
	core                 *ChattoCore
	owner, admin, member string
	general, announce    string
}

func newClosedDefaultsFixture(t *testing.T) closedDefaultsFixture {
	t.Helper()
	c, _ := setupTestCoreWithDefaults(t)
	ctx := testContext(t)

	if err := c.SeedDefaultRooms(ctx); err != nil {
		t.Fatalf("SeedDefaultRooms: %v", err)
	}
	f := closedDefaultsFixture{core: c}
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	for _, room := range rooms {
		switch room.Name {
		case "general":
			f.general = room.Id
		case AnnouncementsRoomName:
			f.announce = room.Id
		}
	}
	if f.general == "" || f.announce == "" {
		t.Fatalf("seeded rooms = %v, want general and %s", rooms, AnnouncementsRoomName)
	}

	for _, u := range []struct {
		id       *string
		username string
		role     string
	}{
		{&f.owner, "closed-owner", RoleOwner},
		{&f.admin, "closed-admin", RoleAdmin},
		{&f.member, "closed-member", ""},
	} {
		user, err := c.CreateUser(ctx, SystemActorID, u.username, u.username, "password123")
		if err != nil {
			t.Fatalf("CreateUser %s: %v", u.username, err)
		}
		if u.role != "" {
			if err := c.AssignServerRole(ctx, SystemActorID, user.Id, u.role); err != nil {
				t.Fatalf("AssignServerRole %s: %v", u.role, err)
			}
		}
		*u.id = user.Id
	}
	return f
}

// assertCan fails the test when check does not return want.
func assertCan(t *testing.T, what string, want bool, check func() (bool, error)) {
	t.Helper()
	got, err := check()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// assertJoin joins userID to roomID and checks for success or a permission
// denial.
func assertJoin(t *testing.T, c *ChattoCore, userID, roomID string, wantAllowed bool) {
	t.Helper()
	_, err := c.RoomCommands().JoinRoom(testContext(t), RoomIDInput{ActorID: userID, RoomID: roomID})
	switch {
	case wantAllowed && err != nil:
		t.Fatalf("JoinRoom(%s, %s): %v, want success", userID, roomID, err)
	case !wantAllowed && !errors.Is(err, ErrPermissionDenied):
		t.Fatalf("JoinRoom(%s, %s) error = %v, want ErrPermissionDenied", userID, roomID, err)
	}
}

func TestClosedDefaults_GeneralRoomIsOpenToMembers(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c, ctx := f.core, testContext(t)

	assertCan(t, "member sees general", true, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, f.general) })
	assertJoin(t, c, f.member, f.general, true)
	assertCan(t, "member reads general", true, func() (bool, error) { return c.CanReadMessages(ctx, f.member, KindChannel, f.general) })
	assertCan(t, "member posts in general", true, func() (bool, error) { return c.CanPostMessage(ctx, f.member, KindChannel, f.general) })
	assertCan(t, "member reacts in general", true, func() (bool, error) { return c.CanReactToMessage(ctx, f.member, KindChannel, f.general) })
	assertCan(t, "member attaches in general", true, func() (bool, error) { return c.CanAttachFiles(ctx, f.member, KindChannel, f.general) })

	// The same member has no server-scope content permissions.
	assertCan(t, "member server message.post", false, func() (bool, error) { return c.HasServerPermission(ctx, f.member, PermMessagePost) })
	assertCan(t, "member server room.list", false, func() (bool, error) { return c.HasServerPermission(ctx, f.member, PermRoomList) })
}

func TestClosedDefaults_AnnouncementsRoomLetsOnlyAdminsPost(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c, ctx := f.core, testContext(t)

	assertCan(t, "member sees announcements", true, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, f.announce) })
	assertCan(t, "member is in universal announcements", true, func() (bool, error) {
		return c.RoomMembershipExists(ctx, KindChannel, f.member, f.announce)
	})
	assertCan(t, "member reads announcements", true, func() (bool, error) { return c.CanReadMessages(ctx, f.member, KindChannel, f.announce) })
	assertCan(t, "member reacts in announcements", true, func() (bool, error) { return c.CanReactToMessage(ctx, f.member, KindChannel, f.announce) })
	assertCan(t, "member replies in announcement threads", true, func() (bool, error) { return c.CanPostInThread(ctx, f.member, KindChannel, f.announce) })
	assertCan(t, "member posts root messages in announcements", false, func() (bool, error) { return c.CanPostMessage(ctx, f.member, KindChannel, f.announce) })

	assertCan(t, "admin posts root messages in announcements", true, func() (bool, error) { return c.CanPostMessage(ctx, f.admin, KindChannel, f.announce) })
	assertCan(t, "owner posts root messages in announcements", true, func() (bool, error) { return c.CanPostMessage(ctx, f.owner, KindChannel, f.announce) })
}

func TestClosedDefaults_NewRoomStartsClosed(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c, ctx := f.core, testContext(t)

	room, err := c.CreateRoom(ctx, f.owner, KindChannel, "", "new-closed-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	assertCan(t, "member sees new room", false, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, room.Id) })
	assertJoin(t, c, f.member, room.Id, false)
	// Admins have no server-scope room.list or room.join (ADR-116).
	assertCan(t, "admin sees new room", false, func() (bool, error) { return c.CanSeeRoom(ctx, f.admin, KindChannel, room.Id) })
	assertJoin(t, c, f.admin, room.Id, false)

	if err := c.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomList); err != nil {
		t.Fatalf("GrantRoomPermission room.list: %v", err)
	}
	assertCan(t, "member sees room after room.list allow", true, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, room.Id) })
	assertJoin(t, c, f.member, room.Id, false)

	if err := c.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("GrantRoomPermission room.join: %v", err)
	}
	assertJoin(t, c, f.member, room.Id, true)
	assertJoin(t, c, f.admin, room.Id, true)
}

func TestClosedDefaults_NewRoomGroupStartsClosed(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c, ctx := f.core, testContext(t)

	group, err := c.CreateRoomGroup(ctx, f.owner, "Closed group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := c.CreateRoom(ctx, f.owner, KindChannel, group.Id, "closed-group-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	assertCan(t, "member sees room in new group", false, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, room.Id) })
	assertJoin(t, c, f.member, room.Id, false)

	for _, perm := range []Permission{PermRoomList, PermRoomJoin} {
		if err := c.GrantGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, perm); err != nil {
			t.Fatalf("GrantGroupPermission %s: %v", perm, err)
		}
	}
	assertCan(t, "member sees room after group allow", true, func() (bool, error) { return c.CanSeeRoom(ctx, f.member, KindChannel, room.Id) })
	assertJoin(t, c, f.member, room.Id, true)
}

func TestClosedDefaults_MembersCanUseDirectMessages(t *testing.T) {
	t.Parallel()

	f := newClosedDefaultsFixture(t)
	c, ctx := f.core, testContext(t)

	assertCan(t, "member starts DM", true, func() (bool, error) { return c.CanStartDM(ctx, f.member) })
	dm, _, err := c.FindOrCreateDM(ctx, f.member, []string{f.admin})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	for _, check := range []struct {
		what string
		can  func() (bool, error)
	}{
		{"read", func() (bool, error) { return c.CanReadMessages(ctx, f.member, KindDM, dm.Id) }},
		{"post", func() (bool, error) { return c.CanPostMessage(ctx, f.member, KindDM, dm.Id) }},
		{"attach", func() (bool, error) { return c.CanAttachFiles(ctx, f.member, KindDM, dm.Id) }},
		{"react", func() (bool, error) { return c.CanReactToMessage(ctx, f.member, KindDM, dm.Id) }},
	} {
		assertCan(t, "member DM "+check.what, true, check.can)
	}
	if _, err := c.PostMessage(ctx, KindDM, dm.Id, f.member, "hello", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage in DM: %v", err)
	}

	// The permission comes from the everyone Direct-messages scope allow.
	if err := c.SetRolePermissionState(ctx, f.owner, RoleEveryone, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessagePost, PermissionStateNone); err != nil {
		t.Fatalf("clear everyone DM message.post: %v", err)
	}
	assertCan(t, "member DM post after clear", false, func() (bool, error) { return c.CanPostMessage(ctx, f.member, KindDM, dm.Id) })
}
