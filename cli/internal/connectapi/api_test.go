package connectapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
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
	"hmans.de/chatto/internal/core/linkpreview"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
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
		"/" + apiv1connect.AccountServiceName + "/",
		"/" + adminv1connect.AdminServerServiceName + "/",
		"/" + apiv1connect.AttachmentServiceName + "/",
		"/" + adminv1connect.AdminDiagnosticsServiceName + "/",
		"/" + adminv1connect.AdminEventLogServiceName + "/",
		"/" + adminv1connect.AdminRoomLayoutServiceName + "/",
		"/" + adminv1connect.AdminMemberServiceName + "/",
		"/" + apiv1connect.LinkPreviewServiceName + "/",
		"/" + apiv1connect.MessageServiceName + "/",
		"/" + apiv1connect.MemberDirectoryServiceName + "/",
		"/" + apiv1connect.NotificationServiceName + "/",
		"/" + apiv1connect.NotificationPreferencesServiceName + "/",
		"/" + adminv1connect.AdminPermissionServiceName + "/",
		"/" + apiv1connect.PushNotificationServiceName + "/",
		"/" + apiv1connect.ReadStateServiceName + "/",
		"/" + apiv1connect.ReactionServiceName + "/",
		"/" + adminv1connect.AdminRoleServiceName + "/",
		"/" + apiv1connect.RoomDirectoryServiceName + "/",
		"/" + apiv1connect.RoomServiceName + "/",
		"/" + apiv1connect.RoomTimelineServiceName + "/",
		"/" + apiv1connect.ServerDiscoveryServiceName + "/",
		"/" + apiv1connect.ServerServiceName + "/",
		"/" + apiv1connect.ThreadServiceName + "/",
		"/" + apiv1connect.UserDirectoryServiceName + "/",
		"/" + apiv1connect.ViewerServiceName + "/",
		"/" + apiv1connect.VoiceCallServiceName + "/",
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
		"/" + apiv1connect.AccountServiceName + "/":                 AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminServerServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.AttachmentServiceName + "/":              AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminDiagnosticsServiceName + "/":      AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminEventLogServiceName + "/":         AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminRoomLayoutServiceName + "/":       AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminMemberServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.LinkPreviewServiceName + "/":             AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.MessageServiceName + "/":                 AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.MemberDirectoryServiceName + "/":         AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.NotificationServiceName + "/":            AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.NotificationPreferencesServiceName + "/": AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminPermissionServiceName + "/":       AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.PushNotificationServiceName + "/":        AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ReadStateServiceName + "/":               AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ReactionServiceName + "/":                AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminRoleServiceName + "/":             AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomDirectoryServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomServiceName + "/":                    AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomTimelineServiceName + "/":            AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ServerDiscoveryServiceName + "/":         AuthPolicyPublic,
		"/" + apiv1connect.ServerServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ThreadServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.UserDirectoryServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ViewerServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.VoiceCallServiceName + "/":               AuthPolicyAuthenticatedUser,
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

func TestServerDiscoveryServiceGetServerPublicMetadata(t *testing.T) {
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

	client := apiv1connect.NewServerDiscoveryServiceClient(ts.Client(), ts.URL)
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

func TestUserDirectoryServiceReadsPublicProfiles(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.users.GetUser(env.ctx, connect.NewRequest(&apiv1.GetUserRequest{UserId: env.viewer.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.users.BatchGetUsers(env.ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetUsers code = %v, want unauthenticated", connect.CodeOf(err))
	}

	ctx := withCaller(env.ctx, env.viewer)
	offlineUser, err := env.core.CreateUser(env.ctx, core.SystemActorID, "offline-profile", "Offline Profile", "password")
	if err != nil {
		t.Fatalf("CreateUser offline profile: %v", err)
	}
	offlineResp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{UserId: offlineUser.Id}))
	if err != nil {
		t.Fatalf("GetUser offline profile: %v", err)
	}
	if offlineResp.Msg.GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("offline profile presence = %v, want OFFLINE", offlineResp.Msg.GetUser().GetPresenceStatus())
	}

	if _, err := env.core.SetUserCustomStatus(env.ctx, env.viewer.Id, "wave", "around", nil); err != nil {
		t.Fatalf("SetUserCustomStatus: %v", err)
	}
	if err := env.core.SetPresenceWithOptions(env.ctx, env.viewer.Id, "ONLINE", true); err != nil {
		t.Fatalf("SetPresenceWithOptions: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, "admin"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	resp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{UserId: env.viewer.Id}))
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	user := resp.Msg.GetUser()
	summary := user.GetUser()
	if summary.GetId() != env.viewer.Id || summary.GetLogin() != env.viewer.Login || summary.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("GetUser user = %+v, want viewer public profile", user)
	}
	if user.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE {
		t.Fatalf("PresenceStatus = %v, want ONLINE", user.GetPresenceStatus())
	}
	if user.GetCustomStatus().GetText() != "around" {
		t.Fatalf("CustomStatus = %+v, want status text", user.GetCustomStatus())
	}
	batchResp, err := env.users.BatchGetUsers(ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{
		UserIds: []string{env.viewer.Id, "missing-user", env.viewer.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if got := batchResp.Msg.GetUsers(); len(got) != 1 {
		t.Fatalf("BatchGetUsers len = %d, want 1: %+v", len(got), got)
	} else if got[0].GetId() != env.viewer.Id || got[0].GetLogin() != env.viewer.Login || got[0].GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("BatchGetUsers user = %+v, want viewer summary", got[0])
	}

	byLoginResp, err := env.users.GetUserByLogin(ctx, connect.NewRequest(&apiv1.GetUserByLoginRequest{Login: env.viewer.Login}))
	if err != nil {
		t.Fatalf("GetUserByLogin: %v", err)
	}
	if byLoginResp.Msg.GetUser().GetUser().GetId() != env.viewer.Id {
		t.Fatalf("GetUserByLogin id = %q, want %q", byLoginResp.Msg.GetUser().GetUser().GetId(), env.viewer.Id)
	}

	if _, err := env.users.GetUserByLogin(ctx, connect.NewRequest(&apiv1.GetUserByLoginRequest{Login: "missing-user"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUserByLogin code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{UserId: "missing-user"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser code = %v, want not found", connect.CodeOf(err))
	}
}

func TestAdminRoleServiceManagesRoles(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.roles.ListRoles(env.ctx, connect.NewRequest(&adminv1.ListRolesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}

	listResp, err := env.roles.ListRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListRolesRequest{}))
	if err != nil {
		t.Fatalf("ListRoles regular: %v", err)
	}
	if len(listResp.Msg.GetRoles()) < 4 {
		t.Fatalf("ListRoles regular len = %d, want default roles", len(listResp.Msg.GetRoles()))
	}
	if listResp.Msg.GetViewerCanManageRoles() || listResp.Msg.GetViewerCanAssignRoles() {
		t.Fatalf("regular capabilities manage=%v assign=%v, want false/false", listResp.Msg.GetViewerCanManageRoles(), listResp.Msg.GetViewerCanAssignRoles())
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Helpdesk",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular CreateRole code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "InvalidName",
		DisplayName: "Invalid",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid CreateRole code = %v, want invalid argument", connect.CodeOf(err))
	}

	createResp, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Helpdesk",
		Description: "Support queue",
		Pingable:    true,
	}))
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if got := createResp.Msg.GetRole(); got.GetName() != "helpdesk" || !got.GetPingable() {
		t.Fatalf("created role = %+v, want helpdesk pingable", got)
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Duplicate",
	})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("duplicate CreateRole code = %v, want already exists", connect.CodeOf(err))
	}
	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "triage",
		DisplayName: "Triage",
	})); err != nil {
		t.Fatalf("CreateRole triage: %v", err)
	}
	reorderResp, err := env.roles.ReorderRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ReorderRolesRequest{
		RoleNames: []string{"triage", "helpdesk"},
	}))
	if err != nil {
		t.Fatalf("ReorderRoles: %v", err)
	}
	var customOrder []string
	for _, role := range reorderResp.Msg.GetRoles() {
		if !role.GetIsSystem() {
			customOrder = append(customOrder, role.GetName())
		}
	}
	if strings.Join(customOrder, ",") != "triage,helpdesk" {
		t.Fatalf("custom role order = %v, want triage,helpdesk", customOrder)
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "role-service-member", "Role Service Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, member.Id, "helpdesk"); err != nil {
		t.Fatalf("AssignServerRole helpdesk: %v", err)
	}

	getResp, err := env.roles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if !getResp.Msg.GetViewerCanManageRoles() || !getResp.Msg.GetViewerCanAssignRoles() {
		t.Fatalf("GetRole capabilities manage=%v assign=%v, want true/true", getResp.Msg.GetViewerCanManageRoles(), getResp.Msg.GetViewerCanAssignRoles())
	}
	if len(getResp.Msg.GetUsers()) != 1 || getResp.Msg.GetUsers()[0].GetId() != member.Id {
		t.Fatalf("GetRole users = %+v, want member %s", getResp.Msg.GetUsers(), member.Id)
	}
	if _, err := env.roles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRoleRequest{Name: "missing-role"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetRole code = %v, want not found", connect.CodeOf(err))
	}

	pingable := false
	updateResp, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Support",
		Description: "Support team",
		Pingable:    &pingable,
	}))
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updateResp.Msg.GetRole().GetDisplayName() != "Support" || updateResp.Msg.GetRole().GetPingable() {
		t.Fatalf("updated role = %+v, want Support pingable false", updateResp.Msg.GetRole())
	}

	if _, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{
		Name: core.RoleOwner,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("DeleteRole owner code = %v, want failed precondition", connect.CodeOf(err))
	}
	deleteResp, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("DeleteRole Deleted = false, want true")
	}
	if _, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{Name: "triage"})); err != nil {
		t.Fatalf("DeleteRole triage: %v", err)
	}
}

