package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFrontendATProtoClientMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		webserverURL string
		wantStatus   int
	}{{"https://chat.example", http.StatusOK}, {"http://localhost:4510", http.StatusNotFound}} {
		server := &HTTPServer{config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: tc.webserverURL}}, router: gin.New()}
		server.setupFrontendATProtoRoutes()
		host := strings.TrimPrefix(strings.TrimPrefix(tc.webserverURL, "https://"), "http://")
		request := httptest.NewRequest(http.MethodGet, frontendATProtoClientMetadataPath, nil)
		request.Host = host
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		require.Equal(t, tc.wantStatus, response.Code, tc.webserverURL)
		if tc.wantStatus != http.StatusOK {
			continue
		}
		var metadata oauth.ClientMetadata
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &metadata))
		clientID := "https://chat.example" + frontendATProtoClientMetadataPath
		require.NoError(t, metadata.Validate(clientID))
		require.Equal(t, clientID, metadata.ClientID)
		require.Equal(t, []string{"https://chat.example/atproto/callback"}, metadata.RedirectURIs)
		// The frontend may change only account link records.
		require.Equal(t, "atproto repo:run.chatto.accountLink rpc:run.chatto.signIn?aud=*", metadata.Scope)
	}
}

func TestFrontendATProtoLoopbackCallbackIsForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		webserverURL, host, want string
	}{
		{"http://chatto.dev.localhost:4510", "127.0.0.1:4510", "http://chatto.dev.localhost:4510/atproto/callback?state=s&code=c"},
		{"http://chatto.dev.localhost:4510", "chatto.dev.localhost:4510", ""},
		{"https://chat.example", "127.0.0.1:443", ""},
	} {
		server := &HTTPServer{config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: tc.webserverURL}}, router: gin.New()}
		server.setupFrontendATProtoRoutes()
		server.router.NoRoute(func(c *gin.Context) { c.String(http.StatusOK, "spa") })
		request := httptest.NewRequest(http.MethodGet, "/atproto/callback?state=s&code=c", nil)
		request.Host = tc.host
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if tc.want == "" {
			require.Equal(t, http.StatusOK, response.Code, tc.host)
			require.Equal(t, "spa", response.Body.String())
		} else {
			require.Equal(t, http.StatusTemporaryRedirect, response.Code, tc.host)
			require.Equal(t, tc.want, response.Header().Get("Location"))
		}
	}
}
