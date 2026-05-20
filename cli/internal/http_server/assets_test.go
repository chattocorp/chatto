package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
)

// ============================================================================
// Asset Test Helpers
// ============================================================================

// assetTestEnv holds all test dependencies for asset tests
type assetTestEnv struct {
	server *httptest.Server
	client *http.Client
	core   *core.ChattoCore
	ctx    context.Context
}

// setupAssetTestServer creates a test server for asset testing with caching enabled.
func setupAssetTestServer(t *testing.T) *assetTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Start embedded NATS server
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()
	if !ns.ReadyForConnections(5 * 1e9) {
		t.Fatal("NATS server not ready")
	}
	t.Cleanup(func() { ns.Shutdown() })

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Create ChattoCore with assets config and caching enabled
	coreConfig := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret-32-bytes-!!",
			MaxUploadSize: 10 * 1024 * 1024, // 10MB
			Cache: config.AssetsCacheConfig{
				Enabled: true,
				TTL:     config.Duration(7 * 24 * time.Hour), // 7 days
			},
		},
	}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Create router with session middleware
	router := gin.New()
	router.Use(gin.Recovery())

	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	sessionStore.Options(sessions.Options{
		MaxAge:   60 * 60 * 24 * 90,
		HttpOnly: true,
		Secure:   false,
		Path:     "/",
	})
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	// Create HTTPServer
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
			Core: coreConfig,
		},
		nc:     nc,
		router: router,
		core:   chattoCore,
		mailer: email.NewMockSender(true),
		logger: log.WithPrefix("test"),
	}

	s.setupAuthRoutes()
	s.setupGraphQLAPI(s.buildAllowedOrigins())
	s.setupAssetRoutes()

	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &assetTestEnv{
		server: ts,
		client: client,
		core:   chattoCore,
		ctx:    ctx,
	}
}

// login authenticates a user
func (env *assetTestEnv) login(t *testing.T, login, password string) {
	t.Helper()

	loginBody := fmt.Sprintf(`{"login":"%s","password":"%s"}`, login, password)
	resp, err := env.client.Post(env.server.URL+"/auth/login", "application/json", bytes.NewReader([]byte(loginBody)))
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", resp.StatusCode)
	}
}

// createAssetTestPNG creates a simple PNG image for testing
func createAssetTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a test color
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("Failed to encode PNG: %v", err)
	}
	return buf.Bytes()
}

