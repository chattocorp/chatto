package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

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
			Core:  coreConfig,
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

	// viewer returns null for unauthenticated users (not an error)
	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", resp.Errors)
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
		t.Error("Expected viewer to be null for unauthenticated user")
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

// TestGraphQL_Query_Spaces_PublicDiscovery tests that the spaces query is public
// PR(a) retired Query.spaces / Query.space(id). Unauthenticated discovery now
// happens via the `instance` query, which exposes the deployment's name, logo,
// banner, etc.
func TestGraphQL_Query_Instance_PublicDiscovery(t *testing.T) {
	env := setupGraphQLTestServer(t)

	_ = env.createTestUser(t, "spacesuser", "password123")

	t.Run("unauthenticated user can read instance metadata", func(t *testing.T) {
		resp := env.doGraphQL(t, `query { server { version config { serverName } } }`, nil)
		if len(resp.Errors) > 0 {
			t.Errorf("Expected no errors for public discovery, got: %v", resp.Errors)
		}
	})
}

func TestGraphQL_Query_Room_RequiresMembership(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create user, space, and room
	userID := env.createTestUser(t, "roomowner", "password123")


	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "private-room", "")
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
		"roomId":  room.Id,
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
	room, err := env.core.CreateRoom(env.ctx, ownerID, "channel", "message-room", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Create and login as outsider (not a room member)
	env.createTestUser(t, "msgoutsider", "password123")
	env.login(t, "msgoutsider", "password123")

	// Try to post a message
	resp := env.doGraphQL(t, `mutation($input: PostMessageInput!) {
		postMessage(input: $input) {
			event { id }
		}
	}`, map[string]any{
		"input": map[string]any{
			"roomId":  room.Id,
			"body":    "Hello!",
		},
	})

	// Should get an error (not a room member)
	if len(resp.Errors) == 0 {
		t.Error("Expected error for non-member posting message")
	}
}

// ============================================================================
// Admin Tests
// ============================================================================

func TestGraphQL_Query_Users_RequiresAdmin(t *testing.T) {
	env := setupGraphQLTestServer(t)

	// Create regular user (no instance role assigned).
	env.createTestUser(t, "regular", "password123")
	env.login(t, "regular", "password123")

	resp := env.doGraphQL(t, `query { users { id login } }`, nil)

	// Should get authorization error
	if len(resp.Errors) == 0 {
		t.Error("Expected error for non-admin querying users")
	}
}

func TestGraphQL_Query_Users_AdminSucceeds(t *testing.T) {
	adminEmail := "admin@example.com"
	// Create server with admin config (uses verified emails for admin check)
	env := setupGraphQLTestServerWithConfig(t, config.OwnersConfig{
		Emails: []string{adminEmail},
	})

	// Create admin user and verify their email to match config
	// Note: Username "admin" is blocked by default, so we use "adminuser"
	adminID := env.createTestUser(t, "adminuser", "password123")
	if err := env.core.AddVerifiedEmailDirect(env.ctx, adminID, adminEmail); err != nil {
		t.Fatalf("Failed to verify admin email: %v", err)
	}
	env.login(t, "adminuser", "password123")

	resp := env.doGraphQL(t, `query { users { id login } }`, nil)

	if len(resp.Errors) > 0 {
		t.Errorf("Expected no errors for admin, got: %v", resp.Errors)
	}

	var data struct {
		Users []struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"users"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(data.Users) == 0 {
		t.Error("Expected at least one user")
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
}

func TestGraphQL_Variables(t *testing.T) {
	env := setupGraphQLTestServer(t)

	userID := env.createTestUser(t, "varsuser", "password123")
	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "vars-room", "")
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
		"roomId":  room.Id,
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


	room, err := env.core.CreateRoom(env.ctx, userID, "channel", "general", "")
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
			"roomId":  room.Id,
			"body":    "This is a secret message",
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

	// Make a GraphQL query with an invalid token
	resp := env.doGraphQLWithToken(t, "cht_ATinvalidtoken1234", `{ viewer { user { id login } } }`)

	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", resp.Errors)
	}

	// viewer should return null (unauthenticated, not error)
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
		t.Errorf("viewer should be null for invalid token, got %+v", data.Viewer)
	}
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
			User struct{ ID string `json:"id"` } `json:"user"`
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

	// Verify it no longer works
	resp = env.doGraphQLWithToken(t, token, `{ viewer { user { id } } }`)
	json.Unmarshal(resp.Data, &data)
	if data.Viewer != nil {
		t.Error("viewer should be null after token revocation")
	}
}
