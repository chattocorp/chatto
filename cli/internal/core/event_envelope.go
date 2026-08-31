package core

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
)

// EventEnvelope is the in-process envelope used by StreamMyEvents and the
// realtime API. Concrete implementations are intentionally private so an
// envelope can only wrap one backing source: a durable EVT fact, a transient
// canonical Event, a legacy LiveEvent during rolling replacement, or a
// synthetic heartbeat.
type EventEnvelope interface {
	ID() string
	CreatedAt() *timestamppb.Timestamp
	ActorID() string
	Payload() any
	DeliverySeq() uint64
	// CanonicalEvent returns the single Event shape used by durable and
	// transient delivery. Callers must treat the returned value as immutable.
	CanonicalEvent() *evtv1.Event

	EVTEvent() *evtv1.Event
	LiveEvent() *livev1.LiveEvent
	HeartbeatEvent() *livev1.HeartbeatEvent
}

type evtEventEnvelope struct {
	event       *evtv1.Event
	deliverySeq uint64
	transient   bool
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

// NewTransientEventEnvelope wraps a canonical Event that must not enter EVT.
func NewTransientEventEnvelope(event *evtv1.Event) EventEnvelope {
	if event == nil {
		return nil
	}
	return &evtEventEnvelope{event: event, transient: true}
}

func (e *evtEventEnvelope) ID() string                        { return e.event.GetId() }
func (e *evtEventEnvelope) CreatedAt() *timestamppb.Timestamp { return e.event.GetCreatedAt() }
func (e *evtEventEnvelope) ActorID() string                   { return e.event.GetActorId() }
func (e *evtEventEnvelope) Payload() any                      { return e.event.GetEvent() }
func (e *evtEventEnvelope) DeliverySeq() uint64               { return e.deliverySeq }
func (e *evtEventEnvelope) CanonicalEvent() *evtv1.Event      { return e.event }
func (e *evtEventEnvelope) EVTEvent() *evtv1.Event {
	if e.transient {
		return nil
	}
	return e.event
}
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
func (e *liveEventEnvelope) CanonicalEvent() *evtv1.Event           { return CanonicalEventFromLive(e.event) }
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
func (e *heartbeatEventEnvelope) CanonicalEvent() *evtv1.Event           { return nil }
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
	if event == nil || event.CanonicalEvent() == nil {
		return nil
	}
	return event.CanonicalEvent().GetSessionTerminatedSignal()
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
	if event == nil || event.CanonicalEvent() == nil {
		return nil
	}
	return event.CanonicalEvent().GetUserTypingSignal()
}

func EventPresenceChanged(event EventEnvelope) *livev1.PresenceChangedEvent {
	if event == nil || event.CanonicalEvent() == nil {
		return nil
	}
	return event.CanonicalEvent().GetPresenceChangedSignal()
}

// CanonicalEventFromLive translates the read-only rolling-upgrade LiveEvent
// envelope into the canonical Event envelope. The transient payload messages
// themselves are reused without changing their wire contract.
func CanonicalEventFromLive(live *livev1.LiveEvent) *evtv1.Event {
	if live == nil || live.GetEvent() == nil {
		return nil
	}
	event := &evtv1.Event{Id: live.GetId(), CreatedAt: live.GetCreatedAt(), ActorId: live.GetActorId()}
	switch payload := live.GetEvent().(type) {
	case *livev1.LiveEvent_UserCreated:
		event.Event = &evtv1.Event_UserCreatedSync{UserCreatedSync: payload.UserCreated}
	case *livev1.LiveEvent_UserProfileUpdated:
		event.Event = &evtv1.Event_UserProfileSync{UserProfileSync: payload.UserProfileUpdated}
	case *livev1.LiveEvent_ServerUserPreferencesUpdated:
		event.Event = &evtv1.Event_ServerUserPreferencesSync{ServerUserPreferencesSync: payload.ServerUserPreferencesUpdated}
	case *livev1.LiveEvent_ThreadFollowChanged:
		event.Event = &evtv1.Event_ThreadFollowChangedSync{ThreadFollowChangedSync: payload.ThreadFollowChanged}
	case *livev1.LiveEvent_ServerMemberDeleted:
		event.Event = &evtv1.Event_ServerMemberDeletedSync{ServerMemberDeletedSync: payload.ServerMemberDeleted}
	case *livev1.LiveEvent_ServerUpdated:
		event.Event = &evtv1.Event_ServerUpdatedSync{ServerUpdatedSync: payload.ServerUpdated}
	case *livev1.LiveEvent_UserTyping:
		event.Event = &evtv1.Event_UserTypingSignal{UserTypingSignal: payload.UserTyping}
	case *livev1.LiveEvent_PresenceChanged:
		event.Event = &evtv1.Event_PresenceChangedSignal{PresenceChangedSignal: payload.PresenceChanged}
	case *livev1.LiveEvent_CallParticipantJoined:
		event.Event = &evtv1.Event_CallParticipantJoinedSignal{CallParticipantJoinedSignal: payload.CallParticipantJoined}
	case *livev1.LiveEvent_CallParticipantLeft:
		event.Event = &evtv1.Event_CallParticipantLeftSignal{CallParticipantLeftSignal: payload.CallParticipantLeft}
	case *livev1.LiveEvent_NotificationOccurrencesInvalidated:
		event.Event = &evtv1.Event_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: payload.NotificationOccurrencesInvalidated}
	case *livev1.LiveEvent_NotificationUnreadChanged:
		event.Event = &evtv1.Event_NotificationUnreadChanged{NotificationUnreadChanged: payload.NotificationUnreadChanged}
	case *livev1.LiveEvent_RoomMarkedAsRead:
		event.Event = &evtv1.Event_RoomMarkedAsReadSync{RoomMarkedAsReadSync: payload.RoomMarkedAsRead}
	case *livev1.LiveEvent_MentionStatusCleared:
		event.Event = &evtv1.Event_MentionStatusClearedSync{MentionStatusClearedSync: payload.MentionStatusCleared}
	case *livev1.LiveEvent_RoomGroupsUpdated:
		event.Event = &evtv1.Event_RoomGroupsUpdatedSync{RoomGroupsUpdatedSync: payload.RoomGroupsUpdated}
	case *livev1.LiveEvent_SessionTerminated:
		event.Event = &evtv1.Event_SessionTerminatedSignal{SessionTerminatedSignal: payload.SessionTerminated}
	default:
		return nil
	}
	return event
}

