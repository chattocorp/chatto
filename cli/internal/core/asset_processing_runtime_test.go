package core

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/testutil"
)

func TestAssetProcessingRuntimeUsesVideoUploadLimit(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVT", Subjects: []string{"evt.>"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "SERVER_ASSETS"}); err != nil {
		t.Fatal(err)
	}

	runtime, err := NewAssetProcessingRuntime(ctx, nc, js,
		config.CoreConfig{Assets: config.AssetsConfig{MaxUploadSize: 10}},
		config.VideoConfig{MaxUploadSize: 20},
		log.New(io.Discard),
	)
	if err != nil {
		t.Fatal(err)
	}

	// The standalone worker does not need video uploads enabled. It still
	// needs the video limit when it stores generated playback segments.
	_, err = runtime.Core().mediaModel.uploadAttachmentBinary(ctx, "R-test", "segment.ts", "video/mp2t", strings.NewReader("12345678901"))
	if err != nil {
		t.Fatalf("upload 11-byte video segment with 10-byte general limit: %v", err)
	}
	_, err = runtime.Core().mediaModel.uploadAttachmentBinary(ctx, "R-test", "segment.ts", "video/mp2t", strings.NewReader(strings.Repeat("x", 21)))
	if err == nil || !strings.Contains(err.Error(), "maximum size of 20 bytes") {
		t.Fatalf("oversized video segment error = %v, want 20-byte limit", err)
	}
	_, err = runtime.Core().mediaModel.uploadAttachmentBinary(ctx, "R-test", "data.bin", "application/octet-stream", strings.NewReader("12345678901"))
	if err == nil || !strings.Contains(err.Error(), "maximum size of 10 bytes") {
		t.Fatalf("oversized non-video attachment error = %v, want 10-byte limit", err)
	}

	defaultRuntime, err := NewAssetProcessingRuntime(ctx, nc, js,
		config.CoreConfig{Assets: config.AssetsConfig{MaxUploadSize: 10}},
		config.VideoConfig{},
		log.New(io.Discard),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := defaultRuntime.Core().VideoMaxUploadSize, int64(config.DefaultVideoMaxUploadSize); got != want {
		t.Fatalf("default video limit = %d, want %d", got, want)
	}
}
