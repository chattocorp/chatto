package connectapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/core/subjects"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAPIRoomThreadingModeChangeValueFailsClosed(t *testing.T) {
	for _, mode := range []corev1.RoomThreadingMode{
		corev1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED,
		corev1.RoomThreadingMode(99),
	} {
		if got := apiRoomThreadingModeChangeValue(mode); got != apiv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED {
			t.Fatalf("apiRoomThreadingModeChangeValue(%v) = %v, want DISABLED", mode, got)
		}
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
		Name:    "connect\nroom",
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
		Name:          "Connect room 💬",
		Description:   "created through ConnectRPC",
		GroupId:       groupID,
		Universal:     true,
		ThreadingMode: apiv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED,
	}))
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room := createResp.Msg.GetRoom()
	if room.GetId() == "" || room.GetKind() != apiv1.RoomKind_ROOM_KIND_CHANNEL || room.GetGroupId() != groupID || !room.GetUniversal() || room.GetThreadingMode() != apiv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED {
		t.Fatalf("created room = %+v", room)
	}

	updateResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:      room.GetId(),
		Name:        stringPtr("Connect / renamed!"),
		Description: stringPtr("updated through ConnectRPC"),
	}))
	if err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if updateResp.Msg.GetRoom().GetName() != "Connect / renamed!" {
		t.Fatalf("UpdateRoom name = %q, want flexible name", updateResp.Msg.GetRoom().GetName())
	}
	encouraged := apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED
	threadingResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId: room.GetId(), ThreadingMode: &encouraged,
	}))
	if err != nil {
		t.Fatalf("UpdateRoom threading mode: %v", err)
	}
	if got := threadingResp.Msg.GetRoom().GetThreadingMode(); got != encouraged {
		t.Fatalf("UpdateRoom threading_mode = %v, want %v", got, encouraged)
	}
	unspecified := apiv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId: room.GetId(), ThreadingMode: &unspecified,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unspecified threading mode update code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId: room.GetId(),
		Name:   stringPtr("Invalid\u2028name"),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid UpdateRoom name code = %v, want invalid argument", connect.CodeOf(err))
	}

	if _, err := env.rooms.CreateRoom(ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:    "Straße",
		GroupId: groupID,
	})); err != nil {
		t.Fatalf("CreateRoom compatibility baseline: %v", err)
	}
	if _, err := env.rooms.CreateRoom(ctx, connect.NewRequest(&apiv1.CreateRoomRequest{
		Name:    "STRASSE",
		GroupId: groupID,
	})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("compatibility-equivalent CreateRoom code = %v, want already exists", connect.CodeOf(err))
	}
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId: room.GetId(),
		Name:   stringPtr("ＳＴＲＡＳＳＥ"),
	})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("compatibility-equivalent UpdateRoom code = %v, want already exists", connect.CodeOf(err))
	}
	slowModeSeconds := uint32(30)
	slowModeResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:          room.GetId(),
		SlowModeSeconds: &slowModeSeconds,
	}))
	if err != nil {
		t.Fatalf("UpdateRoom Slow Mode: %v", err)
	}
	if got := slowModeResp.Msg.GetRoom().GetSlowModeSeconds(); got != slowModeSeconds {
		t.Fatalf("UpdateRoom slow_mode_seconds = %d, want %d", got, slowModeSeconds)
	}
	partialUpdateResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:      room.GetId(),
		Description: stringPtr("description-only patch"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRoom: %v", err)
	}
	if got := partialUpdateResp.Msg.GetRoom(); got.GetName() != "Connect / renamed!" || got.GetDescription() != "description-only patch" {
		t.Fatalf("partial room update = %+v, want preserved name and updated description", got)
	}
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId: room.GetId(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateRoom code = %v, want invalid argument", connect.CodeOf(err))
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

	universalResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:    room.GetId(),
		Universal: boolPtr(false),
	}))
	if err != nil {
		t.Fatalf("UpdateRoom universal: %v", err)
	}
	if universalResp.Msg.GetRoom().GetUniversal() {
		t.Fatalf("UpdateRoom universal = true, want false")
	}
}

