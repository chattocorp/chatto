package connectapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestAPIHandlers(t *testing.T) {
	api := New(nil, config.ChattoConfig{}, "test")
	handlers := api.Handlers()

	paths := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		if handler.Handler == nil {
			t.Fatalf("handler for %q is nil", handler.ServicePath)
		}
		paths = append(paths, handler.ServicePath)
	}
	sort.Strings(paths)

	want := []string{
		"/" + apiv1connect.MessageServiceName + "/",
		"/" + apiv1connect.NotificationPreferencesServiceName + "/",
		"/" + apiv1connect.PresenceServiceName + "/",
		"/" + apiv1connect.ReadStateServiceName + "/",
		"/" + apiv1connect.ReactionServiceName + "/",
		"/" + apiv1connect.RoomDirectoryServiceName + "/",
		"/" + apiv1connect.RoomServiceName + "/",
		"/" + apiv1connect.RoomTimelineServiceName + "/",
		"/" + apiv1connect.ServerServiceName + "/",
		"/" + apiv1connect.ThreadServiceName + "/",
		"/" + apiv1connect.UserStatusServiceName + "/",
	}
	sort.Strings(want)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("handler paths = %v, want %v", paths, want)
	}
}

func TestAPIHandlerAuthPolicies(t *testing.T) {
	api := New(nil, config.ChattoConfig{}, "test")
	got := make(map[string]AuthPolicy)
	for _, handler := range api.Handlers() {
		if _, exists := got[handler.ServicePath]; exists {
			t.Fatalf("duplicate handler path %q", handler.ServicePath)
		}
		got[handler.ServicePath] = handler.AuthPolicy
	}

	want := map[string]AuthPolicy{
		"/" + apiv1connect.MessageServiceName + "/":                 AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.NotificationPreferencesServiceName + "/": AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.PresenceServiceName + "/":                AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ReadStateServiceName + "/":               AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ReactionServiceName + "/":                AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomDirectoryServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomServiceName + "/":                    AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomTimelineServiceName + "/":            AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ServerServiceName + "/":                  AuthPolicyPublic,
		"/" + apiv1connect.ThreadServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.UserStatusServiceName + "/":              AuthPolicyAuthenticatedUser,
	}
	if len(got) != len(want) {
		t.Fatalf("auth policy count = %d, want %d (%v)", len(got), len(want), got)
	}
	for servicePath, wantPolicy := range want {
		if gotPolicy := got[servicePath]; gotPolicy != wantPolicy {
			t.Fatalf("auth policy for %s = %q, want %q", servicePath, gotPolicy, wantPolicy)
		}
	}
}

func TestRequireCaller(t *testing.T) {
	t.Run("rejects missing authn info", func(t *testing.T) {
		_, err := requireCaller(context.Background())
		requireConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("rejects wrong authn info type", func(t *testing.T) {
		_, err := requireCaller(authn.SetInfo(context.Background(), "user-id"))
		requireConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("rejects empty caller user id", func(t *testing.T) {
		_, err := requireCaller(authn.SetInfo(context.Background(), Caller{}))
		requireConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("returns typed caller", func(t *testing.T) {
		caller, err := requireCaller(authn.SetInfo(context.Background(), Caller{UserID: "user-123"}))
		if err != nil {
			t.Fatalf("requireCaller: %v", err)
		}
		if caller.UserID != "user-123" {
			t.Fatalf("UserID = %q, want user-123", caller.UserID)
		}
	})
}

func TestPrivateHandlersRequireAuth(t *testing.T) {
	api := New(nil, config.ChattoConfig{}, "test")
	mux := http.NewServeMux()
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL)
	_, err := client.PostMessage(context.Background(), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: "room",
		Body:   "hello",
	}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestServerServiceGetServerPublicMetadata(t *testing.T) {
	api := New(nil, config.ChattoConfig{
		Auth: config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub provider", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
			},
		},
	}, "9.8.7")
	mux := http.NewServeMux()
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := apiv1connect.NewServerServiceClient(ts.Client(), ts.URL)
	resp, err := client.GetServer(context.Background(), connect.NewRequest(&apiv1.GetServerRequest{}))
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}

	msg := resp.Msg
	if msg.Name != "Chatto" {
		t.Fatalf("Name = %q, want Chatto", msg.Name)
	}
	if msg.Version != "9.8.7" {
		t.Fatalf("Version = %q, want 9.8.7", msg.Version)
	}
	if got, want := strings.Join(msg.AuthMethods, ","), "password,oidc"; got != want {
		t.Fatalf("AuthMethods = %v, want %s", msg.AuthMethods, want)
	}
	if len(msg.AuthProviders) != 1 {
		t.Fatalf("AuthProviders len = %d, want 1", len(msg.AuthProviders))
	}
	provider := msg.AuthProviders[0]
	if provider.Id != "hub provider" {
		t.Fatalf("provider Id = %q, want hub provider", provider.Id)
	}
	if provider.LoginUrl != "/auth/providers/hub%20provider" {
		t.Fatalf("provider LoginUrl = %q, want escaped provider path", provider.LoginUrl)
	}
}

func TestRoomServiceLifecycleCommands(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	groupID := env.defaultRoomGroupID(t)

	if _, err := env.rooms.CreateRoom(env.ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:    "connect-room",
		GroupId: groupID,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated CreateRoom code = %v, want unauthenticated", connect.CodeOf(err))
	}

	if _, err := env.rooms.CreateRoom(ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:    "connect room",
		GroupId: groupID,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid CreateRoom name code = %v, want invalid argument", connect.CodeOf(err))
	}

	if _, err := env.rooms.CreateRoom(ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:    "connect-room",
		GroupId: groupID,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreateRoom without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomCreate); err != nil {
		t.Fatalf("GrantServerPermission create: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomManage); err != nil {
		t.Fatalf("GrantServerPermission manage: %v", err)
	}

	createResp, err := env.rooms.CreateRoom(ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:        "connect-room",
		Description: "created through ConnectRPC",
		GroupId:     groupID,
		Universal:   true,
	}))
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := createResp.Msg.GetRoom()
	if room.GetId() == "" || room.GetKind() != apiv1.RoomKind_ROOM_KIND_CHANNEL || room.GetGroupId() != groupID || !room.GetUniversal() {
		t.Fatalf("created room = %+v", room)
	}

	updateResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:      room.GetId(),
		Name:        "connect-renamed",
		Description: "updated through ConnectRPC",
	}))
	if err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if updateResp.Msg.GetRoom().GetName() != "connect-renamed" {
		t.Fatalf("UpdateRoom name = %q, want connect-renamed", updateResp.Msg.GetRoom().GetName())
	}

	archiveResp, err := env.rooms.ArchiveRoom(ctx, connect.NewRequest(&apiv1.ArchiveRoomRequest{RoomId: room.GetId()}))
	if err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	if !archiveResp.Msg.GetRoom().GetArchived() {
		t.Fatalf("ArchiveRoom archived = false, want true")
	}

	unarchiveResp, err := env.rooms.UnarchiveRoom(ctx, connect.NewRequest(&apiv1.UnarchiveRoomRequest{RoomId: room.GetId()}))
	if err != nil {
		t.Fatalf("UnarchiveRoom: %v", err)
	}
	if unarchiveResp.Msg.GetRoom().GetArchived() {
		t.Fatalf("UnarchiveRoom archived = true, want false")
	}

	universalResp, err := env.rooms.SetRoomUniversal(ctx, connect.NewRequest(&apiv1.SetRoomUniversalRequest{
		RoomId:    room.GetId(),
		Universal: false,
	}))
	if err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	if universalResp.Msg.GetRoom().GetUniversal() {
		t.Fatalf("SetRoomUniversal universal = true, want false")
	}
}

