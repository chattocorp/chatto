package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"hmans.de/chatto/internal/config"
)

func TestCIMDDocumentForConfiguredPublicOIDCClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const baseURL = "https://chat.example"
	server := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: baseURL},
			Auth: config.AuthConfig{Providers: []config.AuthProviderConfig{{
				ID: "authling", Type: config.AuthProviderTypeOpenIDConnect,
				ClientID: baseURL + cimdPath,
			}}},
		},
		router: gin.New(),
	}
	server.setupCIMDRoutes()

	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, cimdPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var document cimdDocument
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.ClientID != baseURL+cimdPath || document.ClientName != "Chatto Server" || document.TokenEndpointAuthMethod != "none" || len(document.RedirectURIs) != 1 || document.RedirectURIs[0] != baseURL+"/auth/providers/authling/callback" {
		t.Fatalf("document = %#v", document)
	}
}

func TestFrontendCIMDDocumentUsesSeparateClientIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const baseURL = "https://chat.example"
	server := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: baseURL},
			Frontend:  config.FrontendConfig{AuthlingIssuer: "https://id.example"},
		},
		router: gin.New(),
	}
	server.setupCIMDRoutes()

	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, frontendCIMDPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var document cimdDocument
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.ClientID != baseURL+frontendCIMDPath || document.ClientName != "Chatto Web" || len(document.RedirectURIs) != 1 || document.RedirectURIs[0] != baseURL+accountDataCallbackPath {
		t.Fatalf("document = %#v", document)
	}
}

func TestCIMDDocumentIsNotPublishedWithoutMatchingClientID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://chat.example"},
			Auth: config.AuthConfig{Providers: []config.AuthProviderConfig{{
				ID: "authling", Type: config.AuthProviderTypeOpenIDConnect, ClientID: "conventional-client",
			}}},
		},
		router: gin.New(),
	}
	server.setupCIMDRoutes()

	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, cimdPath, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestPublicOIDCClientUsesTokenRequestParameters(t *testing.T) {
	issuer := newNoEmailOIDCIssuer(t, "https://chat.example"+cimdPath)
	defer issuer.Close()

	var provider oidcProvider
	if err := provider.init(issuer.URL(), "https://chat.example"+cimdPath, "", "https://chat.example/callback", []string{"openid"}); err != nil {
		t.Fatal(err)
	}
	if provider.oauth2Config.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
		t.Fatalf("AuthStyle = %d, want token request parameters", provider.oauth2Config.Endpoint.AuthStyle)
	}
}
