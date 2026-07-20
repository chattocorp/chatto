package http_server

import (
	"context"
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
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/testutil"
)

type wsTestEnv struct {
	server     *httptest.Server
	client     *http.Client
	core       *core.ChattoCore
	ctx        context.Context
	cookieJar  *cookiejar.Jar
	httpServer *HTTPServer
}

func setupWebSocketTestServer(t testing.TB) *wsTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	coreConfig := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	chattoCore, err := core.NewChattoCore(ctx, nc, coreConfig)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

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

	s := &HTTPServer{
		config: config.ChattoConfig{
			Auth: config.AuthConfig{},
			Webserver: config.WebserverConfig{
				URL:                 "http://localhost:4000",
				CookieSigningSecret: "test-secret-key-32-bytes-long!!",
			},
			Core: coreConfig,
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		mailer:  email.NewMockSender(true),
		version: "test",
		logger:  log.WithPrefix("test"),
	}
	s.connectAPI = connectapi.New(chattoCore, s.config, s.version)

	s.setupAuthRoutes()
	s.setupRealtimeAPI(s.buildAllowedOrigins())

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
		server:     ts,
		client:     client,
		core:       chattoCore,
		ctx:        ctx,
		cookieJar:  jar,
		httpServer: s,
	}
}

func (env *wsTestEnv) login(t testing.TB, login, password string) {
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

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
