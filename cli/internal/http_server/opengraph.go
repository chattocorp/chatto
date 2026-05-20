package http_server

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
)

const ogPlaceholder = "<!-- OG_META_PLACEHOLDER -->"

// OpenGraphMeta holds metadata for OpenGraph tags.
type OpenGraphMeta struct {
	Title        string
	Description  string
	Image        string
	URL          string
	Type         string // "website" for all pages
	SiteName     string
	CanonicalURL string // set when the content lives on a different instance
}

// Regex patterns for route matching.
var (
	spaceRoutePattern = regexp.MustCompile(`^/chat/([a-zA-Z0-9._-]+)/([a-zA-Z0-9_-]+)(?:/.*)?$`)
)

// generateTags returns the HTML string for OpenGraph meta tags.
func (meta *OpenGraphMeta) generateTags() string {
	var sb strings.Builder

	// Core OpenGraph tags
	sb.WriteString(fmt.Sprintf(`<meta property="og:title" content="%s" />`, html.EscapeString(meta.Title)))
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta property="og:description" content="%s" />`, html.EscapeString(meta.Description)))
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta property="og:url" content="%s" />`, html.EscapeString(meta.URL)))
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta property="og:type" content="%s" />`, html.EscapeString(meta.Type)))
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta property="og:site_name" content="%s" />`, html.EscapeString(meta.SiteName)))

	// Image tag (only if we have one)
	if meta.Image != "" {
		sb.WriteString("\n\t\t")
		sb.WriteString(fmt.Sprintf(`<meta property="og:image" content="%s" />`, html.EscapeString(meta.Image)))
	}

	// Logo tag (non-standard, but some validators check for it)
	if meta.Image != "" {
		sb.WriteString("\n\t\t")
		sb.WriteString(fmt.Sprintf(`<meta property="og:logo" content="%s" />`, html.EscapeString(meta.Image)))
	}

	// Twitter Card tags
	if meta.Image != "" {
		sb.WriteString("\n\t\t")
		sb.WriteString(`<meta name="twitter:card" content="summary_large_image" />`)
	} else {
		sb.WriteString("\n\t\t")
		sb.WriteString(`<meta name="twitter:card" content="summary" />`)
	}
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta name="twitter:title" content="%s" />`, html.EscapeString(meta.Title)))
	sb.WriteString("\n\t\t")
	sb.WriteString(fmt.Sprintf(`<meta name="twitter:description" content="%s" />`, html.EscapeString(meta.Description)))

	if meta.Image != "" {
		sb.WriteString("\n\t\t")
		sb.WriteString(fmt.Sprintf(`<meta name="twitter:image" content="%s" />`, html.EscapeString(meta.Image)))
	}

	// Canonical URL for content that lives on a different instance
	if meta.CanonicalURL != "" {
		sb.WriteString("\n\t\t")
		sb.WriteString(fmt.Sprintf(`<link rel="canonical" href="%s" />`, html.EscapeString(meta.CanonicalURL)))
	}

	return sb.String()
}

// getOpenGraphMeta determines the appropriate OpenGraph metadata for a URL path.
func (s *HTTPServer) getOpenGraphMeta(ctx context.Context, urlPath string) *OpenGraphMeta {
	baseURL := strings.TrimSuffix(s.config.Webserver.URL, "/")

	// Get server name for both site_name and og:title — server identity
	// is the single source of truth for link previews.
	serverName := "Chatto"
	description := "Come join our community!"
	if s.core != nil && s.core.ConfigManager() != nil {
		if name, err := s.core.ConfigManager().GetEffectiveServerName(ctx); err == nil && name != "" {
			serverName = name
		}
		if desc, err := s.core.ConfigManager().GetEffectiveDescription(ctx); err == nil && desc != "" {
			description = desc
		}
	}

	// The server banner doubles as the OG link-preview image.
	var defaultImage string
	if s.core != nil {
		width, height := 1200, 630
		bannerURL, err := s.core.GetServerBannerURL(ctx, &width, &height)
		if err == nil && bannerURL != "" {
			defaultImage = bannerURL
		}
	}

	// Default metadata (for /, /login, /register, etc.)
	defaultMeta := &OpenGraphMeta{
		Title:       serverName,
		Description: description,
		Image:       defaultImage,
		URL:         baseURL + urlPath,
		Type:        "website",
		SiteName:    serverName,
	}

	// Check for space routes: /chat/{serverSegment}/{spaceId}/*
	var serverSegment, spaceID string
	if matches := spaceRoutePattern.FindStringSubmatch(urlPath); len(matches) > 2 {
		serverSegment = matches[1]
		spaceID = matches[2]
	}

	// Remote server: we can't look up the resource locally, but we can
	// point crawlers to the canonical URL on the server that owns it.
	if spaceID != "" && !isSpecialRoute(spaceID) && serverSegment != "" && serverSegment != "-" {
		canonicalURL := fmt.Sprintf("https://%s/chat/-/%s", serverSegment, spaceID)
		defaultMeta.CanonicalURL = canonicalURL
		defaultMeta.URL = canonicalURL
		return defaultMeta
	}

	return defaultMeta
}

// isSpecialRoute returns true for route segments that look like room IDs but aren't.
func isSpecialRoute(segment string) bool {
	specialRoutes := map[string]bool{
		"admin":    true,
		"settings": true,
		"spaces":   true,
		"dm":       true,
	}
	return specialRoutes[segment]
}

// injectOpenGraphTags replaces the OG placeholder in HTML content with actual meta tags.
func (s *HTTPServer) injectOpenGraphTags(ctx context.Context, content []byte, urlPath string) []byte {
	ogMeta := s.getOpenGraphMeta(ctx, urlPath)
	ogTags := ogMeta.generateTags()
	return bytes.Replace(content, []byte(ogPlaceholder), []byte(ogTags), 1)
}
