package http_server

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (env *wsTestEnv) connectWire(t *testing.T) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/wire"
	header := http.Header{}
	for _, c := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", c.String())
	}

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("wire WebSocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("wire WebSocket dial failed: %v", err)
	}

	t.Cleanup(func() { conn.Close() })
	return conn
}

func sendWireFrame(t *testing.T, conn *websocket.Conn, frame *wirev1.ClientFrame) {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal wire frame: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("send wire frame: %v", err)
	}
}

func readWireFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) *wirev1.ServerFrame {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read wire frame: %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("wire message type = %d, want binary", messageType)
	}
	var frame wirev1.ServerFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal wire frame: %v", err)
	}
	return &frame
}

func mustProtoBytes(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal proto body: %v", err)
	}
	return data
}

func sendWireHello(t *testing.T, conn *websocket.Conn, resumeAfter string) {
	t.Helper()
	sendWireFrame(t, conn, &wirev1.ClientFrame{
		FrameId: "hello",
		Kind: &wirev1.ClientFrame_Hello{Hello: &wirev1.ClientHello{
			ProtocolVersion: wireProtocolVersion,
			ResumeAfter:     resumeAfter,
		}},
	})
	frame := readWireFrame(t, conn, 5*time.Second)
	if frame.GetHello() == nil {
		t.Fatalf("first wire frame = %T, want ServerHello", frame.GetKind())
	}
	if frame.GetHello().GetProtocolVersion() != wireProtocolVersion {
		t.Fatalf("protocol version = %q, want %q", frame.GetHello().GetProtocolVersion(), wireProtocolVersion)
	}
}

func sendWireRequest(t *testing.T, conn *websocket.Conn, frameID, requestID, method string, body proto.Message) {
	t.Helper()
	sendWireFrame(t, conn, &wirev1.ClientFrame{
		FrameId: frameID,
		Kind: &wirev1.ClientFrame_Request{Request: &wirev1.Request{
			RequestId: requestID,
			Method:    method,
			Body:      mustProtoBytes(t, body),
		}},
	})
}

func TestWireAPI_RequestResponseAndLiveEvent(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wireuser", "Wire User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, user.Id, "wireuser@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}

	env.login(t, "wireuser", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "viewer-frame", "viewer", "/chatto.api.v1.ChattoApiService/GetViewer", &apiv1.GetViewerRequest{})
	viewerResp := readWireResponse(t, conn, "viewer", 5*time.Second)
	var viewer apiv1.GetViewerResponse
	if err := proto.Unmarshal(viewerResp.GetBody(), &viewer); err != nil {
		t.Fatalf("unmarshal viewer response: %v", err)
	}
	if viewer.GetViewer().GetUser().GetId() != user.Id {
		t.Fatalf("viewer user id = %q, want %q", viewer.GetViewer().GetUser().GetId(), user.Id)
	}
	if viewer.GetServerProfile().GetName() == "" {
		t.Fatal("viewer response server profile name is empty")
	}
	if viewer.GetViewer().GetPermissions() == nil {
		t.Fatal("viewer response permissions are nil")
	}
	if !viewer.GetViewer().GetPermissions().GetCanStartDms() {
		t.Fatal("viewer response should allow starting DMs by default")
	}
	if viewer.GetViewer().GetServerNotificationPreference().GetEffectiveLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL {
		t.Fatalf(
			"viewer server notification effective level = %s, want NORMAL",
			viewer.GetViewer().GetServerNotificationPreference().GetEffectiveLevel(),
		)
	}
	if !containsRoomPreference(viewer.GetViewer().GetRoomNotificationPreferences(), room.Id) {
		t.Fatalf("viewer room notification preferences did not include room %q", room.Id)
	}

	sendWireRequest(t, conn, "current-user-frame", "current-user", "/"+wireMethodGetCurrentUser, &apiv1.GetCurrentUserRequest{})
	currentUserWireResp := readWireResponse(t, conn, "current-user", 5*time.Second)
	var currentUser apiv1.GetCurrentUserResponse
	if err := proto.Unmarshal(currentUserWireResp.GetBody(), &currentUser); err != nil {
		t.Fatalf("unmarshal current user response: %v", err)
	}
	if currentUser.GetUser().GetUser().GetId() != user.Id {
		t.Fatalf("current user id = %q, want %q", currentUser.GetUser().GetUser().GetId(), user.Id)
	}
	if !currentUser.GetUser().GetHasVerifiedEmail() {
		t.Fatal("current user response should report verified email")
	}
	if currentUser.GetUser().GetSettings() == nil {
		t.Fatal("current user settings are nil")
	}

	sendWireRequest(t, conn, "settings-frame", "server-settings", "/"+wireMethodGetServerSettings, &apiv1.GetAuthenticatedServerSettingsRequest{})
	settingsWireResp := readWireResponse(t, conn, "server-settings", 5*time.Second)
	var serverSettings apiv1.GetAuthenticatedServerSettingsResponse
	if err := proto.Unmarshal(settingsWireResp.GetBody(), &serverSettings); err != nil {
		t.Fatalf("unmarshal authenticated server settings response: %v", err)
	}
	if serverSettings.GetSettings().GetMessageEditWindowSeconds() != int32(core.MessageEditWindow/time.Second) {
		t.Fatalf(
			"message edit window = %d, want %d",
			serverSettings.GetSettings().GetMessageEditWindowSeconds(),
			int32(core.MessageEditWindow/time.Second),
		)
	}
	if serverSettings.GetSettings().GetMaxUploadSize() <= 0 {
		t.Fatal("authenticated server settings max upload size should be positive")
	}

	sendWireRequest(t, conn, "post-frame", "post", "/chatto.api.v1.ChattoApiService/PostMessage", &apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "hello over wire",
	})

	var postResp *apiv1.PostMessageResponse
	var pushed *wirev1.StreamEvent
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (postResp == nil || pushed == nil) {
		frame := readWireFrame(t, conn, time.Until(deadline))
		if resp := frame.GetResponse(); resp != nil && resp.GetRequestId() == "post" {
			var decoded apiv1.PostMessageResponse
			if err := proto.Unmarshal(resp.GetBody(), &decoded); err != nil {
				t.Fatalf("unmarshal post response: %v", err)
			}
			postResp = &decoded
			continue
		}
		if event := frame.GetEvent(); event != nil {
			if event.GetDurableEvent().GetMessagePosted().GetRoomId() == room.Id {
				pushed = event
			}
		}
		if errFrame := frame.GetError(); errFrame != nil {
			t.Fatalf("unexpected wire error: %s", errFrame.GetMessage())
		}
	}
	if postResp == nil {
		t.Fatal("did not receive PostMessage response")
	}
	if pushed == nil {
		t.Fatal("did not receive pushed MessagePosted event")
	}
	if postResp.GetEvent().GetId() == "" {
		t.Fatal("PostMessage response event id is empty")
	}
	if pushed.GetDurableEvent().GetId() != postResp.GetEvent().GetId() {
		t.Fatalf("pushed event id = %q, want response event id %q", pushed.GetDurableEvent().GetId(), postResp.GetEvent().GetId())
	}
	if pushed.GetEventType() != "message_posted" {
		t.Fatalf("pushed event type = %q, want message_posted", pushed.GetEventType())
	}
	if !hasInvalidation(pushed, wirev1.InvalidationKind_INVALIDATION_KIND_ROOM_TIMELINE, room.Id) {
		t.Fatal("pushed event did not include room timeline invalidation")
	}
}

func TestWireAPI_ProfileSettingsMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wireprofile", "Wire Profile", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	env.login(t, "wireprofile", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "profile-frame", "profile", "/"+wireMethodGetProfileSettings, &apiv1.GetProfileSettingsRequest{})
	profileWireResp := readWireResponse(t, conn, "profile", 5*time.Second)
	var profileResp apiv1.GetProfileSettingsResponse
	if err := proto.Unmarshal(profileWireResp.GetBody(), &profileResp); err != nil {
		t.Fatalf("unmarshal GetProfileSettings response: %v", err)
	}
	if profileResp.GetProfile().GetUser().GetId() != user.Id {
		t.Fatalf("profile user id = %q, want %q", profileResp.GetProfile().GetUser().GetId(), user.Id)
	}
	if profileResp.GetProfile().GetLastLoginChange() != nil {
		t.Fatal("initial profile last_login_change should be nil")
	}

	sendWireRequest(t, conn, "empty-profile-update-frame", "empty-profile-update", "/"+wireMethodUpdateProfile, &apiv1.UpdateProfileRequest{})
	errFrame := readWireError(t, conn, "empty-profile-update", 5*time.Second)
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("empty UpdateProfile code = %v, want INVALID_ARGUMENT", errFrame.GetCode())
	}

	displayName := "Wire Profiled"
	login := "wireprofiled2"
	sendWireRequest(t, conn, "update-profile-frame", "update-profile", "/"+wireMethodUpdateProfile, &apiv1.UpdateProfileRequest{
		DisplayName: &displayName,
		Login:       &login,
	})
	updateWireResp := readWireResponse(t, conn, "update-profile", 5*time.Second)
	var updateResp apiv1.UpdateProfileResponse
	if err := proto.Unmarshal(updateWireResp.GetBody(), &updateResp); err != nil {
		t.Fatalf("unmarshal UpdateProfile response: %v", err)
	}
	if updateResp.GetProfile().GetUser().GetDisplayName() != displayName {
		t.Fatalf("updated display name = %q, want %q", updateResp.GetProfile().GetUser().GetDisplayName(), displayName)
	}
	if updateResp.GetProfile().GetUser().GetLogin() != login {
		t.Fatalf("updated login = %q, want %q", updateResp.GetProfile().GetUser().GetLogin(), login)
	}
	if updateResp.GetProfile().GetLastLoginChange() == nil {
		t.Fatal("UpdateProfile response last_login_change is nil after login change")
	}

	sendWireRequest(t, conn, "profile-after-frame", "profile-after", "/"+wireMethodGetProfileSettings, &apiv1.GetProfileSettingsRequest{})
	profileAfterWireResp := readWireResponse(t, conn, "profile-after", 5*time.Second)
	var profileAfter apiv1.GetProfileSettingsResponse
	if err := proto.Unmarshal(profileAfterWireResp.GetBody(), &profileAfter); err != nil {
		t.Fatalf("unmarshal GetProfileSettings after response: %v", err)
	}
	if profileAfter.GetProfile().GetUser().GetLogin() != login {
		t.Fatalf("profile after login = %q, want %q", profileAfter.GetProfile().GetUser().GetLogin(), login)
	}
	if profileAfter.GetProfile().GetLastLoginChange() == nil {
		t.Fatal("profile after last_login_change is nil")
	}
}

