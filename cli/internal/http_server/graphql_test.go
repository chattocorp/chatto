package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// GraphQL Test Helpers
// ============================================================================

// graphqlTestEnv holds all test dependencies for GraphQL integration tests
type graphqlTestEnv struct {
	server *httptest.Server
	client *http.Client
	core   *core.ChattoCore
	ctx    context.Context
}

// setupGraphQLTestServer creates a full HTTP server with GraphQL routes for testing.
// This tests the complete HTTP → GraphQL → Resolver → Core → NATS stack.
func setupGraphQLTestServer(t *testing.T) *graphqlTestEnv {
	return setupGraphQLTestServerWithConfig(t, config.OwnersConfig{})
}

// setupGraphQLTestServerWithConfig creates a test server with custom admin config.
func setupGraphQLTestServerWithConfig(t *testing.T, ownersConfig config.OwnersConfig) *graphqlTestEnv {
	coreConfig := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	return setupGraphQLTestServerFull(t, ownersConfig, coreConfig)
}

// setupGraphQLTestServerWithEncryption creates a test server for encryption tests.
func setupGraphQLTestServerWithEncryption(t *testing.T) *graphqlTestEnv {
	coreConfig := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	return setupGraphQLTestServerFull(t, config.OwnersConfig{}, coreConfig)
}

// setupGraphQLTestServerFull creates a test server with full config control.
func setupGraphQLTestServerFull(t *testing.T, ownersConfig config.OwnersConfig, coreConfig config.CoreConfig) *graphqlTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Plumb owners.emails through CoreConfig so the email-verification
	// auto-promotion path can see them. The first user whose verified
	// email matches gets the owner role assigned automatically — there
	// is no separate config-owner fall-through to short-circuit
	// permission checks.
	coreConfig.Owners = ownersConfig

	// Create ChattoCore with provided config
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

	// Create HTTPServer with both auth and GraphQL routes
	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
			Owners: ownersConfig,
			Core:   coreConfig,
		},
		nc:     nc,
		router: router,
		core:   chattoCore,
		mailer: email.NewMockSender(true),
	}

	// Set up both auth AND GraphQL routes
	s.setupAuthRoutes()
	s.setupGraphQLAPI(s.buildAllowedOrigins())

	// Create test server
	ts := httptest.NewServer(router)
	t.Cleanup(func() { ts.Close() })

	// Create client with cookie jar for session persistence
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &graphqlTestEnv{
		server: ts,
		client: client,
		core:   chattoCore,
		ctx:    ctx,
	}
}

// graphqlRequest represents a GraphQL request
type graphqlRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

// graphqlResponse represents a GraphQL response
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors,omitempty"`
}

type graphqlError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

func assertGraphQLAuthRequired(t *testing.T, resp *graphqlResponse) {
	t.Helper()

	if len(resp.Errors) == 0 {
		t.Fatal("Expected GraphQL authentication error")
	}
	if resp.Errors[0].Message != "authentication required" {
		t.Fatalf("Expected authentication error, got: %v", resp.Errors)
	}
}

// doGraphQL makes a GraphQL request and returns the response
func (env *graphqlTestEnv) doGraphQL(t *testing.T, query string, variables map[string]any) *graphqlResponse {
	t.Helper()

	reqBody := graphqlRequest{
		Query:     query,
		Variables: variables,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal GraphQL request: %v", err)
	}

	resp, err := env.client.Post(env.server.URL+"/api/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send GraphQL request: %v", err)
	}
	defer resp.Body.Close()

	// GraphQL typically returns 200, but gqlgen can return 422 for validation errors
	// We accept both and let the caller check for errors in the response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("Expected status 200 or 422, got %d", resp.StatusCode)
	}

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("Failed to decode GraphQL response: %v", err)
	}

	return &gqlResp
}

func (env *graphqlTestEnv) doRawGraphQL(t *testing.T, body string, contentLength *int64) (*http.Response, *graphqlResponse) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/api/graphql", io.NopCloser(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("Failed to create GraphQL request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if contentLength == nil {
		req.ContentLength = -1
	} else {
		req.ContentLength = *contentLength
	}

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GraphQL request: %v", err)
	}
	defer resp.Body.Close()

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("Failed to decode GraphQL response: %v", err)
	}

	return resp, &gqlResp
}

