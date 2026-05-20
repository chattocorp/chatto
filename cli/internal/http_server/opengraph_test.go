package http_server

import (
	"strings"
	"testing"
)

func TestOpenGraphMetaGenerateTags(t *testing.T) {
	tests := []struct {
		name     string
		meta     OpenGraphMeta
		contains []string
		excludes []string
	}{
		{
			name: "basic meta tags",
			meta: OpenGraphMeta{
				Title:       "Test Title",
				Description: "Test Description",
				URL:         "https://example.com/test",
				Type:        "website",
				SiteName:    "Test Site",
			},
			contains: []string{
				`og:title" content="Test Title"`,
				`og:description" content="Test Description"`,
				`og:url" content="https://example.com/test"`,
				`og:type" content="website"`,
				`og:site_name" content="Test Site"`,
				`twitter:card" content="summary"`,
				`twitter:title" content="Test Title"`,
				`twitter:description" content="Test Description"`,
			},
			excludes: []string{
				`og:image`,
				`og:logo`,
				`twitter:image`,
			},
		},
		{
			name: "with image",
			meta: OpenGraphMeta{
				Title:       "Space Name",
				Description: "Space Description",
				URL:         "https://example.com/chat/abc123",
				Type:        "website",
				SiteName:    "My Instance",
				Image:       "https://example.com/assets/logo.png",
			},
			contains: []string{
				`og:image" content="https://example.com/assets/logo.png"`,
				`og:logo" content="https://example.com/assets/logo.png"`,
				`twitter:card" content="summary_large_image"`,
				`twitter:image" content="https://example.com/assets/logo.png"`,
			},
		},
		{
			name: "with canonical URL for remote server",
			meta: OpenGraphMeta{
				Title:        "My Instance",
				Description:  "Real-time chat application",
				URL:          "https://other.host.com/chat/-/S12345",
				Type:         "website",
				SiteName:     "My Instance",
				CanonicalURL: "https://other.host.com/chat/-/S12345",
			},
			contains: []string{
				`<link rel="canonical" href="https://other.host.com/chat/-/S12345" />`,
				`og:url" content="https://other.host.com/chat/-/S12345"`,
			},
		},
		{
			name: "no canonical URL for local space",
			meta: OpenGraphMeta{
				Title:       "Space Name | My Instance",
				Description: "A cool space",
				URL:         "https://example.com/chat/-/abc123",
				Type:        "website",
				SiteName:    "My Instance",
			},
			contains: []string{
				`og:title" content="Space Name | My Instance"`,
			},
			excludes: []string{
				`rel="canonical"`,
			},
		},
		{
			name: "HTML escaping",
			meta: OpenGraphMeta{
				Title:       `Title with "quotes" & <tags>`,
				Description: `Description with "special" chars`,
				URL:         "https://example.com",
				Type:        "website",
				SiteName:    `Site & Name`,
			},
			contains: []string{
				`&#34;quotes&#34;`, // html.EscapeString uses &#34; for quotes
				`&amp;`,
				`&lt;tags&gt;`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.meta.generateTags()

			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("generateTags() missing expected content: %q\nGot: %s", want, result)
				}
			}

			for _, exclude := range tt.excludes {
				if strings.Contains(result, exclude) {
					t.Errorf("generateTags() should not contain: %q\nGot: %s", exclude, result)
				}
			}
		})
	}
}

func TestRoutePatternMatching(t *testing.T) {
	tests := []struct {
		path              string
		expectServerSeg string
		expectSpaceID     string
	}{
		{"/chat/-/abc123", "-", "abc123"},
		{"/chat/-/abc123/rooms", "-", "abc123"},
		{"/chat/-/abc123/rooms/general", "-", "abc123"},
		{"/chat/myinstance/my-space_123", "myinstance", "my-space_123"},
		{"/chat/other.host.com/S12345", "other.host.com", "S12345"},
		{"/chat/abc123", "", ""},  // missing instance segment — no match
		{"/join/xyz789", "", ""},  // /join/* removed — no longer matched
		{"/login", "", ""},
		{"/register", "", ""},
		{"/", "", ""},
		{"/chat", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var serverSeg, spaceID string

			if matches := spaceRoutePattern.FindStringSubmatch(tt.path); len(matches) > 2 {
				serverSeg = matches[1]
				spaceID = matches[2]
			}

			if serverSeg != tt.expectServerSeg {
				t.Errorf("path %q: got serverSeg %q, want %q", tt.path, serverSeg, tt.expectServerSeg)
			}
			if spaceID != tt.expectSpaceID {
				t.Errorf("path %q: got spaceID %q, want %q", tt.path, spaceID, tt.expectSpaceID)
			}
		})
	}
}

func TestIsSpecialRoute(t *testing.T) {
	tests := []struct {
		segment  string
		expected bool
	}{
		{"admin", true},
		{"settings", true},
		{"spaces", true},
		{"dm", true},
		{"abc123", false},
		{"my-space", false},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			if got := isSpecialRoute(tt.segment); got != tt.expected {
				t.Errorf("isSpecialRoute(%q) = %v, want %v", tt.segment, got, tt.expected)
			}
		})
	}
}

