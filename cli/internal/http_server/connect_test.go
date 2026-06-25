package http_server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	graphauth "hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func setupConnectTestServer(t *testing.T, authConfig config.AuthConfig) (*HTTPServer, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	s := setupServerInfoServer(t, authConfig)
	s.setupConnectAPI()

	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	return s, ts
}

func TestConnectServerServiceGetServer(t *testing.T) {
	t.Run("returns public server metadata", func(t *testing.T) {
		_, ts := setupConnectTestServer(t, config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
			},
		})

		client := apiv1connect.NewServerServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		resp, err := client.GetServer(context.Background(), connect.NewRequest(&apiv1.GetServerRequest{}))
		if err != nil {
			t.Fatalf("GetServer: %v", err)
		}

		msg := resp.Msg
		if msg.Name != "Chatto" {
			t.Fatalf("Name = %q, want Chatto", msg.Name)
		}
		if msg.Version != "1.2.3" {
			t.Fatalf("Version = %q, want 1.2.3", msg.Version)
		}
		if got, want := strings.Join(msg.AuthMethods, ","), "password,oidc"; got != want {
			t.Fatalf("AuthMethods = %v, want %s", msg.AuthMethods, want)
		}
		if !msg.RegistrationOpen {
			t.Fatal("RegistrationOpen = false, want true")
		}
		if msg.AuthorizeUrl != "/oauth/authorize" {
			t.Fatalf("AuthorizeUrl = %q, want /oauth/authorize", msg.AuthorizeUrl)
		}
		if len(msg.AuthProviders) != 1 {
			t.Fatalf("AuthProviders len = %d, want 1", len(msg.AuthProviders))
		}
		provider := msg.AuthProviders[0]
		if provider.Id != "hub" || provider.Type != config.AuthProviderTypeOpenIDConnect || provider.Label != "Chatto Hub" || provider.LoginUrl != "/auth/providers/hub" {
			t.Fatalf("AuthProviders[0] = %+v", provider)
		}
	})

	t.Run("serves protobuf over HTTP", func(t *testing.T) {
		_, ts := setupConnectTestServer(t, config.AuthConfig{})

		body := strings.NewReader("")
		req, err := http.NewRequest(http.MethodPost, ts.URL+connectAPIPrefix+apiv1connect.ServerServiceGetServerProcedure, body)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/proto")

		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("raw Connect request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/proto") {
			t.Fatalf("Content-Type = %q, want application/proto", ct)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var msg apiv1.GetServerResponse
		if err := proto.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if msg.Name != "Chatto" {
			t.Fatalf("Name = %q, want Chatto", msg.Name)
		}
	})
}

func TestConnectAPIRejectsOversizedRequestMessages(t *testing.T) {
	s, ts := setupConnectTestServer(t, config.AuthConfig{})
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-oversized", "Connect Oversized", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := s.core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
	req := connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: "oversized-room",
		Body:   strings.Repeat("a", connectapi.MaxRequestMessageBytes),
	})
	req.Header().Set("Authorization", "Bearer "+token)
	_, err = client.PostMessage(ctx, req)
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("PostMessage oversized err = %v, want resource exhausted", err)
	}
}