// login authenticates a user and returns true if successful
func (env *graphqlTestEnv) login(t *testing.T, login, password string) bool {
	t.Helper()

	loginBody := map[string]string{
		"login":    login,
		"password": password,
	}
	body, _ := json.Marshal(loginBody)

	resp, err := env.client.Post(env.server.URL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send login request: %v", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// createTestUser creates a test user and returns their ID
func (env *graphqlTestEnv) createTestUser(t *testing.T, login, password string) string {
	t.Helper()

	user, err := env.core.CreateUser(env.ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user.Id
}

// createVerifiedTestUser creates a test user with a verified email address.
// Verified users have additional permissions like joining/creating spaces.
func (env *graphqlTestEnv) createVerifiedTestUser(t *testing.T, login, password string) string {
	t.Helper()

	userID := env.createTestUser(t, login, password)

	// Add verified email directly (simulating OAuth or email verification)
	err := env.core.AddVerifiedEmailDirect(env.ctx, userID, login+"@example.com")
	if err != nil {
		t.Fatalf("Failed to verify test user: %v", err)
	}

	return userID
}

// ============================================================================
// Query Tests
// ============================================================================

func TestGraphQL_Query_Viewer_Unauthenticated(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { viewer { user { id login } } }`, nil)
	if len(resp.Errors) > 0 {
		t.Fatalf("Expected no errors for unauthenticated nullable viewer, got: %v", resp.Errors)
	}
	var data struct {
		Viewer *struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if data.Viewer != nil {
		t.Fatal("Expected nil viewer for unauthenticated request")
	}
}

func TestGraphQL_JSONRequestBodyLimit_AcceptsBodyWithinLimit(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { server { version } }`, nil)
	if len(resp.Errors) > 0 {
		t.Fatalf("Expected request within body limit to succeed, got errors: %v", resp.Errors)
	}
}

func TestGraphQL_JSONRequestBodyLimit_RejectsContentLengthOverLimit(t *testing.T) {
	env := setupGraphQLTestServer(t)
	body := oversizedGraphQLJSONBody(t)
	contentLength := int64(len(body))

	resp, gqlResp := env.doRawGraphQL(t, body, &contentLength)

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	assertGraphQLBodyLimitError(t, gqlResp, graphQLJSONMaxRequestBodySize)
}

func TestGraphQL_JSONRequestBodyLimit_RejectsUnknownLengthBodyOverLimit(t *testing.T) {
	env := setupGraphQLTestServer(t)
	body := oversizedGraphQLJSONBody(t)

	resp, gqlResp := env.doRawGraphQL(t, body, nil)

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
	assertGraphQLBodyLimitError(t, gqlResp, graphQLJSONMaxRequestBodySize)
}

func TestGraphQL_JSONRequestBodyLimit_IgnoresMultipartRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := strings.Repeat("x", int(graphQLJSONMaxRequestBodySize)+1)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	req.ContentLength = int64(len(body))
	ctx.Request = req

	if !limitGraphQLJSONRequestBody(ctx) {
		t.Fatal("multipart request should bypass the JSON request body limit")
	}
}

func mustMarshalGraphQLBody(t *testing.T, req graphqlRequest) string {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal GraphQL request: %v", err)
	}
	return string(body)
}

func oversizedGraphQLJSONBody(t *testing.T) string {
	t.Helper()
	return mustMarshalGraphQLBody(t, graphqlRequest{
		Query: `query($value: String!) { server { version } }`,
		Variables: map[string]any{
			"value": strings.Repeat("x", int(graphQLJSONMaxRequestBodySize)),
		},
	})
}

func assertGraphQLBodyLimitError(t *testing.T, resp *graphqlResponse, limit int64) {
	t.Helper()
	if len(resp.Errors) != 1 {
		t.Fatalf("Expected one GraphQL error, got %d: %v", len(resp.Errors), resp.Errors)
	}
	want := "GraphQL request body exceeds maximum size of " + strconv.FormatInt(limit, 10) + " bytes"
	if resp.Errors[0].Message != want {
		t.Fatalf("Error message = %q, want %q", resp.Errors[0].Message, want)
	}
}