func TestRoomServiceMembershipAndModerationCommands(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("connect-members")

	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-ban-target", "Room Ban Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("GrantServerPermission join: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomMemberBan); err != nil {
		t.Fatalf("GrantServerPermission ban: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom target: %v", err)
	}

	if _, err := env.rooms.LeaveRoom(ctx, connect.NewRequest(&apiv1.LeaveRoomRequest{RoomId: room.Id})); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, env.viewer.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after leave: %v", err)
	}
	if isMember {
		t.Fatalf("viewer is still a member after LeaveRoom")
	}

	joinResp, err := env.rooms.JoinRoom(ctx, connect.NewRequest(&apiv1.JoinRoomRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if joinResp.Msg.GetRoom().GetId() != room.Id {
		t.Fatalf("JoinRoom room id = %q, want %s", joinResp.Msg.GetRoom().GetId(), room.Id)
	}

	if _, err := env.rooms.BanRoomMember(ctx, connect.NewRequest(&apiv1.BanRoomMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "  ",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("blank BanRoomMember reason code = %v, want invalid argument", connect.CodeOf(err))
	}

	banResp, err := env.rooms.BanRoomMember(ctx, connect.NewRequest(&apiv1.BanRoomMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "moderation test",
	}))
	if err != nil {
		t.Fatalf("BanRoomMember: %v", err)
	}
	if !banResp.Msg.GetBanned() {
		t.Fatalf("BanRoomMember banned = false, want true")
	}
	isTargetMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, target.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists target after ban: %v", err)
	}
	if isTargetMember {
		t.Fatalf("target is still a member after BanRoomMember")
	}

	unbanResp, err := env.rooms.UnbanRoomMember(ctx, connect.NewRequest(&apiv1.UnbanRoomMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "appeal accepted",
	}))
	if err != nil {
		t.Fatalf("UnbanRoomMember: %v", err)
	}
	if !unbanResp.Msg.GetUnbanned() {
		t.Fatalf("UnbanRoomMember unbanned = false, want true")
	}
}

func TestRoomServiceStartDM(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-dm-participant", "Connect DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	participantTwo, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-dm-participant-two", "Connect DM Participant Two", "password")
	if err != nil {
		t.Fatalf("CreateUser participantTwo: %v", err)
	}

	if _, err := env.rooms.StartDM(env.ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated StartDM code = %v, want unauthenticated", connect.CodeOf(err))
	}

	tooManyParticipants := make([]string, core.MaxDMParticipants)
	for i := range tooManyParticipants {
		tooManyParticipants[i] = "participant"
	}
	if _, err := env.rooms.StartDM(ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: tooManyParticipants,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("oversized StartDM code = %v, want invalid argument", connect.CodeOf(err))
	}

	resp, err := env.rooms.StartDM(ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	}))
	if err != nil {
		t.Fatalf("StartDM: %v", err)
	}
	room := resp.Msg.GetRoom()
	if room.GetKind() != apiv1.RoomKind_ROOM_KIND_DM {
		t.Fatalf("StartDM room kind = %v, want DM", room.GetKind())
	}

	again, err := env.rooms.StartDM(ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	}))
	if err != nil {
		t.Fatalf("StartDM again: %v", err)
	}
	if again.Msg.GetRoom().GetId() != room.GetId() {
		t.Fatalf("StartDM returned different room IDs: %q and %q", room.GetId(), again.Msg.GetRoom().GetId())
	}

	groupResp, err := env.rooms.StartDM(ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id, participantTwo.Id},
	}))
	if err != nil {
		t.Fatalf("StartDM group: %v", err)
	}
	if groupResp.Msg.GetRoom().GetId() == room.GetId() {
		t.Fatalf("group StartDM reused two-person room ID %q", room.GetId())
	}

	blocked, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-dm-blocked", "Connect DM Blocked", "password")
	if err != nil {
		t.Fatalf("CreateUser blocked: %v", err)
	}
	if _, err := env.core.CreateServerRole(env.ctx, core.SystemActorID, "connect-dm-blocked-role", "Connect DM Blocked", ""); err != nil {
		t.Fatalf("CreateServerRole blocked: %v", err)
	}
	if err := env.core.DenyServerPermission(env.ctx, core.SystemActorID, "connect-dm-blocked-role", core.PermMessagePost); err != nil {
		t.Fatalf("DenyServerPermission message.post: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, blocked.Id, "connect-dm-blocked-role"); err != nil {
		t.Fatalf("AssignServerRole blocked: %v", err)
	}
	if _, err := env.rooms.StartDM(withCaller(env.ctx, blocked), connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("StartDM denied user code = %v, want permission denied", connect.CodeOf(err))
	}
}

func TestRoomServiceRejectsDMRooms(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-dm-participant", "Room DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-dm-outsider", "Room DM Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	dm, created, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{participant.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if !created {
		t.Fatalf("expected new DM room")
	}

	outsiderCtx := withCaller(env.ctx, outsider)
	if _, err := env.rooms.JoinRoom(outsiderCtx, connect.NewRequest(&apiv1.JoinRoomRequest{
		RoomId: dm.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("JoinRoom for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	isOutsiderMember, err := env.core.RoomMembershipExists(env.ctx, core.KindDM, outsider.Id, dm.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists outsider: %v", err)
	}
	if isOutsiderMember {
		t.Fatalf("outsider became a DM member through RoomService.JoinRoom")
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomManage); err != nil {
		t.Fatalf("GrantServerPermission manage: %v", err)
	}
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:      dm.Id,
		Name:        "dm-renamed",
		Description: "should not change",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateRoom for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.ArchiveRoom(ctx, connect.NewRequest(&apiv1.ArchiveRoomRequest{
		RoomId: dm.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ArchiveRoom for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.UnarchiveRoom(ctx, connect.NewRequest(&apiv1.UnarchiveRoomRequest{
		RoomId: dm.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UnarchiveRoom for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.SetRoomUniversal(ctx, connect.NewRequest(&apiv1.SetRoomUniversalRequest{
		RoomId:    dm.Id,
		Universal: true,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SetRoomUniversal for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.BanRoomMember(ctx, connect.NewRequest(&apiv1.BanRoomMemberRequest{
		RoomId: dm.Id,
		UserId: participant.Id,
		Reason: "should not ban",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("BanRoomMember for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.UnbanRoomMember(ctx, connect.NewRequest(&apiv1.UnbanRoomMemberRequest{
		RoomId: dm.Id,
		UserId: participant.Id,
		Reason: "should not unban",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UnbanRoomMember for DM code = %v, want invalid argument", connect.CodeOf(err))
	}

	stored, err := env.core.GetRoom(env.ctx, core.KindDM, dm.Id)
	if err != nil {
		t.Fatalf("GetRoom DM after rejected mutations: %v", err)
	}
	if stored.GetName() != "" || stored.GetDescription() != "" || stored.GetArchived() || stored.GetUniversal() {
		t.Fatalf("DM room mutated by rejected RoomService calls: %+v", stored)
	}
}

func TestConnectServicesRejectDMOutsiders(t *testing.T) {
	env := newConnectAPITestEnv(t)

	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-dm-participant", "Connect DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-dm-outsider", "Connect DM Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{participant.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "private root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, participant.Id, "private reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	ctx := withCaller(env.ctx, outsider)
	checkInaccessible := func(name string, err error) {
		t.Helper()
		switch got := connect.CodeOf(err); got {
		case connect.CodePermissionDenied, connect.CodeNotFound:
		default:
			t.Fatalf("%s code = %v, want %v or %v", name, got, connect.CodePermissionDenied, connect.CodeNotFound)
		}
	}

	_, err = env.messages.PostMessage(ctx, connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: dm.Id,
		Body:   "not a participant",
	}))
	checkInaccessible("PostMessage", err)

	_, err = env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: dm.Id,
	}))
	checkInaccessible("GetRoomEvents", err)

	_, err = env.timeline.GetRoomEventsAround(ctx, connect.NewRequest(&apiv1.GetRoomEventsAroundRequest{
		RoomId:  dm.Id,
		EventId: root.Id,
	}))
	checkInaccessible("GetRoomEventsAround", err)

	_, err = env.timeline.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
	}))
	checkInaccessible("GetThreadEvents", err)

	_, err = env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
		EventId:           reply.Id,
	}))
	checkInaccessible("GetThreadEventsAround", err)

	_, err = env.reactions.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         dm.Id,
		MessageEventId: root.Id,
		Emoji:          "thumbsup",
	}))
	checkInaccessible("AddReaction", err)

	_, err = env.reactions.RemoveReaction(ctx, connect.NewRequest(&apiv1.RemoveReactionRequest{
		RoomId:         dm.Id,
		MessageEventId: root.Id,
		Emoji:          "thumbsup",
	}))
	checkInaccessible("RemoveReaction", err)

	_, err = env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      dm.Id,
		UpToEventId: root.Id,
	}))
	checkInaccessible("MarkRoomAsRead", err)

	_, err = env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       reply.Id,
	}))
	checkInaccessible("MarkThreadAsRead", err)

	_, err = env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
	}))
	checkInaccessible("FollowThread", err)

	_, err = env.threads.UnfollowThread(ctx, connect.NewRequest(&apiv1.UnfollowThreadRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
	}))
	checkInaccessible("UnfollowThread", err)

	_, err = env.prefs.GetRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
		RoomId: dm.Id,
	}))
	checkInaccessible("GetRoomNotificationPreference", err)

	_, err = env.prefs.SetRoomNotificationLevel(ctx, connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
		RoomId: dm.Id,
		Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	}))
	checkInaccessible("SetRoomNotificationLevel", err)
}

