package http_server

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"time"
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

// ogMetaCache provides a simple TTL cache for space metadata.
type ogMetaCache struct {
	mu      sync.RWMutex
	entries map[string]ogCacheEntry
	ttl     time.Duration
}

type ogCacheEntry struct {
	meta      *OpenGraphMeta
	expiresAt time.Time
}

func newOGMetaCache(ttl time.Duration) *ogMetaCache {
	return &ogMetaCache{
		entries: make(map[string]ogCacheEntry),
		ttl:     ttl,
	}
}

func (c *ogMetaCache) get(key string) (*OpenGraphMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.meta, true
}

func (c *ogMetaCache) set(key string, meta *OpenGraphMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = ogCacheEntry{
		meta:      meta,
		expiresAt: time.Now().Add(c.ttl),
	}
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

	// Get instance name for site_name (always instance name, not custom OG title)
	instanceName := "Chatto"
	if s.core != nil && s.core.ConfigManager() != nil {
		if name, err := s.core.ConfigManager().GetEffectiveInstanceName(ctx); err == nil && name != "" {
			instanceName = name
		}
	}

	// Get custom OG title and description (with fallbacks)
	ogTitle := instanceName
	ogDescription := "Real-time chat application"
	if s.core != nil && s.core.ConfigManager() != nil {
		if title, err := s.core.ConfigManager().GetEffectiveOGTitle(ctx); err == nil && title != "" {
			ogTitle = title
		}
		if desc, err := s.core.ConfigManager().GetEffectiveOGDescription(ctx); err == nil && desc != "" {
			ogDescription = desc
		}
	}

	// Get instance OG image for default metadata
	var defaultImage string
	if s.core != nil {
		width, height := 1200, 630
		ogImageURL, err := s.core.GetInstanceOGImageURL(ctx, &width, &height)
		if err == nil && ogImageURL != "" {
			defaultImage = ogImageURL
		}
	}

	// Default metadata (for /, /login, /register, etc.)
	defaultMeta := &OpenGraphMeta{
		Title:       ogTitle,
		Description: ogDescription,
		Image:       defaultImage,
		URL:         baseURL + urlPath,
		Type:        "website",
		SiteName:    instanceName,
	}

	// Check for space routes: /chat/{instanceSegment}/{spaceId}/*
	var instanceSegment, spaceID string
	if matches := spaceRoutePattern.FindStringSubmatch(urlPath); len(matches) > 2 {
		instanceSegment = matches[1]
		spaceID = matches[2]
	}

	// Skip special space IDs that are actually other routes
	if spaceID != "" && !isSpecialRoute(spaceID) {
		// Remote instance: we can't look up the space locally, but we can
		// point crawlers to the canonical URL on the instance that owns it.
		if instanceSegment != "" && instanceSegment != "-" {
			canonicalURL := fmt.Sprintf("https://%s/chat/-/%s", instanceSegment, spaceID)
			defaultMeta.CanonicalURL = canonicalURL
			defaultMeta.URL = canonicalURL
			return defaultMeta
		}

		if meta := s.getSpaceOpenGraphMeta(ctx, spaceID, urlPath, baseURL, instanceName); meta != nil {
			return meta
		}
	}

	return defaultMeta
}

// isSpecialRoute returns true for route segments that look like spaceIds but aren't.
func isSpecialRoute(segment string) bool {
	specialRoutes := map[string]bool{
		"admin":    true,
		"settings": true,
		"spaces":   true,
		"dm":       true,
	}
	return specialRoutes[segment]
}

// getSpaceOpenGraphMeta fetches space-specific metadata with caching.
func (s *HTTPServer) getSpaceOpenGraphMeta(ctx context.Context, spaceID, urlPath, baseURL, instanceName string) *OpenGraphMeta {
	// Check cache first
	cacheKey := "space:" + spaceID
	if cached, ok := s.ogCache.get(cacheKey); ok {
		// Clone and update URL for this specific path
		meta := *cached
		meta.URL = baseURL + urlPath
		return &meta
	}

	// Fetch from Core (no auth required for GetSpace)
	space, err := s.core.GetSpace(ctx, spaceID)
	if err != nil {
		return nil // Space not found, use default
	}

	// Build metadata
	title := space.Name
	if instanceName != "" && instanceName != space.Name {
		title = space.Name + " | " + instanceName
	}

	description := space.Description
	if description == "" {
		description = fmt.Sprintf("Join %s on %s", space.Name, instanceName)
	}

	// Get space banner for og:image (1200x630 is optimal for social sharing),
	// falling back to space logo if no banner is set.
	var imageURL string
	width, height := 1200, 630
	bannerURL, err := s.core.GetSpaceBannerURL(ctx, spaceID, &width, &height)
	if err == nil && bannerURL != "" {
		imageURL = bannerURL
	} else {
		logoURL, err := s.core.GetSpaceLogoURL(ctx, spaceID, &width, &height)
		if err == nil && logoURL != "" {
			imageURL = logoURL
		}
	}

	meta := &OpenGraphMeta{
		Title:       title,
		Description: description,
		Image:       imageURL,
		URL:         baseURL + urlPath,
		Type:        "website",
		SiteName:    instanceName,
	}

	// Cache the result (without URL, as that changes per-path)
	cacheMeta := *meta
	s.ogCache.set(cacheKey, &cacheMeta)

	return meta
}

// injectOpenGraphTags replaces the OG placeholder in HTML content with actual meta tags.
func (s *HTTPServer) injectOpenGraphTags(ctx context.Context, content []byte, urlPath string) []byte {
	ogMeta := s.getOpenGraphMeta(ctx, urlPath)
	ogTags := ogMeta.generateTags()
	return bytes.Replace(content, []byte(ogPlaceholder), []byte(ogTags), 1)
}
