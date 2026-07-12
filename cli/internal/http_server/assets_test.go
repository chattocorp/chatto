package http_server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/internal/testutil/fakes3"
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

func setupAssetTestServerWithS3AndVideo(t *testing.T) *assetTestEnv {
	return setupAssetTestServerWithOptions(t, true, true)
}

func setupAssetTestServerWithConfig(t *testing.T, useS3 bool) *assetTestEnv {
	return setupAssetTestServerWithOptions(t, useS3, false)
}

func setupAssetTestServerWithOptions(t *testing.T, useS3 bool, videoEnabled bool) *assetTestEnv {
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
		s3Server := fakes3.NewServer(t)

		useSSL := false
		pathStyle := true
		assetsCfg.StorageBackend = config.StorageBackendS3
		assetsCfg.S3 = config.S3Config{
			Endpoint:        s3Server.EndpointHost(),
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
			Video: config.VideoConfig{
				Enabled: videoEnabled,
			},
		},
		nc:     nc,
		router: router,
		core:   chattoCore,
		mailer: email.NewMockSender(true),
		logger: log.WithPrefix("test"),
	}

	s.setupAuthRoutes()
	s.setupConnectAPI()
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

func (env *assetTestEnv) postAssetMessageWithAttachment(t *testing.T, roomID, body string, fileData []byte, fileName string) (string, *apiv1.MessageAttachment) {
	t.Helper()
	return env.postAssetMessageWithAttachmentContentType(t, roomID, body, fileData, fileName, "image/png")
}

func (env *assetTestEnv) postAssetMessageWithAttachmentContentType(t *testing.T, roomID, body string, fileData []byte, fileName, contentType string) (string, *apiv1.MessageAttachment) {
	t.Helper()

	assetUploadClient := apiv1connect.NewAssetUploadServiceClient(env.client, env.server.URL+connectAPIPrefix)
	sum := sha256.Sum256(fileData)
	created, err := assetUploadClient.CreateUpload(env.ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      roomID,
		Filename:    fileName,
		ContentType: contentType,
		Size:        int64(len(fileData)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("Failed to create asset upload: %v", err)
	}
	chunkSum := sha256.Sum256(fileData)
	if _, err := assetUploadClient.UploadChunk(env.ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    created.Msg.GetUpload().GetUploadId(),
		Content:     fileData,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); err != nil {
		t.Fatalf("Failed to upload asset chunk: %v", err)
	}
	completed, err := assetUploadClient.CompleteUpload(env.ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: created.Msg.GetUpload().GetUploadId(),
	}))
	if err != nil {
		t.Fatalf("Failed to complete asset upload: %v", err)
	}
	assetID := completed.Msg.GetAsset().GetId()
	if assetID == "" {
		t.Fatal("Completed asset upload returned empty asset id")
	}

	client := apiv1connect.NewMessageServiceClient(env.client, env.server.URL+connectAPIPrefix)
	req := connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             roomID,
		Body:               body,
		AttachmentAssetIds: []string{assetID},
	})
	resp, err := client.CreateMessage(env.ctx, req)
	if err != nil {
		t.Fatalf("Failed to post message with attachment: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatal("Expected posted message")
	}
	if len(message.GetAttachments()) == 0 {
		t.Fatal("Expected at least one attachment")
	}
	return message.GetId(), message.GetAttachments()[0]
}

func (env *assetTestEnv) deleteAssetMessage(t *testing.T, roomID, eventID string) {
	t.Helper()

	client := apiv1connect.NewMessageServiceClient(env.client, env.server.URL+connectAPIPrefix)
	req := connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  roomID,
		EventId: eventID,
	})
	if _, err := client.DeleteMessage(env.ctx, req); err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test message with image", imageData, "test-image.png")
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if thumbnailURL == "" {
		t.Fatal("Expected thumbnail asset URL")
	}

	// First request to transformed URL should be a cache MISS
	transformResp, err := env.client.Get(env.server.URL + thumbnailURL)
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
	transformResp2, err := env.client.Get(env.server.URL + thumbnailURL)
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