func TestWireAPI_AdminMemberMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	owner, err := env.core.CreateUser(env.ctx, "system", "wirememberadmin", "Wire Member Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "wiremembertarget", "Wire Member Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "wiremembertarget2"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}

	env.login(t, "wirememberadmin", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "list-members-frame", "list-members", "/"+wireMethodListAdminMembers, &apiv1.ListAdminMembersRequest{
		Search: "wiremembertarget2",
		Limit:  10,
	})
	listWireResp := readWireResponse(t, conn, "list-members", 5*time.Second)
	var listResp apiv1.ListAdminMembersResponse
	if err := proto.Unmarshal(listWireResp.GetBody(), &listResp); err != nil {
		t.Fatalf("unmarshal ListAdminMembers response: %v", err)
	}
	if listResp.GetTotalCount() != 1 {
		t.Fatalf("member total_count = %d, want 1", listResp.GetTotalCount())
	}
	if len(listResp.GetMembers()) != 1 || listResp.GetMembers()[0].GetUser().GetId() != target.Id {
		t.Fatalf("member list did not include target user")
	}
	if len(listResp.GetRoles()) == 0 {
		t.Fatal("member list should include role catalog")
	}

	sendWireRequest(t, conn, "get-member-frame", "get-member", "/"+wireMethodGetAdminMember, &apiv1.GetAdminMemberRequest{
		UserId: target.Id,
	})
	memberWireResp := readWireResponse(t, conn, "get-member", 5*time.Second)
	var memberResp apiv1.GetAdminMemberResponse
	if err := proto.Unmarshal(memberWireResp.GetBody(), &memberResp); err != nil {
		t.Fatalf("unmarshal GetAdminMember response: %v", err)
	}
	if memberResp.GetMember().GetUser().GetId() != target.Id {
		t.Fatalf("member id = %q, want %q", memberResp.GetMember().GetUser().GetId(), target.Id)
	}
	if memberResp.GetMember().GetLastLoginChange() == nil {
		t.Fatal("owner should see target last_login_change")
	}
	if !memberResp.GetViewerCanAssignRoles() || !memberResp.GetViewerCanManageRoles() || !memberResp.GetViewerCanManageUserPermissions() {
		t.Fatal("owner should receive member admin capability flags")
	}
	if len(memberResp.GetAvailablePermissions()) == 0 {
		t.Fatal("member detail should include available permissions")
	}

	newLogin := "wiremembermanaged"
	newDisplayName := "Wire Member Managed"
	sendWireRequest(t, conn, "update-member-frame", "update-member", "/"+wireMethodAdminUpdateUser, &apiv1.AdminUpdateUserRequest{
		UserId:      target.Id,
		Login:       &newLogin,
		DisplayName: &newDisplayName,
	})
	updateWireResp := readWireResponse(t, conn, "update-member", 5*time.Second)
	var updateResp apiv1.AdminUpdateUserResponse
	if err := proto.Unmarshal(updateWireResp.GetBody(), &updateResp); err != nil {
		t.Fatalf("unmarshal AdminUpdateUser response: %v", err)
	}
	if updateResp.GetMember().GetUser().GetLogin() != newLogin {
		t.Fatalf("updated login = %q, want %q", updateResp.GetMember().GetUser().GetLogin(), newLogin)
	}
	if updateResp.GetMember().GetUser().GetDisplayName() != newDisplayName {
		t.Fatalf("updated display name = %q, want %q", updateResp.GetMember().GetUser().GetDisplayName(), newDisplayName)
	}

	sendWireRequest(t, conn, "clear-cooldown-frame", "clear-cooldown", "/"+wireMethodAdminClearCooldown, &apiv1.AdminClearUsernameCooldownRequest{
		UserId: target.Id,
	})
	clearWireResp := readWireResponse(t, conn, "clear-cooldown", 5*time.Second)
	var clearResp apiv1.AdminClearUsernameCooldownResponse
	if err := proto.Unmarshal(clearWireResp.GetBody(), &clearResp); err != nil {
		t.Fatalf("unmarshal AdminClearUsernameCooldown response: %v", err)
	}
	if clearResp.GetMember().GetLastLoginChange() != nil {
		t.Fatal("cleared cooldown response should omit last_login_change")
	}

	sendWireRequest(t, conn, "assign-role-frame", "assign-role", "/"+wireMethodAssignMemberRole, &apiv1.AssignMemberRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})
	assignWireResp := readWireResponse(t, conn, "assign-role", 5*time.Second)
	var assignResp apiv1.AssignMemberRoleResponse
	if err := proto.Unmarshal(assignWireResp.GetBody(), &assignResp); err != nil {
		t.Fatalf("unmarshal AssignMemberRole response: %v", err)
	}
	if !containsString(assignResp.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("assigned roles = %v, want moderator", assignResp.GetMember().GetRoles())
	}

	sendWireRequest(t, conn, "revoke-role-frame", "revoke-role", "/"+wireMethodRevokeMemberRole, &apiv1.RevokeMemberRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})
	revokeWireResp := readWireResponse(t, conn, "revoke-role", 5*time.Second)
	var revokeResp apiv1.RevokeMemberRoleResponse
	if err := proto.Unmarshal(revokeWireResp.GetBody(), &revokeResp); err != nil {
		t.Fatalf("unmarshal RevokeMemberRole response: %v", err)
	}
	if containsString(revokeResp.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("revoked roles = %v, did not expect moderator", revokeResp.GetMember().GetRoles())
	}
}

func TestWireAPI_AdminRoleMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	owner, err := env.core.CreateUser(env.ctx, "system", "wireroleadmin", "Wire Role Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "wireroletarget", "Wire Role Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}

	env.login(t, "wireroleadmin", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "role-caps-frame", "role-caps", "/"+wireMethodGetAdminRoleCaps, &apiv1.GetAdminRoleCapabilitiesRequest{})
	capsWireResp := readWireResponse(t, conn, "role-caps", 5*time.Second)
	var caps apiv1.GetAdminRoleCapabilitiesResponse
	if err := proto.Unmarshal(capsWireResp.GetBody(), &caps); err != nil {
		t.Fatalf("unmarshal GetAdminRoleCapabilities response: %v", err)
	}
	if !caps.GetViewerCanManageRoles() || !caps.GetViewerCanAssignRoles() {
		t.Fatal("owner should receive role management capability flags")
	}

	sendWireRequest(t, conn, "create-role-frame", "create-role", "/"+wireMethodCreateAdminRole, &apiv1.CreateAdminRoleRequest{
		Name:        "wirerole",
		DisplayName: "Wire Role",
		Description: "Created over wire",
		Pingable:    true,
	})
	createWireResp := readWireResponse(t, conn, "create-role", 5*time.Second)
	var created apiv1.CreateAdminRoleResponse
	if err := proto.Unmarshal(createWireResp.GetBody(), &created); err != nil {
		t.Fatalf("unmarshal CreateAdminRole response: %v", err)
	}
	if created.GetRole().GetName() != "wirerole" {
		t.Fatalf("created role name = %q, want wirerole", created.GetRole().GetName())
	}
	if !created.GetRole().GetPingable() {
		t.Fatal("created role should be pingable")
	}

	if err := env.core.AssignServerRole(env.ctx, owner.Id, target.Id, "wirerole"); err != nil {
		t.Fatalf("AssignServerRole target: %v", err)
	}

	sendWireRequest(t, conn, "get-role-frame", "get-role", "/"+wireMethodGetAdminRole, &apiv1.GetAdminRoleRequest{
		Name: "wirerole",
	})
	getWireResp := readWireResponse(t, conn, "get-role", 5*time.Second)
	var roleResp apiv1.GetAdminRoleResponse
	if err := proto.Unmarshal(getWireResp.GetBody(), &roleResp); err != nil {
		t.Fatalf("unmarshal GetAdminRole response: %v", err)
	}
	if roleResp.GetRole().GetDisplayName() != "Wire Role" {
		t.Fatalf("role display name = %q, want Wire Role", roleResp.GetRole().GetDisplayName())
	}
	if !containsString(userIDs(roleResp.GetUsers()), target.Id) {
		t.Fatalf("role users = %v, want target %q", userIDs(roleResp.GetUsers()), target.Id)
	}

	pingable := false
	sendWireRequest(t, conn, "update-role-frame", "update-role", "/"+wireMethodUpdateAdminRole, &apiv1.UpdateAdminRoleRequest{
		Name:        "wirerole",
		DisplayName: "Wire Role Renamed",
		Description: "Updated over wire",
		Pingable:    &pingable,
	})
	updateWireResp := readWireResponse(t, conn, "update-role", 5*time.Second)
	var updated apiv1.UpdateAdminRoleResponse
	if err := proto.Unmarshal(updateWireResp.GetBody(), &updated); err != nil {
		t.Fatalf("unmarshal UpdateAdminRole response: %v", err)
	}
	if updated.GetRole().GetDisplayName() != "Wire Role Renamed" {
		t.Fatalf("updated role display name = %q, want Wire Role Renamed", updated.GetRole().GetDisplayName())
	}
	if updated.GetRole().GetPingable() {
		t.Fatal("updated role should not be pingable")
	}

	sendWireRequest(t, conn, "delete-role-frame", "delete-role", "/"+wireMethodDeleteAdminRole, &apiv1.DeleteAdminRoleRequest{
		Name: "wirerole",
	})
	deleteWireResp := readWireResponse(t, conn, "delete-role", 5*time.Second)
	var deleted apiv1.DeleteAdminRoleResponse
	if err := proto.Unmarshal(deleteWireResp.GetBody(), &deleted); err != nil {
		t.Fatalf("unmarshal DeleteAdminRole response: %v", err)
	}
	if !deleted.GetDeleted() {
		t.Fatal("DeleteAdminRole should return deleted=true")
	}
	if _, err := env.core.GetServerRole(env.ctx, "wirerole"); !errors.Is(err, core.ErrRoleNotFound) {
		t.Fatalf("GetServerRole after delete error = %v, want ErrRoleNotFound", err)
	}
}

func TestWireAPI_AdminPermissionMatrixMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	owner, err := env.core.CreateUser(env.ctx, "system", "wirematrixadmin", "Wire Matrix Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "wirematrixtarget", "Wire Matrix Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := env.core.CreateServerRole(env.ctx, owner.Id, "wirematrixrole", "Wire Matrix Role", "Editable over wire", false); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}

	env.login(t, "wirematrixadmin", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "tier-frame", "tier", "/"+wireMethodGetRoleTierMatrix, &apiv1.GetRolePermissionTierMatrixRequest{})
	tierWireResp := readWireResponse(t, conn, "tier", 5*time.Second)
	var tier apiv1.GetRolePermissionTierMatrixResponse
	if err := proto.Unmarshal(tierWireResp.GetBody(), &tier); err != nil {
		t.Fatalf("unmarshal GetRolePermissionTierMatrix response: %v", err)
	}
	if len(tier.GetMatrix().GetApplicablePermissions()) == 0 {
		t.Fatal("tier matrix should include applicable permissions")
	}
	if len(tier.GetMatrix().GetRoles()) == 0 {
		t.Fatal("tier matrix should include roles")
	}

	sendWireRequest(t, conn, "set-role-frame", "set-role", "/"+wireMethodSetRolePermission, &apiv1.SetRolePermissionStateRequest{
		RoleName:   "wirematrixrole",
		Permission: string(core.PermMessagePost),
		State:      apiv1.PermissionEditState_PERMISSION_EDIT_STATE_ALLOW,
	})
	setRoleWireResp := readWireResponse(t, conn, "set-role", 5*time.Second)
	var setRole apiv1.SetPermissionStateResponse
	if err := proto.Unmarshal(setRoleWireResp.GetBody(), &setRole); err != nil {
		t.Fatalf("unmarshal SetRolePermissionState response: %v", err)
	}
	if !setRole.GetChanged() {
		t.Fatal("SetRolePermissionState should return changed=true")
	}

	sendWireRequest(t, conn, "role-matrix-frame", "role-matrix", "/"+wireMethodGetRoleMatrix, &apiv1.GetRolePermissionMatrixRequest{
		RoleName: "wirematrixrole",
	})
	roleMatrixWireResp := readWireResponse(t, conn, "role-matrix", 5*time.Second)
	var roleMatrix apiv1.GetRolePermissionMatrixResponse
	if err := proto.Unmarshal(roleMatrixWireResp.GetBody(), &roleMatrix); err != nil {
		t.Fatalf("unmarshal GetRolePermissionMatrix response: %v", err)
	}
	roleCell := findPermissionMatrixCell(roleMatrix.GetMatrix().GetCells(), "server", string(core.PermMessagePost))
	if roleCell == nil {
		t.Fatal("role matrix did not include server message.post cell")
	}
	if roleCell.GetOverride() != apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_ALLOW {
		t.Fatalf("role server override = %v, want ALLOW", roleCell.GetOverride())
	}

	sendWireRequest(t, conn, "set-user-frame", "set-user", "/"+wireMethodSetUserPermission, &apiv1.SetUserPermissionStateRequest{
		UserId:     target.Id,
		Permission: string(core.PermUserDeleteSelf),
		State:      apiv1.PermissionEditState_PERMISSION_EDIT_STATE_DENY,
	})
	setUserWireResp := readWireResponse(t, conn, "set-user", 5*time.Second)
	var setUser apiv1.SetPermissionStateResponse
	if err := proto.Unmarshal(setUserWireResp.GetBody(), &setUser); err != nil {
		t.Fatalf("unmarshal SetUserPermissionState response: %v", err)
	}
	if !setUser.GetChanged() {
		t.Fatal("SetUserPermissionState should return changed=true")
	}

	sendWireRequest(t, conn, "user-matrix-frame", "user-matrix", "/"+wireMethodGetUserMatrix, &apiv1.GetUserPermissionMatrixRequest{
		UserId: target.Id,
	})
	userMatrixWireResp := readWireResponse(t, conn, "user-matrix", 5*time.Second)
	var userMatrix apiv1.GetUserPermissionMatrixResponse
	if err := proto.Unmarshal(userMatrixWireResp.GetBody(), &userMatrix); err != nil {
		t.Fatalf("unmarshal GetUserPermissionMatrix response: %v", err)
	}
	userCell := findPermissionMatrixCell(userMatrix.GetMatrix().GetCells(), "server", string(core.PermUserDeleteSelf))
	if userCell == nil {
		t.Fatal("user matrix did not include server user.delete-self cell")
	}
	if userCell.GetOverride() != apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_DENY {
		t.Fatalf("user server override = %v, want DENY", userCell.GetOverride())
	}
}

