package http_server

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

const cimdPath = "/oauth/client-metadata.json"
const frontendCIMDPath = "/oauth/frontend-client-metadata.json"
const popupCallbackPath = "/servers/callback?mode=popup"

type cimdDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	ApplicationType         string   `json:"application_type,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

// setupCIMDRoutes publishes metadata for OIDC providers configured to use
// this Chatto deployment's built-in Client ID Metadata Document URL.
func (s *HTTPServer) setupCIMDRoutes() {
	baseURL := configuredWebserverOrigin(s.config.Webserver.URL)
	clientID := baseURL + cimdPath
	redirects := make([]string, 0)
	for _, provider := range s.config.Auth.Providers {
		if provider.Type == config.AuthProviderTypeOpenIDConnect && provider.ClientID == clientID {
			redirects = append(redirects, s.providerCallbackURL(provider.ID))
		}
	}
	if len(redirects) > 0 {
		s.publishCIMD(cimdPath, cimdDocument{
			ClientID:                clientID,
			ClientName:              "Chatto Server",
			ClientURI:               baseURL,
			ApplicationType:         "web",
			RedirectURIs:            redirects,
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code"},
			ResponseTypes:           []string{"code"},
		})
	}

	s.router.GET(frontendCIMDPath, func(c *gin.Context) {
		origin, ok := s.frontendCIMDOrigin(c.Request.Host)
		if !ok {
			c.Status(http.StatusNotFound)
			return
		}
		s.writeCIMD(c, cimdDocument{
			ClientID:                origin + frontendCIMDPath,
			ClientName:              "Chatto Web",
			ClientURI:               origin,
			ApplicationType:         "web",
			RedirectURIs:            []string{origin + popupCallbackPath},
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
		})
	})
}

func (s *HTTPServer) publishCIMD(path string, document cimdDocument) {
	s.router.GET(path, func(c *gin.Context) {
		s.writeCIMD(c, document)
	})
}

func (s *HTTPServer) writeCIMD(c *gin.Context, document cimdDocument) {
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, document)
}

// frontendCIMDOrigin returns the configured public origin whose canonical host
// matches the request target. Exact allowed origins can therefore publish a
// self-consistent frontend identity without trusting arbitrary Host values.
func (s *HTTPServer) frontendCIMDOrigin(requestHost string) (string, bool) {
	hostURL, err := url.Parse("//" + requestHost)
	if err != nil || hostURL.Host == "" || hostURL.User != nil || hostURL.Path != "" || hostURL.RawQuery != "" || hostURL.Fragment != "" {
		return "", false
	}

	origins := make([]string, 0, len(s.config.Webserver.AllowedOrigins)+1)
	if origin := configuredWebserverOrigin(s.config.Webserver.URL); origin != "" {
		origins = append(origins, origin)
	}
	for _, raw := range s.config.Webserver.AllowedOrigins {
		if originURL, ok := parseBrowserOrigin(raw); ok {
			origins = append(origins, canonicalOrigin(originURL))
		}
	}
	matchedOrigin := ""
	for _, origin := range origins {
		originURL, _ := url.Parse(origin)
		requestOriginURL := *hostURL
		requestOriginURL.Scheme = originURL.Scheme
		if canonicalOrigin(&requestOriginURL) == origin {
			if matchedOrigin != "" && matchedOrigin != origin {
				return "", false
			}
			matchedOrigin = origin
		}
	}
	return matchedOrigin, matchedOrigin != ""
}
