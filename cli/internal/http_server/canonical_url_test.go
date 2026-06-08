package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"hmans.de/chatto/internal/config"
)

func newCanonicalRedirectTestRouter(webserverURL string) *gin.Engine {
	server := &HTTPServer{
		config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: webserverURL}},
	}
	router := gin.New()
	router.Use(server.canonicalRedirectMiddleware())
	router.Any("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "content")
	})
	return router
}

func TestCanonicalRedirectMiddlewareRedirectsAliasRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCanonicalRedirectTestRouter("https://chat.example.com")

	req := httptest.NewRequest("POST", "/api/graphql?operation=Ping", nil)
	req.Host = "alias.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	assert.Equal(t, "https://chat.example.com/api/graphql?operation=Ping", w.Header().Get("Location"))
}

func TestCanonicalRedirectMiddlewareExemptsHealthProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCanonicalRedirectTestRouter("https://chat.example.com")

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			req.Host = "alias.example.com"
			req.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
		})
	}
}

func TestCanonicalRedirectMiddlewareDoesNotExemptHealthProbePrefixes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newCanonicalRedirectTestRouter("https://chat.example.com")

	req := httptest.NewRequest("GET", "/healthz/details", nil)
	req.Host = "alias.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	assert.Equal(t, "https://chat.example.com/healthz/details", w.Header().Get("Location"))
}
