package connectapi

import "testing"

func TestCanonicalPersonalServerURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://example.com",
		"https://EXAMPLE.com",
		"https://example.com:443/path?ignored=yes#fragment",
	} {
		canonical, err := canonicalPersonalServerURL(raw)
		if err != nil {
			t.Fatalf("canonicalPersonalServerURL(%q): %v", raw, err)
		}
		if canonical != "https://example.com" {
			t.Errorf("canonicalPersonalServerURL(%q) = %q", raw, canonical)
		}
	}
}

func TestCanonicalPersonalServerURLPreservesNonDefaultPort(t *testing.T) {
	t.Parallel()

	canonical, err := canonicalPersonalServerURL("HTTP://EXAMPLE.com:8080/path")
	if err != nil {
		t.Fatalf("canonicalPersonalServerURL: %v", err)
	}
	if canonical != "http://example.com:8080" {
		t.Fatalf("canonicalPersonalServerURL = %q", canonical)
	}
}
