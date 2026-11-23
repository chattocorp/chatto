package http_server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/core/remoteimage"
	"github.com/chattocorp/chatto/cli/pkg/signedurl"
)

const (
	// remoteImageCacheControl lets browsers and shared caches keep a served
	// remote image for one day. The image at a remote URL can change, so the
	// response is not immutable. The image is public on the remote host, so
	// shared caching reveals nothing private.
	remoteImageCacheControl = "public, max-age=86400"
	// remoteImageFailureCacheControl lets clients ask for a failed remote
	// image again after five minutes instead of on every render.
	remoteImageFailureCacheControl = "public, max-age=300"
	// remoteImageGenerationLength is how long one cache generation of a
	// remote image lasts. A new generation fetches the source again, so a
	// changed source shows within a day.
	remoteImageGenerationLength = 24 * time.Hour
	// remoteImageCacheReadTimeout bounds one read of a cached remote
	// derivative. Small cached images read in milliseconds; a slower read
	// overlaps a rare concurrent replacement and counts as a miss.
	remoteImageCacheReadTimeout = 2 * time.Second
)

// remoteImageNow returns the current time for cache generations. Tests
// advance it; production code must not change it.
var remoteImageNow = time.Now

// remoteImageGeneration returns the cache generation of a source at now.
// Each source starts its generations at its own offset, derived from its ID,
// so that the refreshes of all images spread over the day. A refresh writes
// a new cache object instead of replacing one: reading an object store
// object while it is replaced can stall and fail.
func remoteImageGeneration(sourceID string, now time.Time) int64 {
	offset, _ := strconv.ParseUint(sourceID[:15], 16, 64)
	length := int64(remoteImageGenerationLength)
	return (now.UnixNano() + int64(offset%uint64(length))) / length
}

// remoteImageCachePrefix keeps remote images apart from attachment and server
// asset derivatives in ASSET_CACHE.
const remoteImageCachePrefix = "remote_image"

// serveRemoteImage serves GET /assets/remote/{preset}/{signature}?url={url}.
// The server fetches the remote image and resizes it to the preset with the
// image transform pipeline, which caches the result, so the browser does not
// contact the remote host. Only the combinations of preset and source URL
// that the server signed are served.
func (s *HTTPServer) serveRemoteImage(c *gin.Context) {
	preset, ok := remoteimage.ParsePreset(c.Param("preset"))
	rawURL := c.Query("url")
	if !ok || rawURL == "" || !s.core.VerifyRemoteImageSignature(preset, rawURL, c.Param("signature")) {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNotFound)
		return
	}

	size := preset.Size()
	sourceID := remoteimage.SourceID(rawURL)
	generation := remoteImageGeneration(sourceID, remoteImageNow())
	s.serveTransformedAssetWithParams(c, transformRequest{
		CachePrefix:  remoteImageCachePrefix,
		AssetID:      fmt.Sprintf("%s-%d", sourceID, generation),
		StaleAssetID: fmt.Sprintf("%s-%d", sourceID, generation-1),
		Remote:       true,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			data, err := s.core.FetchRemoteImage(ctx, rawURL)
			if err != nil {
				return nil, "", err
			}
			// The declared type of a remote host is not reliable; the bytes are.
			// A source that is not a decodable raster image, such as SVG or
			// corrupt data, counts as a failure, so it is not fetched again on
			// every request.
			contentType := assets.DetectImageContentType(data)
			if _, inspectErr := assets.InspectImage(bytes.NewReader(data)); inspectErr != nil || contentType == "application/octet-stream" {
				s.core.MarkRemoteImageFailure(ctx, rawURL)
				return nil, "", fmt.Errorf("%w: not a decodable image", remoteimage.ErrRejected)
			}
			return bytes.NewReader(data), contentType, nil
		},
	}, &signedurl.TransformParams{Width: size.Width, Height: size.Height, Fit: size.Fit})
}
