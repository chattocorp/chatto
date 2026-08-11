package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
)

func TestBrowserCookieAuthenticationIsSameOriginOnly(t *testing.T) {
	server := setupOAuthServer(t)
	cookies, user := loginOAuthTestUser(t, server, "same-origin-cookie")
	server.router.GET("/test/whoami", func(c *gin.Context) {
		request := server.injectUserIntoContext(c)
		if authenticated := authctx.ForContext(request.Context()); authenticated != nil {
			c.String(http.StatusOK, authenticated.Id)
			return
		}
		c.Status(http.StatusUnauthorized)
	})

	sameOrigin := httptest.NewRequest(http.MethodGet, "/test/whoami", nil)
	sameOrigin.Header.Set("Origin", "https://chatto.example")
	addCookies(sameOrigin, cookies)
	sameOriginResponse := httptest.NewRecorder()
	server.router.ServeHTTP(sameOriginResponse, sameOrigin)
	if sameOriginResponse.Code != http.StatusOK || sameOriginResponse.Body.String() != user.Id {
		t.Fatalf("same-origin status/body = %d/%q", sameOriginResponse.Code, sameOriginResponse.Body.String())
	}

	crossOrigin := httptest.NewRequest(http.MethodGet, "/test/whoami", nil)
	crossOrigin.Header.Set("Origin", "https://client.example")
	addCookies(crossOrigin, cookies)
	crossOriginResponse := httptest.NewRecorder()
	server.router.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("cross-origin cookie status = %d, want 401", crossOriginResponse.Code)
	}

	token, err := server.core.CreateAuthToken(crossOrigin.Context(), user.Id)
	if err != nil {
		t.Fatal(err)
	}
	crossOriginBearer := httptest.NewRequest(http.MethodGet, "/test/whoami", nil)
	crossOriginBearer.Header.Set("Origin", "https://client.example")
	crossOriginBearer.Header.Set("Authorization", "Bearer "+token)
	addCookies(crossOriginBearer, cookies)
	crossOriginBearerResponse := httptest.NewRecorder()
	server.router.ServeHTTP(crossOriginBearerResponse, crossOriginBearer)
	if crossOriginBearerResponse.Code != http.StatusOK || crossOriginBearerResponse.Body.String() != user.Id {
		t.Fatalf("cross-origin bearer status/body = %d/%q", crossOriginBearerResponse.Code, crossOriginBearerResponse.Body.String())
	}
}

func TestBrowserCookieAuthenticationCanonicalizesDefaultPort(t *testing.T) {
	server := setupOAuthServer(t)
	server.config.Webserver.URL = "https://chatto.example:443"
	cookies, user := loginOAuthTestUser(t, server, "same-origin-default-port")
	server.router.GET("/test/default-port-whoami", func(c *gin.Context) {
		request := server.injectUserIntoContext(c)
		if authenticated := authctx.ForContext(request.Context()); authenticated != nil {
			c.String(http.StatusOK, authenticated.Id)
			return
		}
		c.Status(http.StatusUnauthorized)
	})

	request := httptest.NewRequest(http.MethodGet, "/test/default-port-whoami", nil)
	request.Header.Set("Origin", "https://chatto.example")
	addCookies(request, cookies)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != user.Id {
		t.Fatalf("same-origin status/body = %d/%q", response.Code, response.Body.String())
	}
}

func TestParseBrowserOrigin(t *testing.T) {
	tests := []struct {
		origin string
		valid  bool
	}{
		{"https://client.example", true},
		{"chatto://desktop", true},
		{"http://localhost:5173", true},
		{"null", false},
		{"https://client.example/path", false},
		{"https://client.example?query", false},
		{"https://client.example#fragment", false},
		{"https://user@client.example", false},
	}
	for _, test := range tests {
		t.Run(test.origin, func(t *testing.T) {
			_, valid := parseBrowserOrigin(test.origin)
			if valid != test.valid {
				t.Fatalf("valid = %v, want %v", valid, test.valid)
			}
		})
	}
}
