package http_server

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	eventv1 "hmans.de/chatto/internal/pb/chatto/core/event/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestProjectRealtimeEventUsesFreshAuthorizedShape(t *testing.T) {
	plaintext := "public-login"
	source := &evtv1.Event{
		Id:      "event-id",
		ActorId: "actor-id",
		Event: &evtv1.Event_UserLoginChanged{UserLoginChanged: &evtv1.UserLoginChangedEvent{
			UserId: "user-id",
			EncryptedLogin: &evtv1.EncryptedUserString{
				EncryptedValue:  []byte("ciphertext"),
				Nonce:           []byte("nonce"),
				ContentKeyEpoch: 2,
			},
			LoginPlaintext: &plaintext,
		}},
	}
	unknown := protowire.AppendTag(nil, 19000, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	source.ProtoReflect().SetUnknown(unknown)

	projected := projectRealtimeEvent(source)
	if projected == nil || projected == source {
		t.Fatalf("projectRealtimeEvent() = %p, want a fresh public event", projected)
	}
	if projected.GetId() != source.GetId() || projected.GetActorId() != source.GetActorId() {
		t.Fatalf("projected metadata = %+v, want source metadata", projected)
	}
	login := projected.GetUserLoginChanged()
	if login.GetUserId() != "user-id" || login.GetLoginPlaintext() != plaintext {
		t.Fatalf("projected login = %+v, want shared and client-only fields", login)
	}
	if login.GetEncryptedLogin() != nil {
		t.Fatalf("projected login leaked encrypted storage field: %+v", login.GetEncryptedLogin())
	}
	if len(projected.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("projected event retained unknown fields: %x", projected.ProtoReflect().GetUnknown())
	}
}

func TestProjectRealtimeEventOmitsInternalEvent(t *testing.T) {
	event := &evtv1.Event{
		Id: "event-id",
		Event: &evtv1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{
			UserId:       "user-id",
			PasswordHash: []byte("hash"),
		}},
	}

	if projected := projectRealtimeEvent(event); projected != nil {
		t.Fatalf("projectRealtimeEvent() = %+v, want internal event omission", projected)
	}
}

func TestRealtimeEventCatalogueClassifiesTransientAndPublicFields(t *testing.T) {
	descriptor := (&evtv1.Event{}).ProtoReflect().Descriptor()
	eventOneof := descriptor.Oneofs().ByName("event")
	if eventOneof == nil {
		t.Fatal("Event.event descriptor is missing")
	}
	for index := 0; index < eventOneof.Fields().Len(); index++ {
		field := eventOneof.Fields().Get(index)
		dynamicEvent := dynamicpb.NewMessage(descriptor)
		dynamicEvent.Mutable(field)
		wire, err := proto.Marshal(dynamicEvent)
		if err != nil {
			t.Fatalf("marshal %s: %v", field.FullName(), err)
		}
		var event evtv1.Event
		if err := proto.Unmarshal(wire, &event); err != nil {
			t.Fatalf("unmarshal %s: %v", field.FullName(), err)
		}
		isTransient := field.Number() >= 20000 && field.Number() <= 29999
		if isTransient && !isRealtimePublicEvent(&event) {
			t.Errorf("transient event %s is not in the public catalogue", field.FullName())
		}
		if !isRealtimePublicEvent(&event) {
			continue
		}
		payloadFields := field.Message().Fields()
		for payloadIndex := 0; payloadIndex < payloadFields.Len(); payloadIndex++ {
			payloadField := payloadFields.Get(payloadIndex)
			if got := realtimeFieldSurface(payloadField); got == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_UNSPECIFIED {
				t.Errorf("public event field %s has no surface classification", payloadField.FullName())
			}
		}
	}
}
