package core

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// EventEnvelope is the in-process envelope used by StreamMyEvents and the
// GraphQL Event type. Concrete implementations are intentionally private so an
// envelope can only wrap one backing source: a durable EVT fact, a transient
// LiveEvent, or a synthetic heartbeat.
type EventEnvelope interface {
	ID() string
	CreatedAt() *timestamppb.Timestamp
	ActorID() string
	Payload() any

	EVTEvent() *corev1.Event
	LiveEvent() *corev1.LiveEvent
	HeartbeatEvent() *corev1.HeartbeatEvent
}

type evtEventEnvelope struct {
	event *corev1.Event
}

func NewEVTEventEnvelope(event *corev1.Event) EventEnvelope {
	if event == nil {
		return nil
	}
	return &evtEventEnvelope{event: event}
}

func (e *evtEventEnvelope) ID() string                             { return e.event.GetId() }
func (e *evtEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.event.GetCreatedAt() }
func (e *evtEventEnvelope) ActorID() string                        { return e.event.GetActorId() }
func (e *evtEventEnvelope) Payload() any                           { return e.event.GetEvent() }
func (e *evtEventEnvelope) EVTEvent() *corev1.Event                { return e.event }
func (e *evtEventEnvelope) LiveEvent() *corev1.LiveEvent           { return nil }
func (e *evtEventEnvelope) HeartbeatEvent() *corev1.HeartbeatEvent { return nil }

type liveEventEnvelope struct {
	event *corev1.LiveEvent
}

func NewLiveEventEnvelope(event *corev1.LiveEvent) EventEnvelope {
	if event == nil {
		return nil
	}
	return &liveEventEnvelope{event: event}
}

func (e *liveEventEnvelope) ID() string                             { return e.event.GetId() }
func (e *liveEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.event.GetCreatedAt() }
func (e *liveEventEnvelope) ActorID() string                        { return e.event.GetActorId() }
func (e *liveEventEnvelope) Payload() any                           { return e.event.GetEvent() }
func (e *liveEventEnvelope) EVTEvent() *corev1.Event                { return nil }
func (e *liveEventEnvelope) LiveEvent() *corev1.LiveEvent           { return e.event }
func (e *liveEventEnvelope) HeartbeatEvent() *corev1.HeartbeatEvent { return nil }

type heartbeatEventEnvelope struct {
	id        string
	createdAt *timestamppb.Timestamp
	event     *corev1.HeartbeatEvent
}

func NewHeartbeatEventEnvelope(id string, createdAt *timestamppb.Timestamp) EventEnvelope {
	return &heartbeatEventEnvelope{
		id:        id,
		createdAt: createdAt,
		event:     &corev1.HeartbeatEvent{},
	}
}

func (e *heartbeatEventEnvelope) ID() string                             { return e.id }
func (e *heartbeatEventEnvelope) CreatedAt() *timestamppb.Timestamp      { return e.createdAt }
func (e *heartbeatEventEnvelope) ActorID() string                        { return "" }
func (e *heartbeatEventEnvelope) Payload() any                           { return e.event }
func (e *heartbeatEventEnvelope) EVTEvent() *corev1.Event                { return nil }
func (e *heartbeatEventEnvelope) LiveEvent() *corev1.LiveEvent           { return nil }
func (e *heartbeatEventEnvelope) HeartbeatEvent() *corev1.HeartbeatEvent { return e.event }

func WrapEVTEventEnvelopes(events []*corev1.Event) []EventEnvelope {
	wrapped := make([]EventEnvelope, 0, len(events))
	for _, event := range events {
		if wrappedEvent := NewEVTEventEnvelope(event); wrappedEvent != nil {
			wrapped = append(wrapped, wrappedEvent)
		}
	}
	return wrapped
}

func EventSessionTerminated(event EventEnvelope) *corev1.SessionTerminatedEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetSessionTerminated()
}

func EventMessagePosted(event EventEnvelope) *corev1.MessagePostedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessagePosted()
}

func EventMessageEdited(event EventEnvelope) *corev1.MessageEditedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessageEdited()
}

func EventMessageRetracted(event EventEnvelope) *corev1.MessageRetractedEvent {
	if event == nil || event.EVTEvent() == nil {
		return nil
	}
	return event.EVTEvent().GetMessageRetracted()
}

func EventUserTyping(event EventEnvelope) *corev1.UserTypingEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetUserTyping()
}

func EventPresenceChanged(event EventEnvelope) *corev1.PresenceChangedEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetPresenceChanged()
}

func EventMentionNotification(event EventEnvelope) *corev1.MentionNotificationEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetMentionNotification()
}

func EventNotificationCreated(event EventEnvelope) *corev1.NotificationCreatedEvent {
	if event == nil || event.LiveEvent() == nil {
		return nil
	}
	return event.LiveEvent().GetNotificationCreated()
}
