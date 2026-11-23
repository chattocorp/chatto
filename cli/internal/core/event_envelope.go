package core

import (
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	pubsubv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/realtime/v1"
)

// EventEnvelope is the in-process envelope used by StreamMyEvents and the
// realtime API. Concrete implementations are intentionally private so an
// envelope can only wrap one backing source: a durable EVT fact, a pubsub
// event, or a synthetic heartbeat.
type EventEnvelope interface {
	ID() string
	CreatedAt() *timestamppb.Timestamp
	ActorID() string
	Payload() any
	DeliverySeq() uint64
	EVTEvent() *evtv1.Event
	PubSubEvent() *pubsubv1.PubSubEvent
	Heartbeat() bool
	// SharedRealtimeMapping returns the viewer-independent realtime mapping of
	// the envelope. The first successful compute result is stored on the
	// envelope and returned to all later callers, so concurrent connections
	// that get the same envelope from the hub do the work once. An error is
	// returned only to its caller and is not stored; the next caller computes
	// again. The realtime API owns this slot. The value must be safe to share:
	// it must not hold a value for one viewer, and callers must not change it.
	SharedRealtimeMapping(compute func() (any, error)) (any, error)
}

// envelopeMemo stores one shared value for the lifetime of its envelope.
type envelopeMemo struct {
	mu    sync.Mutex
	done  bool
	value any
}

func (m *envelopeMemo) SharedRealtimeMapping(compute func() (any, error)) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done {
		return m.value, nil
	}
	value, err := compute()
	if err != nil {
		return nil, err
	}
	m.value, m.done = value, true
	return value, nil
}

type evtEventEnvelope struct {
	envelopeMemo
	event       *evtv1.Event
	deliverySeq uint64
}

func NewEVTEventEnvelopeWithDeliverySeq(event *evtv1.Event, seq uint64) EventEnvelope {
	if event == nil {
		return nil
	}
	return &evtEventEnvelope{event: event, deliverySeq: seq}
}

func (e *evtEventEnvelope) ID() string                         { return e.event.GetId() }
func (e *evtEventEnvelope) CreatedAt() *timestamppb.Timestamp  { return e.event.GetCreatedAt() }
func (e *evtEventEnvelope) ActorID() string                    { return e.event.GetActorId() }
func (e *evtEventEnvelope) Payload() any                       { return e.event.GetEvent() }
func (e *evtEventEnvelope) DeliverySeq() uint64                { return e.deliverySeq }
func (e *evtEventEnvelope) EVTEvent() *evtv1.Event             { return e.event }
func (e *evtEventEnvelope) PubSubEvent() *pubsubv1.PubSubEvent { return nil }
func (e *evtEventEnvelope) Heartbeat() bool                    { return false }

type pubsubEventEnvelope struct {
	envelopeMemo
	event *pubsubv1.PubSubEvent
}

// NewPubSubEventEnvelope wraps one NATS Core pubsub event.
func NewPubSubEventEnvelope(event *pubsubv1.PubSubEvent) EventEnvelope {
	if event == nil {
		return nil
	}
	return &pubsubEventEnvelope{event: event}
}

func (e *pubsubEventEnvelope) ID() string                        { return e.event.GetId() }
func (e *pubsubEventEnvelope) CreatedAt() *timestamppb.Timestamp { return e.event.GetCreatedAt() }
func (e *pubsubEventEnvelope) ActorID() string                   { return e.event.GetActorId() }
func (e *pubsubEventEnvelope) Payload() any                      { return e.event.GetEvent() }
func (e *pubsubEventEnvelope) DeliverySeq() uint64               { return 0 }
func (e *pubsubEventEnvelope) EVTEvent() *evtv1.Event            { return nil }
func (e *pubsubEventEnvelope) PubSubEvent() *pubsubv1.PubSubEvent {
	return e.event
}
func (e *pubsubEventEnvelope) Heartbeat() bool { return false }

type heartbeatEventEnvelope struct {
	envelopeMemo
	id        string
	createdAt *timestamppb.Timestamp
}

func NewHeartbeatEventEnvelope(id string, createdAt *timestamppb.Timestamp) EventEnvelope {
	return &heartbeatEventEnvelope{
		id:        id,
		createdAt: createdAt,
	}
}

func (e *heartbeatEventEnvelope) ID() string                         { return e.id }
func (e *heartbeatEventEnvelope) CreatedAt() *timestamppb.Timestamp  { return e.createdAt }
func (e *heartbeatEventEnvelope) ActorID() string                    { return "" }
func (e *heartbeatEventEnvelope) Payload() any                       { return nil }
func (e *heartbeatEventEnvelope) DeliverySeq() uint64                { return 0 }
func (e *heartbeatEventEnvelope) EVTEvent() *evtv1.Event             { return nil }
func (e *heartbeatEventEnvelope) PubSubEvent() *pubsubv1.PubSubEvent { return nil }
func (e *heartbeatEventEnvelope) Heartbeat() bool                    { return true }

func EventSessionTerminated(event EventEnvelope) *pubsubv1.SessionTerminatedEvent {
	if event == nil || event.PubSubEvent() == nil {
		return nil
	}
	return event.PubSubEvent().GetSessionTerminated()
}

func EventMessagePosted(event EventEnvelope) *evtv1.MessagePostedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessagePosted()
}

func EventMessageEdited(event EventEnvelope) *evtv1.MessageEditedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessageEdited()
}

func EventMessageRetracted(event EventEnvelope) *evtv1.MessageRetractedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessageRetracted()
}

func EventUserTyping(event EventEnvelope) *realtimev1.UserTypingEvent {
	if event == nil || event.PubSubEvent() == nil {
		return nil
	}
	return event.PubSubEvent().GetUserTyping()
}
