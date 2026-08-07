package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

func TestClientConfigurationPublishesSelectedAuthling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &HTTPServer{
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://chat.example"},
			Frontend:  config.FrontendConfig{AuthlingIssuer: "https://id.example"},
		},
		router: gin.New(),
	}
	server.setupClientConfigurationRoutes()

	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, clientConfigPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var document clientConfiguration
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != 1 || document.Authling == nil || document.Authling.Issuer != "https://id.example" || document.Authling.ClientID != "https://chat.example"+frontendCIMDPath {
		t.Fatalf("document = %#v", document)
	}
}

func TestClientConfigurationOmitsAuthlingWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &HTTPServer{router: gin.New()}
	server.setupClientConfigurationRoutes()

	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, clientConfigPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var document clientConfiguration
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != 1 || document.Authling != nil {
		t.Fatalf("document = %#v", document)
	}
}
