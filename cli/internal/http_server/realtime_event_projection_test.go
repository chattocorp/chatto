package http_server

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestProjectRealtimeEventUsesDedicatedPublicShape(t *testing.T) {
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
		}},
	}
	unknown := protowire.AppendTag(nil, 19000, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	unknown = protowire.AppendTag(unknown, 11, protowire.BytesType)
	unknown = protowire.AppendString(unknown, "untrusted-source-plaintext")
	source.GetUserLoginChanged().ProtoReflect().SetUnknown(unknown)

	projected, err := projectRealtimeEvent(source, &core.EventPlaintext{Login: &plaintext})
	if err != nil {
		t.Fatalf("projectRealtimeEvent() error = %v", err)
	}
	if projected == nil {
		t.Fatal("projectRealtimeEvent() = nil, want a public event")
	}
	if projected.GetId() != source.GetId() || projected.GetActorId() != source.GetActorId() {
		t.Fatalf("projected metadata = %+v, want source metadata", projected)
	}
	login := projected.GetUserLoginChanged()
	if login.GetUserId() != "user-id" || login.GetLoginPlaintext() != plaintext {
		t.Fatalf("projected login = %+v, want public fields", login)
	}
	if field := login.ProtoReflect().Descriptor().Fields().ByName("encrypted_login"); field != nil {
		t.Fatalf("public login schema contains storage field %s", field.FullName())
	}
	if len(login.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("projected login retained unknown fields: %x", login.ProtoReflect().GetUnknown())
	}

	withoutPlaintext, err := projectRealtimeEvent(source, nil)
	if err != nil {
		t.Fatalf("projectRealtimeEvent() without plaintext error = %v", err)
	}
	plaintextField := withoutPlaintext.GetUserLoginChanged().ProtoReflect().Descriptor().Fields().ByName("login_plaintext")
	if withoutPlaintext.GetUserLoginChanged().ProtoReflect().Has(plaintextField) {
		t.Fatal("public login retained an untrusted source value in login_plaintext")
	}
}

func TestProjectRealtimeEventCarriesMessagePlaintext(t *testing.T) {
	bodyPlaintext := "public message"
	projected, err := projectRealtimeEvent(&evtv1.Event{
		Id: "message-id",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId: "room-id",
		}},
	}, &core.EventPlaintext{MessageBody: &bodyPlaintext})
	if err != nil {
		t.Fatalf("projectRealtimeEvent() error = %v", err)
	}
	if projected.GetMessagePosted().GetBodyPlaintext() != bodyPlaintext {
		t.Fatalf("body_plaintext = %q, want %q", projected.GetMessagePosted().GetBodyPlaintext(), bodyPlaintext)
	}
}

func TestProjectRealtimeEventKeepsCompatibleWireForSharedPayload(t *testing.T) {
	canonical := &evtv1.Event{
		Id:      "room-event-id",
		ActorId: "actor-id",
		Event: &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{
			RoomId:      "room-id",
			Name:        "Room",
			Description: "Description",
		}},
	}
	public, err := projectRealtimeEvent(canonical, nil)
	if err != nil {
		t.Fatalf("projectRealtimeEvent() error = %v", err)
	}
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

