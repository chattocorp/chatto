package core

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/core/remoteimage"
)

// This test does not call t.Parallel: it replaces the package-wide
// remoteImageClient.
func TestChattoCore_FetchRemoteImage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	pngData := createTestPNG(64, 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
		}
	}))
	defer server.Close()
	previousClient := remoteImageClient
	remoteImageClient = server.Client()
	t.Cleanup(func() { remoteImageClient = previousClient })

	data, err := core.FetchRemoteImage(ctx, server.URL+"/avatar.png")
	if err != nil {
		t.Fatalf("FetchRemoteImage: %v", err)
	}
	if string(data) != string(pngData) {
		t.Fatal("FetchRemoteImage changed the image bytes")
	}

	for name, rawURL := range map[string]string{
		"provider 404":    server.URL + "/missing",
		"not an image":    server.URL + "/page",
		"unsupported URL": "data:image/png;base64,AAAA",
	} {
		if _, err := core.FetchRemoteImage(ctx, rawURL); !errors.Is(err, remoteimage.ErrRejected) {
			t.Errorf("%s: err = %v, want ErrRejected", name, err)
		}
	}
}

// This test does not call t.Parallel: it replaces the package-wide
// remoteImageClient.
func TestChattoCore_FetchRemoteImageRemembersFailures(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	previousClient := remoteImageClient
	remoteImageClient = server.Client()
	t.Cleanup(func() { remoteImageClient = previousClient })

	broken := server.URL + "/broken.png"
	for attempt := range 3 {
		if _, err := core.FetchRemoteImage(ctx, broken); !errors.Is(err, remoteimage.ErrRejected) {
			t.Fatalf("attempt %d: err = %v, want ErrRejected", attempt, err)
		}
	}
	// Only the first attempt reaches the remote host; the marker stops the
	// others on every replica until it expires.
	if got := requests.Load(); got != 1 {
		t.Fatalf("remote requests = %d, want 1", got)
	}
	// The marker key does not contain the source URL.
	key := core.runtimeTokenKey(remoteImageFailureKeyPrefix, broken)
	if strings.Contains(key, "broken") {
		t.Fatalf("marker key %q contains the URL", key)
	}
	assertRuntimeKVHasTTL(t, core, key)

	// Another source is not affected.
	if _, err := core.FetchRemoteImage(ctx, server.URL+"/other.png"); !errors.Is(err, remoteimage.ErrRejected) || requests.Load() != 2 {
		t.Fatalf("other source: err = %v, requests = %d; want a remote request", err, requests.Load())
	}
}

func TestChattoCore_RemoteImagePathVerifies(t *testing.T) {
	core, _ := setupTestCore(t)
	const imageURL = "https://cdn.example.test/a.png"
	path := core.RemoteImagePath(remoteimage.PresetAvatar, imageURL)
	signature := remoteimage.Sign(core.config.Assets.SigningSecret, remoteimage.PresetAvatar, imageURL)
	if !strings.Contains(path, "/"+signature+"?") {
		t.Fatalf("path %q does not contain the signature", path)
	}
	if !core.VerifyRemoteImageSignature(remoteimage.PresetAvatar, imageURL, signature) {
		t.Fatal("signature does not verify")
	}
	if core.VerifyRemoteImageSignature(remoteimage.PresetBanner, imageURL, signature) {
		t.Fatal("signature verifies for another preset")
	}
	if core.VerifyRemoteImageSignature(remoteimage.PresetAvatar, imageURL+"?x", signature) {
		t.Fatal("signature verifies for another URL")
	}
}

func TestRemoteImageFetchErrorOmitsURL(t *testing.T) {
	// The production SSRF-safe client rejects loopback addresses, so the
	// fetch fails. The error must not contain the URL, which can identify
	// the account.
	avatarURL := "http://127.0.0.1:1/xrpc/com.atproto.sync.getBlob?did=did:plc:secret&cid=bafy"
	_, err := remoteimage.Fetch(t.Context(), remoteImageClient, avatarURL, remoteimage.MaxSourceBytes)
	if err == nil {
		t.Fatal("Fetch() error = nil, want fetch error")
	}
	if strings.Contains(err.Error(), "did:plc:secret") || strings.Contains(err.Error(), "127.0.0.1:1/xrpc") {
		t.Fatalf("Fetch() error = %q, must not contain the URL", err)
	}
}

func TestMediaModel_StoreCachedResizeIfAbsent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	const key = "remote_image.source-1.params"
	if err := core.StoreCachedResizeIfAbsent(ctx, key, []byte("first")); err != nil {
		t.Fatalf("first store: %v", err)
	}
	// An existing object is never replaced.
	if err := core.StoreCachedResizeIfAbsent(ctx, key, []byte("second")); err != nil {
		t.Fatalf("second store: %v", err)
	}
	if got, err := core.GetCachedResize(ctx, key); err != nil || string(got) != "first" {
		t.Fatalf("cached = %q, %v; want first", got, err)
	}
}