func TestWireAPI_AccountAndServerSettingsMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wireaccount", "Wire Account", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, user.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}

	env.login(t, "wireaccount", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "account-status-frame", "account-status", "/"+wireMethodGetAccountDeletion, &apiv1.GetAccountDeletionStatusRequest{})
	accountStatusWireResp := readWireResponse(t, conn, "account-status", 5*time.Second)
	var accountStatus apiv1.GetAccountDeletionStatusResponse
	if err := proto.Unmarshal(accountStatusWireResp.GetBody(), &accountStatus); err != nil {
		t.Fatalf("unmarshal GetAccountDeletionStatus response: %v", err)
	}
	if !accountStatus.GetViewerCanDeleteAccount() {
		t.Fatal("account deletion status should allow owner self-deletion")
	}

	sendWireRequest(t, conn, "editable-settings-frame", "editable-settings", "/"+wireMethodGetEditableServerConfig, &apiv1.GetServerSettingsRequest{})
	settingsWireResp := readWireResponse(t, conn, "editable-settings", 5*time.Second)
	var settings apiv1.GetServerSettingsResponse
	if err := proto.Unmarshal(settingsWireResp.GetBody(), &settings); err != nil {
		t.Fatalf("unmarshal GetServerSettings response: %v", err)
	}
	if !settings.GetSettings().GetViewerCanManageServer() {
		t.Fatal("server settings response should allow owner management")
	}
	if settings.GetSettings().GetName() != "Chatto" {
		t.Fatalf("server settings name = %q, want Chatto", settings.GetSettings().GetName())
	}

	serverName := "Wire Settings"
	description := "Server settings over wire"
	motd := "wire motd"
	welcomeMessage := "wire welcome"
	sendWireRequest(t, conn, "update-settings-frame", "update-settings", "/"+wireMethodUpdateServerConfig, &apiv1.UpdateServerSettingsRequest{
		ServerName:     &serverName,
		Description:    &description,
		Motd:           &motd,
		WelcomeMessage: &welcomeMessage,
	})
	updateSettingsWireResp := readWireResponse(t, conn, "update-settings", 5*time.Second)
	var updateSettings apiv1.UpdateServerSettingsResponse
	if err := proto.Unmarshal(updateSettingsWireResp.GetBody(), &updateSettings); err != nil {
		t.Fatalf("unmarshal UpdateServerSettings response: %v", err)
	}
	if updateSettings.GetSettings().GetName() != serverName {
		t.Fatalf("updated server settings name = %q, want %q", updateSettings.GetSettings().GetName(), serverName)
	}
	if updateSettings.GetSettings().GetDescription() != description {
		t.Fatalf("updated server settings description = %q, want %q", updateSettings.GetSettings().GetDescription(), description)
	}

	sendWireRequest(t, conn, "security-config-frame", "security-config", "/"+wireMethodGetAdminSecurityConfig, &apiv1.GetAdminSecurityConfigRequest{})
	securityConfigWireResp := readWireResponse(t, conn, "security-config", 5*time.Second)
	var securityConfig apiv1.GetAdminSecurityConfigResponse
	if err := proto.Unmarshal(securityConfigWireResp.GetBody(), &securityConfig); err != nil {
		t.Fatalf("unmarshal GetAdminSecurityConfig response: %v", err)
	}
	if securityConfig.GetConfig().GetBlockedUsernames() != core.DefaultBlockedUsernames {
		t.Fatalf("blocked usernames = %q, want %q", securityConfig.GetConfig().GetBlockedUsernames(), core.DefaultBlockedUsernames)
	}

	blockedUsernames := "root\nadmin\nreserved"
	sendWireRequest(t, conn, "update-blocked-usernames-frame", "update-blocked-usernames", "/"+wireMethodUpdateBlockedUsernames, &apiv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: blockedUsernames,
	})
	blockedUsernamesWireResp := readWireResponse(t, conn, "update-blocked-usernames", 5*time.Second)
	var blockedUsernamesResp apiv1.UpdateBlockedUsernamesResponse
	if err := proto.Unmarshal(blockedUsernamesWireResp.GetBody(), &blockedUsernamesResp); err != nil {
		t.Fatalf("unmarshal UpdateBlockedUsernames response: %v", err)
	}
	if blockedUsernamesResp.GetConfig().GetBlockedUsernames() != blockedUsernames {
		t.Fatalf("updated blocked usernames = %q, want %q", blockedUsernamesResp.GetConfig().GetBlockedUsernames(), blockedUsernames)
	}
	effectiveBlockedUsernames, err := env.core.ConfigManager().GetEffectiveBlockedUsernames(env.ctx)
	if err != nil {
		t.Fatalf("GetEffectiveBlockedUsernames: %v", err)
	}
	if effectiveBlockedUsernames != blockedUsernames {
		t.Fatalf("effective blocked usernames = %q, want %q", effectiveBlockedUsernames, blockedUsernames)
	}

	sendWireRequest(t, conn, "system-info-frame", "system-info", "/"+wireMethodGetAdminSystemInfo, &apiv1.GetAdminSystemInfoRequest{})
	systemInfoWireResp := readWireResponse(t, conn, "system-info", 5*time.Second)
	var systemInfo apiv1.GetAdminSystemInfoResponse
	if err := proto.Unmarshal(systemInfoWireResp.GetBody(), &systemInfo); err != nil {
		t.Fatalf("unmarshal GetAdminSystemInfo response: %v", err)
	}
	if systemInfo.GetSystemInfo().GetConnection() == nil {
		t.Fatal("system info response connection is nil")
	}
	if systemInfo.GetSystemInfo().GetNats() == nil {
		t.Fatal("system info response NATS stats are nil")
	}
	if len(systemInfo.GetProjections()) == 0 {
		t.Fatal("system info response should include projection states")
	}

	sendWireRequest(t, conn, "event-log-frame", "event-log", "/"+wireMethodListAdminEventLog, &apiv1.ListAdminEventLogRequest{
		Limit: 5,
	})
	eventLogWireResp := readWireResponse(t, conn, "event-log", 5*time.Second)
	var eventLog apiv1.ListAdminEventLogResponse
	if err := proto.Unmarshal(eventLogWireResp.GetBody(), &eventLog); err != nil {
		t.Fatalf("unmarshal ListAdminEventLog response: %v", err)
	}
	if len(eventLog.GetEntries()) == 0 {
		t.Fatal("event log response should include entries")
	}
	if eventLog.GetTotalCount() <= 0 {
		t.Fatal("event log response total_count should be positive")
	}

	eventLogSeq := eventLog.GetEntries()[0].GetSequence()
	sendWireRequest(t, conn, "event-log-entry-frame", "event-log-entry", "/"+wireMethodGetAdminEventLogEntry, &apiv1.GetAdminEventLogEntryRequest{
		Sequence: eventLogSeq,
	})
	eventLogEntryWireResp := readWireResponse(t, conn, "event-log-entry", 5*time.Second)
	var eventLogEntry apiv1.GetAdminEventLogEntryResponse
	if err := proto.Unmarshal(eventLogEntryWireResp.GetBody(), &eventLogEntry); err != nil {
		t.Fatalf("unmarshal GetAdminEventLogEntry response: %v", err)
	}
	if eventLogEntry.GetEntry().GetSequence() != eventLogSeq {
		t.Fatalf("event log entry sequence = %q, want %q", eventLogEntry.GetEntry().GetSequence(), eventLogSeq)
	}
	if eventLogEntry.GetEntry().GetPayloadJson() == "" {
		t.Fatal("event log entry payload_json is empty")
	}

	sendWireRequest(t, conn, "request-delete-frame", "request-delete", "/"+wireMethodRequestAccountDeletion, &apiv1.RequestAccountDeletionRequest{})
	tokenWireResp := readWireResponse(t, conn, "request-delete", 5*time.Second)
	var token apiv1.RequestAccountDeletionResponse
	if err := proto.Unmarshal(tokenWireResp.GetBody(), &token); err != nil {
		t.Fatalf("unmarshal RequestAccountDeletion response: %v", err)
	}
	if token.GetConfirmationToken() == "" {
		t.Fatal("account deletion token is empty")
	}

	sendWireRequest(t, conn, "delete-account-frame", "delete-account", "/"+wireMethodDeleteMyAccount, &apiv1.DeleteMyAccountRequest{
		ConfirmationToken: token.GetConfirmationToken(),
	})
	deleteWireResp := readWireResponse(t, conn, "delete-account", 5*time.Second)
	var deleted apiv1.DeleteMyAccountResponse
	if err := proto.Unmarshal(deleteWireResp.GetBody(), &deleted); err != nil {
		t.Fatalf("unmarshal DeleteMyAccount response: %v", err)
	}
	if !deleted.GetDeleted() {
		t.Fatal("DeleteMyAccount should return deleted=true")
	}
}

