package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/evtstream"
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
	existingDM, created, err := core.FindOrCreateDM(ctx, dmParticipant.Id, []string{blocked.Id})
	if err != nil || !created {
		t.Fatalf("FindOrCreateDM for blocked user = %v, created=%v, want existing DM setup", err, created)
	}
	foundDM, created, err := commands.StartDM(ctx, RoomStartDMInput{
		ActorID:        blocked.Id,
		ParticipantIDs: []string{dmParticipant.Id},
	})
	if err != nil {
		t.Fatalf("StartDM existing DM for denied user: %v", err)
	}
	if created || foundDM.GetId() != existingDM.GetId() {
		t.Fatalf("StartDM existing DM = %q, created=%v, want %q without creation", foundDM.GetId(), created, existingDM.GetId())
	}
	if _, _, err := commands.StartDM(ctx, RoomStartDMInput{
		ActorID:        blocked.Id,
		ParticipantIDs: []string{actor.Id},
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("StartDM new DM for denied user error = %v, want ErrPermissionDenied", err)
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

	addEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMemberAdded))
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
	addEvents, _, err = core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMemberAdded))
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
	removeEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMemberRemoved))
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

func TestBotOwnerRoomMembership(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "membership-owner", "Membership Owner", "password")
	require.NoError(t, err)
	other, err := c.CreateUser(ctx, SystemActorID, "membership-other", "Other", "password")
	require.NoError(t, err)
	bot, err := c.CreateBot(ctx, owner.Id, "membership_bot", "Membership Bot")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-membership", "")
	require.NoError(t, err)
	input := RoomUserInput{ActorID: owner.Id, RoomID: room.Id, UserID: bot.User.Id}
	commands := c.RoomCommands()
	canManage, err := c.PermResolver().HasRoomPermission(ctx, owner.Id, KindChannel, room.Id, PermRoomManage)
	require.NoError(t, err)
	require.False(t, canManage, "fixture owner must not have room.manage")

	joined := func() bool {
		t.Helper()
		joined, err := c.RoomMembershipExists(ctx, KindChannel, bot.User.Id, room.Id)
		require.NoError(t, err)
		return joined
	}
	canJoin := func() bool {
		t.Helper()
		allowed, err := c.CanJoinRoomAt(ctx, bot.User.Id, KindChannel, room.Id)
		require.NoError(t, err)
		return allowed
	}
	grant := func(state PermissionState) {
		t.Helper()
		require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, bot.User.Id,
			PermissionTargetScope{Kind: MatrixScopeRoom, ID: room.Id}, PermRoomJoin, state))
	}

	require.False(t, canJoin())
	_, err = commands.AddMember(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied, "bot needs an explicit grant")
	grant(PermissionStateAllow)
	require.True(t, canJoin())
	otherInput := input
	otherInput.ActorID = other.Id
	_, err = commands.AddMember(ctx, otherInput)
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = commands.AddMember(ctx, input)
	require.NoError(t, err)
	_, err = commands.AddMember(ctx, input)
	require.NoError(t, err, "join is idempotent")
	require.True(t, joined())
	require.True(t, joined())
	canRead, err := c.CanReadMessages(ctx, bot.User.Id, KindChannel, room.Id)
	require.NoError(t, err)
	require.False(t, canRead, "joining must not grant message permissions")
	_, err = commands.RemoveMember(ctx, otherInput)
	require.ErrorIs(t, err, ErrPermissionDenied)

	grant(PermissionStateNone)
	require.True(t, joined(), "join grant loss must not prevent removal")
	removed, err := commands.RemoveMember(ctx, input)
	require.NoError(t, err)
	require.True(t, removed)
	removed, err = commands.RemoveMember(ctx, input)
	require.NoError(t, err)
	require.False(t, removed, "leave is idempotent")

	grant(PermissionStateAllow)
	_, err = commands.AddMember(ctx, input)
	require.NoError(t, err)
	// Keep the room visible through the owner's own membership after its join
	// permission is revoked; matrix directory visibility remains unchanged.
	_, err = c.JoinRoom(ctx, owner.Id, KindChannel, owner.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, c.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin))
	_, err = commands.AddMember(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied, "owner ceiling applies even to an existing member")
	require.True(t, joined())
	_, err = commands.RemoveMember(ctx, input)
	require.NoError(t, err)
	matrix, err := c.GetUserPermissionMatrix(ctx, owner.Id, bot.User.Id)
	require.NoError(t, err)
	for _, cell := range matrix.Cells {
		if cell.ScopeID == "room:"+room.Id && cell.Permission == string(PermRoomJoin) {
			require.Equal(t, MatrixDecisionAllow, cell.Override, "leave preserves the stored grant")
		}
	}
	_, err = commands.AddMember(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin))
	_, err = commands.AddMember(ctx, input)
	require.NoError(t, err)
	_, err = c.ArchiveRoom(ctx, SystemActorID, KindChannel, room.Id)
	require.NoError(t, err)
	_, err = commands.RemoveMember(ctx, input)
	require.NoError(t, err, "bot owner can clean up archived rooms")
	_, err = commands.AddMember(ctx, input)
	require.ErrorIs(t, err, ErrRoomArchived)
	_, err = c.UnarchiveRoom(ctx, SystemActorID, KindChannel, room.Id)
	require.NoError(t, err)
	_, err = commands.AddMember(ctx, input)
	require.NoError(t, err)
	_, err = c.BanMember(ctx, SystemActorID, KindChannel, room.Id, bot.User.Id, "membership test", nil)
	require.NoError(t, err)
	_, err = commands.AddMember(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.False(t, canJoin())

	// Universal membership follows effective room.join, not explicit writes.
	universal, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-universal", "")
	require.NoError(t, err)
	_, err = c.SetRoomUniversal(ctx, SystemActorID, KindChannel, universal.Id, true)
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, bot.User.Id,
		PermissionTargetScope{Kind: MatrixScopeRoom, ID: universal.Id}, PermRoomJoin, PermissionStateAllow))
	universalInput := input
	universalInput.RoomID = universal.Id
	_, err = commands.AddMember(ctx, universalInput)
	require.ErrorIs(t, err, ErrInvalidArgument)
	_, err = commands.RemoveMember(ctx, universalInput)
	require.ErrorIs(t, err, ErrInvalidArgument)
	universalJoined, err := c.RoomMembershipExists(ctx, KindChannel, bot.User.Id, universal.Id)
	require.NoError(t, err)
	require.True(t, universalJoined)
	dm, _, err := commands.StartDM(ctx, RoomStartDMInput{ActorID: owner.Id, ParticipantIDs: []string{bot.User.Id}})
	require.NoError(t, err)
	dmInput := input
	dmInput.RoomID = dm.Id
	_, err = commands.AddMember(ctx, dmInput)
	require.ErrorIs(t, err, ErrInvalidArgument)
	_, err = commands.RemoveMember(ctx, dmInput)
	require.ErrorIs(t, err, ErrInvalidArgument)

	// Bot management does not authorize adding human accounts.
	humanInput := input
	humanInput.UserID = other.Id
	_, err = commands.AddMember(ctx, humanInput)
	require.ErrorIs(t, err, ErrPermissionDenied)
}

