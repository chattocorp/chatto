// Package storage creates Authling-owned JetStream resources.
package storage

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// EventStreamName is Authling's primary event-sourcing stream.
	EventStreamName = "AUTHLING_EVT"
	// EventSubjects contains every Authling durable domain event.
	EventSubjects = "authling.evt.>"
)

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