// doAssetMultipartUpload performs a GraphQL multipart upload request for attachments
func (env *assetTestEnv) doAssetMultipartUpload(t *testing.T, operations string, fileData []byte, fileName string) *graphqlResponse {
	t.Helper()

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add operations field
	if err := writer.WriteField("operations", operations); err != nil {
		t.Fatalf("Failed to write operations: %v", err)
	}

	// Add map field (maps file to variable)
	if err := writer.WriteField("map", `{"0": ["variables.file"]}`); err != nil {
		t.Fatalf("Failed to write map: %v", err)
	}

	// Add file with correct content type header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="0"; filename="%s"`, fileName))
	h.Set("Content-Type", "image/png") // Set content type explicitly for PNG images
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(fileData)); err != nil {
		t.Fatalf("Failed to copy file data: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	// Make request
	req, err := http.NewRequest("POST", env.server.URL+"/api/graphql", &body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return &gqlResp
}

// doAssetGraphQL helper for non-upload GraphQL requests
func (env *assetTestEnv) doAssetGraphQL(t *testing.T, query string, variables map[string]any) *graphqlResponse {
	t.Helper()

	reqBody := graphqlRequest{
		Query:     query,
		Variables: variables,
	}

	body, _ := json.Marshal(reqBody)
	resp, err := env.client.Post(env.server.URL+"/api/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return &gqlResp
}

// ============================================================================
// Asset Caching Tests
// ============================================================================

func TestAsset_TransformedImage_CacheHitMiss(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create user and space with room
	user, err := env.core.CreateUser(env.ctx, "system", "cacheuser", "Cache User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login
	env.login(t, "cacheuser", "password123")

	// Upload an attachment via postMessage mutation
	imageData := createAssetTestPNG(t, 800, 600)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url thumbnailUrl(width: 200, height: 200, fit: CONTAIN) } } } } }",
		"variables": { "roomId": "%s", "body": "Test message with image", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "test-image.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message with attachment: %v", resp.Errors)
	}

	// Extract attachment info
	var data struct {
		PostMessage struct {
			Event struct {
				Attachments []struct {
					ID           string `json:"id"`
					URL          string `json:"url"`
					ThumbnailURL string `json:"thumbnailUrl"`
				} `json:"attachments"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(data.PostMessage.Event.Attachments) == 0 {
		t.Fatal("Expected at least one attachment")
	}

	attachment := data.PostMessage.Event.Attachments[0]

	// First request to transformed URL should be a cache MISS
	transformResp, err := env.client.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp.Body.Close()

	if transformResp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", transformResp.StatusCode)
	}

	// Wait a bit for the async cache store to complete
	time.Sleep(100 * time.Millisecond)

	// Second request should be a cache HIT
	transformResp2, err := env.client.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp2.Body.Close()

	if transformResp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", transformResp2.StatusCode)
	}

	xCache := transformResp2.Header.Get("X-Cache")
	if xCache != "HIT" {
		t.Errorf("Expected X-Cache: HIT, got: %s", xCache)
	}
}

func TestAsset_DeleteAttachment_CleansUpCache(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create user and space with room
	user, err := env.core.CreateUser(env.ctx, "system", "cleanupuser", "Cleanup User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login
	env.login(t, "cleanupuser", "password123")

	// Upload an attachment
	imageData := createAssetTestPNG(t, 800, 600)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { id event { ... on MessagePostedEvent { attachments { id url thumbnailUrl(width: 200, height: 200, fit: CONTAIN) } } } } }",
		"variables": { "roomId": "%s", "body": "Test message for cleanup", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "cleanup-test.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", resp.Errors)
	}

	var data struct {
		PostMessage struct {
			ID    string `json:"id"`
			Event struct {
				Attachments []struct {
					ID           string `json:"id"`
					URL          string `json:"url"`
					ThumbnailURL string `json:"thumbnailUrl"`
				} `json:"attachments"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	eventID := data.PostMessage.ID
	attachment := data.PostMessage.Event.Attachments[0]

	// Request transformed image to populate cache
	transformResp, err := env.client.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp.Body.Close()
	if transformResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", transformResp.StatusCode)
	}

	// Wait for async cache store
	time.Sleep(100 * time.Millisecond)

	// Verify cache hit
	transformResp2, err := env.client.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp2.Body.Close()
	if transformResp2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("Expected cache HIT before deletion")
	}

	// Delete the message (which should delete the attachment and its cache)
	deleteResp := env.doAssetGraphQL(t, `mutation($input: DeleteMessageInput!) {
		deleteMessage(input: $input)
	}`, map[string]any{
		"input": map[string]any{
			"roomId":  room.Id,
			"eventId": eventID,
		},
	})

	if len(deleteResp.Errors) > 0 {
		t.Fatalf("Failed to delete message: %v", deleteResp.Errors)
	}

	// Original attachment URL should now return 404
	originalResp, err := env.client.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to get original attachment: %v", err)
	}
	originalResp.Body.Close()
	if originalResp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for deleted attachment, got %d", originalResp.StatusCode)
	}

	// Transformed URL should also return 404 (not cache hit from stale cache)
	transformResp3, err := env.client.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp3.Body.Close()
	if transformResp3.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for deleted attachment transform, got %d", transformResp3.StatusCode)
	}
}