func TestRoomServicePinnedMessages(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("connect-pinned-messages")
	messageResponse, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{RoomId: room.Id, Body: "pin me"}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := messageResponse.Msg.GetMessage()
	if _, err := env.rooms.CreatePinnedMessage(ctx, connect.NewRequest(&apiv1.CreatePinnedMessageRequest{RoomId: room.Id, MessageEventId: message.GetId()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreatePinnedMessage without room.manage code = %v", connect.CodeOf(err))
	}
	if err := env.core.GrantRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomManage); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}
	created, err := env.rooms.CreatePinnedMessage(ctx, connect.NewRequest(&apiv1.CreatePinnedMessageRequest{RoomId: room.Id, MessageEventId: message.GetId()}))
	if err != nil {
		t.Fatalf("CreatePinnedMessage: %v", err)
	}
	if got := created.Msg.GetPinnedMessage(); got.GetMessage().GetBody() != "pin me" || got.GetMessage().GetActorId() != env.viewer.Id || !got.GetMessage().GetPinned() {
		t.Fatalf("created pinned message = %+v", got)
	}
	listed, err := env.rooms.ListPinnedMessages(ctx, connect.NewRequest(&apiv1.ListPinnedMessagesRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListPinnedMessages: %v", err)
	}
	latestPinMarker := listed.Msg.GetLatestPinMarker()
	if len(listed.Msg.GetPinnedMessages()) != 1 || listed.Msg.GetPage().GetTotalCount() != 1 || latestPinMarker == "" {
		t.Fatalf("ListPinnedMessages = %+v", listed.Msg)
	}
	batch, err := env.messages.BatchGetMessages(ctx, connect.NewRequest(&apiv1.BatchGetMessagesRequest{
		RoomId: room.Id, EventIds: []string{"missing", message.GetId(), message.GetId()},
	}))
	if err != nil || len(batch.Msg.GetMessages()) != 1 || batch.Msg.GetMessages()[0].GetId() != message.GetId() || !batch.Msg.GetMessages()[0].GetPinned() {
		t.Fatalf("BatchGetMessages pinned state = %+v, %v", batch.Msg, err)
	}
	if _, err := env.rooms.DeletePinnedMessage(ctx, connect.NewRequest(&apiv1.DeletePinnedMessageRequest{RoomId: room.Id, MessageEventId: message.GetId()})); err != nil {
		t.Fatalf("DeletePinnedMessage: %v", err)
	}
	listed, err = env.rooms.ListPinnedMessages(ctx, connect.NewRequest(&apiv1.ListPinnedMessagesRequest{RoomId: room.Id}))
	if err != nil || len(listed.Msg.GetPinnedMessages()) != 0 || listed.Msg.GetLatestPinMarker() != latestPinMarker {
		t.Fatalf("ListPinnedMessages after delete = %+v, %v", listed.Msg, err)
	}
	batch, err = env.messages.BatchGetMessages(ctx, connect.NewRequest(&apiv1.BatchGetMessagesRequest{
		RoomId: room.Id, EventIds: []string{message.GetId()},
	}))
	if err != nil || len(batch.Msg.GetMessages()) != 1 || batch.Msg.GetMessages()[0].GetPinned() {
		t.Fatalf("BatchGetMessages after unpin = %+v, %v", batch.Msg, err)
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
	if _, err := env.rooms.ListBans(env.ctx, connect.NewRequest(&apiv1.ListBansRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListBans code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.rooms.ListBans(ctx, connect.NewRequest(&apiv1.ListBansRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("ListBans without permission code = %v, want permission denied", connect.CodeOf(err))
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

	addTarget, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-add-target", "Room Add Target", "password")
	if err != nil {
		t.Fatalf("CreateUser add target: %v", err)
	}
	if _, err := env.rooms.AddMember(ctx, connect.NewRequest(&apiv1.AddMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("AddMember without room.manage code = %v, want permission denied", connect.CodeOf(err))
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission room.join: %v", err)
	}
	addResp, err := env.rooms.AddMember(ctx, connect.NewRequest(&apiv1.AddMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	}))
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if addResp.Msg.GetMember().GetUser().GetId() != addTarget.Id {
		t.Fatalf("AddMember member = %+v, want target", addResp.Msg.GetMember())
	}
	if _, err := env.rooms.GetMember(ctx, connect.NewRequest(&apiv1.GetRoomMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	})); err != nil {
		t.Fatalf("RoomService.GetMember after AddMember: %v", err)
	}
	removeResp, err := env.rooms.RemoveMember(ctx, connect.NewRequest(&apiv1.RemoveMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	}))
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if !removeResp.Msg.GetRemoved() {
		t.Fatalf("RemoveMember removed = false, want true")
	}
	removeAgainResp, err := env.rooms.RemoveMember(ctx, connect.NewRequest(&apiv1.RemoveMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	}))
	if err != nil {
		t.Fatalf("idempotent RemoveMember: %v", err)
	}
	if removeAgainResp.Msg.GetRemoved() {
		t.Fatalf("idempotent RemoveMember removed = true, want false")
	}
	if _, err := env.rooms.GetMember(ctx, connect.NewRequest(&apiv1.GetRoomMemberRequest{
		RoomId: room.Id,
		UserId: addTarget.Id,
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("RoomService.GetMember after RemoveMember code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.rooms.BanMember(ctx, connect.NewRequest(&apiv1.BanMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "  ",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("blank BanMember reason code = %v, want invalid argument", connect.CodeOf(err))
	}

	banResp, err := env.rooms.BanMember(ctx, connect.NewRequest(&apiv1.BanMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "moderation test",
	}))
	if err != nil {
		t.Fatalf("BanMember: %v", err)
	}
	if !banResp.Msg.GetBanned() {
		t.Fatalf("BanMember banned = false, want true")
	}
	isTargetMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, target.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists target after ban: %v", err)
	}
	if isTargetMember {
		t.Fatalf("target is still a member after BanMember")
	}

	listResp, err := env.rooms.ListBans(ctx, connect.NewRequest(&apiv1.ListBansRequest{}))
	if err != nil {
		t.Fatalf("ListBans: %v", err)
	}
	if got := len(listResp.Msg.GetBans()); got != 1 {
		t.Fatalf("ListBans count = %d, want 1", got)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 1 || listResp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListBans page = %+v, want total_count 1 has_more false", listResp.Msg.GetPage())
	}
	listedBan := listResp.Msg.GetBans()[0]
	if listedBan.GetId() == "" {
		t.Fatalf("ListBans ban id is empty")
	}
	if listedBan.GetRoomId() != room.Id || listedBan.GetRoom().GetName() != room.Name {
		t.Fatalf("ListBans room = %+v, want id %s name %q", listedBan.GetRoom(), room.Id, room.Name)
	}
	if listedBan.GetUserId() != target.Id || listedBan.GetUser().GetUser().GetDisplayName() != target.DisplayName {
		t.Fatalf("ListBans user = %+v, want target %s", listedBan.GetUser(), target.Id)
	}
	if listedBan.GetModeratorId() != env.viewer.Id || listedBan.GetModerator().GetUser().GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("ListBans moderator = %+v, want viewer %s", listedBan.GetModerator(), env.viewer.Id)
	}
	if listedBan.GetReason() != "moderation test" {
		t.Fatalf("ListBans reason = %q, want moderation test", listedBan.GetReason())
	}
	if listedBan.GetCreatedAt() == nil {
		t.Fatalf("ListBans created_at is nil")
	}
	if listedBan.GetExpiresAt() != nil {
		t.Fatalf("ListBans expires_at = %v, want nil", listedBan.GetExpiresAt())
	}

	filteredResp, err := env.rooms.ListBans(ctx, connect.NewRequest(&apiv1.ListBansRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListBans filtered: %v", err)
	}
	if got := len(filteredResp.Msg.GetBans()); got != 1 {
		t.Fatalf("filtered ListBans count = %d, want 1", got)
	}
	if filteredResp.Msg.GetPage().GetTotalCount() != 1 || filteredResp.Msg.GetPage().GetHasMore() {
		t.Fatalf("filtered ListBans page = %+v, want total_count 1 has_more false", filteredResp.Msg.GetPage())
	}

	unbanResp, err := env.rooms.UnbanMember(ctx, connect.NewRequest(&apiv1.UnbanMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "appeal accepted",
	}))
	if err != nil {
		t.Fatalf("UnbanMember: %v", err)
	}
	if !unbanResp.Msg.GetUnbanned() {
		t.Fatalf("UnbanMember unbanned = false, want true")
	}
	afterUnbanResp, err := env.rooms.ListBans(ctx, connect.NewRequest(&apiv1.ListBansRequest{}))
	if err != nil {
		t.Fatalf("ListBans after unban: %v", err)
	}
	if got := len(afterUnbanResp.Msg.GetBans()); got != 0 {
		t.Fatalf("ListBans after unban count = %d, want 0", got)
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
		Name:        stringPtr("dm-renamed"),
		Description: stringPtr("should not change"),
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
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:    dm.Id,
		Universal: boolPtr(true),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateRoom universal for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	slowModeSeconds := uint32(30)
	if _, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:          dm.Id,
		SlowModeSeconds: &slowModeSeconds,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateRoom Slow Mode for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.AddMember(ctx, connect.NewRequest(&apiv1.AddMemberRequest{
		RoomId: dm.Id,
		UserId: outsider.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("AddMember for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.RemoveMember(ctx, connect.NewRequest(&apiv1.RemoveMemberRequest{
		RoomId: dm.Id,
		UserId: participant.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("RemoveMember for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.BanMember(ctx, connect.NewRequest(&apiv1.BanMemberRequest{
		RoomId: dm.Id,
		UserId: participant.Id,
		Reason: "should not ban",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("BanMember for DM code = %v, want invalid argument", connect.CodeOf(err))
	}
	if _, err := env.rooms.UnbanMember(ctx, connect.NewRequest(&apiv1.UnbanMemberRequest{
		RoomId: dm.Id,
		UserId: participant.Id,
		Reason: "should not unban",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UnbanMember for DM code = %v, want invalid argument", connect.CodeOf(err))
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
		t.Fatalf("CreateMessage root: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, participant.Id, "private reply", nil, "", root.Id, nil, false)
	if err != nil {
		t.Fatalf("CreateMessage reply: %v", err)
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

	_, err = env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: dm.Id,
		Body:   "not a participant",
	}))
	checkInaccessible("CreateMessage", err)

	_, err = env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: dm.Id,
	}))
	checkInaccessible("GetRoomEvents", err)

	_, err = env.rooms.GetRoomEventsAround(ctx, connect.NewRequest(&apiv1.GetRoomEventsAroundRequest{
		RoomId:  dm.Id,
		EventId: root.Id,
	}))
	checkInaccessible("GetRoomEventsAround", err)

	_, err = env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
	}))
	checkInaccessible("GetThreadEvents", err)

	_, err = env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            dm.Id,
		ThreadRootEventId: root.Id,
		EventId:           reply.Id,
	}))
	checkInaccessible("GetThreadEventsAround", err)

	_, err = env.rooms.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: dm.Id,
		Page:   &apiv1.PageRequest{Limit: 10},
	}))
	checkInaccessible("ListRoomAttachments", err)

	_, err = env.assets.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{
		RoomId:  dm.Id,
		AssetId: "asset",
	}))
	checkInaccessible("GetAsset", err)

	_, err = env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  dm.Id,
		EventId: root.Id,
	}))
	checkInaccessible("GetMessage", err)

	_, err = env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         dm.Id,
		MessageEventId: root.Id,
		Emoji:          "thumbsup",
	}))
	checkInaccessible("AddReaction", err)

	_, err = env.messages.RemoveReaction(ctx, connect.NewRequest(&apiv1.RemoveReactionRequest{
		RoomId:         dm.Id,
		MessageEventId: root.Id,
		Emoji:          "thumbsup",
	}))
	checkInaccessible("RemoveReaction", err)

	_, err = env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      dm.Id,
		UpToEventId: root.Id,
	}))
	checkInaccessible("MarkRoomAsRead", err)

	_, err = env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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

	roomID := dm.Id
	_, err = env.notifications.GetNotificationPolicy(ctx, connect.NewRequest(&apiv1.GetNotificationPolicyRequest{
		RoomId: &roomID,
	}))
	checkInaccessible("GetNotificationPolicy", err)

	_, err = env.notifications.UpdateNotificationPolicy(ctx, connect.NewRequest(&apiv1.UpdateNotificationPolicyRequest{
		RoomId: &roomID,
		Overrides: &apiv1.NotificationDeliveryModes{
			DirectMessages: apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum(),
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"direct_messages"}},
	}))
	checkInaccessible("UpdateNotificationPolicy", err)
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
		t.Fatalf("CreateMessage DM: %v", err)
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
		t.Fatalf("DM IsMember = false, want true")
	}
	if !apiRoomPermissionGranted(dmRoom, core.PermRoomList) {
		t.Fatalf("DM CanListRoom = false, want true")
	}
	if apiRoomPermissionGranted(dmRoom, core.PermRoomJoin) ||
		apiRoomPermissionGranted(dmRoom, core.PermRoomManage) ||
		apiRoomPermissionGranted(dmRoom, core.PermRoomMemberBan) ||
		apiRoomPermissionGranted(dmRoom, core.PermMessagePostInThread) {
		t.Fatalf("DM exposes channel-only actions: %+v", dmRoom)
	}
	batchResp, err := env.directory.BatchGetRooms(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.BatchGetRoomsRequest{
		RoomIds: []string{visible.Id, hidden.Id, dm.Id, visible.Id, "missing-room"},
	}))
	if err != nil {
		t.Fatalf("BatchGetRooms: %v", err)
	}
	if got := batchResp.Msg.GetRooms(); len(got) != 2 || got[0].GetRoom().GetId() != visible.Id || got[1].GetRoom().GetId() != dm.Id {
		t.Fatalf("BatchGetRooms rooms = %+v, want visible,dm", got)
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
	outsiderBatchResp, err := env.directory.BatchGetRooms(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.BatchGetRoomsRequest{RoomIds: []string{dm.Id}}))
	if err != nil {
		t.Fatalf("outsider BatchGetRooms DM: %v", err)
	}
	if len(outsiderBatchResp.Msg.GetRooms()) != 0 {
		t.Fatalf("outsider BatchGetRooms DM len = %d, want 0", len(outsiderBatchResp.Msg.GetRooms()))
	}

	if err := env.core.AssignOwnerRole(env.ctx, caller.Id); err != nil {
		t.Fatalf("AssignOwnerRole caller: %v", err)
	}
	ownerResp, err := env.directory.GetRoom(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.GetRoomRequest{RoomId: dm.Id}))
	if err != nil {
		t.Fatalf("owner GetRoom DM: %v", err)
	}
	if apiRoomPermissionGranted(ownerResp.Msg.GetRoom(), core.PermMessagePostInThread) {
		t.Fatal("owner DM viewer state grants message.post-in-thread")
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
	if visibleRoom.GetViewerState().GetIsMember() {
		t.Fatalf("visible room IsMember = true, want false")
	}
	if !apiRoomPermissionGranted(visibleRoom, core.PermRoomJoin) {
		t.Fatalf("visible non-member CanJoinRoom = false, want true")
	}
	if apiRoomPermissionGranted(visibleRoom, core.PermMessagePost) ||
		apiRoomPermissionGranted(visibleRoom, core.PermMessagePostInThread) ||
		apiRoomPermissionGranted(visibleRoom, core.PermMessageAttach) ||
		apiRoomPermissionGranted(visibleRoom, core.PermMessageReact) ||
		apiRoomPermissionGranted(visibleRoom, core.PermMessageEcho) ||
		apiRoomPermissionGranted(visibleRoom, core.PermMessageManage) {
		t.Fatalf("visible non-member exposes member-only actions: %+v", visibleRoom)
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
	archivedBatchResp, err := env.directory.BatchGetRooms(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.BatchGetRoomsRequest{
		RoomIds: []string{memberArchived.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetRooms archived: %v", err)
	}
	if got := archivedBatchResp.Msg.GetRooms(); len(got) != 1 || got[0].GetRoom().GetId() != memberArchived.Id || !got[0].GetRoom().GetArchived() {
		t.Fatalf("BatchGetRooms archived rooms = %+v, want archived member room", got)
	}
	archivedRoom := archivedResp.Msg.GetRoom()
	if !archivedRoom.GetViewerState().GetIsMember() {
		t.Fatalf("archived room IsMember = false, want true")
	}
	if apiRoomPermissionGranted(archivedRoom, core.PermRoomJoin) ||
		apiRoomPermissionGranted(archivedRoom, core.PermMessagePost) ||
		apiRoomPermissionGranted(archivedRoom, core.PermMessagePostInThread) ||
		apiRoomPermissionGranted(archivedRoom, core.PermMessageAttach) ||
		apiRoomPermissionGranted(archivedRoom, core.PermMessageReact) ||
		apiRoomPermissionGranted(archivedRoom, core.PermMessageEcho) {
		t.Fatalf("archived room exposes unavailable actions: %+v", archivedRoom)
	}
	if !apiRoomPermissionGranted(archivedRoom, core.PermMessageManage) {
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
	if apiRoomGroupPermissionGranted(group, core.PermRoomCreate) {
		t.Fatalf("group CanCreateRoom = true before group grant, want false")
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
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission room.manage: %v", err)
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, env.viewer.Id, core.PermRoomCreate); err != nil {
		t.Fatalf("GrantUserGroupPermission admin room.create: %v", err)
	}
	adminLayoutResp, err := env.adminLayout.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListRoomGroupsRequest{}))
	if err != nil {
		t.Fatalf("AdminRoomLayout ListRoomGroups: %v", err)
	}
	adminLayoutGroup := findAdminRoomLayoutGroup(adminLayoutResp.Msg.GetGroups(), groupID)
	if adminLayoutGroup == nil {
		t.Fatalf("group %s missing from admin layout response", groupID)
	}
	if !adminRoomLayoutItemsContainRoom(adminLayoutGroup.GetItems(), archived.Id) {
		t.Fatalf("archived room %s missing from admin layout group items", archived.Id)
	}
	if !adminLayoutGroup.GetCanCreateRoom() {
		t.Fatalf("admin layout group CanCreateRoom = false after group grant, want true")
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, caller.Id, core.PermRoomCreate); err != nil {
		t.Fatalf("GrantUserGroupPermission room.create: %v", err)
	}

	getResp, err := env.directory.GetRoomGroup(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.GetRoomGroupRequest{
		GroupId: groupID,
	}))
	if err != nil {
		t.Fatalf("GetRoomGroup: %v", err)
	}
	getGroup := getResp.Msg.GetGroup()
	if getGroup.GetId() != groupID {
		t.Fatalf("GetRoomGroup id = %q, want %q", getGroup.GetId(), groupID)
	}
	if !apiRoomGroupPermissionGranted(getGroup, core.PermRoomCreate) {
		t.Fatalf("group CanCreateRoom = false after group grant, want true")
	}
	if !roomGroupItemsContainRoom(getGroup.GetItems(), visible.Id) ||
		roomGroupItemsContainRoom(getGroup.GetItems(), hidden.Id) ||
		roomGroupItemsContainRoom(getGroup.GetItems(), archived.Id) ||
		!roomGroupItemsContainSidebarLink(getGroup.GetItems(), link.Id) {
		t.Fatalf("GetRoomGroup items = %+v, want visible room and link only", getGroup.GetItems())
	}
	if _, err := env.directory.GetRoomGroup(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.GetRoomGroupRequest{
		GroupId: "missing-group",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetRoomGroup code = %v, want not_found", connect.CodeOf(err))
	}

	batchResp, err := env.directory.BatchGetRoomGroups(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.BatchGetRoomGroupsRequest{
		GroupIds: []string{groupID, "missing-group", groupID},
	}))
	if err != nil {
		t.Fatalf("BatchGetRoomGroups: %v", err)
	}
	if got := batchResp.Msg.GetGroups(); len(got) != 1 || got[0].GetId() != groupID {
		t.Fatalf("BatchGetRoomGroups groups = %+v, want single %s group", got, groupID)
	}
}

func TestRoomServiceJoinRoomGroup(t *testing.T) {
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

	resp, err := env.rooms.JoinRoomGroup(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.JoinRoomGroupRequest{
		GroupId: group.Id,
	}))
	if err != nil {
		t.Fatalf("JoinRoomGroup: %v", err)
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

func TestRoomServiceJoinRoomKeepsNormalPostingPermissions(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "join-posts", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "join-posts-caller", "Join Posts Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	callerCtx := withCaller(env.ctx, caller)

	if _, err := env.rooms.JoinRoom(callerCtx, connect.NewRequest(&apiv1.JoinRoomRequest{
		RoomId: room.Id,
	})); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	getResp, err := env.directory.GetRoom(callerCtx, connect.NewRequest(&apiv1.GetRoomRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if !apiRoomPermissionGranted(getResp.Msg.GetRoom(), core.PermMessagePost) {
		t.Fatalf("joined room message.post = false, room = %+v", getResp.Msg.GetRoom())
	}

	createResp, err := env.messages.CreateMessage(callerCtx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "hello after joining",
	}))
	if err != nil {
		t.Fatalf("CreateMessage after join: %v", err)
	}
	if createResp.Msg.GetMessage().GetBody() != "hello after joining" {
		t.Fatalf("CreateMessage body = %q, want posted body", createResp.Msg.GetMessage().GetBody())
	}
}

func TestUserServiceListUsers(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.users.ListUsers(env.ctx, connect.NewRequest(&apiv1.ListUsersRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListUsers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	if _, err := env.users.GetUser(env.ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetUser code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	if _, err := env.users.BatchGetUsers(env.ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetUsers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	alice, err := env.core.CreateUser(env.ctx, core.SystemActorID, "member-alice", "Alice Member", "password")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := env.core.CreateUser(env.ctx, core.SystemActorID, "member-bob", "Bob Member", "password")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, bob.Id); err != nil {
		t.Fatalf("AssignAdminRole bob: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, alice.Id, core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence alice: %v", err)
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, alice.Id, "member-alice@example.test"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect alice: %v", err)
	}

	resp, err := env.users.ListUsers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListUsersRequest{
		Search: "member",
		Page:   &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 2 || !resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetUsers()) != 1 {
		t.Fatalf("first page = %+v, want total 2, hasMore true, one user", resp.Msg)
	}

	secondResp, err := env.users.ListUsers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListUsersRequest{
		Search: "member",
		Page:   &apiv1.PageRequest{Limit: 1, Offset: 1},
	}))
	if err != nil {
		t.Fatalf("ListUsers second page: %v", err)
	}
	if secondResp.Msg.GetPage().GetHasMore() || len(secondResp.Msg.GetUsers()) != 1 {
		t.Fatalf("second page = %+v, want hasMore false and one user", secondResp.Msg)
	}

	gotByID := map[string]*apiv1.DirectoryMember{}
	for _, user := range append(resp.Msg.GetUsers(), secondResp.Msg.GetUsers()...) {
		gotByID[user.GetUser().GetId()] = user
	}
	if gotByID[alice.Id].GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("alice presence = %v, want AWAY", gotByID[alice.Id].GetUser().GetPresenceStatus())
	}
	if gotByID[bob.Id].GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("bob presence = %v, want OFFLINE", gotByID[bob.Id].GetUser().GetPresenceStatus())
	}
	if roles := strings.Join(gotByID[bob.Id].GetRoles(), ","); roles != "everyone,admin" {
		t.Fatalf("bob roles = %q, want everyone,admin", roles)
	}

	getResp, err := env.users.GetUser(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: alice.Id}}))
	if err != nil {
		t.Fatalf("GetUser alice: %v", err)
	}
	gotAlice := getResp.Msg.GetUser()
	if gotAlice.GetUser().GetId() != alice.Id || gotAlice.GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("GetUser alice = %+v, want hydrated away user", gotAlice)
	}
	if gotAlice.ProtoReflect().Descriptor().Fields().ByName("verified_emails") != nil {
		t.Fatal("DirectoryMember unexpectedly exposes verified_emails")
	}
	if _, err := env.users.GetUser(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: "missing-user"}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser code = %v, want not_found", connect.CodeOf(err))
	}

	batchResp, err := env.users.BatchGetUsers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.BatchGetUsersRequest{
		UserIds: []string{bob.Id, "missing-user", alice.Id, bob.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	gotBatch := batchResp.Msg.GetUsers()
	if len(gotBatch) != 2 || gotBatch[0].GetUser().GetId() != bob.Id || gotBatch[1].GetUser().GetId() != alice.Id {
		t.Fatalf("BatchGetUsers users = %+v, want bob,alice", gotBatch)
	}
}

func TestRoomServiceMemberReadAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("room-members-room")
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-member-alice", "Room Alice", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom member: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, member.Id, core.PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence member: %v", err)
	}

	req := connect.NewRequest(&apiv1.ListRoomMembersRequest{RoomId: room.Id, Search: "alice", Page: &apiv1.PageRequest{Limit: 10}})
	if _, err := env.rooms.ListMembers(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListMembers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	if _, err := env.rooms.GetMember(env.ctx, connect.NewRequest(&apiv1.GetRoomMemberRequest{RoomId: room.Id, UserId: member.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetMember code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	if _, err := env.rooms.BatchGetMembers(env.ctx, connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{RoomId: room.Id, UserIds: []string{member.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetMembers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-member-outsider", "Room Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.ListMembers(withCaller(env.ctx, outsider), req); err != nil {
		t.Fatalf("joinable outsider ListMembers: %v", err)
	}
	if _, err := env.rooms.GetMember(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.GetRoomMemberRequest{RoomId: room.Id, UserId: member.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider GetMember code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if _, err := env.rooms.BatchGetMembers(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{RoomId: room.Id, UserIds: []string{member.Id}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider BatchGetMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission room.join: %v", err)
	}
	if _, err := env.rooms.ListMembers(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("join-denied outsider ListMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if err := env.core.ClearRoomPermissionState(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("ClearRoomPermissionState room.join: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission room.list: %v", err)
	}
	if _, err := env.rooms.ListMembers(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("list-denied outsider ListMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	manager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-member-manager", "Room Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, manager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission manager room.manage: %v", err)
	}
	if err := env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, manager.Id, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyUserRoomPermission manager room.join: %v", err)
	}
	managerCtx := withCaller(env.ctx, manager)
	if _, err := env.rooms.ListMembers(managerCtx, req); err != nil {
		t.Fatalf("manager ListMembers: %v", err)
	}
	if _, err := env.rooms.GetMember(managerCtx, connect.NewRequest(&apiv1.GetRoomMemberRequest{
		RoomId: room.Id,
		UserId: member.Id,
	})); err != nil {
		t.Fatalf("manager GetMember: %v", err)
	}
	if _, err := env.rooms.BatchGetMembers(managerCtx, connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{
		RoomId:  room.Id,
		UserIds: []string{member.Id},
	})); err != nil {
		t.Fatalf("manager BatchGetMembers: %v", err)
	}

	resp, err := env.rooms.ListMembers(withCaller(env.ctx, env.viewer), req)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 1 || resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetMembers()) != 1 {
		t.Fatalf("room member page = %+v, want one alice result", resp.Msg)
	}
	got := resp.Msg.GetMembers()[0]
	if got.GetUser().GetId() != member.Id || got.GetUser().GetDisplayName() != "Room Alice" || got.GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("room member = %+v, want hydrated Room Alice", got)
	}

	getResp, err := env.rooms.GetMember(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomMemberRequest{RoomId: room.Id, UserId: member.Id}))
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if got := getResp.Msg.GetMember(); got.GetUser().GetId() != member.Id || got.GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("GetMember member = %+v, want room member", got)
	}
	if _, err := env.rooms.GetMember(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomMemberRequest{RoomId: room.Id, UserId: outsider.Id})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("non-member GetMember code = %v, want not_found", connect.CodeOf(err))
	}

	batchResp, err := env.rooms.BatchGetMembers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{
		RoomId:  room.Id,
		UserIds: []string{member.Id, outsider.Id, env.viewer.Id, member.Id, "missing-user"},
	}))
	if err != nil {
		t.Fatalf("BatchGetMembers: %v", err)
	}
	gotBatch := batchResp.Msg.GetMembers()
	if len(gotBatch) != 2 || gotBatch[0].GetUser().GetId() != member.Id || gotBatch[1].GetUser().GetId() != env.viewer.Id {
		t.Fatalf("BatchGetMembers members = %+v, want member,viewer", gotBatch)
	}
}

func TestRoomServiceListMembersReturnsStablePreviewPageToJoinableNonmember(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "member-preview-page", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	caller, err := env.core.CreateUser(env.ctx, core.SystemActorID, "member-preview-caller", "Preview Caller", "password")
	if err != nil {
		t.Fatalf("CreateUser caller: %v", err)
	}
	members := []struct {
		login       string
		displayName string
	}{
		{login: "preview-zulu", displayName: "Zulu"},
		{login: "preview-alice", displayName: "alice"},
		{login: "preview-bob", displayName: "Bob"},
		{login: "preview-charlie", displayName: "charlie"},
		{login: "preview-delta", displayName: "Delta"},
		{login: "preview-echo", displayName: "echo"},
	}
	for _, input := range members {
		member, err := env.core.CreateUser(env.ctx, core.SystemActorID, input.login, input.displayName, "password")
		if err != nil {
			t.Fatalf("CreateUser %s: %v", input.login, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", input.login, err)
		}
	}

	resp, err := env.rooms.ListMembers(withCaller(env.ctx, caller), connect.NewRequest(&apiv1.ListRoomMembersRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 5},
	}))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if page := resp.Msg.GetPage(); page.GetTotalCount() != 6 || !page.GetHasMore() {
		t.Fatalf("ListMembers page = %+v, want total 6 and has_more", page)
	}
	gotNames := make([]string, len(resp.Msg.GetMembers()))
	for i, member := range resp.Msg.GetMembers() {
		gotNames[i] = member.GetUser().GetDisplayName()
	}
	if got, want := strings.Join(gotNames, ","), "alice,Bob,charlie,Delta,echo"; got != want {
		t.Fatalf("ListMembers names = %q, want %q", got, want)
	}
}

