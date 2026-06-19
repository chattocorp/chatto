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
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// Upload Test Helpers
// ============================================================================

// uploadTestEnv holds all test dependencies for upload tests
type uploadTestEnv struct {
	server *httptest.Server
	client *http.Client
	core   *core.ChattoCore
	ctx    context.Context
}

// setupUploadTestServer creates a test server for upload testing.
func setupUploadTestServer(t *testing.T) *uploadTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Create ChattoCore with assets config
	coreConfig := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret-32-bytes-!!",
			MaxUploadSize: 10 * 1024 * 1024, // 10MB
		},
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
	s.setupAttachmentUploadRoutes()

	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &uploadTestEnv{
		server: ts,
		client: client,
		core:   chattoCore,
		ctx:    ctx,
	}
}

// login authenticates a user
func (env *uploadTestEnv) login(t *testing.T, login, password string) {
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

// createTestPNG creates a simple PNG image for testing
func createTestPNG(t *testing.T, width, height int) []byte {
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

// doMultipartUpload performs a GraphQL multipart upload request
func (env *uploadTestEnv) doMultipartUpload(t *testing.T, operations string, fileData []byte, fileName string) *graphqlResponse {
	t.Helper()

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add operations field
	if err := writer.WriteField("operations", operations); err != nil {
		t.Fatalf("Failed to write operations: %v", err)
	}

	// Add map field (maps file to variable)
	if err := writer.WriteField("map", `{"0": ["variables.input.file"]}`); err != nil {
		t.Fatalf("Failed to write map: %v", err)
	}

	// Add file
	part, err := writer.CreateFormFile("0", fileName)
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

func (env *uploadTestEnv) doRoomAttachmentUpload(
	t *testing.T,
	roomID string,
	fileData []byte,
	fileName string,
	contentType string,
	fields map[string]string,
) (*http.Response, *apiv1.UploadRoomAttachmentsResponse) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("Failed to write form field %s: %v", name, err)
		}
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, roomAttachmentUploadFieldName, fileName))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("Failed to create attachment part: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(fileData)); err != nil {
		t.Fatalf("Failed to copy file data: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/api/rooms/"+roomID+"/attachments", &body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read upload response: %v", err)
	}
	var decoded apiv1.UploadRoomAttachmentsResponse
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to decode upload response: %v", err)
	}
	return resp, &decoded
}

func (env *uploadTestEnv) doAttachmentURLRefresh(
	t *testing.T,
	roomID string,
	req *apiv1.RefreshMessageAttachmentUrlsRequest,
) (*http.Response, *apiv1.RefreshMessageAttachmentUrlsResponse) {
	t.Helper()

	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal refresh request: %v", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, env.server.URL+"/api/rooms/"+roomID+"/attachments/urls/refresh", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to create refresh request: %v", err)
	}
	httpReq.Header.Set("Content-Type", protobufContentType)
	httpReq.Header.Set("Accept", protobufContentType)

	resp, err := env.client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to send refresh request: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read refresh response: %v", err)
	}
	var decoded apiv1.RefreshMessageAttachmentUrlsResponse
	if err := proto.Unmarshal(responseBody, &decoded); err != nil {
		t.Fatalf("Failed to decode refresh response: %v", err)
	}
	return resp, &decoded
}

func (env *uploadTestEnv) doProfileAssetUpload(
	t *testing.T,
	path string,
	fieldName string,
	fileData []byte,
	fileName string,
	contentType string,
) (*http.Response, []byte) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("Failed to create asset part: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(fileData)); err != nil {
		t.Fatalf("Failed to copy file data: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, env.server.URL+path, &body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", protobufContentType)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read upload response: %v", err)
	}
	return resp, responseBody
}

func (env *uploadTestEnv) doProfileAssetDelete(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, env.server.URL+path, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", protobufContentType)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read delete response: %v", err)
	}
	return resp, responseBody
}

// ============================================================================
// Upload Tests
// ============================================================================