func TestRoomDirectoryServiceListRoomsVisibilityAndDMs(t *testing.T) {
	env := newConnectAPITestEnv(t)

	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-caller", "Directory Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-dm-participant", "Directory DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}

	visible, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "directory-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	hidden, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "directory-hidden", "")
	if err != nil {
		t.Fatalf("CreateRoom hidden: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, hidden.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission hidden list: %v", err)
	}

	dm, _, err := env.core.FindOrCreateDM(env.ctx, caller.Id, []string{participant.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	dmResp, err := env.directory.ListRooms(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.ListRoomsRequest{
		Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_DMS,
	}))
	if err != nil {
		t.Fatalf("ListRooms empty DMs: %v", err)
	}
	if len(dmResp.Msg.GetRooms()) != 0 {
		t.Fatalf("empty DM list len = %d, want 0", len(dmResp.Msg.GetRooms()))
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, caller.Id, "hello DM", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage DM: %v", err)
	}

	resp, err := env.directory.ListRooms(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.ListRoomsRequest{}))
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	rooms := directoryRoomsByID(resp.Msg.GetRooms())
	if _, ok := rooms[visible.Id]; !ok {
		t.Fatalf("visible room %s missing from directory response", visible.Id)
	}
	if _, ok := rooms[hidden.Id]; ok {
		t.Fatalf("hidden room %s appeared in directory response", hidden.Id)
	}
	dmRoom := rooms[dm.Id]
	if dmRoom == nil {
		t.Fatalf("DM room %s missing after first message", dm.Id)
	}
	if dmRoom.GetRoom().GetKind() != apiv1.RoomKind_ROOM_KIND_DM {
		t.Fatalf("DM kind = %v, want DM", dmRoom.GetRoom().GetKind())
	}
	if !dmRoom.GetViewerState().GetIsMember() {
		t.Fatalf("DM viewer state IsMember = false, want true")
	}
	if !dmRoom.GetViewerState().GetCanListRoom() {
		t.Fatalf("DM viewer state CanListRoom = false, want true")
	}
	if dmRoom.GetViewerState().GetCanJoinRoom() || dmRoom.GetViewerState().GetCanManageRoom() || dmRoom.GetViewerState().GetCanBanRoomMembers() {
		t.Fatalf("DM viewer state exposes channel-only actions: %+v", dmRoom.GetViewerState())
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-dm-outsider", "Directory DM Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.directory.GetRoom(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.GetRoomRequest{
		RoomId: dm.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider GetRoom DM code = %v, want permission denied", connect.CodeOf(err))
	}
}

func TestRoomDirectoryServiceViewerStateMatchesWritePreconditions(t *testing.T) {
	env := newConnectAPITestEnv(t)

	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-state-caller", "Directory State Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	visible, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "directory-state-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	memberArchived, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "directory-state-archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	for _, room := range []*corev1.Room{visible, memberArchived} {
		for _, perm := range []core.Permission{
			core.PermRoomJoin,
			core.PermMessagePost,
			core.PermMessagePostInThread,
			core.PermMessageAttach,
			core.PermMessageReact,
			core.PermMessageEcho,
			core.PermMessageManage,
		} {
			if err := env.core.GrantRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, perm); err != nil {
				t.Fatalf("GrantRoomPermission %s %s: %v", room.Id, perm, err)
			}
		}
	}
	if _, err := env.core.JoinRoom(env.ctx, caller.Id, core.KindChannel, caller.Id, memberArchived.Id); err != nil {
		t.Fatalf("JoinRoom archived target: %v", err)
	}
	if _, err := env.core.ArchiveRoom(env.ctx, env.viewer.Id, core.KindChannel, memberArchived.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}

	resp, err := env.directory.ListRooms(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.ListRoomsRequest{
		Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_CHANNELS,
	}))
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	rooms := directoryRoomsByID(resp.Msg.GetRooms())
	visibleRoom := rooms[visible.Id]
	if visibleRoom == nil {
		t.Fatalf("visible room %s missing from directory response", visible.Id)
	}
	visibleState := visibleRoom.GetViewerState()
	if visibleState.GetIsMember() {
		t.Fatalf("visible room IsMember = true, want false")
	}
	if !visibleState.GetCanJoinRoom() {
		t.Fatalf("visible non-member CanJoinRoom = false, want true")
	}
	if visibleState.GetCanPostMessage() ||
		visibleState.GetCanPostInThread() ||
		visibleState.GetCanAttach() ||
		visibleState.GetCanReact() ||
		visibleState.GetCanEchoMessage() ||
		visibleState.GetCanManageOthersMessage() {
		t.Fatalf("visible non-member exposes member-only actions: %+v", visibleState)
	}
	if _, ok := rooms[memberArchived.Id]; ok {
		t.Fatalf("archived room %s appeared in directory response", memberArchived.Id)
	}

	archivedResp, err := env.directory.GetRoom(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.GetRoomRequest{
		RoomId: memberArchived.Id,
	}))
	if err != nil {
		t.Fatalf("GetRoom archived: %v", err)
	}
	archivedState := archivedResp.Msg.GetRoom().GetViewerState()
	if !archivedState.GetIsMember() {
		t.Fatalf("archived room IsMember = false, want true")
	}
	if archivedState.GetCanJoinRoom() ||
		archivedState.GetCanPostMessage() ||
		archivedState.GetCanPostInThread() ||
		archivedState.GetCanAttach() ||
		archivedState.GetCanReact() ||
		archivedState.GetCanEchoMessage() {
		t.Fatalf("archived room exposes unavailable actions: %+v", archivedState)
	}
	if !archivedState.GetCanManageOthersMessage() {
		t.Fatalf("archived room CanManageOthersMessage = false, want true")
	}
}

func TestRoomDirectoryServiceListRoomGroupsFiltersHiddenRoomsAndKeepsLinks(t *testing.T) {
	env := newConnectAPITestEnv(t)

	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-group-caller", "Directory Group Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	groupID := env.defaultRoomGroupID(t)
	visible, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, groupID, "directory-group-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	hidden, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, groupID, "directory-group-hidden", "")
	if err != nil {
		t.Fatalf("CreateRoom hidden: %v", err)
	}
	archived, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, groupID, "directory-group-archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, hidden.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission hidden list: %v", err)
	}
	if _, err := env.core.ArchiveRoom(env.ctx, env.viewer.Id, core.KindChannel, archived.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	link, err := env.core.CreateSidebarLink(env.ctx, env.viewer.Id, groupID, "Docs", "/docs")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	resp, err := env.directory.ListRoomGroups(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.ListRoomGroupsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomGroups: %v", err)
	}
	group := findDirectoryGroup(resp.Msg.GetGroups(), groupID)
	if group == nil {
		t.Fatalf("group %s missing from response", groupID)
	}
	groupRooms := directoryRoomsByID(group.GetRooms())
	if _, ok := groupRooms[visible.Id]; !ok {
		t.Fatalf("visible room %s missing from group rooms", visible.Id)
	}
	if _, ok := groupRooms[hidden.Id]; ok {
		t.Fatalf("hidden room %s appeared in group rooms", hidden.Id)
	}
	if _, ok := groupRooms[archived.Id]; ok {
		t.Fatalf("archived room %s appeared in group rooms", archived.Id)
	}
	if !roomGroupItemsContainRoom(group.GetItems(), visible.Id) {
		t.Fatalf("visible room %s missing from group items", visible.Id)
	}
	if roomGroupItemsContainRoom(group.GetItems(), hidden.Id) {
		t.Fatalf("hidden room %s appeared in group items", hidden.Id)
	}
	if roomGroupItemsContainRoom(group.GetItems(), archived.Id) {
		t.Fatalf("archived room %s appeared in group items", archived.Id)
	}
	if !roomGroupItemsContainSidebarLink(group.GetItems(), link.Id) {
		t.Fatalf("sidebar link %s missing from group items", link.Id)
	}
}