func TestAsset_TransformedAttachmentUsesCompressedProfileAndVersionedCache(t *testing.T) {
	env := setupAssetTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "compressedimageuser", "Compressed Image User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "compressed-images", "Compressed Images")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "compressedimageuser", "password123")

	imageData := createAssetTestPNG(t, 1200, 800)
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "compressed image", imageData, "compressed.png")
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if !strings.Contains(thumbnailURL, "/960x400/contain") {
		t.Fatalf("thumbnail URL = %q, want 960x400 contain transform", thumbnailURL)
	}
	oldCacheKey := core.ImageCacheKey("attachment-stable", attachment.GetId(), 960, 400, "contain")
	if err := env.core.StoreCachedResize(env.ctx, oldCacheKey, []byte("old-quality-cache-entry")); err != nil {
		t.Fatalf("Failed to seed old attachment cache namespace: %v", err)
	}

	resp, err := env.client.Get(env.server.URL + thumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed attachment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Cache"); got != "MISS" {
		t.Fatalf("X-Cache = %q, want MISS for old cache namespace", got)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read transformed attachment: %v", err)
	}

	wantResult, err := assets.TransformImageWithOptions(imageData, 960, 400, assets.FitContain, assets.TransformOptions{
		JPEGQuality: AttachmentDerivativeJPEGQuality,
	})
	if err != nil {
		t.Fatalf("Failed to build expected transform: %v", err)
	}
	want, err := io.ReadAll(wantResult.Reader)
	if err != nil {
		t.Fatalf("Failed to read expected transform: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("attachment derivative did not use the compressed attachment profile")
	}

	cacheKey := core.ImageCacheKey(AttachmentStableCachePrefix, attachment.GetId(), 960, 400, "contain")
	if !strings.HasPrefix(cacheKey, "attachment-stable-v2.") {
		t.Fatalf("cache key = %q, want versioned attachment-stable-v2 prefix", cacheKey)
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
	eventID, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test message for cleanup", imageData, "cleanup-test.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if attachmentURL == "" || thumbnailURL == "" {
		t.Fatal("Expected original and thumbnail asset URLs")
	}

	// Request transformed image to populate cache
	transformResp, err := env.client.Get(env.server.URL + thumbnailURL)
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
	transformResp2, err := env.client.Get(env.server.URL + thumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get transformed image: %v", err)
	}
	transformResp2.Body.Close()
	if transformResp2.Header.Get("X-Cache") != "HIT" {
		t.Fatalf("Expected cache HIT before deletion")
	}

	// Delete the message (which should delete the attachment and its cache)
	env.deleteAssetMessage(t, room.Id, eventID)

	// Original attachment URL should now return 404
	originalResp, err := env.client.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to get original attachment: %v", err)
	}
	originalResp.Body.Close()
	if originalResp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for deleted attachment, got %d", originalResp.StatusCode)
	}

	// Transformed URL should also return 404 (not cache hit from stale cache)
	transformResp3, err := env.client.Get(env.server.URL + thumbnailURL)
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test message", imageData, "serve-test.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected original asset URL")
	}

	// Get original attachment
	originalResp, err := env.client.Get(env.server.URL + attachmentURL)
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

	_, attachment := env.postAssetMessageWithAttachmentContentType(
		t,
		room.Id,
		"html attachment",
		[]byte("<!doctype html><script>window.__ran = true</script>"),
		"demo.html",
		"text/html; charset=utf-8",
	)
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected stable attachment URL")
	}

	stableResp, err := env.client.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to fetch stable attachment URL: %v", err)
	}
	stableResp.Body.Close()
	if stableResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected stable attachment status 200, got %d", stableResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, stableResp)
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

	_, attachment := env.postAssetMessageWithAttachmentContentType(
		t,
		room.Id,
		"s3 html attachment",
		[]byte("<!doctype html><script>window.__ran = true</script>"),
		"s3-demo.html",
		"text/html",
	)
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected stable attachment URL")
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	stableResp, err := noRedirectClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to fetch S3 stable attachment URL: %v", err)
	}
	stableResp.Body.Close()
	if stableResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected S3 stable attachment to stream with 200, got %d", stableResp.StatusCode)
	}
	assertSandboxedOriginalAttachment(t, stableResp)
}

func TestAsset_StableS3ImageStreamsThroughChattoByDefault(t *testing.T) {
	env := setupAssetTestServerWithS3(t)

	user, err := env.core.CreateUser(env.ctx, "system", "s3imageuser", "S3 Image User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "s3imageroom", "S3 Image Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "s3imageuser", "password123")

	imageData := createAssetTestPNG(t, 64, 48)
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "s3 image", imageData, "s3-image.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected stable attachment URL")
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirectClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to fetch S3 image attachment URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected S3 image to stream through Chatto with 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "" {
		t.Fatalf("Expected no redirect Location for ordinary S3 image, got %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != protectedAssetCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, protectedAssetCacheControl)
	}
}