func TestWireAPI_AdminRoomLayoutMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wireadminrooms", "Wire Admin Rooms", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, user.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-admin-room", "room over wire")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	env.login(t, "wireadminrooms", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "admin-layout-frame", "admin-layout", "/"+wireMethodGetAdminRoomLayout, &apiv1.GetAdminRoomLayoutRequest{})
	layoutWireResp := readWireResponse(t, conn, "admin-layout", 5*time.Second)
	var layout apiv1.GetAdminRoomLayoutResponse
	if err := proto.Unmarshal(layoutWireResp.GetBody(), &layout); err != nil {
		t.Fatalf("unmarshal GetAdminRoomLayout response: %v", err)
	}
	if len(layout.GetGroups()) == 0 {
		t.Fatal("admin room layout response should include at least the default group")
	}

	sendWireRequest(t, conn, "empty-group-frame", "empty-group", "/"+wireMethodCreateAdminRoomGroup, &apiv1.CreateAdminRoomGroupRequest{
		Name: "Empty Wire Group",
	})
	emptyGroupWireResp := readWireResponse(t, conn, "empty-group", 5*time.Second)
	var emptyGroup apiv1.CreateAdminRoomGroupResponse
	if err := proto.Unmarshal(emptyGroupWireResp.GetBody(), &emptyGroup); err != nil {
		t.Fatalf("unmarshal CreateAdminRoomGroup empty response: %v", err)
	}
	if emptyGroup.GetGroup().GetId() == "" {
		t.Fatal("created empty group id is empty")
	}

	sendWireRequest(t, conn, "delete-empty-group-frame", "delete-empty-group", "/"+wireMethodDeleteAdminRoomGroup, &apiv1.DeleteAdminRoomGroupRequest{
		GroupId: emptyGroup.GetGroup().GetId(),
	})
	deleteEmptyWireResp := readWireResponse(t, conn, "delete-empty-group", 5*time.Second)
	var deleteEmpty apiv1.DeleteAdminRoomGroupResponse
	if err := proto.Unmarshal(deleteEmptyWireResp.GetBody(), &deleteEmpty); err != nil {
		t.Fatalf("unmarshal DeleteAdminRoomGroup response: %v", err)
	}
	if !deleteEmpty.GetDeleted() {
		t.Fatal("DeleteAdminRoomGroup should return deleted=true")
	}

	sendWireRequest(t, conn, "layout-group-frame", "layout-group", "/"+wireMethodCreateAdminRoomGroup, &apiv1.CreateAdminRoomGroupRequest{
		Name:        "Wire Layout Group",
		Description: "managed over wire",
	})
	groupWireResp := readWireResponse(t, conn, "layout-group", 5*time.Second)
	var createdGroup apiv1.CreateAdminRoomGroupResponse
	if err := proto.Unmarshal(groupWireResp.GetBody(), &createdGroup); err != nil {
		t.Fatalf("unmarshal CreateAdminRoomGroup response: %v", err)
	}
	groupID := createdGroup.GetGroup().GetId()
	if groupID == "" {
		t.Fatal("created group id is empty")
	}

	sendWireRequest(t, conn, "update-group-frame", "update-group", "/"+wireMethodUpdateAdminRoomGroup, &apiv1.UpdateAdminRoomGroupRequest{
		GroupId:     groupID,
		Name:        "Wire Layout Renamed",
		Description: "renamed over wire",
	})
	updateGroupWireResp := readWireResponse(t, conn, "update-group", 5*time.Second)
	var updatedGroup apiv1.UpdateAdminRoomGroupResponse
	if err := proto.Unmarshal(updateGroupWireResp.GetBody(), &updatedGroup); err != nil {
		t.Fatalf("unmarshal UpdateAdminRoomGroup response: %v", err)
	}
	if updatedGroup.GetGroup().GetName() != "Wire Layout Renamed" {
		t.Fatalf("updated group name = %q, want Wire Layout Renamed", updatedGroup.GetGroup().GetName())
	}

	sendWireRequest(t, conn, "move-room-frame", "move-room", "/"+wireMethodMoveAdminRoomToGroup, &apiv1.MoveAdminRoomToGroupRequest{
		RoomId:  room.Id,
		GroupId: groupID,
	})
	moveWireResp := readWireResponse(t, conn, "move-room", 5*time.Second)
	var moved apiv1.MoveAdminRoomToGroupResponse
	if err := proto.Unmarshal(moveWireResp.GetBody(), &moved); err != nil {
		t.Fatalf("unmarshal MoveAdminRoomToGroup response: %v", err)
	}
	if moved.GetRoom().GetId() != room.Id {
		t.Fatalf("moved room id = %q, want %q", moved.GetRoom().GetId(), room.Id)
	}

	sendWireRequest(t, conn, "reorder-rooms-frame", "reorder-rooms", "/"+wireMethodReorderAdminRooms, &apiv1.ReorderAdminRoomsInGroupRequest{
		GroupId:        groupID,
		OrderedRoomIds: []string{room.Id},
	})
	reorderRoomsWireResp := readWireResponse(t, conn, "reorder-rooms", 5*time.Second)
	var reorderedRooms apiv1.ReorderAdminRoomsInGroupResponse
	if err := proto.Unmarshal(reorderRoomsWireResp.GetBody(), &reorderedRooms); err != nil {
		t.Fatalf("unmarshal ReorderAdminRoomsInGroup response: %v", err)
	}
	if reorderedRooms.GetGroup().GetId() != groupID {
		t.Fatalf("reordered group id = %q, want %q", reorderedRooms.GetGroup().GetId(), groupID)
	}

	sendWireRequest(t, conn, "update-room-frame", "update-room", "/"+wireMethodUpdateAdminRoom, &apiv1.UpdateAdminRoomRequest{
		RoomId:      room.Id,
		Name:        "wire-admin-room-renamed",
		Description: "renamed over wire",
	})
	updateRoomWireResp := readWireResponse(t, conn, "update-room", 5*time.Second)
	var updatedRoom apiv1.UpdateAdminRoomResponse
	if err := proto.Unmarshal(updateRoomWireResp.GetBody(), &updatedRoom); err != nil {
		t.Fatalf("unmarshal UpdateAdminRoom response: %v", err)
	}
	if updatedRoom.GetRoom().GetName() != "wire-admin-room-renamed" {
		t.Fatalf("updated room name = %q, want wire-admin-room-renamed", updatedRoom.GetRoom().GetName())
	}

	sendWireRequest(t, conn, "archive-room-frame", "archive-room", "/"+wireMethodArchiveAdminRoom, &apiv1.ArchiveAdminRoomRequest{
		RoomId: room.Id,
	})
	archiveWireResp := readWireResponse(t, conn, "archive-room", 5*time.Second)
	var archived apiv1.ArchiveAdminRoomResponse
	if err := proto.Unmarshal(archiveWireResp.GetBody(), &archived); err != nil {
		t.Fatalf("unmarshal ArchiveAdminRoom response: %v", err)
	}
	if !archived.GetRoom().GetArchived() {
		t.Fatal("ArchiveAdminRoom response should mark room archived")
	}

	sendWireRequest(t, conn, "unarchive-room-frame", "unarchive-room", "/"+wireMethodUnarchiveAdminRoom, &apiv1.UnarchiveAdminRoomRequest{
		RoomId: room.Id,
	})
	unarchiveWireResp := readWireResponse(t, conn, "unarchive-room", 5*time.Second)
	var unarchived apiv1.UnarchiveAdminRoomResponse
	if err := proto.Unmarshal(unarchiveWireResp.GetBody(), &unarchived); err != nil {
		t.Fatalf("unmarshal UnarchiveAdminRoom response: %v", err)
	}
	if unarchived.GetRoom().GetArchived() {
		t.Fatal("UnarchiveAdminRoom response should clear archived flag")
	}

	groups, err := env.core.ListRoomGroupsOrdered(env.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	orderedGroupIDs := make([]string, 0, len(groups))
	for i := len(groups) - 1; i >= 0; i-- {
		orderedGroupIDs = append(orderedGroupIDs, groups[i].GetId())
	}
	sendWireRequest(t, conn, "reorder-groups-frame", "reorder-groups", "/"+wireMethodReorderAdminRoomGroups, &apiv1.ReorderAdminRoomGroupsRequest{
		OrderedGroupIds: orderedGroupIDs,
	})
	reorderGroupsWireResp := readWireResponse(t, conn, "reorder-groups", 5*time.Second)
	var reorderedGroups apiv1.ReorderAdminRoomGroupsResponse
	if err := proto.Unmarshal(reorderGroupsWireResp.GetBody(), &reorderedGroups); err != nil {
		t.Fatalf("unmarshal ReorderAdminRoomGroups response: %v", err)
	}
	if len(reorderedGroups.GetGroups()) != len(groups) {
		t.Fatalf("reordered group count = %d, want %d", len(reorderedGroups.GetGroups()), len(groups))
	}
}

func TestWireAPI_UserPreferenceMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	if _, err := env.core.CreateUser(env.ctx, "system", "wireprefs", "Wire Prefs", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	env.login(t, "wireprefs", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "settings-frame", "settings", "/"+wireMethodGetUserSettings, &apiv1.GetUserSettingsRequest{})
	settingsWireResp := readWireResponse(t, conn, "settings", 5*time.Second)
	var settingsResp apiv1.GetUserSettingsResponse
	if err := proto.Unmarshal(settingsWireResp.GetBody(), &settingsResp); err != nil {
		t.Fatalf("unmarshal GetUserSettings response: %v", err)
	}
	if settingsResp.GetSettings().GetTimezone() != "" {
		t.Fatalf("initial timezone = %q, want empty", settingsResp.GetSettings().GetTimezone())
	}

	tz := "Europe/Berlin"
	timeFormat := corev1.TimeFormat_TIME_FORMAT_24H
	sendWireRequest(t, conn, "update-settings-frame", "update-settings", "/"+wireMethodUpdateUserSettings, &apiv1.UpdateUserSettingsRequest{
		Timezone:   &tz,
		TimeFormat: &timeFormat,
	})
	updateWireResp := readWireResponse(t, conn, "update-settings", 5*time.Second)
	var updateResp apiv1.UpdateUserSettingsResponse
	if err := proto.Unmarshal(updateWireResp.GetBody(), &updateResp); err != nil {
		t.Fatalf("unmarshal UpdateUserSettings response: %v", err)
	}
	if updateResp.GetSettings().GetTimezone() != tz {
		t.Fatalf("updated timezone = %q, want %q", updateResp.GetSettings().GetTimezone(), tz)
	}
	if updateResp.GetSettings().GetTimeFormat() != corev1.TimeFormat_TIME_FORMAT_24H {
		t.Fatalf("updated time format = %v, want 24H", updateResp.GetSettings().GetTimeFormat())
	}

	badTZ := "Not/AZone"
	sendWireRequest(t, conn, "bad-settings-frame", "bad-settings", "/"+wireMethodUpdateUserSettings, &apiv1.UpdateUserSettingsRequest{
		Timezone: &badTZ,
	})
	errFrame := readWireError(t, conn, "bad-settings", 5*time.Second)
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("bad timezone code = %v, want INVALID_ARGUMENT", errFrame.GetCode())
	}
}

func TestWireAPI_NotificationPreferenceMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirenotify", "Wire Notify", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-notify", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	env.login(t, "wirenotify", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "server-notification-frame", "server-notification", "/"+wireMethodSetServerNotification, &apiv1.SetServerNotificationLevelRequest{
		Level: corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	})
	serverWireResp := readWireResponse(t, conn, "server-notification", 5*time.Second)
	var serverResp apiv1.SetNotificationLevelResponse
	if err := proto.Unmarshal(serverWireResp.GetBody(), &serverResp); err != nil {
		t.Fatalf("unmarshal SetServerNotificationLevel response: %v", err)
	}
	if serverResp.GetPreference().GetLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		t.Fatalf("server level = %v, want MUTED", serverResp.GetPreference().GetLevel())
	}
	if serverResp.GetPreference().GetEffectiveLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		t.Fatalf("server effective level = %v, want MUTED", serverResp.GetPreference().GetEffectiveLevel())
	}

	sendWireRequest(t, conn, "room-notification-frame", "room-notification", "/"+wireMethodSetRoomNotification, &apiv1.SetRoomNotificationLevelRequest{
		RoomId: room.Id,
		Level:  corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES,
	})
	roomWireResp := readWireResponse(t, conn, "room-notification", 5*time.Second)
	var roomResp apiv1.SetNotificationLevelResponse
	if err := proto.Unmarshal(roomWireResp.GetBody(), &roomResp); err != nil {
		t.Fatalf("unmarshal SetRoomNotificationLevel response: %v", err)
	}
	if roomResp.GetPreference().GetLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("room level = %v, want ALL_MESSAGES", roomResp.GetPreference().GetLevel())
	}
	if roomResp.GetPreference().GetEffectiveLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("room effective level = %v, want ALL_MESSAGES", roomResp.GetPreference().GetEffectiveLevel())
	}
}