func TestRoomDirectoryServiceJoinGroup(t *testing.T) {
	env := newConnectAPITestEnv(t)

	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-join-caller", "Directory Join Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	group, err := env.core.CreateRoomGroup(env.ctx, env.viewer.Id, "Directory Join", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	openRoom, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, group.Id, "directory-join-open", "")
	if err != nil {
		t.Fatalf("CreateRoom open: %v", err)
	}
	restricted, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, group.Id, "directory-join-restricted", "")
	if err != nil {
		t.Fatalf("CreateRoom restricted: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, restricted.Id, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission restricted join: %v", err)
	}
	archived, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, group.Id, "directory-join-archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if _, err := env.core.ArchiveRoom(env.ctx, env.viewer.Id, core.KindChannel, archived.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}

	resp, err := env.directory.JoinGroup(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.JoinGroupRequest{
		GroupId: group.Id,
	}))
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}
	if got, want := strings.Join(resp.Msg.GetJoinedRoomIds(), ","), openRoom.Id; got != want {
		t.Fatalf("joined room ids = %q, want %q", got, want)
	}
	if isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, caller.Id, openRoom.Id); err != nil || !isMember {
		t.Fatalf("open membership = %v, %v; want true, nil", isMember, err)
	}
	if isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, caller.Id, restricted.Id); err != nil || isMember {
		t.Fatalf("restricted membership = %v, %v; want false, nil", isMember, err)
	}
	if isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, caller.Id, archived.Id); err != nil || isMember {
		t.Fatalf("archived membership = %v, %v; want false, nil", isMember, err)
	}
}

func TestUserStatusServiceSetAndClearCustomStatus(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	expiresAt := timestamppb.New(time.Now().Add(time.Hour).UTC())

	setResp, err := env.status.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
		Emoji:     "🌿",
		Text:      "In focus mode",
		ExpiresAt: expiresAt,
	}))
	if err != nil {
		t.Fatalf("SetCustomStatus: %v", err)
	}
	if got := setResp.Msg.GetStatus(); got.GetEmoji() != "🌿" || got.GetText() != "In focus mode" {
		t.Fatalf("status = %+v, want focus status", got)
	}
	if got := setResp.Msg.GetStatus().GetExpiresAt(); got == nil || !got.AsTime().Equal(expiresAt.AsTime()) {
		t.Fatalf("ExpiresAt = %v, want %v", got, expiresAt)
	}

	stored, err := env.core.GetUser(ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if stored.GetCustomStatus().GetEmoji() != "🌿" {
		t.Fatalf("stored CustomStatus = %+v, want set status", stored.GetCustomStatus())
	}

	_, err = env.status.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
		Emoji: "🌿",
		Text:  "   ",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SetCustomStatus blank text error = %v, want InvalidArgument", err)
	}

	clearResp, err := env.status.ClearCustomStatus(ctx, connect.NewRequest(&apiv1.ClearCustomStatusRequest{}))
	if err != nil {
		t.Fatalf("ClearCustomStatus: %v", err)
	}
	if clearResp.Msg.GetStatus() != nil {
		t.Fatalf("cleared status = %+v, want nil", clearResp.Msg.GetStatus())
	}
}

func TestPresenceServiceReportPresence(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.presence.ReportPresence(env.ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ReportPresence code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	if _, err := env.presence.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unspecified ReportPresence code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	resp, err := env.presence.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status:       apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB,
		UserSelected: true,
	}))
	if err != nil {
		t.Fatalf("ReportPresence: %v", err)
	}
	if resp.Msg.Status != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("ReportPresence status = %v, want DO_NOT_DISTURB", resp.Msg.Status)
	}

	stored, err := env.core.GetUserPresence(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPresence: %v", err)
	}
	if stored != core.PresenceStatusDoNotDisturb {
		t.Fatalf("stored presence = %q, want %q", stored, core.PresenceStatusDoNotDisturb)
	}

	autoResp, err := env.presence.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
	}))
	if err != nil {
		t.Fatalf("automatic online ReportPresence: %v", err)
	}
	if autoResp.Msg.Status != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("automatic online response status = %v, want DO_NOT_DISTURB", autoResp.Msg.Status)
	}
	stored, err = env.core.GetUserPresence(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPresence after automatic online: %v", err)
	}
	if stored != core.PresenceStatusDoNotDisturb {
		t.Fatalf("automatic online stored presence = %q, want %q", stored, core.PresenceStatusDoNotDisturb)
	}

	if _, err := env.presence.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status:       apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
		UserSelected: true,
	})); err != nil {
		t.Fatalf("explicit online ReportPresence: %v", err)
	}
	stored, err = env.core.GetUserPresence(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPresence after explicit online: %v", err)
	}
	if stored != core.PresenceStatusOnline {
		t.Fatalf("explicit online stored presence = %q, want %q", stored, core.PresenceStatusOnline)
	}
}

func TestAbsolutizeAssetURL(t *testing.T) {
	t.Run("uses configured webserver URL first", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://configured.example.com/chatto"},
		}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://request.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "/assets/logo.png"), "https://configured.example.com/assets/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})

	t.Run("falls back to request base URL", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://remote.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "/assets/logo.png"), "https://remote.example.com/assets/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})

	t.Run("keeps already absolute URLs", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://remote.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "https://cdn.example.com/logo.png"), "https://cdn.example.com/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})
}

func TestRoomTimelineServiceRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "timeline-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	req := connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id})
	if _, err := env.timeline.GetRoomEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-outsider", "Timeline Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.timeline.GetRoomEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServicePostMessageRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-authz")
	req := connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "hello",
	})

	if _, err := env.messages.PostMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-outsider", "Message Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.PostMessage(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.PostMessage(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestReactionServiceAddAndRemoveRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-authz")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	req := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})

	if _, err := env.reactions.AddReaction(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reaction-outsider", "Reaction Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.reactions.AddReaction(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageReact); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.reactions.AddReaction(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestReactionServiceAddAndRemoveResponseSemantics(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-response")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	ctx := withCaller(env.ctx, env.viewer)

	addReq := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	addResp, err := env.reactions.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !addResp.Msg.Added {
		t.Fatal("AddReaction Added = false, want true")
	}

	addResp, err = env.reactions.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("duplicate AddReaction: %v", err)
	}
	if addResp.Msg.Added {
		t.Fatal("duplicate AddReaction Added = true, want false")
	}

	removeReq := connect.NewRequest(&apiv1.RemoveReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	removeResp, err := env.reactions.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if !removeResp.Msg.Removed {
		t.Fatal("RemoveReaction Removed = false, want true")
	}

	removeResp, err = env.reactions.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("duplicate RemoveReaction: %v", err)
	}
	if removeResp.Msg.Removed {
		t.Fatal("duplicate RemoveReaction Removed = true, want false")
	}
}

func TestReactionServiceValidatesEmoji(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-validation")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")

	_, err := env.reactions.AddReaction(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "totally_bogus",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid emoji AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestMessageServicePostMessageValidatesInput(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-validation")
	ctx := withCaller(env.ctx, env.viewer)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	tests := []struct {
		name string
		req  *apiv1.PostMessageRequest
		code connect.Code
	}{
		{
			name: "missing room",
			req:  &apiv1.PostMessageRequest{Body: "hello"},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "empty body and no attachments",
			req:  &apiv1.PostMessageRequest{RoomId: room.Id, Body: "   "},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "channel echo outside thread",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "hello",
				AlsoSendToChannel: true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "link preview URL too long",
			req: &apiv1.PostMessageRequest{
				RoomId: room.Id,
				Body:   "hello",
				LinkPreview: &apiv1.MessageLinkPreviewInput{
					Url: strings.Repeat("x", core.MaxLinkPreviewURLLength+1),
				},
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := env.messages.PostMessage(ctx, connect.NewRequest(tt.req)); connect.CodeOf(err) != tt.code {
				t.Fatalf("PostMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
		})
	}
}

func TestMessageServicePostMessageInfersVideoProcessingAssetIDs(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-video")

	original, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("original video")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}

	if _, err := env.messages.PostMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{original.Id},
	})); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	manifest, ok := env.core.Assets.VideoAttachmentManifest(original.Id)
	if !ok || manifest.Started == nil {
		t.Fatalf("VideoAttachmentManifest = %+v, %v; want started", manifest, ok)
	}
}

func TestMessageServicePostMessageReturnsRenderableTimelineEvent(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-success")

	resp, err := env.messages.PostMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "hello over connect",
	}))
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	event := resp.Msg.GetEvent()
	if event == nil {
		t.Fatalf("PostMessage event = nil, response = %+v", resp.Msg)
	}
	message := event.GetMessagePosted()
	if message == nil {
		t.Fatalf("PostMessage payload = %T, want message_posted", event.GetEvent())
	}
	if message.Body == nil || message.GetBody() != "hello over connect" {
		t.Fatalf("message body = %q present=%v, want posted body", message.GetBody(), message.Body != nil)
	}
	if got := resp.Msg.GetIncludes().GetUsers()[env.viewer.Id]; got == nil || got.DisplayName != env.viewer.DisplayName {
		t.Fatalf("included viewer = %+v, want %q", got, env.viewer.DisplayName)
	}
}

