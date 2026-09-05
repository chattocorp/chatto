package http_server

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestProjectRealtimeEventUsesDedicatedPublicShape(t *testing.T) {
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
	if projected == nil {
		t.Fatal("projectRealtimeEvent() = nil, want a public event")
	}
	if projected.GetId() != source.GetId() || projected.GetActorId() != source.GetActorId() {
		t.Fatalf("projected metadata = %+v, want source metadata", projected)
	}
	profile := projected.GetUserProfileChanged()
	if profile.GetUserId() != "user-id" {
		t.Fatalf("projected profile hint = %+v, want user ID", profile)
	}
	if fields := profile.ProtoReflect().Descriptor().Fields(); fields.Len() != 1 {
		t.Fatalf("public profile hint has %d fields, want only user_id", fields.Len())
	}
	if len(profile.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("projected profile hint retained unknown fields: %x", profile.ProtoReflect().GetUnknown())
	}
	if wire, err := proto.Marshal(projected); err != nil {
		t.Fatalf("marshal projected event: %v", err)
	} else if containsBytes(wire, []byte("ciphertext")) || containsBytes(wire, []byte("untrusted-source-plaintext")) {
		t.Fatalf("public profile hint contains stored or untrusted values: %x", wire)
	}
}

func TestPublicRealtimeMessageCarriesPlaintextAndRoomKindWithoutChangingEVT(t *testing.T) {
	env := setupWebSocketTestServer(t)
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "plaintext-viewer", "Plaintext Viewer", "password123")
	if err != nil {
		t.Fatal(err)
	}
	room, err := env.core.CreateRoom(env.ctx, user.GetId(), core.KindChannel, "", "plaintext-room", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.core.JoinRoom(env.ctx, user.GetId(), core.KindChannel, user.GetId(), room.GetId()); err != nil {
		t.Fatal(err)
	}
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, room.GetId(), user.GetId(), "public message", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	before := proto.Clone(event)
	projected, err := env.httpServer.publicRealtimeEvent(env.ctx, user.GetId(), core.NewEVTEventEnvelopeWithDeliverySeq(event, 42))
	if err != nil {
		t.Fatal(err)
	}
	posted := projected.GetMessagePosted()
	if posted.GetBodyPlaintext() != "public message" || posted.GetRoomKind() != apiv1.RoomKind_ROOM_KIND_CHANNEL {
		t.Fatalf("public post = %v, want plaintext and channel kind", posted)
	}
	if !proto.Equal(before, event) {
		t.Fatal("public projection mutated the source EVT event")
	}
}

func TestProjectRealtimeEventCollapsesMixedRoomGroupOrder(t *testing.T) {
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

	if projected.GetRoomLayoutChanged() == nil {
		t.Fatalf("expected layout hint, got %v", projected)
	}
	wire, err := proto.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"group-id", "room-id", "link-id"} {
		if bytes.Contains(wire, []byte(private)) {
			t.Fatalf("layout hint exposed %s", private)
		}
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
	if projected.GetUserProfileChanged().GetUserId() != "user-id" {
		t.Fatalf("user_profile_changed = %+v, want user ID", projected.GetUserProfileChanged())
	}
	if fields := projected.GetUserProfileChanged().ProtoReflect().Descriptor().Fields(); fields.Len() != 1 {
		t.Fatalf("public profile payload has %d fields, want only user_id", fields.Len())
	}
	if wire, err := proto.Marshal(projected); err != nil {
		t.Fatalf("marshal projected event: %v", err)
	} else if containsBytes(wire, []byte("private/key")) || containsBytes(wire, []byte("private-bucket")) {
		t.Fatalf("public avatar event contains storage coordinates: %x", wire)
	}
}

