package linkpreview

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HugoSmits86/nativewebp"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/assets"
)

func TestFetcherRecognizesDirectImagesByContent(t *testing.T) {
	restoreLocalhost := AllowLocalhostForTesting()
	defer restoreLocalhost()

	for _, tc := range []struct {
		name        string
		contentType string
		data        []byte
	}{
		{name: "jpeg", contentType: "text/plain", data: encodeDirectImage(t, "jpeg")},
		{name: "png", contentType: "application/octet-stream", data: encodeDirectImage(t, "png")},
		{name: "static webp", contentType: "image/webp", data: encodeDirectImage(t, "webp")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write(tc.data)
			}))
			defer server.Close()

			fetcher := directImageFetcher(t)
			result, err := fetcher.Fetch(context.Background(), server.URL+"/misleading.txt")
			require.NoError(t, err)
			require.NotNil(t, result.DirectImage)
			require.Equal(t, assets.DetectImageContentType(tc.data), result.DirectImage.ContentType)
			require.Equal(t, 3, result.DirectImage.Width)
			require.Equal(t, 2, result.DirectImage.Height)
			require.Equal(t, tc.data, result.DirectImage.Data)
		})
	}
}

func TestFetcherPreservesAnimatedDirectGIF(t *testing.T) {
	restoreLocalhost := AllowLocalhostForTesting()
	defer restoreLocalhost()

	animatedGIF := encodeAnimatedGIF(t, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(animatedGIF)
	}))
	defer server.Close()

	fetcher := directImageFetcher(t)
	result, err := fetcher.Fetch(context.Background(), server.URL+"/animation")
	require.NoError(t, err)
	require.NotNil(t, result.DirectImage)
	require.Equal(t, "image/gif", result.DirectImage.ContentType)
	require.Equal(t, animatedGIF, result.DirectImage.Data)
	require.Equal(t, 3, result.DirectImage.Width)
	require.Equal(t, 2, result.DirectImage.Height)
	require.True(t, assets.IsAnimatedGIF(result.DirectImage.Data))
}

func TestFetcherRejectsInvalidAndExcessiveDirectImages(t *testing.T) {
	restoreLocalhost := AllowLocalhostForTesting()
	defer restoreLocalhost()

	for _, tc := range []struct {
		name        string
		contentType string
		data        []byte
	}{
		{name: "invalid declared image", contentType: "image/png", data: []byte("not an image")},
		{name: "too many gif frames", contentType: "image/gif", data: encodeAnimatedGIF(t, assets.MaxAnimatedImageFrames+1)},
		{name: "too large", contentType: "image/jpeg", data: append([]byte{0xff, 0xd8, 0xff}, make([]byte, MaxImageSize+1)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write(tc.data)
			}))
			defer server.Close()

			fetcher := directImageFetcher(t)
			_, err := fetcher.Fetch(context.Background(), server.URL+"/image")
			require.ErrorIs(t, err, ErrUnavailable)
		})
	}
}

func directImageFetcher(t *testing.T) *Fetcher {
	t.Helper()
	return NewFetcher(&assets.Config{MaxUploadSize: MaxImageSize}, func() string { return "preview-asset" }, nil)
}

func encodeDirectImage(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.RGBA{R: 0xff, A: 0xff})
	var buf bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "png":
		err = png.Encode(&buf, img)
	case "webp":
		err = nativewebp.Encode(&buf, img, nil)
	}
	require.NoError(t, err)
	return buf.Bytes()
}

func encodeAnimatedGIF(t *testing.T, frameCount int) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	frames := make([]*image.Paletted, frameCount)
	delays := make([]int, frameCount)
	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 3, 2), palette)
		frame.SetColorIndex(i%3, i%2, uint8(i%2))
		frames[i] = frame
		delays[i] = 1
	}
	var buf bytes.Buffer
	require.NoError(t, gif.EncodeAll(&buf, &gif.GIF{Image: frames, Delay: delays, LoopCount: -1}))
	return buf.Bytes()
}