func TestGraphQL_Query_Viewer_Authenticated(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create and login user
	login := "graphqlme"
	password := "password123"
	env.createTestUser(t, login, password)

	if !env.login(t, login, password) {
		t.Fatal("Login failed")
	}

	resp := env.doGraphQL(t, `query { viewer { user { id login } } }`, nil)

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		Viewer struct {
			User struct {
				ID    string `json:"id"`
				Login string `json:"login"`
			} `json:"user"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.Viewer.User.Login != login {
		t.Errorf("Expected login %s, got %s", login, data.Viewer.User.Login)
	}
}

func TestGraphQL_Query_Viewer_CookieSessionRevoked(t *testing.T) {
	env := setupGraphQLTestServer(t)

	login := "graphqlrevoked"
	password := "password123"
	userID := env.createTestUser(t, login, password)

	if !env.login(t, login, password) {
		t.Fatal("Login failed")
	}

	resp := env.doGraphQL(t, `query { viewer { user { id login } } }`, nil)
	if len(resp.Errors) > 0 {
		t.Fatalf("Expected no errors before revocation, got: %v", resp.Errors)
	}
	var before struct {
		Viewer *struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(resp.Data, &before); err != nil {
		t.Fatalf("Failed to unmarshal pre-revocation response: %v", err)
	}
	if before.Viewer == nil {
		t.Fatal("Expected authenticated viewer before revocation")
	}

	if _, err := env.core.RevokeCookieSessionsForUser(env.ctx, userID); err != nil {
		t.Fatalf("RevokeCookieSessionsForUser: %v", err)
	}

	resp = env.doGraphQL(t, `query { viewer { user { id login } } }`, nil)
	if len(resp.Errors) > 0 {
		t.Fatalf("Expected no errors for revoked nullable viewer, got: %v", resp.Errors)
	}
	var after struct {
		Viewer *struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(resp.Data, &after); err != nil {
		t.Fatalf("Failed to unmarshal post-revocation response: %v", err)
	}
	if after.Viewer != nil {
		t.Fatal("Expected nil viewer after cookie session revocation")
	}
}

// TestGraphQL_Query_Spaces_PublicDiscovery tests that the spaces query is public
// PR(a) retired Query.spaces / Query.space(id). Unauthenticated discovery now
// happens via the `instance` query, which exposes the deployment's name, logo,
// banner, etc.
func TestGraphQL_Query_Server_PublicDiscovery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	_ = env.createTestUser(t, "spacesuser", "password123")

	t.Run("unauthenticated user can read instance metadata", func(t *testing.T) {
		resp := env.doGraphQL(t, `query { server { version profile { name logoUrl bannerUrl } } }`, nil)
		if len(resp.Errors) > 0 {
			t.Errorf("Expected no errors for public discovery, got: %v", resp.Errors)
		}
	})

	t.Run("server profile image fields do not accept transform arguments", func(t *testing.T) {
		resp := env.doGraphQL(t, `query {
			server {
				profile {
					logoUrl(width: 96, height: 96)
					bannerUrl(width: 1200, height: 630, fit: COVER)
				}
			}
		}`, nil)
		if len(resp.Errors) == 0 {
			t.Fatal("Expected GraphQL validation errors for server profile image arguments")
		}
		messages := make([]string, 0, len(resp.Errors))
		for _, err := range resp.Errors {
			messages = append(messages, err.Message)
		}
		joined := strings.Join(messages, "\n")
		for _, want := range []string{"Unknown argument \"width\"", "Unknown argument \"height\"", "Unknown argument \"fit\""} {
			if !strings.Contains(joined, want) {
				t.Fatalf("Expected validation errors to contain %q, got:\n%s", want, joined)
			}
		}
	})
}

func TestGraphQL_Query_Room_RequiresMembership(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create user, space, and room
	userID := env.createTestUser(t, "roomowner", "password123")

	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "", "private-room", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Create another user who is NOT a member
	env.createTestUser(t, "outsider", "password123")
	env.login(t, "outsider", "password123")

	// Try to query the room
	resp := env.doGraphQL(t, `query($roomId: ID!) {
		room(roomId: $roomId) { id name }
	}`, map[string]any{
		"roomId": room.Id,
	})

	// Should get an error
	if len(resp.Errors) == 0 {
		t.Error("Expected error for non-member accessing room")
	}
}

// ============================================================================
// Mutation Tests
// ============================================================================

// joinSpace mutation was retired in PR(a). Server membership is implicit on
// signup; callers join individual rooms via Mutation.joinRoom.

func TestGraphQL_Mutation_PostMessage_RequiresRoomMembership(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create owner, space, and room
	ownerID := env.createTestUser(t, "msgowner", "password123")
	room, err := env.core.CreateRoom(env.ctx, ownerID, "channel", "", "message-room", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Create and login as outsider (not a room member)
	env.createTestUser(t, "msgoutsider", "password123")
	env.login(t, "msgoutsider", "password123")

	// Try to post a message
	resp := env.doGraphQL(t, `mutation($input: PostMessageInput!) {
		postMessage(input: $input) {
			id
		}
	}`, map[string]any{
		"input": map[string]any{
			"roomId": room.Id,
			"body":   "Hello!",
		},
	})

	// Should get an error (not a room member)
	if len(resp.Errors) == 0 {
		t.Error("Expected error for non-member posting message")
	}
}

func TestGraphQL_LengthDirective_RejectsOverlongNestedInputField(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `mutation($input: PostMessageInput!) {
		postMessage(input: $input) {
			id
		}
	}`, map[string]any{
		"input": map[string]any{
			"roomId": "Rtest",
			"body":   "hello",
			"linkPreview": map[string]any{
				"url":   "https://example.com",
				"title": strings.Repeat("t", core.MaxLinkPreviewTitleLength+1),
			},
		},
	})

	if len(resp.Errors) != 1 {
		t.Fatalf("Expected one GraphQL validation error, got %d: %#v", len(resp.Errors), resp.Errors)
	}
	if !graphQLErrorPathContains(resp.Errors[0].Path, "linkPreview") || !graphQLErrorPathContains(resp.Errors[0].Path, "title") {
		t.Fatalf("Expected error path to include nested input field, got message=%q path=%#v", resp.Errors[0].Message, resp.Errors[0].Path)
	}
	if !strings.Contains(resp.Errors[0].Message, "must be at most 300 bytes") {
		t.Fatalf("Expected length validation message, got %q", resp.Errors[0].Message)
	}
}

func TestGraphQL_LengthDirective_RejectsOverlongLinkPreviewImageAssetID(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `mutation($input: PostMessageInput!) {
		postMessage(input: $input) {
			id
		}
	}`, map[string]any{
		"input": map[string]any{
			"roomId": "Rtest",
			"body":   "hello",
			"linkPreview": map[string]any{
				"url":          "https://example.com",
				"imageAssetId": strings.Repeat("a", core.MaxLinkPreviewImageAssetIDLength+1),
			},
		},
	})

	if len(resp.Errors) != 1 {
		t.Fatalf("Expected one GraphQL validation error, got %d: %#v", len(resp.Errors), resp.Errors)
	}
	if !graphQLErrorPathContains(resp.Errors[0].Path, "linkPreview") || !graphQLErrorPathContains(resp.Errors[0].Path, "imageAssetId") {
		t.Fatalf("Expected error path to include imageAssetId input field, got message=%q path=%#v", resp.Errors[0].Message, resp.Errors[0].Path)
	}
	if !strings.Contains(resp.Errors[0].Message, "must be at most 15 bytes") {
		t.Fatalf("Expected length validation message, got %q", resp.Errors[0].Message)
	}
}

func TestGraphQL_LengthDirective_RejectsOverlongLinkPreviewQueryURL(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query($url: String!) {
		linkPreview(url: $url) {
			url
		}
	}`, map[string]any{
		"url": strings.Repeat("u", core.MaxLinkPreviewURLLength+1),
	})

	if len(resp.Errors) != 1 {
		t.Fatalf("Expected one GraphQL validation error, got %d: %#v", len(resp.Errors), resp.Errors)
	}
	if !graphQLErrorPathContains(resp.Errors[0].Path, "linkPreview") || !graphQLErrorPathContains(resp.Errors[0].Path, "url") {
		t.Fatalf("Expected error path to include linkPreview url argument, got message=%q path=%#v", resp.Errors[0].Message, resp.Errors[0].Path)
	}
	if !strings.Contains(resp.Errors[0].Message, "must be at most 2048 bytes") {
		t.Fatalf("Expected length validation message, got %q", resp.Errors[0].Message)
	}
}

