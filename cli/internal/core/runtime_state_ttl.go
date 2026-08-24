package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// updateRuntimeStateWithTTL performs a revision-checked KV update with a new
// per-message TTL. The NATS KV API supports a per-key TTL on Create, but not on
// Update. Keep the direct JetStream operation in this function so all callers
// use the same subject format, optimistic-concurrency check, and TTL header.
func (c *ChattoCore) updateRuntimeStateWithTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
	if ttl <= 0 {
		return 0, fmt.Errorf("runtime-state TTL must be positive")
	}
	msg := nats.NewMsg("$KV.RUNTIME_STATE." + key)
	msg.Data = value
	ack, err := c.js.PublishMsg(ctx, msg,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(ttl),
	)
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

// updateRuntimeStateUntil preserves physical retention until the record's
// authoritative absolute expiry. It does not change that expiry.
func (c *ChattoCore) updateRuntimeStateUntil(ctx context.Context, key string, value []byte, revision uint64, expiresAt, now time.Time) (uint64, error) {
	if !now.Before(expiresAt) {
		return 0, fmt.Errorf("runtime-state record has expired")
	}
	return c.updateRuntimeStateWithTTL(ctx, key, value, revision, expiresAt.Sub(now))
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
