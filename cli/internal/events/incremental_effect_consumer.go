package events

import (
	"context"
	"errors"
	"fmt"
	"sync"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// IncrementalEffectConsumer processes independent effects derived from events
// matching one subject filter. Its cursor and failed-work queue are process-local:
// a new consumer replays the full matching history. Successful effects are not
// repeated within one process, while failed effects remain queued without
// blocking later events. Handlers must be idempotent.
type IncrementalEffectConsumer struct {
	publisher *Publisher
	subject   string
	handle    func(context.Context, *SubjectEvent) error

	mu       sync.Mutex
	afterSeq uint64
	pending  []*SubjectEvent
}

// NewIncrementalEffectConsumer constructs an incremental consumer. Lifecycle,
// polling cadence, lease ownership, and retry backoff remain domain concerns.
func NewIncrementalEffectConsumer(
	publisher *Publisher,
	subject string,
	handle func(context.Context, *corev1.Event) error,
) *IncrementalEffectConsumer {
	return NewIncrementalEffectConsumerWithSubject(
		publisher,
		subject,
		func(ctx context.Context, subjectEvent *SubjectEvent) error {
			return handle(ctx, subjectEvent.Event)
		},
	)
}

// NewIncrementalEffectConsumerWithSubject constructs a consumer whose handler
// can validate the durable aggregate subject before performing an effect.
func NewIncrementalEffectConsumerWithSubject(
	publisher *Publisher,
	subject string,
	handle func(context.Context, *SubjectEvent) error,
) *IncrementalEffectConsumer {
	return &IncrementalEffectConsumer{
		publisher: publisher,
		subject:   subject,
		handle:    handle,
	}
}

// Consume discovers new events and attempts every pending effect. Concurrent
// calls are serialized so they cannot race cursor or queue advancement.
func (c *IncrementalEffectConsumer) Consume(ctx context.Context) error {
	if c == nil || c.publisher == nil || c.handle == nil || c.subject == "" {
		return fmt.Errorf("incremental effect consumer is not configured")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	events, lastSeq, readErr := c.publisher.SubjectEventsWithSubjectsAfter(ctx, c.subject, c.afterSeq)
	if readErr == nil {
		c.pending = append(c.pending, events...)
		if lastSeq > c.afterSeq {
			c.afterSeq = lastSeq
		}
	} else {
		readErr = fmt.Errorf("read incremental effects for %s: %w", c.subject, readErr)
	}

	remaining := c.pending[:0]
	var handleErr error
	for _, event := range c.pending {
		if err := c.handle(ctx, event); err != nil {
			remaining = append(remaining, event)
			handleErr = errors.Join(handleErr, fmt.Errorf("handle incremental effect %s for %s: %w", event.Event.GetId(), c.subject, err))
		}
	}
	c.pending = remaining
	return errors.Join(readErr, handleErr)
}
