package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const changePasswordWellKnownPath = "/.well-known/change-password"
const changePasswordSettingsPath = "/chat/-/settings/account"

// setupPasswordManagementRoutes lets password managers find the account page
// for changing a password on this Chatto origin.
func (s *HTTPServer) setupPasswordManagementRoutes() {
	s.router.Match([]string{http.MethodGet, http.MethodHead}, changePasswordWellKnownPath, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Redirect(http.StatusFound, changePasswordSettingsPath)
	})
}
