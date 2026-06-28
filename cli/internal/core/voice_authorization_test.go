package core

import (
	"errors"
	"testing"
)

func TestVoiceCallRoomForMemberRequiresRoomMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "voice-member", "Voice Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "voice-outsider-core", "Voice Outsider Core", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "voice-auth-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom member: %v", err)
	}

	if _, _, err := core.VoiceCallRoomForMember(ctx, outsider.Id, room.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("VoiceCallRoomForMember outsider error = %v, want ErrNotRoomMember", err)
	}
	gotRoom, kind, err := core.VoiceCallRoomForMember(ctx, member.Id, room.Id)
	if err != nil {
		t.Fatalf("VoiceCallRoomForMember member: %v", err)
	}
	if gotRoom.GetId() != room.Id || kind != KindChannel {
		t.Fatalf("VoiceCallRoomForMember = room %q kind %s, want room %q kind %s", gotRoom.GetId(), kind, room.Id, KindChannel)
	}
}
