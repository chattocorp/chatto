package core

import (
	"errors"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomMemberReadOperationsRequireMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "room-read-member", "Room Read Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "room-read-outsider", "Room Read Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	actor, err := core.CreateUser(ctx, SystemActorID, "room-read-actor", "Room Read Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	manager, err := core.CreateUser(ctx, SystemActorID, "room-read-manager", "Room Read Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "room-read-auth", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom member: %v", err)
	}

	if _, err := core.ListRoomMemberReferences(ctx, outsider.Id, room.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("ListRoomMemberReferences outsider error = %v, want ErrNotRoomMember", err)
	}
	listableMembers, err := core.ListRoomMemberReferencesForList(ctx, outsider.Id, room.Id)
	if err != nil {
		t.Fatalf("ListRoomMemberReferencesForList joinable outsider: %v", err)
	}
	if !userRefsContain(listableMembers, member.Id) {
		t.Fatalf("listable room member references = %+v, want member %s", listableMembers, member.Id)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission room.join: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, outsider.Id, room.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("join-denied ListRoomMemberReferencesForList error = %v, want ErrPermissionDenied", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomList); err != nil {
		t.Fatalf("DenyUserRoomPermission room.list: %v", err)
	}
	if err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomJoin); err != nil {
		t.Fatalf("DenyUserRoomPermission room.join: %v", err)
	}
	managerMembers, err := core.ListRoomMemberReferencesForList(ctx, manager.Id, room.Id)
	if err != nil {
		t.Fatalf("manager ListRoomMemberReferencesForList: %v", err)
	}
	if !userRefsContain(managerMembers, member.Id) {
		t.Fatalf("manager list member references = %+v, want member %s", managerMembers, member.Id)
	}
	managerLookups, err := core.ListRoomMemberReferencesForLookup(ctx, manager.Id, room.Id)
	if err != nil {
		t.Fatalf("manager ListRoomMemberReferencesForLookup: %v", err)
	}
	if !userRefsContain(managerLookups, member.Id) {
		t.Fatalf("manager lookup member references = %+v, want member %s", managerLookups, member.Id)
	}
	members, err := core.ListRoomMemberReferences(ctx, member.Id, room.Id)
	if err != nil {
		t.Fatalf("ListRoomMemberReferences member: %v", err)
	}
	if !userRefsContain(members, member.Id) {
		t.Fatalf("room member references = %+v, want member %s", members, member.Id)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, member.Id, room.Id); err != nil {
		t.Fatalf("join-denied member ListRoomMemberReferencesForList: %v", err)
	}
	dm, _, err := core.FindOrCreateDM(ctx, member.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, outsider.Id, dm.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("DM outsider ListRoomMemberReferencesForList error = %v, want ErrNotRoomMember", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, dm.Id, outsider.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission DM room.manage: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, outsider.Id, dm.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("DM manager ListRoomMemberReferencesForList error = %v, want ErrNotRoomMember", err)
	}
	if _, err := core.ListRoomMemberReferencesForLookup(ctx, outsider.Id, dm.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("DM manager ListRoomMemberReferencesForLookup error = %v, want ErrNotRoomMember", err)
	}
	archivedRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "room-read-archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if _, err := core.ArchiveRoom(ctx, SystemActorID, KindChannel, archivedRoom.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, outsider.Id, archivedRoom.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("archived outsider ListRoomMemberReferencesForList error = %v, want ErrNotRoomMember", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, archivedRoom.Id, manager.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission archived room.manage: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForList(ctx, manager.Id, archivedRoom.Id); err != nil {
		t.Fatalf("archived manager ListRoomMemberReferencesForList: %v", err)
	}
	if _, err := core.ListRoomMemberReferencesForLookup(ctx, manager.Id, archivedRoom.Id); err != nil {
		t.Fatalf("archived manager ListRoomMemberReferencesForLookup: %v", err)
	}

	if _, err := core.CreateNotification(ctx, member.Id, actor.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{RoomId: room.Id, EventId: "event-id"},
		},
	}); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	outsiderNotifications, err := core.GetRoomNotificationsForMember(ctx, outsider.Id, room.Id)
	if err != nil {
		t.Fatalf("GetRoomNotificationsForMember outsider: %v", err)
	}
	if len(outsiderNotifications) != 0 {
		t.Fatalf("outsider room notifications = %+v, want empty", outsiderNotifications)
	}
	memberNotifications, err := core.GetRoomNotificationsForMember(ctx, member.Id, room.Id)
	if err != nil {
		t.Fatalf("GetRoomNotificationsForMember member: %v", err)
	}
	if len(memberNotifications) != 1 || memberNotifications[0].GetMention().GetRoomId() != room.Id {
		t.Fatalf("member room notifications = %+v, want one room mention", memberNotifications)
	}
}

func userRefsContain(users []*corev1.User, userID string) bool {
	for _, user := range users {
		if user.GetId() == userID {
			return true
		}
	}
	return false
}
