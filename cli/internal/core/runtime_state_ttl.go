package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// runtimeStateRetention rounds a positive lifetime up to JetStream's whole-second
// TTL precision. Readers enforce the absolute deadline stored in the record.
func runtimeStateRetention(ttl time.Duration) time.Duration {
	// A configured session lifetime can span the full duration range. Keep
	// retention finite if the next whole second cannot be represented.
	const largestWholeSecond = time.Duration(math.MaxInt64/int64(time.Second)) * time.Second
	if ttl >= largestWholeSecond {
		return largestWholeSecond
	}
	return (ttl - time.Nanosecond).Truncate(time.Second) + time.Second
}

// updateRuntimeStateUntil preserves physical retention until the record's
// authoritative absolute expiry. It does not change that expiry.
func (c *ChattoCore) updateRuntimeStateUntil(ctx context.Context, key string, value []byte, revision uint64, expiresAt, now time.Time) (uint64, error) {
	if !now.Before(expiresAt) {
		return 0, fmt.Errorf("runtime-state record has expired")
	}
	return c.storage.runtimeStateKV.UpdateWithTTL(ctx, key, value, revision, runtimeStateRetention(expiresAt.Sub(now)))
}

// deleteRuntimeStateKey removes one key idempotently. A revision option can
// fence a cleanup operation against a concurrent update.
func (c *ChattoCore) deleteRuntimeStateKey(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	err := c.storage.runtimeStateKV.Delete(ctx, key, opts...)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("delete runtime-state key %s: %w", key, err)
	}
	return nil
}