func TestUpload_RoomAttachmentEndpoint_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "roomattach", "Room Attach", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "room-attachment-upload", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	env.login(t, "roomattach", "password123")

	resp, decoded := env.doRoomAttachmentUpload(t, room.Id, []byte("hello attachment"), "note.txt", "text/plain", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != protobufContentType {
		t.Fatalf("Content-Type = %q, want %q", contentType, protobufContentType)
	}
	if decoded == nil || len(decoded.GetAttachments()) != 1 {
		t.Fatalf("attachments = %v, want one attachment", decoded.GetAttachments())
	}
	uploaded := decoded.GetAttachments()[0]
	if uploaded.GetNeedsVideoProcessing() {
		t.Fatal("plain text attachment should not need video processing")
	}
	if len(decoded.GetVideoProcessingAssetIds()) != 0 {
		t.Fatalf("video processing IDs = %v, want none", decoded.GetVideoProcessingAssetIds())
	}
	attachment := uploaded.GetAttachment()
	if attachment.GetId() == "" {
		t.Fatal("attachment id is empty")
	}
	if attachment.GetFilename() != "note.txt" {
		t.Fatalf("filename = %q, want note.txt", attachment.GetFilename())
	}
	if attachment.GetContentType() != "text/plain" {
		t.Fatalf("content type = %q, want text/plain", attachment.GetContentType())
	}

	reader, _, err := env.core.GetAttachmentReader(env.ctx, attachment)
	if err != nil {
		t.Fatalf("GetAttachmentReader: %v", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read attachment: %v", err)
	}
	if string(data) != "hello attachment" {
		t.Fatalf("stored attachment = %q, want %q", string(data), "hello attachment")
	}
}

func TestUpload_RoomAttachmentEndpoint_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "roomattachunauth", "Room Attach", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "room-attachment-upload-unauth", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	resp, _ := env.doRoomAttachmentUpload(t, room.Id, []byte("hello attachment"), "note.txt", "text/plain", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 401: %s", resp.StatusCode, body)
	}
}

func TestUpload_AttachmentURLRefreshEndpoint_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "urlrefresh", "URL Refresh", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "attachment-url-refresh", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	attachment, err := env.core.UploadAttachment(env.ctx, user.Id, room.Id, "test.png", "image/png", bytes.NewReader(createTestPNG(t, 64, 64)))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, user.Id, "with attachment", []string{attachment.GetId()}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	env.login(t, "urlrefresh", "password123")

	resp, decoded := env.doAttachmentURLRefresh(t, room.Id, &apiv1.RefreshMessageAttachmentUrlsRequest{
		RoomId:          room.Id,
		EventId:         event.GetId(),
		ThumbnailWidth:  120,
		ThumbnailHeight: 120,
		ThumbnailFit:    apiv1.AssetFitMode_ASSET_FIT_MODE_COVER,
	})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != protobufContentType {
		t.Fatalf("Content-Type = %q, want %q", contentType, protobufContentType)
	}
	views := decoded.GetAttachments()
	if len(views) != 1 {
		t.Fatalf("attachments = %v, want one attachment", views)
	}
	view := views[0]
	if view.GetId() != attachment.GetId() {
		t.Fatalf("attachment id = %q, want %q", view.GetId(), attachment.GetId())
	}
	if view.GetAssetUrl().GetUrl() == "" {
		t.Fatal("asset URL is empty")
	}
	if thumbURL := view.GetThumbnailAssetUrl().GetUrl(); !strings.Contains(thumbURL, "/image/120x120/cover") {
		t.Fatalf("thumbnail URL = %q, want requested transform", thumbURL)
	}
	if view.GetAssetUrl().GetExpiresAt() == nil || view.GetThumbnailAssetUrl().GetExpiresAt() == nil {
		t.Fatal("expected expiring asset URLs")
	}
}

func TestUpload_AttachmentURLRefreshEndpoint_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "urlrefreshunauth", "URL Refresh", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.Id, core.KindChannel, "", "attachment-url-refresh-unauth", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	resp, _ := env.doAttachmentURLRefresh(t, room.Id, &apiv1.RefreshMessageAttachmentUrlsRequest{
		EventId: "event_missing",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 401: %s", resp.StatusCode, body)
	}
}

