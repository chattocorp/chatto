package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
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
		Name:    stringPtrForCoreTest("room-command-renamed"),
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("UpdateRoom without room.manage error = %v, want ErrPermissionDenied", err)
	}

	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("GrantRoomPermission room.manage: %v", err)
	}
	if _, err := commands.UpdateRoom(ctx, RoomUpdateInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		Name:    stringPtrForCoreTest("room-command-renamed"),
	}); err != nil {
		t.Fatalf("UpdateRoom with room-scoped room.manage: %v", err)
	}
	universal := true
	updatedRoom, err := commands.UpdateRoom(ctx, RoomUpdateInput{
		ActorID:   actor.Id,
		RoomID:    room.Id,
		Universal: &universal,
	})
	if err != nil {
		t.Fatalf("UpdateRoom universal with room-scoped room.manage: %v", err)
	}
	if !updatedRoom.GetUniversal() {
		t.Fatal("UpdateRoom universal = false, want true")
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
	if _, err := commands.BanMember(ctx, RoomBanInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
		Reason:  "test",
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("BanMember without room.ban-member error = %v, want ErrPermissionDenied", err)
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
	if _, err := commands.BanMember(ctx, RoomBanInput{
		ActorID: actor.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
		Reason:  "test",
	}); err != nil {
		t.Fatalf("BanMember with room-scoped room.ban-member: %v", err)
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

func TestRoomCommandModelManageRoomMembers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	commands := core.RoomCommands()

	manager, err := core.CreateUser(ctx, SystemActorID, "room-member-manager", "Room Member Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "room-member-target", "Room Member Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "room-member-outsider", "Room Member Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	room, err := core.CreateRoom(ctx, manager.Id, KindChannel, "", "managed-members", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission room.join: %v", err)
	}

	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: outsider.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AddMember without room.manage error = %v, want ErrPermissionDenied", err)
	}

	membership, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
	})
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if membership.GetUserId() != target.Id || membership.GetRoomId() != room.Id {
		t.Fatalf("AddMember membership = %+v, want target room membership", membership)
	}
	isMember, err := core.RoomMembershipExists(ctx, KindChannel, target.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after add: %v", err)
	}
	if !isMember {
		t.Fatalf("target is not a room member after AddMember")
	}

	addEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(room.Id).Subject(events.EventRoomMemberAdded))
	if err != nil {
		t.Fatalf("SubjectEvents room_member_added: %v", err)
	}
	if len(addEvents) != 1 || addEvents[0].GetActorId() != manager.Id || addEvents[0].GetRoomMemberAdded().GetUserId() != target.Id {
		t.Fatalf("room_member_added events = %+v, want one manager audit event for target", addEvents)
	}

	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
	}); err != nil {
		t.Fatalf("idempotent AddMember: %v", err)
	}
	addEvents, _, err = core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(room.Id).Subject(events.EventRoomMemberAdded))
	if err != nil {
		t.Fatalf("SubjectEvents room_member_added after idempotent add: %v", err)
	}
	if len(addEvents) != 1 {
		t.Fatalf("idempotent AddMember wrote %d audit events, want 1", len(addEvents))
	}

	removed, err := commands.RemoveMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
	})
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if !removed {
		t.Fatalf("RemoveMember removed = false, want true")
	}
	isMember, err = core.RoomMembershipExists(ctx, KindChannel, target.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after remove: %v", err)
	}
	if isMember {
		t.Fatalf("target is still a room member after RemoveMember")
	}
	removeEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(room.Id).Subject(events.EventRoomMemberRemoved))
	if err != nil {
		t.Fatalf("SubjectEvents room_member_removed: %v", err)
	}
	if len(removeEvents) != 1 || removeEvents[0].GetActorId() != manager.Id || removeEvents[0].GetRoomMemberRemoved().GetUserId() != target.Id {
		t.Fatalf("room_member_removed events = %+v, want one manager audit event for target", removeEvents)
	}

	removed, err = commands.RemoveMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  room.Id,
		UserID:  target.Id,
	})
	if err != nil {
		t.Fatalf("idempotent RemoveMember: %v", err)
	}
	if removed {
		t.Fatalf("idempotent RemoveMember removed = true, want false")
	}
}

func TestRoomCommandModelManageRoomMembersRejectsInvalidTargets(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	commands := core.RoomCommands()

	manager, err := core.CreateUser(ctx, SystemActorID, "room-member-invalid-manager", "Room Member Invalid Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "room-member-invalid-target", "Room Member Invalid Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	room, err := core.CreateRoom(ctx, manager.Id, KindChannel, "", "invalid-member-targets", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}

	universal, err := core.CreateRoom(ctx, manager.Id, KindChannel, "", "invalid-universal", "", WithUniversalRoom(true))
	if err != nil {
		t.Fatalf("CreateRoom universal: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, universal.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission universal room.manage: %v", err)
	}
	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  universal.Id,
		UserID:  target.Id,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("AddMember universal error = %v, want ErrInvalidArgument", err)
	}
	if _, err := commands.RemoveMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  universal.Id,
		UserID:  target.Id,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("RemoveMember universal error = %v, want ErrInvalidArgument", err)
	}

	dm, _, err := core.FindOrCreateDM(ctx, manager.Id, []string{target.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  dm.Id,
		UserID:  target.Id,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("AddMember DM error = %v, want ErrInvalidArgument", err)
	}

	archived, err := core.CreateRoom(ctx, manager.Id, KindChannel, "", "invalid-archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, archived.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission archived room.manage: %v", err)
	}
	if _, err := core.ArchiveRoom(ctx, manager.Id, KindChannel, archived.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  archived.Id,
		UserID:  target.Id,
	}); !errors.Is(err, ErrRoomArchived) {
		t.Fatalf("AddMember archived error = %v, want ErrRoomArchived", err)
	}

	banned, err := core.CreateUser(ctx, SystemActorID, "room-member-invalid-banned", "Room Member Invalid Banned", "password")
	if err != nil {
		t.Fatalf("CreateUser banned: %v", err)
	}
	if _, err := core.JoinRoom(ctx, banned.Id, KindChannel, banned.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom banned target: %v", err)
	}
	if _, err := core.BanMember(ctx, manager.Id, KindChannel, room.Id, banned.Id, "test ban", nil); err != nil {
		t.Fatalf("BanMember: %v", err)
	}
	if _, err := commands.AddMember(ctx, RoomUserInput{
		ActorID: manager.Id,
		RoomID:  room.Id,
		UserID:  banned.Id,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AddMember banned error = %v, want ErrPermissionDenied", err)
	}
}
