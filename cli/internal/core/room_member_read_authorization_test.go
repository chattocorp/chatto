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
	members, err := core.ListRoomMemberReferences(ctx, member.Id, room.Id)
	if err != nil {
		t.Fatalf("ListRoomMemberReferences member: %v", err)
	}
	if !userRefsContain(members, member.Id) {
		t.Fatalf("room member references = %+v, want member %s", members, member.Id)
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