func TestBotManagerMembershipAndAuthorizationRetry(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "retry-owner", "Owner", "password")
	require.NoError(t, err)
	manager, err := c.CreateUser(ctx, SystemActorID, "retry-manager", "Manager", "password")
	require.NoError(t, err)
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, manager.Id, PermBotManage))
	bot, err := c.CreateBot(ctx, owner.Id, "retry_bot", "Retry Bot")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-retry", "")
	require.NoError(t, err)
	input := RoomUserInput{ActorID: manager.Id, UserID: bot.User.Id, RoomID: room.Id}
	_, err = c.RoomCommands().AddMember(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied, "global manager cannot bypass the bot allowlist")
	require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, bot.User.Id,
		PermissionTargetScope{Kind: MatrixScopeRoom, ID: room.Id}, PermRoomJoin, PermissionStateAllow))
	_, err = c.RoomCommands().AddMember(ctx, input)
	require.NoError(t, err)
	_, err = c.RoomCommands().RemoveMember(ctx, input)
	require.NoError(t, err)

	// The second authorization call follows room catch-up. Change that room's
	// tail to force OCC failure, then reject the next authorization decision.
	calls := 0
	_, err = c.addMember(ctx, manager.Id, KindChannel, room.Id, bot.User.Id, func() error {
		calls++
		if calls > 2 {
			return ErrPermissionDenied
		}
		if calls == 2 {
			_, err := c.UpdateRoom(ctx, SystemActorID, KindChannel, room.Id, room.Name, "concurrent change")
			return err
		}
		return c.RoomCommands().authorizeMembershipChange(ctx, input, true)
	})
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.Equal(t, 3, calls, "authorization must be repeated after OCC conflict")
	joined, err := c.RoomMembershipExists(ctx, KindChannel, bot.User.Id, room.Id)
	require.NoError(t, err)
	require.False(t, joined)
}

