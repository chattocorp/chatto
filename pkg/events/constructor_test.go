package events_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

// Constructors panic when a required argument is nil: that is a wiring error,
// not a condition that a caller can handle at runtime.
func TestConstructorsPanicOnMissingRequiredArguments(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := events.NewEncodedEventLog(js, stream, nil)
	encode := func(string) (events.EncodedRecord, error) { return events.EncodedRecord{}, nil }
	handle := func(context.Context, events.DurableDelivery) error { return nil }
	var consumer jetstream.Consumer = struct{ jetstream.Consumer }{}
	var bucket jetstream.KeyValue = struct{ jetstream.KeyValue }{}

	for name, construct := range map[string]func(){
		"encoded event log without JetStream": func() { events.NewEncodedEventLog(nil, stream, nil) },
		"encoded event log without stream":    func() { events.NewEncodedEventLog(js, nil, nil) },
		"typed event log without log":         func() { events.NewTypedEventLog[string](nil, encode, nil) },
		"typed event log without encoder":     func() { events.NewTypedEventLog[string](eventLog, nil, nil) },
		"durable worker without consumer": func() {
			_, _ = events.NewDurableWorker(nil, handle, events.DurableWorkerOptions{MaxConcurrent: 1})
		},
		"durable worker without handler": func() {
			_, _ = events.NewDurableWorker(consumer, nil, events.DurableWorkerOptions{MaxConcurrent: 1})
		},
		"stream message reader without stream": func() {
			_, _ = events.NewStreamMessageReader(nil, events.StreamMessageReaderOptions{})
		},
		"key-value without JetStream": func() { _, _ = events.NewKeyValue(nil, bucket) },
		"key-value without bucket":    func() { _, _ = events.NewKeyValue(js, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("constructor did not panic")
				}
			}()
			construct()
		})
	}
}

// Invalid options are runtime configuration errors, so constructors return
// them.
func TestConstructorsReturnOptionErrors(t *testing.T) {
	_, stream := setupTestStream(t)
	var consumer jetstream.Consumer = struct{ jetstream.Consumer }{}
	handle := func(context.Context, events.DurableDelivery) error { return nil }
	if _, err := events.NewDurableWorker(consumer, handle, events.DurableWorkerOptions{}); err == nil {
		t.Fatal("NewDurableWorker accepted zero MaxConcurrent")
	}
	if _, err := events.NewStreamMessageReader(stream, events.StreamMessageReaderOptions{MaxConcurrentReads: -1}); err == nil {
		t.Fatal("NewStreamMessageReader accepted negative MaxConcurrentReads")
	}
}