func TestMessageServiceUpdateMessageAuthorAndRBAC(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-update-rbac")
	authorCtx := withCaller(env.ctx, env.viewer)
	original := env.post(room.Id, env.viewer.Id, "original", "")

	if _, err := env.messages.UpdateMessage(env.ctx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    "ignored",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-outsider", "Message Update Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    "ignored",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-other", "Message Update Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, other), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    "ignored",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member without manage UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if _, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    "author edit",
	})); err != nil {
		t.Fatalf("author UpdateMessage: %v", err)
	}
	if body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, original.Id); err != nil || body != "author edit" {
		t.Fatalf("body after author edit = %q, %v; want author edit, nil", body, err)
	}

	echo := false
	if _, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:            room.Id,
		EventId:           original.Id,
		Body:              "invalid echo edit",
		AlsoSendToChannel: &echo,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("root echo-state UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	moderator, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-moderator", "Message Update Moderator", "password")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom moderator: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, moderator.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission moderator manage: %v", err)
	}
	moderated := env.post(room.Id, env.viewer.Id, "moderated original", "")
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: moderated.Id,
		Body:    "moderator edit",
	})); err != nil {
		t.Fatalf("moderator UpdateMessage: %v", err)
	}
	if body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, moderated.Id); err != nil || body != "moderator edit" {
		t.Fatalf("body after moderator edit = %q, %v; want moderator edit, nil", body, err)
	}

	echo = true
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:            room.Id,
		EventId:           moderated.Id,
		Body:              "moderator echo edit",
		AlsoSendToChannel: &echo,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("moderator echo UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceDeleteMessageAuthorAndRBAC(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-delete-rbac")
	target := env.post(room.Id, env.viewer.Id, "delete target", "")

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-delete-other", "Message Delete Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if _, err := env.messages.DeleteMessage(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: target.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member without manage DeleteMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	moderator, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-delete-moderator", "Message Delete Moderator", "password")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom moderator: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, moderator.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission moderator manage: %v", err)
	}
	resp, err := env.messages.DeleteMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: target.Id,
	}))
	if err != nil {
		t.Fatalf("moderator DeleteMessage: %v", err)
	}
	if !resp.Msg.Deleted {
		t.Fatal("moderator DeleteMessage Deleted = false, want true")
	}
	if body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, target.Id); err != nil || body != "" {
		t.Fatalf("body after moderator delete = %q, %v; want empty, nil", body, err)
	}

	own := env.post(room.Id, env.viewer.Id, "own delete target", "")
	if _, err := env.messages.DeleteMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: own.Id,
	})); err != nil {
		t.Fatalf("author DeleteMessage: %v", err)
	}
}

func TestMessageServiceDeleteAttachmentAndLinkPreviewAuthorOnly(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-partial-delete")

	attachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "note.txt", "text/plain", bytes.NewReader([]byte("note")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	attachmentEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage attachment: %v", err)
	}
	previewURL := "https://example.test/preview"
	previewEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "with preview", nil, "", "", &corev1.LinkPreview{
		Url:   previewURL,
		Title: "Preview",
	}, false)
	if err != nil {
		t.Fatalf("PostMessage preview: %v", err)
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-partial-other", "Message Partial Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, other.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission other manage: %v", err)
	}
	if _, err := env.messages.DeleteAttachment(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteAttachmentRequest{
		RoomId:       room.Id,
		EventId:      attachmentEvent.Id,
		AttachmentId: attachment.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-author DeleteAttachment code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if _, err := env.messages.DeleteLinkPreview(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteLinkPreviewRequest{
		RoomId:  room.Id,
		EventId: previewEvent.Id,
		Url:     previewURL,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-author DeleteLinkPreview code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if _, err := env.messages.DeleteAttachment(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteAttachmentRequest{
		RoomId:       room.Id,
		EventId:      attachmentEvent.Id,
		AttachmentId: attachment.Id,
	})); err != nil {
		t.Fatalf("author DeleteAttachment: %v", err)
	}
	body, err := env.core.GetFullMessageBody(env.ctx, core.KindChannel, attachmentEvent.Id)
	if err != nil {
		t.Fatalf("GetFullMessageBody attachment: %v", err)
	}
	if len(body.Attachments) != 0 {
		t.Fatalf("attachments after delete = %d, want 0", len(body.Attachments))
	}

	if _, err := env.messages.DeleteLinkPreview(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteLinkPreviewRequest{
		RoomId:  room.Id,
		EventId: previewEvent.Id,
		Url:     previewURL,
	})); err != nil {
		t.Fatalf("author DeleteLinkPreview: %v", err)
	}
	body, err = env.core.GetFullMessageBody(env.ctx, core.KindChannel, previewEvent.Id)
	if err != nil {
		t.Fatalf("GetFullMessageBody preview: %v", err)
	}
	if body.LinkPreview != nil {
		t.Fatalf("link preview after delete = %+v, want nil", body.LinkPreview)
	}
}

func TestMessageServiceSendTypingIndicatorRequiresMembershipOnly(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-typing")
	req := connect.NewRequest(&apiv1.SendTypingIndicatorRequest{RoomId: room.Id})

	if _, err := env.messages.SendTypingIndicator(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated SendTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-typing-outsider", "Message Typing Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.SendTypingIndicator(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider SendTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission post: %v", err)
	}
	resp, err := env.messages.SendTypingIndicator(withCaller(env.ctx, env.viewer), req)
	if err != nil {
		t.Fatalf("member SendTypingIndicator with post denied: %v", err)
	}
	if !resp.Msg.Sent {
		t.Fatal("SendTypingIndicator Sent = false, want true")
	}
}

func TestRoomTimelineServiceGetRoomEventsPaginatesWithOpaqueCursors(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-pagination")

	m1 := env.post(room.Id, env.viewer.Id, "one", "")
	m2 := env.post(room.Id, env.viewer.Id, "two", "")
	m3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  2,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents latest: %v", err)
	}
	page := resp.Msg.GetPage()
	if len(page.Events) != 2 {
		t.Fatalf("latest event count = %d, want 2", len(page.Events))
	}
	if got := []string{page.Events[0].Id, page.Events[1].Id}; got[0] != m2.Id || got[1] != m3.Id {
		t.Fatalf("latest event IDs = %v, want [%s %s]", got, m2.Id, m3.Id)
	}
	if !page.HasOlder || page.HasNewer {
		t.Fatalf("latest page HasOlder/HasNewer = %v/%v, want true/false", page.HasOlder, page.HasNewer)
	}
	if !strings.HasPrefix(page.StartCursor, roomTimelineCursorSeqPrefix) || !strings.HasPrefix(page.EndCursor, roomTimelineCursorSeqPrefix) {
		t.Fatalf("cursors = %q/%q, want seq-prefixed cursors", page.StartCursor, page.EndCursor)
	}

	olderResp, err := env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
		Cursor: &apiv1.GetRoomEventsRequest_Before{Before: page.StartCursor},
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents before: %v", err)
	}
	if !timelinePageContains(olderResp.Msg.GetPage(), m1.Id) {
		t.Fatalf("older page does not contain first message %s", m1.Id)
	}
	if olderResp.Msg.GetPage().HasNewer != true {
		t.Fatalf("older page HasNewer = false, want true")
	}
}