func TestWireAPI_UpdateMyPresence(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirepresence", "Wire Presence", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	env.login(t, "wirepresence", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "presence-frame", "presence", "/"+wireMethodUpdateMyPresence, &apiv1.UpdateMyPresenceRequest{
		Status: corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY,
	})
	presenceWireResp := readWireResponse(t, conn, "presence", 5*time.Second)
	var presenceResp apiv1.UpdateMyPresenceResponse
	if err := proto.Unmarshal(presenceWireResp.GetBody(), &presenceResp); err != nil {
		t.Fatalf("unmarshal UpdateMyPresence response: %v", err)
	}
	if !presenceResp.GetUpdated() {
		t.Fatal("UpdateMyPresence updated = false, want true")
	}
	status, err := env.core.GetUserPresence(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserPresence: %v", err)
	}
	if status != core.PresenceStatusAway {
		t.Fatalf("presence status = %q, want %q", status, core.PresenceStatusAway)
	}

	sendWireRequest(t, conn, "presence-bad-frame", "presence-bad", "/"+wireMethodUpdateMyPresence, &apiv1.UpdateMyPresenceRequest{})
	errFrame := readWireError(t, conn, "presence-bad", 5*time.Second)
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("bad presence code = %v, want INVALID_ARGUMENT", errFrame.GetCode())
	}
}

func TestWireAPI_PushSubscriptionMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.config.Push = config.PushConfig{
		Enabled:         true,
		VAPIDPublicKey:  "test-public-key",
		VAPIDPrivateKey: "test-private-key",
		VAPIDSubject:    "mailto:test@example.com",
	}

	user, err := env.core.CreateUser(env.ctx, "system", "wirepush", "Wire Push", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	env.login(t, "wirepush", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	endpoint := "https://push.example.com/subscription/wire"
	sendWireRequest(t, conn, "push-subscribe-frame", "push-subscribe", "/"+wireMethodSubscribeToPush, &apiv1.SubscribeToPushRequest{
		Endpoint:  endpoint,
		P256Dh:    "client-public-key",
		Auth:      "auth-secret",
		UserAgent: "test-browser",
	})
	subscribeWireResp := readWireResponse(t, conn, "push-subscribe", 5*time.Second)
	var subscribeResp apiv1.SubscribeToPushResponse
	if err := proto.Unmarshal(subscribeWireResp.GetBody(), &subscribeResp); err != nil {
		t.Fatalf("unmarshal SubscribeToPush response: %v", err)
	}
	if !subscribeResp.GetSubscribed() {
		t.Fatal("SubscribeToPush subscribed = false, want true")
	}
	subs, err := env.core.GetUserPushSubscriptions(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].GetEndpoint() != endpoint || subs[0].GetUserAgent() != "test-browser" {
		t.Fatalf("stored subscriptions = %#v, want one matching subscription", subs)
	}

	sendWireRequest(t, conn, "push-unsubscribe-frame", "push-unsubscribe", "/"+wireMethodUnsubscribeFromPush, &apiv1.UnsubscribeFromPushRequest{
		Endpoint: endpoint,
	})
	unsubscribeWireResp := readWireResponse(t, conn, "push-unsubscribe", 5*time.Second)
	var unsubscribeResp apiv1.UnsubscribeFromPushResponse
	if err := proto.Unmarshal(unsubscribeWireResp.GetBody(), &unsubscribeResp); err != nil {
		t.Fatalf("unmarshal UnsubscribeFromPush response: %v", err)
	}
	if !unsubscribeResp.GetUnsubscribed() {
		t.Fatal("UnsubscribeFromPush unsubscribed = false, want true")
	}
	subs, err = env.core.GetUserPushSubscriptions(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions after unsubscribe: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions after unsubscribe = %d, want 0", len(subs))
	}

	sendWireRequest(t, conn, "push-bad-frame", "push-bad", "/"+wireMethodSubscribeToPush, &apiv1.SubscribeToPushRequest{
		Endpoint: endpoint,
		P256Dh:   "client-public-key",
	})
	errFrame := readWireError(t, conn, "push-bad", 5*time.Second)
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("bad push code = %v, want INVALID_ARGUMENT", errFrame.GetCode())
	}
}

func TestWireAPI_NotificationMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	alice, err := env.core.CreateUser(env.ctx, "system", "wirenotifications", "Wire Notifications", "password123")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := env.core.CreateUser(env.ctx, "system", "wirenotifyactor", "Wire Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, alice.Id, core.KindChannel, "", "wire-notifications", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, alice.Id, core.KindChannel, alice.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom alice: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, alice.Id, core.KindChannel, bob.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom bob: %v", err)
	}

	created, err := env.core.CreateNotification(env.ctx, alice.Id, bob.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{
				RoomId:   room.Id,
				EventId:  "event-mention",
				InThread: "thread-root",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	env.login(t, "wirenotifications", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "notifications-list-frame", "notifications-list", "/"+wireMethodListNotifications, &apiv1.ListNotificationsRequest{
		Limit: 10,
	})
	listWireResp := readWireResponse(t, conn, "notifications-list", 5*time.Second)
	var listResp apiv1.ListNotificationsResponse
	if err := proto.Unmarshal(listWireResp.GetBody(), &listResp); err != nil {
		t.Fatalf("unmarshal ListNotifications response: %v", err)
	}
	if listResp.GetTotalCount() != 1 || len(listResp.GetItems()) != 1 {
		t.Fatalf("notification count = total %d len %d, want 1/1", listResp.GetTotalCount(), len(listResp.GetItems()))
	}
	item := listResp.GetItems()[0]
	if item.GetId() != created.GetId() {
		t.Fatalf("notification id = %q, want %q", item.GetId(), created.GetId())
	}
	if item.GetKind() != apiv1.NotificationKind_NOTIFICATION_KIND_MENTION {
		t.Fatalf("notification kind = %v, want MENTION", item.GetKind())
	}
	if item.GetActor().GetId() != bob.Id {
		t.Fatalf("notification actor = %q, want %q", item.GetActor().GetId(), bob.Id)
	}
	if item.GetSummary() != "Wire Actor mentioned you" {
		t.Fatalf("notification summary = %q, want Wire Actor mentioned you", item.GetSummary())
	}
	if item.GetRoomId() != room.Id || item.GetRoomName() != room.Name {
		t.Fatalf("notification room = %q/%q, want %q/%q", item.GetRoomId(), item.GetRoomName(), room.Id, room.Name)
	}
	if item.GetEventId() != "event-mention" || item.GetThreadRootEventId() != "thread-root" {
		t.Fatalf("notification event/thread = %q/%q, want event-mention/thread-root", item.GetEventId(), item.GetThreadRootEventId())
	}
	if listResp.GetServerName() == "" {
		t.Fatal("ListNotifications server_name is empty")
	}

	sendWireRequest(t, conn, "notifications-has-frame", "notifications-has", "/"+wireMethodHasNotifications, &apiv1.HasNotificationsRequest{})
	hasWireResp := readWireResponse(t, conn, "notifications-has", 5*time.Second)
	var hasResp apiv1.HasNotificationsResponse
	if err := proto.Unmarshal(hasWireResp.GetBody(), &hasResp); err != nil {
		t.Fatalf("unmarshal HasNotifications response: %v", err)
	}
	if !hasResp.GetHasNotifications() {
		t.Fatal("HasNotifications returned false, want true")
	}

	sendWireRequest(t, conn, "notification-dismiss-frame", "notification-dismiss", "/"+wireMethodDismissNotification, &apiv1.DismissNotificationRequest{
		NotificationId: created.GetId(),
	})
	dismissWireResp := readWireResponse(t, conn, "notification-dismiss", 5*time.Second)
	var dismissResp apiv1.DismissNotificationResponse
	if err := proto.Unmarshal(dismissWireResp.GetBody(), &dismissResp); err != nil {
		t.Fatalf("unmarshal DismissNotification response: %v", err)
	}
	if !dismissResp.GetDismissed() {
		t.Fatal("DismissNotification dismissed = false, want true")
	}

	sendWireRequest(t, conn, "notification-dismiss-bad-frame", "notification-dismiss-bad", "/"+wireMethodDismissNotification, &apiv1.DismissNotificationRequest{})
	errFrame := readWireError(t, conn, "notification-dismiss-bad", 5*time.Second)
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT {
		t.Fatalf("bad dismiss code = %v, want INVALID_ARGUMENT", errFrame.GetCode())
	}

	for _, eventID := range []string{"event-one", "event-two"} {
		if _, err := env.core.CreateNotification(env.ctx, alice.Id, bob.Id, &corev1.Notification{
			Notification: &corev1.Notification_RoomMessage{
				RoomMessage: &corev1.RoomMessageNotification{
					RoomId:  room.Id,
					EventId: eventID,
				},
			},
		}); err != nil {
			t.Fatalf("CreateNotification %s: %v", eventID, err)
		}
	}

	sendWireRequest(t, conn, "notifications-clear-frame", "notifications-clear", "/"+wireMethodDismissAllNotifications, &apiv1.DismissAllNotificationsRequest{})
	clearWireResp := readWireResponse(t, conn, "notifications-clear", 5*time.Second)
	var clearResp apiv1.DismissAllNotificationsResponse
	if err := proto.Unmarshal(clearWireResp.GetBody(), &clearResp); err != nil {
		t.Fatalf("unmarshal DismissAllNotifications response: %v", err)
	}
	if clearResp.GetDismissedCount() != 2 {
		t.Fatalf("dismissed count = %d, want 2", clearResp.GetDismissedCount())
	}

	remaining, err := env.core.GetNotifications(env.ctx, alice.Id)
	if err != nil {
		t.Fatalf("GetNotifications after clear: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("notifications after clear = %d, want 0", len(remaining))
	}
}

func TestWireAPI_MessageWriteMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirewrites", "Wire Writes", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-writes", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	env.login(t, "wirewrites", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	previewTitle := "Wire Preview"
	previewDescription := "A protobuf wire link preview"
	previewSiteName := "Example"
	sendWireRequest(t, conn, "post-frame", "post", "/chatto.api.v1.ChattoApiService/PostMessage", &apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "wire write original",
		LinkPreview: &apiv1.LinkPreviewInput{
			Url:         "https://example.com/wire-preview",
			Title:       &previewTitle,
			Description: &previewDescription,
			SiteName:    &previewSiteName,
		},
	})
	postWireResp := readWireResponse(t, conn, "post", 5*time.Second)
	var postResp apiv1.PostMessageResponse
	if err := proto.Unmarshal(postWireResp.GetBody(), &postResp); err != nil {
		t.Fatalf("unmarshal post response: %v", err)
	}
	messageEventID := postResp.GetEvent().GetId()
	if messageEventID == "" {
		t.Fatal("PostMessage response event id is empty")
	}

	sendWireRequest(t, conn, "edit-frame", "edit", "/chatto.api.v1.ChattoApiService/UpdateMessage", &apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: messageEventID,
		Body:    "wire write edited",
	})
	editWireResp := readWireResponse(t, conn, "edit", 5*time.Second)
	var editResp apiv1.UpdateMessageResponse
	if err := proto.Unmarshal(editWireResp.GetBody(), &editResp); err != nil {
		t.Fatalf("unmarshal edit response: %v", err)
	}
	body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, messageEventID)
	if err != nil {
		t.Fatalf("GetMessageBody after edit: %v", err)
	}
	if body != "wire write edited" {
		t.Fatalf("message body after edit = %q, want wire write edited", body)
	}

	sendWireRequest(t, conn, "delete-link-preview-frame", "delete-link-preview", "/chatto.api.v1.ChattoApiService/DeleteLinkPreview", &apiv1.DeleteLinkPreviewRequest{
		RoomId:  room.Id,
		EventId: messageEventID,
		Url:     "https://example.com/wire-preview",
	})
	deleteLinkPreviewWireResp := readWireResponse(t, conn, "delete-link-preview", 5*time.Second)
	var deleteLinkPreviewResp apiv1.DeleteLinkPreviewResponse
	if err := proto.Unmarshal(deleteLinkPreviewWireResp.GetBody(), &deleteLinkPreviewResp); err != nil {
		t.Fatalf("unmarshal delete link preview response: %v", err)
	}
	messageBody, err := env.core.GetFullMessageBody(env.ctx, core.KindChannel, messageEventID)
	if err != nil {
		t.Fatalf("GetFullMessageBody after link preview delete: %v", err)
	}
	if messageBody.LinkPreview != nil {
		t.Fatal("link preview should be removed after DeleteLinkPreview")
	}

	sendWireRequest(t, conn, "add-reaction-frame", "add-reaction", "/chatto.api.v1.ChattoApiService/AddReaction", &apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: messageEventID,
		Emoji:          "thumbsup",
	})
	addReactionWireResp := readWireResponse(t, conn, "add-reaction", 5*time.Second)
	var addReactionResp apiv1.AddReactionResponse
	if err := proto.Unmarshal(addReactionWireResp.GetBody(), &addReactionResp); err != nil {
		t.Fatalf("unmarshal add reaction response: %v", err)
	}
	if !addReactionResp.GetChanged() {
		t.Fatal("AddReaction changed = false, want true")
	}

	sendWireRequest(t, conn, "remove-reaction-frame", "remove-reaction", "/chatto.api.v1.ChattoApiService/RemoveReaction", &apiv1.RemoveReactionRequest{
		RoomId:         room.Id,
		MessageEventId: messageEventID,
		Emoji:          "thumbsup",
	})
	removeReactionWireResp := readWireResponse(t, conn, "remove-reaction", 5*time.Second)
	var removeReactionResp apiv1.RemoveReactionResponse
	if err := proto.Unmarshal(removeReactionWireResp.GetBody(), &removeReactionResp); err != nil {
		t.Fatalf("unmarshal remove reaction response: %v", err)
	}
	if !removeReactionResp.GetChanged() {
		t.Fatal("RemoveReaction changed = false, want true")
	}

	attachment, err := env.core.UploadAttachment(env.ctx, core.SystemActorID, room.Id, "wire.png", "image/png", bytes.NewReader(createTestPNG(t, 64, 64)))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	sendWireRequest(t, conn, "post-attachment-frame", "post-attachment", "/chatto.api.v1.ChattoApiService/PostMessage", &apiv1.PostMessageRequest{
		RoomId:             room.Id,
		Body:               "wire attachment",
		AttachmentAssetIds: []string{attachment.Id},
	})
	postAttachmentWireResp := readWireResponse(t, conn, "post-attachment", 5*time.Second)
	var postAttachmentResp apiv1.PostMessageResponse
	if err := proto.Unmarshal(postAttachmentWireResp.GetBody(), &postAttachmentResp); err != nil {
		t.Fatalf("unmarshal attachment post response: %v", err)
	}
	attachmentMessageEventID := postAttachmentResp.GetEvent().GetId()
	if attachmentMessageEventID == "" {
		t.Fatal("attachment PostMessage response event id is empty")
	}

	sendWireRequest(t, conn, "delete-attachment-frame", "delete-attachment", "/chatto.api.v1.ChattoApiService/DeleteAttachment", &apiv1.DeleteAttachmentRequest{
		RoomId:       room.Id,
		EventId:      attachmentMessageEventID,
		AttachmentId: attachment.Id,
	})
	deleteAttachmentWireResp := readWireResponse(t, conn, "delete-attachment", 5*time.Second)
	var deleteAttachmentResp apiv1.DeleteAttachmentResponse
	if err := proto.Unmarshal(deleteAttachmentWireResp.GetBody(), &deleteAttachmentResp); err != nil {
		t.Fatalf("unmarshal delete attachment response: %v", err)
	}
	attachmentMessageBody, err := env.core.GetFullMessageBody(env.ctx, core.KindChannel, attachmentMessageEventID)
	if err != nil {
		t.Fatalf("GetFullMessageBody after attachment delete: %v", err)
	}
	if len(attachmentMessageBody.Attachments) != 0 {
		t.Fatal("attachment should be removed from message body after DeleteAttachment")
	}

	sendWireRequest(t, conn, "delete-frame", "delete", "/chatto.api.v1.ChattoApiService/DeleteMessage", &apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: messageEventID,
	})
	deleteWireResp := readWireResponse(t, conn, "delete", 5*time.Second)
	var deleteResp apiv1.DeleteMessageResponse
	if err := proto.Unmarshal(deleteWireResp.GetBody(), &deleteResp); err != nil {
		t.Fatalf("unmarshal delete response: %v", err)
	}
	body, err = env.core.GetMessageBody(env.ctx, core.KindChannel, messageEventID)
	if err != nil {
		t.Fatalf("GetMessageBody after delete: %v", err)
	}
	if body != "" {
		t.Fatalf("message body after delete = %q, want empty", body)
	}

	sendWireRequest(t, conn, "leave-room-frame", "leave-room", "/chatto.api.v1.ChattoApiService/LeaveRoom", &apiv1.LeaveRoomRequest{
		RoomId: room.Id,
	})
	leaveRoomWireResp := readWireResponse(t, conn, "leave-room", 5*time.Second)
	var leaveRoomResp apiv1.LeaveRoomResponse
	if err := proto.Unmarshal(leaveRoomWireResp.GetBody(), &leaveRoomResp); err != nil {
		t.Fatalf("unmarshal leave room response: %v", err)
	}
	isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after LeaveRoom: %v", err)
	}
	if isMember {
		t.Fatal("room membership should be removed after LeaveRoom")
	}
}

