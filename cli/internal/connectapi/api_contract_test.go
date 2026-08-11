package connectapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
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
		"/" + apiv1connect.MyAccountServiceName + "/",
		"/" + apiv1connect.AssetServiceName + "/",
		"/" + apiv1connect.AssetUploadServiceName + "/",
		"/" + adminv1connect.AdminServerServiceName + "/",
		"/" + authv1connect.ExternalIdentityAuthServiceName + "/",
		"/" + adminv1connect.AdminDiagnosticsServiceName + "/",
		"/" + adminv1connect.AdminEventLogServiceName + "/",
		"/" + adminv1connect.AdminRoomLayoutServiceName + "/",
		"/" + adminv1connect.AdminUserServiceName + "/",
		"/" + grpcreflect.ReflectV1AlphaServiceName + "/",
		"/" + grpcreflect.ReflectV1ServiceName + "/",
		"/" + apiv1connect.MessageServiceName + "/",
		"/" + apiv1connect.MessageSearchServiceName + "/",
		"/" + apiv1connect.NotificationServiceName + "/",
		"/" + apiv1connect.NotificationPreferencesServiceName + "/",
		"/" + adminv1connect.AdminPermissionServiceName + "/",
		"/" + apiv1connect.PushNotificationServiceName + "/",
		"/" + adminv1connect.AdminRoleServiceName + "/",
		"/" + apiv1connect.RoleServiceName + "/",
		"/" + apiv1connect.RoomDirectoryServiceName + "/",
		"/" + apiv1connect.RoomServiceName + "/",
		"/" + discoveryv1connect.ServerDiscoveryServiceName + "/",
		"/" + apiv1connect.ServerServiceName + "/",
		"/" + apiv1connect.ThreadServiceName + "/",
		"/" + apiv1connect.UserServiceName + "/",
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
		"/" + apiv1connect.MyAccountServiceName + "/":               AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.AssetServiceName + "/":                   AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.AssetUploadServiceName + "/":             AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminServerServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + authv1connect.ExternalIdentityAuthServiceName + "/":   AuthPolicyPublic,
		"/" + adminv1connect.AdminDiagnosticsServiceName + "/":      AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminEventLogServiceName + "/":         AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminRoomLayoutServiceName + "/":       AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminUserServiceName + "/":             AuthPolicyAuthenticatedUser,
		"/" + grpcreflect.ReflectV1AlphaServiceName + "/":           AuthPolicyPublic,
		"/" + grpcreflect.ReflectV1ServiceName + "/":                AuthPolicyPublic,
		"/" + apiv1connect.MessageServiceName + "/":                 AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.MessageSearchServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.NotificationServiceName + "/":            AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.NotificationPreferencesServiceName + "/": AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminPermissionServiceName + "/":       AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.PushNotificationServiceName + "/":        AuthPolicyAuthenticatedUser,
		"/" + adminv1connect.AdminRoleServiceName + "/":             AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoleServiceName + "/":                    AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomDirectoryServiceName + "/":           AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.RoomServiceName + "/":                    AuthPolicyAuthenticatedUser,
		"/" + discoveryv1connect.ServerDiscoveryServiceName + "/":   AuthPolicyPublic,
		"/" + apiv1connect.ServerServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.ThreadServiceName + "/":                  AuthPolicyAuthenticatedUser,
		"/" + apiv1connect.UserServiceName + "/":                    AuthPolicyAuthenticatedUser,
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

