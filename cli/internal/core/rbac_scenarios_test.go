package core

import (
	"errors"
	"testing"
)

// Roles only grant, so a private group removes room.list and room.join from
// everyone at server scope. It then allows both for everyone in the public
// group and for the chosen role in the private group, so every room in each
// group inherits the allow.
func TestRBACScenario_RoleOnlyRoomGroup(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "scenario-group-member", "Engineering Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "scenario-group-outsider", "Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	admin, err := core.CreateUser(ctx, SystemActorID, "scenario-group-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "engineering", "Engineering", "Access to engineering rooms"); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, member.Id, "engineering"); err != nil {
		t.Fatalf("AssignServerRole engineering: %v", err)
	}

	privateGroup, err := core.CreateRoomGroup(ctx, SystemActorID, "Private Engineering", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup private: %v", err)
	}
	publicGroup, err := core.CreateRoomGroup(ctx, SystemActorID, "Public Scenario", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup public: %v", err)
	}
	for _, permission := range []Permission{PermRoomList, PermRoomJoin} {
		if err := core.GrantGroupPermission(ctx, SystemActorID, publicGroup.Id, RoleEveryone, permission); err != nil {
			t.Fatalf("GrantGroupPermission everyone/%s: %v", permission, err)
		}
		if err := core.GrantGroupPermission(ctx, SystemActorID, privateGroup.Id, "engineering", permission); err != nil {
			t.Fatalf("GrantGroupPermission engineering/%s: %v", permission, err)
		}
		if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, permission); err != nil {
			t.Fatalf("ClearServerPermissionState everyone/%s: %v", permission, err)
		}
	}

	privateRoomIDs := make([]string, 0, 2)
	for _, name := range []string{"engineering-chat", "engineering-planning"} {
		room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, privateGroup.Id, name, "")
		if err != nil {
			t.Fatalf("CreateRoom %s: %v", name, err)
		}
		privateRoomIDs = append(privateRoomIDs, room.Id)
	}

	publicRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, publicGroup.Id, "public-scenario-room", "")
	if err != nil {
		t.Fatalf("CreateRoom public: %v", err)
	}

	// Roles only grant: admin's default server-scope room.list and room.join
	// allows apply in every room, so admin keeps access to the private group.
	for _, roomID := range privateRoomIDs {
		assertRoomAccessScenario(t, core, member.Id, roomID, true, true)
		assertRoomAccessScenario(t, core, outsider.Id, roomID, false, false)
		assertRoomAccessScenario(t, core, admin.Id, roomID, true, true)
	}
	assertRoomAccessScenario(t, core, outsider.Id, publicRoom.Id, true, true)

	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: member.Id, RoomID: privateRoomIDs[0]}); err != nil {
		t.Fatalf("engineering member joins private room: %v", err)
	}
	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: outsider.Id, RoomID: privateRoomIDs[0]}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("outsider join error = %v, want ErrPermissionDenied", err)
	}
	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: admin.Id, RoomID: privateRoomIDs[0]}); err != nil {
		t.Fatalf("admin joins private room through its server allow: %v", err)
	}
}

// A private room inside an otherwise public group needs room-local decisions.
// Roles only grant, so the setup removes room.list and room.join from everyone
// at server scope, allows both for everyone in each public room, and allows
// both for the chosen role in the private room. A group allow for everyone
// would also open the private room.
func TestRBACScenario_RoleOnlySpecificRoom(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "scenario-room-member", "Project Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "scenario-room-outsider", "Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "project-x", "Project X", "Access to the Project X room"); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, member.Id, "project-x"); err != nil {
		t.Fatalf("AssignServerRole project-x: %v", err)
	}

	group, err := core.CreateRoomGroup(ctx, SystemActorID, "Mixed Access", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	privateRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "project-x-private", "")
	if err != nil {
		t.Fatalf("CreateRoom private: %v", err)
	}
	publicSibling, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "mixed-access-public", "")
	if err != nil {
		t.Fatalf("CreateRoom public sibling: %v", err)
	}
	for _, permission := range []Permission{PermRoomList, PermRoomJoin} {
		if err := core.GrantRoomPermission(ctx, SystemActorID, publicSibling.Id, RoleEveryone, permission); err != nil {
			t.Fatalf("GrantRoomPermission everyone/%s: %v", permission, err)
		}
		if err := core.GrantRoomPermission(ctx, SystemActorID, privateRoom.Id, "project-x", permission); err != nil {
			t.Fatalf("GrantRoomPermission project-x/%s: %v", permission, err)
		}
		if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, permission); err != nil {
			t.Fatalf("ClearServerPermissionState everyone/%s: %v", permission, err)
		}
	}

	assertRoomAccessScenario(t, core, member.Id, privateRoom.Id, true, true)
	assertRoomAccessScenario(t, core, outsider.Id, privateRoom.Id, false, false)
	assertRoomAccessScenario(t, core, outsider.Id, publicSibling.Id, true, true)

	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: member.Id, RoomID: privateRoom.Id}); err != nil {
		t.Fatalf("project member joins private room: %v", err)
	}
	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: outsider.Id, RoomID: privateRoom.Id}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("outsider join error = %v, want ErrPermissionDenied", err)
	}
}

// A discoverable but restricted room keeps the everyone room.list baseline.
// Roles only grant, so the setup removes room.join from everyone at server
// scope and allows room.join for the chosen role in the room. Rooms that
// everyone can join then need their own allow for everyone.
func TestRBACScenario_VisibleRoomJoinableOnlyByRole(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "scenario-join-member", "Event Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "scenario-join-outsider", "Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "event-attendee", "Event Attendee", "Can join the event room"); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, member.Id, "event-attendee"); err != nil {
		t.Fatalf("AssignServerRole event-attendee: %v", err)
	}

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "event-attendees", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, "event-attendee", PermRoomJoin); err != nil {
		t.Fatalf("GrantRoomPermission event-attendee/room.join: %v", err)
	}
	if err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("ClearServerPermissionState everyone/room.join: %v", err)
	}

	assertRoomAccessScenario(t, core, member.Id, room.Id, true, true)
	assertRoomAccessScenario(t, core, outsider.Id, room.Id, true, false)

	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: member.Id, RoomID: room.Id}); err != nil {
		t.Fatalf("event attendee joins room: %v", err)
	}
	if _, err := core.RoomCommands().JoinRoom(ctx, RoomIDInput{ActorID: outsider.Id, RoomID: room.Id}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("outsider join error = %v, want ErrPermissionDenied", err)
	}
}

func assertRoomAccessScenario(t *testing.T, core *ChattoCore, userID, roomID string, wantVisible, wantJoinable bool) {
	t.Helper()
	ctx := testContext(t)

	visible, err := core.CanSeeRoom(ctx, userID, KindChannel, roomID)
	if err != nil {
		t.Fatalf("CanSeeRoom(user=%s, room=%s): %v", userID, roomID, err)
	}
	if visible != wantVisible {
		t.Errorf("CanSeeRoom(user=%s, room=%s) = %v, want %v", userID, roomID, visible, wantVisible)
	}

	joinable, err := core.CanJoinRoomAt(ctx, userID, KindChannel, roomID)
	if err != nil {
		t.Fatalf("CanJoinRoomAt(user=%s, room=%s): %v", userID, roomID, err)
	}
	if joinable != wantJoinable {
		t.Errorf("CanJoinRoomAt(user=%s, room=%s) = %v, want %v", userID, roomID, joinable, wantJoinable)
	}
}
