package remoteimage

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return buf.Bytes()
}

func TestFetch(t *testing.T) {
	pngData := testPNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
		case "/untyped":
			_, _ = w.Write(pngData)
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html></html>"))
		case "/large":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(bytes.Repeat([]byte("x"), 101))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	client := server.Client()

	data, err := Fetch(ctx, client, server.URL+"/image.png", MaxSourceBytes)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !bytes.Equal(data, pngData) {
		t.Fatal("Fetch changed the image bytes")
	}
	if _, err := Fetch(ctx, client, server.URL+"/untyped", MaxSourceBytes); err != nil {
		t.Fatalf("Fetch without a declared type: %v", err)
	}

	rejected := map[string]struct {
		url      string
		maxBytes int64
	}{
		"not found":     {server.URL + "/missing", MaxSourceBytes},
		"not an image":  {server.URL + "/page", MaxSourceBytes},
		"too large":     {server.URL + "/large", 100},
		"other scheme":  {"ftp://example.test/a.png", MaxSourceBytes},
		"relative URL":  {"/a.png", MaxSourceBytes},
		"credentials":   {strings.Replace(server.URL, "://", "://user:pass@", 1) + "/image.png", MaxSourceBytes},
		"malformed URL": {"http://[::1", MaxSourceBytes},
	}
	for name, tc := range rejected {
		_, err := Fetch(ctx, client, tc.url, tc.maxBytes)
		if !errors.Is(err, ErrRejected) {
			t.Errorf("%s: err = %v, want ErrRejected", name, err)
		}
		if err != nil && strings.Contains(err.Error(), server.URL) {
			t.Errorf("%s: error contains the source URL: %v", name, err)
		}
	}
}

func TestSignedPath(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	const source = "https://cdn.example.test/a b.png?size=2&x=1"

	path := Path(secret, PresetBanner, source)
	parsed, err := url.Parse(path)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	prefix := RoutePrefix + string(PresetBanner) + "/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		t.Fatalf("path = %q, want prefix %q", parsed.Path, prefix)
	}
	signature := strings.TrimPrefix(parsed.Path, prefix)
	if got := parsed.Query().Get("url"); got != source {
		t.Fatalf("url parameter = %q, want %q", got, source)
	}
	if !Verify(secret, PresetBanner, source, signature) {
		t.Fatal("signature does not verify")
	}
	for name, ok := range map[string]bool{
		"other preset": Verify(secret, PresetAvatar, source, signature),
		"other URL":    Verify(secret, PresetBanner, source+"&y=2", signature),
		"other secret": Verify(secret+"x", PresetBanner, source, signature),
		"bad encoding": Verify(secret, PresetBanner, source, "!!"),
	} {
		if ok {
			t.Errorf("%s: signature verifies", name)
		}
	}
	if Path(secret, PresetBanner, source) != path {
		t.Fatal("Path is not stable")
	}
}

func TestPresets(t *testing.T) {
	for name, want := range map[string]Size{
		"avatar": {Width: 256, Height: 256, Fit: "contain"},
		"banner": {Width: 1200, Height: 630, Fit: "contain"},
	} {
		preset, ok := ParsePreset(name)
		if !ok {
			t.Fatalf("ParsePreset(%q) failed", name)
		}
		if preset.Size() != want {
			t.Errorf("%s size = %+v, want %+v", name, preset.Size(), want)
		}
	}
	if _, ok := ParsePreset("original"); ok {
		t.Error("ParsePreset accepted an unknown preset")
	}
	id := SourceID("https://cdn.example.test/a.png")
	if len(id) != 64 || strings.Contains(id, "example") {
		t.Errorf("SourceID = %q", id)
	}
}

func TestFetchErrorsArePrivate(t *testing.T) {
	// A malformed redirect location makes net/http quote the location in its
	// error. The location here carries an account identifier.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://[::1/xrpc/getBlob?did=did:plc:secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	_, err := Fetch(context.Background(), server.Client(), server.URL+"/avatar.png", MaxSourceBytes)
	if err == nil {
		t.Fatal("Fetch followed a malformed redirect")
	}
	if strings.Contains(err.Error(), "did:plc:secret") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("redirect error = %q, must not contain the location or URL", err)
	}

	// A failed connection names the address in the transport error.
	_, err = Fetch(context.Background(), http.DefaultClient, "http://127.0.0.1:1/avatar.png?did=did:plc:secret", MaxSourceBytes)
	if err == nil {
		t.Fatal("Fetch reached a closed port")
	}
	if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("connection error = %q, must not contain the address", err)
	}
	if err.Error() != "fetch remote image: network error" {
		t.Fatalf("connection error = %q, want the network error category", err)
	}

	// The cause stays available for classification.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Fetch(ctx, http.DefaultClient, "http://127.0.0.1:1/avatar.png", MaxSourceBytes)
	if !errors.Is(err, context.Canceled) || err.Error() != "fetch remote image: canceled" {
		t.Fatalf("canceled fetch error = %v", err)
	}
}