func TestPublicReflectionResolver(t *testing.T) {
	resolver, err := publicReflectionResolver(publicReflectionServiceNames)
	if err != nil {
		t.Fatalf("publicReflectionResolver: %v", err)
	}

	if _, err := resolver.FindDescriptorByName(protoreflect.FullName(discoveryv1connect.ServerDiscoveryServiceName)); err != nil {
		t.Fatalf("FindDescriptorByName(%s): %v", discoveryv1connect.ServerDiscoveryServiceName, err)
	}
	if _, err := resolver.FindDescriptorByName(protoreflect.FullName(authv1connect.ExternalIdentityAuthServiceName)); err != nil {
		t.Fatalf("FindDescriptorByName(%s): %v", authv1connect.ExternalIdentityAuthServiceName, err)
	}
	if _, err := resolver.FindFileByPath("chatto/auth/v1/external_identity_auth.proto"); err != nil {
		t.Fatalf("FindFileByPath(chatto/auth/v1/external_identity_auth.proto): %v", err)
	}
	if _, err := resolver.FindFileByPath("chatto/discovery/v1/server.proto"); err != nil {
		t.Fatalf("FindFileByPath(chatto/discovery/v1/server.proto): %v", err)
	}
	if _, err := resolver.FindFileByPath("chatto/admin/v1/diagnostics.proto"); err != nil {
		t.Fatalf("FindFileByPath(chatto/admin/v1/diagnostics.proto): %v", err)
	}
	if _, err := resolver.FindFileByPath("chatto/core/v1/event.proto"); !errors.Is(err, protoregistry.NotFound) {
		t.Fatalf("FindFileByPath(chatto/core/v1/event.proto) err = %v, want NotFound", err)
	}
	if _, err := resolver.FindDescriptorByName("chatto.core.v1.Event"); !errors.Is(err, protoregistry.NotFound) {
		t.Fatalf("FindDescriptorByName(chatto.core.v1.Event) err = %v, want NotFound", err)
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

func TestUserSummaryTreatsInvalidPresenceKeyAsOffline(t *testing.T) {
	env := newConnectAPITestEnv(t)

	user, err := userSummary(env.ctx, env.api, core.DeletedUserReference("bad>"), nil)
	if err != nil {
		t.Fatalf("userSummary: %v", err)
	}
	if user.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("PresenceStatus = %v, want OFFLINE", user.GetPresenceStatus())
	}
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
	_, err := client.CreateMessage(context.Background(), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: "room",
		Body:   "hello",
	}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestCreateMessageAttachmentAssetIDsValidateThroughConnectHandler(t *testing.T) {
	env := newConnectAPITestEnv(t)
	mux := http.NewServeMux()
	path, handler := apiv1connect.NewMessageServiceHandler(env.messages, HandlerOptions()...)
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL)
	maxIDs := make([]string, core.MaxMessageAttachmentAssetIDs)
	for i := range maxIDs {
		maxIDs[i] = strings.Repeat("A", core.MaxMessageAttachmentAssetIDLength)
	}

	tests := []struct {
		name     string
		assetIDs []string
		wantCode connect.Code
	}{
		{name: "at limits reaches authentication", assetIDs: maxIDs, wantCode: connect.CodeUnauthenticated},
		{name: "too many", assetIDs: append(append([]string(nil), maxIDs...), "A"), wantCode: connect.CodeInvalidArgument},
		{name: "empty", assetIDs: []string{""}, wantCode: connect.CodeInvalidArgument},
		{name: "too long", assetIDs: []string{strings.Repeat("A", core.MaxMessageAttachmentAssetIDLength+1)}, wantCode: connect.CodeInvalidArgument},
		{name: "too many multibyte bytes", assetIDs: []string{strings.Repeat("é", core.MaxMessageAttachmentAssetIDLength/2+1)}, wantCode: connect.CodeInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.CreateMessage(context.Background(), connect.NewRequest(&apiv1.CreateMessageRequest{
				RoomId:             "room",
				Body:               "hello",
				AttachmentAssetIds: tt.assetIDs,
			}))
			if connect.CodeOf(err) != tt.wantCode {
				t.Fatalf("CreateMessage() code = %v, want %v", connect.CodeOf(err), tt.wantCode)
			}
		})
	}
}

