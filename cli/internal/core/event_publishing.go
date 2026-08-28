package core

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
func (c *ChattoCore) publishLiveEvent(ctx context.Context, subject string, event *livev1.LiveEvent) error {
	return c.publishLiveEvents(ctx, []liveEventPublication{{subject: subject, event: event}})
}

type liveEventPublication struct {
	subject string
	event   *livev1.LiveEvent
}

// publishLiveEvents publishes a related set of transient events and flushes
// once after the complete set has entered the client buffer. This keeps large
// fanouts linear without imposing one network round trip per recipient.
func (c *ChattoCore) publishLiveEvents(_ context.Context, publications []liveEventPublication) error {
	type encodedPublication struct {
		subject string
		data    []byte
	}
	encoded := make([]encodedPublication, 0, len(publications))
	for index, publication := range publications {
		if err := validateLiveEvent(publication.event); err != nil {
			return fmt.Errorf("live publication %d: %w", index, err)
		}
		eventData, err := proto.Marshal(publication.event)
		if err != nil {
			return fmt.Errorf("marshal live publication %d: %w", index, err)
		}
		encoded = append(encoded, encodedPublication{subject: publication.subject, data: eventData})
	}
	if len(encoded) == 0 {
		return nil
	}
	for _, publication := range encoded {
		if err := c.nc.Publish(publication.subject, publication.data); err != nil {
			return fmt.Errorf("publish live event to %s: %w", publication.subject, err)
		}
	}
	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("flush %d live events: %w", len(encoded), err)
	}
	return nil
}

func validateEvent(event *evtv1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validateLiveEvent(event *livev1.LiveEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: live event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newEvent fills in the Id, ActorID, and CreatedAt fields of an Event
// envelope if they're not already set. The caller provides the event
// with the concrete oneof variant already populated.
func newEvent(actorID string, event *evtv1.Event) *evtv1.Event {
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
func newLiveEvent(actorID string, event *livev1.LiveEvent) *livev1.LiveEvent {
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
