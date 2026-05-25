package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
)

// ============================================================================
// WebSocket Test Helpers
// ============================================================================

// wsTestEnv holds all test dependencies for WebSocket tests
type wsTestEnv struct {
	server    *httptest.Server
	client    *http.Client
	core      *core.ChattoCore
	ctx       context.Context
	cookieJar *cookiejar.Jar
}

// setupWebSocketTestServer creates a test server for WebSocket testing.
func setupWebSocketTestServer(t *testing.T) *wsTestEnv {
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// Create ChattoCore
	coreConfig := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Start all core background services (PresenceHub + every
	// registered projector) via the shared test helper. Blocks
	// until projectors report started so test code can read from
	// projection-backed accessors without racing the goroutine
	// startup.
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

	// Create HTTPServer with auth and GraphQL routes
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

	return &wsTestEnv{
		server:    ts,
		client:    client,
		core:      chattoCore,
		ctx:       ctx,
		cookieJar: jar,
	}
}

// login authenticates a user
func (env *wsTestEnv) login(t *testing.T, login, password string) {
	t.Helper()

	loginBody := `{"login":"` + login + `","password":"` + password + `"}`
	resp, err := env.client.Post(env.server.URL+"/auth/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d", resp.StatusCode)
	}
}

// connectWebSocket establishes a WebSocket connection to the GraphQL endpoint
func (env *wsTestEnv) connectWebSocket(t *testing.T) *websocket.Conn {
	t.Helper()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(env.server.URL, "http") + "/api/graphql"

	// Build headers with cookies
	header := http.Header{}
	for _, c := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		header.Add("Cookie", c.String())
	}

	// Use graphql-ws subprotocol
	dialer := websocket.Dialer{
		Subprotocols: []string{"graphql-transport-ws"},
	}

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("WebSocket dial failed with status %d: %v", resp.StatusCode, err)
		}
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	t.Cleanup(func() { conn.Close() })
	return conn
}

// mustParseURL parses a URL or panics
func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}

// graphqlWSMessage represents a graphql-transport-ws protocol message
type graphqlWSMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// subscriptionPayload represents the payload for a subscribe message
type subscriptionPayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// sendWSMessage sends a graphql-ws message
func sendWSMessage(t *testing.T, conn *websocket.Conn, msg graphqlWSMessage) {
	t.Helper()
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("Failed to send WebSocket message: %v", err)
	}
}

// readWSMessage reads a graphql-ws message with timeout
func readWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) *graphqlWSMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))

	_, data, err := conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return nil
		}
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	var msg graphqlWSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal WebSocket message: %v", err)
	}
	return &msg
}

// ============================================================================
// WebSocket Connection Tests
// ============================================================================

func TestWebSocket_ConnectionInit(t *testing.T) {
	env := setupWebSocketTestServer(t)

	// Create and login user
	_, err := env.core.CreateUser(env.ctx, "system", "wsuser", "WS User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	env.login(t, "wsuser", "password123")

	conn := env.connectWebSocket(t)

	// Send connection_init
	sendWSMessage(t, conn, graphqlWSMessage{Type: "connection_init"})

	// Should receive connection_ack
	msg := readWSMessage(t, conn, 5*time.Second)
	if msg == nil {
		t.Fatal("Expected connection_ack, got nil")
	}
	if msg.Type != "connection_ack" {
		t.Errorf("Expected connection_ack, got %s", msg.Type)
	}
}

func TestWebSocket_Subscription_Authenticated(t *testing.T) {
	env := setupWebSocketTestServer(t)

	// Create user, space, and room
	user, err := env.core.CreateUser(env.ctx, "system", "subuser", "Sub User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}



	room, err := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "sub-room", "")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	_, err = env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Login and connect
	env.login(t, "subuser", "password123")
	conn := env.connectWebSocket(t)

	// Connection init
	sendWSMessage(t, conn, graphqlWSMessage{Type: "connection_init"})
	msg := readWSMessage(t, conn, 5*time.Second)
	if msg.Type != "connection_ack" {
		t.Fatalf("Expected connection_ack, got %s", msg.Type)
	}

	// Subscribe to server events
	payload, _ := json.Marshal(subscriptionPayload{
		Query: `subscription {
			myEvents {
				id
				event {
					... on MessagePostedEvent {
						body
						roomId
					}
				}
			}
		}`,
	})

	sendWSMessage(t, conn, graphqlWSMessage{
		ID:      "1",
		Type:    "subscribe",
		Payload: payload,
	})

	// Give subscription time to be ready
	time.Sleep(100 * time.Millisecond)

	// Post a message
	go func() {
		_, err = env.core.PostMessage(env.ctx, "channel", room.Id, user.Id, "Hello WebSocket!", nil, "", "", nil, false)
		if err != nil {
			t.Logf("Failed to post message: %v", err)
		}
	}()

	// Read events until we get the message (skip non-message events like presence)
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		msg = readWSMessage(t, conn, time.Until(deadline))
		if msg == nil {
			break
		}
		if msg.Type != "next" {
			t.Errorf("Expected next, got %s", msg.Type)
			break
		}
		if msg.ID != "1" {
			t.Errorf("Expected subscription ID 1, got %s", msg.ID)
			break
		}
		if strings.Contains(string(msg.Payload), "Hello WebSocket!") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to receive message event with 'Hello WebSocket!'")
	}
}

