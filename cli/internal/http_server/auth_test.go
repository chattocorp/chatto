package http_server

import (
	"strings"
	"testing"
)

// TestIsValidInternalRedirect tests the OAuth redirect URL validation logic.
// This is a security test to prevent open redirect vulnerabilities.
func TestIsValidInternalRedirect(t *testing.T) {
	tests := []struct {
		name     string
		redirect string
		want     bool
	}{
		// Valid internal redirects
		{"simple relative path", "/chat", true},
		{"nested relative path", "/chat/settings/profile", true},
		{"relative with query params", "/chat?tab=settings", true},
		{"root path", "/", true},
		{"path with hash", "/chat#section", true},

		// Invalid redirects - absolute URLs
		{"https absolute URL", "https://evil.com", false},
		{"http absolute URL", "http://evil.com", false},
		{"ftp absolute URL", "ftp://evil.com", false},

		// Invalid redirects - protocol-relative URLs
		{"protocol-relative URL", "//evil.com", false},
		{"protocol-relative with path", "//evil.com/phishing", false},

		// Invalid redirects - no leading slash
		{"relative without slash", "chat", false},
		{"domain-like path", "evil.com/path", false},

		// Invalid redirects - backslash variants (browser normalization attacks)
		{"backslash at start", "\\evil.com", false},
		{"backslash in path", "/chat\\..\\evil", false},
		{"mixed slashes", "/path\\to\\evil", false},

		// Edge cases
		{"empty string", "", false},
		{"just double slash", "//", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidInternalRedirect(tt.redirect)
			if got != tt.want {
				t.Errorf("isValidInternalRedirect(%q) = %v, want %v", tt.redirect, got, tt.want)
			}
		})
	}
}

// TestIsValidLogin tests the login name validation logic.
func TestIsValidLogin(t *testing.T) {
	tests := []struct {
		name  string
		login string
		want  bool
	}{
		// Valid logins
		{"simple lowercase", "john", true},
		{"with numbers", "john123", true},
		{"with dot", "john.doe", true},
		{"multiple dots", "j.d.smith", true},
		{"with underscore", "john_doe", true},
		{"with dash", "john-doe", true},
		{"mixed", "john.doe_123-test", true},
		{"uppercase", "JohnDoe", true},
		{"starts with number", "123john", true},
		{"min length", "ab", true},
		{"max length", strings.Repeat("a", 32), true},

		// Invalid - consecutive periods
		{"consecutive periods middle", "john..doe", false},
		{"consecutive periods start", "..john", false},
		{"consecutive periods end", "john..", false},
		{"triple periods", "john...doe", false},
		{"trailing period", "john.", false},
		{"minimum length trailing period", "j.", false},

		// Invalid - length
		{"too short", "a", false},
		{"too long", strings.Repeat("a", 33), false},
		{"empty", "", false},

		// Invalid - characters
		{"with space", "john doe", false},
		{"with at sign", "john@doe", false},
		{"with exclamation", "john!", false},
		{"with hash", "john#doe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidLogin(tt.login)
			if got != tt.want {
				t.Errorf("isValidLogin(%q) = %v, want %v", tt.login, got, tt.want)
			}
		})
	}
}
