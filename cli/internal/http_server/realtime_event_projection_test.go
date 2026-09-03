package http_server

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	livev1 "hmans.de/chatto/internal/pb/chatto/core/live/v1"
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

	projected := projectRealtimeEvent(source)
	applyRealtimePlaintext(projected, &core.EventPlaintext{Login: &plaintext})
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

	withoutPlaintext := projectRealtimeEvent(source)
	plaintextField := withoutPlaintext.GetUserLoginChanged().ProtoReflect().Descriptor().Fields().ByName("login_plaintext")
	if withoutPlaintext.GetUserLoginChanged().ProtoReflect().Has(plaintextField) {
		t.Fatal("public login retained an untrusted source value in login_plaintext")
	}
}

func TestProjectRealtimeEventCarriesMessagePlaintext(t *testing.T) {
	bodyPlaintext := "public message"
	projected := projectRealtimeEvent(&evtv1.Event{
		Id: "message-id",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
			RoomId: "room-id",
		}},
	})
	applyRealtimePlaintext(projected, &core.EventPlaintext{MessageBody: &bodyPlaintext})
	if projected.GetMessagePosted().GetBodyPlaintext() != bodyPlaintext {
		t.Fatalf("body_plaintext = %q, want %q", projected.GetMessagePosted().GetBodyPlaintext(), bodyPlaintext)
	}
}

func TestProjectRealtimeEventPreservesMixedRoomGroupOrder(t *testing.T) {
	projected := projectRealtimeEvent(&evtv1.Event{
		Id: "reorder-event-id",
		Event: &evtv1.Event_SidebarGroupEntriesReordered{
			SidebarGroupEntriesReordered: &evtv1.SidebarGroupEntriesReorderedEvent{
				GroupId: "group-id",
				Entries: []*evtv1.SidebarGroupEntry{
					{Kind: evtv1.SidebarGroupEntry_ROOM, Id: "room-id"},
					{Kind: evtv1.SidebarGroupEntry_SIDEBAR_LINK, Id: "link-id"},
				},
			},
		},
	})

	entries := projected.GetSidebarGroupEntriesReordered().GetEntries()
	if len(entries) != 2 {
		t.Fatalf("entries length = %d, want 2", len(entries))
	}
	if entries[0].GetKind() != realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_ROOM || entries[0].GetId() != "room-id" {
		t.Fatalf("first entry = %+v, want room-id with room kind", entries[0])
	}
	if entries[1].GetKind() != realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_SIDEBAR_LINK || entries[1].GetId() != "link-id" {
		t.Fatalf("second entry = %+v, want link-id with sidebar-link kind", entries[1])
	}
}

func TestProjectRealtimeEventDoesNotExposeLegacyAvatarStoragePointer(t *testing.T) {
	projected := projectRealtimeEvent(&evtv1.Event{
		Id: "avatar-event-id",
		Event: &evtv1.Event_UserAvatarSet{UserAvatarSet: &evtv1.UserAvatarSetEvent{
			UserId: "user-id",
			Avatar: &evtv1.DeprecatedAsset{Asset: &evtv1.DeprecatedAsset_S3{S3: &evtv1.S3Asset{
				Key:    "private/key",
				Bucket: proto.String("private-bucket"),
			}}},
		}},
	})
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

	projected := projectRealtimeEvent(event)
	if projected != nil {
		t.Fatalf("projectRealtimeEvent() = %+v, want internal event omission", projected)
	}
}