func TestRoomTimelineServiceHydratesMessagesWithoutClientNPlusOne(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-hydration")
	replier, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-replier", "Timeline Replier", "password")
	if err != nil {
		t.Fatalf("CreateUser replier: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, replier.Id, core.KindChannel, replier.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom replier: %v", err)
	}

	root := env.post(room.Id, env.viewer.Id, "root", "")
	env.post(room.Id, replier.Id, "reply", root.Id)
	if _, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, root.Id, "thumbsup", env.viewer.Id); err != nil {
		t.Fatalf("AddReaction viewer: %v", err)
	}
	if _, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, root.Id, "thumbsup", replier.Id); err != nil {
		t.Fatalf("AddReaction replier: %v", err)
	}

	resp, err := env.timeline.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	event := timelinePageEvent(resp.Msg.GetPage(), root.Id)
	if event == nil {
		t.Fatalf("root message %s not found in page", root.Id)
	}
	payload := event.GetMessagePosted()
	if payload == nil {
		t.Fatalf("root payload = nil, want message_posted")
	}
	if payload.Body == nil || payload.GetBody() != "root" {
		t.Fatalf("message body present/body = %v/%q, want true/root", payload.Body != nil, payload.GetBody())
	}
	if payload.ReplyCount != 1 {
		t.Fatalf("reply count = %d, want 1", payload.ReplyCount)
	}
	if got := payload.ThreadParticipantUserIds; len(got) != 1 || got[0] != replier.Id {
		t.Fatalf("thread participants = %v, want [%s]", got, replier.Id)
	}
	if len(payload.Reactions) != 1 {
		t.Fatalf("reaction summaries = %d, want 1", len(payload.Reactions))
	}
	reaction := payload.Reactions[0]
	if reaction.Emoji != "thumbsup" || reaction.Count != 2 || !reaction.HasReacted {
		t.Fatalf("reaction = %+v, want thumbsup count 2 hasReacted true", reaction)
	}
	if resp.Msg.GetPage().Includes.GetUsers()[env.viewer.Id].DisplayName != "Timeline Viewer" {
		t.Fatalf("viewer include missing or wrong: %+v", resp.Msg.GetPage().Includes.GetUsers()[env.viewer.Id])
	}
	if resp.Msg.GetPage().Includes.GetUsers()[replier.Id].DisplayName != "Timeline Replier" {
		t.Fatalf("replier include missing or wrong: %+v", resp.Msg.GetPage().Includes.GetUsers()[replier.Id])
	}
}

func TestRoomTimelineServiceGetThreadEventsIncludesRootAndPaginatesReplies(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.timeline.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             2,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents latest: %v", err)
	}
	page := resp.Msg.GetPage()
	gotIDs := timelinePageEventIDs(page)
	wantIDs := []string{root.Id, reply2.Id, reply3.Id}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("thread latest event IDs = %v, want %v", gotIDs, wantIDs)
	}
	if !page.HasOlder || page.HasNewer {
		t.Fatalf("thread latest HasOlder/HasNewer = %v/%v, want true/false", page.HasOlder, page.HasNewer)
	}
	if page.StartCursor == "" || page.EndCursor == "" {
		t.Fatalf("thread reply cursors are empty, want reply-window cursors")
	}

	olderResp, err := env.timeline.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
		Cursor:            &apiv1.GetThreadEventsRequest_Before{Before: page.StartCursor},
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents before: %v", err)
	}
	olderIDs := timelinePageEventIDs(olderResp.Msg.GetPage())
	wantOlderIDs := []string{reply1.Id}
	if strings.Join(olderIDs, ",") != strings.Join(wantOlderIDs, ",") {
		t.Fatalf("thread older event IDs = %v, want %v", olderIDs, wantOlderIDs)
	}
	if olderResp.Msg.GetPage().HasOlder {
		t.Fatalf("older thread page HasOlder = true, want false")
	}
}

func TestRoomTimelineServiceGetThreadEventsAroundRootAndReply(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-around")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           root.Id,
		Limit:             2,
	}))
	if err != nil {
		t.Fatalf("GetThreadEventsAround root: %v", err)
	}
	rootIDs := timelinePageEventIDs(rootResp.Msg.GetPage())
	wantRootIDs := []string{root.Id, reply1.Id, reply2.Id}
	if strings.Join(rootIDs, ",") != strings.Join(wantRootIDs, ",") {
		t.Fatalf("root-anchored thread IDs = %v, want %v", rootIDs, wantRootIDs)
	}
	if rootResp.Msg.TargetIndex != 0 {
		t.Fatalf("root target index = %d, want 0", rootResp.Msg.TargetIndex)
	}
	if rootResp.Msg.GetPage().HasOlder || !rootResp.Msg.GetPage().HasNewer {
		t.Fatalf("root-anchored HasOlder/HasNewer = %v/%v, want false/true", rootResp.Msg.GetPage().HasOlder, rootResp.Msg.GetPage().HasNewer)
	}

	replyResp, err := env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           reply2.Id,
		Limit:             3,
	}))
	if err != nil {
		t.Fatalf("GetThreadEventsAround reply: %v", err)
	}
	replyIDs := timelinePageEventIDs(replyResp.Msg.GetPage())
	wantReplyIDs := []string{root.Id, reply1.Id, reply2.Id, reply3.Id}
	if strings.Join(replyIDs, ",") != strings.Join(wantReplyIDs, ",") {
		t.Fatalf("reply-anchored thread IDs = %v, want %v", replyIDs, wantReplyIDs)
	}
	if replyResp.Msg.TargetIndex != 2 {
		t.Fatalf("reply target index = %d, want 2", replyResp.Msg.TargetIndex)
	}

	_, err = env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           "missing-anchor",
		Limit:             3,
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("missing anchor code = %v, want %v", got, connect.CodeNotFound)
	}
}