func TestMemberDirectoryPaginationDefaultsAndClamps(t *testing.T) {
	userLimit, userOffset := userDirectoryPagination(nil)
	if userLimit != 20 || userOffset != 0 {
		t.Fatalf("userDirectoryPagination nil page = %d, %d; want 20, 0", userLimit, userOffset)
	}

	roomLimit, roomOffset := roomMemberDirectoryPagination(nil)
	if roomLimit != 250 || roomOffset != 0 {
		t.Fatalf("roomMemberDirectoryPagination nil page = %d, %d; want 250, 0", roomLimit, roomOffset)
	}

	roomZeroLimit, roomZeroOffset := roomMemberDirectoryPagination(&apiv1.PageRequest{Limit: 0, Offset: 7})
	if roomZeroLimit != 250 || roomZeroOffset != 7 {
		t.Fatalf("roomMemberDirectoryPagination zero-limit page = %d, %d; want 250, 7", roomZeroLimit, roomZeroOffset)
	}

	limit, offset := roomMemberDirectoryPagination(&apiv1.PageRequest{Limit: 9999})
	if limit != 500 || offset != 0 {
		t.Fatalf("roomMemberDirectoryPagination oversized page = %d, %d; want 500, 0", limit, offset)
	}

	users := make([]*corev1.User, 501)
	for i := range users {
		users[i] = &corev1.User{Id: fmt.Sprintf("user-%03d", i)}
	}
	page, totalCount, hasMore := paginateDirectoryUsers(users, limit, offset)
	if len(page) != 500 || totalCount != 501 || !hasMore {
		t.Fatalf("paginated users len, total, hasMore = %d, %d, %v; want 500, 501, true", len(page), totalCount, hasMore)
	}
}