func TestAdminPermissionServiceMatricesAndWrites(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.permissions.GetRolePermissionTierMatrix(env.ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRolePermissionTierMatrix code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.permissions.GetRolePermissionTierMatrix(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular GetRolePermissionTierMatrix code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	tierResp, err := env.permissions.GetRolePermissionTierMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix: %v", err)
	}
	if len(tierResp.Msg.GetMatrix().GetRoles()) == 0 || len(tierResp.Msg.GetMatrix().GetApplicablePermissions()) == 0 {
		t.Fatalf("tier matrix = %+v, want roles and permissions", tierResp.Msg.GetMatrix())
	}
	emptyScopeTierResp, err := env.permissions.GetRolePermissionTierMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{
		Scope: &adminv1.PermissionScope{},
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix empty scope: %v", err)
	}
	if len(emptyScopeTierResp.Msg.GetMatrix().GetRoles()) == 0 || len(emptyScopeTierResp.Msg.GetMatrix().GetApplicablePermissions()) == 0 {
		t.Fatalf("empty-scope tier matrix = %+v, want roles and permissions", emptyScopeTierResp.Msg.GetMatrix())
	}

	setResp, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
		Scope:      &adminv1.PermissionScope{},
	}))
	if err != nil {
		t.Fatalf("SetRolePermission empty scope allow: %v", err)
	}
	if !setResp.Msg.GetOk() {
		t.Fatal("SetRolePermission Ok = false, want true")
	}
	roleMatrixResp, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionMatrix: %v", err)
	}
	if cell := findAPIPermissionCell(roleMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermMessagePost)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW {
		t.Fatalf("server message.post cell = %+v, want allow override", cell)
	}
	if _, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: "missing-role",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetRolePermissionMatrix code = %v, want not found", connect.CodeOf(err))
	}
	if _, err := env.permissions.SetRolePermission(env.ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_NONE,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated SetRolePermission clear code = %v, want unauthenticated", connect.CodeOf(err))
	}
	clearResp, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_NONE,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	}))
	if err != nil {
		t.Fatalf("SetRolePermission clear: %v", err)
	}
	if !clearResp.Msg.GetOk() {
		t.Fatal("SetRolePermission clear Ok = false, want true")
	}
	roleMatrixResp, err = env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionMatrix after revoke: %v", err)
	}
	if cell := findAPIPermissionCell(roleMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermMessagePost)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_NONE {
		t.Fatalf("server message.post cell after revoke = %+v, want no override", cell)
	}
	if _, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind(99), Id: "future"},
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("future scope SetRolePermission code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManagePermissions); err != nil {
		t.Fatalf("GrantUserPermission user.manage-permissions: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-target", "Permission Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := env.permissions.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{
		UserId:     target.Id,
		Permission: string(core.PermAdminUsersView),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_DENY,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	})); err != nil {
		t.Fatalf("SetUserPermission server deny: %v", err)
	}
	userMatrixResp, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("GetUserPermissionMatrix: %v", err)
	}
	if cell := findAPIPermissionCell(userMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermAdminUsersView)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY {
		t.Fatalf("user server admin.users.view cell = %+v, want deny override", cell)
	}
	if _, err := env.permissions.ExplainPermissions(env.ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ExplainPermissions code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: env.viewer.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
	unprivileged, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-unprivileged", "Permission Unprivileged", "password")
	if err != nil {
		t.Fatalf("CreateUser unprivileged: %v", err)
	}
	if _, err := env.permissions.ExplainPermissions(withCaller(env.ctx, unprivileged), connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("unprivileged ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
	explainResp, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id}))
	if err != nil {
		t.Fatalf("ExplainPermissions: %v", err)
	}
	if len(explainResp.Msg.GetExplanations()) == 0 {
		t.Fatal("ExplainPermissions returned no explanations")
	}

	roomManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-room-manager", "Permission Room Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser room manager: %v", err)
	}
	room := env.createJoinedRoom("permission-room")
	if _, err := env.core.JoinRoom(env.ctx, roomManager.Id, core.KindChannel, roomManager.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom room manager: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, roomManager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	roomManagerCtx := withCaller(env.ctx, roomManager)
	if _, err := env.permissions.SetRolePermission(roomManagerCtx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleEveryone,
		Permission: string(core.PermMessageReact),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_DENY,
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM,
			Id:   room.Id,
		},
	})); err != nil {
		t.Fatalf("SetRolePermission room manager deny: %v", err)
	}
	roomTierResp, err := env.permissions.GetRolePermissionTierMatrix(roomManagerCtx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM,
			Id:   room.Id,
		},
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix room manager: %v", err)
	}
	everyone := findAPITierRole(roomTierResp.Msg.GetMatrix().GetRoles(), core.RoleEveryone)
	if everyone == nil || !stringSliceContains(everyone.GetOverride().GetPermissionDenials(), string(core.PermMessageReact)) {
		t.Fatalf("everyone room override = %+v, want message.react denial", everyone)
	}
	roomExplainResp, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{
		UserId: target.Id,
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ExplainPermissions room: %v", err)
	}
	if len(roomExplainResp.Msg.GetExplanations()) == 0 {
		t.Fatal("ExplainPermissions room returned no explanations")
	}
	if _, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{
		UserId: target.Id,
		RoomId: "missing-room",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("missing room ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
}

func TestAdminDiagnosticsServiceGetSystemInfoRequiresOwner(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.adminDiagnostics.GetSystemInfo(env.ctx, connect.NewRequest(&adminv1.GetSystemInfoRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetSystemInfo code = %v, want unauthenticated", connect.CodeOf(err))
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "diagnostics-member", "Diagnostics Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := env.adminDiagnostics.GetSystemInfo(withCaller(env.ctx, member), connect.NewRequest(&adminv1.GetSystemInfoRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-owner GetSystemInfo code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole owner: %v", err)
	}
	resp, err := env.adminDiagnostics.GetSystemInfo(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetSystemInfoRequest{}))
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}

	if resp.Msg.GetSystemInfo().GetConnection() == nil {
		t.Fatal("Connection = nil")
	}
	if resp.Msg.GetSystemInfo().GetAccount() == nil {
		t.Fatal("Account = nil")
	}
	if resp.Msg.GetSystemInfo().GetNats() == nil {
		t.Fatal("Nats = nil")
	}
	if resp.Msg.GetSystemInfo().GetStats() == nil {
		t.Fatal("Stats = nil")
	}
	if len(resp.Msg.GetProjections()) == 0 {
		t.Fatal("Projections len = 0, want projection diagnostics")
	}
}

func TestAdminEventLogServiceListsFiltersAndReadsEntries(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.adminEventLog.ListEvents(env.ctx, connect.NewRequest(&adminv1.ListEventsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListEvents code = %v, want unauthenticated", connect.CodeOf(err))
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "event-log-member", "Event Log Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := env.adminEventLog.ListEvents(withCaller(env.ctx, member), connect.NewRequest(&adminv1.ListEventsRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-auditor ListEvents code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminAuditView); err != nil {
		t.Fatalf("GrantUserPermission admin.view-audit: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("event-log-connect")
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "event-log-actor", "Event Log Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, actor.Id, core.KindChannel, actor.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom actor: %v", err)
	}

	resp, err := env.adminEventLog.ListEvents(ctx, connect.NewRequest(&adminv1.ListEventsRequest{
		Limit: 2,
		Filter: &adminv1.AdminEventLogFilter{
			EventType: "UserJoinedRoomEvent",
			ActorId:   actor.Id,
		},
	}))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(resp.Msg.GetEntries()) != 1 {
		t.Fatalf("filtered entries len = %d, want 1 (%+v)", len(resp.Msg.GetEntries()), resp.Msg.GetEntries())
	}
	entry := resp.Msg.GetEntries()[0]
	if entry.GetEventType() != "UserJoinedRoomEvent" || entry.GetActorId() != actor.Id || entry.GetCreatedAt() == nil {
		t.Fatalf("filtered event entry = %+v, want actor join event", entry)
	}
	if resp.Msg.GetScanLimit() != core.FilteredEventLogScanLimit || resp.Msg.GetScannedCount() <= 0 {
		t.Fatalf("scan metadata = limit %d scanned %d, want filtered scan metadata", resp.Msg.GetScanLimit(), resp.Msg.GetScannedCount())
	}

	typesResp, err := env.adminEventLog.ListEventTypes(ctx, connect.NewRequest(&adminv1.ListEventTypesRequest{}))
	if err != nil {
		t.Fatalf("ListEventTypes: %v", err)
	}
	if !stringSliceContains(typesResp.Msg.GetEventTypes(), "UserJoinedRoomEvent") || !stringSliceContains(typesResp.Msg.GetEventTypes(), "decode-error") {
		t.Fatalf("event types = %v, want joined-room and decode-error", typesResp.Msg.GetEventTypes())
	}

	getResp, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: entry.GetSequence()}))
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if getResp.Msg.GetEntry().GetSequence() != entry.GetSequence() || getResp.Msg.GetEntry().GetPayloadJson() == "" {
		t.Fatalf("GetEvent entry = %+v, want payload for sequence %s", getResp.Msg.GetEntry(), entry.GetSequence())
	}

	if _, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: "9999999"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetEvent code = %v, want not_found", connect.CodeOf(err))
	}
	if _, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: "not-a-number"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid sequence code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestAdminRoomLayoutServiceListAdminRoomLayout(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	room := env.createJoinedRoom("layout-room")
	link, err := env.core.CreateSidebarLink(env.ctx, core.SystemActorID, groupID, "Docs", "/docs")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	resp, err := env.adminLayout.ListAdminRoomLayout(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListAdminRoomLayoutRequest{}))
	if err != nil {
		t.Fatalf("ListAdminRoomLayout: %v", err)
	}

	group := adminLayoutGroupByID(resp.Msg.GetGroups(), groupID)
	if group == nil {
		t.Fatalf("group %q missing from response", groupID)
	}
	if adminLayoutRoomByID(group.GetRooms(), room.Id) == nil {
		t.Fatalf("room %q missing from group rooms", room.Id)
	}
	if !adminLayoutItemsContainRoom(group.GetItems(), room.Id) {
		t.Fatalf("room %q missing from group items", room.Id)
	}
	if !adminLayoutItemsContainSidebarLink(group.GetItems(), link.Id) {
		t.Fatalf("sidebar link %q missing from group items", link.Id)
	}
}

