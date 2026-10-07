// Package storage creates Authling-owned JetStream resources.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/pkg/jetstreamutil"
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

// Stores contains Authling's non-event JetStream stores. Both buckets allow
// direct gets, so a follower can answer a plain Get with stale data. The bound
// handles read through the stream leader and observe every committed write.
type Stores struct {
	RuntimeState *jetstreamutil.KeyValue
	Keys         *jetstreamutil.KeyValue
}

// provisioningRetry is Authling's retry budget for stream and bucket
// provisioning. A clustered JetStream can reject a request while it elects a
// meta leader or places replicas.
var provisioningRetry = jetstreamutil.JetStreamResourceRetryPolicy{
	MaxAttempts: 3,
	RetryDelay:  25 * time.Millisecond,
}

// KeyValue is the runtime-state access that Authling services use: the bucket's
// own methods, leader-routed reads, and the revision-checked TTL update and
// delete of jetstreamutil.KeyValue. Both report a revision conflict as
// jetstream.ErrKeyRevisionMismatch. Tests can wrap it to inject faults.
type KeyValue interface {
	jetstream.KeyValue
	UpdateWithTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error)
	DeleteAt(ctx context.Context, key string, revision uint64) (uint64, error)
}

var _ KeyValue = (*jetstreamutil.KeyValue)(nil)

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
	stream, err := jetstreamutil.CreateJetStreamResourceWithRetry(ctx, provisioningRetry, func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:               EventStreamName,
			Description:        "Authling durable event log",
			Subjects:           []string{EventSubjects},
			Retention:          jetstream.LimitsPolicy,
			Storage:            jetstream.FileStorage,
			Compression:        jetstream.S2Compression,
			Replicas:           replicas,
			AllowAtomicPublish: true,
		})
	})
	if err != nil {
		return nil, nil, fmt.Errorf("ensure %s stream: %w", EventStreamName, err)
	}
	return js, stream, nil
}

// OpenStores ensures Authling's runtime-state and key-store buckets exist and
// binds both to reads through the stream leader.
func OpenStores(ctx context.Context, js jetstream.JetStream, replicas int) (Stores, error) {
	runtimeState, err := openBucket(ctx, js, jetstream.KeyValueConfig{
		Bucket: RuntimeStateBucket, Description: "Authling expiring runtime state",
		Storage: jetstream.FileStorage, Replicas: replicas, History: 1, LimitMarkerTTL: time.Hour,
	})
	if err != nil {
		return Stores{}, err
	}
	keys, err := openBucket(ctx, js, jetstream.KeyValueConfig{
		Bucket: KeyStoreBucket, Description: "Authling protected key material",
		Storage: jetstream.FileStorage, Replicas: replicas, History: 1,
	})
	if err != nil {
		return Stores{}, err
	}
	return Stores{RuntimeState: runtimeState, Keys: keys}, nil
}

func openBucket(ctx context.Context, js jetstream.JetStream, config jetstream.KeyValueConfig) (*jetstreamutil.KeyValue, error) {
	bucket, err := jetstreamutil.CreateJetStreamResourceWithRetry(ctx, provisioningRetry, func(ctx context.Context) (jetstream.KeyValue, error) {
		return js.CreateOrUpdateKeyValue(ctx, config)
	})
	if err != nil {
		return nil, fmt.Errorf("ensure %s bucket: %w", config.Bucket, err)
	}
	kv, err := jetstreamutil.NewKeyValue(js, bucket)
	if err != nil {
		return nil, fmt.Errorf("bind %s bucket: %w", config.Bucket, err)
	}
	return kv, nil
}
