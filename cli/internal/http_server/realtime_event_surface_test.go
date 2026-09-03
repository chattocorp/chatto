package http_server

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	eventv1 "hmans.de/chatto/internal/pb/chatto/core/event/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
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
	if projected == nil {
		t.Fatal("projectRealtimeEvent() = nil, want a fresh public event")
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

func TestProjectRealtimeEventCarriesClientOnlyMessagePlaintext(t *testing.T) {
	bodyPlaintext := "public message"
	projected := projectRealtimeEvent(&evtv1.Event{
		Id: "message-id",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId:        "room-id",
			BodyPlaintext: &bodyPlaintext,
		}},
	})
	if projected.GetMessagePosted().GetBodyPlaintext() != bodyPlaintext {
		t.Fatalf("body_plaintext = %q, want authorized plaintext", projected.GetMessagePosted().GetBodyPlaintext())
	}
}

func TestProjectRealtimeEventKeepsCanonicalWireEncoding(t *testing.T) {
	bodyPlaintext := "public message"
	canonical := &evtv1.Event{
		Id:      "message-id",
		ActorId: "actor-id",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId:        "room-id",
			BodyPlaintext: &bodyPlaintext,
		}},
	}
	public := projectRealtimeEvent(canonical)
	canonicalWire, err := proto.MarshalOptions{Deterministic: true}.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
	}
	publicWire, err := proto.MarshalOptions{Deterministic: true}.Marshal(public)
	if err != nil {
		t.Fatalf("marshal public event: %v", err)
	}
	if string(publicWire) != string(canonicalWire) {
		t.Fatalf("public wire = %x, want canonical wire %x", publicWire, canonicalWire)
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

func TestRealtimePublicEventUnknownPayloadKeepsMetadataAndCursor(t *testing.T) {
	publicEvent := &realtimev1.PublicEvent{Id: "event-id", ActorId: "actor-id"}
	unknown := protowire.AppendTag(nil, 25000, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, nil)
	publicEvent.ProtoReflect().SetUnknown(unknown)
	cursor := "opaque-cursor"
	wire, err := proto.Marshal(&realtimev1.RealtimeEvent{Event: publicEvent, ResumeCursor: &cursor})
	if err != nil {
		t.Fatalf("marshal RealtimeEvent: %v", err)
	}

	var decoded realtimev1.RealtimeEvent
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal RealtimeEvent: %v", err)
	}
	if decoded.GetEvent().GetId() != "event-id" || decoded.GetEvent().GetActorId() != "actor-id" {
		t.Fatalf("decoded metadata = %+v, want public event metadata", decoded.GetEvent())
	}
	if decoded.GetEvent().GetEvent() != nil {
		t.Fatalf("decoded payload = %T, want unknown variant to remain unset", decoded.GetEvent().GetEvent())
	}
	if decoded.GetResumeCursor() != cursor {
		t.Fatalf("decoded cursor = %q, want %q", decoded.GetResumeCursor(), cursor)
	}
}

func TestRealtimeEventCatalogueClassifiesTransientAndPublicFields(t *testing.T) {
	canonicalDescriptor := (&evtv1.Event{}).ProtoReflect().Descriptor()
	canonicalOneof := canonicalDescriptor.Oneofs().ByName("event")
	publicDescriptor := (&realtimev1.PublicEvent{}).ProtoReflect().Descriptor()
	publicOneof := publicDescriptor.Oneofs().ByName("event")
	if canonicalOneof == nil || publicOneof == nil {
		t.Fatal("Event.event descriptor is missing")
	}
	for _, number := range []protoreflect.FieldNumber{1, 2, 3} {
		canonicalField := canonicalDescriptor.Fields().ByNumber(number)
		publicField := publicDescriptor.Fields().ByNumber(number)
		if !matchingRealtimeField(canonicalField, publicField) {
			t.Errorf("public metadata field %d does not match the canonical field", number)
		}
	}
	for index := 0; index < publicOneof.Fields().Len(); index++ {
		publicField := publicOneof.Fields().Get(index)
		canonicalField := canonicalOneof.Fields().ByNumber(publicField.Number())
		if canonicalField == nil || canonicalField.Name() != publicField.Name() ||
			canonicalField.Message().FullName() != publicField.Message().FullName() {
			t.Errorf("public field %s does not match canonical field %d", publicField.FullName(), publicField.Number())
			continue
		}
		dynamicEvent := dynamicpb.NewMessage(canonicalDescriptor)
		dynamicEvent.Set(canonicalField, dynamicEvent.NewField(canonicalField))
		wire, err := proto.Marshal(dynamicEvent)
		if err != nil {
			t.Fatalf("marshal %s: %v", canonicalField.FullName(), err)
		}
		var event evtv1.Event
		if err := proto.Unmarshal(wire, &event); err != nil {
			t.Fatalf("unmarshal %s: %v", canonicalField.FullName(), err)
		}
		projected := projectRealtimeEvent(&event)
		if projected == nil {
			t.Errorf("public event %s was not projected", publicField.FullName())
			continue
		}
		projectedField := projected.ProtoReflect().WhichOneof(publicOneof)
		if projectedField == nil || projectedField.Number() != publicField.Number() {
			t.Errorf("projected field = %v, want %s", projectedField, publicField.FullName())
		}
		payloadFields := publicField.Message().Fields()
		for payloadIndex := 0; payloadIndex < payloadFields.Len(); payloadIndex++ {
			payloadField := payloadFields.Get(payloadIndex)
			if got := realtimeFieldSurface(payloadField); got == eventv1.EventFieldSurface_EVENT_FIELD_SURFACE_UNSPECIFIED {
				t.Errorf("public event field %s has no surface classification", payloadField.FullName())
			}
		}
	}
	if publicOneof.Fields().Len() < 40 {
		t.Fatalf("public event catalogue contains %d variants, want at least 40", publicOneof.Fields().Len())
	}
	for index := 0; index < canonicalOneof.Fields().Len(); index++ {
		field := canonicalOneof.Fields().Get(index)
		if field.Number() >= 20000 && field.Number() <= 29999 && publicOneof.Fields().ByNumber(field.Number()) == nil {
			t.Errorf("transient event %s is not in the public catalogue", field.FullName())
		}
	}
}