func TestMyAccountServiceSetAndDeleteCustomStatus(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	expiresAt := timestamppb.New(time.Now().Add(time.Hour).UTC())

	setResp, err := env.account.UpdateCustomStatus(ctx, connect.NewRequest(&apiv1.UpdateCustomStatusRequest{
		Emoji:     "🌿",
		Text:      "In focus mode",
		ExpiresAt: expiresAt,
	}))
	if err != nil {
		t.Fatalf("UpdateCustomStatus: %v", err)
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

	_, err = env.account.UpdateCustomStatus(ctx, connect.NewRequest(&apiv1.UpdateCustomStatusRequest{
		Emoji: "🌿",
		Text:  "   ",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateCustomStatus blank text error = %v, want InvalidArgument", err)
	}

	_, err = env.account.UpdateCustomStatus(ctx, connect.NewRequest(&apiv1.UpdateCustomStatusRequest{
		Emoji: "e",
		Text:  "Invalid emoji",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateCustomStatus invalid emoji error = %v, want InvalidArgument", err)
	}

	clearResp, err := env.account.DeleteCustomStatus(ctx, connect.NewRequest(&apiv1.DeleteCustomStatusRequest{}))
	if err != nil {
		t.Fatalf("DeleteCustomStatus: %v", err)
	}
	if clearResp.Msg.GetStatus() != nil {
		t.Fatalf("cleared status = %+v, want nil", clearResp.Msg.GetStatus())
	}
}

func TestNotificationServiceOccurrenceLifecycle(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-v2-actor", "Notification 2 Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, "hello from v2", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	list, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil {
		t.Fatalf("ListNotificationOccurrences: %v", err)
	}
	if len(list.Msg.GetOccurrences()) != 1 || list.Msg.GetUnreadCount() != 1 || list.Msg.GetImportantUnreadCount() != 1 {
		t.Fatalf("occurrences = %+v, unread = %d, want one unread occurrence", list.Msg.GetOccurrences(), list.Msg.GetUnreadCount())
	}
	if counts := list.Msg.GetRoomUnreadCounts(); len(counts) != 1 || counts[0].GetRoomId() != dm.Id || counts[0].GetUnreadCount() != 1 || counts[0].GetImportantUnreadCount() != 1 {
		t.Fatalf("room unread-occurrence counts = %+v, want one group for %s", counts, dm.Id)
	}
	occurrence := list.Msg.GetOccurrences()[0]
	if occurrence.GetSignal().GetDirectMessageReceived().GetMessage().GetRoom().GetId() != dm.Id || occurrence.GetSignal().GetDirectMessageReceived().GetMessage().GetEventId() != posted.Id ||
		!occurrence.GetUnread() || occurrence.GetAttentionLevel() != apiv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT {
		t.Fatalf("occurrence = %+v, want exact unread DM target", occurrence)
	}
	got, err := env.notifications.GetNotificationOccurrence(ctx, connect.NewRequest(&apiv1.GetNotificationOccurrenceRequest{
		NotificationId: occurrence.GetId(),
	}))
	if err != nil || got.Msg.GetOccurrence().GetId() != occurrence.GetId() {
		t.Fatalf("GetNotificationOccurrence = (%+v, %v), want %s", got, err, occurrence.GetId())
	}
	if _, err := env.notifications.GetNotificationOccurrence(ctx, connect.NewRequest(&apiv1.GetNotificationOccurrenceRequest{
		NotificationId: "missing-notification",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetNotificationOccurrence missing code = %v, want not found", connect.CodeOf(err))
	}
	batchGet, err := env.notifications.BatchGetNotificationOccurrences(ctx, connect.NewRequest(&apiv1.BatchGetNotificationOccurrencesRequest{
		NotificationIds: []string{"missing-notification", occurrence.GetId(), occurrence.GetId()},
	}))
	if err != nil || len(batchGet.Msg.GetOccurrences()) != 1 || batchGet.Msg.GetOccurrences()[0].GetId() != occurrence.GetId() {
		t.Fatalf("BatchGetNotificationOccurrences = (%+v, %v), want one de-duplicated occurrence", batchGet, err)
	}

	if _, err := env.notifications.MarkNotificationRead(ctx, connect.NewRequest(&apiv1.MarkNotificationReadRequest{
		NotificationId: occurrence.GetId(),
	})); err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	readList, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(readList.Msg.GetOccurrences()) != 1 || readList.Msg.GetOccurrences()[0].GetUnread() || readList.Msg.GetUnreadCount() != 0 {
		t.Fatalf("ListNotificationOccurrences after read = %+v, %v, want one read occurrence", readList, err)
	}

	deleted, err := env.notifications.BatchDeleteNotificationOccurrences(ctx, connect.NewRequest(&apiv1.BatchDeleteNotificationOccurrencesRequest{
		NotificationIds: []string{occurrence.GetId(), occurrence.GetId()},
	}))
	if err != nil || deleted.Msg.GetDeletedCount() != 1 {
		t.Fatalf("BatchDeleteNotificationOccurrences: %v", err)
	}
	afterDelete, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(afterDelete.Msg.GetOccurrences()) != 0 {
		t.Fatalf("list after delete = %+v, %v, want empty", afterDelete, err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, "dismiss all of this", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage before DeleteAllNotificationOccurrences: %v", err)
	}
	deleteAll, err := env.notifications.DeleteAllNotificationOccurrences(ctx, connect.NewRequest(&apiv1.DeleteAllNotificationOccurrencesRequest{}))
	if err != nil || deleteAll.Msg.GetDeletedCount() != 1 {
		t.Fatalf("DeleteAllNotificationOccurrences = (%+v, %v), want one", deleteAll, err)
	}
	afterDeleteAll, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(afterDeleteAll.Msg.GetOccurrences()) != 0 {
		t.Fatalf("list after delete all = %+v, %v, want empty", afterDeleteAll, err)
	}

	policy, err := env.notifications.UpdateNotificationPolicy(ctx, connect.NewRequest(&apiv1.UpdateNotificationPolicyRequest{
		Overrides: &apiv1.NotificationDeliveryModes{
			DirectMentions: apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT.Enum(),
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"direct_mentions"}},
	}))
	if err != nil {
		t.Fatalf("UpdateNotificationPolicy: %v", err)
	}
	if policy.Msg.GetPolicy().GetOverrides().GetDirectMentions() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT ||
		policy.Msg.GetPolicy().GetEffective().GetDirectMentions() != apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT {
		t.Fatalf("notification policy = %+v, want direct mention Silent", policy.Msg.GetPolicy())
	}
}

func TestNotificationServiceDeleteOccurrenceIsIdempotent(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-delete-actor", "Notification Delete Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, "delete exactly this", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	inbox, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(inbox.Msg.GetOccurrences()) != 1 {
		t.Fatalf("ListNotificationOccurrences = (%+v, %v), want one occurrence", inbox, err)
	}
	notificationID := inbox.Msg.GetOccurrences()[0].GetId()
	first, err := env.notifications.DeleteNotificationOccurrence(ctx, connect.NewRequest(&apiv1.DeleteNotificationOccurrenceRequest{
		NotificationId: notificationID,
	}))
	if err != nil || !first.Msg.GetDeleted() {
		t.Fatalf("first DeleteNotificationOccurrence = (%+v, %v), want deleted", first, err)
	}
	second, err := env.notifications.DeleteNotificationOccurrence(ctx, connect.NewRequest(&apiv1.DeleteNotificationOccurrenceRequest{
		NotificationId: notificationID,
	}))
	if err != nil || second.Msg.GetDeleted() {
		t.Fatalf("second DeleteNotificationOccurrence = (%+v, %v), want idempotent deleted=false", second, err)
	}
	after, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(after.Msg.GetOccurrences()) != 0 {
		t.Fatalf("ListNotificationOccurrences after delete = (%+v, %v), want empty", after, err)
	}
}

func TestNotificationServiceRejectsRetractedTargetsBeforeCleanup(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-stale-actor", "Notification Stale Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, "soon retracted", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	sequence, err := env.core.GetEventSequence(env.ctx, core.KindDM, dm.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	if err := env.core.DeleteMessage(env.ctx, actor.Id, core.KindDM, dm.Id, posted.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	createStale := func(sourceID string) *corev1.NotificationOccurrence {
		t.Helper()
		occurrence, created, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
			RecipientID:          env.viewer.Id,
			SourceEventID:        sourceID,
			SourceCreated:        posted.GetCreatedAt().AsTime(),
			SourceStreamSequence: sequence,
			ActorID:              actor.Id,
			Signal:               testNotificationSignal(notificationTestSignalDirectMention, dm.Id, posted.Id),
			Mode:                 corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT,
			AttentionLevel:       corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SkipReadLookup:       true,
		})
		if err != nil || !created {
			t.Fatalf("Create stale occurrence = (%v, %v, %v), want created", occurrence, created, err)
		}
		return occurrence
	}
	createVisible := func(body string) *corev1.NotificationOccurrence {
		t.Helper()
		message, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, body, nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage visible occurrence: %v", err)
		}
		if err := env.core.NotificationOccurrences().WaitCurrent(env.ctx); err != nil {
			t.Fatalf("WaitCurrent visible occurrence: %v", err)
		}
		occurrences, err := env.core.NotificationOccurrences().List(env.ctx, env.viewer.Id)
		if err != nil {
			t.Fatalf("List visible occurrences: %v", err)
		}
		for _, occurrence := range occurrences {
			if occurrence.GetSourceEventId() == message.GetId() {
				return occurrence
			}
		}
		t.Fatalf("visible occurrence for source %s was not materialized", message.GetId())
		return nil
	}

	createStale("stale-list-" + posted.Id)
	inbox, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(inbox.Msg.GetOccurrences()) != 0 {
		t.Fatalf("ListNotificationOccurrences with retracted target = (%+v, %v), want empty", inbox, err)
	}
	staleGet := createStale("stale-get-" + posted.Id)
	if _, err := env.notifications.GetNotificationOccurrence(ctx, connect.NewRequest(&apiv1.GetNotificationOccurrenceRequest{
		NotificationId: staleGet.GetId(),
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetNotificationOccurrence retracted target code = %v, want not found", connect.CodeOf(err))
	}
	staleBatchGet := createStale("stale-batch-get-" + posted.Id)
	visibleBatchGet := createVisible("visible batch-get target")
	batchGet, err := env.notifications.BatchGetNotificationOccurrences(ctx, connect.NewRequest(&apiv1.BatchGetNotificationOccurrencesRequest{
		NotificationIds: []string{staleBatchGet.GetId(), visibleBatchGet.GetId(), visibleBatchGet.GetId(), "missing-notification"},
	}))
	if err != nil || len(batchGet.Msg.GetOccurrences()) != 1 || batchGet.Msg.GetOccurrences()[0].GetId() != visibleBatchGet.GetId() {
		t.Fatalf("BatchGetNotificationOccurrences mixed visibility = (%+v, %v), want one visible occurrence", batchGet, err)
	}
	if deleted, err := env.notifications.DeleteNotificationOccurrence(ctx, connect.NewRequest(&apiv1.DeleteNotificationOccurrenceRequest{
		NotificationId: visibleBatchGet.GetId(),
	})); err != nil || !deleted.Msg.GetDeleted() {
		t.Fatalf("DeleteNotificationOccurrence batch-get cleanup = (%+v, %v), want deleted", deleted, err)
	}

	staleUpdate := createStale("stale-update-" + posted.Id)
	_, err = env.notifications.MarkNotificationRead(ctx, connect.NewRequest(&apiv1.MarkNotificationReadRequest{
		NotificationId: staleUpdate.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkNotificationRead retracted target code = %v, want not found", connect.CodeOf(err))
	}

	staleDelete := createStale("stale-delete-" + posted.Id)
	deleted, err := env.notifications.DeleteNotificationOccurrence(ctx, connect.NewRequest(&apiv1.DeleteNotificationOccurrenceRequest{
		NotificationId: staleDelete.GetId(),
	}))
	if err != nil || deleted.Msg.GetDeleted() {
		t.Fatalf("DeleteNotificationOccurrence retracted target = (%+v, %v), want deleted=false", deleted, err)
	}
	if _, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, staleDelete.GetId()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("stale single-delete occurrence Get error = %v, want visibility tombstone", err)
	}

	staleBatch := createStale("stale-batch-" + posted.Id)
	visibleBatch := createVisible("visible batch target")
	batch, err := env.notifications.BatchDeleteNotificationOccurrences(ctx, connect.NewRequest(&apiv1.BatchDeleteNotificationOccurrencesRequest{
		NotificationIds: []string{staleBatch.GetId(), visibleBatch.GetId()},
	}))
	if err != nil || batch.Msg.GetDeletedCount() != 1 {
		t.Fatalf("BatchDeleteNotificationOccurrences mixed visibility = (%+v, %v), want one visible deletion", batch, err)
	}
	for _, occurrence := range []*corev1.NotificationOccurrence{staleBatch, visibleBatch} {
		if _, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, occurrence.GetId()); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("batch-deleted occurrence %s Get error = %v, want not found", occurrence.GetId(), err)
		}
	}

	staleAll := createStale("stale-all-" + posted.Id)
	visibleAll := createVisible("visible delete-all target")
	all, err := env.notifications.DeleteAllNotificationOccurrences(ctx, connect.NewRequest(&apiv1.DeleteAllNotificationOccurrencesRequest{}))
	if err != nil || all.Msg.GetDeletedCount() != 1 {
		t.Fatalf("DeleteAllNotificationOccurrences mixed visibility = (%+v, %v), want one visible deletion", all, err)
	}
	for _, occurrence := range []*corev1.NotificationOccurrence{staleAll, visibleAll} {
		if _, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, occurrence.GetId()); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("delete-all occurrence %s Get error = %v, want not found", occurrence.GetId(), err)
		}
	}
}

func TestNotificationServiceDeleteRejectsOccurrenceAfterAccessLoss(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-access-loss-actor", "Notification Access Loss Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, actor.Id, core.KindChannel, "", "notification-access-loss-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, actor.Id, core.KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, actor.Id, "access loss target", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	sequence, err := env.core.GetEventSequence(env.ctx, core.KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	if err := env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}
	input := core.CreateNotificationOccurrenceInput{
		RecipientID:          env.viewer.Id,
		SourceEventID:        "notification-access-loss-source",
		SourceCreated:        posted.GetCreatedAt().AsTime(),
		SourceStreamSequence: sequence,
		ActorID:              actor.Id,
		Signal:               testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
		Mode:                 corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT,
		AttentionLevel:       corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup:       true,
	}
	occurrence, created, err := env.core.NotificationOccurrences().Create(env.ctx, input)
	if err != nil || !created {
		t.Fatalf("Create stale occurrence = (%+v, %v, %v), want created", occurrence, created, err)
	}

	deleted, err := env.notifications.DeleteNotificationOccurrence(ctx, connect.NewRequest(&apiv1.DeleteNotificationOccurrenceRequest{
		NotificationId: occurrence.GetId(),
	}))
	if err != nil || deleted.Msg.GetDeleted() {
		t.Fatalf("DeleteNotificationOccurrence after access loss = (%+v, %v), want deleted=false", deleted, err)
	}
	if recreated, created, err := env.core.NotificationOccurrences().Create(env.ctx, input); err != nil || created || recreated != nil {
		t.Fatalf("Create after visibility tombstone = (%+v, %v, %v), want nil, false, nil", recreated, created, err)
	}
}

func TestNotificationServiceVisibilityFilteringFillsOffsetPages(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-page-filter-actor", "Notification Page Filter Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	baseTime := time.Now().UTC().Add(-time.Minute)
	created := make([]*corev1.NotificationOccurrence, 0, 3)
	for index := 0; index < 3; index++ {
		room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", fmt.Sprintf("notification-page-filter-%d", index), "")
		if err != nil {
			t.Fatalf("CreateRoom %d: %v", index, err)
		}
		for _, userID := range []string{env.viewer.Id, actor.Id} {
			if _, err := env.core.JoinRoom(env.ctx, userID, core.KindChannel, userID, room.Id); err != nil {
				t.Fatalf("JoinRoom %d for %s: %v", index, userID, err)
			}
		}
		posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, actor.Id, fmt.Sprintf("page target %d", index), nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage %d: %v", index, err)
		}
		sequence, err := env.core.GetEventSequence(env.ctx, core.KindChannel, room.Id, posted.Id)
		if err != nil {
			t.Fatalf("GetEventSequence %d: %v", index, err)
		}
		if index == 0 {
			if err := env.core.DeleteMessage(env.ctx, actor.Id, core.KindChannel, room.Id, posted.Id); err != nil {
				t.Fatalf("DeleteMessage: %v", err)
			}
		}
		occurrence, wasCreated, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
			RecipientID:          env.viewer.Id,
			SourceEventID:        fmt.Sprintf("page-filter-source-%d", index),
			SourceCreated:        baseTime.Add(-time.Duration(index) * time.Second),
			SourceStreamSequence: sequence,
			ActorID:              actor.Id,
			Signal:               testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
			Mode:                 corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT,
			AttentionLevel:       corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SkipReadLookup:       true,
		})
		if err != nil || !wasCreated {
			t.Fatalf("Create occurrence %d = (%v, %v, %v), want created", index, occurrence, wasCreated, err)
		}
		created = append(created, occurrence)
	}

	first, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{
		Page: &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil || len(first.Msg.GetOccurrences()) != 1 || !first.Msg.GetPage().GetHasMore() || first.Msg.GetPage().GetTotalCount() != 2 {
		t.Fatalf("first filtered page = (%+v, %v), want one of two with more", first, err)
	}
	second, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{
		Page: &apiv1.PageRequest{Limit: 1, Offset: 1},
	}))
	if err != nil || len(second.Msg.GetOccurrences()) != 1 || second.Msg.GetPage().GetHasMore() {
		t.Fatalf("second filtered page = (%+v, %v), want final row", second, err)
	}
	if first.Msg.GetOccurrences()[0].GetId() == second.Msg.GetOccurrences()[0].GetId() {
		t.Fatalf("filtered pages repeated occurrence %s", first.Msg.GetOccurrences()[0].GetId())
	}
	if _, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, created[0].GetId()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("stale occurrence Get error = %v, want not found after visibility purge", err)
	}
}

