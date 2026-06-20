package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// bannerImageBytes returns an in-memory PNG suitable as a banner upload.
// Banners double as OG link-preview images at 1200x630.
func bannerImageBytes(t *testing.T) io.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 630))
	for y := 0; y < 630; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

// setupServerInfoServer creates a minimal HTTPServer for instance info endpoint tests.
func setupServerInfoServer(t *testing.T, authConfig config.AuthConfig) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	router := gin.New()
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	router.Use(sessions.Sessions("chatto_session", sessionStore))
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: authConfig,
			Webserver: config.WebserverConfig{
				URL:                 "http://chat.example.test",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		version: "1.2.3",
		metrics: newProcessMetrics(),
	}
	allowedOrigins := s.buildAllowedOrigins()
	s.router.Use(s.corsMiddleware(allowedOrigins))
	s.setupServerInfoRoutes()
	s.setupLiveRoutes(allowedOrigins)

	return s
}

func TestServerInfo(t *testing.T) {
	t.Run("returns correct JSON structure with defaults", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Name != "Chatto" {
			t.Errorf("expected name 'Chatto', got %q", resp.Name)
		}
		if resp.Version != "1.2.3" {
			t.Errorf("expected version '1.2.3', got %q", resp.Version)
		}
		if !resp.RegistrationOpen {
			t.Error("expected registrationOpen true by default")
		}
	})

	t.Run("includes password in authMethods when direct registration enabled", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if len(resp.AuthMethods) != 1 || resp.AuthMethods[0] != "password" {
			t.Errorf("expected authMethods [password], got %v", resp.AuthMethods)
		}
	})

	t.Run("includes configured auth provider metadata", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
				{ID: "github-main", Type: config.AuthProviderTypeGitHub},
			},
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if got, want := resp.AuthMethods, []string{"password", "oidc", "github"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("authMethods = %v, want %v", got, want)
		}
		if len(resp.AuthProviders) != 2 {
			t.Fatalf("authProviders len = %d, want 2", len(resp.AuthProviders))
		}
		if resp.AuthProviders[0].ID != "hub" || resp.AuthProviders[0].Type != config.AuthProviderTypeOpenIDConnect || resp.AuthProviders[0].Label != "Chatto Hub" || resp.AuthProviders[0].LoginURL != "/auth/providers/hub" {
			t.Fatalf("authProviders[0] = %+v", resp.AuthProviders[0])
		}
		if resp.AuthProviders[1].ID != "github-main" || resp.AuthProviders[1].Type != config.AuthProviderTypeGitHub || resp.AuthProviders[1].Label != "GitHub" || resp.AuthProviders[1].LoginURL != "/auth/providers/github-main" {
			t.Fatalf("authProviders[1] = %+v", resp.AuthProviders[1])
		}
	})

	t.Run("registration disabled hides password and sets registrationOpen false", func(t *testing.T) {
		disabled := false
		s := setupServerInfoServer(t, config.AuthConfig{
			DirectRegistration: &disabled,
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.RegistrationOpen {
			t.Error("expected registrationOpen false")
		}
		// authMethods should be empty (no password, no SSO)
		if len(resp.AuthMethods) != 0 {
			t.Errorf("expected empty authMethods, got %v", resp.AuthMethods)
		}
	})

	t.Run("returns empty array not null for authMethods", func(t *testing.T) {
		disabled := false
		s := setupServerInfoServer(t, config.AuthConfig{
			DirectRegistration: &disabled,
		})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Parse raw JSON to check for null vs empty array
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if string(raw["authMethods"]) == "null" {
			t.Error("authMethods should be [] not null")
		}
	})

	t.Run("includes authorizeUrl for OAuth discovery", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.AuthorizeURL != "/oauth/authorize" {
			t.Errorf("expected authorizeUrl '/oauth/authorize', got %q", resp.AuthorizeURL)
		}
	})

	t.Run("sets CORS headers", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin *, got %q", origin)
		}
		if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods 'GET, OPTIONS', got %q", methods)
		}
		if headers := w.Header().Get("Access-Control-Allow-Headers"); headers != "Authorization, Content-Type" {
			t.Errorf("expected Access-Control-Allow-Headers 'Authorization, Content-Type', got %q", headers)
		}
	})

	t.Run("includes Chatto live discovery", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "chat.example.test"
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp.Live == nil {
			t.Fatal("expected live discovery metadata")
		}
		if resp.Live.URL != "ws://chat.example.test/api/live" {
			t.Errorf("live.url = %q, want ws://chat.example.test/api/live", resp.Live.URL)
		}
		if resp.Live.TokenURL != "/api/live-token" {
			t.Errorf("live.tokenUrl = %q, want /api/live-token", resp.Live.TokenURL)
		}
		if resp.Live.Protocol != clientLiveProtocol {
			t.Errorf("live.protocol = %q", resp.Live.Protocol)
		}
	})

	t.Run("sets Cache-Control header", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
			t.Errorf("expected Cache-Control 'public, max-age=300', got %q", cc)
		}
	})

	t.Run("absolutizes bannerUrl using request scheme/host when a banner is set", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		// Configure a banner on the instance (simulates an admin upload).
		// The Core helper returns a relative URL when AssetBaseURL is empty
		// (the case in this test), so we exercise the http_server's
		// absolutize path.
		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		// Request via plain http.
		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com"
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "http://remote.example.com/") {
			t.Errorf("expected absolute http://remote.example.com URL, got %q", resp.BannerURL)
		}
	})

	t.Run("absolutizes bannerUrl as https when X-Forwarded-Proto is https", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com"
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "https://remote.example.com/") {
			t.Errorf("expected absolute https://remote.example.com URL, got %q", resp.BannerURL)
		}
	})

	t.Run("preserves already-absolute bannerUrl from AssetBaseURL", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})
		// Mirror what cmd/run.go does when [webserver] url is configured.
		s.core.AssetBaseURL = "https://chat.example.com"

		ctx := testContext(t)
		asset, err := s.core.UploadServerBanner(ctx, bannerImageBytes(t))
		if err != nil {
			t.Fatalf("upload banner: %v", err)
		}
		if err := s.core.SetServerBanner(ctx, "test-admin", asset); err != nil {
			t.Fatalf("set banner: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/server", nil)
		req.Host = "remote.example.com" // different from AssetBaseURL
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		var resp serverInfoResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if !strings.HasPrefix(resp.BannerURL, "https://chat.example.com/") {
			t.Errorf("expected absolute URL to keep AssetBaseURL host, got %q", resp.BannerURL)
		}
	})

	t.Run("omits bannerUrl when no banner is set", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("GET", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		// Inspect the raw JSON: the JSON tag is `omitempty`, so when no
		// banner is configured the field must not appear at all (rather
		// than serialize as `"bannerUrl": ""`).
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if _, present := raw["bannerUrl"]; present {
			t.Errorf("expected bannerUrl absent when no banner set, got %s", string(raw["bannerUrl"]))
		}
	})

	t.Run("OPTIONS preflight returns 204 with CORS headers", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("OPTIONS", "/api/server", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("expected Access-Control-Allow-Origin *, got %q", origin)
		}
		if maxAge := w.Header().Get("Access-Control-Max-Age"); maxAge != "86400" {
			t.Errorf("expected Access-Control-Max-Age '86400', got %q", maxAge)
		}
	})
}

