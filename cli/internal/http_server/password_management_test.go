package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChangePasswordWellKnownURLRedirectsToOriginAccountSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &HTTPServer{router: gin.New()}
	server.setupPasswordManagementRoutes()
	if err := server.setupFrontendRoutes(); err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			request := httptest.NewRequest(method, "https://chat.example"+changePasswordWellKnownPath, nil)
			response := httptest.NewRecorder()
			server.router.ServeHTTP(response, request)

			if response.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusFound)
			}
			if got := response.Header().Get("Location"); got != changePasswordSettingsPath {
				t.Fatalf("Location = %q, want %q", got, changePasswordSettingsPath)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}
