package connectapi

import "testing"

func TestCanonicalClientSyncServerURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://example.com",
		"https://EXAMPLE.com",
		"https://example.com:443/path?ignored=yes#fragment",
	} {
		canonical, err := canonicalClientSyncServerURL(raw)
		if err != nil {
			t.Fatalf("canonicalClientSyncServerURL(%q): %v", raw, err)
		}
		if canonical != "https://example.com" {
			t.Errorf("canonicalClientSyncServerURL(%q) = %q", raw, canonical)
		}
	}
}

func TestCanonicalClientSyncServerURLPreservesNonDefaultPort(t *testing.T) {
	t.Parallel()

	canonical, err := canonicalClientSyncServerURL("HTTP://EXAMPLE.com:8080/path")
	if err != nil {
		t.Fatalf("canonicalClientSyncServerURL: %v", err)
	}
	if canonical != "http://example.com:8080" {
		t.Fatalf("canonicalClientSyncServerURL = %q", canonical)
	}
}
