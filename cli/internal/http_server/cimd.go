package http_server

import (
	"net/http"
	"strings"
	"net/url"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/gin-gonic/gin"
)

const cimdPath = "/oauth/client-metadata.json"
const frontendCIMDPath = "/oauth/frontend-client-metadata.json"
const popupCallbackPath = "/servers/callback?mode=popup"

// frontendATProtoClientMetadataPath publishes the bundled frontend's AT
// Protocol OAuth client metadata. Its URL is the frontend's client ID.
const frontendATProtoClientMetadataPath = "/oauth/atproto-client-metadata.json"

// frontendATProtoCallbackPath is the frontend route that completes an AT
// Protocol sign-in.
const frontendATProtoCallbackPath = "/atproto/callback"

// frontendATProtoScope lets the frontend prove its user's identity and
// change only Chatto account link records (ADR-123). It must not grant write
// access to other collections.
const frontendATProtoScope = "atproto repo:run.chatto.accountLink"

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
		origin, ok := s.configuredOriginForHost(c.Request.Host)
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

// atprotoClientMetadataDocument is the AT Protocol form of client metadata.
// It extends the OAuth fields with scope and DPoP binding.
type atprotoClientMetadataDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	ApplicationType         string   `json:"application_type"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	DPoPBoundAccessTokens   bool     `json:"dpop_bound_access_tokens"`
}

// setupFrontendATProtoRoutes publishes the frontend's AT Protocol client
// metadata for each configured HTTPS origin. Plain HTTP origins use AT
// Protocol's built-in loopback client instead.
func (s *HTTPServer) setupFrontendATProtoRoutes() {
	s.router.GET(frontendATProtoClientMetadataPath, func(c *gin.Context) {
		origin, ok := s.configuredOriginForHost(c.Request.Host)
		if !ok || !strings.HasPrefix(origin, "https://") {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=300")
		c.JSON(http.StatusOK, atprotoClientMetadataDocument{
			ClientID:                origin + frontendATProtoClientMetadataPath,
			ClientName:              "Chatto Web",
			ClientURI:               origin,
			ApplicationType:         "web",
			RedirectURIs:            []string{origin + frontendATProtoCallbackPath},
			Scope:                   frontendATProtoScope,
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			DPoPBoundAccessTokens:   true,
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

// configuredOriginForHost returns the configured public origin whose
// canonical host matches the request target: webserver.url or an exact
// webserver.allowed_origins entry. The configured scheme applies, so the
// result stays correct behind a TLS-terminating proxy. It reports false for
// an unknown host, so an arbitrary Host value never selects the origin. It
// also reports false when the host matches more than one configured origin.
func (s *HTTPServer) configuredOriginForHost(requestHost string) (string, bool) {
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