func TestGraphQL_LengthDirective_AllowsNullNullableInputField(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `mutation($input: UpdateServerConfigInput!) {
		updateServerConfig(input: $input) {
			name
		}
	}`, map[string]any{
		"input": map[string]any{
			"serverName":  "Test Server",
			"description": nil,
		},
	})

	if len(resp.Errors) == 0 {
		t.Fatal("Expected resolver authentication error")
	}
	if strings.Contains(resp.Errors[0].Message, "must be at most") {
		t.Fatalf("Expected null nullable field to bypass length validation, got %q", resp.Errors[0].Message)
	}
}

func graphQLErrorPathContains(path []any, want string) bool {
	for _, segment := range path {
		if segment == want {
			return true
		}
	}
	return false
}

// ============================================================================
// Member Directory Tests
// ============================================================================

func TestGraphQL_Query_ServerMembers_RequiresAuthentication(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `query { server { members(limit: 5) { users { id login } } } }`, nil)

	if len(resp.Errors) == 0 {
		t.Error("Expected error for unauthenticated member directory query")
	}
}

func TestGraphQL_Query_ServerMembers_AuthenticatedUserSucceeds(t *testing.T) {
	env := setupGraphQLTestServer(t)

	env.createTestUser(t, "memberuser", "password123")
	env.login(t, "memberuser", "password123")

	resp := env.doGraphQL(t, `query { server { members(limit: 5) { users { id login } totalCount hasMore } } }`, nil)

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors for authenticated member, got: %v", resp.Errors)
	}

	var data struct {
		Server struct {
			Members struct {
				Users []struct {
					ID    string `json:"id"`
					Login string `json:"login"`
				} `json:"users"`
				TotalCount int  `json:"totalCount"`
				HasMore    bool `json:"hasMore"`
			} `json:"members"`
		} `json:"server"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(data.Server.Members.Users) == 0 {
		t.Error("Expected at least one user")
	}
	if data.Server.Members.TotalCount == 0 {
		t.Error("Expected non-zero totalCount")
	}
}

func TestGraphQL_Query_Viewer(t *testing.T) {
	adminEmail := "admin@example.com"
	env := setupGraphQLTestServerWithConfig(t, config.OwnersConfig{
		Emails: []string{adminEmail},
	})

	// Create admin user with verified email matching config
	// Note: Username "admin" is blocked by default, so we use "adminuser"
	adminID := env.createTestUser(t, "adminuser", "password123")
	if err := env.core.AddVerifiedEmailDirect(env.ctx, adminID, adminEmail); err != nil {
		t.Fatalf("Failed to verify admin email: %v", err)
	}

	// Create regular user (no verified email matching admin config)
	env.createTestUser(t, "regular", "password123")

	t.Run("admin user can view admin", func(t *testing.T) {
		env.login(t, "adminuser", "password123")
		resp := env.doGraphQL(t, `query { viewer { canViewAdmin } }`, nil)

		var data struct {
			Viewer struct {
				CanViewAdmin bool `json:"canViewAdmin"`
			} `json:"viewer"`
		}
		json.Unmarshal(resp.Data, &data)

		if !data.Viewer.CanViewAdmin {
			t.Error("Expected canViewAdmin=true for admin user with verified email")
		}
	})

	t.Run("regular user cannot view admin", func(t *testing.T) {
		env.login(t, "regular", "password123")
		resp := env.doGraphQL(t, `query { viewer { canViewAdmin } }`, nil)

		var data struct {
			Viewer struct {
				CanViewAdmin bool `json:"canViewAdmin"`
			} `json:"viewer"`
		}
		json.Unmarshal(resp.Data, &data)

		if data.Viewer.CanViewAdmin {
			t.Error("Expected canViewAdmin=false for regular user")
		}
	})
}

// ============================================================================
// Error Format Tests
// ============================================================================

func TestGraphQL_ErrorFormat(t *testing.T) {
	var logOutput bytes.Buffer
	oldLogger := log.Default()
	log.SetDefault(log.NewWithOptions(&logOutput, log.Options{
		Level:     log.DebugLevel,
		Formatter: log.JSONFormatter,
	}))
	t.Cleanup(func() {
		log.SetDefault(oldLogger)
	})

	env := setupGraphQLTestServer(t)

	// Invalid query syntax
	resp := env.doGraphQL(t, `query { thisFieldDoesNotExist }`, nil)

	if len(resp.Errors) == 0 {
		t.Error("Expected error for invalid field")
	}

	// Verify error has proper structure
	err := resp.Errors[0]
	if err.Message == "" {
		t.Error("Expected error message")
	}

	logs := logOutput.String()
	if !strings.Contains(logs, `"level":"error"`) {
		t.Fatalf("Expected GraphQL error to be logged at error level, got logs: %s", logs)
	}
	if !strings.Contains(logs, `"prefix":"graphql"`) {
		t.Fatalf("Expected GraphQL error log to use graphql prefix, got logs: %s", logs)
	}
	if !strings.Contains(logs, `"msg":"GraphQL operation failed"`) {
		t.Fatalf("Expected GraphQL error log message, got logs: %s", logs)
	}
	if !strings.Contains(logs, "thisFieldDoesNotExist") {
		t.Fatalf("Expected GraphQL error log to include validation error, got logs: %s", logs)
	}
}

func TestGraphQL_CreateRoom_RequiresGroupID(t *testing.T) {
	env := setupGraphQLTestServer(t)

	resp := env.doGraphQL(t, `mutation {
		createRoom(input: { name: "missing-group" }) { id }
	}`, nil)

	if len(resp.Errors) == 0 {
		t.Fatal("Expected validation error for missing groupId")
	}
	if !strings.Contains(resp.Errors[0].Message, "groupId") {
		t.Fatalf("Expected missing groupId error, got: %v", resp.Errors)
	}
}

func TestGraphQL_Variables(t *testing.T) {
	env := setupGraphQLTestServer(t)

	userID := env.createTestUser(t, "varsuser", "password123")
	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "", "vars-room", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, userID, "channel", userID, room.Id); err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	env.login(t, "varsuser", "password123")

	resp := env.doGraphQL(t, `query GetRoom($roomId: ID!) {
		room(roomId: $roomId) { id name }
	}`, map[string]any{
		"roomId": room.Id,
	})

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
	}

	var data struct {
		Room struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"room"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.Room.ID != room.Id {
		t.Errorf("Expected room ID %s, got %s", room.Id, data.Room.ID)
	}
}

// ============================================================================
// Encryption Tests
// ============================================================================

func TestGraphQL_CryptoShredding_MessageBodyBecomesNull(t *testing.T) {
	env := setupGraphQLTestServerWithEncryption(t)

	// Create user, space, and room
	userID := env.createTestUser(t, "alice", "password123")

	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "", "general", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Join room (actor, space, user, room)
	_, err = env.core.JoinRoom(env.ctx, userID, "channel", userID, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login as the user
	if !env.login(t, "alice", "password123") {
		t.Fatal("Failed to login")
	}

	// Post a message via GraphQL
	postResp := env.doGraphQL(t, `
		mutation PostMessage($input: PostMessageInput!) {
			postMessage(input: $input) {
				id
				event {
					... on MessagePostedEvent {
						body
					}
				}
			}
		}
	`, map[string]any{
		"input": map[string]any{
			"roomId": room.Id,
			"body":   "This is a secret message",
		},
	})

	if len(postResp.Errors) > 0 {
		t.Fatalf("Failed to post message: %v", postResp.Errors)
	}

	var postData struct {
		PostMessage struct {
			ID    string `json:"id"`
			Event struct {
				Body *string `json:"body"`
			} `json:"event"`
		} `json:"postMessage"`
	}
	if err := json.Unmarshal(postResp.Data, &postData); err != nil {
		t.Fatalf("Failed to unmarshal post response: %v", err)
	}

	// Verify message is readable
	if postData.PostMessage.Event.Body == nil || *postData.PostMessage.Event.Body != "This is a secret message" {
		t.Fatalf("Expected message body 'This is a secret message', got %v", postData.PostMessage.Event.Body)
	}

	eventID := postData.PostMessage.ID

	// Delete the user's encryption key (crypto-shredding)
	if err := env.core.DeleteUserEncryptionKey(env.ctx, userID); err != nil {
		t.Fatalf("Failed to delete encryption key: %v", err)
	}

	// Query the message again via GraphQL
	queryResp := env.doGraphQL(t, `
		query GetMessage($roomId: ID!, $eventId: ID!) {
			room(roomId: $roomId) {
				event(eventId: $eventId) {
					id
					event {
						... on MessagePostedEvent {
							body
						}
					}
				}
			}
		}
	`, map[string]any{
		"roomId":  room.Id,
		"eventId": eventID,
	})

	if len(queryResp.Errors) > 0 {
		t.Fatalf("Failed to query message: %v", queryResp.Errors)
	}

	var queryData struct {
		Room struct {
			Event struct {
				ID    string `json:"id"`
				Event struct {
					Body *string `json:"body"`
				} `json:"event"`
			} `json:"event"`
		} `json:"room"`
	}
	if err := json.Unmarshal(queryResp.Data, &queryData); err != nil {
		t.Fatalf("Failed to unmarshal query response: %v", err)
	}

	// Verify body is now null (crypto-shredded)
	if queryData.Room.Event.Event.Body != nil {
		t.Errorf("Expected body to be null after crypto-shredding, got %q", *queryData.Room.Event.Event.Body)
	}
}

// ============================================================================
// Bearer Token GraphQL Auth Tests
// ============================================================================

// doGraphQLWithToken makes a GraphQL request using a bearer token instead of cookies.
func (env *graphqlTestEnv) doGraphQLWithToken(t *testing.T, token string, query string) *graphqlResponse {
	t.Helper()

	reqBody := graphqlRequest{Query: query}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal GraphQL request: %v", err)
	}

	req, err := http.NewRequest("POST", env.server.URL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Use a client WITHOUT a cookie jar to ensure we're testing token auth only
	noCookieClient := &http.Client{}
	resp, err := noCookieClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send GraphQL request: %v", err)
	}
	defer resp.Body.Close()

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("Failed to decode GraphQL response: %v", err)
	}

	return &gqlResp
}

func TestBearerToken_AuthenticatesGraphQL(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create a user and get a token
	userID := env.createTestUser(t, "beareruser", "password123")
	token, err := env.core.CreateAuthToken(env.ctx, userID)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}

	// Make a GraphQL query using only the bearer token (no cookies)
	resp := env.doGraphQLWithToken(t, token, `{ viewer { user { id login } } }`)

	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}

	var data struct {
		Viewer struct {
			User struct {
				ID    string `json:"id"`
				Login string `json:"login"`
			} `json:"user"`
		} `json:"viewer"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if data.Viewer.User.ID != userID {
		t.Errorf("viewer.user.id = %q, want %q", data.Viewer.User.ID, userID)
	}
	if data.Viewer.User.Login != "beareruser" {
		t.Errorf("viewer.user.login = %q, want %q", data.Viewer.User.Login, "beareruser")
	}
}

