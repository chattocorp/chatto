// Shared mechanics for application-owned durable effect consumers.
//
// ADR-069 keeps every durable consumer's identity, rollout, and retirement an
// application decision; this file only collapses the identical JetStream
// consumer-creation and durable-worker-option rituals that previously repeated
// at each effect site. Policy values remain per-site constants supplied by the
// caller.
package evtstream

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

// EffectConsumerConfig describes one durable effect consumer's persisted
// contract: its name and description, the event subjects it consumes, and its
// delivery policy. Name, filter subjects, and policy are deployment-wide
// resource decisions; changing them requires ADR-069's staged migration.
type EffectConsumerConfig struct {
	// Name is the durable consumer name (also used as Durable).
	Name string
	// Description is stored on the consumer for operator visibility.
	Description string
	// FilterSubjects lists one or more consumed subjects or wildcard filters.
	FilterSubjects []string
	// AckWait bounds unacknowledged redelivery for each delivery.
	AckWait time.Duration
	// MaxAckPending bounds outstanding deliveries and in-flight handler
	// concurrency: the same value feeds MaxAckPending, MaxRequestBatch, and
	// the worker's MaxConcurrent, matching every existing effect site.
	MaxAckPending int
	// DeliverPolicy selects where a new consumer starts reading the stream:
	// DeliverAllPolicy replays retained history; DeliverNewPolicy starts at
	// the creation boundary.
	DeliverPolicy jetstream.DeliverPolicy
}

func (c EffectConsumerConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("effect consumer name is required")
	}
	if len(c.FilterSubjects) == 0 {
		return fmt.Errorf("effect consumer %s needs at least one filter subject", c.Name)
	}
	for _, subject := range c.FilterSubjects {
		if subject == "" {
			return fmt.Errorf("effect consumer %s has an empty filter subject", c.Name)
		}
	}
	if c.AckWait <= 0 {
		return fmt.Errorf("effect consumer %s ack wait must be positive", c.Name)
	}
	if c.MaxAckPending <= 0 {
		return fmt.Errorf("effect consumer %s max ack pending must be positive", c.Name)
	}
	return nil
}

// CreateEffectConsumer creates or updates one durable effect consumer with
// Chatto's standard effect policies: explicit acknowledgement, unlimited
// redelivery, instant replay, and request batches bounded by MaxAckPending.
//
// A single filter subject is persisted via the singular Config.FilterSubject
// field so the stored consumer config stays byte-compatible with consumers
// that predate this helper; multi-subject consumers use FilterSubjects.
func CreateEffectConsumer(ctx context.Context, stream jetstream.Stream, config EffectConsumerConfig) (jetstream.Consumer, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	consumerConfig := jetstream.ConsumerConfig{
		Name:            config.Name,
		Durable:         config.Name,
		Description:     config.Description,
		DeliverPolicy:   config.DeliverPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         config.AckWait,
		MaxDeliver:      -1,
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   config.MaxAckPending,
		MaxRequestBatch: config.MaxAckPending,
	}
	if len(config.FilterSubjects) == 1 {
		consumerConfig.FilterSubject = config.FilterSubjects[0]
	} else {
		consumerConfig.FilterSubjects = config.FilterSubjects
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerConfig)
	if err != nil {
		return nil, fmt.Errorf("create effect consumer %s: %w", config.Name, err)
	}
	return consumer, nil
}

// EffectWorkerOptions holds the process-local execution knobs for one effect
// worker. They are deliberately narrower than events.DurableWorkerOptions:
// FetchMaxWait stays unset so the framework default applies, which is what
// every effect site already used.
type EffectWorkerOptions struct {
	// MaxConcurrent bounds simultaneously executing handlers. Feed it the
	// same EffectConsumerConfig.MaxAckPending used to create the consumer.
	MaxConcurrent int
	// RetryDelay waits before retrying after a failed delivery.
	RetryDelay time.Duration
	// AckTimeout fails handlers whose acknowledgement confirmation exceeds it.
	AckTimeout time.Duration
	// HeartbeatInterval bounds progress heartbeats to JetStream during pulls.
	HeartbeatInterval time.Duration
	// Logger receives delivery-failure diagnostics; errors must stay PII-free.
	Logger events.Logger
}

// NewEffectWorker builds a durable worker over an application-owned effect
// consumer using Chatto's standard effect execution options.
func NewEffectWorker(consumer jetstream.Consumer, handle events.DurableDeliveryHandler, options EffectWorkerOptions) (*events.DurableWorker, error) {
	return events.NewDurableWorker(consumer, handle, events.DurableWorkerOptions{
		MaxConcurrent:     options.MaxConcurrent,
		RetryDelay:        options.RetryDelay,
		AckTimeout:        options.AckTimeout,
		HeartbeatInterval: options.HeartbeatInterval,
		Logger:            options.Logger,
	})
}
