package http_server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// instanceInfoResponse is the JSON response for GET /api/instance.
type instanceInfoResponse struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	AuthMethods      []string `json:"authMethods"`
	RegistrationOpen bool     `json:"registrationOpen"`
	WelcomeMessage   string   `json:"welcomeMessage,omitempty"`
	AuthorizeURL     string   `json:"authorizeUrl,omitempty"`
	OGImageURL       string   `json:"ogImageUrl,omitempty"`
}

// setupInstanceInfoRoutes registers the instance discovery endpoint.
// This endpoint is used by multi-instance clients to probe an instance
// before setting up a full GraphQL client.
func (s *HTTPServer) setupInstanceInfoRoutes() {
	s.router.GET("/api/instance", s.handleInstanceInfo)
	s.router.OPTIONS("/api/instance", s.handleInstanceInfoPreflight)
}

// setCORSHeaders sets CORS headers for the instance info endpoint.
// This endpoint needs to be accessible cross-origin for the "add instance" flow.
func setCORSHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
}

// handleInstanceInfo returns basic instance metadata for discovery.
// No authentication required — this is public information needed before login.
func (s *HTTPServer) handleInstanceInfo(c *gin.Context) {
	setCORSHeaders(c)
	c.Header("Cache-Control", "public, max-age=300")

	ctx := c.Request.Context()

	// Get instance name (defaults to "Chatto")
	name := "Chatto"
	if s.core != nil && s.core.ConfigManager() != nil {
		if n, err := s.core.ConfigManager().GetEffectiveInstanceName(ctx); err == nil {
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

	// Get OG image URL (used by the multi-instance "Add Server" preview to
	// show a banner before the user signs in). Matches the size used in the
	// app's OpenGraph metadata so the same transformed asset can be cached.
	//
	// The Core helper returns a relative URL when AssetBaseURL is unset
	// (i.e. when chatto.toml has no [webserver] url). Cross-origin clients
	// would resolve that against their own origin and 404, so absolutize
	// from the incoming request when needed.
	var ogImageURL string
	if s.core != nil {
		width, height := 1200, 630
		if u, err := s.core.GetInstanceOGImageURL(ctx, &width, &height); err == nil {
			ogImageURL = absolutizeAssetURL(c, u)
		}
	}

	c.JSON(http.StatusOK, instanceInfoResponse{
		Name:             name,
		Version:          s.version,
		AuthMethods:      authMethods,
		RegistrationOpen: s.config.Auth.DirectRegistrationOrDefault(),
		WelcomeMessage:   welcomeMessage,
		AuthorizeURL:     "/oauth/authorize",
		OGImageURL:       ogImageURL,
	})
}

// absolutizeAssetURL turns a relative asset path into a fully-qualified URL
// using the incoming request's scheme + host. No-op for empty strings and
// already-absolute URLs. Used so /api/instance returns absolute URLs to
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

// handleInstanceInfoPreflight responds to CORS preflight requests.
func (s *HTTPServer) handleInstanceInfoPreflight(c *gin.Context) {
	setCORSHeaders(c)
	c.Header("Access-Control-Max-Age", "86400")
	c.Status(http.StatusNoContent)
}