func TestBearerToken_InvalidToken(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Make a protected GraphQL query with an invalid token. Nullable viewer
	// intentionally returns null for unauthenticated callers, so use a field
	// that still requires authentication to prove the bearer token failed.
	resp := env.doGraphQLWithToken(t, "cht_ATinvalidtoken1234", `{ server { rooms { id } } }`)
	assertGraphQLAuthRequired(t, resp)
}

func TestBearerToken_RevokedTokenFails(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create user and token
	userID := env.createTestUser(t, "revokeuser", "password123")
	token, err := env.core.CreateAuthToken(env.ctx, userID)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}

	// Verify it works
	resp := env.doGraphQLWithToken(t, token, `{ viewer { user { id } } }`)
	var data struct {
		Viewer *struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"viewer"`
	}
	json.Unmarshal(resp.Data, &data)
	if data.Viewer == nil {
		t.Fatal("Token should authenticate before revocation")
	}

	// Revoke it
	if err := env.core.RevokeAuthToken(env.ctx, token); err != nil {
		t.Fatalf("Failed to revoke token: %v", err)
	}

	// Verify it no longer works against a protected field.
	resp = env.doGraphQLWithToken(t, token, `{ server { rooms { id } } }`)
	assertGraphQLAuthRequired(t, resp)
}
