package connectapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/events"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
	"hmans.de/chatto/internal/pb/chatto/auth/v1/authv1connect"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
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

	client := discoveryv1connect.NewServerDiscoveryServiceClient(ts.Client(), ts.URL)
	resp, err := client.GetServer(context.Background(), connect.NewRequest(&discoveryv1.GetServerRequest{}))
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}

	msg := resp.Msg
	if msg.GetProfile().GetName() != "Chatto" {
		t.Fatalf("profile name = %q, want Chatto", msg.GetProfile().GetName())
	}
	if msg.GetProfile().GetVersion() != "9.8.7" {
		t.Fatalf("profile version = %q, want 9.8.7", msg.GetProfile().GetVersion())
	}
	if !msg.GetLogin().GetDirectRegistrationEnabled() {
		t.Fatal("DirectRegistrationEnabled = false, want true")
	}
	if len(msg.GetLogin().GetProviders()) != 1 {
		t.Fatalf("providers len = %d, want 1", len(msg.GetLogin().GetProviders()))
	}
	provider := msg.GetLogin().GetProviders()[0]
	if provider.Id != "hub provider" {
		t.Fatalf("provider Id = %q, want hub provider", provider.Id)
	}
	if provider.LoginUrl != "/auth/providers/hub%20provider" {
		t.Fatalf("provider LoginUrl = %q, want escaped provider path", provider.LoginUrl)
	}
}

func TestExternalIdentityFlowsAndAccountManagement(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Auth.Providers = []config.AuthProviderConfig{
		{ID: "github-main", Type: config.AuthProviderTypeGitHub, Label: "GitHub"},
		{ID: "discord-main", Type: config.AuthProviderTypeDiscord, Label: "Discord"},
		{ID: "gitlab-main", Type: config.AuthProviderTypeGitLab, Label: "GitLab"},
	}

	createToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:      "github-main",
		ProviderType:    config.AuthProviderTypeGitHub,
		ProviderLabel:   "GitHub",
		Issuer:          "github-main",
		Subject:         "12345",
		LoginHint:       "sso-user",
		DisplayNameHint: "SSO User",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}

	pending, err := env.externalAuth.GetPendingExternalIdentity(env.ctx, connect.NewRequest(&authv1.GetPendingExternalIdentityRequest{
		Token: createToken,
	}))
	if err != nil {
		t.Fatalf("GetPendingExternalIdentity: %v", err)
	}
	if pending.Msg.Pending.GetKind() != authv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_CREATE_ACCOUNT || pending.Msg.Pending.GetProviderId() != "github-main" {
		t.Fatalf("pending = %+v", pending.Msg.Pending)
	}

	created, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: createToken,
		Login: "sso-user",
	}))
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount: %v", err)
	}
	createdAuthToken := created.Msg.GetToken()
	userID, err := env.core.ValidateAuthToken(env.ctx, createdAuthToken)
	if err != nil {
		t.Fatalf("ValidateAuthToken: %v", err)
	}
	if userID != created.Msg.GetUserId() {
		t.Fatalf("created token user = %q, want %q", userID, created.Msg.GetUserId())
	}
	createdUser, err := env.core.GetUser(env.ctx, created.Msg.GetUserId())
	if err != nil {
		t.Fatalf("GetUser created: %v", err)
	}
	if createdUser.GetDisplayName() != "SSO User" {
		t.Fatalf("created display name = %q, want SSO User", createdUser.GetDisplayName())
	}
	if _, err := env.core.GetPendingExternalIdentityFlow(env.ctx, createToken); !errors.Is(err, core.ErrExternalIdentityFlowNotFound) {
		t.Fatalf("pending create flow after confirmation error = %v, want ErrExternalIdentityFlowNotFound", err)
	}

	fallbackToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:      "discord-main",
		ProviderType:    config.AuthProviderTypeDiscord,
		ProviderLabel:   "Discord",
		Issuer:          "discord-main",
		Subject:         "fallback-display-name",
		LoginHint:       "fallback-user",
		DisplayNameHint: strings.Repeat("Provider ", 8),
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow fallback: %v", err)
	}
	fallbackCreated, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: fallbackToken,
		Login: "fallback-user",
	}))
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount fallback: %v", err)
	}
	fallbackUser, err := env.core.GetUser(env.ctx, fallbackCreated.Msg.GetUserId())
	if err != nil {
		t.Fatalf("GetUser fallback: %v", err)
	}
	if fallbackUser.GetDisplayName() != "fallback-user" {
		t.Fatalf("fallback display name = %q, want login", fallbackUser.GetDisplayName())
	}

	createdUserRef := &corev1.User{Id: created.Msg.GetUserId()}
	createdCtx := withBearerCredential(env.ctx, createdUserRef, createdAuthToken)
	list, err := env.account.ListExternalIdentities(createdCtx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	if err != nil {
		t.Fatalf("ListExternalIdentities: %v", err)
	}
	if len(list.Msg.GetProviders()) != 3 ||
		list.Msg.GetProviders()[0].GetLinkUrl() != "/auth/providers/github-main?intent=link" ||
		!list.Msg.GetProviders()[0].GetLinked() ||
		list.Msg.GetProviders()[0].GetLinkedIdentitySubjectHash() == "" {
		t.Fatalf("providers = %+v", list.Msg.GetProviders())
	}
	if len(list.Msg.GetLinkedIdentities()) != 1 || list.Msg.GetLinkedIdentities()[0].GetProviderId() != "github-main" {
		t.Fatalf("linked identities = %+v", list.Msg.GetLinkedIdentities())
	}
	_, err = env.account.DisconnectExternalIdentity(createdCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash: list.Msg.GetProviders()[0].GetLinkedIdentitySubjectHash(),
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	if _, err := env.account.StartExternalIdentityLink(withCaller(env.ctx, createdUserRef), connect.NewRequest(&apiv1.StartExternalIdentityLinkRequest{
		ProviderId:   "discord-main",
		RedirectPath: "/chat/-/settings/account",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("StartExternalIdentityLink without credential code = %v, want failed_precondition", connect.CodeOf(err))
	}
	started, err := env.account.StartExternalIdentityLink(createdCtx, connect.NewRequest(&apiv1.StartExternalIdentityLinkRequest{
		ProviderId:   "discord-main",
		RedirectPath: "/chat/-/settings/account",
	}))
	if err != nil {
		t.Fatalf("StartExternalIdentityLink: %v", err)
	}
	startURL, err := url.Parse(started.Msg.GetStartUrl())
	if err != nil {
		t.Fatalf("start url parse: %v", err)
	}
	if startURL.Path != "/auth/providers/discord-main" || startURL.Query().Get("intent") != "link" || startURL.Query().Get("link_start") == "" {
		t.Fatalf("start url = %q", started.Msg.GetStartUrl())
	}
	linkStart, err := env.core.ConsumePendingExternalIdentityLinkStart(env.ctx, startURL.Query().Get("link_start"))
	if err != nil {
		t.Fatalf("ConsumePendingExternalIdentityLinkStart: %v", err)
	}
	if linkStart.BoundUserID != created.Msg.GetUserId() || linkStart.ProviderID != "discord-main" || linkStart.RedirectPath != "/chat/-/settings/account" {
		t.Fatalf("link start = %+v", linkStart)
	}

	linkToken, err := env.core.CreatePendingExternalIdentityLinkFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:   "discord-main",
		ProviderType: config.AuthProviderTypeDiscord,
		Issuer:       "discord-main",
		Subject:      "abc123",
	}, env.viewer.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkFlow: %v", err)
	}
	_, err = env.account.LinkExternalIdentity(createdCtx, connect.NewRequest(&apiv1.LinkExternalIdentityRequest{Token: linkToken}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	linked, err := env.account.LinkExternalIdentity(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.LinkExternalIdentityRequest{
		Token: linkToken,
	}))
	if err != nil {
		t.Fatalf("LinkExternalIdentity: %v", err)
	}
	if linked.Msg.LinkedIdentity.GetProviderId() != "discord-main" || linked.Msg.LinkedIdentity.GetSubjectHash() == "" {
		t.Fatalf("linked identity = %+v", linked.Msg.LinkedIdentity)
	}
	oauthViewerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, env.viewer.Id, "oauth_code_exchange")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource oauth viewer: %v", err)
	}
	oauthCredentialCtx := withBearerCredential(env.ctx, env.viewer, oauthViewerToken)
	_, err = env.account.DisconnectExternalIdentity(oauthCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash:     linked.Msg.LinkedIdentity.GetSubjectHash(),
		CurrentPassword: "password",
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	staleViewerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, env.viewer.Id, "unknown")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource stale viewer: %v", err)
	}
	viewerCredentialCtx := withBearerCredential(env.ctx, env.viewer, staleViewerToken)
	_, err = env.account.DisconnectExternalIdentity(viewerCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash: linked.Msg.LinkedIdentity.GetSubjectHash(),
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)
	disconnected, err := env.account.DisconnectExternalIdentity(viewerCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash:     linked.Msg.LinkedIdentity.GetSubjectHash(),
		CurrentPassword: "password",
	}))
	if err != nil {
		t.Fatalf("DisconnectExternalIdentity: %v", err)
	}
	if !disconnected.Msg.GetDisconnected() {
		t.Fatalf("DisconnectExternalIdentity disconnected = false")
	}
	found, err := env.core.GetUserByExternalIdentity(env.ctx, "discord-main", "abc123")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity after disconnect: %v", err)
	}
	if found != nil {
		t.Fatalf("GetUserByExternalIdentity after disconnect = %+v, want nil", found)
	}
	if _, err := env.core.GetPendingExternalIdentityFlow(env.ctx, linkToken); !errors.Is(err, core.ErrExternalIdentityFlowNotFound) {
		t.Fatalf("pending link flow after confirmation error = %v, want ErrExternalIdentityFlowNotFound", err)
	}

	publicLinkToken, err := env.core.CreatePendingExternalIdentityLinkFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:   "gitlab-main",
		ProviderType: config.AuthProviderTypeGitLab,
		Issuer:       "gitlab-main",
		Subject:      "gitlab-123",
	}, env.viewer.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkFlow public: %v", err)
	}
	publicLinked, err := env.externalAuth.ConfirmExternalIdentityLink(env.ctx, connect.NewRequest(&authv1.ConfirmExternalIdentityLinkRequest{
		Token: publicLinkToken,
	}))
	if err != nil {
		t.Fatalf("ConfirmExternalIdentityLink: %v", err)
	}
	if publicLinked.Msg.LinkedIdentity.GetProviderId() != "gitlab-main" {
		t.Fatalf("public linked identity = %+v", publicLinked.Msg.LinkedIdentity)
	}
}

func TestExternalIdentityCreateDisplayName(t *testing.T) {
	tests := []struct {
		name  string
		login string
		hint  string
		want  string
	}{
		{
			name:  "valid hint",
			login: "sso-user",
			hint:  "SSO User",
			want:  "SSO User",
		},
		{
			name:  "empty hint falls back",
			login: "sso-user",
			hint:  " ",
			want:  "sso-user",
		},
		{
			name:  "invalid punctuation falls back",
			login: "sso-user",
			hint:  "User, Inc.",
			want:  "sso-user",
		},
		{
			name:  "too long falls back",
			login: "sso-user",
			hint:  strings.Repeat("A", core.MaxDisplayNameLength+1),
			want:  "sso-user",
		},
		{
			name:  "invalid start falls back",
			login: "sso-user",
			hint:  "😀 User",
			want:  "sso-user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := externalIdentityCreateDisplayName(tt.login, tt.hint); got != tt.want {
				t.Fatalf("externalIdentityCreateDisplayName(%q, %q) = %q, want %q", tt.login, tt.hint, got, tt.want)
			}
		})
	}
}