func TestAdminRoomLayoutServiceCreateRoomGroupRequiresRoleManage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-member", "Layout Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	_, err = env.adminLayout.CreateRoomGroup(withCaller(env.ctx, member), connect.NewRequest(&adminv1.CreateRoomGroupRequest{
		Name:        "Operations",
		Description: "Private operations rooms",
	}))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	resp, err := env.adminLayout.CreateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoomGroupRequest{
		Name:        "Operations",
		Description: "Private operations rooms",
	}))
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if resp.Msg.GetGroup().GetName() != "Operations" {
		t.Fatalf("group name = %q, want Operations", resp.Msg.GetGroup().GetName())
	}
}

func TestAdminRoomLayoutServiceCreateSidebarLinkRequiresRoomManage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-link-member", "Layout Link Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	req := &adminv1.CreateSidebarLinkRequest{GroupId: groupID, Label: "Status", Url: "/status"}
	_, err = env.adminLayout.CreateSidebarLink(withCaller(env.ctx, member), connect.NewRequest(req))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	resp, err := env.adminLayout.CreateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}
	if resp.Msg.GetSidebarLink().GetUrl() != "/status" {
		t.Fatalf("sidebar link URL = %q, want /status", resp.Msg.GetSidebarLink().GetUrl())
	}
}

func TestViewerServiceGetViewerReturnsSelfScopedState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.viewerService.GetViewer(env.ctx, connect.NewRequest(&apiv1.GetViewerRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetViewer code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, env.viewer.Id, "viewer-connect@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}
	tz := "Europe/Berlin"
	tf := corev1.TimeFormat_TIME_FORMAT_24H
	if _, err := env.core.UpdateUserSettings(env.ctx, env.viewer.Id, core.UserSettingsInput{
		Timezone:   &tz,
		TimeFormat: &tf,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	offlineResp, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer offline presence: %v", err)
	}
	if offlineResp.Msg.GetUser().GetProfile().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("initial viewer presence = %v, want OFFLINE", offlineResp.Msg.GetUser().GetProfile().GetPresenceStatus())
	}

	if err := env.core.SetPresence(env.ctx, env.viewer.Id, core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence: %v", err)
	}
	if err := env.core.SetSpaceNotificationLevel(env.ctx, env.viewer.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED); err != nil {
		t.Fatalf("SetSpaceNotificationLevel: %v", err)
	}
	room := env.createJoinedRoom("viewer-prefs")
	if err := env.core.SetRoomNotificationLevel(env.ctx, env.viewer.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES); err != nil {
		t.Fatalf("SetRoomNotificationLevel: %v", err)
	}

	resp, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer: %v", err)
	}
	user := resp.Msg.GetUser()
	profile := user.GetProfile()
	summary := profile.GetUser()
	if summary.GetId() != env.viewer.Id || summary.GetLogin() != env.viewer.Login || summary.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("viewer user = %+v, want id/login/display name from fixture", user)
	}
	if !user.GetHasVerifiedEmail() {
		t.Fatal("HasVerifiedEmail = false, want true")
	}
	if profile.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("PresenceStatus = %v, want AWAY", profile.GetPresenceStatus())
	}
	if settings := user.GetSettings(); settings.GetTimezone() != tz || settings.GetTimeFormat() != apiv1.TimeFormat_TIME_FORMAT_24_HOUR {
		t.Fatalf("settings = %+v, want timezone %q and 24-hour format", settings, tz)
	}
	if pref := resp.Msg.GetServerNotificationPreference(); pref.GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED || pref.GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		t.Fatalf("server notification preference = %+v, want muted/muted", pref)
	}
	foundRoomPref := false
	for _, pref := range resp.Msg.GetRoomNotificationPreferences() {
		if pref.GetRoomId() == room.Id {
			foundRoomPref = true
			if pref.GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || pref.GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
				t.Fatalf("room notification preference = %+v, want all/all", pref)
			}
		}
	}
	if !foundRoomPref {
		t.Fatalf("room notification preferences did not include %s: %+v", room.Id, resp.Msg.GetRoomNotificationPreferences())
	}
}

func TestAccountServiceUpdatesSelfProfileAndSettings(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UpdateProfile(env.ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("No Auth"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateProfile code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateProfile code = %v, want invalid_argument", connect.CodeOf(err))
	}

	profileResp, err := env.account.UpdateProfile(ctx, connect.NewRequest(&apiv1.UpdateProfileRequest{
		DisplayName: stringPtr("Connect Profile"),
		Login:       stringPtr("connect-profile"),
	}))
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if user := profileResp.Msg.GetUser(); user.GetId() != env.viewer.Id || user.GetDisplayName() != "Connect Profile" || user.GetLogin() != "connect-profile" {
		t.Fatalf("updated profile = %+v, want renamed viewer", user)
	}

	tz := "Europe/Berlin"
	settingsResp, err := env.account.UpdateSettings(ctx, connect.NewRequest(&apiv1.UpdateSettingsRequest{
		Timezone:   &tz,
		TimeFormat: apiv1.TimeFormat_TIME_FORMAT_24_HOUR.Enum(),
	}))
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if settings := settingsResp.Msg.GetSettings(); settings.GetTimezone() != tz || settings.GetTimeFormat() != apiv1.TimeFormat_TIME_FORMAT_24_HOUR {
		t.Fatalf("settings = %+v, want timezone %q and 24-hour format", settings, tz)
	}

	clear := ""
	clearResp, err := env.account.UpdateSettings(ctx, connect.NewRequest(&apiv1.UpdateSettingsRequest{
		Timezone: &clear,
	}))
	if err != nil {
		t.Fatalf("UpdateSettings clear timezone: %v", err)
	}
	if clearResp.Msg.GetSettings().Timezone != nil {
		t.Fatalf("cleared timezone = %q, want nil", clearResp.Msg.GetSettings().GetTimezone())
	}
}

func TestAccountServiceDeletesAvatarAndAccount(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UploadAvatar(env.ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image: connectAPITestPNG(),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadAvatar code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadAvatar code = %v, want invalid_argument", connect.CodeOf(err))
	}

	uploadAvatarResp, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image:       connectAPITestPNG(),
		Filename:    "avatar.png",
		ContentType: "image/png",
	}))
	if err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	if user := uploadAvatarResp.Msg.GetUser(); user.GetId() != env.viewer.Id || user.GetAvatarUrl() == "" {
		t.Fatalf("UploadAvatar user = %+v, want viewer with avatar URL", user)
	}

	deleteAvatarResp, err := env.account.DeleteAvatar(ctx, connect.NewRequest(&apiv1.DeleteAvatarRequest{}))
	if err != nil {
		t.Fatalf("DeleteAvatar: %v", err)
	}
	if deleteAvatarResp.Msg.GetUser().GetId() != env.viewer.Id {
		t.Fatalf("DeleteAvatar user id = %q, want %q", deleteAvatarResp.Msg.GetUser().GetId(), env.viewer.Id)
	}
	if deleteAvatarResp.Msg.GetUser().AvatarUrl != nil {
		t.Fatalf("DeleteAvatar avatar URL = %q, want nil", deleteAvatarResp.Msg.GetUser().GetAvatarUrl())
	}

	tokenResp, err := env.account.RequestAccountDeletion(ctx, connect.NewRequest(&apiv1.RequestAccountDeletionRequest{}))
	if err != nil {
		t.Fatalf("RequestAccountDeletion: %v", err)
	}
	if tokenResp.Msg.GetConfirmationToken() == "" {
		t.Fatal("confirmation token is empty")
	}
	if _, err := env.account.DeleteMyAccount(ctx, connect.NewRequest(&apiv1.DeleteMyAccountRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty DeleteMyAccount code = %v, want invalid_argument", connect.CodeOf(err))
	}

	deleteResp, err := env.account.DeleteMyAccount(ctx, connect.NewRequest(&apiv1.DeleteMyAccountRequest{
		ConfirmationToken: tokenResp.Msg.GetConfirmationToken(),
	}))
	if err != nil {
		t.Fatalf("DeleteMyAccount: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
}

func TestAdminMemberServiceUpdatesUsersAndClearsCooldown(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-target", "Admin User Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-regular", "Admin User Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}

	if _, err := env.adminUsers.UpdateUser(env.ctx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("No Auth"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Denied"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular UpdateUser code = %v, want permission_denied", connect.CodeOf(err))
	}

	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-admin", "Admin User Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	adminCtx := withCaller(env.ctx, admin)

	if _, err := env.adminUsers.UpdateUser(adminCtx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateUser code = %v, want invalid_argument", connect.CodeOf(err))
	}
	resp, err := env.adminUsers.UpdateUser(adminCtx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Managed Target"),
		Login:       stringPtr("managed-target"),
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if user := resp.Msg.GetUser(); user.GetId() != target.Id || user.GetDisplayName() != "Managed Target" || user.GetLogin() != "managed-target" {
		t.Fatalf("updated user = %+v, want managed target", user)
	}

	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-self-rename"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-blocked"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("second self rename err = %v, want cooldown", err)
	}
	clearResp, err := env.adminUsers.ClearUsernameCooldown(adminCtx, connect.NewRequest(&adminv1.ClearUsernameCooldownRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("ClearUsernameCooldown: %v", err)
	}
	if !clearResp.Msg.GetCleared() {
		t.Fatal("Cleared = false, want true")
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "target-unblocked"); err != nil {
		t.Fatalf("self rename after cooldown clear: %v", err)
	}
}

func TestAdminMemberServiceListsAndGetsMembers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-target", "Admin Member Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-regular", "Admin Member Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-member-admin", "Admin Member Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, target.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole target: %v", err)
	}
	if err := env.core.AddVerifiedEmailDirect(env.ctx, target.Id, "admin-member-target@example.test"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect target: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, target.Id, "admin-member-target-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin target: %v", err)
	}

	if _, err := env.adminUsers.ListMembers(env.ctx, connect.NewRequest(&adminv1.ListMembersRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListMembers code = %v, want unauthenticated", connect.CodeOf(err))
	}

	regularCtx := withCaller(env.ctx, regular)
	listResp, err := env.adminUsers.ListMembers(regularCtx, connect.NewRequest(&adminv1.ListMembersRequest{
		Search: "target",
		Page:   &apiv1.PageRequest{Limit: 10},
	}))
	if err != nil {
		t.Fatalf("ListMembers regular: %v", err)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 1 || len(listResp.Msg.GetUsers()) != 1 {
		t.Fatalf("ListMembers returned %d/%d users, want 1/1", len(listResp.Msg.GetUsers()), listResp.Msg.GetPage().GetTotalCount())
	}
	listUser := listResp.Msg.GetUsers()[0]
	if listUser.GetUser().GetId() != target.Id {
		t.Fatalf("ListMembers user ID = %q, want %q", listUser.GetUser().GetId(), target.Id)
	}
	if got := listUser.GetRoles(); len(got) != 1 || got[0] != core.RoleModerator {
		t.Fatalf("ListMembers roles = %v, want explicit moderator only", got)
	}
	if listUser.GetHasVerifiedEmail() || len(listUser.GetVerifiedEmails()) != 0 || listUser.GetLastLoginChange() != nil {
		t.Fatalf("ListMembers leaked sensitive fields: %+v", listUser)
	}
	if len(listResp.Msg.GetRoles()) == 0 {
		t.Fatal("ListMembers roles are empty")
	}

	adminCtx := withCaller(env.ctx, admin)
	getResp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("GetMember admin: %v", err)
	}
	member := getResp.Msg.GetMember()
	if member.GetUser().GetId() != target.Id || member.GetUser().GetLogin() != "admin-member-target-renamed" {
		t.Fatalf("GetMember member = %+v, want renamed target", member)
	}
	if !member.GetHasVerifiedEmail() || len(member.GetVerifiedEmails()) != 1 || member.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("GetMember emails = has:%v emails:%v, want target email", member.GetHasVerifiedEmail(), member.GetVerifiedEmails())
	}
	if member.GetLastLoginChange() == nil {
		t.Fatal("GetMember LastLoginChange is nil, want visible cooldown timestamp")
	}
	if !getResp.Msg.GetViewerCanAssignRoles() || !getResp.Msg.GetViewerCanManageRoles() || !getResp.Msg.GetViewerCanManageUserPermissions() {
		t.Fatalf("GetMember admin capabilities = assign:%v manage:%v perms:%v, want all true", getResp.Msg.GetViewerCanAssignRoles(), getResp.Msg.GetViewerCanManageRoles(), getResp.Msg.GetViewerCanManageUserPermissions())
	}
	if len(getResp.Msg.GetRoles()) == 0 || len(getResp.Msg.GetAvailablePermissions()) == 0 {
		t.Fatalf("GetMember roles/perms empty: roles=%d perms=%d", len(getResp.Msg.GetRoles()), len(getResp.Msg.GetAvailablePermissions()))
	}
	if _, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		UserId: "missing-user",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetMember code = %v, want not found", connect.CodeOf(err))
	}
}

