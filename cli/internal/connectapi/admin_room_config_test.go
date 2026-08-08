package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestAdminRoomConfigServiceAndRoomDirectoryEffectiveValues(t *testing.T) {
	env := newConnectAPITestEnv(t)
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	group, err := env.core.CreateRoomGroup(env.ctx, env.viewer.Id, "Room Config API", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, group.Id, "room-config-api-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	serverScope := &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_Server{Server: true}}
	serverValue := 2 * time.Hour
	server, err := env.roomConfig.UpdateRoomConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomConfigRequest{
		Scope: serverScope, Layer: &adminv1.RoomConfigLayer{AuthorEditWindow: durationpb.New(serverValue)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window"}},
	}))
	if err != nil {
		t.Fatalf("Update server room configuration: %v", err)
	}
	if got := server.Msg.GetState().GetEffective().GetAuthorEditWindow().AsDuration(); got != serverValue {
		t.Fatalf("server effective = %s, want %s", got, serverValue)
	}

	groupValue := time.Hour
	groupScope := &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_RoomGroupId{RoomGroupId: group.Id}}
	if _, err := env.roomConfig.UpdateRoomConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomConfigRequest{
		Scope: groupScope, Layer: &adminv1.RoomConfigLayer{AuthorEditWindow: durationpb.New(groupValue)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window"}},
	})); err != nil {
		t.Fatalf("Update group room configuration: %v", err)
	}

	roomValue := 30 * time.Minute
	roomScope := &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_RoomId{RoomId: room.Id}}
	_, err = env.roomConfig.UpdateRoomConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomConfigRequest{
		Scope: roomScope, Layer: &adminv1.RoomConfigLayer{AuthorEditWindow: durationpb.New(roomValue)},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	roomConfiguration, err := env.roomConfig.UpdateRoomConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomConfigRequest{
		Scope: roomScope, Layer: &adminv1.RoomConfigLayer{AuthorEditWindow: durationpb.New(roomValue)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window"}},
	}))
	if err != nil {
		t.Fatalf("Update room configuration: %v", err)
	}
	configuration := roomConfiguration.Msg.GetState()
	if configuration.GetLayer().GetAuthorEditWindow().AsDuration() != roomValue ||
		configuration.GetEffective().GetAuthorEditWindow().AsDuration() != roomValue ||
		configuration.GetSources().GetAuthorEditWindow().GetRoomId() != room.Id {
		t.Fatalf("room configuration state = %+v", configuration)
	}

	directory, err := env.directory.GetRoom(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got := directory.Msg.GetRoom().GetViewerState().GetRoomConfig().GetAuthorEditWindow().AsDuration(); got != roomValue {
		t.Fatalf("directory effective window = %s, want %s", got, roomValue)
	}

	cleared, err := env.roomConfig.UpdateRoomConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomConfigRequest{
		Scope: roomScope, Layer: &adminv1.RoomConfigLayer{},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window"}},
	}))
	if err != nil {
		t.Fatalf("Clear room configuration layer: %v", err)
	}
	if cleared.Msg.GetState().GetLayer().AuthorEditWindow != nil ||
		cleared.Msg.GetState().GetEffective().GetAuthorEditWindow().AsDuration() != groupValue {
		t.Fatalf("cleared room configuration = %+v", cleared.Msg.GetState())
	}

	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-config-api-regular", "Regular", "password123")
	if err != nil {
		t.Fatalf("Create regular: %v", err)
	}
	_, err = env.roomConfig.GetRoomConfig(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.GetRoomConfigRequest{Scope: serverScope}))
	requireConnectCode(t, err, connect.CodePermissionDenied)
}
