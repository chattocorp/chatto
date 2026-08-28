package core

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
)

// EventEnvelope is the in-process envelope used by StreamMyEvents and the
// realtime API. Concrete implementations are intentionally private so an
// envelope can only wrap one backing source: a durable EVT fact, a transient
// LiveEvent, or a synthetic heartbeat.
type EventEnvelope interface {
	ID() string
	CreatedAt() *timestamppb.Timestamp
	ActorID() string
	Payload() any
	DeliverySeq() uint64

	EVTEvent() *evtv1.Event
	LiveEvent() *livev1.LiveEvent
	HeartbeatEvent() *livev1.HeartbeatEvent
}

type evtEventEnvelope struct {
	event       *evtv1.Event
	deliverySeq uint64
}

func NewEVTEventEnvelope(event *evtv1.Event) EventEnvelope {
	if event == nil {
		return nil
	}
	return &evtEventEnvelope{event: event}
}

func NewEVTEventEnvelopeWithDeliverySeq(event *evtv1.Event, seq uint64) EventEnvelope {
	if event == nil {
		return nil
	}
	return &evtEventEnvelope{event: event, deliverySeq: seq}
}

func (e *evtEventEnvelope) ID() string                             { return e.event.GetId() }
func (e *evtEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.event.GetCreatedAt() }
func (e *evtEventEnvelope) ActorID() string                        { return e.event.GetActorId() }
func (e *evtEventEnvelope) Payload() any                           { return e.event.GetEvent() }
func (e *evtEventEnvelope) DeliverySeq() uint64                    { return e.deliverySeq }
func (e *evtEventEnvelope) EVTEvent() *evtv1.Event                 { return e.event }
func (e *evtEventEnvelope) LiveEvent() *livev1.LiveEvent           { return nil }
func (e *evtEventEnvelope) HeartbeatEvent() *livev1.HeartbeatEvent { return nil }

type liveEventEnvelope struct {
	event *livev1.LiveEvent
}

func NewLiveEventEnvelope(event *livev1.LiveEvent) EventEnvelope {
	if event == nil {
		return nil
	}
	return &liveEventEnvelope{event: event}
}

func (e *liveEventEnvelope) ID() string                             { return e.event.GetId() }
func (e *liveEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.event.GetCreatedAt() }
func (e *liveEventEnvelope) ActorID() string                        { return e.event.GetActorId() }
func (e *liveEventEnvelope) Payload() any                           { return e.event.GetEvent() }
func (e *liveEventEnvelope) DeliverySeq() uint64                    { return 0 }
func (e *liveEventEnvelope) EVTEvent() *evtv1.Event                 { return nil }
func (e *liveEventEnvelope) LiveEvent() *livev1.LiveEvent           { return e.event }
func (e *liveEventEnvelope) HeartbeatEvent() *livev1.HeartbeatEvent { return nil }

type heartbeatEventEnvelope struct {
	id        string
	createdAt *timestamppb.Timestamp
	event     *livev1.HeartbeatEvent
}

func NewHeartbeatEventEnvelope(id string, createdAt *timestamppb.Timestamp) EventEnvelope {
	return &heartbeatEventEnvelope{
		id:        id,
		createdAt: createdAt,
		event:     &livev1.HeartbeatEvent{},
	}
}

func (e *heartbeatEventEnvelope) ID() string                             { return e.id }
func (e *heartbeatEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.createdAt }
func (e *heartbeatEventEnvelope) ActorID() string                        { return "" }
func (e *heartbeatEventEnvelope) Payload() any                           { return e.event }
func (e *heartbeatEventEnvelope) DeliverySeq() uint64                    { return 0 }
func (e *heartbeatEventEnvelope) EVTEvent() *evtv1.Event                 { return nil }
func (e *heartbeatEventEnvelope) LiveEvent() *livev1.LiveEvent           { return nil }
func (e *heartbeatEventEnvelope) HeartbeatEvent() *livev1.HeartbeatEvent { return e.event }

func WrapEVTEventEnvelopes(events []*evtv1.Event) []EventEnvelope {
	wrapped := make([]EventEnvelope, 0, len(events))
	for _, event := range events {
		if wrappedEvent := NewEVTEventEnvelope(event); wrappedEvent != nil {
			wrapped = append(wrapped, wrappedEvent)
		}
	}
	return wrapped
}

func EventSessionTerminated(event EventEnvelope) *livev1.SessionTerminatedEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetSessionTerminated()
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

func EventUserTyping(event EventEnvelope) *livev1.UserTypingEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetUserTyping()
}

func EventPresenceChanged(event EventEnvelope) *livev1.PresenceChangedEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetPresenceChanged()
}