func TestAsset_StableS3VideoRedirectsUnlessProxyForcesStream(t *testing.T) {
	env := setupAssetTestServerWithS3AndVideo(t)
	env.core.OnVideoProcessingRequested = func(context.Context, string, string) error { return nil }

	user, err := env.core.CreateUser(env.ctx, "system", "s3videouser", "S3 Video User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "s3videoroom", "S3 Video Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "s3videouser", "password123")

	_, attachment := env.postAssetMessageWithAttachmentContentType(
		t,
		room.Id,
		"s3 video",
		[]byte("fake-video-bytes"),
		"s3-video.mp4",
		"video/mp4",
	)
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected stable attachment URL")
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	redirectResp, err := noRedirectClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to fetch S3 video attachment URL: %v", err)
	}
	redirectResp.Body.Close()
	if redirectResp.StatusCode != http.StatusFound {
		t.Fatalf("Expected S3 video to redirect with 302, got %d", redirectResp.StatusCode)
	}
	if got := redirectResp.Header.Get("Cache-Control"); got != protectedAssetCacheControl {
		t.Fatalf("Redirect Cache-Control = %q, want %q", got, protectedAssetCacheControl)
	}
	if got := redirectResp.Header.Get("Location"); got == "" || !strings.Contains(got, "X-Amz-Expires=300") {
		t.Fatalf("Expected short-lived presigned S3 Location, got %q", got)
	}

}

func TestAsset_StableNilStorageS3VideoRedirectsViaProbe(t *testing.T) {
	env := setupAssetTestServerWithS3AndVideo(t)
	env.core.OnVideoProcessingRequested = func(context.Context, string, string) error { return nil }

	user, err := env.core.CreateUser(env.ctx, "system", "s3legacyvideouser", "S3 Legacy Video User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "s3legacyvideoroom", "S3 Legacy Video Room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "s3legacyvideouser", "password123")

	videoBytes := []byte("fake legacy video bytes")
	_, attachment := env.postAssetMessageWithAttachmentContentType(
		t,
		room.Id,
		"s3 legacy video",
		videoBytes,
		"s3-legacy-video.mp4",
		"video/mp4",
	)
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected stable attachment URL")
	}

	if err := env.core.Assets.Apply(&corev1.Event{
		Id: "E-storage-less-" + attachment.GetId(),
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				RoomId:                  room.Id,
				Asset: &corev1.AssetRecord{
					Id:          attachment.GetId(),
					Filename:    "s3-legacy-video.mp4",
					ContentType: "video/mp4",
					Size:        int64(len(videoBytes)),
				},
			},
		},
	}, 999); err != nil {
		t.Fatalf("Failed to project storage-less asset metadata: %v", err)
	}

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	redirectResp, err := noRedirectClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to fetch storage-less S3 video attachment URL: %v", err)
	}
	redirectResp.Body.Close()
	if redirectResp.StatusCode != http.StatusFound {
		t.Fatalf("Expected storage-less S3 video to redirect with 302, got %d", redirectResp.StatusCode)
	}
	if got := redirectResp.Header.Get("Location"); got == "" || !strings.Contains(got, "X-Amz-Expires=300") {
		t.Fatalf("Expected probed short-lived presigned S3 Location, got %q", got)
	}
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test message", imageData, "cache-header-test.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	if attachmentURL == "" {
		t.Fatal("Expected original asset URL")
	}

	// Get original attachment
	originalResp, err := env.client.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to get original attachment: %v", err)
	}
	defer originalResp.Body.Close()

	if originalResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", originalResp.StatusCode)
	}

	// Verify caching headers
	cacheControl := originalResp.Header.Get("Cache-Control")
	if cacheControl != protectedAssetCacheControl {
		t.Errorf("Expected Cache-Control: %s, got: %s", protectedAssetCacheControl, cacheControl)
	}

	etag := originalResp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header to be set")
	}

	vary := originalResp.Header.Get("Vary")
	if vary != "Accept-Encoding, Authorization, Cookie" {
		t.Errorf("Expected Vary: Accept-Encoding, Authorization, Cookie, got: %s", vary)
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "bearer asset", imageData, "bearer.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if attachmentURL == "" || thumbnailURL == "" {
		t.Fatal("Expected original and thumbnail asset URLs")
	}

	unauthClient := &http.Client{}

	withoutAccess, err := url.Parse(attachmentURL)
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

	ticketResp, err := unauthClient.Get(env.server.URL + attachmentURL)
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

	thumbResp, err := unauthClient.Get(env.server.URL + thumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get stable thumbnail URL with access ticket: %v", err)
	}
	thumbResp.Body.Close()
	if thumbResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected stable thumbnail request with access ticket to return 200, got %d", thumbResp.StatusCode)
	}

	mutatedThumbnailURL := strings.Replace(thumbnailURL, "960x400", "961x400", 1)
	if mutatedThumbnailURL == thumbnailURL {
		t.Fatalf("Expected thumbnail URL to contain transform dimensions, got %q", thumbnailURL)
	}
	mutatedResp, err := unauthClient.Get(env.server.URL + mutatedThumbnailURL)
	if err != nil {
		t.Fatalf("Failed to get mutated stable thumbnail URL: %v", err)
	}
	mutatedResp.Body.Close()
	if mutatedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected mutated stable thumbnail request to return 403, got %d", mutatedResp.StatusCode)
	}

	thumbnailWithoutAccess, err := url.Parse(thumbnailURL)
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

