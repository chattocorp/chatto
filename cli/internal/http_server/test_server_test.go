package http_server

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
)

// bannerImageBytes returns an in-memory PNG suitable as a banner upload.
// Banners double as OG link-preview images at 1200x630.
func bannerImageBytes(t *testing.T) io.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 630))
	for y := 0; y < 630; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func setupHTTPServerTestServer(t *testing.T, authConfig config.AuthConfig) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	_, nc := testutil.StartSharedNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	router := gin.New()
	return &HTTPServer{
		config: config.ChattoConfig{
			Auth: authConfig,
		},
		nc:      nc,
		router:  router,
		core:    chattoCore,
		version: "1.2.3",
	}
}
