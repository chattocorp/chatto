package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"

	"hmans.de/chatto/internal/config"
)

func TestOAuthAuthorizationServerMetadataAdvertisesMCPCompatibility(t *testing.T) {
	router := gin.New()
	server := &HTTPServer{
		router: router,
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://chat.example/some-path"},
			MCP:       config.MCPConfig{Enabled: true},
		},
	}
	server.setupOAuthMetadataRoutes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var metadata oauthAuthorizationServerMetadata
	if err := json.Unmarshal(recorder.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Issuer != "https://chat.example" || metadata.AuthorizationEndpoint != "https://chat.example/oauth/authorize" || metadata.TokenEndpoint != "https://chat.example/oauth/token" {
		t.Fatalf("metadata endpoints = %#v", metadata)
	}
	if !metadata.ClientIDMetadataDocumentSupported || !metadata.AuthorizationResponseISSParameterSupport {
		t.Fatalf("MCP OAuth capabilities are absent: %#v", metadata)
	}
	if !slices.Equal(metadata.ScopesSupported, config.MCPOAuthScopes()) {
		t.Fatalf("scopes = %v", metadata.ScopesSupported)
	}
}

func TestStandardRefreshRequestIDIsRandomUUIDv4Shape(t *testing.T) {
	first, err := newStandardRefreshRequestID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newStandardRefreshRequestID()
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("request IDs are equal: %q", first)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(first) {
		t.Fatalf("request ID %q is not a UUIDv4 shape", first)
	}
}
