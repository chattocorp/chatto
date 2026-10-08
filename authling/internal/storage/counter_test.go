package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
)

// failingCreateKeyValue fails every Create with an error that is not an OCC
// conflict.
type failingCreateKeyValue struct {
	KeyValue
	creates int
}

var errStorageUnavailable = errors.New("storage unavailable")

func (kv *failingCreateKeyValue) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	kv.creates++
	return 0, errStorageUnavailable
}

func TestIncrementCounterStopsOnStorageFailure(t *testing.T) {
	cfg := config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	_, _, kv := openDeliveryStore(t, cfg)
	failing := &failingCreateKeyValue{KeyValue: kv}

	limited, err := IncrementCounter(t.Context(), failing, "failing.counter", 5, time.Minute)
	if limited || !errors.Is(err, errStorageUnavailable) {
		t.Fatalf("IncrementCounter = %v, %v; want the storage error", limited, err)
	}
	if failing.creates != 1 {
		t.Fatalf("creates = %d, want 1", failing.creates)
	}
}

func TestIncrementCounterStopsAtLimitWithoutWrite(t *testing.T) {
	cfg := config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	_, _, kv := openDeliveryStore(t, cfg)
	const key = "limit.counter"
	for range 2 {
		if limited, err := IncrementCounter(t.Context(), kv, key, 2, time.Minute); limited || err != nil {
			t.Fatalf("IncrementCounter = %v, %v; want an increment", limited, err)
		}
	}
	before, err := kv.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	limited, err := IncrementCounter(t.Context(), kv, key, 2, time.Minute)
	if !limited || err != nil {
		t.Fatalf("IncrementCounter = %v, %v; want limited", limited, err)
	}
	after, err := kv.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision() != before.Revision() {
		t.Fatalf("revision = %d, want unchanged %d", after.Revision(), before.Revision())
	}
}