func TestWebSocket_Subscription_Unauthenticated(t *testing.T) {
	env := setupWebSocketTestServer(t)

	// Create a space (need one for the subscription)
	_, _ = env.core.CreateUser(env.ctx, "system", "owner", "Owner", "password123")

	// Connect without logging in
	conn := env.connectWebSocket(t)

	// Connection init
	sendWSMessage(t, conn, graphqlWSMessage{Type: "connection_init"})
	msg := readWSMessage(t, conn, 5*time.Second)
	if msg.Type != "connection_ack" {
		t.Fatalf("Expected connection_ack, got %s", msg.Type)
	}

	// Try to subscribe
	payload, _ := json.Marshal(subscriptionPayload{
		Query: `subscription { myEvents { id event { ... on MessagePostedEvent { body } } } }`,
	})

	sendWSMessage(t, conn, graphqlWSMessage{
		ID:      "1",
		Type:    "subscribe",
		Payload: payload,
	})

	// Should receive an error response (may come as "next" with errors or "error")
	msg = readWSMessage(t, conn, 5*time.Second)
	if msg == nil {
		t.Fatal("Expected error response, got nil")
	}

	// GraphQL subscription errors can come as "next" with errors array or "error" type
	// Either way, the payload should contain "authentication required"
	if !strings.Contains(string(msg.Payload), "authentication required") {
		t.Errorf("Expected authentication error in payload, got: %s", string(msg.Payload))
	}
}

func TestWebSocket_MultipleSubscriptions(t *testing.T) {
	env := setupWebSocketTestServer(t)

	// Create user and two spaces
	user, _ := env.core.CreateUser(env.ctx, "system", "multiuser", "Multi User", "password123")

	room1, _ := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "room1", "")
	env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room1.Id)

	room2, _ := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "room2", "")
	env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room2.Id)

	// Login and connect
	env.login(t, "multiuser", "password123")
	conn := env.connectWebSocket(t)

	// Connection init
	sendWSMessage(t, conn, graphqlWSMessage{Type: "connection_init"})
	readWSMessage(t, conn, 5*time.Second) // connection_ack

	// myEvents is deployment-wide and takes no args. Two subscriptions
	// over the single feed exercise the multi-subscription dispatch path.
	for i := 0; i < 2; i++ {
		payload, _ := json.Marshal(subscriptionPayload{
			Query: `subscription { myEvents { id event { ... on MessagePostedEvent { body } } } }`,
		})
		sendWSMessage(t, conn, graphqlWSMessage{
			ID:      string(rune('1' + i)),
			Type:    "subscribe",
			Payload: payload,
		})
	}

	time.Sleep(100 * time.Millisecond)

	// Post message to space1
	go func() {
		env.core.PostMessage(env.ctx, "channel", room1.Id, user.Id, "Message in space 1", nil, "", "", nil, false)
	}()

	// Should receive event for space1 (ID "1")
	// There may be other events (e.g., from room joining) so keep reading until we find it
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		msg := readWSMessage(t, conn, time.Until(deadline))
		if msg == nil {
			break
		}
		if msg.Type == "next" && msg.ID == "1" && strings.Contains(string(msg.Payload), "Message in space 1") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Did not receive expected event on subscription ID 1 for space1")
	}
}

func TestWebSocket_Unsubscribe(t *testing.T) {
	env := setupWebSocketTestServer(t)

	// Create user, space, room
	user, _ := env.core.CreateUser(env.ctx, "system", "unsubuser", "Unsub User", "password123")
	room, _ := env.core.CreateRoom(env.ctx, user.Id, "channel", "", "unsub-room", "")
	env.core.JoinRoom(env.ctx, user.Id, "channel", user.Id, room.Id)

	env.login(t, "unsubuser", "password123")
	conn := env.connectWebSocket(t)

	// Connection init
	sendWSMessage(t, conn, graphqlWSMessage{Type: "connection_init"})
	readWSMessage(t, conn, 5*time.Second)

	// Subscribe
	payload, _ := json.Marshal(subscriptionPayload{
		Query: `subscription { myEvents { id event { ... on MessagePostedEvent { body } } } }`,
	})
	sendWSMessage(t, conn, graphqlWSMessage{ID: "1", Type: "subscribe", Payload: payload})

	// Drain any initial events (e.g., presence changes) before unsubscribing
	time.Sleep(300 * time.Millisecond)
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break // No more buffered messages
		}
	}

	// Unsubscribe
	sendWSMessage(t, conn, graphqlWSMessage{ID: "1", Type: "complete"})

	// Wait for the server to process the unsubscribe
	time.Sleep(200 * time.Millisecond)

	// Post a message - should NOT receive it
	env.core.PostMessage(env.ctx, "channel", room.Id, user.Id, "Should not receive", nil, "", "", nil, false)

	// Read with short timeout - should timeout without receiving event
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, data, err := conn.ReadMessage()
	if err == nil {
		// If we received something, it might be a "complete" ack, which is ok
		var msg graphqlWSMessage
		json.Unmarshal(data, &msg)
		if msg.Type == "next" && strings.Contains(string(msg.Payload), "Should not receive") {
			t.Error("Should not receive message events after unsubscribe")
		}
	}
	// Timeout is the expected behavior (no events after unsubscribe)
}
