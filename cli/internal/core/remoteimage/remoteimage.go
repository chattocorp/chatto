// Package remoteimage fetches images from remote hosts for Chatto and signs
// the URLs through which the server serves them.
//
// The server, not the browser, contacts the remote host. A browser receives
// only a signed same-origin URL (see Path). The server signs only the source
// URLs that it decided to show, so the route that serves them is not an open
// proxy. The route resizes the image to a preset with the image transform
// pipeline, which also caches the result.
package remoteimage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/assets"
)

// MaxSourceBytes is the largest remote image that Fetch accepts by default.
const MaxSourceBytes = 5 * 1024 * 1024

// userAgent identifies Chatto to remote image hosts.
const userAgent = "ChattoBot/1.0 (Remote Image)"

// ErrRejected reports that a remote image is unavailable or not acceptable:
// an unsupported URL, a response other than 200 OK, a declared type that is
// not an image, or a body that is too large.
var ErrRejected = errors.New("remote image rejected")

// Preset names a fixed size that the server resizes a remote image to. The
// set of presets is closed, so processing and cache space stay bounded. A
// signature covers one preset and one source URL.
type Preset string

const (
	// PresetAvatar fits a user avatar or a server logo.
	PresetAvatar Preset = "avatar"
	// PresetBanner fits a wide image such as a server banner.
	PresetBanner Preset = "banner"
)

// Size is the box that a preset fits an image into.
type Size struct {
	Width  int
	Height int
	// Fit is an assets.FitMode name.
	Fit string
}

// presets maps each preset to its size. Add a preset here.
var presets = map[Preset]Size{
	PresetAvatar: {Width: assets.MaxAvatarDim, Height: assets.MaxAvatarDim, Fit: string(assets.FitContain)},
	PresetBanner: {Width: assets.MaxLinkPreviewWidth, Height: assets.MaxLinkPreviewHeight, Fit: string(assets.FitContain)},
}

// ParsePreset returns the preset with the given name.
func ParsePreset(name string) (Preset, bool) {
	_, ok := presets[Preset(name)]
	return Preset(name), ok
}

// Size returns the size of the preset.
func (p Preset) Size() Size {
	return presets[p]
}

// Fetch downloads the raw bytes of a remote image with client, which must be
// SSRF-safe. It accepts only absolute HTTP(S) URLs without credentials, a
// 200 OK response, a declared type that is missing, image/*, or
// application/octet-stream, and at most maxBytes of body. It returns
// ErrRejected for each of these failures. Errors do not contain the URL,
// which can identify an account.
func Fetch(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, error) {
	data, _, err := FetchCounted(ctx, client, rawURL, maxBytes)
	return data, err
}

// FetchCounted is Fetch that also reports how many body bytes it read, also
// when it fails. Callers with a download budget charge that count.
func FetchCounted(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, int64, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, 0, fmt.Errorf("%w: unsupported URL", ErrRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: unsupported URL", ErrRejected)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/webp,image/png,image/jpeg,image/gif,image/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, privateTransportError("fetch remote image", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("%w: status %d", ErrRejected, resp.StatusCode)
	}
	if declared := resp.Header.Get("Content-Type"); declared != "" {
		mediaType, _, err := mime.ParseMediaType(declared)
		if err != nil || (!strings.HasPrefix(mediaType, "image/") && mediaType != "application/octet-stream") {
			return nil, 0, fmt.Errorf("%w: not an image", ErrRejected)
		}
	}
	if resp.ContentLength > maxBytes {
		return nil, 0, fmt.Errorf("%w: larger than %d bytes", ErrRejected, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	read := int64(len(data))
	if err != nil {
		return nil, read, privateTransportError("read remote image", err)
	}
	if read > maxBytes {
		return nil, read, fmt.Errorf("%w: larger than %d bytes", ErrRejected, maxBytes)
	}
	return data, read, nil
}

// privateError prints only a fixed category. Transport errors can contain the
// source URL, a redirect location, host names, and IP addresses, which can
// identify an account or a user. The cause stays available to errors.Is and
// errors.As, so callers can still classify timeouts and network errors.
type privateError struct {
	message string
	cause   error
}

func (e *privateError) Error() string { return e.message }
func (e *privateError) Unwrap() error { return e.cause }

// privateTransportError wraps a transport error of operation op in a
// privateError with a category: canceled, timeout, or network error.
func privateTransportError(op string, err error) error {
	category := "network error"
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		category = "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		category = "timeout"
	}
	return &privateError{message: op + ": " + category, cause: err}
}

// RoutePrefix is the path prefix of the route that serves remote images.
const RoutePrefix = "/assets/remote/"

// Path returns the signed, server-relative URL through which the server
// serves rawURL resized to preset:
//
//	/assets/remote/{preset}/{signature}?url={rawURL}
//
// The signature covers the preset and the source URL, so a client cannot
// request another preset with it. The URL is stable for the same secret,
// preset, and source URL, so browsers and the server cache can reuse it.
func Path(secret string, preset Preset, rawURL string) string {
	return RoutePrefix + string(preset) + "/" + Sign(secret, preset, rawURL) + "?" + url.Values{"url": {rawURL}}.Encode()
}

// Sign returns the URL-safe HMAC-SHA256 signature of preset and rawURL.
func Sign(secret string, preset Preset, rawURL string) string {
	return base64.RawURLEncoding.EncodeToString(mac(secret, preset, rawURL))
}

// Verify reports whether signature is the signature of preset and rawURL.
func Verify(secret string, preset Preset, rawURL, signature string) bool {
	given, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(given, mac(secret, preset, rawURL))
}

// mac separates remote image signatures from the other uses of the asset
// signing secret with a fixed domain prefix. A NUL byte separates the preset
// from the source URL; neither contains one.
func mac(secret string, preset Preset, rawURL string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("chatto-remote-image\x00"))
	h.Write([]byte(preset))
	h.Write([]byte{0})
	h.Write([]byte(rawURL))
	return h.Sum(nil)
}

// SourceID returns a stable identifier of rawURL for cache keys and ETags.
// The hash keeps the source URL out of storage names and logs.
func SourceID(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}