func TestProjectRealtimeEventCollapsesDurableProfileFacts(t *testing.T) {
	tests := []struct {
		name  string
		event *evtv1.Event
	}{
		{name: "login", event: &evtv1.Event{Event: &evtv1.Event_UserLoginChanged{UserLoginChanged: &evtv1.UserLoginChangedEvent{UserId: "U1"}}}},
		{name: "display name", event: &evtv1.Event{Event: &evtv1.Event_UserDisplayNameChanged{UserDisplayNameChanged: &evtv1.UserDisplayNameChangedEvent{UserId: "U1"}}}},
		{name: "legacy avatar", event: &evtv1.Event{Event: &evtv1.Event_UserAvatarSet{UserAvatarSet: &evtv1.UserAvatarSetEvent{UserId: "U1"}}}},
		{name: "avatar cleared", event: &evtv1.Event{Event: &evtv1.Event_UserAvatarCleared{UserAvatarCleared: &evtv1.UserAvatarClearedEvent{UserId: "U1"}}}},
		{name: "current avatar", event: &evtv1.Event{Event: &evtv1.Event_AssetCreated{AssetCreated: &evtv1.AssetCreatedEvent{UserId: "U1", Asset: &evtv1.AssetRecord{Id: "A1"}}}}},
		{name: "custom status", event: &evtv1.Event{Event: &evtv1.Event_UserCustomStatusSet{UserCustomStatusSet: &evtv1.UserCustomStatusSetEvent{UserId: "U1"}}}},
		{name: "custom status cleared", event: &evtv1.Event{Event: &evtv1.Event_UserCustomStatusCleared{UserCustomStatusCleared: &evtv1.UserCustomStatusClearedEvent{UserId: "U1"}}}},
		{name: "bio", event: &evtv1.Event{Event: &evtv1.Event_UserBioChanged{UserBioChanged: &evtv1.UserBioChangedEvent{UserId: "U1"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projected := projectRealtimeEvent(test.event)
			if got := projected.GetUserProfileChanged().GetUserId(); got != "U1" {
				t.Fatalf("user_profile_changed.user_id = %q, want U1", got)
			}
		})
	}
}

func TestProjectRealtimeEventOmitsModerationAuditFacts(t *testing.T) {
	for name, event := range map[string]*evtv1.Event{
		"ban":    {Event: &evtv1.Event_RoomMemberBanned{RoomMemberBanned: &evtv1.RoomMemberBannedEvent{RoomId: "R1", UserId: "U1", Reason: "private"}}},
		"add":    {Event: &evtv1.Event_RoomMemberAdded{RoomMemberAdded: &evtv1.RoomMemberAddedEvent{RoomId: "R1", UserId: "U1"}}},
		"remove": {Event: &evtv1.Event_RoomMemberRemoved{RoomMemberRemoved: &evtv1.RoomMemberRemovedEvent{RoomId: "R1", UserId: "U1"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if projected := projectRealtimeEvent(event); projected != nil {
				t.Fatalf("projectRealtimeEvent() = %+v, want private audit omission", projected)
			}
		})
	}
}

func TestPublicRealtimeEventProjectsViewerSpecificSemantics(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "semantic-owner", "Semantic Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "semantic-other", "Semantic Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}

	privateFormat := &evtv1.Event{Id: "E1", Event: &evtv1.Event_UserTimeFormatChanged{
		UserTimeFormatChanged: &evtv1.UserTimeFormatChangedEvent{UserId: owner.GetId(), TimeFormat: evtv1.TimeFormat_TIME_FORMAT_24H},
	}}
	if projected, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, owner.GetId(), privateFormat); err != nil || projected.GetViewerPreferencesChanged() == nil {
		t.Fatalf("owner preference projection = %+v, %v; want viewer hint", projected, err)
	}
	if projected, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, other.GetId(), privateFormat); err != nil || projected != nil {
		t.Fatalf("other preference projection = %+v, %v; want omission", projected, err)
	}

	sharing := &evtv1.Event{Id: "E2", Event: &evtv1.Event_UserTimezoneSharingChanged{
		UserTimezoneSharingChanged: &evtv1.UserTimezoneSharingChangedEvent{UserId: owner.GetId()},
	}}
	if projected, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, other.GetId(), sharing); err != nil || projected.GetUserProfileChanged().GetUserId() != owner.GetId() {
		t.Fatalf("public sharing projection = %+v, %v; want profile hint", projected, err)
	}

	thread := &evtv1.Event{Id: "E3", Event: &evtv1.Event_ThreadFollowed{
		ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "M1", UserId: owner.GetId()},
	}}
	if projected, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, other.GetId(), thread); err != nil || projected != nil {
		t.Fatalf("other thread projection = %+v, %v; want omission", projected, err)
	}
}

