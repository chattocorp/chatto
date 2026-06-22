package connectapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
		"/" + apiv1connect.NotificationPreferencesServiceName + "/",
		"/" + apiv1connect.ServerServiceName + "/",
	}
	sort.Strings(want)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("handler paths = %v, want %v", paths, want)
	}
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
		{"core not found", core.ErrNotFound, connect.CodeNotFound},
		{"jetstream key not found", jetstream.ErrKeyNotFound, connect.CodeNotFound},
		{"unknown", errors.New("boom"), connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connect.CodeOf(connectError(tt.err)); got != tt.code {
				t.Fatalf("connectError code = %v, want %v", got, tt.code)
			}
		})
	}
}
