package core

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// updateRuntimeStateTokenTTL is retained for mutable non-session runtime
// records whose lifecycle migration is separate from authentication. Session
// expiry uses explicit deadlines, ordinary KV updates, and immutable markers.
func (c *ChattoCore) updateRuntimeStateTokenTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
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
