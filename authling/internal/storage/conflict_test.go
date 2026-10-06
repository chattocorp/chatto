package storage

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/config"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// replicatedConflictJS rejects its first publish with the conflict code that a
// replicated stream returns while another write to the same key is in
// progress. That code does not match jetstream.ErrKeyExists.
type replicatedConflictJS struct {
	jetstream.JetStream
	rejected atomic.Bool
}

func (js *replicatedConflictJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if js.rejected.CompareAndSwap(false, true) {
		return nil, &jetstream.APIError{
			Code:        400,
			ErrorCode:   jetstream.JSErrCodeStreamWrongLastSequenceConstant,
			Description: "wrong last sequence",
		}
	}
	return js.JetStream.PublishMsg(ctx, msg, opts...)
}

func TestCountersRetryReplicatedStreamConflicts(t *testing.T) {
	const key = "conflict.counter"
	policy := DeliveryPolicy{GlobalKey: "conflict.global", GlobalLimit: 10, RecipientLimit: 10, Window: time.Minute}
	tests := []struct {
		name      string
		operation func(context.Context, KeyValue) error
		want      int
	}{
		{"admission", func(ctx context.Context, kv KeyValue) error {
			return AdmitRequest(ctx, kv, key, 10, time.Minute)
		}, 3},
		{"delivery reservation", func(ctx context.Context, kv KeyValue) error {
			return NewDeliveryBudget(kv, policy).reserveCounter(ctx, key, 10)
		}, 3},
		{"delivery rollback", func(ctx context.Context, kv KeyValue) error {
			return NewDeliveryBudget(kv, policy).rollbackCounter(ctx, key)
		}, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.NATSConfig{Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
			_, js, kv := openDeliveryStore(t, cfg)
			if _, err := kv.Create(t.Context(), key, []byte(`{"count":2}`), jetstream.KeyTTL(time.Minute)); err != nil {
				t.Fatal(err)
			}
			conflicting := &replicatedConflictJS{JetStream: js}
			bound, err := jetstreamutil.NewKeyValue(conflicting, kv.KeyValue)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.operation(t.Context(), bound); err != nil {
				t.Fatalf("operation failed after a replicated conflict: %v", err)
			}
			if !conflicting.rejected.Load() {
				t.Fatal("operation did not publish a revision-checked update")
			}
			if got := deliveryCount(t, kv, key); got != test.want {
				t.Fatalf("count = %d, want %d", got, test.want)
			}
		})
	}
}
