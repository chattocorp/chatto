package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestAssetVersionTracksNamesAndContents(t *testing.T) {
	fixtures := []struct {
		name  string
		files fstest.MapFS
		same  bool
	}{
		{"identical", fstest.MapFS{"a.css": {Data: []byte("a")}, "b.woff2": {Data: []byte("b")}}, true},
		{"changed bytes", fstest.MapFS{"a.css": {Data: []byte("changed")}, "b.woff2": {Data: []byte("b")}}, false},
		{"renamed file", fstest.MapFS{"renamed.css": {Data: []byte("a")}, "b.woff2": {Data: []byte("b")}}, false},
		{"removed file", fstest.MapFS{"a.css": {Data: []byte("a")}}, false},
	}
	original, err := newAssetBundle(fixtures[0].files)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			bundle, err := newAssetBundle(fixture.files)
			if err != nil {
				t.Fatal(err)
			}
			if same := bundle.version == original.version; same != fixture.same {
				t.Fatalf("same version = %v, want %v", same, fixture.same)
			}
		})
	}
}

func TestAssetCachePolicy(t *testing.T) {
	handler := Handler()
	for _, test := range []struct {
		path   string
		status int
		cache  string
	}{
		{browserAssets.url("app.css"), 200, "public, max-age=31536000, immutable"},
		{browserAssets.url("ibm-plex-sans-latin-wght-normal.woff2"), 200, "public, max-age=31536000, immutable"},
		{"/assets/app.css", 200, "no-cache"},
		{"/assets/ibm-plex-sans-latin-wght-normal.woff2", 200, "no-cache"},
		{browserAssets.url("missing.css"), 404, "no-store"},
		{"/assets/" + strings.Repeat("0", 64) + "/app.css", 404, "no-store"},
		{"/assets/missing.css", 404, "no-store"},
		{"/assets/", 404, "no-store"},
		{"/", 200, "no-store"},
		{"/login", 200, "no-store"},
		{"/signup", 200, "no-store"},
	} {
		t.Run(test.path, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, test.path, nil))
				if response.Code != test.status || response.Header().Get("Cache-Control") != test.cache {
					t.Fatalf("%s status/cache = %d %q, want %d %q", method, response.Code, response.Header().Get("Cache-Control"), test.status, test.cache)
				}
				if method == http.MethodHead && test.status == 200 && strings.HasPrefix(test.path, "/assets/") && response.Body.Len() != 0 {
					t.Fatal("HEAD returned a body")
				}
			}
		})
	}
}

func TestUnversionedAssetRevalidation(t *testing.T) {
	handler := Handler()
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/assets/app.css", nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	for _, test := range []struct {
		etag   string
		status int
	}{{etag, 304}, {`"old-build"`, 200}} {
		request := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
		request.Header.Set("If-None-Match", test.etag)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status || response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("status/cache = %d %q", response.Code, response.Header().Get("Cache-Control"))
		}
	}
}

func TestFontPreloadMatchesBuiltStylesheet(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	body := response.Body.String()
	preload := regexp.MustCompile(`<link rel="preload" href="([^"]+)" as="font" type="font/woff2" crossorigin="anonymous"`).FindStringSubmatch(body)
	stylesheet := regexp.MustCompile(`<link rel="stylesheet" href="([^"]+)"`).FindStringSubmatch(body)
	if len(preload) != 2 || len(stylesheet) != 2 {
		t.Fatal("missing font preload or stylesheet")
	}
	cssURL, err := url.Parse(stylesheet[1])
	if err != nil {
		t.Fatal(err)
	}
	css := httptest.NewRecorder()
	Handler().ServeHTTP(css, httptest.NewRequest(http.MethodGet, cssURL.String(), nil))
	matched := false
	for _, match := range regexp.MustCompile(`url\(["']?([^\s)"']+)["']?\)`).FindAllStringSubmatch(css.Body.String(), -1) {
		reference, err := url.Parse(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if cssURL.ResolveReference(reference).String() == preload[1] {
			matched = true
		}
	}
	if !matched {
		t.Fatal("built CSS does not reference the preloaded font URL")
	}
	font := httptest.NewRecorder()
	Handler().ServeHTTP(font, httptest.NewRequest(http.MethodGet, preload[1], nil))
	if font.Code != http.StatusOK || font.Body.Len() == 0 {
		t.Fatal("preloaded font is not served")
	}
}