func TestPublicRealtimeEventTranslatesEffectiveUnbanToOrdinaryJoin(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "unban-owner", "Unban Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "unban-target", "Unban Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, owner.GetId(), core.KindChannel, "", "unban-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.SetRoomUniversal(env.ctx, owner.GetId(), core.KindChannel, room.GetId(), true); err != nil {
		t.Fatalf("SetRoomUniversal: %v", err)
	}
	event := &evtv1.Event{Id: "E-UNBAN", ActorId: owner.GetId(), Event: &evtv1.Event_RoomMemberUnbanned{
		RoomMemberUnbanned: &evtv1.RoomMemberUnbannedEvent{RoomId: room.GetId(), UserId: target.GetId(), Reason: "private"},
	}}
	projected, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, owner.GetId(), event)
	if err != nil {
		t.Fatalf("projectViewerRealtimeEvent: %v", err)
	}
	if projected.GetUserJoinedRoom().GetRoomId() != room.GetId() || projected.GetActorId() != target.GetId() {
		t.Fatalf("projected unban = %+v, want ordinary target-authored join", projected)
	}
}

func TestPublicRealtimeSchemaOmitsInternalFields(t *testing.T) {
	accountFields := (&realtimev1.UserAccountCreatedEvent{}).ProtoReflect().Descriptor().Fields()
	if accountFields.ByName("bot_owner_user_id") != nil {
		t.Fatal("public user account event exposes bot_owner_user_id")
	}
	for _, message := range []proto.Message{
		&realtimev1.VoiceCallParticipantJoinedEvent{},
		&realtimev1.VoiceCallParticipantLeftEvent{},
		&realtimev1.VoiceCallStartedEvent{},
		&realtimev1.VoiceCallEndedEvent{},
	} {
		if message.ProtoReflect().Descriptor().Fields().ByName("source") != nil {
			t.Fatalf("public call event %s exposes internal source", message.ProtoReflect().Descriptor().FullName())
		}
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

func TestProjectRealtimePubSubEventReturnsIsolatedPublicCopy(t *testing.T) {
	payload := &realtimev1.UserTypingEvent{RoomId: "room-id"}
	source := &pubsubv1.PubSubEvent{
		Id: "typing-id",
		Event: &pubsubv1.PubSubEvent_UserTyping{
			UserTyping: payload,
		},
	}

	projected := projectRealtimePubSubEvent(source)
	if projected == nil || projected.GetUserTyping() == nil {
		t.Fatalf("projectRealtimePubSubEvent() = %+v, want typing event", projected)
	}
	if projected.GetUserTyping() == payload {
		t.Fatal("projected payload aliases the private pubsub message")
	}

	projected.GetUserTyping().RoomId = "viewer-filtered-room"
	if got := source.GetUserTyping().GetRoomId(); got != "room-id" {
		t.Fatalf("source room_id = %q after public mutation, want room-id", got)
	}
}

func TestRealtimeEventUnknownPayloadKeepsMetadataAndCursor(t *testing.T) {
	publicEvent := &realtimev1.RealtimeEvent{Id: "event-id", ActorId: proto.String("actor-id")}
	unknown := protowire.AppendTag(nil, 25000, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, nil)
	publicEvent.ProtoReflect().SetUnknown(unknown)
	cursor := "opaque-cursor"
	publicEvent.Cursor = &cursor
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
	if decoded.GetCursor() != cursor {
		t.Fatalf("decoded cursor = %q, want %q", decoded.GetCursor(), cursor)
	}
}

func TestRealtimeEventCatalogueIsDedicatedAndExhaustivelyMapped(t *testing.T) {
	canonicalDescriptor := (&evtv1.Event{}).ProtoReflect().Descriptor()
	canonicalOneof := canonicalDescriptor.Oneofs().ByName("event")
	pubsubDescriptor := (&pubsubv1.PubSubEvent{}).ProtoReflect().Descriptor()
	pubsubOneof := pubsubDescriptor.Oneofs().ByName("event")
	publicDescriptor := (&realtimev1.RealtimeEvent{}).ProtoReflect().Descriptor()
	publicOneof := publicDescriptor.Oneofs().ByName("event")
	if canonicalOneof == nil || pubsubOneof == nil || publicOneof == nil {
		t.Fatal("event catalogue descriptor is missing")
	}
	pubsubSourceNames := map[string]string{
		"thread_viewer_state_changed":       "thread_viewer_state_changed",
		"user_typing":                       "user_typing",
		"presence_changed":                  "presence_changed",
		"notification_occurrences_changed":  "notification_occurrences_changed",
		"notification_unread_state_changed": "notification_unread_state_changed",
		"room_read_state_changed":           "room_read_state_changed",
	}
	evtSourceNames := map[string]string{
		"user_profile_changed":       "user_login_changed",
		"viewer_preferences_changed": "user_time_format_changed",
		"server_profile_changed":     "server_name_changed",
		"room_layout_changed":        "room_group_created",
	}
	mappedPubSubFields := map[protoreflect.Name]bool{}
	for index := 0; index < publicOneof.Fields().Len(); index++ {
		publicField := publicOneof.Fields().Get(index)
		canonicalName := string(publicField.Name())
		if sourceName := evtSourceNames[canonicalName]; sourceName != "" {
			canonicalName = sourceName
		}
		canonicalField := canonicalOneof.Fields().ByName(protoreflect.Name(canonicalName))
		if got := publicField.Message().ParentFile().Package(); got != "chatto.realtime.v1" {
			t.Errorf("public payload %s is owned by package %s", publicField.Message().FullName(), got)
		}

		var projected *realtimev1.RealtimeEvent
		if canonicalField != nil {
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
			if publicField.Name() == "viewer_preferences_changed" {
				projected = &realtimev1.RealtimeEvent{Event: &realtimev1.RealtimeEvent_ViewerPreferencesChanged{ViewerPreferencesChanged: &realtimev1.ViewerPreferencesChangedEvent{}}}
			} else {
				projected = projectRealtimeEvent(&event)
			}
		} else {
			pubsubName, ok := pubsubSourceNames[string(publicField.Name())]
			if !ok {
				t.Errorf("public field %s has no Event or PubSubEvent source", publicField.FullName())
				continue
			}
			pubsubField := pubsubOneof.Fields().ByName(protoreflect.Name(pubsubName))
			if pubsubField == nil {
				t.Errorf("public field %s refers to missing PubSubEvent field %s", publicField.FullName(), pubsubName)
				continue
			}
			if got, want := pubsubField.Message().FullName(), publicField.Message().FullName(); got != want {
				t.Errorf("PubSubEvent field %s uses payload %s, want public payload %s", pubsubField.FullName(), got, want)
			}
			mappedPubSubFields[pubsubField.Name()] = true
			dynamicEvent := dynamicpb.NewMessage(pubsubDescriptor)
			dynamicEvent.Set(pubsubField, dynamicEvent.NewField(pubsubField))
			wire, err := proto.Marshal(dynamicEvent)
			if err != nil {
				t.Fatalf("marshal %s: %v", pubsubField.FullName(), err)
			}
			var event pubsubv1.PubSubEvent
			if err := proto.Unmarshal(wire, &event); err != nil {
				t.Fatalf("unmarshal %s: %v", pubsubField.FullName(), err)
			}
			projected = projectRealtimePubSubEvent(&event)
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
	for index := 0; index < pubsubOneof.Fields().Len(); index++ {
		pubsubField := pubsubOneof.Fields().Get(index)
		if pubsubField.Name() == "session_terminated" {
			continue
		}
		if !mappedPubSubFields[pubsubField.Name()] {
			t.Errorf("PubSubEvent field %s has no public realtime mapping", pubsubField.FullName())
		}
	}
	for index := 0; index < publicOneof.Fields().Len(); index++ {
		if field := publicOneof.Fields().Get(index); field.Number() >= 35 && field.Number() <= 45 {
			t.Errorf("public field %s reuses a retired layout tag", field.FullName())
		}
	}
}

func TestRealtimeSessionTerminationUsesCloseFrame(t *testing.T) {
	frame, err := (&HTTPServer{}).realtimeServerFrameForEvent(t.Context(), "viewer", core.NewPubSubEventEnvelope(
		&pubsubv1.PubSubEvent{
			Id: "session-event",
			Event: &pubsubv1.PubSubEvent_SessionTerminated{
				SessionTerminated: &pubsubv1.SessionTerminatedEvent{Reason: "admin_boot"},
			},
		},
	))
	if err != nil {
		t.Fatalf("project session termination: %v", err)
	}
	close := frame.GetClose()
	if close == nil {
		t.Fatalf("frame = %T, want close", frame.GetFrame())
	}
	if close.GetCode() != realtimev1.RealtimeCloseCode_REALTIME_CLOSE_CODE_SESSION_TERMINATED {
		t.Fatalf("close code = %v, want session terminated", close.GetCode())
	}
	if close.GetReconnect() {
		t.Fatal("session termination requested reconnect")
	}
}

func containsBytes(value, target []byte) bool {
	return bytes.Contains(value, target)
}
