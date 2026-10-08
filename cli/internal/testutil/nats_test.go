package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/natsresources"
)

// TestResetChattoJetStreamRemovesDataWhenServerDeletionLeavesIt simulates a
// slow background removal of an earlier deletion. The server then cannot move
// the stream directory away, ignores the error, and leaves the old message
// blocks on disk. A stream created later with the same name must still start
// empty.
func TestResetChattoJetStreamRemovesDataWhenServerDeletionLeavesIt(t *testing.T) {
	ns, nc := StartSharedNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := jetstream.StreamConfig{Name: natsresources.EVT, Subjects: []string{"evt.>"}, Storage: jetstream.FileStorage}
	if _, err := js.CreateStream(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := js.Publish(ctx, "evt.test", []byte("old")); err != nil {
			t.Fatal(err)
		}
	}

	// An earlier deletion that is still in progress keeps this directory.
	streams := filepath.Join(ns.JetStreamConfig().StoreDir, "$G", "streams")
	tombstone := filepath.Join(streams, "."+natsresources.EVT)
	if err := os.MkdirAll(tombstone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tombstone, "block"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ResetChattoJetStream(t, ns, nc)

	stream, err := js.CreateStream(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 0 {
		t.Fatalf("recreated stream has %d old messages, want 0", info.State.Msgs)
	}
}
