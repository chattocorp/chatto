package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

func TestProviderScopesForOIDC(t *testing.T) {
	t.Run("default requests openid profile email", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{Type: config.AuthProviderTypeOpenIDConnect})
		want := []string{oidc.ScopeOpenID, "profile", "email"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("request_email false keeps openid profile", func(t *testing.T) {
		requestEmail := false
		scopes := providerScopes(config.AuthProviderConfig{
			Type:         config.AuthProviderTypeOpenIDConnect,
			RequestEmail: &requestEmail,
		})
		want := []string{oidc.ScopeOpenID, "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("custom scopes are honored with openid required", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{
			Type:   config.AuthProviderTypeOpenIDConnect,
			Scopes: []string{"groups", "profile"},
		})
		want := []string{oidc.ScopeOpenID, "groups", "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})
}

func TestLegacyOIDCRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 issuer.URL,
			"authorization_endpoint": issuer.URL + "/authorize",
			"token_endpoint":         issuer.URL + "/token",
			"jwks_uri":               issuer.URL + "/keys",
			"userinfo_endpoint":      issuer.URL + "/userinfo",
		})
	}))
	t.Cleanup(issuer.Close)

	router := gin.New()
	sessionStore := cookie.NewStore([]byte("test-secret-key-32-bytes-long!!"))
	router.Use(sessions.Sessions("chatto_session", sessionStore))

	s := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{
				URL: "http://chat.example",
			},
			Auth: config.AuthConfig{
				Providers: []config.AuthProviderConfig{{
					ID:           "hub",
					Type:         config.AuthProviderTypeOpenIDConnect,
					IssuerURL:    issuer.URL,
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				}},
			},
		},
		router: router,
		logger: log.WithPrefix("test.HTTP"),
	}
	s.setupOIDCRoutes()

	t.Run("legacy login route uses legacy callback URI", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/oidc", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/oidc status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		assertRedirectURI(t, w.Header().Get("Location"), "http://chat.example/auth/oidc/callback")
	})

	t.Run("provider login route keeps provider callback URI", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/providers/hub", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/providers/hub status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		assertRedirectURI(t, w.Header().Get("Location"), "http://chat.example/auth/providers/hub/callback")
	})

	t.Run("legacy callback route is served", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/oidc/callback?state=missing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatal("GET /auth/oidc/callback returned 404")
		}
		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET /auth/oidc/callback status = %d, want %d", w.Code, http.StatusTemporaryRedirect)
		}
		if location := w.Header().Get("Location"); !strings.HasPrefix(location, "/login?error=") {
			t.Fatalf("GET /auth/oidc/callback Location = %q, want login error redirect", location)
		}
	})
}

func assertRedirectURI(t *testing.T, location, want string) {
	t.Helper()
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("redirect Location %q did not parse: %v", location, err)
	}
	got := redirectURL.Query().Get("redirect_uri")
	if got != want {
		t.Fatalf("redirect_uri = %q, want %q; Location = %q", got, want, location)
	}
}
