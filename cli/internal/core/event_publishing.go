package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Event Publishing Helpers
// ============================================================================

// natsPublishFlushTimeout bounds how long a fire-and-forget publish will wait
// for the NATS server to acknowledge buffered bytes. Without a timeout, a
// hung server (e.g. network partition) would block the calling goroutine
// indefinitely instead of surfacing as a normal error.
const natsPublishFlushTimeout = 5 * time.Second

// publishLiveEvent publishes a transient LiveEvent directly to a live.sync.>
// subject, bypassing JetStream storage. The subject should already include
// the "live.sync." prefix.
func (c *ChattoCore) publishLiveEvent(_ context.Context, subject string, event *corev1.LiveEvent) error {
	if err := validateLiveEvent(event); err != nil {
		return err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal live event: %w", err)
	}

	if err := c.nc.Publish(subject, eventData); err != nil {
		return fmt.Errorf("failed to publish live event to %s: %w", subject, err)
	}

	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("failed to flush live event to %s: %w", subject, err)
	}
	return nil
}

func validateEvent(event *corev1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validateLiveEvent(event *corev1.LiveEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: live event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newEvent fills in the Id, ActorID, and CreatedAt fields of an Event
// envelope if they're not already set. The caller provides the event
// with the concrete oneof variant already populated.
func newEvent(actorID string, event *corev1.Event) *corev1.Event {
	if event.Id == "" {
		event.Id = NewEventID()
	}
	if event.ActorId == "" {
		event.ActorId = actorID
	}
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}
	return event
}

// newLiveEvent fills in the Id, ActorID, and CreatedAt fields of a LiveEvent
// envelope if they're not already set. The caller provides the event with the
// concrete oneof variant already populated.
func newLiveEvent(actorID string, event *corev1.LiveEvent) *corev1.LiveEvent {
	if event.Id == "" {
		event.Id = NewEventID()
	}
	if event.ActorId == "" {
		event.ActorId = actorID
	}
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}
	return event
}

// ============================================================================
// Event Streaming
// ============================================================================

// isTerminalIteratorError returns true if the error indicates the iterator
// cannot be recovered (connection closed, consumer deleted, etc.).
// Recoverable errors (heartbeat missed, leadership changed) return false.
func isTerminalIteratorError(err error) bool {
	if err == nil {
		return false
	}
	// Terminal errors - cannot recover, must stop
	if errors.Is(err, jetstream.ErrMsgIteratorClosed) ||
		errors.Is(err, jetstream.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrServerShutdown) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) {
		return true
	}
	return false
}