func TestOperatorUserServiceLifecycle(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	createResp, err := operator.CreateUser(env.ctx, connect.NewRequest(&operatorv1.CreateUserRequest{
		Login:         "operator-api-user",
		DisplayName:   "Operator API User",
		Password:      "password123",
		VerifiedEmail: "operator-api@example.com",
		RoleNames:     []string{core.RoleAdmin},
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	member := createResp.Msg.GetMember()
	user := member.GetUser()
	if user.GetId() == "" || user.GetLogin() != "operator-api-user" || user.GetDisplayName() != "Operator API User" {
		t.Fatalf("created member = %+v", member)
	}
	if got := strings.Join(member.GetRoles(), ","); got != core.RoleAdmin {
		t.Fatalf("created roles = %q, want %s", got, core.RoleAdmin)
	}
	if got := len(member.GetVerifiedEmails()); got != 1 {
		t.Fatalf("created verified email count = %d, want 1", got)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "operator-api-user", "password123"); err != nil {
		t.Fatalf("VerifyPassword initial: %v", err)
	}

	getByLoginResp, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Login: "operator-api-user"}))
	if err != nil {
		t.Fatalf("GetUser by login: %v", err)
	}
	if getByLoginResp.Msg.GetMember().GetUser().GetId() != user.GetId() {
		t.Fatalf("GetMember by login id = %q, want %q", getByLoginResp.Msg.GetMember().GetUser().GetId(), user.GetId())
	}
	getByEmailResp, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Email: "operator-api@example.com"}))
	if err != nil {
		t.Fatalf("GetUser by email: %v", err)
	}
	if getByEmailResp.Msg.GetMember().GetUser().GetId() != user.GetId() {
		t.Fatalf("GetMember by email id = %q, want %q", getByEmailResp.Msg.GetMember().GetUser().GetId(), user.GetId())
	}
	if _, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Email: "missing-operator-api@example.com"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser by email err = %v, want not found", err)
	}

	updateResp, err := operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		Login:       stringPtr("operator-api-renamed"),
		DisplayName: stringPtr("Operator API Renamed"),
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got := updateResp.Msg.GetMember().GetUser().GetLogin(); got != "operator-api-renamed" {
		t.Fatalf("updated login = %q, want operator-api-renamed", got)
	}

	if _, err := operator.SetUserPassword(env.ctx, connect.NewRequest(&operatorv1.SetUserPasswordRequest{
		UserId:   user.GetId(),
		Password: "newpassword123",
	})); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "operator-api-renamed", "newpassword123"); err != nil {
		t.Fatalf("VerifyPassword updated: %v", err)
	}

	emailResp, err := operator.AddVerifiedEmail(env.ctx, connect.NewRequest(&operatorv1.AddVerifiedEmailRequest{
		UserId: user.GetId(),
		Email:  "operator-api-alt@example.com",
	}))
	if err != nil {
		t.Fatalf("AddVerifiedEmail: %v", err)
	}
	if got := len(emailResp.Msg.GetMember().GetVerifiedEmails()); got != 2 {
		t.Fatalf("verified email count = %d, want 2", got)
	}

	if _, err := operator.AssignRole(env.ctx, connect.NewRequest(&operatorv1.AssignRoleRequest{
		UserId:   user.GetId(),
		RoleName: core.RoleModerator,
	})); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	roleResp, err := operator.RevokeRole(env.ctx, connect.NewRequest(&operatorv1.RevokeRoleRequest{
		UserId:   user.GetId(),
		RoleName: core.RoleAdmin,
	}))
	if err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if got := strings.Join(roleResp.Msg.GetMember().GetRoles(), ","); got != core.RoleModerator {
		t.Fatalf("roles after revoke = %q, want %s", got, core.RoleModerator)
	}

	if _, err := operator.DeleteUser(env.ctx, connect.NewRequest(&operatorv1.DeleteUserRequest{UserId: user.GetId()})); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{UserId: user.GetId()})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetUser after delete err = %v, want not found", err)
	}
}

func TestAdminUserServiceSelfCannotDeleteAccountFromMemberDetails(t *testing.T) {
	env := newConnectAPITestEnv(t)
	admin := &adminUserManagementService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-api-self-delete", "Admin API Self Delete", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, user.GetId()); err != nil {
		t.Fatalf("AssignAdminRole setup: %v", err)
	}

	details, err := admin.GetMember(withCaller(env.ctx, user), connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: user.GetId()},
	}))
	if err != nil {
		t.Fatalf("GetMember self: %v", err)
	}
	if details.Msg.GetMember().GetViewerCanDeleteAccount() {
		t.Fatalf("ViewerCanDeleteAccount for self = true, want false")
	}
}

func TestOperatorUserServiceListUsesSharedPageInfo(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	for i := 1; i <= 2; i++ {
		login := fmt.Sprintf("operator-api-page-%d", i)
		if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, login, "Admin API Page", "password123"); err != nil {
			t.Fatalf("CreateUser %s: %v", login, err)
		}
	}

	defaultResp, err := operator.ListUsers(env.ctx, connect.NewRequest(&operatorv1.ListUsersRequest{Search: "operator-api-page"}))
	if err != nil {
		t.Fatalf("ListMembers default page: %v", err)
	}
	if got := len(defaultResp.Msg.GetUsers()); got != 2 {
		t.Fatalf("default ListMembers users = %d, want 2", got)
	}
	if page := defaultResp.Msg.GetPage(); page.GetTotalCount() != 2 || page.GetHasMore() {
		t.Fatalf("default ListMembers page = %+v, want total 2 has_more false", page)
	}

	firstPageResp, err := operator.ListUsers(env.ctx, connect.NewRequest(&operatorv1.ListUsersRequest{
		Search: "operator-api-page",
		Page:   &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil {
		t.Fatalf("ListMembers first page: %v", err)
	}
	if got := len(firstPageResp.Msg.GetUsers()); got != 1 {
		t.Fatalf("first page ListMembers users = %d, want 1", got)
	}
	if page := firstPageResp.Msg.GetPage(); page.GetTotalCount() != 2 || !page.GetHasMore() {
		t.Fatalf("first page ListMembers page = %+v, want total 2 has_more true", page)
	}
}

func TestOperatorUserServiceUpdateUserValidatesAllFieldsBeforeWriting(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-update-rollback", "Original Display", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}

	_, err = operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		DisplayName: stringPtr("Changed Display"),
		Login:       stringPtr("bad login"),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateUser error = %v, want invalid argument", err)
	}

	got, err := env.core.GetUser(env.ctx, user.GetId())
	if err != nil {
		t.Fatalf("GetUser after rollback: %v", err)
	}
	if got.GetDisplayName() != "Original Display" {
		t.Fatalf("display name after rollback = %q, want Original Display", got.GetDisplayName())
	}
	if got.GetLogin() != "operator-api-update-rollback" {
		t.Fatalf("login after rollback = %q, want operator-api-update-rollback", got.GetLogin())
	}
}

func TestOperatorUserServiceUpdateUserEventsUseSystemActor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-actor", "Operator Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}

	if _, err := operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		Login:       stringPtr("operator-api-actor-renamed"),
		DisplayName: stringPtr("Operator Actor Renamed"),
	})); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	loginEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, events.UserAggregate(user.GetId()).Subject(events.EventUserLoginChanged))
	if err != nil {
		t.Fatalf("SubjectEvents login changed: %v", err)
	}
	if len(loginEvents) != 1 {
		t.Fatalf("login changed events = %d, want 1", len(loginEvents))
	}
	if got := loginEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("login changed actor = %q, want %q", got, core.SystemActorID)
	}

	displayEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, events.UserAggregate(user.GetId()).Subject(events.EventUserDisplayNameChanged))
	if err != nil {
		t.Fatalf("SubjectEvents display name changed: %v", err)
	}
	if len(displayEvents) != 1 {
		t.Fatalf("display name changed events = %d, want 1", len(displayEvents))
	}
	if got := displayEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("display name changed actor = %q, want %q", got, core.SystemActorID)
	}
}

func TestOperatorUserServiceClearUsernameCooldownUsesSystemActor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-cooldown", "Operator API Cooldown", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin setup: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-blocked"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("second UpdateUserLogin error = %v, want cooldown", err)
	}

	resp, err := operator.ClearUsernameCooldown(env.ctx, connect.NewRequest(&operatorv1.ClearUsernameCooldownRequest{
		UserId: user.GetId(),
	}))
	if err != nil {
		t.Fatalf("ClearUsernameCooldown: %v", err)
	}
	if !resp.Msg.GetCleared() {
		t.Fatal("Cleared = false, want true")
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-unblocked"); err != nil {
		t.Fatalf("UpdateUserLogin after clear: %v", err)
	}

	clearEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, events.UserAggregate(user.GetId()).Subject(events.EventUserLoginCooldownCleared))
	if err != nil {
		t.Fatalf("SubjectEvents login cooldown cleared: %v", err)
	}
	if len(clearEvents) != 1 {
		t.Fatalf("login cooldown cleared events = %d, want 1", len(clearEvents))
	}
	if got := clearEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("login cooldown cleared actor = %q, want %q", got, core.SystemActorID)
	}
}

