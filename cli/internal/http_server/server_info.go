package http_server

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

// serverInfoResponse is the JSON response for GET /api/server.
type serverInfoResponse struct {
	Name             string                   `json:"name"`
	Version          string                   `json:"version"`
	AuthMethods      []string                 `json:"authMethods"`
	AuthProviders    []serverInfoAuthProvider `json:"authProviders"`
	RegistrationOpen bool                     `json:"registrationOpen"`
	WelcomeMessage   string                   `json:"welcomeMessage,omitempty"`
	AuthorizeURL     string                   `json:"authorizeUrl,omitempty"`
	Description      string                   `json:"description,omitempty"`
	IconURL          string                   `json:"iconUrl,omitempty"`
	BannerURL        string                   `json:"bannerUrl,omitempty"`
}

type serverInfoAuthProvider struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Label    string `json:"label"`
	LoginURL string `json:"loginUrl"`
}

// setupServerInfoRoutes registers the server discovery endpoint.
// This endpoint is used by multi-server clients to probe a server before
// setting up authenticated API clients.
func (s *HTTPServer) setupServerInfoRoutes() {
	s.router.GET("/api/server", s.handleServerInfo)
	s.router.OPTIONS("/api/server", s.handleServerInfoPreflight)
}

// setCORSHeaders sets CORS headers for the server info endpoint.
// This endpoint needs to be accessible cross-origin for the "add server" flow.
func setCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
}

// handleServerInfo returns basic server metadata for discovery.
// No authentication required — this is public information needed before login.
func (s *HTTPServer) handleServerInfo(c *gin.Context) {
	setCORSHeaders(c)
	c.Header("Cache-Control", "public, max-age=300")

	ctx := c.Request.Context()

	// Build compatibility auth methods list. Provider-specific IDs are exposed
	// through authProviders; authMethods stays method-oriented for older clients.
	authMethods := s.config.Auth.EnabledProviderMethods()
	if s.config.Auth.DirectRegistrationOrDefault() {
		authMethods = append([]string{"password"}, authMethods...)
	}
	if authMethods == nil {
		authMethods = []string{}
	}
	authProviders := serverInfoAuthProviders(s.config.Auth.PublicProviders())

	// Get welcome message
	var welcomeMessage string
	if s.core != nil && s.core.ConfigManager() != nil {
		if wm, err := s.core.ConfigManager().GetEffectiveWelcomeMessage(ctx); err == nil {
			welcomeMessage = wm
		}
	}

	// Server description (used in the "Add Server" preview alongside name/banner).
	var description string
	if s.core != nil && s.core.ConfigManager() != nil {
		if cfg, err := s.core.ConfigManager().GetServerConfig(ctx); err == nil && cfg != nil {
			description = cfg.Description
		}
	}

	// Banner doubles as the OG link-preview image at the canonical 1200×630.
	// The Core helper returns a relative URL when AssetBaseURL is unset
	// (i.e. when chatto.toml has no [webserver] url). Cross-origin clients
	// would resolve that against their own origin and 404, so absolutize
	// from the incoming request when needed.
	var bannerURL, iconURL string
	if s.core != nil {
		bw, bh := 1200, 630
		if u, err := s.core.GetServerBannerURL(ctx, &bw, &bh, "cover"); err == nil {
			bannerURL = s.absolutizeAssetURL(c, u)
		}
		lw, lh := 256, 256
		if u, err := s.core.GetServerLogoURL(ctx, &lw, &lh, "cover"); err == nil {
			iconURL = s.absolutizeAssetURL(c, u)
		}
	}

	c.JSON(http.StatusOK, serverInfoResponse{
		Name:             s.effectiveServerName(ctx),
		Version:          s.version,
		AuthMethods:      authMethods,
		AuthProviders:    authProviders,
		RegistrationOpen: s.config.Auth.DirectRegistrationOrDefault(),
		WelcomeMessage:   welcomeMessage,
		AuthorizeURL:     "/oauth/authorize",
		Description:      description,
		IconURL:          iconURL,
		BannerURL:        bannerURL,
	})
}

func (s *HTTPServer) effectiveServerName(ctx context.Context) string {
	if s.core != nil && s.core.ConfigManager() != nil {
		if n, err := s.core.ConfigManager().GetEffectiveServerName(ctx); err == nil {
			return n
		}
	}
	return "Chatto"
}

func serverInfoAuthProviders(providers []config.AuthProviderConfig) []serverInfoAuthProvider {
	result := make([]serverInfoAuthProvider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, serverInfoAuthProvider{
			ID:       provider.ID,
			Type:     provider.Type,
			Label:    provider.LabelOrDefault(),
			LoginURL: "/auth/providers/" + url.PathEscape(provider.ID),
		})
	}
	return result
}

// absolutizeAssetURL turns a relative asset path into a fully-qualified URL.
// Prefer the configured public webserver URL. When it is unset, fall back to
// the direct request scheme + host; forwarded headers are ignored until Chatto
// has explicit trusted-proxy configuration.
func (s *HTTPServer) absolutizeAssetURL(c *gin.Context, assetURL string) string {
	if assetURL == "" || strings.HasPrefix(assetURL, "http://") || strings.HasPrefix(assetURL, "https://") {
		return assetURL
	}
	if baseURL := configuredWebserverOrigin(s.config.Webserver.URL); baseURL != "" {
		return baseURL + assetURL
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + assetURL
}

// handleServerInfoPreflight responds to CORS preflight requests.
func (s *HTTPServer) handleServerInfoPreflight(c *gin.Context) {
	setCORSHeaders(c)
	c.Header("Access-Control-Max-Age", "86400")
	c.Status(http.StatusNoContent)
}
