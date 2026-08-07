package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
)

// JetStream documents that Update resets an existing per-key TTL. This is a
// security contract for signup flows: verification must neither remove nor
// prematurely consume their expiration.
func TestRuntimeStateUpdatePreservesAndResetsPerKeyTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	connection, err := natsruntime.Open(ctx, config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := jetstream.New(connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := OpenStores(ctx, js, 1)
	if err != nil {
		t.Fatal(err)
	}

	const ttl = 2 * time.Second
	revision, err := stores.RuntimeState.Create(ctx, "ttl-contract", []byte("created"), jetstream.KeyTTL(ttl))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := UpdateKeyWithTTL(ctx, js, RuntimeStateBucket, "ttl-contract", []byte("updated"), revision, ttl); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := stores.RuntimeState.Get(ctx, "ttl-contract"); err != nil {
		t.Fatalf("updated key expired on original deadline: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, err := stores.RuntimeState.Get(ctx, "ttl-contract")
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("updated key did not expire after its reset TTL")
}
