package http_server

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

func setupCSRFTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
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
			Webserver: config.WebserverConfig{URL: "http://localhost:4000"},
		},
		router: router,
	}
	router.Use(s.csrfMiddleware())

	router.GET("/login-test", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", "u_test")
		if err := session.Save(); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.ensureCSRFToken(c, session); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.String(http.StatusOK, "logged in")
	})
	router.POST("/api/graphql", func(c *gin.Context) {
		c.String(http.StatusOK, "graphql ok")
	})
	router.POST("/auth/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		_ = session.Save()
		clearCSRFCookie(c)
		c.String(http.StatusOK, "logged out")
	})
	router.POST("/auth/verify-email/request-code", func(c *gin.Context) {
		c.String(http.StatusOK, "verification ok")
	})
	router.POST("/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, "login ok")
	})
	router.POST("/oauth/token", func(c *gin.Context) {
		c.String(http.StatusOK, "token ok")
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return server, &http.Client{Jar: jar}
}

func csrfCookieValue(t *testing.T, client *http.Client, serverURL string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, serverURL+"/login-test", nil)
	if err != nil {
		t.Fatalf("create login request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == csrfCookieName {
			if cookie.HttpOnly {
				t.Fatal("CSRF cookie must be readable by the SPA")
			}
			if cookie.Value == "" {
				t.Fatal("CSRF cookie was empty")
			}
			return cookie.Value
		}
	}
	t.Fatal("CSRF cookie was not set")
	return ""
}

func TestCSRFMiddleware(t *testing.T) {
	t.Run("rejects cookie GraphQL POST without token", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		resp, err := client.Post(server.URL+"/api/graphql", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("GraphQL request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 403; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("accepts cookie GraphQL POST with matching token", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		token := csrfCookieValue(t, client, server.URL)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/graphql", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("create GraphQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeaderName, token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GraphQL request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("accepts cookie GraphQL POST with request type header", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/graphql", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("create GraphQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfGraphQLRequestHeader, csrfGraphQLRequestHeaderVal)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GraphQL request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("exempts bearer GraphQL POST", func(t *testing.T) {
		server, _ := setupCSRFTestServer(t)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/graphql", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("create GraphQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer cht_ATtesttoken123")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GraphQL request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("rejects cookie GraphQL POST with bearer header but no CSRF token", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/graphql", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("create GraphQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid-token")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GraphQL request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 403; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("clears CSRF cookie after logout", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		token := csrfCookieValue(t, client, server.URL)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/auth/logout", nil)
		if err != nil {
			t.Fatalf("create logout request: %v", err)
		}
		req.Header.Set(csrfHeaderName, token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("logout request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
		foundExpiredCookie := false
		for _, cookie := range resp.Cookies() {
			if cookie.Name != csrfCookieName {
				continue
			}
			if cookie.MaxAge >= 0 {
				t.Fatalf("CSRF cookie was not expired on logout: MaxAge=%d", cookie.MaxAge)
			}
			foundExpiredCookie = true
		}
		if !foundExpiredCookie {
			t.Fatal("logout did not expire the CSRF cookie")
		}
	})

	t.Run("rejects other cookie-authenticated unsafe routes without token", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		resp, err := client.Post(server.URL+"/auth/verify-email/request-code", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("verification request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 403; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("accepts other cookie-authenticated unsafe routes with matching token", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		token := csrfCookieValue(t, client, server.URL)

		req, err := http.NewRequest(http.MethodPost, server.URL+"/auth/verify-email/request-code", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("create verification request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeaderName, token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("verification request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("exempts public auth endpoints even when a session exists", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		resp, err := client.Post(server.URL+"/auth/login", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})

	t.Run("exempts OAuth token exchange", func(t *testing.T) {
		server, client := setupCSRFTestServer(t)
		csrfCookieValue(t, client, server.URL)

		resp, err := client.Post(server.URL+"/oauth/token", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("OAuth token request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
		}
	})
}
