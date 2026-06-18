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
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/signedurl"
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
	return setupAssetTestServerWithConfig(t, false)
}

// setupAssetTestServerWithS3 mirrors setupAssetTestServer but routes
// attachments through an in-memory fake S3 server. Use this to test the
// S3 presigned-redirect code path in the asset handlers (the path that
// previously contained an authorization bypass on empty room ID).
func setupAssetTestServerWithS3(t *testing.T) *assetTestEnv {
	return setupAssetTestServerWithConfig(t, true)
}

func setupAssetTestServerWithConfig(t *testing.T, useS3 bool) *assetTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	assetsCfg := config.AssetsConfig{
		SigningSecret: "test-signing-secret-32-bytes-!!",
		MaxUploadSize: 10 * 1024 * 1024, // 10MB
		Cache: config.AssetsCacheConfig{
			Enabled: true,
			TTL:     config.Duration(7 * 24 * time.Hour), // 7 days
		},
	}
	if useS3 {
		backend := s3mem.New()
		faker := gofakes3.New(backend)
		s3Server := httptest.NewServer(faker.Server())
		t.Cleanup(s3Server.Close)

		useSSL := false
		pathStyle := true
		assetsCfg.StorageBackend = config.StorageBackendS3
		assetsCfg.S3 = config.S3Config{
			Endpoint:        s3Server.URL[len("http://"):],
			Bucket:          "test-bucket",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
			UseSSL:          &useSSL,
			PathStyle:       &pathStyle,
		}
	}
	coreConfig := config.CoreConfig{
		Assets: assetsCfg,
	}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

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
	return env.doAssetMultipartUploadWithContentType(t, operations, fileData, fileName, "image/png")
}

func (env *assetTestEnv) doAssetMultipartUploadWithContentType(t *testing.T, operations string, fileData []byte, fileName, contentType string) *graphqlResponse {
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
	h.Set("Content-Type", contentType)
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

func TestAsset_ActiveAttachment_UsesSandboxHeaders(t *testing.T) {
	env := setupAssetTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "sandboxuser", "Sandbox User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "sandboxroom", "Sandbox Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "sandboxuser", "password123")

	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url contentType } } } } }",
		"variables": { "roomId": "%s", "body": "html attachment", "file": null }
	}`, room.Id)
	resp := env.doAssetMultipartUploadWithContentType(
		t,
		operations,
		[]byte("<!doctype html><script>window.__ran = true</script>"),
		"demo.html",
		"text/html; charset=utf-8",
	)
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message with HTML attachment: %v", resp.Errors)
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
	if len(data.PostMessage.Event.Attachments) != 1 {
		t.Fatalf("Expected one attachment, got %d", len(data.PostMessage.Event.Attachments))
	}
	attachment := data.PostMessage.Event.Attachments[0]

	stableResp, err := env.client.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to fetch stable attachment URL: %v", err)
	}
	stableResp.Body.Close()
	if stableResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected stable attachment status 200, got %d", stableResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, stableResp)

	loc := signedurl.AttachmentLocator{RoomID: room.Id, AttachmentID: attachment.ID}
	signedURL := env.core.GetAttachmentURL(loc, user.Id)
	legacyResp, err := (&http.Client{}).Get(env.server.URL + signedURL)
	if err != nil {
		t.Fatalf("Failed to fetch legacy signed attachment URL: %v", err)
	}
	legacyResp.Body.Close()
	if legacyResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected legacy signed attachment status 200, got %d", legacyResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, legacyResp)
	assertLegacySandboxedAttachmentCache(t, legacyResp)
}

func TestAsset_ActiveAttachmentOnS3_StreamsWithSandboxInsteadOfRedirect(t *testing.T) {
	env := setupAssetTestServerWithS3(t)

	user, err := env.core.CreateUser(env.ctx, "system", "s3sandboxuser", "S3 Sandbox User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "s3sandboxroom", "S3 Sandbox Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "s3sandboxuser", "password123")

	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url } } } } }",
		"variables": { "roomId": "%s", "body": "s3 html attachment", "file": null }
	}`, room.Id)
	resp := env.doAssetMultipartUploadWithContentType(
		t,
		operations,
		[]byte("<!doctype html><script>window.__ran = true</script>"),
		"s3-demo.html",
		"text/html",
	)
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message with S3 HTML attachment: %v", resp.Errors)
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
	if len(data.PostMessage.Event.Attachments) != 1 {
		t.Fatalf("Expected one attachment, got %d", len(data.PostMessage.Event.Attachments))
	}
	attachment := data.PostMessage.Event.Attachments[0]

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	stableResp, err := noRedirectClient.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to fetch S3 stable attachment URL: %v", err)
	}
	stableResp.Body.Close()
	if stableResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected S3 stable attachment to stream with 200, got %d", stableResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, stableResp)

	loc := signedurl.AttachmentLocator{RoomID: room.Id, AttachmentID: attachment.ID}
	signedURL := env.core.GetAttachmentURL(loc, user.Id)
	legacyResp, err := noRedirectClient.Get(env.server.URL + signedURL)
	if err != nil {
		t.Fatalf("Failed to fetch S3 legacy signed attachment URL: %v", err)
	}
	legacyResp.Body.Close()
	if legacyResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected S3 legacy attachment to stream with 200, got %d", legacyResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, legacyResp)
	assertLegacySandboxedAttachmentCache(t, legacyResp)
}

