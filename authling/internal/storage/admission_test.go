package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/natsruntime"
)

func TestAdmissionConcurrentWritersRestartAndExpiry(t *testing.T) {
	cfg := config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	open := func() (*natsruntime.Connection, jetstream.JetStream, jetstream.KeyValue) {
		t.Helper()
		connection, err := natsruntime.Open(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		js, err := jetstream.New(connection.NATS)
		if err != nil {
			t.Fatal(err)
		}
		stores, err := OpenStores(t.Context(), js, 1)
		if err != nil {
			t.Fatal(err)
		}
		return connection, js, stores.RuntimeState
	}
	connection, js, kv := open()
	// Separate handles share only server state, as on independent replicas.
	other, err := js.KeyValue(t.Context(), RuntimeStateBucket)
	if err != nil {
		t.Fatal(err)
	}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Go(func() {
			store := kv
			if i%2 == 1 {
				store = other
			}
			if err := AdmitRequest(t.Context(), store, js, "admission.test", 7, time.Minute); err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrAdmissionLimited) {
				t.Errorf("admit: %v", err)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 7 {
		t.Fatalf("admitted %d, want 7", admitted.Load())
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	connection, js, kv = open()
	defer connection.Close()
	if err := AdmitRequest(t.Context(), kv, js, "admission.test", 7, time.Minute); !errors.Is(err, ErrAdmissionLimited) {
		t.Fatalf("budget lost on restart: %v", err)
	}
	if err := AdmitRequest(t.Context(), kv, js, "admission.expiry", 1, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := AdmitRequest(t.Context(), kv, js, "admission.expiry", 1, time.Second); !errors.Is(err, ErrAdmissionLimited) {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := AdmitRequest(t.Context(), kv, js, "admission.expiry", 1, time.Second)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrAdmissionLimited) || time.Now().After(deadline) {
			t.Fatalf("expiry: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := kv.Put(t.Context(), "admission.corrupt", []byte(`{"count":-1}`)); err != nil {
		t.Fatal(err)
	}
	if err := AdmitRequest(t.Context(), kv, js, "admission.corrupt", 1, time.Minute); err == nil {
		t.Fatal("accepted malformed counter")
	}
	lost := &lostAdmissionAck{KeyValue: kv}
	if err := AdmitRequest(t.Context(), lost, js, "admission.lost-ack", 1, time.Minute); err == nil {
		t.Fatal("accepted lost acknowledgement")
	}
	if err := AdmitRequest(t.Context(), kv, js, "admission.lost-ack", 1, time.Minute); !errors.Is(err, ErrAdmissionLimited) {
		t.Fatalf("unknown outcome refunded: %v", err)
	}
}

type lostAdmissionAck struct{ jetstream.KeyValue }

func (kv *lostAdmissionAck) Create(ctx context.Context, key string, value []byte, options ...jetstream.KVCreateOpt) (uint64, error) {
	if _, err := kv.KeyValue.Create(ctx, key, value, options...); err != nil {
		return 0, err
	}
	return 0, errors.New("injected acknowledgement loss")
}