func TestBatchGetResourceRequestsValidateThroughConnectHandlers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	mux := http.NewServeMux()
	rolePath, roleHandler := apiv1connect.NewRoleServiceHandler(env.publicRoles, HandlerOptions()...)
	roomDirectoryPath, roomDirectoryHandler := apiv1connect.NewRoomDirectoryServiceHandler(env.directory, HandlerOptions()...)
	assetPath, assetHandler := apiv1connect.NewAssetServiceHandler(env.assets, HandlerOptions()...)
	messagePath, messageHandler := apiv1connect.NewMessageServiceHandler(env.messages, HandlerOptions()...)
	serverPath, serverHandler := apiv1connect.NewServerServiceHandler(env.serverState, HandlerOptions()...)
	userPath, userHandler := apiv1connect.NewUserServiceHandler(env.users, HandlerOptions()...)
	roomPath, roomHandler := apiv1connect.NewRoomServiceHandler(env.rooms, HandlerOptions()...)
	notificationPath, notificationHandler := apiv1connect.NewNotificationServiceHandler(env.notifications, HandlerOptions()...)
	voicePath, voiceHandler := apiv1connect.NewVoiceCallServiceHandler(env.voice, HandlerOptions()...)
	adminMemberPath, adminMemberHandler := adminv1connect.NewAdminUserServiceHandler(env.adminUsers, HandlerOptions()...)
	adminServerPath, adminServerHandler := adminv1connect.NewAdminServerServiceHandler(env.serverState, HandlerOptions()...)
	mux.Handle(rolePath, roleHandler)
	mux.Handle(roomDirectoryPath, roomDirectoryHandler)
	mux.Handle(assetPath, assetHandler)
	mux.Handle(messagePath, messageHandler)
	mux.Handle(serverPath, serverHandler)
	mux.Handle(userPath, userHandler)
	mux.Handle(roomPath, roomHandler)
	mux.Handle(notificationPath, notificationHandler)
	mux.Handle(voicePath, voiceHandler)
	mux.Handle(adminMemberPath, adminMemberHandler)
	mux.Handle(adminServerPath, adminServerHandler)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	roles := apiv1connect.NewRoleServiceClient(ts.Client(), ts.URL)
	roomDirectory := apiv1connect.NewRoomDirectoryServiceClient(ts.Client(), ts.URL)
	assets := apiv1connect.NewAssetServiceClient(ts.Client(), ts.URL)
	messages := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL)
	users := apiv1connect.NewUserServiceClient(ts.Client(), ts.URL)
	rooms := apiv1connect.NewRoomServiceClient(ts.Client(), ts.URL)
	notifications := apiv1connect.NewNotificationServiceClient(ts.Client(), ts.URL)
	voice := apiv1connect.NewVoiceCallServiceClient(ts.Client(), ts.URL)
	adminMembers := adminv1connect.NewAdminUserServiceClient(ts.Client(), ts.URL)
	adminServer := adminv1connect.NewAdminServerServiceClient(ts.Client(), ts.URL)

	if _, err := roles.BatchGetRoles(context.Background(), connect.NewRequest(&apiv1.BatchGetRolesRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetRoles code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := roles.BatchGetRoles(context.Background(), connect.NewRequest(&apiv1.BatchGetRolesRequest{Names: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-name BatchGetRoles code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyRoleNames := make([]string, 101)
	for i := range tooManyRoleNames {
		tooManyRoleNames[i] = fmt.Sprintf("role-%d", i)
	}
	if _, err := roles.BatchGetRoles(context.Background(), connect.NewRequest(&apiv1.BatchGetRolesRequest{Names: tooManyRoleNames})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetRoles code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := roomDirectory.BatchGetRooms(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomsRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetRooms code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := roomDirectory.BatchGetRooms(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomsRequest{RoomIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetRooms code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyRoomIDs := make([]string, 101)
	for i := range tooManyRoomIDs {
		tooManyRoomIDs[i] = fmt.Sprintf("room-%d", i)
	}
	if _, err := roomDirectory.BatchGetRooms(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomsRequest{RoomIds: tooManyRoomIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetRooms code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := roomDirectory.GetRoomGroup(context.Background(), connect.NewRequest(&apiv1.GetRoomGroupRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty GetRoomGroup code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := roomDirectory.BatchGetRoomGroups(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomGroupsRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetRoomGroups code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := roomDirectory.BatchGetRoomGroups(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomGroupsRequest{GroupIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetRoomGroups code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyGroupIDs := make([]string, 101)
	for i := range tooManyGroupIDs {
		tooManyGroupIDs[i] = fmt.Sprintf("group-%d", i)
	}
	if _, err := roomDirectory.BatchGetRoomGroups(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomGroupsRequest{GroupIds: tooManyGroupIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetRoomGroups code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := users.BatchGetUsers(context.Background(), connect.NewRequest(&apiv1.BatchGetUsersRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetUsers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := users.BatchGetUsers(context.Background(), connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetUsers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyUserIDs := make([]string, 101)
	for i := range tooManyUserIDs {
		tooManyUserIDs[i] = fmt.Sprintf("user-%d", i)
	}
	if _, err := users.BatchGetUsers(context.Background(), connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: tooManyUserIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetUsers code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := rooms.BatchGetMembers(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetRoomMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := rooms.BatchGetMembers(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{RoomId: "room", UserIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetRoomMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := rooms.BatchGetMembers(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{UserIds: []string{"user"}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-room BatchGetRoomMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := rooms.BatchGetMembers(context.Background(), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{RoomId: "room", UserIds: tooManyUserIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetRoomMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := notifications.GetNotification(context.Background(), connect.NewRequest(&apiv1.GetNotificationRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty GetNotification code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := notifications.BatchGetNotifications(context.Background(), connect.NewRequest(&apiv1.BatchGetNotificationsRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetNotifications code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := notifications.BatchGetNotifications(context.Background(), connect.NewRequest(&apiv1.BatchGetNotificationsRequest{NotificationIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetNotifications code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyNotificationIDs := make([]string, 101)
	for i := range tooManyNotificationIDs {
		tooManyNotificationIDs[i] = fmt.Sprintf("notification-%d", i)
	}
	if _, err := notifications.BatchGetNotifications(context.Background(), connect.NewRequest(&apiv1.BatchGetNotificationsRequest{NotificationIds: tooManyNotificationIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetNotifications code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := messages.BatchGetMessages(context.Background(), connect.NewRequest(&apiv1.BatchGetMessagesRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetMessages code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := messages.BatchGetMessages(context.Background(), connect.NewRequest(&apiv1.BatchGetMessagesRequest{RoomId: "room", EventIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetMessages code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := messages.BatchGetMessages(context.Background(), connect.NewRequest(&apiv1.BatchGetMessagesRequest{EventIds: []string{"event"}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-room BatchGetMessages code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyEventIDs := make([]string, 101)
	for i := range tooManyEventIDs {
		tooManyEventIDs[i] = fmt.Sprintf("event-%d", i)
	}
	if _, err := messages.BatchGetMessages(context.Background(), connect.NewRequest(&apiv1.BatchGetMessagesRequest{RoomId: "room", EventIds: tooManyEventIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetMessages code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := assets.BatchGetAssets(context.Background(), connect.NewRequest(&apiv1.BatchGetAssetsRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetAssets code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := assets.BatchGetAssets(context.Background(), connect.NewRequest(&apiv1.BatchGetAssetsRequest{RoomId: "room", AssetIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetAssets code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := assets.BatchGetAssets(context.Background(), connect.NewRequest(&apiv1.BatchGetAssetsRequest{AssetIds: []string{"asset"}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-room BatchGetAssets code = %v, want invalid_argument", connect.CodeOf(err))
	}
	tooManyAssetIDs := make([]string, 101)
	for i := range tooManyAssetIDs {
		tooManyAssetIDs[i] = fmt.Sprintf("asset-%d", i)
	}
	if _, err := assets.BatchGetAssets(context.Background(), connect.NewRequest(&apiv1.BatchGetAssetsRequest{RoomId: "room", AssetIds: tooManyAssetIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetAssets code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := voice.GetActiveCall(context.Background(), connect.NewRequest(&apiv1.GetActiveCallRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty GetActiveCall code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := voice.BatchGetActiveCalls(context.Background(), connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetActiveCalls code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := voice.BatchGetActiveCalls(context.Background(), connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{RoomIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetActiveCalls code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := voice.BatchGetActiveCalls(context.Background(), connect.NewRequest(&apiv1.BatchGetActiveCallsRequest{RoomIds: tooManyRoomIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetActiveCalls code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := adminMembers.BatchGetMembers(context.Background(), connect.NewRequest(&adminv1.BatchGetMembersRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty BatchGetMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := adminMembers.BatchGetMembers(context.Background(), connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{""}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty-id BatchGetMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := adminMembers.BatchGetMembers(context.Background(), connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: tooManyUserIDs})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many BatchGetMembers code = %v, want invalid_argument", connect.CodeOf(err))
	}

	tooManyBlockedUsernames := make([]string, 1001)
	for i := range tooManyBlockedUsernames {
		tooManyBlockedUsernames[i] = fmt.Sprintf("blocked-%d", i)
	}
	if _, err := adminServer.UpdateBlockedUsernames(context.Background(), connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{BlockedUsernames: tooManyBlockedUsernames})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("too-many UpdateBlockedUsernames code = %v, want invalid_argument", connect.CodeOf(err))
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
		{"limit exceeded", core.ErrLimitExceeded, connect.CodeResourceExhausted},
		{"reaction limit exceeded", core.ErrReactionLimitExceeded, connect.CodeResourceExhausted},
		{"slow mode active", &core.SlowModeActiveError{}, connect.CodeResourceExhausted},
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

func TestSafeInternalErrorForLogRedactsSensitiveSubstrings(t *testing.T) {
	err := errors.New("failed for email=person@example.test token=cht_ATabcdef123456 redirect=https://chat.example.test/callback?code=secret&state=s url=https://chat.example.test/path?code=secret&state=s and raw other@example.test")

	got := safeInternalErrorForLog(err)
	for _, forbidden := range []string{
		"person@example.test",
		"other@example.test",
		"cht_ATabcdef123456",
		"code=secret",
		"state=s",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("safeInternalErrorForLog leaked %q in %q", forbidden, got)
		}
	}
	for _, want := range []string{"email=[redacted]", "redirect=[redacted]", "token=[redacted]", "?[redacted]", "[redacted-email]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("safeInternalErrorForLog = %q, want redaction marker %q", got, want)
		}
	}
}

func TestAPIPermissionExplanationMarksWinningTraceFirst(t *testing.T) {
	got := apiPermissionExplanation(core.PermissionExplanation{
		Permission:    core.PermAdminUsersView,
		State:         core.DecisionDeny,
		DecidedAt:     core.LevelRoom,
		DecidedByRole: core.RoleEveryone,
		Trace: []core.TraceEntry{
			{
				Level:    core.LevelServer,
				RoleName: "custom",
				Decision: core.DecisionAllow,
			},
			{
				Level:    core.LevelRoom,
				RoleName: core.RoleEveryone,
				Decision: core.DecisionDeny,
			},
		},
	})

	if got.GetState() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY {
		t.Fatalf("state = %v, want deny", got.GetState())
	}
	trace := got.GetTrace()
	if len(trace) != 2 {
		t.Fatalf("trace length = %d, want 2", len(trace))
	}
	if trace[0].GetRoleName() != core.RoleEveryone || trace[0].GetDecision() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY || !trace[0].GetApplied() {
		t.Fatalf("first trace entry = %+v, want winning deny applied", trace[0])
	}
	if trace[1].GetApplied() {
		t.Fatalf("second trace entry applied = true, want false")
	}
}
