package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const clientConfigPath = "/client-config.json"

type clientConfiguration struct {
	Version  int                          `json:"version"`
	Authling *authlingClientConfiguration `json:"authling,omitempty"`
}

type authlingClientConfiguration struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"client_id"`
}

// setupClientConfigurationRoutes publishes trusted, non-secret configuration
// for the web client served by this origin. Standalone clients can publish the
// same JSON contract from their own trusted origin.
func (s *HTTPServer) setupClientConfigurationRoutes() {
	document := clientConfiguration{Version: 1}
	if s.config.Frontend.AuthlingIssuer != "" {
		baseURL := configuredWebserverOrigin(s.config.Webserver.URL)
		document.Authling = &authlingClientConfiguration{
			Issuer:   s.config.Frontend.AuthlingIssuer,
			ClientID: baseURL + frontendCIMDPath,
		}
	}

	s.router.GET(clientConfigPath, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, document)
	})
}