func TestAsset_OriginalAttachment_ServesCorrectly(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create user and space with room
	user, err := env.core.CreateUser(env.ctx, "system", "serveuser", "Serve User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login
	env.login(t, "serveuser", "password123")

	// Upload an attachment
	imageData := createAssetTestPNG(t, 400, 300)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url contentType } } } } }",
		"variables": { "roomId": "%s", "body": "Test message", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "serve-test.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", resp.Errors)
	}

	var data struct {
		PostMessage struct {
			Event struct {
				Attachments []struct {
					ID          string `json:"id"`
					URL         string `json:"url"`
					ContentType string `json:"contentType"`
				} `json:"attachments"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	attachment := data.PostMessage.Event.Attachments[0]

	// Get original attachment
	originalResp, err := env.client.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to get original attachment: %v", err)
	}
	defer originalResp.Body.Close()

	if originalResp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", originalResp.StatusCode)
	}

	// Should have correct content type
	contentType := originalResp.Header.Get("Content-Type")
	if contentType != "image/png" {
		t.Errorf("Expected Content-Type: image/png, got: %s", contentType)
	}

	// Body should be readable
	body, err := io.ReadAll(originalResp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if len(body) == 0 {
		t.Error("Expected non-empty response body")
	}
}

func TestAsset_OriginalAttachment_HasCacheHeaders(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create user and space with room
	user, err := env.core.CreateUser(env.ctx, "system", "cacheheaderuser", "Cache Header User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login
	env.login(t, "cacheheaderuser", "password123")

	// Upload an attachment
	imageData := createAssetTestPNG(t, 400, 300)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url } } } } }",
		"variables": { "roomId": "%s", "body": "Test message", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "cache-header-test.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", resp.Errors)
	}

	var data struct {
		PostMessage struct {
			Event struct {
				Attachments []struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"attachments"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	attachment := data.PostMessage.Event.Attachments[0]

	// Get original attachment
	originalResp, err := env.client.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to get original attachment: %v", err)
	}
	defer originalResp.Body.Close()

	if originalResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", originalResp.StatusCode)
	}

	// Verify caching headers
	cacheControl := originalResp.Header.Get("Cache-Control")
	if cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("Expected Cache-Control: public, max-age=31536000, immutable, got: %s", cacheControl)
	}

	etag := originalResp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header to be set")
	}

	vary := originalResp.Header.Get("Vary")
	if vary != "Accept-Encoding" {
		t.Errorf("Expected Vary: Accept-Encoding, got: %s", vary)
	}
}

func TestAsset_ServerAsset_HasCacheHeaders(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create a user with an avatar (server asset)
	user, err := env.core.CreateUser(env.ctx, "system", "serverassetuser", "Instance Asset User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload an avatar for the user
	avatarData := createAssetTestPNG(t, 200, 200)
	avatarPath := fmt.Sprintf("avatar/%s.png", user.Id)

	store := env.core.ServerStore()
	_, err = store.PutBytes(env.ctx, avatarPath, avatarData)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}

	// Get the server asset (avatars are public, no auth needed)
	resp, err := env.client.Get(env.server.URL + "/assets/server/" + avatarPath)
	if err != nil {
		t.Fatalf("Failed to get server asset: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// Verify caching headers
	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("Expected Cache-Control: public, max-age=31536000, immutable, got: %s", cacheControl)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header to be set")
	}
	// ETag should contain the path
	expectedETag := fmt.Sprintf("\"%s\"", avatarPath)
	if etag != expectedETag {
		t.Errorf("Expected ETag: %s, got: %s", expectedETag, etag)
	}

	vary := resp.Header.Get("Vary")
	if vary != "Accept-Encoding" {
		t.Errorf("Expected Vary: Accept-Encoding, got: %s", vary)
	}
}

func TestAsset_UnauthenticatedAccess_Denied(t *testing.T) {
	env := setupAssetTestServer(t)

	// Create user and space with room
	user, err := env.core.CreateUser(env.ctx, "system", "authuser", "Auth User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login to upload
	env.login(t, "authuser", "password123")

	// Upload an attachment
	imageData := createAssetTestPNG(t, 400, 300)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url thumbnailUrl(width: 200, height: 200, fit: CONTAIN) } } } } }",
		"variables": { "roomId": "%s", "body": "Test message", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "auth-test.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", resp.Errors)
	}

	var data struct {
		PostMessage struct {
			Event struct {
				Attachments []struct {
					URL          string `json:"url"`
					ThumbnailURL string `json:"thumbnailUrl"`
				} `json:"attachments"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	attachment := data.PostMessage.Event.Attachments[0]

	// Create a new client without cookies (unauthenticated)
	unauthClient := &http.Client{}

	// Try to access original attachment - should be denied
	originalResp, err := unauthClient.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	originalResp.Body.Close()

	if originalResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for unauthenticated access, got %d", originalResp.StatusCode)
	}

	// Try to access transformed attachment - should also be denied
	transformResp, err := unauthClient.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	transformResp.Body.Close()

	if transformResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for unauthenticated transform access, got %d", transformResp.StatusCode)
	}
}