func TestAdminMemberServiceAssignsAndRevokesRoles(t *testing.T) {
	env := newConnectAPITestEnv(t)
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-target", "Admin Role Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	regular, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-regular", "Admin Role Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-admin", "Admin Role Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	adminCtx := withCaller(env.ctx, admin)

	if _, err := env.adminUsers.AssignRole(env.ctx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated AssignRole code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.AssignRole(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular AssignRole code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.AssignRole(adminCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty role AssignRole code = %v, want invalid_argument", connect.CodeOf(err))
	}

	assignResp, err := env.adminUsers.AssignRole(adminCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if !assignResp.Msg.GetAssigned() {
		t.Fatal("Assigned = false, want true")
	}
	roles, err := env.core.GetUserRoles(env.ctx, target.Id)
	if err != nil {
		t.Fatalf("GetUserRoles after assign: %v", err)
	}
	if len(roles) != 1 || roles[0] != core.RoleModerator {
		t.Fatalf("roles after assign = %v, want moderator", roles)
	}

	revokeResp, err := env.adminUsers.RevokeRole(adminCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if !revokeResp.Msg.GetRevoked() {
		t.Fatal("Revoked = false, want true")
	}
	roles, err = env.core.GetUserRoles(env.ctx, target.Id)
	if err != nil {
		t.Fatalf("GetUserRoles after revoke: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("roles after revoke = %v, want none", roles)
	}

	if _, err := env.adminUsers.RevokeRole(adminCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   admin.Id,
		RoleName: core.RoleAdmin,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("self admin RevokeRole code = %v, want failed_precondition", connect.CodeOf(err))
	}
}

func TestServerServiceGetServerStateReturnsAuthenticatedServerState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config = config.ChattoConfig{
		Auth: config.AuthConfig{DirectRegistration: boolPtr(false)},
		Push: config.PushConfig{
			Enabled:         true,
			VAPIDPublicKey:  "test-public-key",
			VAPIDPrivateKey: "test-private-key",
			VAPIDSubject:    "mailto:admin@example.com",
		},
		Video: config.VideoConfig{Enabled: true},
		LiveKit: config.LiveKitConfig{
			Enabled:   true,
			URL:       "wss://livekit.example.test",
			APIKey:    "lk-key",
			APISecret: "lk-secret",
		},
	}

	if _, err := env.serverState.GetServerState(env.ctx, connect.NewRequest(&apiv1.GetServerStateRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetServerState code = %v, want unauthenticated", connect.CodeOf(err))
	}

	resp, err := env.serverState.GetServerState(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetServerStateRequest{}))
	if err != nil {
		t.Fatalf("GetServerState: %v", err)
	}
	msg := resp.Msg
	if msg.GetProfile().GetName() != "Chatto" {
		t.Fatalf("profile name = %q, want Chatto", msg.GetProfile().GetName())
	}
	if !msg.GetPushNotificationsEnabled() || msg.GetVapidPublicKey() != "test-public-key" {
		t.Fatalf("push fields = enabled %v key %q, want true/test-public-key", msg.GetPushNotificationsEnabled(), msg.GetVapidPublicKey())
	}
	if msg.GetDirectRegistrationEnabled() {
		t.Fatal("DirectRegistrationEnabled = true, want false")
	}
	if !msg.GetVideoProcessingEnabled() {
		t.Fatal("VideoProcessingEnabled = false, want true")
	}
	if msg.GetLivekitUrl() != "wss://livekit.example.test" {
		t.Fatalf("LivekitUrl = %q, want configured URL", msg.GetLivekitUrl())
	}
	if msg.GetMaxUploadSize() <= 0 || msg.GetMaxVideoUploadSize() <= 0 {
		t.Fatalf("upload sizes = %d/%d, want positive values", msg.GetMaxUploadSize(), msg.GetMaxVideoUploadSize())
	}
	if msg.GetMessageEditWindowSeconds() != int32(core.MessageEditWindow/time.Second) {
		t.Fatalf("MessageEditWindowSeconds = %d, want %d", msg.GetMessageEditWindowSeconds(), int32(core.MessageEditWindow/time.Second))
	}
	if msg.GetViewerCapabilities() == nil {
		t.Fatal("ViewerCapabilities = nil")
	}
}

func TestAdminServerServiceUpdateServerConfig(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.UpdateServerConfig(env.ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName: stringPtr("Nope"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateServerConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}

	if _, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName: stringPtr("Nope"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UpdateServerConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	resp, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName:     stringPtr("Connect Settings"),
		Description:    stringPtr("Description from Connect"),
		Motd:           stringPtr("MOTD from Connect"),
		WelcomeMessage: stringPtr("Welcome from Connect"),
	}))
	if err != nil {
		t.Fatalf("UpdateServerConfig: %v", err)
	}
	profile := resp.Msg.GetProfile()
	if profile.GetName() != "Connect Settings" ||
		profile.GetDescription() != "Description from Connect" ||
		profile.GetMotd() != "MOTD from Connect" ||
		profile.GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("updated profile = %+v", profile)
	}

	cfg, err := env.core.ConfigManager().GetServerConfig(env.ctx)
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if cfg.GetServerName() != "Connect Settings" ||
		cfg.GetDescription() != "Description from Connect" ||
		cfg.GetMotd() != "MOTD from Connect" ||
		cfg.GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("stored config = %+v", cfg)
	}

	if _, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		Description: stringPtr("Updated description only"),
	})); err != nil {
		t.Fatalf("partial UpdateServerConfig: %v", err)
	}
	cfg, err = env.core.ConfigManager().GetServerConfig(env.ctx)
	if err != nil {
		t.Fatalf("GetServerConfig after partial update: %v", err)
	}
	if cfg.GetServerName() != "Connect Settings" || cfg.GetDescription() != "Updated description only" {
		t.Fatalf("partial stored config = %+v", cfg)
	}
}

