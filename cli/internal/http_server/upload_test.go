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

// ============================================================================
// Upload Tests
// ============================================================================

func TestUpload_InstanceLogo_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	user, err := env.core.CreateUser(env.ctx, "system", "uploader", "Uploader", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if _, err := env.core.CreateSpace(env.ctx, user.Id, "Upload Test Space", ""); err != nil {
		t.Fatalf("Failed to create server space: %v", err)
	}

	env.login(t, "uploader", "password123")

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadInstanceLogoInput!) { uploadInstanceLogo(input: $input) { config { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadInstanceLogo struct {
			Config struct {
				LogoURL *string `json:"logoUrl"`
			} `json:"config"`
		} `json:"uploadInstanceLogo"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.UploadInstanceLogo.Config.LogoURL == nil || *data.UploadInstanceLogo.Config.LogoURL == "" {
		t.Error("Expected logoUrl to be set after upload")
	}
}

func TestUpload_InstanceLogo_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	user, _ := env.core.CreateUser(env.ctx, "system", "owner", "Owner", "password123")
	if _, err := env.core.CreateSpace(env.ctx, user.Id, "Test Space", ""); err != nil {
		t.Fatalf("Failed to create server space: %v", err)
	}

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadInstanceLogoInput!) { uploadInstanceLogo(input: $input) { config { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected authentication error")
	}
}

func TestUpload_InstanceLogo_NotAdmin(t *testing.T) {
	env := setupUploadTestServer(t)

	owner, _ := env.core.CreateUser(env.ctx, "system", "owner", "Owner", "password123")
	if _, err := env.core.CreateSpace(env.ctx, owner.Id, "Owner's Space", ""); err != nil {
		t.Fatalf("Failed to create server space: %v", err)
	}

	env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")
	env.login(t, "other", "password123")

	imageData := createTestPNG(t, 256, 256)

	operations := `{
		"query": "mutation($input: UploadInstanceLogoInput!) { uploadInstanceLogo(input: $input) { config { logoUrl } } }",
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

func TestUpload_InstanceLogo_DeleteLogo(t *testing.T) {
	env := setupUploadTestServer(t)

	user, _ := env.core.CreateUser(env.ctx, "system", "deleter", "Deleter", "password123")
	if _, err := env.core.CreateSpace(env.ctx, user.Id, "Delete Logo Test", ""); err != nil {
		t.Fatalf("Failed to create server space: %v", err)
	}

	env.login(t, "deleter", "password123")

	imageData := createTestPNG(t, 256, 256)
	operations := `{
		"query": "mutation($input: UploadInstanceLogoInput!) { uploadInstanceLogo(input: $input) { config { logoUrl } } }",
		"variables": { "input": { "file": null } }
	}`

	resp := env.doMultipartUpload(t, operations, imageData, "logo.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to upload logo: %v", resp.Errors)
	}

	deleteResp := env.doGraphQL(t, `mutation { deleteInstanceLogo { config { logoUrl } } }`, nil)

	if len(deleteResp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", deleteResp.Errors)
	}

	var data struct {
		DeleteInstanceLogo struct {
			Config struct {
				LogoURL *string `json:"logoUrl"`
			} `json:"config"`
		} `json:"deleteInstanceLogo"`
	}
	if err := json.Unmarshal(deleteResp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.DeleteInstanceLogo.Config.LogoURL != nil {
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

	// Create user and space
	user, _ := env.core.CreateUser(env.ctx, "system", "largeuser", "Large User", "password123")
	space, _ := env.core.CreateSpace(env.ctx, user.Id, "Large Image Test", "")

	// Login
	env.login(t, "largeuser", "password123")

	// Create a larger test image (1024x1024)
	imageData := createTestPNG(t, 1024, 1024)

	// Upload
	operations := fmt.Sprintf(`{
		"query": "mutation($input: UploadSpaceLogoInput!) { uploadSpaceLogo(input: $input) { id logoUrl } }",
		"variables": { "input": { "spaceId": "%s", "file": null } }
	}`, space.Id)

	resp := env.doMultipartUpload(t, operations, imageData, "large-logo.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadSpaceLogo struct {
			LogoURL *string `json:"logoUrl"`
		} `json:"uploadSpaceLogo"`
	}
	json.Unmarshal(resp.Data, &data)

	// Logo should be uploaded successfully (server resizes to 512x512 max)
	if data.UploadSpaceLogo.LogoURL == nil {
		t.Error("Expected logoUrl to be set")
	}
}

// ============================================================================
// Space Banner Upload Tests
// ============================================================================

func TestUpload_SpaceBanner_Success(t *testing.T) {
	env := setupUploadTestServer(t)

	// Create user and space (user is admin of own space)
	user, err := env.core.CreateUser(env.ctx, "system", "banneruser", "Banner User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	space, err := env.core.CreateSpace(env.ctx, user.Id, "Banner Test Space", "")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Login
	env.login(t, "banneruser", "password123")

	// Create test image (banner is wider aspect ratio)
	imageData := createTestPNG(t, 1200, 400)

	// Upload banner
	operations := fmt.Sprintf(`{
		"query": "mutation($input: UploadSpaceBannerInput!) { uploadSpaceBanner(input: $input) { id name bannerUrl } }",
		"variables": { "input": { "spaceId": "%s", "file": null } }
	}`, space.Id)

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		UploadSpaceBanner struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			BannerURL *string `json:"bannerUrl"`
		} `json:"uploadSpaceBanner"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.UploadSpaceBanner.ID != space.Id {
		t.Errorf("Expected space ID %s, got %s", space.Id, data.UploadSpaceBanner.ID)
	}

	if data.UploadSpaceBanner.BannerURL == nil || *data.UploadSpaceBanner.BannerURL == "" {
		t.Error("Expected bannerUrl to be set after upload")
	}
}

func TestUpload_SpaceBanner_Unauthenticated(t *testing.T) {
	env := setupUploadTestServer(t)

	// Create user and space
	user, _ := env.core.CreateUser(env.ctx, "system", "owner", "Owner", "password123")
	space, _ := env.core.CreateSpace(env.ctx, user.Id, "Test Space", "")

	// Don't login

	// Create test image
	imageData := createTestPNG(t, 1200, 400)

	// Try to upload (should fail)
	operations := fmt.Sprintf(`{
		"query": "mutation($input: UploadSpaceBannerInput!) { uploadSpaceBanner(input: $input) { id bannerUrl } }",
		"variables": { "input": { "spaceId": "%s", "file": null } }
	}`, space.Id)

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected authentication error")
	}
}

func TestUpload_SpaceBanner_NotSpaceAdmin(t *testing.T) {
	env := setupUploadTestServer(t)

	// Create owner and space
	owner, _ := env.core.CreateUser(env.ctx, "system", "owner", "Owner", "password123")
	space, _ := env.core.CreateSpace(env.ctx, owner.Id, "Owner's Space", "")

	// Create another user who is NOT the space admin
	env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")

	// Login as the non-admin user
	env.login(t, "other", "password123")

	// Create test image
	imageData := createTestPNG(t, 1200, 400)

	// Try to upload (should fail - no permission)
	operations := fmt.Sprintf(`{
		"query": "mutation($input: UploadSpaceBannerInput!) { uploadSpaceBanner(input: $input) { id bannerUrl } }",
		"variables": { "input": { "spaceId": "%s", "file": null } }
	}`, space.Id)

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")

	if len(resp.Errors) == 0 {
		t.Error("Expected permission denied error")
	}

	// Check error message
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

func TestUpload_SpaceBanner_DeleteBanner(t *testing.T) {
	env := setupUploadTestServer(t)

	// Create user and space
	user, _ := env.core.CreateUser(env.ctx, "system", "deleter", "Deleter", "password123")
	space, _ := env.core.CreateSpace(env.ctx, user.Id, "Delete Banner Test", "")

	// Login
	env.login(t, "deleter", "password123")

	// First upload a banner
	imageData := createTestPNG(t, 1200, 400)
	operations := fmt.Sprintf(`{
		"query": "mutation($input: UploadSpaceBannerInput!) { uploadSpaceBanner(input: $input) { id bannerUrl } }",
		"variables": { "input": { "spaceId": "%s", "file": null } }
	}`, space.Id)

	resp := env.doMultipartUpload(t, operations, imageData, "banner.png")
	if len(resp.Errors) > 0 {
		t.Fatalf("Failed to upload banner: %v", resp.Errors)
	}

	// Now delete the banner using regular GraphQL
	deleteResp := env.doGraphQL(t, `mutation($input: DeleteSpaceBannerInput!) {
		deleteSpaceBanner(input: $input) { id bannerUrl }
	}`, map[string]any{"input": map[string]any{"spaceId": space.Id}})

	if len(deleteResp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", deleteResp.Errors)
	}

	var data struct {
		DeleteSpaceBanner struct {
			ID        string  `json:"id"`
			BannerURL *string `json:"bannerUrl"`
		} `json:"deleteSpaceBanner"`
	}
	if err := json.Unmarshal(deleteResp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.DeleteSpaceBanner.BannerURL != nil {
		t.Error("Expected bannerUrl to be null after deletion")
	}
}
