package http_server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

const cimdPath = "/oauth/client-metadata.json"

type cimdDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// setupCIMDRoutes publishes metadata for OIDC providers configured to use
// this Chatto deployment's built-in Client ID Metadata Document URL.
func (s *HTTPServer) setupCIMDRoutes() {
	baseURL := strings.TrimRight(s.config.Webserver.URL, "/")
	clientID := baseURL + cimdPath
	redirects := make([]string, 0)
	for _, provider := range s.config.Auth.Providers {
		if provider.Type == config.AuthProviderTypeOpenIDConnect && provider.ClientID == clientID {
			redirects = append(redirects, s.providerCallbackURL(provider.ID))
		}
	}
	if len(redirects) == 0 {
		return
	}

	document := cimdDocument{
		ClientID:                clientID,
		ClientName:              "Chatto",
		ClientURI:               baseURL,
		RedirectURIs:            redirects,
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
	}
	s.router.GET(cimdPath, func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=300")
		c.JSON(http.StatusOK, document)
	})
}