func TestNotificationServiceSummaryExcludesImplicitMembershipLossOutsidePage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-implicit-actor", "Notification Implicit Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	baseTime := time.Now().UTC().Add(-time.Minute)
	createOccurrence := func(room *corev1.Room, sourceID string, sourceCreated time.Time) *corev1.NotificationOccurrence {
		t.Helper()
		posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, actor.Id, sourceID, nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage %s: %v", sourceID, err)
		}
		sequence, err := env.core.GetEventSequence(env.ctx, core.KindChannel, room.Id, posted.Id)
		if err != nil {
			t.Fatalf("GetEventSequence %s: %v", sourceID, err)
		}
		occurrence, created, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
			RecipientID:          env.viewer.Id,
			SourceEventID:        sourceID,
			SourceCreated:        sourceCreated,
			SourceStreamSequence: sequence,
			ActorID:              actor.Id,
			Signal:               testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
			Mode:                 corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_SILENT,
			AttentionLevel:       corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SkipReadLookup:       true,
		})
		if err != nil || !created {
			t.Fatalf("Create occurrence %s = (%+v, %v, %v), want created", sourceID, occurrence, created, err)
		}
		return occurrence
	}

	implicitRoom, err := env.core.CreateRoom(env.ctx, actor.Id, core.KindChannel, "", "notification-implicit-old", "")
	if err != nil {
		t.Fatalf("CreateRoom implicit: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, actor.Id, core.KindChannel, implicitRoom.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}
	stale := createOccurrence(implicitRoom, "implicit-old-source", baseTime)

	for index := 0; index < 3; index++ {
		room, err := env.core.CreateRoom(env.ctx, actor.Id, core.KindChannel, "", fmt.Sprintf("notification-explicit-new-%d", index), "")
		if err != nil {
			t.Fatalf("CreateRoom explicit %d: %v", index, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom explicit %d: %v", index, err)
		}
		createOccurrence(room, fmt.Sprintf("explicit-new-source-%d", index), baseTime.Add(time.Duration(index+1)*time.Second))
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, actor.Id, core.KindChannel, implicitRoom.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal false: %v", err)
	}

	response, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{
		Page: &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil {
		t.Fatalf("ListNotificationOccurrences: %v", err)
	}
	if len(response.Msg.GetOccurrences()) != 1 || response.Msg.GetPage().GetTotalCount() != 3 ||
		response.Msg.GetUnreadCount() != 3 || len(response.Msg.GetRoomUnreadCounts()) != 3 {
		t.Fatalf("summary after implicit membership loss = %+v, want three visible groups", response.Msg)
	}
	if _, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, stale.GetId()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("stale off-page occurrence Get error = %v, want not found", err)
	}
}

func TestNotificationServiceBoundsOccurrencePage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-v2-page-actor", "Notification Page Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	for index := 0; index < defaultNotificationLimit+5; index++ {
		if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, fmt.Sprintf("message %d", index), nil, "", "", nil, false); err != nil {
			t.Fatalf("PostMessage %d: %v", index, err)
		}
	}

	inbox, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{}))
	if err != nil || len(inbox.Msg.GetOccurrences()) != defaultNotificationLimit {
		t.Fatalf("ListNotificationOccurrences = %+v, %v, want one bounded page", inbox, err)
	}
	if inbox.Msg.GetPage().GetTotalCount() != int64(defaultNotificationLimit+5) ||
		!inbox.Msg.GetPage().GetHasMore() ||
		inbox.Msg.GetUnreadCount() != int32(defaultNotificationLimit+5) ||
		inbox.Msg.GetImportantUnreadCount() != int32(defaultNotificationLimit+5) ||
		inbox.Msg.GetNextExpiryAt() == nil {
		t.Fatalf("bounded occurrence metadata = %+v", inbox.Msg)
	}
	if counts := inbox.Msg.GetRoomUnreadCounts(); len(counts) != 1 ||
		counts[0].GetRoomId() != dm.Id ||
		counts[0].GetUnreadCount() != int32(defaultNotificationLimit+5) ||
		counts[0].GetImportantUnreadCount() != int32(defaultNotificationLimit+5) {
		t.Fatalf("bounded room occurrence metadata = %+v", counts)
	}
	projection, err := env.api.BuildRealtimeProjectionNotifications(env.ctx, env.viewer.Id)
	if err != nil ||
		projection.Occurrences.GetNextExpiryAt() == nil ||
		projection.Occurrences.GetImportantUnreadCount() != int32(defaultNotificationLimit+5) {
		t.Fatalf("realtime expiry boundary = %+v, %v", projection, err)
	}

	live, err := env.nc.SubscribeSync(subjects.LiveSyncUserEvent(env.viewer.Id, "notification_v2"))
	if err != nil {
		t.Fatalf("subscribe to notification invalidations: %v", err)
	}
	defer live.Unsubscribe()
	if err := env.nc.Flush(); err != nil {
		t.Fatalf("flush notification subscription: %v", err)
	}
	updated, err := env.notifications.MarkNotificationRead(ctx, connect.NewRequest(&apiv1.MarkNotificationReadRequest{
		NotificationId: inbox.Msg.GetOccurrences()[0].GetId(),
	}))
	if err != nil || updated.Msg.GetOccurrence().GetUnread() {
		t.Fatalf("MarkNotificationRead = %+v, %v", updated, err)
	}
	if _, err := live.NextMsg(2 * time.Second); err != nil {
		t.Fatalf("wait for coalesced notification invalidation: %v", err)
	}
	if _, err := live.NextMsg(200 * time.Millisecond); err == nil {
		t.Fatal("mark read published more than one notification invalidation")
	}
	allOccurrences, err := env.core.NotificationOccurrences().List(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("List occurrences before bulk delete: %v", err)
	}
	ids := make([]string, 0, len(allOccurrences))
	for _, occurrence := range allOccurrences {
		ids = append(ids, occurrence.GetId())
	}
	if deleted, err := env.core.NotificationOccurrences().DeleteMany(env.ctx, env.viewer.Id, ids); err != nil || deleted != len(ids) {
		t.Fatalf("DeleteMany = (%d, %v), want (%d, nil)", deleted, err, len(ids))
	}
	if _, err := live.NextMsg(2 * time.Second); err != nil {
		t.Fatalf("wait for coalesced bulk-delete invalidation: %v", err)
	}
	if _, err := live.NextMsg(200 * time.Millisecond); err == nil {
		t.Fatal("bulk delete published more than one notification invalidation")
	}
}