func TestOperatorUserServiceAddVerifiedEmailRejectsMissingUserWithoutClaimingEmail(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	_, err := operator.AddVerifiedEmail(env.ctx, connect.NewRequest(&operatorv1.AddVerifiedEmailRequest{
		UserId: "UmissingVerifiedEmail",
		Email:  "missing-operator@example.com",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("AddVerifiedEmail error = %v, want not found", err)
	}
	if claimed, err := env.core.IsEmailClaimed(env.ctx, "missing-operator@example.com"); err != nil || claimed {
		t.Fatalf("IsEmailClaimed after missing user add = %t, %v; want false, nil", claimed, err)
	}
}

func TestOperatorUserServiceAssignRoleRejectsMissingUserWithoutPersistingRole(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	_, err := operator.AssignRole(env.ctx, connect.NewRequest(&operatorv1.AssignRoleRequest{
		UserId:   "UmissingAdminUser",
		RoleName: core.RoleAdmin,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("AssignRole error = %v, want not found", err)
	}
	if env.core.RBAC.HasRole("UmissingAdminUser", core.RoleAdmin) {
		t.Fatal("missing user was assigned admin role despite NotFound response")
	}

	beforeRevocations, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, events.RBACAggregate().Subject(events.EventRBACRoleRevoked))
	if err != nil {
		t.Fatalf("SubjectEvents role revoked before: %v", err)
	}
	_, err = operator.RevokeRole(env.ctx, connect.NewRequest(&operatorv1.RevokeRoleRequest{
		UserId:   "UmissingAdminUser",
		RoleName: core.RoleAdmin,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("RevokeRole error = %v, want not found", err)
	}
	afterRevocations, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, events.RBACAggregate().Subject(events.EventRBACRoleRevoked))
	if err != nil {
		t.Fatalf("SubjectEvents role revoked after: %v", err)
	}
	if len(afterRevocations) != len(beforeRevocations) {
		t.Fatalf("role revocation events changed from %d to %d for missing user", len(beforeRevocations), len(afterRevocations))
	}
}

func TestUserServiceGetUserReadsPublicUsers(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.users.GetUser(env.ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
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
	offlineResp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: offlineUser.Id}}))
	if err != nil {
		t.Fatalf("GetUser offline profile: %v", err)
	}
	if offlineResp.Msg.GetUser().GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("offline profile presence = %v, want OFFLINE", offlineResp.Msg.GetUser().GetUser().GetPresenceStatus())
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

	resp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}}))
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	userRow := resp.Msg.GetUser()
	user := userRow.GetUser()
	if user.GetId() != env.viewer.Id || user.GetLogin() != env.viewer.Login || user.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("GetUser user = %+v, want viewer public profile", user)
	}
	if user.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE {
		t.Fatalf("PresenceStatus = %v, want ONLINE", user.GetPresenceStatus())
	}
	if user.GetCustomStatus().GetText() != "around" {
		t.Fatalf("CustomStatus = %+v, want status text", user.GetCustomStatus())
	}
	if roles := strings.Join(userRow.GetRoles(), ","); roles != "everyone,admin" {
		t.Fatalf("GetUser roles = %q, want everyone,admin", roles)
	}
	batchResp, err := env.users.BatchGetUsers(ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{
		UserIds: []string{env.viewer.Id, "missing-user", env.viewer.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if got := batchResp.Msg.GetUsers(); len(got) != 1 {
		t.Fatalf("BatchGetUsers len = %d, want 1: %+v", len(got), got)
	} else if got[0].GetUser().GetId() != env.viewer.Id || got[0].GetUser().GetLogin() != env.viewer.Login || got[0].GetUser().GetDisplayName() != env.viewer.DisplayName || got[0].GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE {
		t.Fatalf("BatchGetUsers user = %+v, want viewer user profile", got[0])
	}

	byLoginResp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_Login{Login: env.viewer.Login}}))
	if err != nil {
		t.Fatalf("GetUser by login: %v", err)
	}
	if byLoginResp.Msg.GetUser().GetUser().GetId() != env.viewer.Id {
		t.Fatalf("GetUser by login id = %q, want %q", byLoginResp.Msg.GetUser().GetUser().GetId(), env.viewer.Id)
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_Login{Login: "missing-user"}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser by login code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: "missing-user"}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing GetUser target code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestAdminRoleServiceManagesRoles(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.roles.ListRoles(env.ctx, connect.NewRequest(&adminv1.ListRolesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.ListRoles(env.ctx, connect.NewRequest(&apiv1.ListRolesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public ListRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.GetRole(env.ctx, connect.NewRequest(&apiv1.GetRoleRequest{Name: core.RoleEveryone})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public GetRole code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.BatchGetRoles(env.ctx, connect.NewRequest(&apiv1.BatchGetRolesRequest{Names: []string{core.RoleEveryone}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public BatchGetRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}

	publicListResp, err := env.publicRoles.ListRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRolesRequest{}))
	if err != nil {
		t.Fatalf("public ListRoles regular: %v", err)
	}
	if len(publicListResp.Msg.GetRoles()) < 4 {
		t.Fatalf("public ListRoles regular len = %d, want default roles", len(publicListResp.Msg.GetRoles()))
	}
	if publicListResp.Msg.GetRoles()[0].GetName() == "" {
		t.Fatalf("public ListRoles first role = %+v, want role metadata", publicListResp.Msg.GetRoles()[0])
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
	if got := createResp.Msg.GetRole().GetRole(); got.GetName() != "helpdesk" || !got.GetPingable() {
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
	publicGetResp, err := env.publicRoles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("public GetRole: %v", err)
	}
	if got := publicGetResp.Msg.GetRole(); got.GetName() != "helpdesk" || got.GetDisplayName() != "Helpdesk" || !got.GetPingable() {
		t.Fatalf("public GetRole role = %+v, want helpdesk metadata", got)
	}
	if _, err := env.publicRoles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoleRequest{Name: "missing-role"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing public GetRole code = %v, want not found", connect.CodeOf(err))
	}
	publicBatchResp, err := env.publicRoles.BatchGetRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.BatchGetRolesRequest{
		Names: []string{"helpdesk", "missing-role", core.RoleEveryone, "helpdesk"},
	}))
	if err != nil {
		t.Fatalf("public BatchGetRoles: %v", err)
	}
	if got := publicBatchResp.Msg.GetRoles(); len(got) != 2 || got[0].GetName() != "helpdesk" || got[1].GetName() != core.RoleEveryone {
		t.Fatalf("public BatchGetRoles roles = %+v, want helpdesk,everyone", got)
	}
	reorderResp, err := env.roles.ReorderRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ReorderRolesRequest{
		RoleNames: []string{"triage", "helpdesk"},
	}))
	if err != nil {
		t.Fatalf("ReorderRoles: %v", err)
	}
	var customOrder []string
	for _, role := range reorderResp.Msg.GetRoles() {
		if !role.GetRole().GetIsSystem() {
			customOrder = append(customOrder, role.GetRole().GetName())
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

	if _, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name: "helpdesk",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateRole code = %v, want invalid argument", connect.CodeOf(err))
	}
	pingable := false
	updateResp, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name:        "helpdesk",
		DisplayName: stringPtr("Support"),
		Description: stringPtr("Support team"),
		Pingable:    &pingable,
	}))
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updateResp.Msg.GetRole().GetRole().GetDisplayName() != "Support" || updateResp.Msg.GetRole().GetRole().GetPingable() {
		t.Fatalf("updated role = %+v, want Support pingable false", updateResp.Msg.GetRole())
	}
	partialRoleResp, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name:        "helpdesk",
		Description: stringPtr("Escalation queue"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRole: %v", err)
	}
	if got := partialRoleResp.Msg.GetRole().GetRole(); got.GetDisplayName() != "Support" || got.GetDescription() != "Escalation queue" || got.GetPingable() {
		t.Fatalf("partial role = %+v, want preserved display/pingable and updated description", got)
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
	if decision := setResp.Msg.GetDecision(); decision.GetPermission() != string(core.PermMessagePost) || decision.GetDecision() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW || decision.GetScope().GetKind() != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER {
		t.Fatalf("SetRolePermission decision = %+v, want server allow", decision)
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
	roleDecisionsResp, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("ListRolePermissionDecisions: %v", err)
	}
	if roleDecisionsResp.Msg.GetRoleName() != core.RoleModerator {
		t.Fatalf("role decisions role name = %q, want %q", roleDecisionsResp.Msg.GetRoleName(), core.RoleModerator)
	}
	if decision := findAPIPermissionDecision(roleDecisionsResp.Msg.GetDecisions(), adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, "", string(core.PermMessagePost)); decision == nil || decision.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW {
		t.Fatalf("server message.post decision = %+v, want allow override", decision)
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
	if decision := clearResp.Msg.GetDecision(); decision.GetPermission() != string(core.PermMessagePost) || decision.GetDecision() != adminv1.PermissionDecision_PERMISSION_DECISION_NONE || decision.GetScope().GetKind() != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER {
		t.Fatalf("SetRolePermission clear decision = %+v, want server none", decision)
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
	if _, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
		UserId: "missing-user",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUserPermissionMatrix code = %v, want not found", connect.CodeOf(err))
	}
	userDecisionsResp, err := env.permissions.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("ListUserPermissionDecisions: %v", err)
	}
	if userDecisionsResp.Msg.GetUserId() != target.Id {
		t.Fatalf("user decisions user ID = %q, want %q", userDecisionsResp.Msg.GetUserId(), target.Id)
	}
	if decision := findAPIPermissionDecision(userDecisionsResp.Msg.GetDecisions(), adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, "", string(core.PermAdminUsersView)); decision == nil || decision.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY {
		t.Fatalf("user server admin.users.view decision = %+v, want deny override", decision)
	}
	if _, err := env.permissions.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{
		UserId: "missing-user",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing ListUserPermissionDecisions code = %v, want not found", connect.CodeOf(err))
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

func TestRoomDirectoryServiceListRoomGroupsIncludesSidebarItems(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	room := env.createJoinedRoom("layout-room")
	link, err := env.core.CreateSidebarLink(env.ctx, core.SystemActorID, groupID, "Docs", "/docs")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	resp, err := env.directory.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRoomGroupsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomGroups: %v", err)
	}

	group := findDirectoryGroup(resp.Msg.GetGroups(), groupID)
	if group == nil {
		t.Fatalf("group %q missing from response", groupID)
	}
	if !roomGroupItemsContainRoom(group.GetItems(), room.Id) {
		t.Fatalf("room %q missing from group items", room.Id)
	}
	if !roomGroupItemsContainSidebarLink(group.GetItems(), link.Id) {
		t.Fatalf("sidebar link %q missing from group items", link.Id)
	}
}

func TestAdminRoomLayoutServiceCreateRoomGroupRequiresRoleManage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-member", "Layout Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	_, err = env.adminLayout.ListRoomGroups(withCaller(env.ctx, member), connect.NewRequest(&adminv1.ListRoomGroupsRequest{}))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	_, err = env.adminLayout.CreateRoomGroup(withCaller(env.ctx, member), connect.NewRequest(&adminv1.CreateRoomGroupRequest{
		Name:        "Operations",
		Description: "Private operations rooms",
	}))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	if _, err := env.adminLayout.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListRoomGroupsRequest{})); err != nil {
		t.Fatalf("ListRoomGroups: %v", err)
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
	if _, err := env.adminLayout.UpdateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomGroupRequest{
		GroupId: resp.Msg.GetGroup().GetId(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateRoomGroup code = %v, want invalid argument", connect.CodeOf(err))
	}
	partialResp, err := env.adminLayout.UpdateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomGroupRequest{
		GroupId:     resp.Msg.GetGroup().GetId(),
		Description: stringPtr("Updated operations description"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRoomGroup: %v", err)
	}
	if got := partialResp.Msg.GetGroup(); got.GetName() != "Operations" || got.GetDescription() != "Updated operations description" {
		t.Fatalf("partial group = %+v, want preserved name and updated description", got)
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
	if _, err := env.adminLayout.UpdateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateSidebarLinkRequest{
		LinkId: resp.Msg.GetSidebarLink().GetId(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateSidebarLink code = %v, want invalid argument", connect.CodeOf(err))
	}
	partialResp, err := env.adminLayout.UpdateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateSidebarLinkRequest{
		LinkId: resp.Msg.GetSidebarLink().GetId(),
		Url:    stringPtr("/health"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateSidebarLink: %v", err)
	}
	if got := partialResp.Msg.GetSidebarLink(); got.GetLabel() != "Status" || got.GetUrl() != "/health" {
		t.Fatalf("partial sidebar link = %+v, want preserved label and updated URL", got)
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
	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantServerPermission role.assign: %v", err)
	}

	resp, err := env.viewerService.GetViewer(ctx, connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer: %v", err)
	}
	user := resp.Msg.GetUser()
	profile := user.GetProfile()
	if profile.GetId() != env.viewer.Id || profile.GetLogin() != env.viewer.Login || profile.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("viewer user = %+v, want id/login/display name from fixture", user)
	}
	if !user.GetHasVerifiedEmail() {
		t.Fatal("HasVerifiedEmail = false, want true")
	}
	if !user.GetHasPassword() {
		t.Fatal("HasPassword = false, want true")
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
			if pref.GetPreference().GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || pref.GetPreference().GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
				t.Fatalf("room notification preference = %+v, want all/all", pref)
			}
		}
	}
	if !foundRoomPref {
		t.Fatalf("room notification preferences did not include %s: %+v", room.Id, resp.Msg.GetRoomNotificationPreferences())
	}
	if caps := resp.Msg.GetCapabilities(); !apiCapabilityGranted(caps.GetGrants(), viewerCapabilityAssignRoles) || apiCapabilityGranted(caps.GetGrants(), viewerCapabilityAdminManageUsers) {
		t.Fatalf("viewer capabilities = %+v, want role.assign true and account management false", caps.GetGrants())
	}
	if apiCapabilityGranted(resp.Msg.GetCapabilities().GetGrants(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("viewer system capability = true for regular viewer, want false")
	}
	if resp.Msg.GetViewerPermissions() == nil {
		t.Fatal("ViewerPermissions = nil")
	}
	if got, want := len(resp.Msg.GetViewerPermissions().GetPermissions()), len(core.AllPermissions()); got != want {
		t.Fatalf("viewer permissions len = %d, want %d", got, want)
	}
	if apiPermissionGrantPresent(resp.Msg.GetViewerPermissions().GetPermissions(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("%s should be exposed only as an owner-only capability, not as a permission grant", viewerCapabilityAdminViewSystem)
	}
	if resp.Msg.GetViewerState() == nil {
		t.Fatal("ViewerState = nil")
	}

	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "viewer-owner", "Viewer Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := env.core.AssignOwnerRole(env.ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	ownerResp, err := env.viewerService.GetViewer(withCaller(env.ctx, owner), connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer owner: %v", err)
	}
	if !apiCapabilityGranted(ownerResp.Msg.GetCapabilities().GetGrants(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("owner system capability = false, want true")
	}
	if apiPermissionGrantPresent(ownerResp.Msg.GetViewerPermissions().GetPermissions(), viewerCapabilityAdminViewSystem) {
		t.Fatalf("%s should not be exposed as a permission grant for owners", viewerCapabilityAdminViewSystem)
	}
}

func TestMyAccountServiceUpdatesSelfProfileAndSettings(t *testing.T) {
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

func TestMyAccountServiceSetsPassword(t *testing.T) {
	env := newConnectAPITestEnv(t)
	passwordless, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-passwordless", "Connect Passwordless", "")
	if err != nil {
		t.Fatalf("CreateUser passwordless: %v", err)
	}
	ctx := withCaller(env.ctx, passwordless)
	freshToken, err := env.core.CreateAuthTokenWithSource(env.ctx, passwordless.Id, "test_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}
	freshCtx := withBearerCredential(env.ctx, passwordless, freshToken)
	oauthToken, err := env.core.CreateAuthTokenWithSource(env.ctx, passwordless.Id, "oauth_code_exchange")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource oauth: %v", err)
	}
	oauthCtx := withBearerCredential(env.ctx, passwordless, oauthToken)

	if _, err := env.account.UpdatePassword(env.ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdatePassword code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdatePassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "short",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("short UpdatePassword code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdatePassword without fresh credential code = %v, want failed_precondition", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(oauthCtx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdatePassword with OAuth token code = %v, want failed_precondition", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(freshCtx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "newpassword456",
	})); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, passwordless.Login, "newpassword456"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password: "anotherpassword456",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("second UpdatePassword without current code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password:        "anotherpassword456",
		CurrentPassword: "wrongpassword",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("second UpdatePassword wrong current code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.account.UpdatePassword(ctx, connect.NewRequest(&apiv1.UpdatePasswordRequest{
		Password:        "anotherpassword456",
		CurrentPassword: "newpassword456",
	})); err != nil {
		t.Fatalf("UpdatePassword with current: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, passwordless.Login, "anotherpassword456"); err != nil {
		t.Fatalf("VerifyPassword changed: %v", err)
	}
}

func TestMyAccountServiceDeletesAvatarAndAccount(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.account.UploadAvatar(env.ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadAvatar code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadAvatar code = %v, want invalid_argument", connect.CodeOf(err))
	}

	uploadAvatarResp, err := env.account.UploadAvatar(ctx, connect.NewRequest(&apiv1.UploadAvatarRequest{
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "avatar.png",
			ContentType: "image/png",
		},
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

func TestAdminUserServiceUpdatesUsersAndClearsCooldown(t *testing.T) {
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
	if _, err := env.adminUsers.UpdateUserPassword(env.ctx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateUserPassword code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(env.ctx, connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated DeleteUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Denied"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular UpdateUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular DeleteUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "admin-user-regular-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin regular: %v", err)
	}
	if _, err := env.adminUsers.UpdateUser(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId: regular.Id,
		Login:  stringPtr("admin-user-regular-bypass"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular self UpdateUser code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "admin-user-regular-cooldown"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("regular cooldown after denied self UpdateUser err = %v, want cooldown", err)
	}
	if _, err := env.adminUsers.ClearUsernameCooldown(withCaller(env.ctx, regular), connect.NewRequest(&adminv1.ClearUsernameCooldownRequest{
		UserId: regular.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular self ClearUsernameCooldown code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, regular.Id, "regular-still-cooldown"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("regular cooldown after denied self clear err = %v, want cooldown", err)
	}

	roleAssigner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-role-assigner", "Admin User Role Assigner", "password")
	if err != nil {
		t.Fatalf("CreateUser role assigner: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, roleAssigner), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("role.assign-only UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
	}

	accountManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-account-manager", "Admin User Account Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser account manager: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, accountManager.Id, core.PermUserManageAccounts); err != nil {
		t.Fatalf("GrantUserPermission user.manage-accounts: %v", err)
	}
	if _, err := env.adminUsers.GetMember(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("account manager GetMember code = %v, want permission_denied", connect.CodeOf(err))
	}
	accountUpdateResp, err := env.adminUsers.UpdateUser(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId:      target.Id,
		DisplayName: stringPtr("Account Managed Target"),
	}))
	if err != nil {
		t.Fatalf("account manager UpdateUser: %v", err)
	}
	if accountUpdateResp.Msg.GetUser().GetDisplayName() != "Account Managed Target" {
		t.Fatalf("account manager UpdateUser user display name = %q, want Account Managed Target", accountUpdateResp.Msg.GetUser().GetDisplayName())
	}
	if member := accountUpdateResp.Msg.GetMember(); member.GetUser().GetId() != target.Id || member.GetUser().GetDisplayName() != "Account Managed Target" {
		t.Fatalf("account manager UpdateUser member = %+v, want updated target", member)
	}
	if _, err := env.adminUsers.UpdateUserPassword(withCaller(env.ctx, accountManager), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "accountmanagerpass456",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("account manager stale UpdateUserPassword code = %v, want failed_precondition", connect.CodeOf(err))
	}
	accountManagerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, accountManager.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource account manager: %v", err)
	}
	accountManagerResp, err := env.adminUsers.UpdateUserPassword(withBearerCredential(env.ctx, accountManager, accountManagerToken), connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "accountmanagerpass456",
	}))
	if err != nil {
		t.Fatalf("account manager UpdateUserPassword: %v", err)
	}
	if got := accountManagerResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("account manager password member ID = %q, want %q", got, target.Id)
	}
	if _, err := env.core.VerifyPassword(env.ctx, target.Login, "accountmanagerpass456"); err != nil {
		t.Fatalf("account-manager-set password should verify: %v", err)
	}

	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-user-admin", "Admin User Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	adminToken, err := env.core.CreateAuthTokenWithSource(env.ctx, admin.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource admin: %v", err)
	}
	adminCtx := withBearerCredential(env.ctx, admin, adminToken)

	if _, err := env.adminUsers.UpdateUser(adminCtx, connect.NewRequest(&adminv1.UpdateUserRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateUser code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId: target.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateUserPassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.DeleteUser(adminCtx, connect.NewRequest(&adminv1.DeleteUserRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty DeleteUser code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "short",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("short UpdateUserPassword code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   admin.Id,
		Password: "newpassword456",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self UpdateUserPassword code = %v, want permission_denied", connect.CodeOf(err))
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
	passwordResp, err := env.adminUsers.UpdateUserPassword(adminCtx, connect.NewRequest(&adminv1.UpdateUserPasswordRequest{
		UserId:   target.Id,
		Password: "adminpassword456",
	}))
	if err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	if got := passwordResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("password member ID = %q, want %q", got, target.Id)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "managed-target", "adminpassword456"); err != nil {
		t.Fatalf("admin password should verify: %v", err)
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
	deleteResp, err := env.adminUsers.DeleteUser(adminCtx, connect.NewRequest(&adminv1.DeleteUserRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("Deleted = false, want true")
	}
	if _, err := env.core.GetUser(env.ctx, target.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after DeleteUser err = %v, want not found", err)
	}
}

func TestAdminUserServiceListsAndGetsMembers(t *testing.T) {
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
	if _, err := env.adminUsers.BatchGetMembers(env.ctx, connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{target.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetMembers code = %v, want unauthenticated", connect.CodeOf(err))
	}

	regularCtx := withCaller(env.ctx, regular)
	if _, err := env.adminUsers.ListMembers(regularCtx, connect.NewRequest(&adminv1.ListMembersRequest{
		Search: "target",
		Page:   &apiv1.PageRequest{Limit: 10},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular ListMembers code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.GetMember(regularCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular GetMember code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.adminUsers.BatchGetMembers(regularCtx, connect.NewRequest(&adminv1.BatchGetMembersRequest{UserIds: []string{target.Id}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular BatchGetMembers code = %v, want permission denied", connect.CodeOf(err))
	}

	adminCtx := withCaller(env.ctx, admin)
	batchResp, err := env.adminUsers.BatchGetMembers(adminCtx, connect.NewRequest(&adminv1.BatchGetMembersRequest{
		UserIds: []string{target.Id, "missing-user", regular.Id, target.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetMembers admin: %v", err)
	}
	if got := batchResp.Msg.GetMembers(); len(got) != 2 || got[0].GetUser().GetId() != target.Id || got[1].GetUser().GetId() != regular.Id {
		t.Fatalf("BatchGetMembers members = %+v, want target,regular", got)
	}
	batchTarget := batchResp.Msg.GetMembers()[0]
	if got := batchTarget.GetRoles(); len(got) != 1 || got[0] != core.RoleModerator {
		t.Fatalf("BatchGetMembers target roles = %v, want explicit moderator only", got)
	}
	if !batchTarget.GetHasVerifiedEmail() || len(batchTarget.GetVerifiedEmails()) != 1 || batchTarget.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("BatchGetMembers emails = has:%v emails:%v, want target email", batchTarget.GetHasVerifiedEmail(), batchTarget.GetVerifiedEmails())
	}
	if batchTarget.GetLastLoginChange() == nil {
		t.Fatal("BatchGetMembers LastLoginChange is nil, want visible cooldown timestamp")
	}
	if len(batchResp.Msg.GetRoles()) == 0 {
		t.Fatal("BatchGetMembers roles are empty")
	}

	listResp, err := env.adminUsers.ListMembers(adminCtx, connect.NewRequest(&adminv1.ListMembersRequest{
		Search: "target",
		Page:   &apiv1.PageRequest{Limit: 10},
	}))
	if err != nil {
		t.Fatalf("ListMembers admin: %v", err)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 1 || len(listResp.Msg.GetMembers()) != 1 {
		t.Fatalf("ListMembers returned %d/%d members, want 1/1", len(listResp.Msg.GetMembers()), listResp.Msg.GetPage().GetTotalCount())
	}
	listUser := listResp.Msg.GetMembers()[0]
	if listUser.GetUser().GetId() != target.Id {
		t.Fatalf("ListMembers user ID = %q, want %q", listUser.GetUser().GetId(), target.Id)
	}
	if got := listUser.GetRoles(); len(got) != 1 || got[0] != core.RoleModerator {
		t.Fatalf("ListMembers roles = %v, want explicit moderator only", got)
	}
	if !listUser.GetHasVerifiedEmail() || len(listUser.GetVerifiedEmails()) != 1 || listUser.GetVerifiedEmails()[0] != "admin-member-target@example.test" {
		t.Fatalf("ListMembers emails = has:%v emails:%v, want target email", listUser.GetHasVerifiedEmail(), listUser.GetVerifiedEmails())
	}
	if listUser.GetLastLoginChange() == nil {
		t.Fatal("ListMembers LastLoginChange is nil, want visible cooldown timestamp")
	}
	if len(listResp.Msg.GetRoles()) == 0 {
		t.Fatal("ListMembers roles are empty")
	}

	getResp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
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
	getByLoginResp, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_Login{Login: "admin-member-target-renamed"},
	}))
	if err != nil {
		t.Fatalf("GetMember by login: %v", err)
	}
	if got := getByLoginResp.Msg.GetMember().GetUser().GetId(); got != target.Id {
		t.Fatalf("GetMember by login id = %q, want %q", got, target.Id)
	}
	if _, err := env.adminUsers.GetMember(adminCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: "missing-user"},
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetMember code = %v, want not found", connect.CodeOf(err))
	}
}

func TestAdminUserServiceAssignsAndRevokesRoles(t *testing.T) {
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
	roleAssigner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-role-assigner-only", "Admin Role Assigner Only", "password")
	if err != nil {
		t.Fatalf("CreateUser role assigner: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleAssigner.Id, core.PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}
	roleAssignerCtx := withCaller(env.ctx, roleAssigner)
	if _, err := env.adminUsers.GetMember(roleAssignerCtx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: target.Id},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("role.assign-only GetMember code = %v, want permission_denied", connect.CodeOf(err))
	}
	roleAssignerResp, err := env.adminUsers.AssignRole(roleAssignerCtx, connect.NewRequest(&adminv1.AssignRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("role.assign-only AssignRole: %v", err)
	}
	if !roleAssignerResp.Msg.GetAssigned() || !stringSliceContains(roleAssignerResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("role.assign-only AssignRole response = %+v, want assigned moderator", roleAssignerResp.Msg)
	}
	roleAssignerRevokeResp, err := env.adminUsers.RevokeRole(roleAssignerCtx, connect.NewRequest(&adminv1.RevokeRoleRequest{
		UserId:   target.Id,
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("role.assign-only RevokeRole: %v", err)
	}
	if !roleAssignerRevokeResp.Msg.GetRevoked() || stringSliceContains(roleAssignerRevokeResp.Msg.GetMember().GetRoles(), core.RoleModerator) {
		t.Fatalf("role.assign-only RevokeRole response = %+v, want revoked moderator", roleAssignerRevokeResp.Msg)
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

func TestServerServiceGetMotdAndRuntimeConfig(t *testing.T) {
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
	if err := env.core.ConfigManager().SetServerConfig(env.ctx, core.SystemActorID, &configv1.ServerConfig{
		Motd: "Authenticated MOTD",
	}); err != nil {
		t.Fatalf("SetServerConfig: %v", err)
	}

	if _, err := env.serverState.GetMotd(env.ctx, connect.NewRequest(&apiv1.GetMotdRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetMotd code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetRuntimeConfig(env.ctx, connect.NewRequest(&apiv1.GetRuntimeConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRuntimeConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}

	motdResp, err := env.serverState.GetMotd(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetMotdRequest{}))
	if err != nil {
		t.Fatalf("GetMotd: %v", err)
	}
	if motdResp.Msg.GetMotd() != "Authenticated MOTD" {
		t.Fatalf("MOTD = %q, want Authenticated MOTD", motdResp.Msg.GetMotd())
	}

	runtimeResp, err := env.serverState.GetRuntimeConfig(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRuntimeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetRuntimeConfig: %v", err)
	}
	runtime := runtimeResp.Msg.GetRuntime()
	if !runtime.GetPushNotificationsEnabled() || runtime.GetVapidPublicKey() != "test-public-key" {
		t.Fatalf("push fields = enabled %v key %q, want true/test-public-key", runtime.GetPushNotificationsEnabled(), runtime.GetVapidPublicKey())
	}
	if !runtime.GetVideoProcessingEnabled() {
		t.Fatal("VideoProcessingEnabled = false, want true")
	}
	if runtime.GetLivekitUrl() != "wss://livekit.example.test" {
		t.Fatalf("LivekitUrl = %q, want configured URL", runtime.GetLivekitUrl())
	}
	if runtime.GetMaxUploadSize() <= 0 || runtime.GetMaxVideoUploadSize() <= 0 {
		t.Fatalf("upload sizes = %d/%d, want positive values", runtime.GetMaxUploadSize(), runtime.GetMaxVideoUploadSize())
	}
	if runtime.GetMessageEditWindowSeconds() != int32(core.MessageEditWindow/time.Second) {
		t.Fatalf("MessageEditWindowSeconds = %d, want %d", runtime.GetMessageEditWindowSeconds(), int32(core.MessageEditWindow/time.Second))
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
	if _, err := env.serverState.GetServerConfig(env.ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetServerConfig code = %v, want unauthenticated", connect.CodeOf(err))
	}

	if _, err := env.serverState.UpdateServerConfig(ctx, connect.NewRequest(&adminv1.UpdateServerConfigRequest{
		ServerName: stringPtr("Nope"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UpdateServerConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}
	if _, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("GetServerConfig without permission code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantServerPermission(env.ctx, core.SystemActorID, core.RoleEveryone, core.PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission manage server: %v", err)
	}

	initialResp, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerConfig: %v", err)
	}
	if initialResp.Msg.GetConfig().GetServerName() != "" || initialResp.Msg.GetProfile().GetPublicProfile().GetName() != "Chatto" {
		t.Fatalf("initial server config response = %+v", initialResp.Msg)
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
	if profile.GetPublicProfile().GetName() != "Connect Settings" ||
		profile.GetPublicProfile().GetDescription() != "Description from Connect" ||
		profile.GetMotd() != "MOTD from Connect" ||
		profile.GetPublicProfile().GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("updated profile = %+v", profile)
	}
	if resp.Msg.GetConfig().GetServerName() != "Connect Settings" ||
		resp.Msg.GetConfig().GetDescription() != "Description from Connect" ||
		resp.Msg.GetConfig().GetMotd() != "MOTD from Connect" ||
		resp.Msg.GetConfig().GetWelcomeMessage() != "Welcome from Connect" {
		t.Fatalf("updated config response = %+v", resp.Msg.GetConfig())
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
	getResp, err := env.serverState.GetServerConfig(ctx, connect.NewRequest(&adminv1.GetServerConfigRequest{}))
	if err != nil {
		t.Fatalf("GetServerConfig after partial update: %v", err)
	}
	if getResp.Msg.GetConfig().GetServerName() != "Connect Settings" ||
		getResp.Msg.GetConfig().GetDescription() != "Updated description only" ||
		getResp.Msg.GetProfile().GetPublicProfile().GetDescription() != "Updated description only" {
		t.Fatalf("partial server config response = %+v", getResp.Msg)
	}
}

func TestAdminServerServiceUpdatesServerBranding(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := env.serverState.UploadServerLogo(env.ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UploadServerLogo code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.serverState.UploadServerLogo(ctx, connect.NewRequest(&adminv1.UploadServerLogoRequest{
		Image: &apiv1.ImageUpload{Image: connectAPITestPNG()},
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
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "logo.png",
			ContentType: "image/png",
		},
	}))
	if err != nil {
		t.Fatalf("UploadServerLogo: %v", err)
	}
	if logoResp.Msg.GetProfile().GetPublicProfile().GetLogoUrl() == "" {
		t.Fatalf("UploadServerLogo profile = %+v, want logo URL", logoResp.Msg.GetProfile())
	}

	deleteLogoResp, err := env.serverState.DeleteServerLogo(ctx, connect.NewRequest(&adminv1.DeleteServerLogoRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerLogo: %v", err)
	}
	if deleteLogoResp.Msg.GetProfile().GetPublicProfile().LogoUrl != nil {
		t.Fatalf("DeleteServerLogo logo URL = %q, want nil", deleteLogoResp.Msg.GetProfile().GetPublicProfile().GetLogoUrl())
	}

	if _, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UploadServerBanner code = %v, want invalid_argument", connect.CodeOf(err))
	}
	bannerResp, err := env.serverState.UploadServerBanner(ctx, connect.NewRequest(&adminv1.UploadServerBannerRequest{
		Image: &apiv1.ImageUpload{
			Image:       connectAPITestPNG(),
			Filename:    "banner.png",
			ContentType: "image/png",
		},
	}))
	if err != nil {
		t.Fatalf("UploadServerBanner: %v", err)
	}
	if bannerResp.Msg.GetProfile().GetPublicProfile().GetBannerUrl() == "" {
		t.Fatalf("UploadServerBanner profile = %+v, want banner URL", bannerResp.Msg.GetProfile())
	}

	deleteBannerResp, err := env.serverState.DeleteServerBanner(ctx, connect.NewRequest(&adminv1.DeleteServerBannerRequest{}))
	if err != nil {
		t.Fatalf("DeleteServerBanner: %v", err)
	}
	if deleteBannerResp.Msg.GetProfile().GetPublicProfile().BannerUrl != nil {
		t.Fatalf("DeleteServerBanner banner URL = %q, want nil", deleteBannerResp.Msg.GetProfile().GetPublicProfile().GetBannerUrl())
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
		BlockedUsernames: []string{"root"},
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
	defaultBlockedUsernames := []string{"root", "admin", "superuser", "op", "operator", "support"}
	if !slices.Equal(configResp.Msg.GetBlockedUsernames(), defaultBlockedUsernames) {
		t.Fatalf("default blocked usernames = %q, want %q", configResp.Msg.GetBlockedUsernames(), defaultBlockedUsernames)
	}

	updateResp, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{"root", "Reserved", " admin "},
	}))
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames: %v", err)
	}
	if want := []string{"root", "reserved", "admin"}; !slices.Equal(updateResp.Msg.GetBlockedUsernames(), want) {
		t.Fatalf("updated blocked usernames = %q, want %q", updateResp.Msg.GetBlockedUsernames(), want)
	}
	stored, err := env.core.ConfigManager().GetEffectiveBlockedUsernames(env.ctx)
	if err != nil {
		t.Fatalf("GetEffectiveBlockedUsernames: %v", err)
	}
	if stored != "root\nreserved\nadmin" {
		t.Fatalf("stored blocked usernames = %q, want root/reserved/admin", stored)
	}

	compatResp, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{"root\nreserved"},
	}))
	if err != nil {
		t.Fatalf("compat UpdateBlockedUsernames: %v", err)
	}
	if want := []string{"root", "reserved"}; !slices.Equal(compatResp.Msg.GetBlockedUsernames(), want) {
		t.Fatalf("compat blocked usernames = %q, want %q", compatResp.Msg.GetBlockedUsernames(), want)
	}

	if _, err := env.serverState.UpdateBlockedUsernames(ctx, connect.NewRequest(&adminv1.UpdateBlockedUsernamesRequest{
		BlockedUsernames: []string{strings.Repeat("u", core.MaxLoginLength+1)},
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
		Name:        stringPtr("connect-renamed"),
		Description: stringPtr("updated through ConnectRPC"),
	}))
	if err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if updateResp.Msg.GetRoom().GetName() != "connect-renamed" {
		t.Fatalf("UpdateRoom name = %q, want connect-renamed", updateResp.Msg.GetRoom().GetName())
	}
	partialUpdateResp, err := env.rooms.UpdateRoom(ctx, connect.NewRequest(&apiv1.UpdateRoomRequest{
		RoomId:      room.GetId(),
		Description: stringPtr("description-only patch"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRoom: %v", err)
	}
	if got := partialUpdateResp.Msg.GetRoom(); got.GetName() != "connect-renamed" || got.GetDescription() != "description-only patch" {
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
	reply, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, participant.Id, "private reply", nil, root.Id, "", nil, false)
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

	_, err = env.prefs.GetRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
		RoomId: dm.Id,
	}))
	checkInaccessible("GetRoomNotificationPreference", err)

	_, err = env.prefs.UpdateRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.UpdateRoomNotificationPreferenceRequest{
		RoomId: dm.Id,
		Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	}))
	checkInaccessible("UpdateRoomNotificationPreference", err)
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
	if apiRoomPermissionGranted(dmRoom, core.PermRoomJoin) || apiRoomPermissionGranted(dmRoom, core.PermRoomManage) || apiRoomPermissionGranted(dmRoom, core.PermRoomMemberBan) {
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
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
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

func TestRoomServiceListMembersRequiresMembership(t *testing.T) {
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
	if _, err := env.rooms.ListMembers(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider ListMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if _, err := env.rooms.GetMember(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.GetRoomMemberRequest{RoomId: room.Id, UserId: member.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider GetMember code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if _, err := env.rooms.BatchGetMembers(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.BatchGetRoomMembersRequest{RoomId: room.Id, UserIds: []string{member.Id}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider BatchGetMembers code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
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

func TestMemberDirectoryOversizedPagesClampTo500(t *testing.T) {
	limit, offset := apiPagination(&apiv1.PageRequest{Limit: 9999}, defaultMemberDirectoryLimit, maxMemberDirectoryLimit)
	if limit != 500 || offset != 0 {
		t.Fatalf("apiPagination limit, offset = %d, %d; want 500, 0", limit, offset)
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

	clearResp, err := env.account.DeleteCustomStatus(ctx, connect.NewRequest(&apiv1.DeleteCustomStatusRequest{}))
	if err != nil {
		t.Fatalf("DeleteCustomStatus: %v", err)
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
	dm, err := env.core.CreateNotification(env.ctx, env.viewer.Id, actor.Id, &corev1.Notification{
		Notification: &corev1.Notification_DmMessage{
			DmMessage: &corev1.DMMessageNotification{
				RoomId:  "dm-room",
				EventId: "dm-event",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification dm: %v", err)
	}

	if _, err := env.notifications.GetNotification(env.ctx, connect.NewRequest(&apiv1.GetNotificationRequest{NotificationId: mention.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetNotification code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.notifications.BatchGetNotifications(env.ctx, connect.NewRequest(&apiv1.BatchGetNotificationsRequest{NotificationIds: []string{mention.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetNotifications code = %v, want unauthenticated", connect.CodeOf(err))
	}

	listResp, err := env.notifications.ListNotifications(ctx, connect.NewRequest(&apiv1.ListNotificationsRequest{Page: &apiv1.PageRequest{Limit: 1}}))
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if listResp.Msg.GetPage().GetTotalCount() != 2 || !listResp.Msg.GetPage().GetHasMore() || len(listResp.Msg.GetNotifications()) != 1 {
		t.Fatalf("ListNotifications page = %+v, want total 2, has_more true, one item", listResp.Msg)
	}
	item := listResp.Msg.GetNotifications()[0]
	if item.GetActor().GetDisplayName() != "Notification Actor" || item.GetActor().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
		t.Fatalf("notification actor = %+v, want hydrated actor", item.GetActor())
	}

	getResp, err := env.notifications.GetNotification(ctx, connect.NewRequest(&apiv1.GetNotificationRequest{NotificationId: mention.Id}))
	if err != nil {
		t.Fatalf("GetNotification: %v", err)
	}
	if got := getResp.Msg.GetNotification(); got.GetId() != mention.Id || got.GetMention().GetEventId() != "mention-event" || got.GetMention().GetThreadRootEventId() != "thread-root" {
		t.Fatalf("GetNotification item = %+v, want mention", got)
	}
	if _, err := env.notifications.GetNotification(ctx, connect.NewRequest(&apiv1.GetNotificationRequest{NotificationId: "missing-notification"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetNotification code = %v, want not_found", connect.CodeOf(err))
	}

	batchResp, err := env.notifications.BatchGetNotifications(ctx, connect.NewRequest(&apiv1.BatchGetNotificationsRequest{
		NotificationIds: []string{mention.Id, "missing-notification", dm.Id, mention.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetNotifications: %v", err)
	}
	gotBatch := batchResp.Msg.GetNotifications()
	if len(gotBatch) != 2 || gotBatch[0].GetId() != mention.Id || gotBatch[1].GetId() != dm.Id {
		t.Fatalf("BatchGetNotifications items = %+v, want mention,dm", gotBatch)
	}

	roomResp, err := env.notifications.ListRoomNotifications(ctx, connect.NewRequest(&apiv1.ListRoomNotificationsRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListRoomNotifications: %v", err)
	}
	if roomResp.Msg.GetPage().GetTotalCount() != 1 || len(roomResp.Msg.GetNotifications()) != 1 {
		t.Fatalf("ListRoomNotifications page = %+v, want one room notification", roomResp.Msg)
	}
	mentionItem := roomResp.Msg.GetNotifications()[0]
	if mentionItem.GetMention().GetRoom().GetId() != room.Id || mentionItem.GetMention().GetThreadRootEventId() != "thread-root" {
		t.Fatalf("mention payload = %+v, want room/thread payload", mentionItem.GetMention())
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "notification-outsider", "Notification Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderResp, err := env.notifications.ListRoomNotifications(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.ListRoomNotificationsRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("ListRoomNotifications outsider: %v", err)
	}
	if outsiderResp.Msg.GetPage().GetTotalCount() != 0 || len(outsiderResp.Msg.GetNotifications()) != 0 {
		t.Fatalf("outsider room notifications = %+v, want empty page", outsiderResp.Msg)
	}

	hasResp, err := env.notifications.HasNotifications(ctx, connect.NewRequest(&apiv1.HasNotificationsRequest{}))
	if err != nil {
		t.Fatalf("HasNotifications: %v", err)
	}
	if !hasResp.Msg.GetHasNotifications() {
		t.Fatal("HasNotifications = false, want true")
	}
	countsResp, err := env.notifications.ListRoomNotificationCounts(ctx, connect.NewRequest(&apiv1.ListRoomNotificationCountsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomNotificationCounts: %v", err)
	}
	counts := make(map[string]int32)
	for _, count := range countsResp.Msg.GetRoomCounts() {
		counts[count.GetRoomId()] = count.GetTotalCount()
	}
	if counts[room.Id] != 1 || counts["dm-room"] != 1 {
		t.Fatalf("ListRoomNotificationCounts = %+v, want counts for channel and DM rooms", counts)
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
	if !dismissAgainResp.Msg.GetDismissed() {
		t.Fatal("DismissNotification again dismissed = false, want idempotent true")
	}
	if _, err := env.notifications.GetNotification(ctx, connect.NewRequest(&apiv1.GetNotificationRequest{NotificationId: mention.Id})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("dismissed GetNotification code = %v, want not_found", connect.CodeOf(err))
	}
	remainingBatchResp, err := env.notifications.BatchGetNotifications(ctx, connect.NewRequest(&apiv1.BatchGetNotificationsRequest{
		NotificationIds: []string{mention.Id, dm.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetNotifications after dismiss: %v", err)
	}
	if got := remainingBatchResp.Msg.GetNotifications(); len(got) != 1 || got[0].GetId() != dm.Id {
		t.Fatalf("BatchGetNotifications after dismiss items = %+v, want dm only", got)
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

	if _, err := env.prefs.UpdateServerNotificationPreference(env.ctx, connect.NewRequest(&apiv1.UpdateServerNotificationPreferenceRequest{
		Level: apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateServerNotificationPreference code = %v, want unauthenticated", connect.CodeOf(err))
	}

	setResp, err := env.prefs.UpdateServerNotificationPreference(ctx, connect.NewRequest(&apiv1.UpdateServerNotificationPreferenceRequest{
		Level: apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES,
	}))
	if err != nil {
		t.Fatalf("UpdateServerNotificationPreference: %v", err)
	}
	if setResp.Msg.GetPreference().GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || setResp.Msg.GetPreference().GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
		t.Fatalf("UpdateServerNotificationPreference response = %+v, want all/all", setResp.Msg)
	}

	getResp, err := env.prefs.GetServerNotificationPreference(ctx, connect.NewRequest(&apiv1.GetServerNotificationPreferenceRequest{}))
	if err != nil {
		t.Fatalf("GetServerNotificationPreference: %v", err)
	}
	if getResp.Msg.GetPreference().GetLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES || getResp.Msg.GetPreference().GetEffectiveLevel() != apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
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

	activeResp, err := env.voice.ListActiveCallRooms(ctx, connect.NewRequest(&apiv1.ListActiveCallRoomsRequest{}))
	if err != nil {
		t.Fatalf("ListActiveCallRooms: %v", err)
	}
	if calls := activeResp.Msg.GetCalls(); len(calls) != 1 || calls[0].GetRoomId() != room.Id || calls[0].GetCallId() == "" {
		t.Fatalf("active calls = %v, want one call for %s", calls, room.Id)
	}
	nonMemberActiveResp, err := env.voice.ListActiveCallRooms(withCaller(env.ctx, nonMember), connect.NewRequest(&apiv1.ListActiveCallRoomsRequest{}))
	if err != nil {
		t.Fatalf("non-member ListActiveCallRooms: %v", err)
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
	if activeCall.GetRoomId() != room.Id || activeCall.GetCallId() == "" || len(activeCall.GetParticipants()) != 1 {
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
	if calls := batchCallsResp.Msg.GetCalls(); len(calls) != 1 || calls[0].GetRoomId() != room.Id || calls[0].GetCallId() != activeCall.GetCallId() {
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

func TestMessageServiceFetchLinkPreviewRequiresAuthMapsPreviewAndPostsToken(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.messages.FetchLinkPreview(env.ctx, connect.NewRequest(&apiv1.FetchLinkPreviewRequest{Url: "https://example.test"})); connect.CodeOf(err) != connect.CodeUnauthenticated {
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

	resp, err := env.messages.FetchLinkPreview(
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
	if resp.Msg.GetPreviewToken() == "" {
		t.Fatalf("PreviewToken is empty")
	}

	room := env.createJoinedRoom("message-preview-token")
	createResp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:           room.Id,
		Body:             "message with preview",
		LinkPreviewToken: resp.Msg.GetPreviewToken(),
	}))
	if err != nil {
		t.Fatalf("CreateMessage with preview token: %v", err)
	}
	message := createResp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil")
	}
	body, err := env.core.GetFullMessageBody(env.ctx, core.KindChannel, message.GetId())
	if err != nil {
		t.Fatalf("GetFullMessageBody: %v", err)
	}
	stored := body.LinkPreview
	if stored == nil || stored.GetTitle() != "Connect Preview" || stored.GetDescription() != "Connect preview description" || stored.GetImageAssetId() == "" {
		t.Fatalf("stored link preview = %+v", stored)
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

func TestRoomAndThreadTimelineRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "timeline-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	req := connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id})
	if _, err := env.rooms.GetRoomEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-outsider", "Timeline Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.GetRoomEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceCreateMessageRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-authz")
	req := connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "hello",
	})

	if _, err := env.messages.CreateMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-outsider", "Message Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.CreateMessage(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceAddAndRemoveRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-authz")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	req := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})

	if _, err := env.messages.AddReaction(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reaction-outsider", "Reaction Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.AddReaction(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageReact); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.AddReaction(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceAddAndRemoveResponseSemantics(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-response")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	ctx := withCaller(env.ctx, env.viewer)

	addReq := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	addResp, err := env.messages.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !addResp.Msg.Added {
		t.Fatal("AddReaction Added = false, want true")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("AddReaction reaction = %+v, want thumbsup count 1 hasReacted", got)
	}

	addResp, err = env.messages.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("duplicate AddReaction: %v", err)
	}
	if addResp.Msg.Added {
		t.Fatal("duplicate AddReaction Added = true, want false")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("duplicate AddReaction reaction = %+v, want unchanged thumbsup count 1 hasReacted", got)
	}

	removeReq := connect.NewRequest(&apiv1.RemoveReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	removeResp, err := env.messages.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if !removeResp.Msg.Removed {
		t.Fatal("RemoveReaction Removed = false, want true")
	}
	if removeResp.Msg.GetReaction() != nil {
		t.Fatalf("RemoveReaction reaction = %+v, want nil after last reaction removed", removeResp.Msg.GetReaction())
	}

	removeResp, err = env.messages.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("duplicate RemoveReaction: %v", err)
	}
	if removeResp.Msg.Removed {
		t.Fatal("duplicate RemoveReaction Removed = true, want false")
	}
	if removeResp.Msg.GetReaction() != nil {
		t.Fatalf("duplicate RemoveReaction reaction = %+v, want nil", removeResp.Msg.GetReaction())
	}
}

func TestMessageServiceReactionOnEchoCanonicalizesToOriginal(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-echo")
	ctx := withCaller(env.ctx, env.viewer)

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "reply", nil, root.Id, root.Id, nil, true)
	if err != nil {
		t.Fatalf("PostMessage reply with echo: %v", err)
	}
	echoID, ok := env.core.RoomTimeline.ChannelEchoEventID(reply.Id)
	if !ok {
		t.Fatal("expected channel echo for reply")
	}

	addResp, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: echoID,
		Emoji:          "thumbsup",
	}))
	if err != nil {
		t.Fatalf("AddReaction via echo: %v", err)
	}
	if !addResp.Msg.GetAdded() {
		t.Fatal("AddReaction via echo Added = false, want true")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("AddReaction via echo reaction = %+v, want thumbsup count 1 hasReacted", got)
	}

	dupResp, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: reply.Id,
		Emoji:          "thumbsup",
	}))
	if err != nil {
		t.Fatalf("duplicate AddReaction via original: %v", err)
	}
	if dupResp.Msg.GetAdded() {
		t.Fatal("duplicate AddReaction via original Added = true, want false")
	}

	roomResp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	echoEvent := timelinePageEvent(roomResp.Msg.GetPage(), echoID)
	if echoEvent == nil || echoEvent.GetMessagePosted() == nil {
		t.Fatalf("echo event %s missing from room page", echoID)
	}
	if got := echoEvent.GetMessagePosted().GetMessage().GetReactions(); len(got) != 1 || got[0].GetEmoji() != "thumbsup" || got[0].GetCount() != 1 || !got[0].GetHasReacted() {
		t.Fatalf("echo reactions = %+v, want thumbsup count 1 hasReacted", got)
	}

	threadResp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents: %v", err)
	}
	replyEvent := timelinePageEvent(threadResp.Msg.GetPage(), reply.Id)
	if replyEvent == nil || replyEvent.GetMessagePosted() == nil {
		t.Fatalf("reply event %s missing from thread page", reply.Id)
	}
	if got := replyEvent.GetMessagePosted().GetMessage().GetReactions(); len(got) != 1 || got[0].GetEmoji() != "thumbsup" || got[0].GetCount() != 1 || !got[0].GetHasReacted() {
		t.Fatalf("reply reactions = %+v, want thumbsup count 1 hasReacted", got)
	}
}

func TestMessageServiceValidatesEmoji(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-validation")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")

	_, err := env.messages.AddReaction(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "totally_bogus",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid emoji AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestMessageServiceCreateMessageValidatesInput(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-validation")
	ctx := withCaller(env.ctx, env.viewer)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	otherRoom := env.createJoinedRoom("message-post-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

	tests := []struct {
		name string
		req  *apiv1.CreateMessageRequest
		code connect.Code
	}{
		{
			name: "missing room",
			req:  &apiv1.CreateMessageRequest{Body: "hello"},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "empty body and no attachments",
			req:  &apiv1.CreateMessageRequest{RoomId: room.Id, Body: "   "},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "channel echo outside thread",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "hello",
				AlsoSendToChannel: true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: "missing-reply-target",
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: otherRoomMessage.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "invalid link preview token",
			req: &apiv1.CreateMessageRequest{
				RoomId:           room.Id,
				Body:             "hello",
				LinkPreviewToken: "not-a-token",
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := env.messages.CreateMessage(ctx, connect.NewRequest(tt.req)); connect.CodeOf(err) != tt.code {
				t.Fatalf("CreateMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
		})
	}
}

func TestMessageServiceCreateMessageInfersVideoProcessingAssetIDs(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Video.Enabled = true
	env.core.OnVideoProcessingRequested = func(context.Context, string, string) error { return nil }
	room := env.createJoinedRoom("message-post-video")
	assetID := env.uploadAttachmentAsset(t, room.Id, "clip.mp4", "video/mp4", []byte("original video"))

	if _, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{assetID},
	})); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	manifest, ok := env.core.Assets.VideoAttachmentManifest(assetID)
	if !ok || manifest.Started == nil {
		t.Fatalf("VideoAttachmentManifest = %+v, %v; want started", manifest, ok)
	}
}

func TestMessageServiceCreateMessageReturnsRenderableMessage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-success")

	resp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "hello over connect",
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil, response = %+v", resp.Msg)
	}
	if message.Body == nil || message.GetBody() != "hello over connect" {
		t.Fatalf("message body = %q present=%v, want posted body", message.GetBody(), message.Body != nil)
	}
}

func TestMessageServiceCreateMessageUploadsAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-upload")
	assetID := env.uploadAttachmentAsset(t, room.Id, "note.txt", "text/plain", []byte("uploaded over connect"))

	resp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{assetID},
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil, response = %+v", resp.Msg)
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

func TestMessageServiceCreateMessageAttachmentPreflightDoesNotCreateAssets(t *testing.T) {
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
	sum := sha256.Sum256([]byte("denied upload"))
	_, err = env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len("denied upload")),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied attachment CreateUpload code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after denied post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after denied attachment = %d, want unchanged %d", after, before)
	}
}

func TestMessageServiceCreateMessageBroadMentionWithAttachment(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-broad-mention")
	ctx := withCaller(env.ctx, env.viewer)

	const targetCount = 12
	for i := 0; i < targetCount; i++ {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "large-mention-target-"+strconv.Itoa(i), "Large Mention Target", "password")
		if err != nil {
			t.Fatalf("CreateUser target %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom target %d: %v", i, err)
		}
	}

	assetID := env.uploadAttachmentAsset(t, room.Id, "note.txt", "text/plain", []byte("broad mention upload"))
	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before post: %v", err)
	}
	resp, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		Body:               "@all please review this attachment",
		AttachmentAssetIds: []string{assetID},
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage response = %+v, want message", resp.Msg)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after message = %d, want unchanged %d", after, before)
	}
}

func TestMessageServiceCreateMessageValidationPreflightDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-validation")
	ctx := withCaller(env.ctx, env.viewer)

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	otherRoom := env.createJoinedRoom("upload-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

	tests := []struct {
		name string
		req  *apiv1.CreateMessageRequest
		code connect.Code
	}{
		{
			name: "missing thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: "missing-reply-target",
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: otherRoomMessage.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "invalid link preview token",
			req: &apiv1.CreateMessageRequest{
				RoomId:             room.Id,
				Body:               "message with bad preview and file",
				AttachmentAssetIds: []string{"missing"},
				LinkPreviewToken:   "not-a-token",
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
			_, err = env.messages.CreateMessage(ctx, connect.NewRequest(tt.req))
			if connect.CodeOf(err) != tt.code {
				t.Fatalf("CreateMessage code = %v, want %v", connect.CodeOf(err), tt.code)
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

func TestMessageServiceCreateMessageRejectsVideoUploadWhenProcessingDisabled(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-upload-video-disabled")
	ctx := withCaller(env.ctx, env.viewer)

	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before video post: %v", err)
	}
	sum := sha256.Sum256([]byte("video"))
	_, err = env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "clip.mp4",
		ContentType: "video/mp4",
		Size:        int64(len("video")),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("video upload CreateUpload code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after video post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after rejected video = %d, want unchanged %d", after, before)
	}
}

func TestAssetUploadServiceChunkResumeCompleteAndCancel(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-flow")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("first chunk and second chunk")
	first := content[:11]
	second := content[11:]
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	uploadID := created.Msg.GetUpload().GetUploadId()
	if uploadID == "" || created.Msg.GetUpload().GetMaxChunkSize() <= 0 {
		t.Fatalf("created upload = %+v, want id and limits", created.Msg.GetUpload())
	}

	firstSum := sha256.Sum256(first)
	chunkResp, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Content:     first,
		ChunkSha256: hex.EncodeToString(firstSum[:]),
	}))
	if err != nil {
		t.Fatalf("UploadChunk first: %v", err)
	}
	if got := chunkResp.Msg.GetUpload().GetCommittedOffset(); got != int64(len(first)) {
		t.Fatalf("committed offset after first chunk = %d, want %d", got, len(first))
	}
	resume, err := env.assetUploads.GetUpload(ctx, connect.NewRequest(&apiv1.GetUploadRequest{UploadId: uploadID}))
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got := resume.Msg.GetUpload().GetCommittedOffset(); got != int64(len(first)) {
		t.Fatalf("resume committed offset = %d, want %d", got, len(first))
	}

	secondSum := sha256.Sum256(second)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Offset:      int64(len(first)),
		Content:     second,
		ChunkSha256: hex.EncodeToString(secondSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk second: %v", err)
	}
	completed, err := env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{UploadId: uploadID}))
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if completed.Msg.GetUpload().GetStatus() != apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_COMPLETED {
		t.Fatalf("completed upload status = %v, want completed", completed.Msg.GetUpload().GetStatus())
	}
	if completed.Msg.GetAsset().GetAssetId() == "" || completed.Msg.GetAsset().GetFilename() != "note.txt" {
		t.Fatalf("completed asset = %+v, want note.txt asset id", completed.Msg.GetAsset())
	}

	cancelContent := []byte("cancel me")
	cancelSum := sha256.Sum256(cancelContent)
	cancelCreated, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "cancel.txt",
		ContentType: "text/plain",
		Size:        int64(len(cancelContent)),
		Sha256:      hex.EncodeToString(cancelSum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload cancel: %v", err)
	}
	cancelResp, err := env.assetUploads.CancelUpload(ctx, connect.NewRequest(&apiv1.CancelUploadRequest{
		UploadId: cancelCreated.Msg.GetUpload().GetUploadId(),
	}))
	if err != nil {
		t.Fatalf("CancelUpload: %v", err)
	}
	if cancelResp.Msg.GetUpload().GetStatus() != apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_CANCELLED {
		t.Fatalf("cancel status = %v, want cancelled", cancelResp.Msg.GetUpload().GetStatus())
	}
	if _, err := env.assetUploads.GetUpload(ctx, connect.NewRequest(&apiv1.GetUploadRequest{
		UploadId: cancelCreated.Msg.GetUpload().GetUploadId(),
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetUpload after cancel code = %v, want not_found", connect.CodeOf(err))
	}
}

func TestAssetUploadServiceDoesNotRequireThreadPostPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-thread-permission")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("thread attachment")
	sum := sha256.Sum256(content)

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePostInThread); err != nil {
		t.Fatalf("DenyRoomPermission thread post: %v", err)
	}

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "thread.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload with thread posting denied: %v", err)
	}
	if created.Msg.GetUpload().GetUploadId() == "" {
		t.Fatal("CreateUpload upload id is empty")
	}

	_, err = env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:            room.Id,
		Body:              "thread reply",
		ThreadRootEventId: root.Id,
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreateMessage thread reply code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestAssetUploadServiceCompleteRechecksAttachmentPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-complete-permission")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("attachment permission revoked")
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "revoked.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	chunkSum := sha256.Sum256(content)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    created.Msg.GetUpload().GetUploadId(),
		Content:     content,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageAttach); err != nil {
		t.Fatalf("DenyRoomPermission attach: %v", err)
	}

	_, err = env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: created.Msg.GetUpload().GetUploadId(),
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CompleteUpload after attach permission revoked code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestAssetUploadServiceRejectsChecksumOffsetAndIncompleteComplete(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-validation")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("validated content")
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	uploadID := created.Msg.GetUpload().GetUploadId()
	if _, err := env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: uploadID,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("incomplete CompleteUpload code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Content:     []byte("bad"),
		ChunkSha256: strings.Repeat("0", sha256.Size*2),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad checksum UploadChunk code = %v, want invalid_argument", connect.CodeOf(err))
	}
	chunk := []byte("valid")
	chunkSum := sha256.Sum256(chunk)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Offset:      1,
		Content:     chunk,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad offset UploadChunk code = %v, want invalid_argument", connect.CodeOf(err))
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
		Body:    stringPtr("ignored"),
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
		Body:    stringPtr("ignored"),
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
		Body:    stringPtr("ignored"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member without manage UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	authorResp, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    stringPtr("author edit"),
	}))
	if err != nil {
		t.Fatalf("author UpdateMessage: %v", err)
	}
	if !authorResp.Msg.GetUpdated() || authorResp.Msg.GetMessage().GetBody() != "author edit" {
		t.Fatalf("author UpdateMessage response = %+v, want hydrated edited event", authorResp.Msg)
	}
	if body, err := env.core.GetMessageBody(env.ctx, core.KindChannel, original.Id); err != nil || body != "author edit" {
		t.Fatalf("body after author edit = %q, %v; want author edit, nil", body, err)
	}

	echo := false
	if _, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:            room.Id,
		EventId:           original.Id,
		Body:              stringPtr("invalid echo edit"),
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
		Body:    stringPtr("moderator edit"),
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
		Body:              stringPtr("moderator echo edit"),
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
		t.Fatalf("CreateMessage attachment: %v", err)
	}
	previewURL := "https://example.test/preview"
	previewEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "with preview", nil, "", "", &corev1.LinkPreview{
		Url:   previewURL,
		Title: "Preview",
	}, false)
	if err != nil {
		t.Fatalf("CreateMessage preview: %v", err)
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

func TestRoomServiceUpdateTypingIndicatorRequiresMembershipOnly(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-typing")
	req := connect.NewRequest(&apiv1.UpdateTypingIndicatorRequest{RoomId: room.Id})

	if _, err := env.rooms.UpdateTypingIndicator(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-typing-outsider", "Message Typing Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.UpdateTypingIndicator(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider UpdateTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission post: %v", err)
	}
	resp, err := env.rooms.UpdateTypingIndicator(withCaller(env.ctx, env.viewer), req)
	if err != nil {
		t.Fatalf("member UpdateTypingIndicator with post denied: %v", err)
	}
	if !resp.Msg.Updated {
		t.Fatal("UpdateTypingIndicator Updated = false, want true")
	}
}

func TestRoomAndThreadTimelineGetRoomEventsPaginatesWithOpaqueCursors(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-pagination")

	m1 := env.post(room.Id, env.viewer.Id, "one", "")
	m2 := env.post(room.Id, env.viewer.Id, "two", "")
	m3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
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

	olderResp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	legacyResp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
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

func TestRoomMessageAndAssetServicesListAttachmentsGetMessagesAndGetAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("attachment-list")

	rootAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "root.txt", "text/plain", bytes.NewReader([]byte("root")))
	if err != nil {
		t.Fatalf("UploadAttachment root: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "root file", []string{rootAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage root: %v", err)
	}
	threadAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "thread.png", "image/png", bytes.NewReader(connectAPITestPNG()))
	if err != nil {
		t.Fatalf("UploadAttachment thread: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "thread file", []string{threadAttachment.Id}, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage reply: %v", err)
	}
	empty, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "no files", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage empty: %v", err)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.rooms.ListRoomAttachments(env.ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 10},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoomAttachments code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	resp, err := env.rooms.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 1},
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  120,
			Height: 120,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_COVER,
		},
	}))
	if err != nil {
		t.Fatalf("ListRoomAttachments: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 2 || !resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetAttachments()) != 1 {
		t.Fatalf("ListRoomAttachments count/hasMore/attachments = %d/%v/%d, want 2/true/1", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore(), len(resp.Msg.GetAttachments()))
	}
	first := resp.Msg.GetAttachments()[0]
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

	get, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	getAttachments := get.Msg.GetMessage().GetAttachments()
	if len(getAttachments) != 1 {
		t.Fatalf("GetMessage attachments = %d, want 1", len(getAttachments))
	}
	fresh := getAttachments[0]
	if fresh.GetId() != threadAttachment.Id {
		t.Fatalf("GetMessage attachment ID = %q, want %q", fresh.GetId(), threadAttachment.Id)
	}
	if fresh.GetAssetUrl().GetUrl() == "" || fresh.GetAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh asset URL missing: %+v", fresh.GetAssetUrl())
	}
	if fresh.GetThumbnailAssetUrl().GetUrl() == "" || fresh.GetThumbnailAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh thumbnail URL missing: %+v", fresh.GetThumbnailAssetUrl())
	}

	asset, err := env.assets.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{
		RoomId:  room.Id,
		AssetId: threadAttachment.Id,
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  64,
			Height: 64,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN,
		},
	}))
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got := asset.Msg.GetAsset().GetThumbnailAssetUrl().GetUrl(); !strings.Contains(got, "/64x64/contain") {
		t.Fatalf("GetAsset thumbnail URL = %q, want 64x64 contain transform", got)
	}

	batch, err := env.messages.BatchGetMessages(ctx, connect.NewRequest(&apiv1.BatchGetMessagesRequest{
		RoomId:   room.Id,
		EventIds: []string{reply.Id, "missing-event", root.Id, reply.Id, empty.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetMessages: %v", err)
	}
	if got := batch.Msg.GetMessages(); len(got) != 3 {
		t.Fatalf("BatchGetMessages messages = %d, want 3", len(got))
	}
	if batch.Msg.Messages[0].GetId() != reply.Id || len(batch.Msg.Messages[0].GetAttachments()) != 1 {
		t.Fatalf("batch first = %+v, want reply with one attachment", batch.Msg.Messages[0])
	}
	if batch.Msg.Messages[1].GetId() != root.Id ||
		len(batch.Msg.Messages[1].GetAttachments()) != 1 ||
		batch.Msg.Messages[1].GetAttachments()[0].GetId() != rootAttachment.Id {
		t.Fatalf("batch second = %+v, want root attachment", batch.Msg.Messages[1])
	}
	if batch.Msg.Messages[2].GetId() != empty.Id || len(batch.Msg.Messages[2].GetAttachments()) != 0 {
		t.Fatalf("batch third = %+v, want empty message with no attachments", batch.Msg.Messages[2])
	}

	assets, err := env.assets.BatchGetAssets(ctx, connect.NewRequest(&apiv1.BatchGetAssetsRequest{
		RoomId:   room.Id,
		AssetIds: []string{threadAttachment.Id, "missing-asset", rootAttachment.Id, threadAttachment.Id},
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  64,
			Height: 64,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN,
		},
	}))
	if err != nil {
		t.Fatalf("BatchGetAssets: %v", err)
	}
	if got := assets.Msg.GetAssets(); len(got) != 2 || got[0].GetId() != threadAttachment.Id || got[1].GetId() != rootAttachment.Id {
		t.Fatalf("BatchGetAssets assets = %+v, want thread then root attachments", got)
	}
}

func TestRoomAndThreadTimelineHydratesMessagesWithoutClientNPlusOne(t *testing.T) {
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

	resp, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	message := payload.GetMessage()
	if message.Body == nil || message.GetBody() != "root" {
		t.Fatalf("message body present/body = %v/%q, want true/root", message.Body != nil, message.GetBody())
	}
	thread := message.GetThread()
	if thread.GetReplyCount() != 1 {
		t.Fatalf("reply count = %d, want 1", thread.GetReplyCount())
	}
	if got := thread.GetParticipantCount(); got != 1 {
		t.Fatalf("thread participant count = %d, want 1", got)
	}
	if got := thread.GetParticipantPreviewUserIds(); len(got) != 1 || got[0] != replier.Id {
		t.Fatalf("thread participant preview = %v, want [%s]", got, replier.Id)
	}
	if len(message.Reactions) != 1 {
		t.Fatalf("reaction summaries = %d, want 1", len(message.Reactions))
	}
	reaction := message.Reactions[0]
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

func TestRoomAndThreadTimelineGetThreadEventsIncludesRootAndPaginatesReplies(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
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

	olderResp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
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

func TestRoomAndThreadTimelineGetThreadEventsAroundRootAndReply(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-around")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
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

	replyResp, err := env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
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

	_, err = env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           "missing-anchor",
		Limit:             3,
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("missing anchor code = %v, want %v", got, connect.CodeNotFound)
	}
}

func TestRoomAndThreadTimelineGetMessageForPermalinks(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-message-link")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	req := connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	})
	if _, err := env.messages.GetMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-link-outsider", "Message Link Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.GetMessage(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: root.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage root: %v", err)
	}
	rootMessage := rootResp.Msg.GetMessage()
	if rootMessage.GetId() != root.Id || rootMessage.GetThreadRootEventId() != "" {
		t.Fatalf("root message = event %q thread %q, want event %q no thread", rootMessage.GetId(), rootMessage.GetThreadRootEventId(), root.Id)
	}
	if rootMessage.GetBody() != "root" {
		t.Fatalf("root body = %q, want root", rootMessage.GetBody())
	}

	replyResp, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage reply: %v", err)
	}
	replyMessage := replyResp.Msg.GetMessage()
	if replyMessage.GetId() != reply.Id || replyMessage.GetThreadRootEventId() != root.Id {
		t.Fatalf("reply message = event %q thread %q, want event %q thread %q", replyMessage.GetId(), replyMessage.GetThreadRootEventId(), reply.Id, root.Id)
	}

	if _, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: "missing-anchor",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing message code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
}

func TestRoomAndThreadTimelineGetThreadEventsRequiresMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-authz")
	root := env.post(room.Id, env.viewer.Id, "root", "")

	req := connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.threads.GetThreadEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-outsider", "Thread Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.threads.GetThreadEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestRoomAndThreadTimelineHydratesProcessedVideoAttachments(t *testing.T) {
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
		t.Fatalf("CreateMessage: %v", err)
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

	resp, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	attachments := apiEvent.GetMessagePosted().GetMessage().GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	if got := attachments[0].GetThumbnailAssetUrl().GetUrl(); !strings.Contains(got, "/960x800/contain") {
		t.Fatalf("attachment thumbnail URL = %q, want 960x800 contain transform", got)
	}
	processing := attachments[0].GetVideoProcessing()
	if processing == nil {
		t.Fatal("videoProcessing = nil, want completed manifest")
	}
	if processing.GetStatus() != apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_COMPLETED {
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

	_, err := h.event(env.ctx, &core.RoomEvent{Event: &corev1.Event{
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
			if _, err := h.event(env.ctx, &core.RoomEvent{Event: tt.event}); err != nil {
				t.Fatalf("hydrate visible event: %v", err)
			}
		})
	}
}

func TestRoomAndThreadServicesRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-authz")

	req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{RoomId: room.Id})
	if _, err := env.rooms.MarkRoomAsRead(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-outsider", "Read State Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.MarkRoomAsRead(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestRoomAndThreadServicesMarkRoomAsReadAnchorsAndDoesNotRegress(t *testing.T) {
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
	roomMention, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{RoomId: room.Id, EventId: e1.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification room mention: %v", err)
	}
	roomReply, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{RoomId: room.Id, EventId: e2.Id, InReplyToId: e1.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification room reply: %v", err)
	}
	futureRoomNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_RoomMessage{
			RoomMessage: &corev1.RoomMessageNotification{RoomId: room.Id, EventId: e3.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification future room message: %v", err)
	}
	threadRoot := env.post(room.Id, env.viewer.Id, "thread root", "")
	threadReply := env.post(room.Id, env.viewer.Id, "thread reply", threadRoot.Id)
	threadNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{RoomId: room.Id, EventId: threadReply.Id, InReplyToId: threadRoot.Id, InThread: threadRoot.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification thread reply: %v", err)
	}

	ctx := withCaller(env.ctx, reader)
	resp, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
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
	assertAPINotifications(t, env, ctx,
		[]string{futureRoomNotification.Id, threadNotification.Id},
		[]string{roomMention.Id, roomReply.Id},
	)
	assertRoomNotificationCount(t, env, ctx, room.Id, 2)

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead stale e1: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after stale e1 = %q, %v; want %s", got, err, e2.Id)
	}
	assertAPINotifications(t, env, ctx,
		[]string{futureRoomNotification.Id, threadNotification.Id},
		[]string{roomMention.Id, roomReply.Id},
	)

	reply := env.post(room.Id, env.viewer.Id, "reply", e2.Id)
	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkRoomAsRead reply anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after reply anchor = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after missing event = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId: room.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead omitted anchor: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != threadRoot.Id {
		t.Fatalf("marker after omitted anchor = %q, %v; want %s", got, err, threadRoot.Id)
	}
	assertAPINotifications(t, env, ctx,
		[]string{threadNotification.Id},
		[]string{futureRoomNotification.Id, roomMention.Id, roomReply.Id},
	)
	assertRoomNotificationCount(t, env, ctx, room.Id, 1)
}

func TestRoomAndThreadServicesMarkRoomAsReadRejectsMissingAnchorWithoutLazyMarker(t *testing.T) {
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
	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker after rejected request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	resp, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
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

func TestRoomAndThreadServicesMarkThreadAsReadAnchorsAndDoesNotRegress(t *testing.T) {
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
	threadReplyNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{RoomId: room.Id, EventId: reply1.Id, InReplyToId: root.Id, InThread: root.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification thread reply: %v", err)
	}
	threadMentionNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{RoomId: room.Id, EventId: reply2.Id, InThread: root.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification thread mention: %v", err)
	}
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)
	futureThreadNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{RoomId: room.Id, EventId: reply3.Id, InReplyToId: root.Id, InThread: root.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification future thread reply: %v", err)
	}
	otherRoot := env.post(room.Id, env.viewer.Id, "other root", "")
	otherReply := env.post(room.Id, env.viewer.Id, "other reply", otherRoot.Id)
	otherThreadNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{RoomId: room.Id, EventId: otherReply.Id, InReplyToId: otherRoot.Id, InThread: otherRoot.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification other thread reply: %v", err)
	}
	roomNotification, err := env.core.CreateNotification(env.ctx, reader.Id, env.viewer.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{RoomId: room.Id, EventId: root.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification room mention: %v", err)
	}

	ctx := withCaller(env.ctx, reader)
	resp, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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
	assertAPINotifications(t, env, ctx,
		[]string{futureThreadNotification.Id, otherThreadNotification.Id, roomNotification.Id},
		[]string{threadReplyNotification.Id, threadMentionNotification.Id},
	)
	assertRoomNotificationCount(t, env, ctx, room.Id, 3)

	resp, err = env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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
	assertAPINotifications(t, env, ctx,
		[]string{futureThreadNotification.Id, otherThreadNotification.Id, roomNotification.Id},
		[]string{threadReplyNotification.Id, threadMentionNotification.Id},
	)

	if _, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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

	if _, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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

func assertAPINotifications(t *testing.T, env *connectAPITestEnv, ctx context.Context, present []string, absent []string) {
	t.Helper()
	resp, err := env.notifications.ListNotifications(ctx, connect.NewRequest(&apiv1.ListNotificationsRequest{
		Page: &apiv1.PageRequest{Limit: 100},
	}))
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	ids := map[string]bool{}
	for _, notification := range resp.Msg.GetNotifications() {
		ids[notification.GetId()] = true
	}
	for _, id := range present {
		if !ids[id] {
			t.Fatalf("notification %s missing from API list; got %v", id, ids)
		}
	}
	for _, id := range absent {
		if ids[id] {
			t.Fatalf("notification %s still present in API list; got %v", id, ids)
		}
	}
}

func assertRoomNotificationCount(t *testing.T, env *connectAPITestEnv, ctx context.Context, roomID string, want int32) {
	t.Helper()
	resp, err := env.notifications.ListRoomNotificationCounts(ctx, connect.NewRequest(&apiv1.ListRoomNotificationCountsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomNotificationCounts: %v", err)
	}
	got := int32(0)
	for _, count := range resp.Msg.GetRoomCounts() {
		if count.GetRoomId() == roomID {
			got = count.GetTotalCount()
			break
		}
	}
	if got != want {
		t.Fatalf("room notification count for %s = %d, want %d", roomID, got, want)
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
	if state := followResp.Msg.GetState(); state.GetRoomId() != room.Id || state.GetThreadRootEventId() != root.Id || !state.GetFollowing() {
		t.Fatalf("FollowThread state = %+v, want current followed thread", state)
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
	if state := unfollowResp.Msg.GetState(); state.GetRoomId() != room.Id || state.GetThreadRootEventId() != root.Id || state.GetFollowing() {
		t.Fatalf("UnfollowThread state = %+v, want current unfollowed thread", state)
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
	if thread.GetRoom().GetId() != room.Id || thread.GetRoom().GetName() != room.Name || thread.GetThread().GetThreadRootEventId() != root.Id {
		t.Fatalf("followed thread identity = room %q name %q root %q, want room %q name %q root %q", thread.GetRoom().GetId(), thread.GetRoom().GetName(), thread.GetThread().GetThreadRootEventId(), room.Id, room.Name, root.Id)
	}
	if thread.GetThread().GetReplyCount() != 1 || !thread.GetThread().GetViewerState().GetHasUnread() || thread.GetThread().GetLastReplyAt() == nil {
		t.Fatalf("followed thread metadata = replies %d unread %v lastReplyAt %v, want replies 1 unread true lastReplyAt set", thread.GetThread().GetReplyCount(), thread.GetThread().GetViewerState().GetHasUnread(), thread.GetThread().GetLastReplyAt())
	}
	rootMessage := thread.GetRootMessage()
	if rootMessage == nil || rootMessage.GetId() != root.Id {
		t.Fatalf("root message = %+v, want hydrated message %s", rootMessage, root.Id)
	}
	if got := rootMessage.GetBody(); got != "root body" {
		t.Fatalf("root message body = %q, want root body", got)
	}
	users := resp.Msg.GetIncludes().GetUsers()
	if users[env.viewer.Id] == nil || users[participant.Id] == nil {
		t.Fatalf("includes users missing viewer or participant: got %d included users", len(users))
	}
}

func TestThreadServiceListFollowedThreadsFiltersMembershipLoss(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("followed-loss")
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-loss-participant", "Thread Loss Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, participant.Id, core.KindChannel, participant.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom participant: %v", err)
	}
	root := env.post(room.Id, env.viewer.Id, "root body", "")
	env.post(room.Id, participant.Id, "reply body", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if err := env.core.LeaveRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom viewer: %v", err)
	}

	resp, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if got := len(resp.Msg.GetThreads()); got != 0 {
		t.Fatalf("ListFollowedThreads returned %d threads after membership loss, want 0", got)
	}
	if resp.Msg.GetPage().GetTotalCount() != 0 || resp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListFollowedThreads page metadata = total %d hasMore %v, want total 0 hasMore false", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore())
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

func withCaller(ctx context.Context, user *corev1.User) context.Context {
	return authn.SetInfo(ctx, Caller{UserID: user.Id})
}

func withBearerCredential(ctx context.Context, user *corev1.User, token string) context.Context {
	ctx = withCaller(ctx, user)
	return authctx.WithCredential(ctx, authctx.RuntimeCredential{
		Kind:   authctx.RuntimeCredentialKindBearerToken,
		UserID: user.Id,
		Handle: token,
	})
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

func findAPIPermissionDecision(decisions []*adminv1.ScopedPermissionDecision, kind adminv1.PermissionScopeKind, scopeID, permission string) *adminv1.ScopedPermissionDecision {
	for _, decision := range decisions {
		scope := decision.GetScope()
		if scope != nil && scope.GetKind() == kind && scope.GetId() == scopeID && decision.GetPermission() == permission {
			return decision
		}
	}
	return nil
}

func findAPITierRole(roles []*adminv1.TierRole, roleName string) *adminv1.TierRole {
	for _, role := range roles {
		if role.GetRole().GetName() == roleName {
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
	adminDiagnostics *adminDiagnosticsService
	adminEventLog    *adminEventLogService
	adminLayout      *adminRoomLayoutService
	adminUsers       *adminUserManagementService
	assets           *assetService
	assetUploads     *assetUploadService
	directory        *roomDirectoryService
	externalAuth     *externalIdentityAuthService
	messages         *messageService
	notifications    *notificationService
	permissions      *permissionService
	prefs            *notificationPreferencesService
	push             *pushNotificationService
	publicRoles      *publicRoleService
	roles            *roleService
	rooms            *roomService
	serverState      *serverService
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
		adminDiagnostics: &adminDiagnosticsService{api: api},
		adminEventLog:    &adminEventLogService{api: api},
		adminLayout:      &adminRoomLayoutService{api: api},
		adminUsers:       &adminUserManagementService{api: api},
		assets:           &assetService{api: api},
		assetUploads:     &assetUploadService{api: api},
		directory:        &roomDirectoryService{api: api},
		externalAuth:     &externalIdentityAuthService{api: api},
		messages:         &messageService{api: api},
		notifications:    &notificationService{api: api},
		permissions:      &permissionService{api: api},
		prefs:            &notificationPreferencesService{api: api},
		push:             &pushNotificationService{api: api},
		publicRoles:      &publicRoleService{api: api},
		roles:            &roleService{api: api},
		rooms:            &roomService{api: api},
		serverState:      &serverService{api: api},
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

func apiCapabilityGranted(grants []*apiv1.CapabilityGrant, capability string) bool {
	for _, grant := range grants {
		if grant.GetCapability() == capability {
			return grant.GetGranted()
		}
	}
	return false
}

func apiPermissionGrantPresent(grants []*apiv1.PermissionGrant, permission string) bool {
	for _, grant := range grants {
		if grant.GetPermission() == permission {
			return true
		}
	}
	return false
}

func apiRoomPermissionGranted(room *apiv1.RoomWithViewerState, permission core.Permission) bool {
	return apiPermissionGranted(room.GetViewerState().GetPermissions(), string(permission))
}

func apiRoomGroupPermissionGranted(group *apiv1.RoomGroup, permission core.Permission) bool {
	return apiPermissionGranted(group.GetViewerState().GetPermissions(), string(permission))
}

func apiPermissionGranted(grants []*apiv1.PermissionGrant, permission string) bool {
	for _, grant := range grants {
		if grant.GetPermission() == permission {
			return grant.GetGranted()
		}
	}
	return false
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

func (e *connectAPITestEnv) uploadAttachmentAsset(t testing.TB, roomID, filename, contentType string, content []byte) string {
	t.Helper()
	sum := sha256.Sum256(content)
	ctx := withCaller(e.ctx, e.viewer)
	created, err := e.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      roomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	chunkSum := sha256.Sum256(content)
	if _, err := e.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    created.Msg.GetUpload().GetUploadId(),
		Content:     content,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	completed, err := e.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: created.Msg.GetUpload().GetUploadId(),
	}))
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	assetID := completed.Msg.GetAsset().GetAssetId()
	if assetID == "" {
		t.Fatal("completed upload asset id is empty")
	}
	return assetID
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

func directoryRoomsByID(rooms []*apiv1.RoomWithViewerState) map[string]*apiv1.RoomWithViewerState {
	result := make(map[string]*apiv1.RoomWithViewerState, len(rooms))
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

func findAdminRoomLayoutGroup(groups []*adminv1.AdminRoomLayoutGroup, groupID string) *adminv1.AdminRoomLayoutGroup {
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

func adminRoomLayoutItemsContainRoom(items []*adminv1.AdminRoomLayoutItem, roomID string) bool {
	for _, item := range items {
		room := item.GetRoom()
		if room != nil && room.GetId() == roomID {
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