func TestAdminServerServiceUpdatesServerBranding(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.UploadServerLogo(env.ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: connectAPITestPNG(),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadServerLogo code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: connectAPITestPNG(),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UploadServerLogo without permission code = %v, want permission_denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	if _, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadServerLogo code = %v, want invalid_argument", connect.CodeOf(err))
	}
	logoResp, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image:       connectAPITestPNG(),
		Filename:    "logo.png",
		ContentType: "image/png",
	}))
	if err != nil {
		t.Fatalf("UploadServerLogo: %v", err)
	}
	if logoResp.Msg.GetProfile().GetLogoUrl() == "" {
		t.Fatalf("UploadServerLogo profile = %+v, want logo URL", logoResp.Msg.GetProfile())
	}

	deleteLogoResp, err := env.serverState.DeleteServerLogo(ctx, connect.NewRequest(&adminv1.DeleteServerLogoRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerLogo: %v", err)
	}
	if deleteLogoResp.Msg.GetProfile().LogoUrl != nil {
		t.Fatalf("DeleteServerLogo logo URL = %q, want nil", deleteLogoResp.Msg.GetProfile().GetLogoUrl())
	}

	if _, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadServerBanner code = %v, want invalid_argument", connect.CodeOf(err))
	}
	bannerResp, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{
		Image:       connectAPITestPNG(),
		Filename:    "banner.png",
		ContentType: "image/png",
	}))
	if err != nil {
		t.Fatalf("UploadServerBanner: %v", err)
	}
	if bannerResp.Msg.GetProfile().GetBannerUrl() == "" {
		t.Fatalf("UploadServerBanner profile = %+v, want banner URL", bannerResp.Msg.GetProfile())
	}

	deleteBannerResp, err := env.serverState.DeleteServerBanner(ctx, connect.NewRequest(&adminv1.DeleteServerBannerRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerBanner: %v", err)
	}
	if deleteBannerResp.Msg.GetProfile().BannerUrl != nil {
		t.Fatalf("DeleteServerBanner banner URL = %q, want nil", deleteBannerResp.Msg.GetProfile().GetBannerUrl())
	}
}