func TestMarkNotificationReadHydratesBeforeCommitting(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-hydration-actor", "Read Hydration", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{actor.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, actor.Id, "hydrate before marking read", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.NotificationOccurrences().WaitCurrent(env.ctx); err != nil {
		t.Fatalf("WaitCurrent: %v", err)
	}
	occurrences, err := env.core.NotificationOccurrences().List(env.ctx, env.viewer.Id)
	if err != nil || len(occurrences) != 1 || occurrences[0].GetSourceEventId() != posted.GetId() {
		t.Fatalf("notification occurrences = (%+v, %v), want posted DM", occurrences, err)
	}
	hydrationErr := errors.New("injected notification hydration failure")
	env.notifications.assembleOccurrence = func(context.Context, *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
		return nil, hydrationErr
	}

	_, err = env.notifications.MarkNotificationRead(ctx, connect.NewRequest(&apiv1.MarkNotificationReadRequest{
		NotificationId: occurrences[0].GetId(),
	}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("MarkNotificationRead code = %v, want internal", connect.CodeOf(err))
	}
	stored, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.Id, occurrences[0].GetId())
	if err != nil {
		t.Fatalf("Get occurrence after hydration failure: %v", err)
	}
	if stored.GetRead() {
		t.Fatal("occurrence was marked read after hydration failure")
	}
}