func TestAccountMembershipManagerOverridesJoinPermission(t *testing.T) {
	for _, authority := range []Permission{PermUserManageAccounts, PermRoomManage} {
		for _, botTarget := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/bot=%t", authority, botTarget), func(t *testing.T) {
				c, _ := setupTestCore(t)
				ctx := testContext(t)
				manager, err := c.CreateUser(ctx, SystemActorID, "account-manager", "Manager", "password")
				require.NoError(t, err)
				owner, err := c.CreateUser(ctx, SystemActorID, "account-owner", "Owner", "password")
				require.NoError(t, err)
				target := owner
				if botTarget {
					bot, err := c.CreateBot(ctx, owner.Id, "managed_account_bot", "Bot")
					require.NoError(t, err)
					target = bot.User
				}
				room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "managed-membership", "")
				require.NoError(t, err)
				require.NoError(t, c.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin))
				input := RoomUserInput{ActorID: manager.Id, RoomID: room.Id, UserID: target.Id}
				_, err = c.RoomCommands().AddMember(ctx, input)
				require.ErrorIs(t, err, ErrPermissionDenied)
				if authority == PermRoomManage {
					require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, authority))
				} else {
					require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, manager.Id, authority))
				}
				_, err = c.RoomCommands().AddMember(ctx, input)
				require.NoError(t, err, "management authority overrides the target's missing room.join")
				addEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMemberAdded))
				require.NoError(t, err)
				require.Len(t, addEvents, 1)
				require.Equal(t, manager.Id, addEvents[0].GetActorId(), "EVT must identify the manager who overrides room.join")
				require.Equal(t, target.Id, addEvents[0].GetRoomMemberAdded().GetUserId())

				allowed, err := c.CanJoinRoomAt(ctx, target.Id, KindChannel, room.Id)
				require.NoError(t, err)
				require.False(t, allowed, "adding membership must not change permission grants")
				members, err := c.GetRoomMemberReferencesForLookup(ctx, manager.Id, room.Id, []string{target.Id})
				require.NoError(t, err)
				require.Len(t, members, 1, "account managers need not join to inspect membership")
				if authority == PermRoomManage {
					otherRoom, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "other-membership", "")
					require.NoError(t, err)
					_, err = c.RoomCommands().AddMember(ctx, RoomUserInput{ActorID: manager.Id, RoomID: otherRoom.Id, UserID: target.Id})
					require.ErrorIs(t, err, ErrPermissionDenied, "room management is scoped to its room")
				}
				_, err = c.ArchiveRoom(ctx, SystemActorID, KindChannel, room.Id)
				require.NoError(t, err)
				_, err = c.RoomCommands().RemoveMember(ctx, input)
				require.NoError(t, err)
				removeEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMemberRemoved))
				require.NoError(t, err)
				require.Len(t, removeEvents, 1)
				require.Equal(t, manager.Id, removeEvents[0].GetActorId(), "EVT must identify the manager who removes the account")
				require.Equal(t, target.Id, removeEvents[0].GetRoomMemberRemoved().GetUserId())

				_, err = c.RoomCommands().AddMember(ctx, input)
				require.ErrorIs(t, err, ErrRoomArchived)
				_, err = c.UnarchiveRoom(ctx, SystemActorID, KindChannel, room.Id)
				require.NoError(t, err)
				_, err = c.RoomCommands().AddMember(ctx, input)
				require.NoError(t, err)
				_, err = c.BanMember(ctx, SystemActorID, KindChannel, room.Id, target.Id, "test", nil)
				require.NoError(t, err)
				_, err = c.RoomCommands().AddMember(ctx, input)
				require.ErrorIs(t, err, ErrPermissionDenied, "a join override must not override a ban")
			})
		}
	}
}
