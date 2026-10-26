package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPasswordRecoveryDisabledBeforeValidation(t *testing.T) {
	ts, client, _ := setupTestHTTPServerWithHook(t, func(server *HTTPServer) {
		server.config.Auth.DirectLogin = new(false)
	})
	for _, path := range []string{"forgot-password", "reset-password"} {
		t.Run(path, func(t *testing.T) {
			resp, err := client.Post(ts.URL+"/auth/"+path, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", resp.StatusCode)
			}
		})
	}
}

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