func TestRoomTimelineServiceGetThreadEventsRequiresMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-authz")
	root := env.post(room.Id, env.viewer.Id, "root", "")

	req := connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.timeline.GetThreadEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-outsider", "Thread Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.timeline.GetThreadEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestRoomTimelineServiceHydratesProcessedVideoAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-video")

	original, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("original video")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}
	thumbnail, err := env.core.UploadDerivativeAttachment(env.ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "clip.thumbnail", "application/octet-stream", bytes.NewReader([]byte("thumbnail")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment thumbnail: %v", err)
	}
	variant, err := env.core.UploadDerivativeAttachment(env.ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, room.Id, "clip-720p.mp4", "video/mp4", bytes.NewReader([]byte("variant video")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment variant: %v", err)
	}

	event, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "video", []string{original.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.RecordAssetProcessed(env.ctx, core.SystemActorID, core.KindChannel, room.Id, event.Id, original.Id, 1234, 1280, 720, thumbnail, []*corev1.VideoVariant{
		{
			AttachmentId: variant.Id,
			Quality:      "720p",
			Width:        1280,
			Height:       720,
			Size:         variant.Size,
			Attachment:   variant,
		},
	}); err != nil {
		t.Fatalf("RecordAssetProcessed: %v", err)
	}

	resp, err := env.timeline.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	apiEvent := timelinePageEvent(resp.Msg.GetPage(), event.Id)
	if apiEvent == nil || apiEvent.GetMessagePosted() == nil {
		t.Fatalf("message event %s not found in page", event.Id)
	}
	attachments := apiEvent.GetMessagePosted().GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	processing := attachments[0].GetVideoProcessing()
	if processing == nil {
		t.Fatal("videoProcessing = nil, want completed manifest")
	}
	if processing.GetStatus() != apiv1.RoomTimelineVideoProcessingStatus_ROOM_TIMELINE_VIDEO_PROCESSING_STATUS_COMPLETED {
		t.Fatalf("videoProcessing status = %v, want COMPLETED", processing.GetStatus())
	}
	if processing.GetDurationMs() != 1234 || processing.GetWidth() != 1280 || processing.GetHeight() != 720 {
		t.Fatalf("videoProcessing dimensions = %d/%d/%d, want 1234/1280/720", processing.GetDurationMs(), processing.GetWidth(), processing.GetHeight())
	}
	if processing.GetThumbnailAssetUrl().GetUrl() == "" {
		t.Fatal("videoProcessing thumbnail URL is empty")
	}
	if len(processing.GetVariants()) != 1 || processing.GetVariants()[0].GetQuality() != "720p" {
		t.Fatalf("videoProcessing variants = %+v, want one 720p variant", processing.GetVariants())
	}
	if processing.GetVariants()[0].GetAssetUrl().GetUrl() == "" {
		t.Fatal("videoProcessing variant URL is empty")
	}
}

func TestRoomTimelineHydratorRejectsUnsupportedEvents(t *testing.T) {
	env := newConnectAPITestEnv(t)
	h := &timelineHydrator{
		api:      env.api,
		ctx:      env.ctx,
		viewerID: env.viewer.Id,
		kind:     core.KindChannel,
		userIDs:  make(map[string]struct{}),
	}

	_, err := h.event(&core.RoomEvent{Event: &corev1.Event{
		Id:      "Eunsupported",
		ActorId: env.viewer.Id,
		Event: &corev1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{RoomId: "Runsupported"},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported room timeline event") {
		t.Fatalf("unsupported event error = %v, want unsupported room timeline event", err)
	}
}

func TestRoomTimelineHydratorSupportsVisibleCoreEvents(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-visible-events")
	posted := env.post(room.Id, env.viewer.Id, "visible root", "")

	h := &timelineHydrator{
		api:      env.api,
		ctx:      env.ctx,
		viewerID: env.viewer.Id,
		kind:     core.KindChannel,
		userIDs:  make(map[string]struct{}),
	}

	tests := []struct {
		name  string
		event *corev1.Event
	}{
		{
			name:  "message posted",
			event: posted,
		},
		{
			name: "room created",
			event: &corev1.Event{
				Id:      "Eroom-created",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomCreated{
					RoomCreated: &corev1.RoomCreatedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room updated",
			event: &corev1.Event{
				Id:      "Eroom-updated",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomUpdated{
					RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room deleted",
			event: &corev1.Event{
				Id:      "Eroom-deleted",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomDeleted{
					RoomDeleted: &corev1.RoomDeletedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room archived",
			event: &corev1.Event{
				Id:      "Eroom-archived",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomArchived{
					RoomArchived: &corev1.RoomArchivedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room unarchived",
			event: &corev1.Event{
				Id:      "Eroom-unarchived",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomUnarchived{
					RoomUnarchived: &corev1.RoomUnarchivedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "user joined room",
			event: &corev1.Event{
				Id:      "Euser-joined",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_UserJoinedRoom{
					UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "user left room",
			event: &corev1.Event{
				Id:      "Euser-left",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_UserLeftRoom{
					UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: room.Id},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !core.IsVisibleRoomTimelineEntry(tt.event) {
				t.Fatalf("test event is not visible according to core")
			}
			if _, err := h.event(&core.RoomEvent{Event: tt.event}); err != nil {
				t.Fatalf("hydrate visible event: %v", err)
			}
		})
	}
}

func TestReadStateServiceRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-authz")

	req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{RoomId: room.Id})
	if _, err := env.readState.MarkRoomAsRead(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-outsider", "Read State Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.readState.MarkRoomAsRead(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestReadStateServiceMarkRoomAsReadAnchorsAndDoesNotRegress(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-room")

	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-reader", "Read State Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, reader.Id, core.KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	e1 := env.post(room.Id, env.viewer.Id, "one", "")
	if err := env.core.SetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id, e1.Id); err != nil {
		t.Fatalf("seed read marker: %v", err)
	}
	e2 := env.post(room.Id, env.viewer.Id, "two", "")
	e3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := withCaller(env.ctx, reader)
	resp, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e2.Id,
	}))
	if err != nil {
		t.Fatalf("MarkRoomAsRead e2: %v", err)
	}
	if resp.Msg.LastReadAt == nil || resp.Msg.PreviousLastReadAt == nil {
		t.Fatalf("timestamps = last %v previous %v, want both set", resp.Msg.LastReadAt, resp.Msg.PreviousLastReadAt)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after e2 = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead stale e1: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after stale e1 = %q, %v; want %s", got, err, e2.Id)
	}

	reply := env.post(room.Id, env.viewer.Id, "reply", e2.Id)
	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkRoomAsRead reply anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after reply anchor = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after missing event = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId: room.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead omitted anchor: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e3.Id {
		t.Fatalf("marker after omitted anchor = %q, %v; want %s", got, err, e3.Id)
	}
}

func TestReadStateServiceMarkRoomAsReadRejectsMissingAnchorWithoutLazyMarker(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "read-state-universal", "", core.WithUniversalRoom(true))
	if err != nil {
		t.Fatalf("CreateRoom universal: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}
	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-lazy-reader", "Read State Lazy Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}

	e1 := env.post(room.Id, env.viewer.Id, "one", "")
	e2 := env.post(room.Id, env.viewer.Id, "two", "")
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker before request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	ctx := withCaller(env.ctx, reader)
	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker after rejected request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	resp, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	}))
	if err != nil {
		t.Fatalf("MarkRoomAsRead e1 after rejected request: %v", err)
	}
	if resp.Msg.PreviousLastReadAt != nil {
		t.Fatalf("PreviousLastReadAt after missing marker = %v, want nil", resp.Msg.PreviousLastReadAt)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e1.Id {
		t.Fatalf("marker after valid e1 = %q, %v; want %s", got, err, e1.Id)
	}
	if e2.Id == e1.Id {
		t.Fatal("test setup posted duplicate event IDs")
	}
}

func TestReadStateServiceMarkThreadAsReadAnchorsAndDoesNotRegress(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-thread")

	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-thread-reader", "Read State Thread Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, reader.Id, core.KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)

	ctx := withCaller(env.ctx, reader)
	resp, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       reply2.Id,
	}))
	if err != nil {
		t.Fatalf("MarkThreadAsRead reply2: %v", err)
	}
	if resp.Msg.PreviousReadAt != nil {
		t.Fatalf("first previous read at = %v, want nil", resp.Msg.PreviousReadAt)
	}
	marker2, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after reply2: %v", err)
	}

	resp, err = env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       reply1.Id,
	}))
	if err != nil {
		t.Fatalf("MarkThreadAsRead stale reply1: %v", err)
	}
	if resp.Msg.PreviousReadAt == nil {
		t.Fatalf("second previous read at = nil, want previous marker")
	}
	markerAfter, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after stale reply1: %v", err)
	}
	if !markerAfter.Equal(marker2) {
		t.Fatalf("thread marker regressed from %v to %v", marker2, markerAfter)
	}

	otherRoot := env.post(room.Id, env.viewer.Id, "other root", "")
	otherReply := env.post(room.Id, env.viewer.Id, "other reply", otherRoot.Id)
	if _, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       otherReply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkThreadAsRead cross-thread anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	markerAfterInvalid, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after invalid anchor: %v", err)
	}
	if !markerAfterInvalid.Equal(marker2) {
		t.Fatalf("thread marker changed after invalid anchor from %v to %v", marker2, markerAfterInvalid)
	}

	if _, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkThreadAsRead missing anchor code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	markerAfterMissing, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after missing anchor: %v", err)
	}
	if !markerAfterMissing.Equal(marker2) {
		t.Fatalf("thread marker changed after missing anchor from %v to %v", marker2, markerAfterMissing)
	}
}

