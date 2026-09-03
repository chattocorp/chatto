package http_server

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// projectRealtimeEvent maps one canonical event to the public realtime
// catalogue. Public payload messages contain no storage-only fields. Wire
// projection copies only fields that the public destination declares and
// discards all unknown source data.
func projectRealtimeEvent(source *evtv1.Event, plaintext *core.EventPlaintext) (*realtimev1.RealtimeEvent, error) {
	sourceMessage, sourcePayload, targetPayload, ok := realtimePublicPayload(source)
	if !ok {
		return nil, nil
	}

	target := &realtimev1.RealtimeEvent{
		Id:        source.GetId(),
		CreatedAt: source.GetCreatedAt(),
		ActorId:   source.GetActorId(),
	}
	targetMessage := target.ProtoReflect()
	publicPayload := targetMessage.NewField(targetPayload).Message()
	wire, err := proto.Marshal(sourceMessage.Get(sourcePayload).Message().Interface())
	if err != nil {
		return nil, fmt.Errorf("marshal canonical realtime payload %s: %w", sourcePayload.FullName(), err)
	}
	if err := (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(wire, publicPayload.Interface()); err != nil {
		return nil, fmt.Errorf("map canonical realtime payload %s: %w", sourcePayload.FullName(), err)
	}
	targetMessage.Set(targetPayload, protoreflect.ValueOfMessage(publicPayload))
	applyRealtimePlaintext(target, plaintext)
	return target, nil
}

func applyRealtimePlaintext(target *realtimev1.RealtimeEvent, plaintext *core.EventPlaintext) {
	if target == nil {
		return
	}
	switch payload := target.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_MessagePosted:
		payload.MessagePosted.BodyPlaintext = nil
		if plaintext != nil {
			payload.MessagePosted.BodyPlaintext = plaintext.MessageBody
		}
	case *realtimev1.RealtimeEvent_UserAccountCreated:
		payload.UserAccountCreated.LoginPlaintext = nil
		payload.UserAccountCreated.DisplayNamePlaintext = nil
		if plaintext != nil {
			payload.UserAccountCreated.LoginPlaintext = plaintext.Login
			payload.UserAccountCreated.DisplayNamePlaintext = plaintext.DisplayName
		}
	case *realtimev1.RealtimeEvent_UserLoginChanged:
		payload.UserLoginChanged.LoginPlaintext = nil
		if plaintext != nil {
			payload.UserLoginChanged.LoginPlaintext = plaintext.Login
		}
	case *realtimev1.RealtimeEvent_UserDisplayNameChanged:
		payload.UserDisplayNameChanged.DisplayNamePlaintext = nil
		if plaintext != nil {
			payload.UserDisplayNameChanged.DisplayNamePlaintext = plaintext.DisplayName
		}
	case *realtimev1.RealtimeEvent_UserBioChanged:
		payload.UserBioChanged.BioPlaintext = nil
		if plaintext != nil {
			payload.UserBioChanged.BioPlaintext = plaintext.Bio
		}
	}
}

func hasRealtimePublicVariant(source *evtv1.Event) bool {
	_, _, _, ok := realtimePublicPayload(source)
	return ok
}

func realtimePublicPayload(source *evtv1.Event) (protoreflect.Message, protoreflect.FieldDescriptor, protoreflect.FieldDescriptor, bool) {
	if source == nil || source.GetEvent() == nil {
		return nil, nil, nil, false
	}
	sourceMessage := source.ProtoReflect()
	sourceOneof := sourceMessage.Descriptor().Oneofs().ByName("event")
	if sourceOneof == nil {
		return nil, nil, nil, false
	}
	sourcePayload := sourceMessage.WhichOneof(sourceOneof)
	if sourcePayload == nil {
		return nil, nil, nil, false
	}
	targetPayload := (&realtimev1.RealtimeEvent{}).ProtoReflect().Descriptor().Fields().ByNumber(sourcePayload.Number())
	if targetPayload == nil || sourcePayload.Message() == nil || targetPayload.Message() == nil ||
		targetPayload.Name() != sourcePayload.Name() ||
		targetPayload.ContainingOneof() == nil ||
		targetPayload.ContainingOneof().Name() != "event" {
		return nil, nil, nil, false
	}
	return sourceMessage, sourcePayload, targetPayload, true
}
