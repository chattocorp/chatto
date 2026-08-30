package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"hmans.de/chatto/internal/config"
)

// oauthAuthorizationServerMetadata is Chatto's RFC 8414 authorization-server
// document. It includes the MCP client and issuer capabilities required by the
// current MCP authorization specification.
type oauthAuthorizationServerMetadata struct {
	Issuer                                   string   `json:"issuer"`
	AuthorizationEndpoint                    string   `json:"authorization_endpoint"`
	TokenEndpoint                            string   `json:"token_endpoint"`
	ResponseTypesSupported                   []string `json:"response_types_supported"`
	GrantTypesSupported                      []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported            []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported        []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                          []string `json:"scopes_supported,omitempty"`
	ClientIDMetadataDocumentSupported        bool     `json:"client_id_metadata_document_supported"`
	AuthorizationResponseISSParameterSupport bool     `json:"authorization_response_iss_parameter_supported"`
}

func (s *HTTPServer) setupOAuthMetadataRoutes() {
	s.router.GET("/.well-known/oauth-authorization-server", func(c *gin.Context) {
		issuer := configuredWebserverOrigin(s.config.Webserver.URL)
		if issuer == "" {
			c.Status(http.StatusNotFound)
			return
		}
		scopes := []string(nil)
		if s.config.MCP.Enabled {
			scopes = config.MCPOAuthScopes()
		}
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cache-Control", "public, max-age=300")
		c.JSON(http.StatusOK, oauthAuthorizationServerMetadata{
			Issuer: issuer, AuthorizationEndpoint: issuer + "/oauth/authorize", TokenEndpoint: issuer + "/oauth/token",
			ResponseTypesSupported: []string{"code"}, GrantTypesSupported: []string{"authorization_code", "refresh_token"},
			CodeChallengeMethodsSupported: []string{"S256"}, TokenEndpointAuthMethodsSupported: []string{"none"},
			ScopesSupported: scopes, ClientIDMetadataDocumentSupported: true, AuthorizationResponseISSParameterSupport: true,
		})
	})
}
