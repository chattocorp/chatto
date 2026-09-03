package http_server

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	eventv1 "hmans.de/chatto/internal/pb/chatto/core/event/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// projectRealtimeEvent maps one canonical event to the public realtime union.
// The public union field number and payload type must match the canonical
// event. The function never mutates or copies unclassified source fields.
func projectRealtimeEvent(source *evtv1.Event) *realtimev1.PublicEvent {
	if source == nil || source.GetEvent() == nil {
		return nil
	}
	sourceMessage := source.ProtoReflect()
	sourceOneof := sourceMessage.Descriptor().Oneofs().ByName("event")
	if sourceOneof == nil {
		return nil
	}
	sourcePayload := sourceMessage.WhichOneof(sourceOneof)
	if sourcePayload == nil {
		return nil
	}

	target := &realtimev1.PublicEvent{}
	targetMessage := target.ProtoReflect()
	targetPayload := targetMessage.Descriptor().Fields().ByNumber(sourcePayload.Number())
	if targetPayload == nil || sourcePayload.Message() == nil || targetPayload.Message() == nil ||
		targetPayload.Name() != sourcePayload.Name() ||
		targetPayload.ContainingOneof() == nil ||
		targetPayload.ContainingOneof().Name() != "event" ||
		targetPayload.Message().FullName() != sourcePayload.Message().FullName() {
		return nil
	}

	for _, number := range []protoreflect.FieldNumber{1, 2, 3} {
		sourceField := sourceMessage.Descriptor().Fields().ByNumber(number)
		targetField := targetMessage.Descriptor().Fields().ByNumber(number)
		if !matchingRealtimeField(sourceField, targetField) {
			return nil
		}
		if !sourceMessage.Has(sourceField) {
			continue
		}
		copyRealtimeField(targetMessage, targetField, sourceMessage.Get(sourceField), true)
	}
	payload := targetMessage.NewField(targetPayload).Message()
	copyRealtimeMessage(sourceMessage.Get(sourcePayload).Message(), payload, false)
	targetMessage.Set(targetPayload, protoreflect.ValueOfMessage(payload))
	return target
}

func matchingRealtimeField(source, target protoreflect.FieldDescriptor) bool {
	if source == nil || target == nil ||
		source.Name() != target.Name() ||
		source.Kind() != target.Kind() ||
		source.Cardinality() != target.Cardinality() ||
		source.HasPresence() != target.HasPresence() {
		return false
	}
	if source.Message() == nil || target.Message() == nil {
		return source.Message() == nil && target.Message() == nil
	}
	return source.Message().FullName() == target.Message().FullName()
}

func copyRealtimeMessage(source, target protoreflect.Message, inheritUnspecified bool) {
	source.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
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
			copyRealtimeMessage(in.Get(index).Message(), item, inheritUnspecified)
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
			copyRealtimeMessage(item.Message(), mapped, inheritUnspecified)
			out.Set(key, protoreflect.ValueOfMessage(mapped))
			return true
		})
		return
	}
	if field.Message() != nil {
		message := target.NewField(field).Message()
		copyRealtimeMessage(value.Message(), message, inheritUnspecified)
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
