package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
)

// fakeLiveKitServer answers ListRooms like LiveKit: it verifies the
// bearer token against the expected key and secret and requires the
// RoomList grant.
func fakeLiveKitServer(t *testing.T, apiKey, apiSecret string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/twirp/livekit.RoomService/ListRooms" {
			http.NotFound(w, r)
			return
		}
		token, err := lkauth.ParseAPIToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil || token.APIKey() != apiKey {
			writeTwirpError(w, http.StatusUnauthorized, "unauthenticated", "invalid API key")
			return
		}
		_, claims, err := token.Verify(apiSecret)
		if err != nil {
			writeTwirpError(w, http.StatusUnauthorized, "unauthenticated", "invalid token")
			return
		}
		if claims.Video == nil || !claims.Video.RoomList {
			writeTwirpError(w, http.StatusForbidden, "permission_denied", "missing room list grant")
			return
		}
		body, err := proto.Marshal(&livekit.ListRoomsResponse{})
		if err != nil {
			t.Errorf("marshal ListRoomsResponse: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/protobuf")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeTwirpError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"code":%q,"msg":%q}`, code, msg)
}

func closedLocalURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return "ws://" + addr
}

func TestLiveKitAdminStatus(t *testing.T) {
	t.Parallel()

	const (
		apiKey    = "diagnostics-key"
		apiSecret = "diagnostics-secret-value"
	)
	liveKit := fakeLiveKitServer(t, apiKey, apiSecret)
	liveKitURL := strings.Replace(liveKit.URL, "http://", "ws://", 1)
	notLiveKit := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(notLiveKit.Close)

	tests := []struct {
		name           string
		cfg            config.LiveKitConfig
		wantConfigured bool
		wantState      LiveKitConnectionState
	}{
		{
			name:      "disabled",
			cfg:       config.LiveKitConfig{URL: liveKitURL, APIKey: apiKey, APISecret: apiSecret},
			wantState: LiveKitConnectionNotConfigured,
		},
		{
			name:      "enabled without secret",
			cfg:       config.LiveKitConfig{Enabled: true, URL: liveKitURL, APIKey: apiKey},
			wantState: LiveKitConnectionNotConfigured,
		},
		{
			name:           "reachable with valid credentials",
			cfg:            config.LiveKitConfig{Enabled: true, URL: liveKitURL, APIKey: apiKey, APISecret: apiSecret},
			wantConfigured: true,
			wantState:      LiveKitConnectionOK,
		},
		{
			name:           "wrong secret",
			cfg:            config.LiveKitConfig{Enabled: true, URL: liveKitURL, APIKey: apiKey, APISecret: "wrong-secret-value"},
			wantConfigured: true,
			wantState:      LiveKitConnectionUnauthorized,
		},
		{
			name:           "wrong key",
			cfg:            config.LiveKitConfig{Enabled: true, URL: liveKitURL, APIKey: "other-key", APISecret: apiSecret},
			wantConfigured: true,
			wantState:      LiveKitConnectionUnauthorized,
		},
		{
			name:           "unreachable",
			cfg:            config.LiveKitConfig{Enabled: true, URL: closedLocalURL(t), APIKey: apiKey, APISecret: apiSecret},
			wantConfigured: true,
			wantState:      LiveKitConnectionUnreachable,
		},
		{
			name:           "not a LiveKit server",
			cfg:            config.LiveKitConfig{Enabled: true, URL: notLiveKit.URL, APIKey: apiKey, APISecret: apiSecret},
			wantConfigured: true,
			wantState:      LiveKitConnectionError,
		},
		{
			name:           "unsupported URL scheme",
			cfg:            config.LiveKitConfig{Enabled: true, URL: "ftp://livekit.example", APIKey: apiKey, APISecret: apiSecret},
			wantConfigured: true,
			wantState:      LiveKitConnectionError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &ChattoCore{}
			c.ConfigureLiveKitDiagnostics(tt.cfg, "https://chat.example/")
			status := c.liveKitAdminStatus(context.Background())

			if status.Configured != tt.wantConfigured {
				t.Errorf("Configured = %v, want %v", status.Configured, tt.wantConfigured)
			}
			if status.ConnectionState != tt.wantState {
				t.Errorf("ConnectionState = %v, want %v (error %q)", status.ConnectionState, tt.wantState, status.ConnectionError)
			}
			if (tt.wantState == LiveKitConnectionOK || tt.wantState == LiveKitConnectionNotConfigured) != (status.ConnectionError == "") {
				t.Errorf("ConnectionError = %q for state %v", status.ConnectionError, status.ConnectionState)
			}
			if status.WebhookURL != "https://chat.example/webhooks/livekit" {
				t.Errorf("WebhookURL = %q", status.WebhookURL)
			}
			if status.APIKey != tt.cfg.APIKey || status.URL != tt.cfg.URL || status.Enabled != tt.cfg.Enabled {
				t.Errorf("status = %+v, want configured URL, key, and enabled flag", status)
			}
			if dump := fmt.Sprintf("%+v", status); tt.cfg.APISecret != "" && strings.Contains(dump, tt.cfg.APISecret) {
				t.Errorf("status exposes API secret: %s", dump)
			}
		})
	}
}

func TestLiveKitAdminStatusReportsWebhookSetup(t *testing.T) {
	t.Parallel()

	c := &ChattoCore{}
	status := c.liveKitAdminStatus(context.Background())
	if status.ConnectionState != LiveKitConnectionNotConfigured || status.WebhookURL != "" || !status.LastWebhookAt.IsZero() {
		t.Fatalf("unconfigured status = %+v", status)
	}

	c.ConfigureLiveKitDiagnostics(config.LiveKitConfig{
		URL:              "wss://livekit.example",
		APIKey:           "key",
		APISecret:        "secret",
		WebhookAPIKey:    "webhook-key",
		WebhookAPISecret: "webhook-secret",
	}, "https://chat.example")
	before := time.Now()
	c.RecordLiveKitWebhook(true)
	status = c.liveKitAdminStatus(context.Background())
	if !status.SeparateWebhookKey {
		t.Error("SeparateWebhookKey = false, want true")
	}
	if status.LastWebhookAt.Before(before) || !status.LastRejectedWebhookAt.IsZero() {
		t.Errorf("after accepted webhook: last = %v, rejected = %v", status.LastWebhookAt, status.LastRejectedWebhookAt)
	}
	c.RecordLiveKitWebhook(false)
	status = c.liveKitAdminStatus(context.Background())
	if status.LastRejectedWebhookAt.Before(before) {
		t.Errorf("LastRejectedWebhookAt = %v, want after %v", status.LastRejectedWebhookAt, before)
	}

	// A webhook key pair that matches the API key, or is incomplete, is not separate.
	c.ConfigureLiveKitDiagnostics(config.LiveKitConfig{APIKey: "key", APISecret: "secret", WebhookAPIKey: "webhook-key"}, "")
	if c.liveKitAdminStatus(context.Background()).SeparateWebhookKey {
		t.Error("SeparateWebhookKey = true for incomplete webhook key pair")
	}
}

func TestLiveKitURLInsecure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		liveKitURL string
		publicURL  string
		want       bool
	}{
		{"wss://livekit.example", "https://chat.example", false},
		{"https://livekit.example", "https://chat.example", false},
		{"ws://livekit.example", "https://chat.example", true},
		{"http://livekit.example", "http://chat.example", true},
		{"ws://localhost:7880", "http://localhost:4000", false},
		{"ws://127.0.0.1:7880", "http://127.0.0.1:4000", false},
		{"ws://localhost:7880", "http://chatto.localhost", false},
		{"ws://[::1]:7880", "http://[::1]:4000", false},
		{"ws://livekit.example", "", true},
		{"", "https://chat.example", false},
	}
	for _, tt := range tests {
		if got := liveKitURLInsecure(tt.liveKitURL, tt.publicURL); got != tt.want {
			t.Errorf("liveKitURLInsecure(%q, %q) = %v, want %v", tt.liveKitURL, tt.publicURL, got, tt.want)
		}
	}
}