// legacyLiveEventFromCanonical creates the temporary rolling-replacement view
// of a transient canonical Event. Remove it with the legacy LiveEvent reader
// after the mixed-version compatibility window ends.
func legacyLiveEventFromCanonical(event *evtv1.Event) *livev1.LiveEvent {
	if event == nil || event.GetEvent() == nil {
		return nil
	}
	legacy := &livev1.LiveEvent{Id: event.GetId(), CreatedAt: event.GetCreatedAt(), ActorId: event.GetActorId()}
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_UserCreatedSync:
		legacy.Event = &livev1.LiveEvent_UserCreated{UserCreated: payload.UserCreatedSync}
	case *evtv1.Event_UserProfileSync:
		legacy.Event = &livev1.LiveEvent_UserProfileUpdated{UserProfileUpdated: payload.UserProfileSync}
	case *evtv1.Event_ServerUserPreferencesSync:
		legacy.Event = &livev1.LiveEvent_ServerUserPreferencesUpdated{ServerUserPreferencesUpdated: payload.ServerUserPreferencesSync}
	case *evtv1.Event_ThreadFollowChangedSync:
		legacy.Event = &livev1.LiveEvent_ThreadFollowChanged{ThreadFollowChanged: payload.ThreadFollowChangedSync}
	case *evtv1.Event_ServerMemberDeletedSync:
		legacy.Event = &livev1.LiveEvent_ServerMemberDeleted{ServerMemberDeleted: payload.ServerMemberDeletedSync}
	case *evtv1.Event_ServerUpdatedSync:
		legacy.Event = &livev1.LiveEvent_ServerUpdated{ServerUpdated: payload.ServerUpdatedSync}
	case *evtv1.Event_UserTypingSignal:
		legacy.Event = &livev1.LiveEvent_UserTyping{UserTyping: payload.UserTypingSignal}
	case *evtv1.Event_PresenceChangedSignal:
		legacy.Event = &livev1.LiveEvent_PresenceChanged{PresenceChanged: payload.PresenceChangedSignal}
	case *evtv1.Event_CallParticipantJoinedSignal:
		legacy.Event = &livev1.LiveEvent_CallParticipantJoined{CallParticipantJoined: payload.CallParticipantJoinedSignal}
	case *evtv1.Event_CallParticipantLeftSignal:
		legacy.Event = &livev1.LiveEvent_CallParticipantLeft{CallParticipantLeft: payload.CallParticipantLeftSignal}
	case *evtv1.Event_NotificationOccurrencesInvalidated:
		legacy.Event = &livev1.LiveEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: payload.NotificationOccurrencesInvalidated}
	case *evtv1.Event_NotificationUnreadChanged:
		legacy.Event = &livev1.LiveEvent_NotificationUnreadChanged{NotificationUnreadChanged: payload.NotificationUnreadChanged}
	case *evtv1.Event_RoomMarkedAsReadSync:
		legacy.Event = &livev1.LiveEvent_RoomMarkedAsRead{RoomMarkedAsRead: payload.RoomMarkedAsReadSync}
	case *evtv1.Event_MentionStatusClearedSync:
		legacy.Event = &livev1.LiveEvent_MentionStatusCleared{MentionStatusCleared: payload.MentionStatusClearedSync}
	case *evtv1.Event_RoomGroupsUpdatedSync:
		legacy.Event = &livev1.LiveEvent_RoomGroupsUpdated{RoomGroupsUpdated: payload.RoomGroupsUpdatedSync}
	case *evtv1.Event_SessionTerminatedSignal:
		legacy.Event = &livev1.LiveEvent_SessionTerminated{SessionTerminated: payload.SessionTerminatedSignal}
	default:
		return nil
	}
	return legacy
}