func TestUpload_UserAvatarEndpoint_SuccessAndDelete(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "avatarhttp", "Avatar HTTP", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "avatarhttp", "password123")

	path := "/api/users/" + url.PathEscape(user.Id) + "/avatar"
	resp, body := env.doProfileAssetUpload(t, path, userAvatarUploadFieldName, createTestPNG(t, 256, 256), "avatar.png", "image/png")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != protobufContentType {
		t.Fatalf("Content-Type = %q, want %q", contentType, protobufContentType)
	}
	var upload apiv1.UserAvatarAssetResponse
	if err := proto.Unmarshal(body, &upload); err != nil {
		t.Fatalf("Failed to decode avatar upload response: %v", err)
	}
	if upload.GetUser().GetId() != user.Id {
		t.Fatalf("response user id = %q, want %q", upload.GetUser().GetId(), user.Id)
	}
	if upload.GetAvatarUrl() == "" {
		t.Fatal("avatar URL is empty after upload")
	}

	avatarURL, err := env.core.GetUserAvatarURL(env.ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("GetUserAvatarURL: %v", err)
	}
	if avatarURL == "" {
		t.Fatal("core avatar URL is empty after upload")
	}

	resp, body = env.doProfileAssetDelete(t, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200: %s", resp.StatusCode, body)
	}
	var deleted apiv1.UserAvatarAssetResponse
	if err := proto.Unmarshal(body, &deleted); err != nil {
		t.Fatalf("Failed to decode avatar delete response: %v", err)
	}
	if deleted.GetAvatarUrl() != "" {
		t.Fatalf("avatar URL after delete = %q, want empty", deleted.GetAvatarUrl())
	}
	asset, err := env.core.GetUserAvatar(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserAvatar: %v", err)
	}
	if asset != nil {
		t.Fatal("expected avatar asset to be cleared")
	}
}

func TestUpload_UserAvatarEndpoint_RejectsOtherUserWithoutRoleAssign(t *testing.T) {
	env := setupUploadTestServer(t)

	actor, err := env.core.CreateUser(env.ctx, "system", "avataractor", "Avatar Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, "system", "avatartarget", "Avatar Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	env.login(t, actor.Login, "password123")

	path := "/api/users/" + url.PathEscape(target.Id) + "/avatar"
	resp, body := env.doProfileAssetUpload(t, path, userAvatarUploadFieldName, createTestPNG(t, 128, 128), "avatar.png", "image/png")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.StatusCode, body)
	}
}

func TestUpload_ServerBrandingEndpoints_SuccessAndDelete(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "brandinghttp", "Branding HTTP", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	env.login(t, "brandinghttp", "password123")

	resp, body := env.doProfileAssetUpload(t, "/api/server/logo", serverLogoUploadFieldName, createTestPNG(t, 256, 256), "logo.png", "image/png")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logo status = %d, want 200: %s", resp.StatusCode, body)
	}
	var logo apiv1.ServerBrandingAssetResponse
	if err := proto.Unmarshal(body, &logo); err != nil {
		t.Fatalf("Failed to decode logo upload response: %v", err)
	}
	if logo.GetProfile().GetLogoUrl() == "" {
		t.Fatal("logo URL is empty after upload")
	}

	resp, body = env.doProfileAssetDelete(t, "/api/server/logo")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logo delete status = %d, want 200: %s", resp.StatusCode, body)
	}
	var logoDeleted apiv1.ServerBrandingAssetResponse
	if err := proto.Unmarshal(body, &logoDeleted); err != nil {
		t.Fatalf("Failed to decode logo delete response: %v", err)
	}
	if logoDeleted.GetProfile().GetLogoUrl() != "" {
		t.Fatalf("logo URL after delete = %q, want empty", logoDeleted.GetProfile().GetLogoUrl())
	}

	resp, body = env.doProfileAssetUpload(t, "/api/server/banner", serverBannerUploadFieldName, createTestPNG(t, 1200, 630), "banner.png", "image/png")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("banner status = %d, want 200: %s", resp.StatusCode, body)
	}
	var banner apiv1.ServerBrandingAssetResponse
	if err := proto.Unmarshal(body, &banner); err != nil {
		t.Fatalf("Failed to decode banner upload response: %v", err)
	}
	if banner.GetProfile().GetBannerUrl() == "" {
		t.Fatal("banner URL is empty after upload")
	}

	resp, body = env.doProfileAssetDelete(t, "/api/server/banner")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("banner delete status = %d, want 200: %s", resp.StatusCode, body)
	}
	var bannerDeleted apiv1.ServerBrandingAssetResponse
	if err := proto.Unmarshal(body, &bannerDeleted); err != nil {
		t.Fatalf("Failed to decode banner delete response: %v", err)
	}
	if bannerDeleted.GetProfile().GetBannerUrl() != "" {
		t.Fatalf("banner URL after delete = %q, want empty", bannerDeleted.GetProfile().GetBannerUrl())
	}
}