func TestThreadServiceRequiresMembershipAndTogglesFollowState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("thread-follow")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	req := connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.threads.FollowThread(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-follow-outsider", "Thread Follow Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.threads.FollowThread(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: "missing-root",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing root FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("reply root FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	followResp, err := env.threads.FollowThread(ctx, req)
	if err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if !followResp.Msg.Following {
		t.Fatalf("FollowThread following = false, want true")
	}
	isFollowing, err := env.core.IsFollowingThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("IsFollowingThread after follow: %v", err)
	}
	if !isFollowing {
		t.Fatalf("core follow state = false, want true")
	}

	unfollowResp, err := env.threads.UnfollowThread(ctx, connect.NewRequest(&apiv1.UnfollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	}))
	if err != nil {
		t.Fatalf("UnfollowThread: %v", err)
	}
	if unfollowResp.Msg.Following {
		t.Fatalf("UnfollowThread following = true, want false")
	}
	isFollowing, err = env.core.IsFollowingThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("IsFollowingThread after unfollow: %v", err)
	}
	if isFollowing {
		t.Fatalf("core follow state = true, want false")
	}
}

func TestNotificationLevelMapping(t *testing.T) {
	valid := []struct {
		name string
		api  apiv1.NotificationLevel
		core corev1.NotificationLevel
	}{
		{"default clears core override", apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED},
		{"muted", apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED},
		{"normal", apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL},
		{"all messages", apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := apiNotificationLevelToCore(tt.api)
			if err != nil {
				t.Fatalf("apiNotificationLevelToCore(%v) returned error: %v", tt.api, err)
			}
			if got != tt.core {
				t.Fatalf("apiNotificationLevelToCore(%v) = %v, want %v", tt.api, got, tt.core)
			}
		})
	}

	invalid := []struct {
		name string
		api  apiv1.NotificationLevel
	}{
		{"unspecified is not user intent", apiv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED},
		{"unknown enum", apiv1.NotificationLevel(99)},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := apiNotificationLevelToCore(tt.api)
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("apiNotificationLevelToCore(%v) error code = %v, want %v", tt.api, got, connect.CodeInvalidArgument)
			}
		})
	}

	if got := coreNotificationLevelToAPI(corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED); got != apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Fatalf("core unspecified maps to %v, want DEFAULT", got)
	}
}

func TestConnectErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code connect.Code
	}{
		{"not authenticated", core.ErrNotAuthenticated, connect.CodeUnauthenticated},
		{"permission denied", core.ErrPermissionDenied, connect.CodePermissionDenied},
		{"not room member", core.ErrNotRoomMember, connect.CodePermissionDenied},
		{"not message author", core.ErrNotMessageAuthor, connect.CodePermissionDenied},
		{"core not found", core.ErrNotFound, connect.CodeNotFound},
		{"message not found", core.ErrMessageNotFound, connect.CodeNotFound},
		{"message attachment not found", core.ErrMessageAttachmentNotFound, connect.CodeNotFound},
		{"message link preview not found", core.ErrMessageLinkPreviewNotFound, connect.CodeNotFound},
		{"jetstream key not found", jetstream.ErrKeyNotFound, connect.CodeNotFound},
		{"message too long", core.ErrMessageTooLong, connect.CodeInvalidArgument},
		{"invalid argument", core.ErrInvalidArgument, connect.CodeInvalidArgument},
		{"string length", &core.StringLengthError{Field: "field", Max: 10}, connect.CodeInvalidArgument},
		{"room archived", core.ErrRoomArchived, connect.CodeFailedPrecondition},
		{"edit window expired", core.ErrEditWindowExpired, connect.CodeFailedPrecondition},
		{"unknown", errors.New("boom"), connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connect.CodeOf(connectError(tt.err)); got != tt.code {
				t.Fatalf("connectError code = %v, want %v", got, tt.code)
			}
		})
	}

	if err := connectError(errors.New("boom")); strings.Contains(err.Error(), "boom") {
		t.Fatalf("connectError leaked internal error: %v", err)
	}
}

func requireConnectCode(t testing.TB, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("connect code = %v, want %v (err = %v)", got, want, err)
	}
}

func withCaller(ctx context.Context, user *corev1.User) context.Context {
	return authn.SetInfo(ctx, Caller{UserID: user.Id})
}

type connectAPITestEnv struct {
	ctx       context.Context
	core      *core.ChattoCore
	nc        *nats.Conn
	api       *API
	directory *roomDirectoryService
	messages  *messageService
	prefs     *notificationPreferencesService
	readState *readStateService
	presence  *presenceService
	reactions *reactionService
	rooms     *roomService
	timeline  *roomTimelineService
	status    *userStatusService
	threads   *threadService
	viewer    *corev1.User
}

func newConnectAPITestEnv(t *testing.T) *connectAPITestEnv {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startConnectAPITestCore(t, c)

	viewer, err := c.CreateUser(ctx, core.SystemActorID, "timeline-viewer", "Timeline Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	api := New(c, config.ChattoConfig{}, "test")
	return &connectAPITestEnv{
		ctx:       ctx,
		core:      c,
		nc:        nc,
		api:       api,
		directory: &roomDirectoryService{api: api},
		messages:  &messageService{api: api},
		prefs:     &notificationPreferencesService{api: api},
		readState: &readStateService{api: api},
		presence:  &presenceService{api: api},
		reactions: &reactionService{api: api},
		rooms:     &roomService{api: api},
		timeline:  &roomTimelineService{api: api},
		status:    &userStatusService{api: api},
		threads:   &threadService{api: api},
		viewer:    viewer,
	}
}

func startConnectAPITestCore(t *testing.T, c *core.ChattoCore) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func (e *connectAPITestEnv) createJoinedRoom(name string) *corev1.Room {
	room, err := e.core.CreateRoom(e.ctx, e.viewer.Id, core.KindChannel, "", name, "")
	if err != nil {
		panic(err)
	}
	if _, err := e.core.JoinRoom(e.ctx, e.viewer.Id, core.KindChannel, e.viewer.Id, room.Id); err != nil {
		panic(err)
	}
	return room
}

func (e *connectAPITestEnv) defaultRoomGroupID(t *testing.T) string {
	t.Helper()
	groups, err := e.core.ListRoomGroupsOrdered(e.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatalf("expected at least one default room group")
	}
	return groups[0].Id
}

func (e *connectAPITestEnv) post(roomID, actorID, body, inReplyTo string) *corev1.Event {
	event, err := e.core.PostMessage(e.ctx, core.KindChannel, roomID, actorID, body, nil, inReplyTo, "", nil, false)
	if err != nil {
		panic(err)
	}
	return event
}

func timelinePageContains(page *apiv1.RoomTimelinePage, eventID string) bool {
	return timelinePageEvent(page, eventID) != nil
}

func timelinePageEvent(page *apiv1.RoomTimelinePage, eventID string) *apiv1.RoomTimelineEvent {
	for _, event := range page.Events {
		if event.Id == eventID {
			return event
		}
	}
	return nil
}

func timelinePageEventIDs(page *apiv1.RoomTimelinePage) []string {
	ids := make([]string, 0, len(page.Events))
	for _, event := range page.Events {
		ids = append(ids, event.Id)
	}
	return ids
}

func directoryRoomsByID(rooms []*apiv1.DirectoryRoom) map[string]*apiv1.DirectoryRoom {
	result := make(map[string]*apiv1.DirectoryRoom, len(rooms))
	for _, room := range rooms {
		if room == nil || room.GetRoom() == nil {
			continue
		}
		result[room.GetRoom().GetId()] = room
	}
	return result
}

func findDirectoryGroup(groups []*apiv1.RoomGroup, groupID string) *apiv1.RoomGroup {
	for _, group := range groups {
		if group.GetId() == groupID {
			return group
		}
	}
	return nil
}

func roomGroupItemsContainRoom(items []*apiv1.RoomGroupItem, roomID string) bool {
	for _, item := range items {
		room := item.GetRoom()
		if room != nil && room.GetRoom().GetId() == roomID {
			return true
		}
	}
	return false
}

func roomGroupItemsContainSidebarLink(items []*apiv1.RoomGroupItem, linkID string) bool {
	for _, item := range items {
		link := item.GetSidebarLink()
		if link != nil && link.GetId() == linkID {
			return true
		}
	}
	return false
}
