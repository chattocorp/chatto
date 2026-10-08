package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// counterRecord is the stored value of a runtime-state counter.
type counterRecord struct {
	Count int `json:"count"`
}

// IncrementCounter adds one to the counter at key and restarts its expiry
// window. It returns limited without a write when the counter has already
// reached limit. OCC keeps the count correct across replicas: a conflict reads
// the counter again. Any other storage error stops the operation and is
// returned, because the outcome of a failed write is unknown. key must be a
// product-owned constant or an opaque keyed digest, never PII.
func IncrementCounter(ctx context.Context, kv KeyValue, key string, limit int, window time.Duration) (bool, error) {
	if limit < 1 || window <= 0 {
		return false, fmt.Errorf("invalid counter policy")
	}
	for range maxCounterAttempts {
		entry, err := kv.Get(ctx, key)
		switch {
		case errors.Is(err, jetstream.ErrKeyNotFound):
			_, err = kv.Create(ctx, key, []byte(`{"count":1}`), jetstream.KeyTTL(window))
		case err != nil:
			return false, fmt.Errorf("read counter: %w", err)
		default:
			counter, decodeErr := decodeCounter(entry.Value())
			if decodeErr != nil {
				return false, decodeErr
			}
			if counter.Count >= limit {
				return true, nil
			}
			counter.Count++
			data, _ := json.Marshal(counter)
			_, err = kv.UpdateWithTTL(ctx, key, data, entry.Revision(), window)
		}
		if err == nil {
			return false, nil
		}
		if !jetstreamutil.IsSequenceConflict(err) {
			return false, fmt.Errorf("write counter: %w", err)
		}
	}
	return false, fmt.Errorf("counter conflicted repeatedly")
}

// ReadCounter returns the current count and revision of the counter at key.
// It returns a zero count and revision when the counter does not exist.
func ReadCounter(ctx context.Context, kv KeyValue, key string) (count int, revision uint64, err error) {
	entry, err := kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("read counter: %w", err)
	}
	counter, err := decodeCounter(entry.Value())
	if err != nil {
		return 0, 0, err
	}
	return counter.Count, entry.Revision(), nil
}

// maxCounterAttempts bounds the OCC retries of one counter operation.
const maxCounterAttempts = 16

func decodeCounter(value []byte) (counterRecord, error) {
	var counter counterRecord
	if json.Unmarshal(value, &counter) != nil || counter.Count < 1 {
		return counterRecord{}, fmt.Errorf("invalid counter")
	}
	return counter, nil
}