func TestOriginalAttachmentNeedsSandbox(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "HTML", contentType: "text/html", want: true},
		{name: "HTML with parameters", contentType: "text/html; charset=utf-8", want: true},
		{name: "XHTML", contentType: "application/xhtml+xml", want: true},
		{name: "SVG", contentType: "image/svg+xml", want: true},
		{name: "XML", contentType: "application/xml", want: true},
		{name: "XML suffix", contentType: "application/atom+xml", want: true},
		{name: "PNG", contentType: "image/png", want: false},
		{name: "PDF", contentType: "application/pdf", want: false},
		{name: "unknown", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originalAttachmentNeedsSandbox(tt.contentType); got != tt.want {
				t.Fatalf("originalAttachmentNeedsSandbox(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func assertSandboxedOriginalAttachment(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != originalAttachmentSandboxCSP {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, originalAttachmentSandboxCSP)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

func assertLegacySandboxedAttachmentCache(t *testing.T, resp *http.Response) {
	t.Helper()
	want := fmt.Sprintf("private, max-age=%d", int(core.AttachmentURLTTL.Seconds()))
	if got := resp.Header.Get("Cache-Control"); got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
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
	if cacheControl != "private, max-age=3600" {
		t.Errorf("Expected Cache-Control: private, max-age=3600, got: %s", cacheControl)
	}

	etag := originalResp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header to be set")
	}

	vary := originalResp.Header.Get("Vary")
	if vary != "Accept-Encoding, Authorization, Cookie, X-Chatto-Asset-Proxy" {
		t.Errorf("Expected Vary: Accept-Encoding, Authorization, Cookie, X-Chatto-Asset-Proxy, got: %s", vary)
	}
}

func TestAsset_StableURLAcceptsAccessTicketAndBearerAuth(t *testing.T) {
	env := setupAssetTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "bearerassetuser", "Bearer Asset User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "bearer-assets", "Bearer Assets")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	env.login(t, "bearerassetuser", "password123")
	imageData := createAssetTestPNG(t, 120, 90)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url thumbnailUrl(width: 80, height: 80, fit: CONTAIN) } } } } }",
		"variables": { "roomId": "%s", "body": "bearer asset", "file": null }
	}`, room.Id)
	resp := env.doAssetMultipartUpload(t, operations, imageData, "bearer.png")
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
	if len(data.PostMessage.Event.Attachments) == 0 {
		t.Fatal("Expected one attachment")
	}

	attachment := data.PostMessage.Event.Attachments[0]
	unauthClient := &http.Client{}

	withoutAccess, err := url.Parse(attachment.URL)
	if err != nil {
		t.Fatalf("Failed to parse stable URL: %v", err)
	}
	withoutAccess.RawQuery = ""

	unauthResp, err := unauthClient.Get(env.server.URL + withoutAccess.String())
	if err != nil {
		t.Fatalf("Failed to get stable URL without credentials: %v", err)
	}
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected stable URL without credentials to return 401, got %d", unauthResp.StatusCode)
	}

	ticketResp, err := unauthClient.Get(env.server.URL + attachment.URL)
	if err != nil {
		t.Fatalf("Failed to get stable URL with access ticket: %v", err)
	}
	ticketResp.Body.Close()
	if ticketResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected stable URL with access ticket to return 200, got %d", ticketResp.StatusCode)
	}

	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, env.server.URL+withoutAccess.String(), nil)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	bearerResp, err := unauthClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get stable URL with bearer: %v", err)
	}
	bearerResp.Body.Close()
	if bearerResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected bearer stable URL request to return 200, got %d", bearerResp.StatusCode)
	}

	thumbResp, err := unauthClient.Get(env.server.URL + attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get stable thumbnail URL with access ticket: %v", err)
	}
	thumbResp.Body.Close()
	if thumbResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected stable thumbnail request with access ticket to return 200, got %d", thumbResp.StatusCode)
	}

	mutatedThumbnailURL := strings.Replace(attachment.ThumbnailURL, "80x80", "81x80", 1)
	mutatedResp, err := unauthClient.Get(env.server.URL + mutatedThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get mutated stable thumbnail URL: %v", err)
	}
	mutatedResp.Body.Close()
	if mutatedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected mutated stable thumbnail request to return 403, got %d", mutatedResp.StatusCode)
	}

	thumbnailWithoutAccess, err := url.Parse(attachment.ThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to parse stable thumbnail URL: %v", err)
	}
	thumbnailWithoutAccess.RawQuery = ""
	req, err = http.NewRequest(http.MethodGet, env.server.URL+thumbnailWithoutAccess.String(), nil)
	if err != nil {
		t.Fatalf("Failed to build unsigned thumbnail request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	unsignedThumbResp, err := unauthClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to get unsigned stable thumbnail URL with bearer: %v", err)
	}
	unsignedThumbResp.Body.Close()
	if unsignedThumbResp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected unsigned stable thumbnail request with bearer to return 403, got %d", unsignedThumbResp.StatusCode)
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

// TestAsset_SignedURLIsCapability covers the legacy signed-locator URL
// compatibility path. GraphQL now emits authenticated stable URLs, but old
// signed links should continue to work until callers have fully migrated.
func TestAsset_SignedURLIsCapability(t *testing.T) {
	env := setupAssetTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "authuser", "Auth User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "testroom", "Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	env.login(t, "authuser", "password123")

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

	attachment := data.PostMessage.Event.Attachments[0]
	loc := signedurl.AttachmentLocator{RoomID: room.Id, AttachmentID: attachment.ID}
	signedURL := env.core.GetAttachmentURL(loc, user.Id)
	signedThumbnailURL := env.core.GetTransformedAttachmentURL(loc, user.Id, 200, 200, "contain")

	// A no-cookie / no-header client holding the signed URL should be able
	// to fetch the binary — this is the cross-origin <img> case.
	unauthClient := &http.Client{}

	originalResp, err := unauthClient.Get(env.server.URL + signedURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	originalResp.Body.Close()
	if originalResp.StatusCode != http.StatusOK {
		t.Errorf("Signed URL should authorize itself; got status %d", originalResp.StatusCode)
	}

	transformResp, err := unauthClient.Get(env.server.URL + signedThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	transformResp.Body.Close()
	if transformResp.StatusCode != http.StatusOK {
		t.Errorf("Signed transform URL should authorize itself; got status %d", transformResp.StatusCode)
	}

	// A tampered locator must still fail.
	tampered := strings.TrimSuffix(signedURL, "X") + "z"
	tamperedResp, err := unauthClient.Get(env.server.URL + tampered)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	tamperedResp.Body.Close()
	if tamperedResp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 for tampered locator, got %d", tamperedResp.StatusCode)
	}
}

// TestAsset_SignedURLOnS3IsCapability is the S3-backend counterpart to
// TestAsset_SignedURLIsCapability — verifies the signed URL is the
// capability for S3-stored attachments too. The handler redirects to a
// presigned S3 URL once the signed locator's claims (signature + expiry
// + current membership) pass.
func TestAsset_SignedURLOnS3IsCapability(t *testing.T) {
	env := setupAssetTestServerWithS3(t)

	user, err := env.core.CreateUser(env.ctx, "system", "s3authuser", "S3 Auth User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "s3testroom", "S3 Test Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	env.login(t, "s3authuser", "password123")

	imageData := createAssetTestPNG(t, 400, 300)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url thumbnailUrl(width: 200, height: 200, fit: CONTAIN) } } } } }",
		"variables": { "roomId": "%s", "body": "Test S3 message", "file": null }
	}`, room.Id)

	resp := env.doAssetMultipartUpload(t, operations, imageData, "s3-auth-test.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", resp.Errors)
	}

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
		t.Fatal("Expected one attachment in S3-backed upload")
	}
	attachment := data.PostMessage.Event.Attachments[0]
	loc := signedurl.AttachmentLocator{RoomID: room.Id, AttachmentID: attachment.ID}
	signedURL := env.core.GetAttachmentURL(loc, user.Id)
	signedThumbnailURL := env.core.GetTransformedAttachmentURL(loc, user.Id, 200, 200, "contain")

	// Anonymous client — the signed URL alone should be enough to fetch.
	unauthClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	originalResp, err := unauthClient.Get(env.server.URL + signedURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	originalResp.Body.Close()
	// Either a direct 200 (NATS) or a 302 redirect to presigned S3 — both prove
	// the signed locator was accepted.
	if originalResp.StatusCode != http.StatusOK && originalResp.StatusCode != http.StatusFound {
		t.Errorf("S3 attachment URL: expected 200 or 302 with signed URL, got %d", originalResp.StatusCode)
	}

	transformResp, err := unauthClient.Get(env.server.URL + signedThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	transformResp.Body.Close()
	if transformResp.StatusCode != http.StatusOK {
		t.Errorf("S3 transform URL: expected 200 with signed URL, got %d", transformResp.StatusCode)
	}
}

// TestAsset_RevokedMembership_RevokesSignedURL covers the "kick / leave"
// path under the per-user signed URL model. The URL is signed for user
// X; once X is removed from the room (or leaves), the handler's
// membership re-check fails and the URL stops working — even though
// the signature itself is still valid.
func TestAsset_RevokedMembership_RevokesSignedURL(t *testing.T) {
	env := setupAssetTestServerWithS3(t)

	owner, err := env.core.CreateUser(env.ctx, "system", "asset-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.Id, "channel", "", "private-room", "Private Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, owner.Id, "channel", owner.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	env.login(t, "asset-owner", "password123")
	imageData := createAssetTestPNG(t, 400, 300)
	operations := fmt.Sprintf(`{
		"query": "mutation($roomId: ID!, $body: String!, $file: Upload!) { postMessage(input: { roomId: $roomId, body: $body, attachments: [$file] }) { event { ... on MessagePostedEvent { attachments { id url } } } } }",
		"variables": { "roomId": "%s", "body": "private", "file": null }
	}`, room.Id)
	resp := env.doAssetMultipartUpload(t, operations, imageData, "private.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to post: %v", resp.Errors)
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
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.PostMessage.Event.Attachments) == 0 {
		t.Fatal("Expected one attachment")
	}
	attachment := data.PostMessage.Event.Attachments[0]
	attachmentURL := env.core.GetAttachmentURL(signedurl.AttachmentLocator{
		RoomID:       room.Id,
		AttachmentID: attachment.ID,
	}, owner.Id)

	// Sanity check: owner can fetch their own URL (cookie not required —
	// the signed URL is the capability).
	plainClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	r, err := plainClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("pre-leave GET: %v", err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusFound {
		t.Fatalf("expected signed URL to work pre-leave, got %d", r.StatusCode)
	}

	// Owner leaves the room → their signed URL should stop working.
	if err := env.core.LeaveRoom(env.ctx, owner.Id, "channel", owner.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	r2, err := plainClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("post-leave GET: %v", err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 after signed user left the room, got %d", r2.StatusCode)
	}
}