func TestConnectAPIValidatesRequiredRequestFields(t *testing.T) {
	s, ts := setupConnectTestServer(t, config.AuthConfig{})
	ctx := context.Background()
	user, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-validation", "Connect Validation", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := s.core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	authorize := func(req interface{ Header() http.Header }) {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	requireInvalidArgument := func(t *testing.T, err error) {
		t.Helper()
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("err = %v, want invalid argument", err)
		}
	}

	t.Run("message room id", func(t *testing.T) {
		client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.PostMessageRequest{Body: "hello"})
		authorize(req)
		_, err := client.PostMessage(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("read state room id", func(t *testing.T) {
		client := apiv1connect.NewReadStateServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{})
		authorize(req)
		_, err := client.MarkRoomAsRead(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("read state thread root id", func(t *testing.T) {
		client := apiv1connect.NewReadStateServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.MarkThreadAsReadRequest{RoomId: "room"})
		authorize(req)
		_, err := client.MarkThreadAsRead(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("timeline room id", func(t *testing.T) {
		client := apiv1connect.NewRoomTimelineServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetRoomEventsRequest{})
		authorize(req)
		_, err := client.GetRoomEvents(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("timeline event id", func(t *testing.T) {
		client := apiv1connect.NewRoomTimelineServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetRoomEventsAroundRequest{RoomId: "room"})
		authorize(req)
		_, err := client.GetRoomEventsAround(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread timeline root id", func(t *testing.T) {
		client := apiv1connect.NewRoomTimelineServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetThreadEventsRequest{RoomId: "room"})
		authorize(req)
		_, err := client.GetThreadEvents(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread timeline event id", func(t *testing.T) {
		client := apiv1connect.NewRoomTimelineServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
			RoomId:            "room",
			ThreadRootEventId: "root",
		})
		authorize(req)
		_, err := client.GetThreadEventsAround(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread follow room id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.FollowThreadRequest{ThreadRootEventId: "root"})
		authorize(req)
		_, err := client.FollowThread(ctx, req)
		requireInvalidArgument(t, err)
	})

	t.Run("thread unfollow root id", func(t *testing.T) {
		client := apiv1connect.NewThreadServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.UnfollowThreadRequest{RoomId: "room"})
		authorize(req)
		_, err := client.UnfollowThread(ctx, req)
		requireInvalidArgument(t, err)
	})
}

func TestConnectAPIAuthenticatesBeforeValidation(t *testing.T) {
	_, ts := setupConnectTestServer(t, config.AuthConfig{})

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
	_, err := client.PostMessage(context.Background(), connect.NewRequest(&apiv1.PostMessageRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("PostMessage err = %v, want unauthenticated", err)
	}
}

func TestAuthenticateConnectRequest(t *testing.T) {
	t.Run("rejects missing injected user", func(t *testing.T) {
		_, err := authenticateConnectRequest(context.Background(), nil)
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("authenticateConnectRequest err = %v, want unauthenticated", err)
		}
	})

	t.Run("returns narrow Connect caller", func(t *testing.T) {
		info, err := authenticateConnectRequest(graphauth.WithUser(context.Background(), &corev1.User{
			Id:          "user-123",
			Login:       "should-not-leak",
			DisplayName: "Should Not Leak",
		}), nil)
		if err != nil {
			t.Fatalf("authenticateConnectRequest: %v", err)
		}
		caller, ok := info.(connectapi.Caller)
		if !ok {
			t.Fatalf("auth info type = %T, want connectapi.Caller", info)
		}
		if caller != (connectapi.Caller{UserID: "user-123"}) {
			t.Fatalf("caller = %+v, want user id only", caller)
		}
	})
}

func TestConnectNotificationPreferencesService(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		_, err = client.SetRoomNotificationLevel(ctx, connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("SetRoomNotificationLevel err = %v, want unauthenticated", err)
		}

		_, err = client.GetRoomNotificationPreference(ctx, connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("GetRoomNotificationPreference err = %v, want unauthenticated", err)
		}
	})

	t.Run("requires room membership", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser(member): %v", err)
		}
		other, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-other", "Connect Other", "password")
		if err != nil {
			t.Fatalf("CreateUser(other): %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, other.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.SetRoomNotificationLevel(ctx, req)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("SetRoomNotificationLevel err = %v, want permission denied", err)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		_, err = client.GetRoomNotificationPreference(ctx, getReq)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("GetRoomNotificationPreference err = %v, want permission denied", err)
		}
	})

	t.Run("rejects invalid room notification requests", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, member.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: "",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.SetRoomNotificationLevel(ctx, req)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("SetRoomNotificationLevel empty room err = %v, want invalid argument", err)
		}

		req = connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: "missing-room",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.SetRoomNotificationLevel(ctx, req)
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("SetRoomNotificationLevel missing room err = %v, want not found", err)
		}

		req = connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: "missing-room",
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		_, err = client.SetRoomNotificationLevel(ctx, req)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("SetRoomNotificationLevel unspecified level err = %v, want invalid argument", err)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: "",
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		_, err = client.GetRoomNotificationPreference(ctx, getReq)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("GetRoomNotificationPreference empty room err = %v, want invalid argument", err)
		}
	})

	t.Run("sets a room notification level for a member", func(t *testing.T) {
		s, ts := setupConnectTestServer(t, config.AuthConfig{})
		ctx := context.Background()
		member, err := s.core.CreateUser(ctx, core.SystemActorID, "connect-member", "Connect Member", "password")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		room, err := s.core.CreateRoom(ctx, member.Id, core.KindChannel, "", "connect-room", "")
		if err != nil {
			t.Fatalf("CreateRoom: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, member.Id)
		if err != nil {
			t.Fatalf("CreateAuthToken: %v", err)
		}

		client := apiv1connect.NewNotificationPreferencesServiceClient(ts.Client(), ts.URL+connectAPIPrefix)
		req := connect.NewRequest(&apiv1.SetRoomNotificationLevelRequest{
			RoomId: room.Id,
			Level:  apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		})
		req.Header().Set("Authorization", "Bearer "+token)
		resp, err := client.SetRoomNotificationLevel(ctx, req)
		if err != nil {
			t.Fatalf("SetRoomNotificationLevel: %v", err)
		}
		if resp.Msg.Level != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Level = %v, want muted", resp.Msg.Level)
		}
		if resp.Msg.EffectiveLevel != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("EffectiveLevel = %v, want muted", resp.Msg.EffectiveLevel)
		}

		getReq := connect.NewRequest(&apiv1.GetRoomNotificationPreferenceRequest{
			RoomId: room.Id,
		})
		getReq.Header().Set("Authorization", "Bearer "+token)
		getResp, err := client.GetRoomNotificationPreference(ctx, getReq)
		if err != nil {
			t.Fatalf("GetRoomNotificationPreference: %v", err)
		}
		if getResp.Msg.Level != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Get level = %v, want muted", getResp.Msg.Level)
		}
		if getResp.Msg.EffectiveLevel != apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("Get effective level = %v, want muted", getResp.Msg.EffectiveLevel)
		}

		got, err := s.core.GetRoomNotificationLevel(ctx, member.Id, room.Id)
		if err != nil {
			t.Fatalf("GetRoomNotificationLevel: %v", err)
		}
		if got != corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			t.Fatalf("stored level = %v, want muted", got)
		}
	})
}
