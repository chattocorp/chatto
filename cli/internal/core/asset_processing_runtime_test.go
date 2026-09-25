package core

import (
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/testutil"
)

func TestAssetProcessingRuntimeDoesNotUseVideoUploadLimit(t *testing.T) {
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
		log.New(io.Discard),
	)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.Core().VideoMaxUploadSize != 0 {
		t.Fatal("asset processing runtime must not apply user video upload limits")
	}
}
