package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ErrAdmissionLimited means the shared request budget is exhausted.
var ErrAdmissionLimited = errors.New("request admission limit reached")

// AdmitRequest consumes a non-refundable request allowance in runtime state.
// key must be a product-owned constant or an opaque keyed digest, never PII.
// Each admission resets the quiet window. OCC bounds admissions across replicas;
// storage failures and unknown acknowledgements fail closed without refunds.
func AdmitRequest(ctx context.Context, kv jetstream.KeyValue, js jetstream.JetStream, key string, limit int, window time.Duration) error {
	if limit < 1 || window <= 0 {
		return fmt.Errorf("invalid request admission policy")
	}
	for range 16 {
		entry, err := kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			_, err = kv.Create(ctx, key, []byte(`{"count":1}`), jetstream.KeyTTL(window))
			if err == nil {
				return nil
			}
		} else if err != nil {
			return fmt.Errorf("read request admission: %w", err)
		} else {
			var counter struct {
				Count int `json:"count"`
			}
			if json.Unmarshal(entry.Value(), &counter) != nil || counter.Count < 1 {
				return fmt.Errorf("invalid request admission counter")
			}
			if counter.Count >= limit {
				return ErrAdmissionLimited
			}
			counter.Count++
			data, _ := json.Marshal(counter)
			_, err = UpdateKeyWithTTL(ctx, js, RuntimeStateBucket, key, data, entry.Revision(), window)
			if err == nil {
				return nil
			}
		}
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("commit request admission: %w", err)
		}
	}
	return fmt.Errorf("request admission conflicted repeatedly")
}