func TestRealtimeEventUnknownPayloadKeepsMetadataAndCursor(t *testing.T) {
	publicEvent := &realtimev1.RealtimeEvent{Id: "event-id", ActorId: proto.String("actor-id")}
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
	liveDescriptor := (&livev1.LiveEvent{}).ProtoReflect().Descriptor()
	liveOneof := liveDescriptor.Oneofs().ByName("event")
	publicDescriptor := (&realtimev1.RealtimeEvent{}).ProtoReflect().Descriptor()
	publicOneof := publicDescriptor.Oneofs().ByName("event")
	if canonicalOneof == nil || liveOneof == nil || publicOneof == nil {
		t.Fatal("event catalogue descriptor is missing")
	}
	liveSourceNames := map[string]string{
		"user_created_sync":                    "user_created",
		"user_profile_sync":                    "user_profile_updated",
		"server_user_preferences_sync":         "server_user_preferences_updated",
		"thread_follow_changed_sync":           "thread_follow_changed",
		"server_member_deleted_sync":           "server_member_deleted",
		"server_updated_sync":                  "server_updated",
		"user_typing_signal":                   "user_typing",
		"presence_changed_signal":              "presence_changed",
		"call_participant_joined_signal":       "call_participant_joined",
		"call_participant_left_signal":         "call_participant_left",
		"notification_occurrences_invalidated": "notification_occurrences_invalidated",
		"notification_unread_changed":          "notification_unread_changed",
		"room_marked_as_read_sync":             "room_marked_as_read",
		"mention_status_cleared_sync":          "mention_status_cleared",
		"session_terminated_signal":            "session_terminated",
	}
	mappedLiveFields := map[protoreflect.Name]bool{}
	for index := 0; index < publicOneof.Fields().Len(); index++ {
		publicField := publicOneof.Fields().Get(index)
		canonicalField := canonicalOneof.Fields().ByNumber(publicField.Number())
		if got := publicField.Message().ParentFile().Package(); got != "chatto.realtime.v1" {
			t.Errorf("public payload %s is owned by package %s", publicField.Message().FullName(), got)
		}

		var projected *realtimev1.RealtimeEvent
		if canonicalField != nil && canonicalField.Name() == publicField.Name() {
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
			projected = projectRealtimeEvent(&event)
		} else {
			liveName, ok := liveSourceNames[string(publicField.Name())]
			if !ok {
				t.Errorf("public field %s has no Event or LiveEvent source", publicField.FullName())
				continue
			}
			liveField := liveOneof.Fields().ByName(protoreflect.Name(liveName))
			if liveField == nil {
				t.Errorf("public field %s refers to missing LiveEvent field %s", publicField.FullName(), liveName)
				continue
			}
			mappedLiveFields[liveField.Name()] = true
			dynamicEvent := dynamicpb.NewMessage(liveDescriptor)
			dynamicEvent.Set(liveField, dynamicEvent.NewField(liveField))
			wire, err := proto.Marshal(dynamicEvent)
			if err != nil {
				t.Fatalf("marshal %s: %v", liveField.FullName(), err)
			}
			var event livev1.LiveEvent
			if err := proto.Unmarshal(wire, &event); err != nil {
				t.Fatalf("unmarshal %s: %v", liveField.FullName(), err)
			}
			projected = projectRealtimeLiveEvent(&event)
		}
		if projected == nil {
			t.Errorf("projected field = nil, want %s", publicField.FullName())
			continue
		}
		projectedField := projected.ProtoReflect().WhichOneof(publicOneof)
		if projectedField == nil || projectedField.Number() != publicField.Number() {
			t.Errorf("projected field = %v, want %s", projectedField, publicField.FullName())
		}
	}
	for index := 0; index < liveOneof.Fields().Len(); index++ {
		liveField := liveOneof.Fields().Get(index)
		if !mappedLiveFields[liveField.Name()] {
			t.Errorf("LiveEvent field %s has no public realtime mapping", liveField.FullName())
		}
	}
	if publicOneof.Fields().Len() < 40 {
		t.Fatalf("public event catalogue contains %d variants, want at least 40", publicOneof.Fields().Len())
	}
	canonicalRoomID := canonicalOneof.Fields().ByName("room_created").Message().Fields().ByName("room_id").Number()
	publicRoomID := publicOneof.Fields().ByName("room_created").Message().Fields().ByName("room_id").Number()
	if canonicalRoomID == publicRoomID {
		t.Fatal("room_created payload tags still depend on the EVT wire layout")
	}
}

func containsBytes(value, target []byte) bool {
	return bytes.Contains(value, target)
}
