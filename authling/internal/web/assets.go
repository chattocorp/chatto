package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// browserAssets is built once from the executable's immutable embedded files.
var browserAssets = func() *assetBundle {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	bundle, err := newAssetBundle(assets)
	if err != nil {
		panic(err)
	}
	return bundle
}()

// assetBundle gives every file a shared content version. Relative CSS URLs
// retain that version, so a deployment cannot reuse stale fonts or styles.
type assetBundle struct {
	version string
	files   map[string][]byte
}

func newAssetBundle(assets fs.FS) (*assetBundle, error) {
	bundle := &assetBundle{files: make(map[string][]byte)}
	digest := sha256.New()
	// WalkDir visits names in lexical order. Length-prefixed names and fixed-size
	// content digests make the input unambiguous, independent of file timestamps.
	err := fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		bundle.files[name] = content
		fmt.Fprintf(digest, "%d:%s:%x\n", len(name), name, sha256.Sum256(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read browser assets: %w", err)
	}
	bundle.version = fmt.Sprintf("%x", digest.Sum(nil))
	return bundle, nil
}

func (bundle *assetBundle) url(name string) string {
	return "/assets/" + bundle.version + "/" + name
}

// ServeHTTP caches only known files. Unversioned URLs must revalidate, while
// URLs for the current content version can be reused without a network request.
func (bundle *assetBundle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	versioned := strings.HasPrefix(name, bundle.version+"/")
	if versioned {
		name = strings.TrimPrefix(name, bundle.version+"/")
	}
	content, ok := bundle.files[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if versioned {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("ETag", `"`+bundle.version+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}