func TestWireAPI_RoomMembershipMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wiremember", "Wire Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	roomOne, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-join-one", "")
	if err != nil {
		t.Fatalf("CreateRoom one: %v", err)
	}
	roomTwo, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, roomOne.GroupId, "wire-join-two", "")
	if err != nil {
		t.Fatalf("CreateRoom two: %v", err)
	}

	env.login(t, "wiremember", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "join-room-frame", "join-room", "/chatto.api.v1.ChattoApiService/JoinRoom", &apiv1.JoinRoomRequest{
		RoomId: roomOne.Id,
	})
	joinRoomWireResp := readWireResponse(t, conn, "join-room", 5*time.Second)
	var joinRoomResp apiv1.JoinRoomResponse
	if err := proto.Unmarshal(joinRoomWireResp.GetBody(), &joinRoomResp); err != nil {
		t.Fatalf("unmarshal join room response: %v", err)
	}
	if joinRoomResp.GetRoom().GetId() != roomOne.Id {
		t.Fatalf("JoinRoom response room id = %q, want %q", joinRoomResp.GetRoom().GetId(), roomOne.Id)
	}
	isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, user.Id, roomOne.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after JoinRoom: %v", err)
	}
	if !isMember {
		t.Fatal("room membership should exist after JoinRoom")
	}

	sendWireRequest(t, conn, "leave-room-frame", "leave-room", "/chatto.api.v1.ChattoApiService/LeaveRoom", &apiv1.LeaveRoomRequest{
		RoomId: roomOne.Id,
	})
	leaveRoomWireResp := readWireResponse(t, conn, "leave-room", 5*time.Second)
	var leaveRoomResp apiv1.LeaveRoomResponse
	if err := proto.Unmarshal(leaveRoomWireResp.GetBody(), &leaveRoomResp); err != nil {
		t.Fatalf("unmarshal leave room response: %v", err)
	}
	isMember, err = env.core.RoomMembershipExists(env.ctx, core.KindChannel, user.Id, roomOne.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after LeaveRoom: %v", err)
	}
	if isMember {
		t.Fatal("room membership should be removed after LeaveRoom")
	}

	sendWireRequest(t, conn, "join-group-frame", "join-group", "/chatto.api.v1.ChattoApiService/JoinGroup", &apiv1.JoinGroupRequest{
		GroupId: roomOne.GroupId,
	})
	joinGroupWireResp := readWireResponse(t, conn, "join-group", 5*time.Second)
	var joinGroupResp apiv1.JoinGroupResponse
	if err := proto.Unmarshal(joinGroupWireResp.GetBody(), &joinGroupResp); err != nil {
		t.Fatalf("unmarshal join group response: %v", err)
	}
	if !containsString(joinGroupResp.GetJoinedRoomIds(), roomOne.Id) {
		t.Fatalf("JoinGroup joined ids = %v, want %q", joinGroupResp.GetJoinedRoomIds(), roomOne.Id)
	}
	if !containsString(joinGroupResp.GetJoinedRoomIds(), roomTwo.Id) {
		t.Fatalf("JoinGroup joined ids = %v, want %q", joinGroupResp.GetJoinedRoomIds(), roomTwo.Id)
	}
	for _, roomID := range []string{roomOne.Id, roomTwo.Id} {
		isMember, err = env.core.RoomMembershipExists(env.ctx, core.KindChannel, user.Id, roomID)
		if err != nil {
			t.Fatalf("RoomMembershipExists after JoinGroup for %s: %v", roomID, err)
		}
		if !isMember {
			t.Fatalf("room membership for %s should exist after JoinGroup", roomID)
		}
	}
}

func TestWireAPI_RoomModerationMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	admin, err := env.core.CreateUser(env.ctx, "system", "wiremoderator", "Wire Moderator", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "wirebantarget", "Wire Ban Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, admin.Id, core.KindChannel, "", "wire-moderation", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, target.Id, core.KindChannel, target.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom target: %v", err)
	}

	env.login(t, "wiremoderator", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	expiresAt := time.Now().Add(24 * time.Hour)
	sendWireRequest(t, conn, "ban-room-member-frame", "ban-room-member", "/chatto.api.v1.ChattoApiService/BanRoomMember", &apiv1.BanRoomMemberRequest{
		RoomId:    room.Id,
		UserId:    target.Id,
		Reason:    "spam",
		ExpiresAt: timestamppb.New(expiresAt),
	})
	banRespFrame := readWireResponse(t, conn, "ban-room-member", 5*time.Second)
	var banResp apiv1.BanRoomMemberResponse
	if err := proto.Unmarshal(banRespFrame.GetBody(), &banResp); err != nil {
		t.Fatalf("unmarshal ban response: %v", err)
	}

	isMember, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, target.Id, room.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists after BanRoomMember: %v", err)
	}
	if isMember {
		t.Fatal("target membership should be removed after BanRoomMember")
	}

	ban, ok := env.core.RoomBans.ActiveBan(room.Id, target.Id, time.Now())
	if !ok {
		t.Fatal("target should have an active room ban")
	}
	if ban.ModeratorID != admin.Id || ban.Reason != "spam" {
		t.Fatalf("unexpected ban projection: moderator=%q reason=%q", ban.ModeratorID, ban.Reason)
	}
	if ban.ExpiresAt == nil || !ban.ExpiresAt.After(time.Now()) {
		t.Fatalf("unexpected ban expiry: %v", ban.ExpiresAt)
	}

	sendWireRequest(t, conn, "list-room-bans-frame", "list-room-bans", "/"+wireMethodListRoomBans, &apiv1.ListRoomBansRequest{})
	listRespFrame := readWireResponse(t, conn, "list-room-bans", 5*time.Second)
	var listResp apiv1.ListRoomBansResponse
	if err := proto.Unmarshal(listRespFrame.GetBody(), &listResp); err != nil {
		t.Fatalf("unmarshal list room bans response: %v", err)
	}
	if len(listResp.GetBans()) != 1 {
		t.Fatalf("ListRoomBans returned %d bans, want 1", len(listResp.GetBans()))
	}
	listedBan := listResp.GetBans()[0]
	if listedBan.GetId() != ban.EventID || listedBan.GetRoomId() != room.Id || listedBan.GetUserId() != target.Id {
		t.Fatalf("unexpected listed ban ids: id=%q room=%q user=%q", listedBan.GetId(), listedBan.GetRoomId(), listedBan.GetUserId())
	}
	if listedBan.GetRoom().GetName() != room.GetName() {
		t.Fatalf("listed ban room name = %q, want %q", listedBan.GetRoom().GetName(), room.GetName())
	}
	if listedBan.GetUser().GetUser().GetLogin() != target.GetLogin() {
		t.Fatalf("listed ban user login = %q, want %q", listedBan.GetUser().GetUser().GetLogin(), target.GetLogin())
	}
	if listedBan.GetExpiresAt() == nil {
		t.Fatal("listed ban expiry is nil")
	}

	sendWireRequest(t, conn, "unban-room-member-frame", "unban-room-member", "/"+wireMethodUnbanRoomMember, &apiv1.UnbanRoomMemberRequest{
		RoomId: room.Id,
		UserId: target.Id,
		Reason: "appeal accepted",
	})
	unbanRespFrame := readWireResponse(t, conn, "unban-room-member", 5*time.Second)
	var unbanResp apiv1.UnbanRoomMemberResponse
	if err := proto.Unmarshal(unbanRespFrame.GetBody(), &unbanResp); err != nil {
		t.Fatalf("unmarshal unban response: %v", err)
	}
	if !unbanResp.GetUnbanned() {
		t.Fatal("UnbanRoomMember returned unbanned=false")
	}
	if _, ok := env.core.RoomBans.ActiveBan(room.Id, target.Id, time.Now()); ok {
		t.Fatal("target should not have an active room ban after UnbanRoomMember")
	}

	sendWireRequest(t, conn, "list-room-bans-after-frame", "list-room-bans-after", "/"+wireMethodListRoomBans, &apiv1.ListRoomBansRequest{
		RoomId: room.Id,
	})
	listAfterFrame := readWireResponse(t, conn, "list-room-bans-after", 5*time.Second)
	var listAfter apiv1.ListRoomBansResponse
	if err := proto.Unmarshal(listAfterFrame.GetBody(), &listAfter); err != nil {
		t.Fatalf("unmarshal list room bans after response: %v", err)
	}
	if len(listAfter.GetBans()) != 0 {
		t.Fatalf("ListRoomBans after unban returned %d bans, want 0", len(listAfter.GetBans()))
	}
}