func TestAsset_ServerAssetTransformKeepsDefaultQuality(t *testing.T) {
	env := setupAssetTestServer(t)

	imageData := createAssetTestPNG(t, 400, 300)
	assetPath := "branding/default-quality.png"
	if _, err := env.core.ServerStore().PutBytes(env.ctx, assetPath, imageData); err != nil {
		t.Fatalf("Failed to store server asset: %v", err)
	}

	transformURL := env.core.GetTransformedServerAssetURL(assetPath, 200, 200, "contain")
	resp, err := env.client.Get(env.server.URL + transformURL)
	if err != nil {
		t.Fatalf("Failed to get transformed server asset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read transformed server asset: %v", err)
	}

	wantResult, err := assets.TransformImage(imageData, 200, 200, assets.FitContain)
	if err != nil {
		t.Fatalf("Failed to build expected server transform: %v", err)
	}
	want, err := io.ReadAll(wantResult.Reader)
	if err != nil {
		t.Fatalf("Failed to read expected server transform: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("server asset transform did not retain the default image quality")
	}
}

func TestAsset_LegacyAttachmentRouteIsGone(t *testing.T) {
	env := setupAssetTestServer(t)

	resp, err := env.client.Get(env.server.URL + "/assets/attachments/not-a-locator")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected removed legacy attachment route to return 404, got %d", resp.StatusCode)
	}
}

func TestAsset_StableURLIsCapability(t *testing.T) {
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test message", imageData, "auth-test.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if attachmentURL == "" || thumbnailURL == "" {
		t.Fatal("Expected original and thumbnail stable asset URLs")
	}

	// A no-cookie / no-header client holding the access-ticket URL should be
	// able to fetch the binary.
	unauthClient := &http.Client{}

	originalResp, err := unauthClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	originalResp.Body.Close()
	if originalResp.StatusCode != http.StatusOK {
		t.Errorf("Stable URL should authorize itself; got status %d", originalResp.StatusCode)
	}

	transformResp, err := unauthClient.Get(env.server.URL + thumbnailURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	transformResp.Body.Close()
	if transformResp.StatusCode != http.StatusOK {
		t.Errorf("Stable transform URL should authorize itself; got status %d", transformResp.StatusCode)
	}

	// A tampered access ticket must fail.
	tampered := strings.TrimSuffix(attachmentURL, "X") + "z"
	tamperedResp, err := unauthClient.Get(env.server.URL + tampered)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	tamperedResp.Body.Close()
	if tamperedResp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 for tampered access ticket, got %d", tamperedResp.StatusCode)
	}
}

func TestAsset_StableURLOnS3IsCapability(t *testing.T) {
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "Test S3 message", imageData, "s3-auth-test.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()
	thumbnailURL := attachment.GetThumbnailAssetUrl().GetUrl()
	if attachmentURL == "" || thumbnailURL == "" {
		t.Fatal("Expected original and thumbnail stable asset URLs")
	}

	// Anonymous client — the access-ticket URL alone should be enough to fetch.
	unauthClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	originalResp, err := unauthClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	originalResp.Body.Close()
	if originalResp.StatusCode != http.StatusOK {
		t.Errorf("S3 image stable URL: expected 200 with access ticket, got %d", originalResp.StatusCode)
	}

	transformResp, err := unauthClient.Get(env.server.URL + thumbnailURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	transformResp.Body.Close()
	if transformResp.StatusCode != http.StatusOK {
		t.Errorf("S3 transform URL: expected 200 with access ticket, got %d", transformResp.StatusCode)
	}
}

// TestAsset_RevokedMembership_RevokesStableURL covers the "kick / leave"
// path under the per-user access-ticket model.
func TestAsset_RevokedMembership_RevokesStableURL(t *testing.T) {
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
	_, attachment := env.postAssetMessageWithAttachment(t, room.Id, "private", imageData, "private.png")
	attachmentURL := attachment.GetAssetUrl().GetUrl()

	// Sanity check: owner can fetch their own URL without a cookie because the
	// access ticket is the capability.
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
	if r.StatusCode != http.StatusOK {
		t.Fatalf("expected stable URL to work pre-leave, got %d", r.StatusCode)
	}

	// Owner leaves the room, so their stable access-ticket URL should stop working.
	if err := env.core.LeaveRoom(env.ctx, owner.Id, "channel", owner.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	r2, err := plainClient.Get(env.server.URL + attachmentURL)
	if err != nil {
		t.Fatalf("post-leave GET: %v", err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 after ticket user left the room, got %d", r2.StatusCode)
	}
}