func TestPushNotificationServiceSubscribeAndUnsubscribe(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.push.Subscribe(env.ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint: "https://push.example.test/sub",
		P256Dh:   "p256dh-key",
		Auth:     "auth-secret",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated Subscribe code = %v, want unauthenticated", connect.CodeOf(err))
	}

	if _, err := env.push.Subscribe(ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint: "https://push.example.test/sub",
		P256Dh:   "p256dh-key",
		Auth:     "auth-secret",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("disabled Subscribe code = %v, want failed_precondition", connect.CodeOf(err))
	}

	env.api.config.Push = config.PushConfig{
		Enabled:         true,
		VAPIDPublicKey:  "public-key",
		VAPIDPrivateKey: "private-key",
		VAPIDSubject:    "mailto:admin@example.com",
	}
	if _, err := env.push.Subscribe(ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint: "https://push.example.test/client-host-required",
		P256Dh:   "p256dh-key",
		Auth:     "auth-secret",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing client host Subscribe code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.push.Subscribe(ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint:     "http://127.0.0.1/internal",
		P256Dh:       "p256dh-key",
		Auth:         "auth-secret",
		ClientHost:   "app.example.test",
		CleanupToken: "0123456789abcdef0123456789abcdef",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unsafe endpoint Subscribe code = %v, want invalid_argument", connect.CodeOf(err))
	}
	subResp, err := env.push.Subscribe(ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint:     "https://push.example.test/sub",
		P256Dh:       "p256dh-key",
		Auth:         "auth-secret",
		UserAgent:    stringPtr("test-agent"),
		ClientHost:   "app.example.test",
		CleanupToken: "0123456789abcdef0123456789abcdef",
	}))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if !subResp.Msg.GetSubscribed() {
		t.Fatalf("Subscribe response = %+v, want subscription acknowledgement", subResp.Msg)
	}
	subs, err := env.core.GetUserPushSubscriptions(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].GetEndpoint() != "https://push.example.test/sub" || subs[0].GetUserAgent() != "test-agent" || subs[0].GetClientHost() != "app.example.test" || subs[0].GetCleanupToken() != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("stored subscriptions = %+v, want one saved subscription", subs)
	}

	testPushCalls := 0
	env.core.OnPushTestRequested = func(_ context.Context, userID string) error {
		if userID == env.viewer.Id {
			testPushCalls++
		}
		return nil
	}
	testResp, err := env.push.SendTestNotification(ctx, connect.NewRequest(&apiv1.SendTestPushNotificationRequest{}))
	if err != nil {
		t.Fatalf("SendTestNotification: %v", err)
	}
	if !testResp.Msg.GetSent() || testPushCalls != 1 {
		t.Fatalf("SendTestNotification sent = %v, callback calls = %d", testResp.Msg.GetSent(), testPushCalls)
	}
	if _, err := env.push.SendTestNotification(ctx, connect.NewRequest(&apiv1.SendTestPushNotificationRequest{})); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("repeated SendTestNotification code = %v, want resource_exhausted", connect.CodeOf(err))
	}
	if testPushCalls != 1 {
		t.Fatalf("rate-limited SendTestNotification callback calls = %d, want 1", testPushCalls)
	}
	unsubResp, err := env.push.Unsubscribe(ctx, connect.NewRequest(&apiv1.UnsubscribePushRequest{
		Endpoint: "https://push.example.test/sub",
	}))
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if !unsubResp.Msg.GetUnsubscribed() {
		t.Fatal("Unsubscribe unsubscribed = false, want true")
	}
	subs, err = env.core.GetUserPushSubscriptions(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions after unsubscribe: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions after unsubscribe = %+v, want none", subs)
	}

	if _, err := env.push.Unsubscribe(ctx, connect.NewRequest(&apiv1.UnsubscribePushRequest{
		Endpoint: "https://push.example.test/sub",
	})); err != nil {
		t.Fatalf("idempotent Unsubscribe: %v", err)
	}

	capabilityEndpoint := "https://push.example.test/capability-cleanup"
	capabilityToken := "0123456789abcdef0123456789abcdef"
	if _, err := env.core.SavePushSubscriptionWithCleanupToken(env.ctx, env.viewer.Id, capabilityEndpoint, "p256dh-key", "capability-auth", "test-agent", capabilityToken); err != nil {
		t.Fatalf("save capability cleanup fixture: %v", err)
	}
	cleanupResp, err := env.pushCleanup.DeleteSubscription(context.Background(), connect.NewRequest(&authv1.DeleteSubscriptionRequest{
		Endpoint:     capabilityEndpoint,
		Auth:         "capability-auth",
		CleanupToken: capabilityToken,
	}))
	if err != nil {
		t.Fatalf("capability-authenticated DeleteSubscription: %v", err)
	}
	if !cleanupResp.Msg.GetCompleted() {
		t.Fatal("DeleteSubscription completed = false, want true")
	}
	if owned, err := env.core.PushSubscriptionOwnedByUser(env.ctx, env.viewer.Id, capabilityEndpoint); err != nil || owned {
		t.Fatalf("capability cleanup ownership = %t, err = %v", owned, err)
	}
}

func TestPushNotificationServiceHidesDeliveryFailureDetails(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	env.api.config.Push = config.PushConfig{
		Enabled:         true,
		VAPIDPublicKey:  "public-key",
		VAPIDPrivateKey: "private-key",
		VAPIDSubject:    "mailto:admin@example.com",
	}
	env.core.OnPushTestRequested = func(context.Context, string) error {
		return errors.New("private response marker")
	}

	_, err := env.push.SendTestNotification(ctx, connect.NewRequest(&apiv1.SendTestPushNotificationRequest{}))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("SendTestNotification code = %v, want unavailable", connect.CodeOf(err))
	}
	if strings.Contains(err.Error(), "private response marker") {
		t.Fatalf("SendTestNotification disclosed delivery error: %v", err)
	}
}