func TestWireAPI_VoiceCallMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)
	env.httpServer.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "wire-test-server",
	}

	user, err := env.core.CreateUser(env.ctx, "system", "wirevoice", "Wire Voice", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-voice", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	env.login(t, "wirevoice", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "list-active-calls-before-frame", "list-active-calls-before", "/chatto.api.v1.ChattoApiService/ListActiveCalls", &apiv1.ListActiveCallsRequest{})
	listBeforeWireResp := readWireResponse(t, conn, "list-active-calls-before", 5*time.Second)
	var listBeforeResp apiv1.ListActiveCallsResponse
	if err := proto.Unmarshal(listBeforeWireResp.GetBody(), &listBeforeResp); err != nil {
		t.Fatalf("unmarshal ListActiveCalls before response: %v", err)
	}
	if len(listBeforeResp.GetCalls()) != 0 {
		t.Fatalf("ListActiveCalls before join = %d calls, want 0", len(listBeforeResp.GetCalls()))
	}

	sendWireRequest(t, conn, "join-voice-call-frame", "join-voice-call", "/chatto.api.v1.ChattoApiService/JoinVoiceCall", &apiv1.JoinVoiceCallRequest{
		RoomId: room.Id,
	})
	joinWireResp := readWireResponse(t, conn, "join-voice-call", 5*time.Second)
	var joinResp apiv1.JoinVoiceCallResponse
	if err := proto.Unmarshal(joinWireResp.GetBody(), &joinResp); err != nil {
		t.Fatalf("unmarshal JoinVoiceCall response: %v", err)
	}
	if !joinResp.GetJoined() {
		t.Fatal("JoinVoiceCall joined = false, want true")
	}

	sendWireRequest(t, conn, "get-call-participants-frame", "get-call-participants", "/chatto.api.v1.ChattoApiService/GetCallParticipants", &apiv1.GetCallParticipantsRequest{
		RoomId: room.Id,
	})
	participantsWireResp := readWireResponse(t, conn, "get-call-participants", 5*time.Second)
	var participantsResp apiv1.GetCallParticipantsResponse
	if err := proto.Unmarshal(participantsWireResp.GetBody(), &participantsResp); err != nil {
		t.Fatalf("unmarshal GetCallParticipants response: %v", err)
	}
	if len(participantsResp.GetParticipants()) != 1 {
		t.Fatalf("GetCallParticipants returned %d participants, want 1", len(participantsResp.GetParticipants()))
	}
	participant := participantsResp.GetParticipants()[0]
	if participant.GetUser().GetId() != user.Id {
		t.Fatalf("participant user id = %q, want %q", participant.GetUser().GetId(), user.Id)
	}
	if participant.GetCallId() == "" {
		t.Fatal("participant call id is empty")
	}
	if participant.GetJoinedAt() == nil {
		t.Fatal("participant joined_at is nil")
	}

	sendWireRequest(t, conn, "list-active-calls-frame", "list-active-calls", "/chatto.api.v1.ChattoApiService/ListActiveCalls", &apiv1.ListActiveCallsRequest{})
	listWireResp := readWireResponse(t, conn, "list-active-calls", 5*time.Second)
	var listResp apiv1.ListActiveCallsResponse
	if err := proto.Unmarshal(listWireResp.GetBody(), &listResp); err != nil {
		t.Fatalf("unmarshal ListActiveCalls response: %v", err)
	}
	if len(listResp.GetCalls()) != 1 {
		t.Fatalf("ListActiveCalls returned %d calls, want 1", len(listResp.GetCalls()))
	}
	if listResp.GetCalls()[0].GetRoomId() != room.Id {
		t.Fatalf("ListActiveCalls room id = %q, want %q", listResp.GetCalls()[0].GetRoomId(), room.Id)
	}
	if listResp.GetCalls()[0].GetCallId() != participant.GetCallId() {
		t.Fatalf("ListActiveCalls call id = %q, want %q", listResp.GetCalls()[0].GetCallId(), participant.GetCallId())
	}
	if len(listResp.GetCalls()[0].GetParticipants()) != 1 {
		t.Fatalf("ListActiveCalls participants = %d, want 1", len(listResp.GetCalls()[0].GetParticipants()))
	}

	sendWireRequest(t, conn, "get-voice-token-frame", "get-voice-token", "/chatto.api.v1.ChattoApiService/GetVoiceCallToken", &apiv1.GetVoiceCallTokenRequest{
		RoomId: room.Id,
	})
	tokenWireResp := readWireResponse(t, conn, "get-voice-token", 5*time.Second)
	var tokenResp apiv1.GetVoiceCallTokenResponse
	if err := proto.Unmarshal(tokenWireResp.GetBody(), &tokenResp); err != nil {
		t.Fatalf("unmarshal GetVoiceCallToken response: %v", err)
	}
	if tokenResp.GetToken().GetToken() == "" {
		t.Fatal("voice call token is empty")
	}
	if tokenResp.GetToken().GetE2EeKey() == "" {
		t.Fatal("voice call e2ee key is empty")
	}
	if tokenResp.GetToken().GetCallId() != participant.GetCallId() {
		t.Fatalf("voice token call id = %q, want %q", tokenResp.GetToken().GetCallId(), participant.GetCallId())
	}

	sendWireRequest(t, conn, "leave-voice-call-frame", "leave-voice-call", "/chatto.api.v1.ChattoApiService/LeaveVoiceCall", &apiv1.LeaveVoiceCallRequest{
		RoomId: room.Id,
	})
	leaveWireResp := readWireResponse(t, conn, "leave-voice-call", 5*time.Second)
	var leaveResp apiv1.LeaveVoiceCallResponse
	if err := proto.Unmarshal(leaveWireResp.GetBody(), &leaveResp); err != nil {
		t.Fatalf("unmarshal LeaveVoiceCall response: %v", err)
	}
	if !leaveResp.GetLeft() {
		t.Fatal("LeaveVoiceCall left = false, want true")
	}

	participants, err := env.core.GetCallParticipants(env.ctx, core.LegacySpaceIDForRoomKind(core.KindChannel), room.Id)
	if err != nil {
		t.Fatalf("GetCallParticipants after LeaveVoiceCall: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants after LeaveVoiceCall = %#v, want none", participants)
	}
}

func TestWireAPI_QuickSwitcherAndRoomCreateMethods(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirequick", "Wire Quick", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "wirequicktarget", "Wire Quick Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomCreate); err != nil {
		t.Fatalf("GrantServerPermission room.create: %v", err)
	}

	env.login(t, "wirequick", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "search-members-frame", "search-members", "/chatto.api.v1.ChattoApiService/SearchMembers", &apiv1.SearchMembersRequest{
		Search: "target",
		Limit:  20,
	})
	searchWireResp := readWireResponse(t, conn, "search-members", 5*time.Second)
	var searchResp apiv1.SearchMembersResponse
	if err := proto.Unmarshal(searchWireResp.GetBody(), &searchResp); err != nil {
		t.Fatalf("unmarshal SearchMembers response: %v", err)
	}
	if searchResp.GetViewerUserId() != user.Id {
		t.Fatalf("SearchMembers viewer_user_id = %q, want %q", searchResp.GetViewerUserId(), user.Id)
	}
	if !searchResp.GetViewerCanStartDms() {
		t.Fatal("SearchMembers viewer_can_start_dms = false, want true")
	}
	if !containsString(userIDs(searchResp.GetUsers()), target.Id) {
		t.Fatalf("SearchMembers users = %v, want target %q", userIDs(searchResp.GetUsers()), target.Id)
	}

	sendWireRequest(t, conn, "start-dm-frame", "start-dm", "/chatto.api.v1.ChattoApiService/StartDM", &apiv1.StartDMRequest{
		ParticipantIds: []string{target.Id},
	})
	startDMWireResp := readWireResponse(t, conn, "start-dm", 5*time.Second)
	var startDMResp apiv1.StartDMResponse
	if err := proto.Unmarshal(startDMWireResp.GetBody(), &startDMResp); err != nil {
		t.Fatalf("unmarshal StartDM response: %v", err)
	}
	if startDMResp.GetRoom().GetKind() != corev1.RoomKind_ROOM_KIND_DM {
		t.Fatalf("StartDM room kind = %s, want DM", startDMResp.GetRoom().GetKind())
	}
	if !startDMResp.GetCreated() {
		t.Fatal("StartDM created = false, want true on first call")
	}

	sendWireRequest(t, conn, "start-dm-again-frame", "start-dm-again", "/chatto.api.v1.ChattoApiService/StartDM", &apiv1.StartDMRequest{
		ParticipantIds: []string{target.Id},
	})
	startDMAgainWireResp := readWireResponse(t, conn, "start-dm-again", 5*time.Second)
	var startDMAgainResp apiv1.StartDMResponse
	if err := proto.Unmarshal(startDMAgainWireResp.GetBody(), &startDMAgainResp); err != nil {
		t.Fatalf("unmarshal second StartDM response: %v", err)
	}
	if startDMAgainResp.GetRoom().GetId() != startDMResp.GetRoom().GetId() {
		t.Fatalf("second StartDM room id = %q, want %q", startDMAgainResp.GetRoom().GetId(), startDMResp.GetRoom().GetId())
	}
	if startDMAgainResp.GetCreated() {
		t.Fatal("second StartDM created = true, want false")
	}

	sendWireRequest(t, conn, "create-room-frame", "create-room", "/chatto.api.v1.ChattoApiService/CreateRoom", &apiv1.CreateRoomRequest{
		Name:        "wire-created-room",
		Description: "created through wire",
	})
	createRoomWireResp := readWireResponse(t, conn, "create-room", 5*time.Second)
	var createRoomResp apiv1.CreateRoomResponse
	if err := proto.Unmarshal(createRoomWireResp.GetBody(), &createRoomResp); err != nil {
		t.Fatalf("unmarshal CreateRoom response: %v", err)
	}
	if createRoomResp.GetRoom().GetName() != "wire-created-room" {
		t.Fatalf("CreateRoom name = %q, want wire-created-room", createRoomResp.GetRoom().GetName())
	}
	if createRoomResp.GetRoom().GetKind() != corev1.RoomKind_ROOM_KIND_CHANNEL {
		t.Fatalf("CreateRoom kind = %s, want CHANNEL", createRoomResp.GetRoom().GetKind())
	}
	if createRoomResp.GetRoom().GetGroupId() == "" {
		t.Fatal("CreateRoom group id is empty")
	}

	sendWireRequest(t, conn, "room-directory-frame", "room-directory", "/chatto.api.v1.ChattoApiService/GetRoomDirectory", &apiv1.GetRoomDirectoryRequest{})
	directoryWireResp := readWireResponse(t, conn, "room-directory", 5*time.Second)
	var directoryResp apiv1.GetRoomDirectoryResponse
	if err := proto.Unmarshal(directoryWireResp.GetBody(), &directoryResp); err != nil {
		t.Fatalf("unmarshal GetRoomDirectory response: %v", err)
	}
	var directoryRoom *apiv1.RoomDirectoryItemView
	for _, view := range directoryResp.GetRoomViews() {
		if view.GetRoom().GetId() == createRoomResp.GetRoom().GetId() {
			directoryRoom = view
			break
		}
	}
	if directoryRoom == nil {
		t.Fatalf("GetRoomDirectory did not include created room %q", createRoomResp.GetRoom().GetId())
	}
	if !directoryRoom.GetViewerCanJoinRoom() {
		t.Fatal("GetRoomDirectory viewer_can_join_room = false, want true")
	}
}