func TestUpload_ServerBrandingEndpoint_RejectsNonManager(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "brandingregular", "Branding Regular", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, user.Login, "password123")

	resp, body := env.doProfileAssetUpload(t, "/api/server/logo", serverLogoUploadFieldName, createTestPNG(t, 256, 256), "logo.png", "image/png")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.StatusCode, body)
	}
}

func TestUpload_ServerLogo_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "uploader", "Uploader", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	env.login(t, "uploader", "password123")

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadServerLogoInput!) { uploadServerLogo(input: $input) { profile { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadServerLogo struct {
			Profile struct {
				LogoURL *string `json:"logoUrl"`
			} `json:"profile"`
		} `json:"uploadServerLogo"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.UploadServerLogo.Profile.LogoURL == nil || *data.UploadServerLogo.Profile.LogoURL == "" {
		t.Error("Expected logoUrl to be set after upload")
	}
}

func TestUpload_ServerLogo_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	_, _ = env.core.CreateUser(env.ctx, "system", "upload-owner-logo-unauth", "Owner", "password123")

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadServerLogoInput!) { uploadServerLogo(input: $input) { profile { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected authentication error")
	}
}

func TestUpload_ServerLogo_NotAdmin(t *testing.T) {
	env := setupUploadTestServer(t)

	_, _ = env.core.CreateUser(env.ctx, "system", "upload-owner-logo-notadmin", "Owner", "password123")

	env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")
	env.login(t, "other", "password123")

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadServerLogoInput!) { uploadServerLogo(input: $input) { profile { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected permission denied error")
	}

	foundPermError := false
	for _, e := range resp.Errors {
		if e.Message == "permission denied" {
			foundPermError = true
		}
	}
	if !foundPermError {
		t.Errorf("Expected 'permission denied' error, got: %v", resp.Errors)
	}
}

func TestUpload_ServerLogo_DeleteLogo(t *testing.T) {
	env := setupUploadTestServer(t)

	user, _ := env.core.CreateUser(env.ctx, "system", "deleter", "Deleter", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	env.login(t, "deleter", "password123")

	imageData := createTestPNG(t, 256, 256)
	operations := `{
		"query": "mutation($input: UploadServerLogoInput!) { uploadServerLogo(input: $input) { profile { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to upload logo: %v", resp.Errors)
	}

	deleteResp := env.doGraphQL(t, `mutation { deleteServerLogo { profile { logoUrl } } }`, nil)

	if len(deleteResp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", deleteResp.Errors)
	}

	var data struct {
		DeleteServerLogo struct {
			Profile struct {
				LogoURL *string `json:"logoUrl"`
			} `json:"profile"`
		} `json:"deleteServerLogo"`
	}
	if err := json.Unmarshal(deleteResp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.DeleteServerLogo.Profile.LogoURL != nil {
		t.Error("Expected logoUrl to be null after deletion")
	}
}

// doGraphQL helper for non-upload GraphQL requests
func (env *uploadTestEnv) doGraphQL(t *testing.T, query string, variables map[string]any) *graphqlResponse {
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

func TestUpload_LargeImage_IsProcessed(t *testing.T) {
	env := setupUploadTestServer(t)

	user, _ := env.core.CreateUser(env.ctx, "system", "largeuser", "Large User", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	env.login(t, "largeuser", "password123")

	imageData := createTestPNG(t, 1024, 1024)

	operations := `{
		"query": "mutation($input: UploadServerLogoInput!) { uploadServerLogo(input: $input) { profile { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "large-logo.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadServerLogo struct {
			Profile struct {
				LogoURL *string `json:"logoUrl"`
			} `json:"profile"`
		} `json:"uploadServerLogo"`
	}
	json.Unmarshal(resp.Data, &data)

	// Logo should be uploaded successfully (server resizes to 512x512 max)
	if data.UploadServerLogo.Profile.LogoURL == nil {
		t.Error("Expected logoUrl to be set")
	}
}

// ============================================================================
// Space Banner Upload Tests
// ============================================================================

func TestUpload_ServerBanner_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "banneruser", "Banner User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	env.login(t, "banneruser", "password123")

	imageData := createTestPNG(t, 1200, 400)

	operations := `{
		"query": "mutation($input: UploadServerBannerInput!) { uploadServerBanner(input: $input) { profile { bannerUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadServerBanner struct {
			Profile struct {
				BannerURL *string `json:"bannerUrl"`
			} `json:"profile"`
		} `json:"uploadServerBanner"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.UploadServerBanner.Profile.BannerURL == nil || *data.UploadServerBanner.Profile.BannerURL == "" {
		t.Error("Expected bannerUrl to be set after upload")
	}
}

func TestUpload_ServerBanner_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	_, _ = env.core.CreateUser(env.ctx, "system", "upload-owner-banner-unauth", "Owner", "password123")

	imageData := createTestPNG(t, 1200, 400)

	operations := `{
		"query": "mutation($input: UploadServerBannerInput!) { uploadServerBanner(input: $input) { profile { bannerUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected authentication error")
	}
}

func TestUpload_ServerBanner_NotAdmin(t *testing.T) {
	env := setupUploadTestServer(t)

	_, _ = env.core.CreateUser(env.ctx, "system", "upload-owner-banner-notadmin", "Owner", "password123")

	env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")
	env.login(t, "other", "password123")

	imageData := createTestPNG(t, 1200, 400)

	operations := `{
		"query": "mutation($input: UploadServerBannerInput!) { uploadServerBanner(input: $input) { profile { bannerUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected permission denied error")
	}

	foundPermError := false
	for _, e := range resp.Errors {
		if e.Message == "permission denied" {
			foundPermError = true
		}
	}
	if !foundPermError {
		t.Errorf("Expected 'permission denied' error, got: %v", resp.Errors)
	}
}

func TestUpload_ServerBanner_DeleteBanner(t *testing.T) {
	env := setupUploadTestServer(t)

	user, _ := env.core.CreateUser(env.ctx, "system", "deleter", "Deleter", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	env.login(t, "deleter", "password123")

	imageData := createTestPNG(t, 1200, 400)
	operations := `{
		"query": "mutation($input: UploadServerBannerInput!) { uploadServerBanner(input: $input) { profile { bannerUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to upload banner: %v", resp.Errors)
	}

	deleteResp := env.doGraphQL(t, `mutation { deleteServerBanner { profile { bannerUrl } } }`, nil)

	if len(deleteResp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", deleteResp.Errors)
	}

	var data struct {
		DeleteServerBanner struct {
			Profile struct {
				BannerURL *string `json:"bannerUrl"`
			} `json:"profile"`
		} `json:"deleteServerBanner"`
	}
	if err := json.Unmarshal(deleteResp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.DeleteServerBanner.Profile.BannerURL != nil {
		t.Error("Expected bannerUrl to be null after deletion")
	}
}
