package http_server

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	eventv1 "hmans.de/chatto/internal/pb/chatto/core/event/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// projectRealtimeEvent creates a new public event. It never mutates or clones
// unclassified fields from the stored or transient source event.
func projectRealtimeEvent(source *evtv1.Event) *evtv1.Event {
	if !isRealtimePublicEvent(source) {
		return nil
	}
	target := &evtv1.Event{}
	copyRealtimeMessage(source.ProtoReflect(), target.ProtoReflect(), true, false)
	return target
}

func copyRealtimeMessage(source, target protoreflect.Message, eventEnvelope, inheritUnspecified bool) {
	source.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if eventEnvelope && field.ContainingOneof() != nil && field.ContainingOneof().Name() == "event" {
			payload := target.NewField(field).Message()
			copyRealtimeMessage(value.Message(), payload, false, false)
			target.Set(field, protoreflect.ValueOfMessage(payload))
			return true
		}
		surface := realtimeFieldSurface(field)
		allowed := surface == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_SHARED ||
			surface == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_CLIENT_ONLY ||
			(surface == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_UNSPECIFIED && inheritUnspecified)
		if !allowed {
			return true
		}
		copyRealtimeField(target, field, value, true)
		return true
	})
}

func realtimeFieldSurface(field protoreflect.FieldDescriptor) eventv1.EventFieldSurface {
	options, ok := field.Options().(*descriptorpb.FieldOptions)
	if !ok || !proto.HasExtension(options, eventv1.E_EventFieldSurface) {
		return eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_UNSPECIFIED
	}
	value := proto.GetExtension(options, eventv1.E_EventFieldSurface)
	switch surface := value.(type) {
	case eventv1.EventFieldSurface:
		return surface
	case *eventv1.EventFieldSurface:
		if surface != nil {
			return *surface
		}
	}
	return eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_UNSPECIFIED
}

func copyRealtimeField(target protoreflect.Message, field protoreflect.FieldDescriptor, value protoreflect.Value, inheritUnspecified bool) {
	if field.IsList() {
		out := target.Mutable(field).List()
		in := value.List()
		for index := 0; index < in.Len(); index++ {
			if field.Message() == nil {
				out.Append(cloneRealtimeScalar(field, in.Get(index)))
				continue
			}
			item := out.NewElement().Message()
			copyRealtimeMessage(in.Get(index).Message(), item, false, inheritUnspecified)
			out.Append(protoreflect.ValueOfMessage(item))
		}
		return
	}
	if field.IsMap() {
		out := target.Mutable(field).Map()
		value.Map().Range(func(key protoreflect.MapKey, item protoreflect.Value) bool {
			if field.MapValue().Message() == nil {
				out.Set(key, cloneRealtimeScalar(field.MapValue(), item))
				return true
			}
			mapped := out.NewValue().Message()
			copyRealtimeMessage(item.Message(), mapped, false, inheritUnspecified)
			out.Set(key, protoreflect.ValueOfMessage(mapped))
			return true
		})
		return
	}
	if field.Message() != nil {
		message := target.NewField(field).Message()
		copyRealtimeMessage(value.Message(), message, false, inheritUnspecified)
		target.Set(field, protoreflect.ValueOfMessage(message))
		return
	}
	target.Set(field, cloneRealtimeScalar(field, value))
}

func cloneRealtimeScalar(field protoreflect.FieldDescriptor, value protoreflect.Value) protoreflect.Value {
	if field.Kind() == protoreflect.BytesKind {
		return protoreflect.ValueOfBytes(append([]byte(nil), value.Bytes()...))
	}
	return value
}

func isRealtimePublicEvent(event *evtv1.Event) bool {
	if event == nil || event.GetEvent() == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted,
		*evtv1.Event_MessageEdited,
		*evtv1.Event_MessageRetracted,
		*evtv1.Event_MessagePinned,
		*evtv1.Event_MessageUnpinned,
		*evtv1.Event_ReactionAdded,
		*evtv1.Event_ReactionRemoved,
		*evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_AssetDeleted,
		*evtv1.Event_VoiceCallStarted,
		*evtv1.Event_VoiceCallParticipantJoined,
		*evtv1.Event_VoiceCallParticipantLeft,
		*evtv1.Event_VoiceCallEnded,
		*evtv1.Event_RoomCreated,
		*evtv1.Event_RoomUpdated,
		*evtv1.Event_RoomDeleted,
		*evtv1.Event_RoomArchived,
		*evtv1.Event_RoomUnarchived,
		*evtv1.Event_RoomUniversalChanged,
		*evtv1.Event_RoomSlowModeChanged,
		*evtv1.Event_RoomThreadingModeChanged,
		*evtv1.Event_UserJoinedRoom,
		*evtv1.Event_UserLeftRoom,
		*evtv1.Event_RoomMemberAdded,
		*evtv1.Event_RoomMemberRemoved,
		*evtv1.Event_RoomMemberBanned,
		*evtv1.Event_RoomMemberUnbanned,
		*evtv1.Event_ThreadCreated,
		*evtv1.Event_UserAccountCreated,
		*evtv1.Event_UserCustomStatusSet,
		*evtv1.Event_UserCustomStatusCleared,
		*evtv1.Event_UserLoginChanged,
		*evtv1.Event_UserDisplayNameChanged,
		*evtv1.Event_UserBioChanged,
		*evtv1.Event_UserAvatarSet,
		*evtv1.Event_UserAvatarCleared,
		*evtv1.Event_UserAccountDeleted,
		*evtv1.Event_UserCreatedSync,
		*evtv1.Event_UserProfileSync,
		*evtv1.Event_ServerUserPreferencesSync,
		*evtv1.Event_ThreadFollowChangedSync,
		*evtv1.Event_ServerMemberDeletedSync,
		*evtv1.Event_ServerUpdatedSync,
		*evtv1.Event_UserTypingSignal,
		*evtv1.Event_PresenceChangedSignal,
		*evtv1.Event_CallParticipantJoinedSignal,
		*evtv1.Event_CallParticipantLeftSignal,
		*evtv1.Event_NotificationOccurrencesInvalidated,
		*evtv1.Event_NotificationUnreadChanged,
		*evtv1.Event_RoomMarkedAsReadSync,
		*evtv1.Event_MentionStatusClearedSync,
		*evtv1.Event_RoomGroupsUpdatedSync,
		*evtv1.Event_SessionTerminatedSignal:
		return true
	default:
		return false
	}
}
