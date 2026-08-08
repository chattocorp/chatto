package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestAdminPolicyServiceAndRoomDirectoryEffectiveValues(t *testing.T) {
	env := newConnectAPITestEnv(t)
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	group, err := env.core.CreateRoomGroup(env.ctx, env.viewer.Id, "Policy API", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, group.Id, "policy-api-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	serverScope := &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_Server{Server: true}}
	serverValue := int32(7200)
	server, err := env.policies.UpdatePolicyConfiguration(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdatePolicyConfigurationRequest{
		Scope: serverScope, Overrides: &adminv1.PolicyOverrides{AuthorEditWindowSeconds: &serverValue},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window_seconds"}},
	}))
	if err != nil {
		t.Fatalf("Update server policy: %v", err)
	}
	if got := server.Msg.GetPolicyConfiguration().GetEffective().GetAuthorEditWindowSeconds(); got != serverValue {
		t.Fatalf("server effective = %d, want %d", got, serverValue)
	}

	groupValue := int32(3600)
	groupScope := &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_RoomGroupId{RoomGroupId: group.Id}}
	if _, err := env.policies.UpdatePolicyConfiguration(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdatePolicyConfigurationRequest{
		Scope: groupScope, Overrides: &adminv1.PolicyOverrides{AuthorEditWindowSeconds: &groupValue},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window_seconds"}},
	})); err != nil {
		t.Fatalf("Update group policy: %v", err)
	}

	roomValue := int32(1800)
	roomScope := &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_RoomId{RoomId: room.Id}}
	_, err = env.policies.UpdatePolicyConfiguration(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdatePolicyConfigurationRequest{
		Scope: roomScope, Overrides: &adminv1.PolicyOverrides{AuthorEditWindowSeconds: &roomValue},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	roomConfiguration, err := env.policies.UpdatePolicyConfiguration(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdatePolicyConfigurationRequest{
		Scope: roomScope, Overrides: &adminv1.PolicyOverrides{AuthorEditWindowSeconds: &roomValue},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window_seconds"}},
	}))
	if err != nil {
		t.Fatalf("Update room policy: %v", err)
	}
	configuration := roomConfiguration.Msg.GetPolicyConfiguration()
	if configuration.GetOverrides().GetAuthorEditWindowSeconds() != roomValue ||
		configuration.GetEffective().GetAuthorEditWindowSeconds() != roomValue ||
		configuration.GetSources().GetAuthorEditWindow().GetScope() != apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_ROOM {
		t.Fatalf("room policy configuration = %+v", configuration)
	}

	directory, err := env.directory.GetRoom(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got := directory.Msg.GetRoom().GetViewerState().GetPolicies().GetAuthorEditWindowSeconds(); got != roomValue {
		t.Fatalf("directory effective window = %d, want %d", got, roomValue)
	}

	cleared, err := env.policies.UpdatePolicyConfiguration(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdatePolicyConfigurationRequest{
		Scope: roomScope, Overrides: &adminv1.PolicyOverrides{},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"author_edit_window_seconds"}},
	}))
	if err != nil {
		t.Fatalf("Clear room policy: %v", err)
	}
	if cleared.Msg.GetPolicyConfiguration().GetOverrides().AuthorEditWindowSeconds != nil ||
		cleared.Msg.GetPolicyConfiguration().GetEffective().GetAuthorEditWindowSeconds() != groupValue {
		t.Fatalf("cleared room configuration = %+v", cleared.Msg.GetPolicyConfiguration())
	}

	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "policy-api-regular", "Regular", "password123")
	if err != nil {
		t.Fatalf("Create regular: %v", err)
	}
	_, err = env.policies.GetPolicyConfiguration(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.GetPolicyConfigurationRequest{Scope: serverScope}))
	requireConnectCode(t, err, connect.CodePermissionDenied)
}
