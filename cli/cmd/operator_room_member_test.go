package cmd

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestOperatorRoomMemberAdd(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-member", "Operator Member", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-member-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	args := []string{"operator", "room", "member", "add", "--room-id", room.GetId(), "--user-id", user.GetId(), "--json"}
	output := env.run(t, args...)
	var added operatorv1.AddMemberResponse
	if err := protojson.Unmarshal([]byte(output), &added); err != nil {
		t.Fatalf("decode add result: %v\n%s", err, output)
	}
	if added.GetRoomId() != room.GetId() || added.GetMember().GetUser().GetId() != user.GetId() {
		t.Fatalf("add result = %+v", &added)
	}
	if _, err := env.core.GetRoomMembership(env.ctx, core.KindChannel, user.GetId(), room.GetId()); err != nil {
		t.Fatalf("GetRoomMembership: %v", err)
	}

	repeated := env.run(t, args...)
	var repeatResult operatorv1.AddMemberResponse
	if err := protojson.Unmarshal([]byte(repeated), &repeatResult); err != nil || !proto.Equal(&added, &repeatResult) {
		t.Fatalf("repeat result = %+v, error = %v", &repeatResult, err)
	}
	for _, eventType := range []string{evtstream.EventRoomMemberAdded, evtstream.EventUserJoinedRoom} {
		events, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.RoomAggregate(room.GetId()).Subject(eventType))
		if err != nil {
			t.Fatalf("SubjectEvents %s: %v", eventType, err)
		}
		if len(events) != 1 {
			t.Fatalf("%s fact count = %d, want 1", eventType, len(events))
		}
		if eventType == evtstream.EventRoomMemberAdded && events[0].GetActorId() != core.SystemActorID {
			t.Fatalf("operator add actor = %q, want system", events[0].GetActorId())
		}
	}

	human := env.run(t, "operator", "room", "member", "add", "--room-id", room.GetId(), "--user-id", user.GetId())
	if !strings.Contains(human, "room="+room.GetId()) || !strings.Contains(human, "user="+user.GetId()) {
		t.Fatalf("human result = %q", human)
	}
}

func TestOperatorRoomMemberAddRejectsInvalidTargets(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-target", "Operator Target", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-other", "Operator Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-target-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	universal, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-universal-room", "")
	if err != nil {
		t.Fatalf("CreateRoom universal: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, core.SystemActorID, core.KindChannel, universal.GetId(), true); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	archived, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-archived-room", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if _, err := env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, archived.GetId()); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, user.GetId(), []string{other.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	banned, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-banned-room", "")
	if err != nil {
		t.Fatalf("CreateRoom banned: %v", err)
	}
	if _, err := env.core.AddMember(env.ctx, core.SystemActorID, core.KindChannel, banned.GetId(), user.GetId()); err != nil {
		t.Fatalf("AddMember before ban: %v", err)
	}
	if _, err := env.core.BanMember(env.ctx, core.SystemActorID, core.KindChannel, banned.GetId(), user.GetId(), "test ban", nil); err != nil {
		t.Fatalf("BanMember: %v", err)
	}

	for _, tc := range []struct {
		name, roomID, userID string
		want                 connect.Code
	}{
		{name: "missing room", roomID: "missing-room", userID: user.GetId(), want: connect.CodeNotFound},
		{name: "missing user", roomID: room.GetId(), userID: "missing-user", want: connect.CodeNotFound},
		{name: "DM room", roomID: dm.GetId(), userID: user.GetId(), want: connect.CodeNotFound},
		{name: "universal room", roomID: universal.GetId(), userID: user.GetId(), want: connect.CodeInvalidArgument},
		{name: "archived room", roomID: archived.GetId(), userID: user.GetId(), want: connect.CodeFailedPrecondition},
		{name: "banned user", roomID: banned.GetId(), userID: user.GetId(), want: connect.CodePermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.execute(t, "operator", "room", "member", "add", "--room-id", tc.roomID, "--user-id", tc.userID)
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("AddMember code = %v, want %v; error = %v", got, tc.want, err)
			}
		})
	}
	for _, args := range [][]string{
		{"operator", "room", "member", "add"},
		{"operator", "room", "member", "add", "--room-id", room.GetId()},
		{"operator", "room", "member", "add", "--user-id", user.GetId()},
	} {
		if _, err := env.execute(t, args...); err == nil {
			t.Fatalf("missing flag accepted: %v", args)
		}
	}
}
