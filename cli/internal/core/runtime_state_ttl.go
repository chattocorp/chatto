package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// updateRuntimeStateUntil preserves physical retention until the record's
// authoritative absolute expiry. It does not change that expiry.
func (c *ChattoCore) updateRuntimeStateUntil(ctx context.Context, key string, value []byte, revision uint64, expiresAt, now time.Time) (uint64, error) {
	if !now.Before(expiresAt) {
		return 0, fmt.Errorf("runtime-state record has expired")
	}
	return c.storage.runtimeStateKV.UpdateWithTTL(ctx, key, value, revision, expiresAt.Sub(now))
}

// deleteRuntimeStateKey removes one key idempotently. A revision option can
// fence a cleanup operation against a concurrent update.
func (c *ChattoCore) deleteRuntimeStateKey(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	err := c.storage.runtimeStateKV.Delete(ctx, key, opts...)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return fmt.Errorf("delete runtime-state key %s: %w", key, err)
	}
	return nil
}