func TestVoiceCallServiceRecordsAndListsCalls(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("voice-connect")

	if _, err := env.voice.JoinCall(env.ctx, connect.NewRequest(&apiv1.JoinCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated JoinCall code = %v, want unauthenticated", connect.CodeOf(err))
	}

	disabledJoin, err := env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("disabled JoinCall: %v", err)
	}
	if disabledJoin.Msg.GetJoined() {
		t.Fatal("disabled JoinCall joined = true, want false")
	}
	disabledActive, err := env.voice.ListActiveCalls(ctx, connect.NewRequest(&apiv1.ListActiveCallsRequest{}))
	if err != nil {
		t.Fatalf("disabled ListActiveCalls: %v", err)
	}
	if len(disabledActive.Msg.GetCalls()) != 0 {
		t.Fatalf("disabled active calls = %v, want none", disabledActive.Msg.GetCalls())
	}
	if _, err := env.voice.GetActiveCall(ctx, connect.NewRequest(&apiv1.GetActiveCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("disabled GetActiveCall code = %v, want not_found", connect.CodeOf(err))
	}
	disabledBatch, err := env.voice.BatchGetActiveCalls(ctx, connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{
		RoomIds: []string{room.Id},
	}))
	if err != nil {
		t.Fatalf("disabled BatchGetActiveCalls: %v", err)
	}
	if len(disabledBatch.Msg.GetCalls()) != 0 {
		t.Fatalf("disabled BatchGetActiveCalls calls = %+v, want none", disabledBatch.Msg.GetCalls())
	}
	if _, err := env.voice.GetCallToken(ctx, connect.NewRequest(&apiv1.GetCallTokenRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("disabled GetCallToken code = %v, want failed_precondition", connect.CodeOf(err))
	}

	env.api.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}
	nonMember, err := env.core.CreateUser(env.ctx, core.SystemActorID, "voice-outsider", "Voice Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser nonMember: %v", err)
	}
	if _, err := env.voice.JoinCall(withCaller(env.ctx, nonMember), connect.NewRequest(&apiv1.JoinCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member JoinCall code = %v, want permission_denied", connect.CodeOf(err))
	}

	joinResp, err := env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("JoinCall: %v", err)
	}
	if !joinResp.Msg.GetJoined() {
		t.Fatal("JoinCall joined = false, want true")
	}

	activeResp, err := env.voice.ListActiveCalls(ctx, connect.NewRequest(&apiv1.ListActiveCallsRequest{}))
	if err != nil {
		t.Fatalf("ListActiveCalls: %v", err)
	}
	if calls := activeResp.Msg.GetCalls(); len(calls) != 1 || calls[0].GetRoom().GetId() != room.Id || calls[0].GetCallId() == "" {
		t.Fatalf("active calls = %v, want one call for %s", calls, room.Id)
	}
	nonMemberActiveResp, err := env.voice.ListActiveCalls(withCaller(env.ctx, nonMember), connect.NewRequest(&apiv1.ListActiveCallsRequest{}))
	if err != nil {
		t.Fatalf("non-member ListActiveCalls: %v", err)
	}
	if len(nonMemberActiveResp.Msg.GetCalls()) != 0 {
		t.Fatalf("non-member active calls = %v, want none", nonMemberActiveResp.Msg.GetCalls())
	}

	activeCallResp, err := env.voice.GetActiveCall(ctx, connect.NewRequest(&apiv1.GetActiveCallRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("GetActiveCall: %v", err)
	}
	activeCall := activeCallResp.Msg.GetCall()
	if activeCall.GetRoom().GetId() != room.Id || activeCall.GetCallId() == "" || len(activeCall.GetParticipants()) != 1 {
		t.Fatalf("GetActiveCall call = %+v, want room, call ID, and one participant", activeCall)
	}
	if _, err := env.voice.GetActiveCall(withCaller(env.ctx, nonMember), connect.NewRequest(&apiv1.GetActiveCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetActiveCall code = %v, want permission_denied", connect.CodeOf(err))
	}
	batchCallsResp, err := env.voice.BatchGetActiveCalls(ctx, connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{
		RoomIds: []string{room.Id, "missing-room", room.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetActiveCalls: %v", err)
	}
	if calls := batchCallsResp.Msg.GetCalls(); len(calls) != 1 || calls[0].GetRoom().GetId() != room.Id || calls[0].GetCallId() != activeCall.GetCallId() {
		t.Fatalf("BatchGetActiveCalls calls = %+v, want one active call for %s", calls, room.Id)
	}
	nonMemberBatchResp, err := env.voice.BatchGetActiveCalls(withCaller(env.ctx, nonMember), connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{
		RoomIds: []string{room.Id},
	}))
	if err != nil {
		t.Fatalf("non-member BatchGetActiveCalls: %v", err)
	}
	if len(nonMemberBatchResp.Msg.GetCalls()) != 0 {
		t.Fatalf("non-member BatchGetActiveCalls calls = %+v, want none", nonMemberBatchResp.Msg.GetCalls())
	}

	participantsResp, err := env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ListCallParticipants: %v", err)
	}
	participants := participantsResp.Msg.GetParticipants()
	if len(participants) != 1 || participants[0].GetUser().GetId() != env.viewer.Id || participants[0].GetCallId() == "" || participants[0].GetJoinedAt() == nil {
		t.Fatalf("participants = %+v, want viewer participant with call metadata", participants)
	}

	tokenResp, err := env.voice.GetCallToken(ctx, connect.NewRequest(&apiv1.GetCallTokenRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("GetCallToken: %v", err)
	}
	if tokenResp.Msg.GetToken() == "" || tokenResp.Msg.GetE2EeKey() == "" || tokenResp.Msg.GetCallId() != participants[0].GetCallId() {
		t.Fatalf("GetCallToken response = %+v, want token/e2ee key/call id", tokenResp.Msg)
	}
	publisherResp, err := env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{
		RoomId: room.Id,
		Kind:   apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE,
	}))
	if err != nil {
		t.Fatalf("CreateCallMediaPublisherToken: %v", err)
	}
	if publisherResp.Msg.GetToken() == "" || publisherResp.Msg.GetE2EeKey() == "" || publisherResp.Msg.GetCallId() != participants[0].GetCallId() {
		t.Fatalf("CreateCallMediaPublisherToken response = %+v", publisherResp.Msg)
	}

	leaveResp, err := env.voice.LeaveCall(ctx, connect.NewRequest(&apiv1.LeaveCallRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("LeaveCall: %v", err)
	}
	if !leaveResp.Msg.GetLeft() {
		t.Fatal("LeaveCall left = false, want true")
	}
	participantsResp, err = env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ListCallParticipants after leave: %v", err)
	}
	if len(participantsResp.Msg.GetParticipants()) != 0 {
		t.Fatalf("participants after leave = %+v, want none", participantsResp.Msg.GetParticipants())
	}
	if _, err := env.voice.GetActiveCall(ctx, connect.NewRequest(&apiv1.GetActiveCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetActiveCall after leave code = %v, want not_found", connect.CodeOf(err))
	}
	if _, err := env.voice.GetCallToken(ctx, connect.NewRequest(&apiv1.GetCallTokenRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("GetCallToken after leave code = %v, want failed_precondition", connect.CodeOf(err))
	}
	if _, err := env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{
		RoomId: room.Id,
		Kind:   apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("CreateCallMediaPublisherToken after leave code = %v, want failed_precondition", connect.CodeOf(err))
	}
}

func TestVoiceCallServiceListsDMCallsForParticipants(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}

	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "voice-dm-participant", "Voice DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "voice-dm-outsider", "Voice DM Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}

	viewerCtx := withCaller(env.ctx, env.viewer)
	start, err := env.rooms.StartDM(viewerCtx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	}))
	if err != nil {
		t.Fatalf("StartDM: %v", err)
	}
	dm := start.Msg.GetRoom()
	if dm.GetKind() != apiv1.RoomKind_ROOM_KIND_DM {
		t.Fatalf("StartDM room kind = %v, want DM", dm.GetKind())
	}

	join, err := env.voice.JoinCall(viewerCtx, connect.NewRequest(&apiv1.JoinCallRequest{
		RoomId: dm.GetId(),
	}))
	if err != nil {
		t.Fatalf("JoinCall: %v", err)
	}
	if !join.Msg.GetJoined() {
		t.Fatal("JoinCall joined = false, want true")
	}

	assertDMCall := func(label string, calls []*apiv1.ActiveCall) {
		t.Helper()
		if len(calls) != 1 || calls[0].GetRoom().GetId() != dm.GetId() {
			t.Fatalf("%s calls = %+v, want DM %s", label, calls, dm.GetId())
		}
		participants := calls[0].GetParticipants()
		if len(participants) != 1 || participants[0].GetUser().GetId() != env.viewer.Id {
			t.Fatalf("%s participants = %+v, want viewer %s", label, participants, env.viewer.Id)
		}
	}

	participantCtx := withCaller(env.ctx, participant)
	listed, err := env.voice.ListActiveCalls(participantCtx, connect.NewRequest(&apiv1.ListActiveCallsRequest{}))
	if err != nil {
		t.Fatalf("ListActiveCalls participant: %v", err)
	}
	assertDMCall("ListActiveCalls participant", listed.Msg.GetCalls())

	projected, err := env.api.BuildRealtimeProjectionActiveCalls(env.ctx, participant.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionActiveCalls participant: %v", err)
	}
	assertDMCall("realtime projection participant", projected)

	outsiderCtx := withCaller(env.ctx, outsider)
	outsiderListed, err := env.voice.ListActiveCalls(outsiderCtx, connect.NewRequest(&apiv1.ListActiveCallsRequest{}))
	if err != nil {
		t.Fatalf("ListActiveCalls outsider: %v", err)
	}
	if len(outsiderListed.Msg.GetCalls()) != 0 {
		t.Fatalf("ListActiveCalls outsider calls = %+v, want none", outsiderListed.Msg.GetCalls())
	}
	outsiderProjected, err := env.api.BuildRealtimeProjectionActiveCalls(env.ctx, outsider.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionActiveCalls outsider: %v", err)
	}
	if len(outsiderProjected) != 0 {
		t.Fatalf("realtime projection outsider calls = %+v, want none", outsiderProjected)
	}
}

func TestVoiceCallServiceRoomRemovalClearsCallParticipant(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("voice-room-removal")
	env.api.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}

	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "voice-room-removal-target", "Voice Room Removal Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom target: %v", err)
	}
	if err := env.core.RecordCallParticipantJoined(env.ctx, room.Id, target.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}

	participantsResp, err := env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ListCallParticipants before removal: %v", err)
	}
	if participants := participantsResp.Msg.GetParticipants(); len(participants) != 1 || participants[0].GetUser().GetId() != target.Id {
		t.Fatalf("participants before removal = %+v, want target", participants)
	}

	removeResp, err := env.rooms.RemoveMember(ctx, connect.NewRequest(&apiv1.RemoveMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if !removeResp.Msg.GetRemoved() {
		t.Fatal("RemoveMember removed=false, want true")
	}

	participantsResp, err = env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ListCallParticipants after removal: %v", err)
	}
	if len(participantsResp.Msg.GetParticipants()) != 0 {
		t.Fatalf("participants after removal = %+v, want none", participantsResp.Msg.GetParticipants())
	}
	if _, err := env.voice.GetActiveCall(ctx, connect.NewRequest(&apiv1.GetActiveCallRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetActiveCall after removal code = %v, want not_found", connect.CodeOf(err))
	}
	if _, err := env.voice.GetCallToken(withCaller(env.ctx, target), connect.NewRequest(&apiv1.GetCallTokenRequest{
		RoomId: room.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("removed member GetCallToken code = %v, want permission_denied", connect.CodeOf(err))
	}
}

func TestMyAccountServiceUpdatePresence(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UpdatePresence(env.ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdatePresence code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	if _, err := env.account.UpdatePresence(ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unspecified UpdatePresence code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if _, err := env.account.UpdatePresence(ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("offline UpdatePresence code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	resp, err := env.account.UpdatePresence(ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status:       apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB,
		UserSelected: true,
	}))
	if err != nil {
		t.Fatalf("UpdatePresence: %v", err)
	}
	if resp.Msg.Status != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("UpdatePresence status = %v, want DO_NOT_DISTURB", resp.Msg.Status)
	}

	stored, err := env.core.GetUserPresence(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPresence: %v", err)
	}
	if stored != core.PresenceStatusDoNotDisturb {
		t.Fatalf("stored presence = %q, want %q", stored, core.PresenceStatusDoNotDisturb)
	}

	autoResp, err := env.account.UpdatePresence(ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
	}))
	if err != nil {
		t.Fatalf("automatic online UpdatePresence: %v", err)
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

	if _, err := env.account.UpdatePresence(ctx, connect.NewRequest(&apiv1.UpdatePresenceRequest{
		Status:       apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
		UserSelected: true,
	})); err != nil {
		t.Fatalf("explicit online UpdatePresence: %v", err)
	}
	stored, err = env.core.GetUserPresence(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPresence after explicit online: %v", err)
	}
	if stored != core.PresenceStatusOnline {
		t.Fatalf("explicit online stored presence = %q, want %q", stored, core.PresenceStatusOnline)
	}
}