func TestClientLiveToken(t *testing.T) {
	t.Run("requires authenticated bearer token", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("POST", "/api/live-token", nil)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("returns transport credentials for authenticated bearer token", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		ctx := testContext(t)
		user, err := s.core.CreateUser(ctx, "system", "client-live-token-user", "Client Live Token User", "password123")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, user.Id)
		if err != nil {
			t.Fatalf("create auth token: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/live-token", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			URL       string    `json:"url"`
			Ticket    string    `json:"ticket"`
			Protocol  string    `json:"protocol"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if resp.URL != "ws://chat.example.test/api/live" {
			t.Errorf("url = %q, want ws://chat.example.test/api/live", resp.URL)
		}
		if resp.Ticket == "" {
			t.Fatal("ticket is empty")
		}
		if resp.Protocol != clientLiveProtocol {
			t.Errorf("protocol = %q", resp.Protocol)
		}
	})

	t.Run("websocket accepts ticket and sends protobuf hello", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})
		ts := httptest.NewServer(s.router)
		defer ts.Close()

		ctx := testContext(t)
		user, err := s.core.CreateUser(ctx, "system", "client-live-ws-user", "Client Live WS User", "password123")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		token, err := s.core.CreateAuthToken(ctx, user.Id)
		if err != nil {
			t.Fatalf("create auth token: %v", err)
		}

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/live-token", nil)
		if err != nil {
			t.Fatalf("create live token request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("live token request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		var tokenResp struct {
			Ticket string `json:"ticket"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			t.Fatalf("parse response: %v", err)
		}

		wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/live?ticket=" + url.QueryEscape(tokenResp.Ticket)
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial live websocket: %v", err)
		}
		defer conn.Close()

		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read hello: %v", err)
		}
		if messageType != websocket.BinaryMessage {
			t.Fatalf("messageType = %d, want binary", messageType)
		}
		var frame corev1.ClientLiveServerFrame
		if err := proto.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("decode hello: %v", err)
		}
		hello := frame.GetHello()
		if hello == nil {
			t.Fatalf("expected hello frame, got %T", frame.GetPayload())
		}
		if hello.GetProtocol() != clientLiveProtocol {
			t.Fatalf("protocol = %q", hello.GetProtocol())
		}

		room, err := s.core.CreateRoom(ctx, user.Id, "channel", "", "client-live-room", "Client Live Room")
		if err != nil {
			t.Fatalf("create room: %v", err)
		}
		if _, err := s.core.JoinRoom(ctx, user.Id, "channel", user.Id, room.Id); err != nil {
			t.Fatalf("join room: %v", err)
		}
		posted, err := s.core.PostMessage(ctx, "channel", room.Id, user.Id, "hello client live", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("post message: %v", err)
		}

		deadline := time.Now().Add(5 * time.Second)
		for {
			if err := conn.SetReadDeadline(deadline); err != nil {
				t.Fatalf("set read deadline: %v", err)
			}
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read live event: %v", err)
			}
			if messageType != websocket.BinaryMessage {
				t.Fatalf("messageType = %d, want binary", messageType)
			}
			var eventFrame corev1.ClientLiveServerFrame
			if err := proto.Unmarshal(payload, &eventFrame); err != nil {
				t.Fatalf("decode live event: %v", err)
			}
			live := eventFrame.GetLiveEvent()
			roomEvent := live.GetRoomEvent()
			if roomEvent == nil || roomEvent.GetMessagePosted() == nil {
				continue
			}
			if roomEvent.GetId() != posted.Id {
				t.Fatalf("delivered room event id = %q, want %q", roomEvent.GetId(), posted.Id)
			}
			if roomEvent.GetMessagePosted().GetBody() != "hello client live" {
				t.Fatalf("delivered message body = %q", roomEvent.GetMessagePosted().GetBody())
			}
			if eventFrame.GetStreamSequence() == 0 {
				t.Fatal("expected stream sequence on delivered event")
			}
			break
		}

		historyReq, err := proto.Marshal(&corev1.ClientRoomEventsRequest{
			RoomId: room.Id,
			Limit:  50,
		})
		if err != nil {
			t.Fatalf("marshal history request: %v", err)
		}
		historyFrame, err := proto.Marshal(&corev1.ClientLiveClientFrame{
			RequestId: 7,
			Payload: &corev1.ClientLiveClientFrame_Request{
				Request: &corev1.ClientLiveRequest{
					Type:    clientLiveRequestRoomEvents,
					Payload: historyReq,
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal history frame: %v", err)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, historyFrame); err != nil {
			t.Fatalf("write history request: %v", err)
		}

		deadline = time.Now().Add(5 * time.Second)
		for {
			if err := conn.SetReadDeadline(deadline); err != nil {
				t.Fatalf("set read deadline: %v", err)
			}
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read history response: %v", err)
			}
			if messageType != websocket.BinaryMessage {
				t.Fatalf("messageType = %d, want binary", messageType)
			}
			var responseFrame corev1.ClientLiveServerFrame
			if err := proto.Unmarshal(payload, &responseFrame); err != nil {
				t.Fatalf("decode history frame: %v", err)
			}
			if responseFrame.GetRequestId() != 7 {
				continue
			}
			response := responseFrame.GetResponse()
			if response == nil {
				t.Fatalf("expected history response, got %T", responseFrame.GetPayload())
			}
			if response.GetType() != clientLiveRequestRoomEvents {
				t.Fatalf("response type = %q, want %q", response.GetType(), clientLiveRequestRoomEvents)
			}
			var page corev1.ClientRoomEventsPage
			if err := proto.Unmarshal(response.GetPayload(), &page); err != nil {
				t.Fatalf("decode history page: %v", err)
			}
			var postedItem *corev1.ClientRoomEventItem
			for _, item := range page.GetEvents() {
				if item.GetEvent().GetId() == posted.Id {
					postedItem = item
					break
				}
			}
			if postedItem == nil {
				t.Fatalf("history response did not include posted event %q", posted.Id)
			}
			if postedItem.GetStreamSequence() == 0 {
				t.Fatal("expected stream sequence on history item")
			}
			if postedItem.GetEvent().GetMessagePosted().GetBody() != "hello client live" {
				t.Fatalf("history message body = %q", postedItem.GetEvent().GetMessagePosted().GetBody())
			}
			break
		}
	})

	t.Run("OPTIONS preflight returns 204 with authorization allowed", func(t *testing.T) {
		s := setupServerInfoServer(t, config.AuthConfig{})

		req := httptest.NewRequest("OPTIONS", "/api/live-token", nil)
		req.Header.Set("Origin", "https://app.example.test")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if headers := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(headers, "Authorization") {
			t.Errorf("expected Access-Control-Allow-Headers to include Authorization, got %q", headers)
		}
	})
}