func TestWireAPI_ListMyRoomsSidebarView(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirerooms", "Wire Rooms", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	other, err := env.core.CreateUser(env.ctx, "system", "wireroomsother", "Wire Rooms Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	channel, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-room-list", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, channel.Id); err != nil {
		t.Fatalf("JoinRoom user: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, channel.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if err := env.core.SetRoomNotificationLevel(env.ctx, user.Id, channel.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES); err != nil {
		t.Fatalf("SetRoomNotificationLevel: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, channel.Id, other.Id, "channel unread", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage channel: %v", err)
	}

	dm, _, err := env.core.FindOrCreateDM(env.ctx, user.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, other.Id, "dm visible", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage dm: %v", err)
	}

	env.login(t, "wirerooms", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "list-channels-frame", "list-channels", "/chatto.api.v1.ChattoApiService/ListMyRooms", &apiv1.ListMyRoomsRequest{
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	channelsWireResp := readWireResponse(t, conn, "list-channels", 5*time.Second)
	var channels apiv1.ListMyRoomsResponse
	if err := proto.Unmarshal(channelsWireResp.GetBody(), &channels); err != nil {
		t.Fatalf("unmarshal channel ListMyRooms response: %v", err)
	}
	if channels.GetViewerUserId() != user.Id {
		t.Fatalf("channel viewer_user_id = %q, want %q", channels.GetViewerUserId(), user.Id)
	}
	if len(channels.GetRoomViews()) != 1 {
		t.Fatalf("channel room view count = %d, want 1", len(channels.GetRoomViews()))
	}
	channelView := channels.GetRoomViews()[0]
	if channelView.GetRoom().GetId() != channel.Id {
		t.Fatalf("channel room id = %q, want %q", channelView.GetRoom().GetId(), channel.Id)
	}
	if !channelView.GetHasUnread() {
		t.Fatal("channel room view should report unread")
	}
	pref := channelView.GetViewerNotificationPreference()
	if pref.GetLevel() != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("channel notification level = %s, want ALL_MESSAGES", pref.GetLevel())
	}
	if !containsString(userIDs(channelView.GetMembers()), user.Id) || !containsString(userIDs(channelView.GetMembers()), other.Id) {
		t.Fatalf("channel members = %v, want user and other", userIDs(channelView.GetMembers()))
	}
	if len(channels.GetRoomGroups()) == 0 {
		t.Fatal("channel ListMyRooms response should include room groups")
	}
	if !containsString(channels.GetRoomGroups()[0].GetRoomIds(), channel.Id) {
		t.Fatalf("first room group room ids = %v, want %q", channels.GetRoomGroups()[0].GetRoomIds(), channel.Id)
	}

	sendWireRequest(t, conn, "list-dms-frame", "list-dms", "/chatto.api.v1.ChattoApiService/ListMyRooms", &apiv1.ListMyRoomsRequest{
		Kind: corev1.RoomKind_ROOM_KIND_DM,
	})
	dmsWireResp := readWireResponse(t, conn, "list-dms", 5*time.Second)
	var dms apiv1.ListMyRoomsResponse
	if err := proto.Unmarshal(dmsWireResp.GetBody(), &dms); err != nil {
		t.Fatalf("unmarshal DM ListMyRooms response: %v", err)
	}
	if len(dms.GetRoomViews()) != 1 {
		t.Fatalf("DM room view count = %d, want 1", len(dms.GetRoomViews()))
	}
	dmView := dms.GetRoomViews()[0]
	if dmView.GetRoom().GetId() != dm.Id {
		t.Fatalf("DM room id = %q, want %q", dmView.GetRoom().GetId(), dm.Id)
	}
	if dmView.GetRoom().GetKind() != corev1.RoomKind_ROOM_KIND_DM {
		t.Fatalf("DM room kind = %s, want DM", dmView.GetRoom().GetKind())
	}
	if !containsString(userIDs(dmView.GetMembers()), user.Id) || !containsString(userIDs(dmView.GetMembers()), other.Id) {
		t.Fatalf("DM members = %v, want user and other", userIDs(dmView.GetMembers()))
	}
	if len(dms.GetRoomGroups()) != 0 {
		t.Fatalf("DM ListMyRooms room groups = %d, want 0", len(dms.GetRoomGroups()))
	}
}

func TestWireAPI_ListMyFollowedThreads(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wirethreads", "Wire Threads", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	replier, err := env.core.CreateUser(env.ctx, "system", "wirethreadreply", "Wire Thread Reply", "password123")
	if err != nil {
		t.Fatalf("CreateUser replier: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-followed-threads", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, replier.Id, core.KindChannel, replier.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom replier: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindChannel, user.Id, room.Id, root.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, replier.Id, "thread reply", nil, root.Id, "", nil, false); err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	env.login(t, "wirethreads", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "followed-threads-frame", "followed-threads", "/chatto.api.v1.ChattoApiService/ListMyFollowedThreads", &apiv1.ListMyFollowedThreadsRequest{
		Limit:  20,
		Offset: 0,
	})
	wireResp := readWireResponse(t, conn, "followed-threads", 5*time.Second)
	var resp apiv1.ListMyFollowedThreadsResponse
	if err := proto.Unmarshal(wireResp.GetBody(), &resp); err != nil {
		t.Fatalf("unmarshal followed threads response: %v", err)
	}
	if resp.GetTotalCount() != 1 {
		t.Fatalf("total count = %d, want 1", resp.GetTotalCount())
	}
	if resp.GetHasMore() {
		t.Fatal("has_more = true, want false")
	}
	if len(resp.GetThreads()) != 1 {
		t.Fatalf("thread count = %d, want 1", len(resp.GetThreads()))
	}
	thread := resp.GetThreads()[0]
	if thread.GetRoomId() != room.Id {
		t.Fatalf("room id = %q, want %q", thread.GetRoomId(), room.Id)
	}
	if thread.GetRoom().GetName() != "wire-followed-threads" {
		t.Fatalf("room name = %q, want wire-followed-threads", thread.GetRoom().GetName())
	}
	if thread.GetThreadRootEventId() != root.Id {
		t.Fatalf("thread root event id = %q, want %q", thread.GetThreadRootEventId(), root.Id)
	}
	if thread.GetRootMessage().GetId() != root.Id {
		t.Fatalf("root message id = %q, want %q", thread.GetRootMessage().GetId(), root.Id)
	}
	if thread.GetRootMessage().GetEvent().GetMessagePosted().GetBody() != "thread root" {
		t.Fatalf("root message body = %q, want thread root", thread.GetRootMessage().GetEvent().GetMessagePosted().GetBody())
	}
	if thread.GetReplyCount() != 1 {
		t.Fatalf("reply count = %d, want 1", thread.GetReplyCount())
	}
	if thread.GetLastReplyAt() == nil {
		t.Fatal("last_reply_at is nil")
	}
	if !thread.GetHasUnread() {
		t.Fatal("has_unread = false, want true")
	}
}

func TestWireAPI_UnknownMethodReturnsStructuredError(t *testing.T) {
	env := setupWebSocketTestServer(t)

	if _, err := env.core.CreateUser(env.ctx, "system", "wireerror", "Wire Error", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "wireerror", "password123")

	conn := env.connectWire(t)
	sendWireHello(t, conn, "")

	sendWireRequest(t, conn, "bad-frame", "bad", "/chatto.api.v1.ChattoApiService/Nope", &apiv1.GetViewerRequest{})
	errFrame := readWireError(t, conn, "bad", 5*time.Second)
	if errFrame == nil {
		t.Fatal("expected wire error")
	}
	if errFrame.GetRequestId() != "bad" {
		t.Fatalf("error request id = %q, want bad", errFrame.GetRequestId())
	}
	if errFrame.GetCode() != wirev1.ErrorCode_ERROR_CODE_UNIMPLEMENTED {
		t.Fatalf("error code = %v, want UNIMPLEMENTED", errFrame.GetCode())
	}
}

func TestWireAPI_ResumeAfterDoesNotBlockLiveEvents(t *testing.T) {
	env := setupWebSocketTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "wireresume", "Wire Resume", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "wire-resume", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	env.login(t, "wireresume", "password123")
	conn := env.connectWire(t)
	sendWireHello(t, conn, "client-held-cursor")

	event, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "resume marker", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	readWireDurableEvent(t, conn, event.GetId(), 5*time.Second)
}

func readWireResponse(t *testing.T, conn *websocket.Conn, requestID string, timeout time.Duration) *wirev1.Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame := readWireFrame(t, conn, time.Until(deadline))
		if resp := frame.GetResponse(); resp != nil && resp.GetRequestId() == requestID {
			return resp
		}
		if errFrame := frame.GetError(); errFrame != nil && errFrame.GetRequestId() == requestID {
			t.Fatalf("wire request %s failed: %s", requestID, errFrame.GetMessage())
		}
	}
	t.Fatalf("did not receive response for request %s", requestID)
	return nil
}

func userIDs(users []*corev1.User) []string {
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.GetId())
	}
	return ids
}

func containsRoomPreference(prefs []*apiv1.RoomNotificationPreferenceView, roomID string) bool {
	for _, pref := range prefs {
		if pref.GetRoomId() == roomID {
			return true
		}
	}
	return false
}

func findPermissionMatrixCell(cells []*apiv1.PermissionMatrixCellView, scopeID, permission string) *apiv1.PermissionMatrixCellView {
	for _, cell := range cells {
		if cell.GetScopeId() == scopeID && cell.GetPermission() == permission {
			return cell
		}
	}
	return nil
}

func readWireError(t *testing.T, conn *websocket.Conn, requestID string, timeout time.Duration) *wirev1.WireError {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame := readWireFrame(t, conn, time.Until(deadline))
		if errFrame := frame.GetError(); errFrame != nil && errFrame.GetRequestId() == requestID {
			return errFrame
		}
	}
	t.Fatalf("did not receive error for request %s", requestID)
	return nil
}

func readWireDurableEvent(t *testing.T, conn *websocket.Conn, eventID string, timeout time.Duration) *wirev1.StreamEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame := readWireFrame(t, conn, time.Until(deadline))
		if event := frame.GetEvent(); event != nil && event.GetDurableEvent().GetId() == eventID {
			return event
		}
		if errFrame := frame.GetError(); errFrame != nil {
			t.Fatalf("unexpected wire error: %s", errFrame.GetMessage())
		}
	}
	t.Fatalf("did not receive durable event %s", eventID)
	return nil
}

func hasInvalidation(event *wirev1.StreamEvent, kind wirev1.InvalidationKind, id string) bool {
	for _, hint := range event.GetInvalidates() {
		if hint.GetKind() == kind && hint.GetId() == id {
			return true
		}
	}
	return false
}