func TestAdminServerServiceSecurityConfig(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.GetServerSecurityConfig(env.ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetServerSecurityConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetServerSecurityConfig(ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("GetServerSecurityConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: "root",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UpdateBlockedUsernames without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	configResp, err := env.serverState.GetServerSecurityConfig(ctx, connect.NewRequest(&adminv1.GetServerSecurityConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerSecurityConfig: %v", err)
	}
	if configResp.Msg.GetBlockedUsernames() != core.DefaultBlockedUsernames {
		t.Fatalf("default blocked usernames = %q, want %q", configResp.Msg.GetBlockedUsernames(), core.DefaultBlockedUsernames)
	}

	updateResp, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: "root\nreserved",
	}))
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames: %v", err)
	}
	if updateResp.Msg.GetBlockedUsernames() != "root\nreserved" {
		t.Fatalf("updated blocked usernames = %q, want root/reserved", updateResp.Msg.GetBlockedUsernames())
	}
	stored, err := env.core.ConfigManager().GetEffectiveBlockedUsernames(env.ctx)
	if err != nil {
		t.Fatalf("GetEffectiveBlockedUsernames: %v", err)
	}
	if stored != "root\nreserved" {
		t.Fatalf("stored blocked usernames = %q, want root/reserved", stored)
	}

	if _, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: strings.Repeat("u", core.MaxLoginLength+1),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("oversized UpdateBlockedUsernames code = %v, want invalid argument", connect.CodeOf(err))
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
	if _, err := env.rooms.ListRoomBans(env.ctx, connect.NewRequest(&apiv1.ListRoomBansRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoomBans code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.rooms.ListRoomBans(ctx, connect.NewRequest(&apiv1.ListRoomBansRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("ListRoomBans without permission code = %v, want permission denied", connect.CodeOf(err))
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

	listResp, err := env.rooms.ListRoomBans(ctx, connect.NewRequest(&apiv1.ListRoomBansRequest{}))
	if err != nil {
		t.Fatalf("ListRoomBans: %v", err)
	}
	if got := len(listResp.Msg.GetBans()); got != 1 {
		t.Fatalf("ListRoomBans count = %d, want 1", got)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 1 || listResp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListRoomBans page = %+v, want total_count 1 has_more false", listResp.Msg.GetPage())
	}
	listedBan := listResp.Msg.GetBans()[0]
	if listedBan.GetId() == "" {
		t.Fatalf("ListRoomBans ban id is empty")
	}
	if listedBan.GetRoomId() != room.Id || listedBan.GetRoom().GetName() != room.Name {
		t.Fatalf("ListRoomBans room = %+v, want id %s name %q", listedBan.GetRoom(), room.Id, room.Name)
	}
	if listedBan.GetUserId() != target.Id || listedBan.GetUser().GetProfile().GetUser().GetDisplayName() != target.DisplayName {
		t.Fatalf("ListRoomBans user = %+v, want target %s", listedBan.GetUser(), target.Id)
	}
	if listedBan.GetModeratorId() != env.viewer.Id || listedBan.GetModerator().GetProfile().GetUser().GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("ListRoomBans moderator = %+v, want viewer %s", listedBan.GetModerator(), env.viewer.Id)
	}
	if listedBan.GetReason() != "moderation test" {
		t.Fatalf("ListRoomBans reason = %q, want moderation test", listedBan.GetReason())
	}
	if listedBan.GetCreatedAt() == nil {
		t.Fatalf("ListRoomBans created_at is nil")
	}
	if listedBan.GetExpiresAt() != nil {
		t.Fatalf("ListRoomBans expires_at = %v, want nil", listedBan.GetExpiresAt())
	}

	filteredResp, err := env.rooms.ListRoomBans(ctx, connect.NewRequest(&apiv1.ListRoomBansRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListRoomBans filtered: %v", err)
	}
	if got := len(filteredResp.Msg.GetBans()); got != 1 {
		t.Fatalf("filtered ListRoomBans count = %d, want 1", got)
	}
	if filteredResp.Msg.GetPage().GetTotalCount() != 1 || filteredResp.Msg.GetPage().GetHasMore() {
		t.Fatalf("filtered ListRoomBans page = %+v, want total_count 1 has_more false", filteredResp.Msg.GetPage())
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
	afterUnbanResp, err := env.rooms.ListRoomBans(ctx, connect.NewRequest(&apiv1.ListRoomBansRequest{}))
	if err != nil {
		t.Fatalf("ListRoomBans after unban: %v", err)
	}
	if got := len(afterUnbanResp.Msg.GetBans()); got != 0 {
		t.Fatalf("ListRoomBans after unban count = %d, want 0", got)
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

	_, err = env.attachments.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: dm.Id,
		Page:   &apiv1.PageRequest{Limit: 10},
	}))
	checkInaccessible("ListRoomAttachments", err)

	_, err = env.attachments.RefreshMessageAttachmentUrls(ctx, connect.NewRequest(&apiv1.RefreshMessageAttachmentUrlsRequest{
		RoomId:  dm.Id,
		EventId: root.Id,
	}))
	checkInaccessible("RefreshMessageAttachmentUrls", err)

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

func TestMemberDirectoryServiceListServerMembers(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.members.ListServerMembers(env.ctx, connect.NewRequest(&apiv1.ListServerMembersRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListServerMembers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
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

	resp, err := env.members.ListServerMembers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListServerMembersRequest{
		Search: "member",
		Page:   &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil {
		t.Fatalf("ListServerMembers: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 2 || !resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetMembers()) != 1 {
		t.Fatalf("first page = %+v, want total 2, hasMore true, one member", resp.Msg)
	}

	secondResp, err := env.members.ListServerMembers(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListServerMembersRequest{
		Search: "member",
		Page:   &apiv1.PageRequest{Limit: 1, Offset: 1},
	}))
	if err != nil {
		t.Fatalf("ListServerMembers second page: %v", err)
	}
	if secondResp.Msg.GetPage().GetHasMore() || len(secondResp.Msg.GetMembers()) != 1 {
		t.Fatalf("second page = %+v, want hasMore false and one member", secondResp.Msg)
	}

	gotByID := map[string]*apiv1.DirectoryMember{}
	for _, member := range append(resp.Msg.GetMembers(), secondResp.Msg.GetMembers()...) {
		gotByID[member.GetProfile().GetUser().GetId()] = member
	}
	if gotByID[alice.Id].GetProfile().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("alice presence = %v, want AWAY", gotByID[alice.Id].GetProfile().GetPresenceStatus())
	}
	if gotByID[bob.Id].GetProfile().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("bob presence = %v, want OFFLINE", gotByID[bob.Id].GetProfile().GetPresenceStatus())
	}
	if roles := strings.Join(gotByID[bob.Id].GetRoles(), ","); roles != "everyone,admin" {
		t.Fatalf("bob roles = %q, want everyone,admin", roles)
	}
}

func TestMemberDirectoryServiceListRoomMembersRequiresMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("member-directory-room")
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
	if _, err := env.members.ListRoomMembers(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoomMembers code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "room-member-outsider", "Room Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.members.ListRoomMembers(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider ListRoomMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	resp, err := env.members.ListRoomMembers(withCaller(env.ctx, env.viewer), req)
	if err != nil {
		t.Fatalf("ListRoomMembers: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 1 || resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetMembers()) != 1 {
		t.Fatalf("room member page = %+v, want one alice result", resp.Msg)
	}
	got := resp.Msg.GetMembers()[0]
	if got.GetProfile().GetUser().GetId() != member.Id || got.GetProfile().GetUser().GetDisplayName() != "Room Alice" || got.GetProfile().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB {
		t.Fatalf("room member = %+v, want hydrated Room Alice", got)
	}
}

func TestAccountServiceSetAndClearCustomStatus(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	expiresAt := timestamppb.New(time.Now().Add(time.Hour).UTC())

	setResp, err := env.account.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
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

	_, err = env.account.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
		Emoji: "🌿",
		Text:  "   ",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SetCustomStatus blank text error = %v, want InvalidArgument", err)
	}

	clearResp, err := env.account.ClearCustomStatus(ctx, connect.NewRequest(&apiv1.ClearCustomStatusRequest{}))
	if err != nil {
		t.Fatalf("ClearCustomStatus: %v", err)
	}
	if clearResp.Msg.GetStatus() != nil {
		t.Fatalf("cleared status = %+v, want nil", clearResp.Msg.GetStatus())
	}
}

func TestNotificationServiceListsAndDismissesNotifications(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("notification-connect-room")
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-actor", "Notification Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, actor.Id, core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence actor: %v", err)
	}

	mention, err := env.core.CreateNotification(env.ctx, env.viewer.Id, actor.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{
				RoomId:   room.Id,
				EventId:  "mention-event",
				InThread: "thread-root",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification mention: %v", err)
	}
	if _, err := env.core.CreateNotification(env.ctx, env.viewer.Id, actor.Id, &corev1.Notification{
		Notification: &corev1.Notification_DmMessage{
			DmMessage: &corev1.DMMessageNotification{
				RoomId:  "dm-room",
				EventId: "dm-event",
			},
		},
	}); err != nil {
		t.Fatalf("CreateNotification dm: %v", err)
	}

	listResp, err := env.notifications.ListNotifications(ctx, connect.NewRequest(&apiv1.ListNotificationsRequest{Page: &apiv1.PageRequest{Limit: 1}}))
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 2 || !listResp.Msg.GetPage().GetHasMore() || len(listResp.Msg.GetItems()) != 1 {
		t.Fatalf("ListNotifications page = %+v, want total 2, has_more true, one item", listResp.Msg)
	}
	item := listResp.Msg.GetItems()[0]
	if item.GetActor().GetUser().GetDisplayName() != "Notification Actor" || item.GetActor().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("notification actor = %+v, want hydrated actor", item.GetActor())
	}

	roomResp, err := env.notifications.ListRoomNotifications(ctx, connect.NewRequest(&apiv1.ListRoomNotificationsRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListRoomNotifications: %v", err)
	}
	if roomResp.Msg.GetPage().GetTotalCount() != 1 || len(roomResp.Msg.GetItems()) != 1 {
		t.Fatalf("ListRoomNotifications page = %+v, want one room notification", roomResp.Msg)
	}
	mentionItem := roomResp.Msg.GetItems()[0]
	if mentionItem.GetMention().GetRoom().GetId() != room.Id || mentionItem.GetMention().GetThreadRootEventId() != "thread-root" {
		t.Fatalf("mention payload = %+v, want room/thread payload", mentionItem.GetMention())
	}
	if mentionItem.GetSummary() != "Notification Actor mentioned you" {
		t.Fatalf("summary = %q, want mention summary", mentionItem.GetSummary())
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-outsider", "Notification Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderResp, err := env.notifications.ListRoomNotifications(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.ListRoomNotificationsRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListRoomNotifications outsider: %v", err)
	}
	if outsiderResp.Msg.GetPage().GetTotalCount() != 0 || len(outsiderResp.Msg.GetItems()) != 0 {
		t.Fatalf("outsider room notifications = %+v, want empty page", outsiderResp.Msg)
	}

	hasResp, err := env.notifications.HasNotifications(ctx, connect.NewRequest(&apiv1.HasNotificationsRequest{}))
	if err != nil {
		t.Fatalf("HasNotifications: %v", err)
	}
	if !hasResp.Msg.GetHasNotifications() {
		t.Fatal("HasNotifications = false, want true")
	}
	countsResp, err := env.notifications.ListNotificationCounts(ctx, connect.NewRequest(&apiv1.ListNotificationCountsRequest{}))
	if err != nil {
		t.Fatalf("ListNotificationCounts: %v", err)
	}
	counts := make(map[string]int32)
	for _, count := range countsResp.Msg.GetRoomCounts() {
		counts[count.GetRoomId()] = count.GetTotalCount()
	}
	if counts[room.Id] != 1 || counts["dm-room"] != 1 {
		t.Fatalf("ListNotificationCounts = %+v, want counts for channel and DM rooms", counts)
	}

	dismissResp, err := env.notifications.DismissNotification(ctx, connect.NewRequest(&apiv1.DismissNotificationRequest{NotificationId: mention.Id}))
	if err != nil {
		t.Fatalf("DismissNotification: %v", err)
	}
	if !dismissResp.Msg.GetDismissed() {
		t.Fatal("DismissNotification dismissed = false, want true")
	}
	dismissAgainResp, err := env.notifications.DismissNotification(ctx, connect.NewRequest(&apiv1.DismissNotificationRequest{NotificationId: mention.Id}))
	if err != nil {
		t.Fatalf("DismissNotification again: %v", err)
	}
	if dismissAgainResp.Msg.GetDismissed() {
		t.Fatal("DismissNotification again dismissed = true, want false")
	}

	dismissAllResp, err := env.notifications.DismissAllNotifications(ctx, connect.NewRequest(&apiv1.DismissAllNotificationsRequest{}))
	if err != nil {
		t.Fatalf("DismissAllNotifications: %v", err)
	}
	if dismissAllResp.Msg.GetDismissedCount() != 1 {
		t.Fatalf("DismissAllNotifications count = %d, want 1", dismissAllResp.Msg.GetDismissedCount())
	}
}

func TestNotificationPreferencesServiceServerLevelPreference(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.prefs.SetServerNotificationLevel(env.ctx, connect.NewRequest(&apiv1.SetServerNotificationLevelRequest{
		Level: apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated SetServerNotificationLevel code = %v, want unauthenticated", connect.CodeOf(err))
	}

	setResp, err := env.prefs.SetServerNotificationLevel(ctx, connect.NewRequest(&apiv1.SetServerNotificationLevelRequest{
		Level: apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES,
	}))
	if err != nil {
		t.Fatalf("SetServerNotificationLevel: %v", err)
	}
	if setResp.Msg.GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || setResp.Msg.GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("SetServerNotificationLevel response = %+v, want all/all", setResp.Msg)
	}

	getResp, err := env.prefs.GetServerNotificationPreference(ctx, connect.NewRequest(&apiv1.GetServerNotificationPreferenceRequest{}))
	if err != nil {
		t.Fatalf("GetServerNotificationPreference: %v", err)
	}
	if getResp.Msg.GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || getResp.Msg.GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("GetServerNotificationPreference response = %+v, want all/all", getResp.Msg)
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
	subResp, err := env.push.Subscribe(ctx, connect.NewRequest(&apiv1.SubscribePushRequest{
		Endpoint:  "https://push.example.test/sub",
		P256Dh:    "p256dh-key",
		Auth:      "auth-secret",
		UserAgent: stringPtr("test-agent"),
	}))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if !subResp.Msg.GetSubscribed() {
		t.Fatal("Subscribe subscribed = false, want true")
	}
	subs, err := env.core.GetUserPushSubscriptions(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUserPushSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].GetEndpoint() != "https://push.example.test/sub" || subs[0].GetUserAgent() != "test-agent" {
		t.Fatalf("stored subscriptions = %+v, want one saved subscription", subs)
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
	disabledActive, err := env.voice.ListActiveCallRooms(ctx, connect.NewRequest(&apiv1.ListActiveCallRoomsRequest{}))
	if err != nil {
		t.Fatalf("disabled ListActiveCallRooms: %v", err)
	}
	if len(disabledActive.Msg.GetRoomIds()) != 0 {
		t.Fatalf("disabled active rooms = %v, want none", disabledActive.Msg.GetRoomIds())
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

	activeResp, err := env.voice.ListActiveCallRooms(ctx, connect.NewRequest(&apiv1.ListActiveCallRoomsRequest{}))
	if err != nil {
		t.Fatalf("ListActiveCallRooms: %v", err)
	}
	if got := strings.Join(activeResp.Msg.GetRoomIds(), ","); got != room.Id {
		t.Fatalf("active room IDs = %v, want %s", activeResp.Msg.GetRoomIds(), room.Id)
	}

	participantsResp, err := env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ListCallParticipants: %v", err)
	}
	participants := participantsResp.Msg.GetParticipants()
	if len(participants) != 1 || participants[0].GetUser().GetUser().GetId() != env.viewer.Id || participants[0].GetCallId() == "" || participants[0].GetJoinedAt() == nil {
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
}

func TestAccountServiceReportPresence(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.ReportPresence(env.ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ReportPresence code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	if _, err := env.account.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unspecified ReportPresence code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if _, err := env.account.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
		Status: apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("offline ReportPresence code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	resp, err := env.account.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
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

	autoResp, err := env.account.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
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

	if _, err := env.account.ReportPresence(ctx, connect.NewRequest(&apiv1.ReportPresenceRequest{
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

func TestLinkPreviewServiceFetchLinkPreviewRequiresAuthAndMapsPreview(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.linkPreviews.FetchLinkPreview(env.ctx, connect.NewRequest(&apiv1.FetchLinkPreviewRequest{Url: "https://example.test"})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated FetchLinkPreview code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	restoreLocalhost := linkpreview.AllowLocalhostForTesting()
	defer restoreLocalhost()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
<meta property="og:title" content="Connect Preview">
<meta property="og:description" content="Connect preview description">
<meta property="og:site_name" content="Connect Site">
<meta property="og:image" content="` + serverURL + `/preview.png">
</head>
<body>hello</body>
</html>`))
		case "/preview.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(connectAPITestPNG())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	resp, err := env.linkPreviews.FetchLinkPreview(
		withCaller(env.ctx, env.viewer),
		connect.NewRequest(&apiv1.FetchLinkPreviewRequest{Url: server.URL + "/article"}),
	)
	if err != nil {
		t.Fatalf("FetchLinkPreview: %v", err)
	}
	preview := resp.Msg.GetPreview()
	if preview == nil {
		t.Fatal("FetchLinkPreview preview = nil")
	}
	if preview.GetUrl() != server.URL+"/article" ||
		preview.GetTitle() != "Connect Preview" ||
		preview.GetDescription() != "Connect preview description" ||
		preview.GetSiteName() != "Connect Site" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.GetImageAssetId() == "" {
		t.Fatalf("ImageAssetId is empty")
	}
	if !strings.Contains(preview.GetImageUrl(), preview.GetImageAssetId()) {
		t.Fatalf("ImageUrl %q does not contain asset id %q", preview.GetImageUrl(), preview.GetImageAssetId())
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
	otherRoom := env.createJoinedRoom("message-post-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

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
			name: "missing in-reply-to target",
			req: &apiv1.PostMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: "missing-reply-target",
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.PostMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: otherRoomMessage.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "link preview URL too long",
			req: &apiv1.PostMessageRequest{
				RoomId: room.Id,
				Body:   "hello",
				LinkPreview: &apiv1.LinkPreview{
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

func TestMessageServicePostMessageUploadsAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-upload")

	resp, err := env.messages.PostMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Attachments: []*apiv1.MessageAttachmentUpload{{
			Filename:    "note.txt",
			ContentType: "text/plain",
			Content:     []byte("uploaded over connect"),
		}},
	}))
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	message := resp.Msg.GetEvent().GetMessagePosted()
	if message == nil {
		t.Fatalf("PostMessage payload = %T, want message_posted", resp.Msg.GetEvent().GetEvent())
	}
	attachments := message.GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}
	if attachments[0].GetFilename() != "note.txt" || attachments[0].GetContentType() != "text/plain" {
		t.Fatalf("attachment = %+v, want note.txt text/plain", attachments[0])
	}
	if attachments[0].GetId() == "" {
		t.Fatal("attachment id is empty")
	}
}

func TestMessageServicePostMessageAttachmentPreflightDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-upload-preflight")
	ctx := withCaller(env.ctx, env.viewer)

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageAttach); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before denied post: %v", err)
	}
	_, err = env.messages.PostMessage(ctx, connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Attachments: []*apiv1.MessageAttachmentUpload{{
			Filename:    "note.txt",
			ContentType: "text/plain",
			Content:     []byte("denied upload"),
		}},
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied attachment PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after denied post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after denied attachment = %d, want unchanged %d", after, before)
	}
}

func TestMessageServicePostMessageMentionConfirmationDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-mention-confirm")
	ctx := withCaller(env.ctx, env.viewer)

	for i := 0; i < core.LargeMentionNotificationThreshold+1; i++ {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "large-mention-target-"+strconv.Itoa(i), "Large Mention Target", "password")
		if err != nil {
			t.Fatalf("CreateUser target %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom target %d: %v", i, err)
		}
	}

	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before challenged post: %v", err)
	}
	resp, err := env.messages.PostMessage(ctx, connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "@all please review this attachment",
		Attachments: []*apiv1.MessageAttachmentUpload{{
			Filename:    "note.txt",
			ContentType: "text/plain",
			Content:     []byte("challenged upload"),
		}},
	}))
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	challenge := resp.Msg.GetMentionConfirmation()
	if challenge == nil {
		t.Fatalf("PostMessage response = %+v, want mention confirmation", resp.Msg)
	}
	if challenge.GetRecipientCount() != int32(core.LargeMentionNotificationThreshold+1) {
		t.Fatalf("RecipientCount = %d, want %d", challenge.GetRecipientCount(), core.LargeMentionNotificationThreshold+1)
	}
	if challenge.GetToken() == "" {
		t.Fatal("confirmation token is empty")
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after challenged post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after challenged attachment = %d, want unchanged %d", after, before)
	}
}

func TestMessageServicePostMessageValidationPreflightDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-validation")
	ctx := withCaller(env.ctx, env.viewer)

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	otherRoom := env.createJoinedRoom("upload-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

	tests := []struct {
		name string
		req  *apiv1.PostMessageRequest
		code connect.Code
	}{
		{
			name: "missing thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: "missing-thread-root",
				Attachments: []*apiv1.MessageAttachmentUpload{{
					Filename:    "note.txt",
					ContentType: "text/plain",
					Content:     []byte("missing root upload"),
				}},
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: reply.Id,
				Attachments: []*apiv1.MessageAttachmentUpload{{
					Filename:    "note.txt",
					ContentType: "text/plain",
					Content:     []byte("bad root upload"),
				}},
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing in-reply-to target",
			req: &apiv1.PostMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: "missing-reply-target",
				Attachments: []*apiv1.MessageAttachmentUpload{{
					Filename:    "note.txt",
					ContentType: "text/plain",
					Content:     []byte("missing reply upload"),
				}},
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.PostMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: otherRoomMessage.Id,
				Attachments: []*apiv1.MessageAttachmentUpload{{
					Filename:    "note.txt",
					ContentType: "text/plain",
					Content:     []byte("other room reply upload"),
				}},
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "link preview URL too long",
			req: &apiv1.PostMessageRequest{
				RoomId: room.Id,
				Body:   "message with bad preview and file",
				LinkPreview: &apiv1.LinkPreview{
					Url: strings.Repeat("x", core.MaxLinkPreviewURLLength+1),
				},
				Attachments: []*apiv1.MessageAttachmentUpload{{
					Filename:    "note.txt",
					ContentType: "text/plain",
					Content:     []byte("bad preview upload"),
				}},
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := env.core.GetAssetCount(env.ctx)
			if err != nil {
				t.Fatalf("GetAssetCount before post: %v", err)
			}
			_, err = env.messages.PostMessage(ctx, connect.NewRequest(tt.req))
			if connect.CodeOf(err) != tt.code {
				t.Fatalf("PostMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
			after, err := env.core.GetAssetCount(env.ctx)
			if err != nil {
				t.Fatalf("GetAssetCount after post: %v", err)
			}
			if after != before {
				t.Fatalf("asset count after invalid attachment post = %d, want unchanged %d", after, before)
			}
		})
	}
}

func TestMessageServicePostMessageRejectsVideoUploadWhenProcessingDisabled(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-upload-video-disabled")
	ctx := withCaller(env.ctx, env.viewer)

	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before video post: %v", err)
	}
	_, err = env.messages.PostMessage(ctx, connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Attachments: []*apiv1.MessageAttachmentUpload{
			{
				Filename:    "note.txt",
				ContentType: "text/plain",
				Content:     []byte("first"),
			},
			{
				Filename:    "clip.mp4",
				ContentType: "video/mp4",
				Content:     []byte("video"),
			},
		},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("video upload PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after video post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after rejected video = %d, want unchanged %d", after, before)
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
	if page.StartCursor == "" || page.EndCursor == "" {
		t.Fatalf("cursors = %q/%q, want non-empty cursors", page.StartCursor, page.EndCursor)
	}
	if strings.HasPrefix(page.StartCursor, roomTimelineCursorSeqPrefix) || strings.HasPrefix(page.EndCursor, roomTimelineCursorSeqPrefix) {
		t.Fatalf("cursors = %q/%q, want opaque cursors", page.StartCursor, page.EndCursor)
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

	startSeq, err := parseRoomTimelineCursor(page.StartCursor)
	if err != nil {
		t.Fatalf("parse emitted start cursor: %v", err)
	}
	legacyResp, err := env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
		Cursor: &apiv1.GetRoomEventsRequest_Before{Before: roomTimelineCursorSeqPrefix + strconv.FormatUint(startSeq, 10)},
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents before legacy cursor: %v", err)
	}
	if !timelinePageContains(legacyResp.Msg.GetPage(), m1.Id) {
		t.Fatalf("legacy cursor page does not contain first message %s", m1.Id)
	}
}

func TestRoomTimelineCursorFormatIsOpaqueAndVersioned(t *testing.T) {
	cursor := formatRoomTimelineCursor(42)
	if cursor == "" {
		t.Fatal("formatRoomTimelineCursor returned empty cursor")
	}
	if strings.HasPrefix(cursor, roomTimelineCursorSeqPrefix) || strings.Contains(cursor, "42") {
		t.Fatalf("cursor %q exposes raw sequence", cursor)
	}
	seq, err := parseRoomTimelineCursor(cursor)
	if err != nil {
		t.Fatalf("parse opaque cursor: %v", err)
	}
	if seq != 42 {
		t.Fatalf("opaque cursor seq = %d, want 42", seq)
	}
	legacySeq, err := parseRoomTimelineCursor("seq:42")
	if err != nil {
		t.Fatalf("parse legacy cursor: %v", err)
	}
	if legacySeq != 42 {
		t.Fatalf("legacy cursor seq = %d, want 42", legacySeq)
	}
	for _, invalid := range []string{"bad", "tl:not-base64", "tl:AQ"} {
		if _, err := parseRoomTimelineCursor(invalid); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("parse invalid cursor %q code = %v, want invalid_argument", invalid, connect.CodeOf(err))
		}
	}
}

func TestAttachmentServiceListsAndRefreshesRoomAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("attachment-list")

	rootAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "root.txt", "text/plain", bytes.NewReader([]byte("root")))
	if err != nil {
		t.Fatalf("UploadAttachment root: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "root file", []string{rootAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	threadAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "thread.png", "image/png", bytes.NewReader(connectAPITestPNG()))
	if err != nil {
		t.Fatalf("UploadAttachment thread: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "thread file", []string{threadAttachment.Id}, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.attachments.ListRoomAttachments(env.ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 10},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoomAttachments code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	resp, err := env.attachments.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 1},
		Thumbnail: &apiv1.AttachmentThumbnailOptions{
			Width:  120,
			Height: 120,
			Fit:    apiv1.AttachmentFitMode_ATTACHMENT_FIT_MODE_COVER,
		},
	}))
	if err != nil {
		t.Fatalf("ListRoomAttachments: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 2 || !resp.Msg.GetPage().GetHasMore() || len(resp.Msg.Items) != 1 {
		t.Fatalf("ListRoomAttachments count/hasMore/items = %d/%v/%d, want 2/true/1", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore(), len(resp.Msg.Items))
	}
	first := resp.Msg.Items[0]
	if first.MessageEventId != reply.Id || first.ThreadRootEventId != root.Id {
		t.Fatalf("first message/thread IDs = %q/%q, want %q/%q", first.MessageEventId, first.ThreadRootEventId, reply.Id, root.Id)
	}
	if first.GetAttachment().GetId() != threadAttachment.Id || first.GetAttachment().GetFilename() != "thread.png" {
		t.Fatalf("first attachment = %+v, want thread.png", first.GetAttachment())
	}
	if first.GetAttachment().GetAssetUrl().GetUrl() == "" || first.GetAttachment().GetThumbnailAssetUrl().GetUrl() == "" {
		t.Fatalf("attachment asset URLs missing: %+v", first.GetAttachment())
	}
	if first.GetCreatedAt() == nil {
		t.Fatal("created_at missing")
	}

	refresh, err := env.attachments.RefreshMessageAttachmentUrls(ctx, connect.NewRequest(&apiv1.RefreshMessageAttachmentUrlsRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
		Thumbnail: &apiv1.AttachmentThumbnailOptions{
			Width:  64,
			Height: 64,
			Fit:    apiv1.AttachmentFitMode_ATTACHMENT_FIT_MODE_CONTAIN,
		},
	}))
	if err != nil {
		t.Fatalf("RefreshMessageAttachmentUrls: %v", err)
	}
	if len(refresh.Msg.Attachments) != 1 {
		t.Fatalf("refresh attachments = %d, want 1", len(refresh.Msg.Attachments))
	}
	fresh := refresh.Msg.Attachments[0]
	if fresh.AttachmentId != threadAttachment.Id {
		t.Fatalf("refresh attachment ID = %q, want %q", fresh.AttachmentId, threadAttachment.Id)
	}
	if fresh.GetAssetUrl().GetUrl() == "" || fresh.GetAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh asset URL missing: %+v", fresh.GetAssetUrl())
	}
	if fresh.GetThumbnailAssetUrl().GetUrl() == "" || fresh.GetThumbnailAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh thumbnail URL missing: %+v", fresh.GetThumbnailAssetUrl())
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

func TestRoomTimelineServiceResolveMessageLinkTarget(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-message-link")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	req := connect.NewRequest(&apiv1.ResolveMessageLinkTargetRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	})
	if _, err := env.timeline.ResolveMessageLinkTarget(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ResolveMessageLinkTarget code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-link-outsider", "Message Link Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.timeline.ResolveMessageLinkTarget(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member ResolveMessageLinkTarget code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.timeline.ResolveMessageLinkTarget(ctx, connect.NewRequest(&apiv1.ResolveMessageLinkTargetRequest{
		RoomId:  room.Id,
		EventId: root.Id,
	}))
	if err != nil {
		t.Fatalf("ResolveMessageLinkTarget root: %v", err)
	}
	if rootResp.Msg.GetEvent().GetId() != root.Id || rootResp.Msg.GetThreadRootEventId() != "" {
		t.Fatalf("root target = event %q thread %q, want event %q no thread", rootResp.Msg.GetEvent().GetId(), rootResp.Msg.GetThreadRootEventId(), root.Id)
	}
	if rootResp.Msg.GetEvent().GetMessagePosted().GetBody() != "root" {
		t.Fatalf("root body = %q, want root", rootResp.Msg.GetEvent().GetMessagePosted().GetBody())
	}
	if rootResp.Msg.GetIncludes().GetUsers()[env.viewer.Id] == nil {
		t.Fatalf("root includes missing viewer %s", env.viewer.Id)
	}

	replyResp, err := env.timeline.ResolveMessageLinkTarget(ctx, connect.NewRequest(&apiv1.ResolveMessageLinkTargetRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	}))
	if err != nil {
		t.Fatalf("ResolveMessageLinkTarget reply: %v", err)
	}
	if replyResp.Msg.GetEvent().GetId() != reply.Id || replyResp.Msg.GetThreadRootEventId() != root.Id {
		t.Fatalf("reply target = event %q thread %q, want event %q thread %q", replyResp.Msg.GetEvent().GetId(), replyResp.Msg.GetThreadRootEventId(), reply.Id, root.Id)
	}

	if _, err := env.timeline.ResolveMessageLinkTarget(ctx, connect.NewRequest(&apiv1.ResolveMessageLinkTargetRequest{
		RoomId:  room.Id,
		EventId: "missing-anchor",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing target code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
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
	if got := attachments[0].GetThumbnailAssetUrl().GetUrl(); !strings.Contains(got, "/960x800/cover") {
		t.Fatalf("attachment thumbnail URL = %q, want 960x800 cover transform", got)
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

func TestThreadServiceListFollowedThreadsReturnsHydratedPage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("followed-thread-list")
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-list-participant", "Thread List Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, participant.Id, core.KindChannel, participant.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom participant: %v", err)
	}
	root := env.post(room.Id, env.viewer.Id, "root body", "")
	env.post(room.Id, participant.Id, "reply body", root.Id)

	if _, err := env.threads.ListFollowedThreads(env.ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListFollowedThreads code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}

	resp, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 1 || resp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListFollowedThreads page metadata = total %d hasMore %v, want total 1 hasMore false", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore())
	}
	if len(resp.Msg.GetThreads()) != 1 {
		t.Fatalf("ListFollowedThreads returned %d threads, want 1", len(resp.Msg.GetThreads()))
	}

	thread := resp.Msg.GetThreads()[0]
	if thread.GetRoomId() != room.Id || thread.GetRoomName() != room.Name || thread.GetThreadRootEventId() != root.Id {
		t.Fatalf("followed thread identity = room %q name %q root %q, want room %q name %q root %q", thread.GetRoomId(), thread.GetRoomName(), thread.GetThreadRootEventId(), room.Id, room.Name, root.Id)
	}
	if thread.GetReplyCount() != 1 || !thread.GetHasUnread() || thread.GetLastReplyAt() == nil {
		t.Fatalf("followed thread metadata = replies %d unread %v lastReplyAt %v, want replies 1 unread true lastReplyAt set", thread.GetReplyCount(), thread.GetHasUnread(), thread.GetLastReplyAt())
	}
	rootMessage := thread.GetRootMessage()
	if rootMessage == nil || rootMessage.GetId() != root.Id || rootMessage.GetMessagePosted() == nil {
		t.Fatalf("root message = %+v, want hydrated message_posted event %s", rootMessage, root.Id)
	}
	if got := rootMessage.GetMessagePosted().GetBody(); got != "root body" {
		t.Fatalf("root message body = %q, want root body", got)
	}
	users := resp.Msg.GetIncludes().GetUsers()
	if users[env.viewer.Id] == nil || users[participant.Id] == nil {
		t.Fatalf("includes users missing viewer or participant: got %d included users", len(users))
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

func boolPtr(value bool) *bool {
	return &value
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func findAPIPermissionCell(cells []*adminv1.PermissionMatrixCell, scopeID, permission string) *adminv1.PermissionMatrixCell {
	for _, cell := range cells {
		if cell.GetScopeId() == scopeID && cell.GetPermission() == permission {
			return cell
		}
	}
	return nil
}

func findAPITierRole(roles []*adminv1.TierRole, roleName string) *adminv1.TierRole {
	for _, role := range roles {
		if role.GetRoleName() == roleName {
			return role
		}
	}
	return nil
}

func connectAPITestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: 60, B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type connectAPITestEnv struct {
	ctx              context.Context
	core             *core.ChattoCore
	nc               *nats.Conn
	api              *API
	account          *accountService
	attachments      *attachmentService
	adminDiagnostics *adminDiagnosticsService
	adminEventLog    *adminEventLogService
	adminLayout      *adminRoomLayoutService
	adminUsers       *adminUserManagementService
	directory        *roomDirectoryService
	linkPreviews     *linkPreviewService
	messages         *messageService
	members          *memberDirectoryService
	notifications    *notificationService
	permissions      *permissionService
	prefs            *notificationPreferencesService
	push             *pushNotificationService
	readState        *readStateService
	reactions        *reactionService
	roles            *roleService
	rooms            *roomService
	serverState      *serverService
	timeline         *roomTimelineService
	threads          *threadService
	users            *userService
	viewerService    *viewerService
	voice            *voiceCallService
	viewer           *corev1.User
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
		ctx:              ctx,
		core:             c,
		nc:               nc,
		api:              api,
		account:          &accountService{api: api},
		attachments:      &attachmentService{api: api},
		adminDiagnostics: &adminDiagnosticsService{api: api},
		adminEventLog:    &adminEventLogService{api: api},
		adminLayout:      &adminRoomLayoutService{api: api},
		adminUsers:       &adminUserManagementService{api: api},
		directory:        &roomDirectoryService{api: api},
		linkPreviews:     &linkPreviewService{api: api},
		messages:         &messageService{api: api},
		members:          &memberDirectoryService{api: api},
		notifications:    &notificationService{api: api},
		permissions:      &permissionService{api: api},
		prefs:            &notificationPreferencesService{api: api},
		push:             &pushNotificationService{api: api},
		readState:        &readStateService{api: api},
		reactions:        &reactionService{api: api},
		roles:            &roleService{api: api},
		rooms:            &roomService{api: api},
		serverState:      &serverService{api: api},
		timeline:         &roomTimelineService{api: api},
		threads:          &threadService{api: api},
		users:            &userService{api: api},
		viewerService:    &viewerService{api: api},
		voice:            &voiceCallService{api: api},
		viewer:           viewer,
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

func adminLayoutGroupByID(groups []*adminv1.AdminRoomLayoutGroup, groupID string) *adminv1.AdminRoomLayoutGroup {
	for _, group := range groups {
		if group.GetId() == groupID {
			return group
		}
	}
	return nil
}

func adminLayoutRoomByID(rooms []*adminv1.AdminRoomLayoutRoom, roomID string) *adminv1.AdminRoomLayoutRoom {
	for _, room := range rooms {
		if room.GetId() == roomID {
			return room
		}
	}
	return nil
}

func adminLayoutItemsContainRoom(items []*adminv1.AdminRoomLayoutItem, roomID string) bool {
	for _, item := range items {
		if item.GetRoom().GetId() == roomID {
			return true
		}
	}
	return false
}

func adminLayoutItemsContainSidebarLink(items []*adminv1.AdminRoomLayoutItem, linkID string) bool {
	for _, item := range items {
		if item.GetSidebarLink().GetId() == linkID {
			return true
		}
	}
	return false
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
