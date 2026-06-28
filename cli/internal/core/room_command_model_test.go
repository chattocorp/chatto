package core

import (
	"errors"
	"testing"
)

func TestRoomCommandModelAuthorization(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	commands := core.RoomCommands()

	actor, err := core.CreateUser(ctx, SystemActorID, "room-command-actor", "Room Command Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected seeded room group")
	}
	groupID := groups[0].Id

	if _, err := commands.CreateRoom(ctx, RoomCreateInput{
		ActorID: actor.Id,
		GroupID: groupID,
		Name:    "room-command-created",
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CreateRoom without room.create error = %v, want ErrPermissionDenied", err)
	}

	if err := core.GrantGroupPermission(ctx, SystemActorID, groupID, RoleEveryone, PermRoomCreate); err != nil {
		t.Fatalf("GrantGroupPermission room.create: %v", err)
	}
	room, err := commands.CreateRoom(ctx, RoomCreateInput{
		ActorID: actor.Id,
		GroupID: groupID,
		Name:    "room-command-created",
	})
	if err != nil {
		t.Fatalf("CreateRoom with group-scoped room.create: %v", err)
	}

	if _, err := commands.UpdateRoom(ctx, RoomUpdateInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		Name:    "room-command-renamed",
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("UpdateRoom without room.manage error = %v, want ErrPermissionDenied", err)
	}

	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("GrantRoomPermission room.manage: %v", err)
	}
	if _, err := commands.UpdateRoom(ctx, RoomUpdateInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		Name:    "room-command-renamed",
	}); err != nil {
		t.Fatalf("UpdateRoom with room-scoped room.manage: %v", err)
	}

	dmParticipant, err := core.CreateUser(ctx, SystemActorID, "room-command-dm-participant", "Room Command DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser dm participant: %v", err)
	}
	dm, created, err := commands.StartDM(ctx, RoomStartDMInput{
		ActorID:        actor.Id,
		ParticipantIDs: []string{dmParticipant.Id},
	})
	if err != nil {
		t.Fatalf("StartDM with default DM permission: %v", err)
	}
	if !created || KindOfRoom(dm) != KindDM {
		t.Fatalf("StartDM result created=%v kind=%v, want created DM", created, KindOfRoom(dm))
	}

	blocked, err := core.CreateUser(ctx, SystemActorID, "room-command-dm-blocked", "Room Command DM Blocked", "password")
	if err != nil {
		t.Fatalf("CreateUser blocked: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "room-command-dm-blocked-role", "Room Command DM Blocked", ""); err != nil {
		t.Fatalf("CreateServerRole blocked: %v", err)
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, "room-command-dm-blocked-role", PermMessagePost); err != nil {
		t.Fatalf("DenyServerPermission message.post: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, blocked.Id, "room-command-dm-blocked-role"); err != nil {
		t.Fatalf("AssignServerRole blocked: %v", err)
	}
	if _, _, err := commands.StartDM(ctx, RoomStartDMInput{
		ActorID:        blocked.Id,
		ParticipantIDs: []string{dmParticipant.Id},
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("StartDM denied user error = %v, want ErrPermissionDenied", err)
	}

	target, err := core.CreateUser(ctx, SystemActorID, "room-command-target", "Room Command Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := core.JoinRoom(ctx, target.Id, KindChannel, target.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom target: %v", err)
	}
	if _, err := commands.BanRoomMember(ctx, RoomBanInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
		Reason:  "test",
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("BanRoomMember without room.ban-member error = %v, want ErrPermissionDenied", err)
	}
	if _, err := commands.ListActiveRoomBans(ctx, RoomBanListInput{
		ActorID: actor.Id,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListActiveRoomBans without room.ban-member error = %v, want ErrPermissionDenied", err)
	}

	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomMemberBan); err != nil {
		t.Fatalf("GrantRoomPermission room.ban-member: %v", err)
	}
	if _, err := commands.ListActiveRoomBans(ctx, RoomBanListInput{
		ActorID: actor.Id,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListActiveRoomBans with only room-scoped room.ban-member error = %v, want ErrPermissionDenied", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomMemberBan); err != nil {
		t.Fatalf("GrantServerPermission room.ban-member: %v", err)
	}
	if _, err := commands.BanRoomMember(ctx, RoomBanInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
		Reason:  "test",
	}); err != nil {
		t.Fatalf("BanRoomMember with room-scoped room.ban-member: %v", err)
	}
	roomID := room.Id
	bans, err := commands.ListActiveRoomBans(ctx, RoomBanListInput{
		ActorID: actor.Id,
		RoomID:  &roomID,
	})
	if err != nil {
		t.Fatalf("ListActiveRoomBans with server-scoped room.ban-member: %v", err)
	}
	if got := len(bans); got != 1 {
		t.Fatalf("ListActiveRoomBans count = %d, want 1", got)
	}
}
