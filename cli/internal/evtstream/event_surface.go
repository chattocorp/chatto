package evtstream

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	eventv1 "hmans.de/chatto/internal/pb/chatto/core/event/v1"
)

func validateStoredEventFields(message protoreflect.Message, path string) error {
	var validationErr error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		fieldPath := string(field.Name())
		if path != "" {
			fieldPath = path + "." + fieldPath
		}
		if storedEventFieldSurface(field) == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_CLIENT_ONLY {
			validationErr = fmt.Errorf("%w: client-only field %s is populated", ErrInvalidEvent, fieldPath)
			return false
		}
		if field.IsList() && field.Message() != nil {
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				if err := validateStoredEventFields(list.Get(index).Message(), fmt.Sprintf("%s[%d]", fieldPath, index)); err != nil {
					validationErr = err
					return false
				}
			}
		} else if field.IsMap() && field.MapValue().Message() != nil {
			value.Map().Range(func(key protoreflect.MapKey, item protoreflect.Value) bool {
				if err := validateStoredEventFields(item.Message(), fmt.Sprintf("%s[%v]", fieldPath, key.Interface())); err != nil {
					validationErr = err
					return false
				}
				return true
			})
		} else if field.Message() != nil {
			validationErr = validateStoredEventFields(value.Message(), fieldPath)
		}
		return validationErr == nil
	})
	return validationErr
}

func storedEventFieldSurface(field protoreflect.FieldDescriptor) eventv1.EventFieldSurface {
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