func TestProjectRealtimeEventDoesNotExposeLegacyAvatarStoragePointer(t *testing.T) {
	projected, err := projectRealtimeEvent(&evtv1.Event{
		Id: "avatar-event-id",
		Event: &evtv1.Event_UserAvatarSet{UserAvatarSet: &evtv1.UserAvatarSetEvent{
			UserId: "user-id",
			Avatar: &evtv1.DeprecatedAsset{Asset: &evtv1.DeprecatedAsset_S3{S3: &evtv1.S3Asset{
				Key:    "private/key",
				Bucket: proto.String("private-bucket"),
			}}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("projectRealtimeEvent() error = %v", err)
	}
	if projected.GetUserAvatarSet().GetUserId() != "user-id" {
		t.Fatalf("user_avatar_set = %+v, want user ID", projected.GetUserAvatarSet())
	}
	if fields := projected.GetUserAvatarSet().ProtoReflect().Descriptor().Fields(); fields.Len() != 1 {
		t.Fatalf("public avatar payload has %d fields, want only user_id", fields.Len())
	}
	if wire, err := proto.Marshal(projected); err != nil {
		t.Fatalf("marshal projected event: %v", err)
	} else if containsBytes(wire, []byte("private/key")) || containsBytes(wire, []byte("private-bucket")) {
		t.Fatalf("public avatar event contains storage coordinates: %x", wire)
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

	projected, err := projectRealtimeEvent(event, nil)
	if err != nil {
		t.Fatalf("projectRealtimeEvent() error = %v", err)
	}
	if projected != nil {
		t.Fatalf("projectRealtimeEvent() = %+v, want internal event omission", projected)
	}
}

func TestRealtimeEventUnknownPayloadKeepsMetadataAndCursor(t *testing.T) {
	publicEvent := &realtimev1.RealtimeEvent{Id: "event-id", ActorId: "actor-id"}
	unknown := protowire.AppendTag(nil, 25000, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, nil)
	publicEvent.ProtoReflect().SetUnknown(unknown)
	cursor := "opaque-cursor"
	publicEvent.ResumeCursor = &cursor
	wire, err := proto.Marshal(publicEvent)
	if err != nil {
		t.Fatalf("marshal RealtimeEvent: %v", err)
	}

	var decoded realtimev1.RealtimeEvent
	if err := proto.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("unmarshal RealtimeEvent: %v", err)
	}
	if decoded.GetId() != "event-id" || decoded.GetActorId() != "actor-id" {
		t.Fatalf("decoded metadata = %+v, want public event metadata", &decoded)
	}
	if decoded.GetEvent() != nil {
		t.Fatalf("decoded payload = %T, want unknown variant to remain unset", decoded.GetEvent())
	}
	if decoded.GetResumeCursor() != cursor {
		t.Fatalf("decoded cursor = %q, want %q", decoded.GetResumeCursor(), cursor)
	}
}

func TestRealtimeEventCatalogueIsDedicatedAndExhaustivelyMapped(t *testing.T) {
	canonicalDescriptor := (&evtv1.Event{}).ProtoReflect().Descriptor()
	canonicalOneof := canonicalDescriptor.Oneofs().ByName("event")
	publicDescriptor := (&realtimev1.RealtimeEvent{}).ProtoReflect().Descriptor()
	publicOneof := publicDescriptor.Oneofs().ByName("event")
	if canonicalOneof == nil || publicOneof == nil {
		t.Fatal("Event.event descriptor is missing")
	}

	publicOnly := map[protoreflect.FullName]bool{
		"chatto.realtime.v1.MessagePostedEvent.body_plaintext":                  true,
		"chatto.realtime.v1.UserAccountCreatedEvent.login_plaintext":            true,
		"chatto.realtime.v1.UserAccountCreatedEvent.display_name_plaintext":     true,
		"chatto.realtime.v1.UserLoginChangedEvent.login_plaintext":              true,
		"chatto.realtime.v1.UserDisplayNameChangedEvent.display_name_plaintext": true,
		"chatto.realtime.v1.UserBioChangedEvent.bio_plaintext":                  true,
	}
	eventsFile := (&realtimev1.RoomCreatedEvent{}).ProtoReflect().Descriptor().ParentFile()
	for index := 0; index < eventsFile.Imports().Len(); index++ {
		if imported := eventsFile.Imports().Get(index); strings.HasPrefix(imported.Path(), "chatto/core/") {
			t.Errorf("public event catalogue imports internal schema %s", imported.Path())
		}
	}
	reachableMessages := map[protoreflect.FullName]bool{}
	reachableEnums := map[protoreflect.FullName]bool{}
	for index := 0; index < publicOneof.Fields().Len(); index++ {
		publicField := publicOneof.Fields().Get(index)
		canonicalField := canonicalOneof.Fields().ByNumber(publicField.Number())
		if canonicalField == nil || canonicalField.Name() != publicField.Name() {
			t.Errorf("public field %s has no canonical variant at field %d", publicField.FullName(), publicField.Number())
			continue
		}
		if got := publicField.Message().ParentFile().Path(); got != "chatto/realtime/v1/events.proto" {
			t.Errorf("public payload %s is owned by %s, want realtime events.proto", publicField.Message().FullName(), got)
		}
		collectPublicDescriptors(publicField.Message(), eventsFile.Path(), reachableMessages, reachableEnums)
		assertPublicMessageCompatible(t, canonicalField.Message(), publicField.Message(), publicOnly, map[string]bool{})

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
		projected, err := projectRealtimeEvent(&event, nil)
		if err != nil {
			t.Errorf("project %s: %v", publicField.FullName(), err)
			continue
		}
		projectedField := projected.ProtoReflect().WhichOneof(publicOneof)
		if projectedField == nil || projectedField.Number() != publicField.Number() {
			t.Errorf("projected field = %v, want %s", projectedField, publicField.FullName())
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
	for index := 0; index < eventsFile.Messages().Len(); index++ {
		message := eventsFile.Messages().Get(index)
		if !reachableMessages[message.FullName()] {
			t.Errorf("public catalogue message %s is not reachable from RealtimeEvent", message.FullName())
		}
	}
	for index := 0; index < eventsFile.Enums().Len(); index++ {
		enum := eventsFile.Enums().Get(index)
		if !reachableEnums[enum.FullName()] {
			t.Errorf("public catalogue enum %s is not reachable from RealtimeEvent", enum.FullName())
		}
	}
}

func collectPublicDescriptors(message protoreflect.MessageDescriptor, filePath string, messages, enums map[protoreflect.FullName]bool) {
	if message == nil || message.ParentFile().Path() != filePath || messages[message.FullName()] {
		return
	}
	messages[message.FullName()] = true
	for index := 0; index < message.Fields().Len(); index++ {
		field := message.Fields().Get(index)
		if field.Message() != nil {
			collectPublicDescriptors(field.Message(), filePath, messages, enums)
		}
		if field.Enum() != nil && field.Enum().ParentFile().Path() == filePath {
			enums[field.Enum().FullName()] = true
		}
	}
}

func assertPublicMessageCompatible(t *testing.T, canonical, public protoreflect.MessageDescriptor, publicOnly map[protoreflect.FullName]bool, seen map[string]bool) {
	t.Helper()
	key := string(canonical.FullName()) + "->" + string(public.FullName())
	if seen[key] {
		return
	}
	seen[key] = true
	for index := 0; index < public.Fields().Len(); index++ {
		publicField := public.Fields().Get(index)
		if publicOnly[publicField.FullName()] {
			if canonical.Fields().ByNumber(publicField.Number()) != nil || !isReservedField(canonical, publicField) {
				t.Errorf("public-only field %s must use a reserved canonical name and number", publicField.FullName())
			}
			continue
		}
		canonicalField := canonical.Fields().ByNumber(publicField.Number())
		if canonicalField == nil {
			t.Errorf("public field %s has no canonical field at number %d", publicField.FullName(), publicField.Number())
			continue
		}
		if canonicalField.Name() != publicField.Name() || canonicalField.Kind() != publicField.Kind() ||
			canonicalField.Cardinality() != publicField.Cardinality() || canonicalField.IsMap() != publicField.IsMap() ||
			canonicalField.HasPresence() != publicField.HasPresence() ||
			oneofName(canonicalField) != oneofName(publicField) {
			t.Errorf("public field %s is not wire-compatible with %s", publicField.FullName(), canonicalField.FullName())
			continue
		}
		if canonicalField.Enum() != nil && publicField.Enum() != nil {
			assertPublicEnumCompatible(t, canonicalField.Enum(), publicField.Enum())
		}
		if canonicalField.Message() != nil && publicField.Message() != nil && canonicalField.Message().FullName() != "google.protobuf.Timestamp" {
			assertPublicMessageCompatible(t, canonicalField.Message(), publicField.Message(), publicOnly, seen)
		}
	}
}

func isReservedField(canonical protoreflect.MessageDescriptor, publicField protoreflect.FieldDescriptor) bool {
	numberReserved := false
	for index := 0; index < canonical.ReservedRanges().Len(); index++ {
		fieldRange := canonical.ReservedRanges().Get(index)
		if publicField.Number() >= fieldRange[0] && publicField.Number() < fieldRange[1] {
			numberReserved = true
			break
		}
	}
	nameReserved := false
	for index := 0; index < canonical.ReservedNames().Len(); index++ {
		if canonical.ReservedNames().Get(index) == publicField.Name() {
			nameReserved = true
			break
		}
	}
	return numberReserved && nameReserved
}

func oneofName(field protoreflect.FieldDescriptor) protoreflect.Name {
	if field.ContainingOneof() == nil {
		return ""
	}
	return field.ContainingOneof().Name()
}

func assertPublicEnumCompatible(t *testing.T, canonical, public protoreflect.EnumDescriptor) {
	t.Helper()
	for index := 0; index < public.Values().Len(); index++ {
		value := public.Values().Get(index)
		if canonical.Values().ByNumber(value.Number()) == nil {
			t.Errorf("public enum value %s has no canonical value at number %d", value.FullName(), value.Number())
		}
	}
}

func containsBytes(value, target []byte) bool {
	return bytes.Contains(value, target)
}
