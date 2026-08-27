// Package storage creates Authling-owned JetStream resources.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// EventStreamName is Authling's primary event-sourcing stream.
	EventStreamName = "AUTHLING_EVT"
	// EventSubjects contains every Authling durable domain event.
	EventSubjects = "authling.evt.>"
	// RuntimeStateBucket contains expiring workflow state and bearer material.
	RuntimeStateBucket = "AUTHLING_RUNTIME_STATE"
	// KeyStoreBucket contains separately protected user and wrapped data keys.
	KeyStoreBucket = "AUTHLING_KEYS"
)

// Stores contains Authling's non-event JetStream stores.
type Stores struct {
	RuntimeState jetstream.KeyValue
	Keys         jetstream.KeyValue
}

// UpdateKeyWithTTL performs an OCC KV update while preserving an explicit
// per-key expiry. The high-level KV Update API cannot attach the TTL header.
func UpdateKeyWithTTL(ctx context.Context, js jetstream.JetStream, bucket, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
	msg := nats.NewMsg("$KV." + bucket + "." + key)
	msg.Data = value
	ack, err := js.PublishMsg(ctx, msg, jetstream.WithExpectLastSequencePerSubject(revision), jetstream.WithMsgTTL(ttl))
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

// DeleteKey performs an OCC KV deletion and returns the resulting stream
// revision so an owning in-memory model can provide read-your-writes.
func DeleteKey(ctx context.Context, js jetstream.JetStream, bucket, key string, revision uint64) (uint64, error) {
	msg := nats.NewMsg("$KV." + bucket + "." + key)
	msg.Header.Set("KV-Operation", "DEL")
	ack, err := js.PublishMsg(ctx, msg, jetstream.WithExpectLastSequencePerSubject(revision))
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

// Open ensures Authling's event stream exists and returns the JetStream
// context and stream bound to the current NATS account.
func Open(
	ctx context.Context,
	connection *nats.Conn,
	replicas int,
) (jetstream.JetStream, jetstream.Stream, error) {
	js, err := jetstream.New(connection)
	if err != nil {
		return nil, nil, fmt.Errorf("create JetStream client: %w", err)
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:               EventStreamName,
		Description:        "Authling durable event log",
		Subjects:           []string{EventSubjects},
		Retention:          jetstream.LimitsPolicy,
		Storage:            jetstream.FileStorage,
		Compression:        jetstream.S2Compression,
		Replicas:           replicas,
		AllowAtomicPublish: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("ensure %s stream: %w", EventStreamName, err)
	}
	return js, stream, nil
}

// OpenStores ensures Authling's runtime-state and key-store buckets exist.
func OpenStores(ctx context.Context, js jetstream.JetStream, replicas int) (Stores, error) {
	runtimeState, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: RuntimeStateBucket, Description: "Authling expiring runtime state",
		Storage: jetstream.FileStorage, Replicas: replicas, History: 1, LimitMarkerTTL: time.Hour,
	})
	if err != nil {
		return Stores{}, fmt.Errorf("ensure %s bucket: %w", RuntimeStateBucket, err)
	}
	keys, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: KeyStoreBucket, Description: "Authling protected key material",
		Storage: jetstream.FileStorage, Replicas: replicas, History: 1,
	})
	if err != nil {
		return Stores{}, fmt.Errorf("ensure %s bucket: %w", KeyStoreBucket, err)
	}
	return Stores{RuntimeState: runtimeState, Keys: keys}, nil
}
