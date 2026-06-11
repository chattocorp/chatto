package migrations

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/testutil"
)

// setupTestES stands up an embedded NATS server with JetStream and
// returns the bits ES-migration tests need: a context, a KV bucket
// for "legacy" pre-ES state, the EVT stream, and a Publisher
// that writes to it.
//
// Mirrors setupTestKV's posture: minimal, self-contained, no
// ChattoCore. The migration functions are package-level functions
// taking explicit deps, so they can be exercised in isolation.
func setupTestES(t *testing.T) (
	ctx context.Context,
	kv jetstream.KeyValue,
	stream jetstream.Stream,
	publisher *events.Publisher,
) {
	ctx, kv, stream, publisher, _ = setupTestESWithJS(t)
	return ctx, kv, stream, publisher
}

func setupTestESWithJS(t *testing.T) (
	ctx context.Context,
	kv jetstream.KeyValue,
	stream jetstream.Stream,
	publisher *events.Publisher,
	js jetstream.JetStream,
) {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST_CONFIG",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create KV: %v", err)
	}

	stream, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "EVT_TEST",
		Subjects: []string{events.SubjectRoot + ">"},
		Storage:  jetstream.MemoryStorage,
		// AppendBatch (used by MigrateRoomAggregateToES) requires
		// the stream to opt into the Nats-Batch-* protocol.
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("create EVT stream: %v", err)
	}

	logger := log.New(io.Discard)
	publisher = events.NewPublisher(js, stream, logger)

	return ctx, kv, stream, publisher, js
}

// testLogger returns a logger that discards output. Migration tests
// don't assert on log content; production migrations log via the
// caller-supplied logger.
func testLogger() *log.Logger {
	return log.New(io.Discard)
}
