package http_server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// serverInfoResponse is the JSON response for GET /api/server.
type serverInfoResponse struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	AuthMethods      []string `json:"authMethods"`
	RegistrationOpen bool     `json:"registrationOpen"`
	WelcomeMessage   string   `json:"welcomeMessage,omitempty"`
	AuthorizeURL     string   `json:"authorizeUrl,omitempty"`
	Description      string   `json:"description,omitempty"`
	IconURL          string   `json:"iconUrl,omitempty"`
	BannerURL        string   `json:"bannerUrl,omitempty"`
}

// setupServerInfoRoutes registers the server discovery endpoint.
// This endpoint is used by multi-server clients to probe a server
// before setting up a full GraphQL client.
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

	// Get server name (defaults to "Chatto")
	name := "Chatto"
	if s.core != nil && s.core.ConfigManager() != nil {
		if n, err := s.core.ConfigManager().GetEffectiveServerName(ctx); err == nil {
			name = n
		}
	}

	// Build auth methods list
	authMethods := s.config.Auth.EnabledProviders()
	if s.config.Auth.DirectRegistrationOrDefault() {
		authMethods = append([]string{"password"}, authMethods...)
	}
	if authMethods == nil {
		authMethods = []string{}
	}

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
		if cfg, _, err := s.core.ConfigManager().GetServerConfig(ctx); err == nil && cfg != nil {
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
		if u, err := s.core.GetServerBannerURL(ctx, &bw, &bh); err == nil {
			bannerURL = absolutizeAssetURL(c, u)
		}
		lw, lh := 256, 256
		if u, err := s.core.GetServerLogoURL(ctx, &lw, &lh); err == nil {
			iconURL = absolutizeAssetURL(c, u)
		}
	}

	c.JSON(http.StatusOK, serverInfoResponse{
		Name:             name,
		Version:          s.version,
		AuthMethods:      authMethods,
		RegistrationOpen: s.config.Auth.DirectRegistrationOrDefault(),
		WelcomeMessage:   welcomeMessage,
		AuthorizeURL:     "/oauth/authorize",
		Description:      description,
		IconURL:          iconURL,
		BannerURL:        bannerURL,
	})
}

// absolutizeAssetURL turns a relative asset path into a fully-qualified URL
// using the incoming request's scheme + host. No-op for empty strings and
// already-absolute URLs. Used so /api/server returns absolute URLs to
// cross-origin clients that would otherwise resolve relative paths against
// their own origin.
func absolutizeAssetURL(c *gin.Context, assetURL string) string {
	if assetURL == "" || strings.HasPrefix(assetURL, "http://") || strings.HasPrefix(assetURL, "https://") {
		return assetURL
	}
	scheme := "http"
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if c.Request.TLS != nil {
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
