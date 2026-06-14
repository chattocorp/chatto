package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core/linkpreview"
)

func TestLinkPreviewImageStorageAndRetrieval(t *testing.T) {
	ctx := context.Background()
	core, _ := setupTestCore(t)

	restoreLocalhost := linkpreview.AllowLocalhostForTesting()
	defer restoreLocalhost()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
<meta property="og:title" content="Local Link Preview">
<meta property="og:description" content="A hermetic preview fixture">
<meta property="og:image" content="` + serverURL + `/preview.png">
</head>
<body>hello</body>
</html>`))
		case "/preview.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(createTestPNG(64, 64))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	url := server.URL + "/article"

	preview, err := core.GetLinkPreview(ctx, url)
	require.NoError(t, err, "GetLinkPreview should succeed")
	require.NotNil(t, preview, "Preview should not be nil")

	t.Logf("Title: %s", preview.Title)
	t.Logf("Description: %s", preview.Description)
	t.Logf("ImageAssetId: %s", preview.GetImageAssetId())

	require.Equal(t, "Local Link Preview", preview.Title)
	require.NotEmpty(t, preview.GetImageAssetId(), "ImageAssetId should not be empty")

	// Now try to retrieve the stored image
	reader, info, err := core.GetServerAssetFromAnyBackend(ctx, preview.GetImageAssetId())
	require.NoError(t, err, "GetServerAssetFromAnyBackend should succeed")
	require.NotNil(t, reader, "Reader should not be nil")

	t.Logf("Content-Type: %s", info.ContentType)
	t.Logf("Size: %d", info.Size)

	require.Equal(t, "image/webp", info.ContentType, "Content type should be image/webp")
	require.Greater(t, info.Size, int64(0), "Size should be greater than 0")

	// Read the data to verify it's valid
	data, err := io.ReadAll(reader)
	require.NoError(t, err, "Reading asset data should succeed")
	require.Greater(t, len(data), 0, "Data should not be empty")

	t.Logf("Read %d bytes of image data", len(data))

	// Verify it starts with WebP signature (RIFF....WEBP)
	require.True(t, len(data) >= 12, "Data should be at least 12 bytes")
	require.Equal(t, "RIFF", string(data[0:4]), "Should start with RIFF")
	require.Equal(t, "WEBP", string(data[8:12]), "Should have WEBP magic number")
}
