package core

import (
	"strings"
	"testing"
)

func TestImportUserAvatarFromURLErrorOmitsURL(t *testing.T) {
	// The SSRF-safe client rejects loopback addresses, so the fetch fails
	// before the core is used.
	avatarURL := "http://127.0.0.1:1/xrpc/com.atproto.sync.getBlob?did=did:plc:secret&cid=bafy"
	err := (&ChattoCore{}).ImportUserAvatarFromURL(t.Context(), "user-1", avatarURL)
	if err == nil {
		t.Fatal("ImportUserAvatarFromURL() error = nil, want fetch error")
	}
	if strings.Contains(err.Error(), "did:plc:secret") || strings.Contains(err.Error(), "127.0.0.1:1/xrpc") {
		t.Fatalf("ImportUserAvatarFromURL() error = %q, must not contain the URL", err)
	}
}
