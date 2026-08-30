package core

import (
	"bytes"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/encryption"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type snapshotProjection interface {
	SnapshotContractID() string
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

func TestCurrentProjectionSnapshotCodecsContainOnlyCurrentState(t *testing.T) {
	assets := NewAssetProjection()
	assets.messageOwners["A1"] = assetMessageRef{roomID: "R1", messageEventID: "M1", authorID: "U1"}
	assetPayload, err := assets.Snapshot()
	require.NoError(t, err)
	assetSnapshot := &projectionv1.AssetProjectionSnapshot{}
	require.NoError(t, proto.Unmarshal(assetPayload, assetSnapshot))
	require.Len(t, assetSnapshot.GetMessageOwners(), 1)
	require.Equal(t, "A1", assetSnapshot.GetMessageOwners()[0].GetAssetId())
	ownerFields := (&projectionv1.AssetMessageOwnerSnapshot{}).ProtoReflect().Descriptor().Fields()
	require.Equal(t, "author_id", string(ownerFields.ByNumber(protoreflect.FieldNumber(4)).Name()))

	timeline := NewRoomTimelineProjection()
	timeline.replayGuard.highestSeq = 41
	timeline.replayGuard.completeReplay()
	timeline.pinnedMessagesByRoom["R1"] = map[string]PinnedMessageState{
		"M1": {PinEventID: "P1", PinSequence: 40, RoomID: "R1", MessageEventID: "M1"},
	}
	timeline.latestPinByRoom["R1"] = latestRoomPinState{PinEventID: "P1", PinSequence: 40}
	timelinePayload, err := timeline.Snapshot()
	require.NoError(t, err)
	timelineSnapshot := &projectionv1.RoomTimelineProjectionSnapshot{}
	require.NoError(t, proto.Unmarshal(timelinePayload, timelineSnapshot))
	require.Equal(t, uint64(41), timelineSnapshot.GetReplayGuard().GetHighestSequence())
	require.Len(t, timelineSnapshot.GetPinnedMessages(), 1)
	require.Equal(t, "M1", timelineSnapshot.GetPinnedMessages()[0].GetMessageEventId())
	require.Len(t, timelineSnapshot.GetLatestRoomPins(), 1)
	require.Equal(t, "P1", timelineSnapshot.GetLatestRoomPins()[0].GetPinEventId())
	timelineFields := timelineSnapshot.ProtoReflect().Descriptor().Fields()
	require.Equal(t, "replay_guard", string(timelineFields.ByNumber(protoreflect.FieldNumber(8)).Name()))
	require.Equal(t, "pinned_messages", string(timelineFields.ByNumber(protoreflect.FieldNumber(9)).Name()))
	require.Equal(t, "latest_room_pins", string(timelineFields.ByNumber(protoreflect.FieldNumber(10)).Name()))
}

func TestProjectionSnapshotContractsIncludeCurrentSchema(t *testing.T) {
	tests := []struct {
		contract  string
		semantics string
		message   proto.Message
	}{
		{assetSnapshotContractID, "v3", &projectionv1.AssetProjectionSnapshot{}},
		{callStateSnapshotContractID, "v1", &projectionv1.CallStateProjectionSnapshot{}},
		{configSnapshotContractID, "v2", &projectionv1.ConfigProjectionSnapshot{}},
		{contentKeySnapshotContractID, "v1", &projectionv1.ContentKeyProjectionSnapshot{}},
		{mentionablesSnapshotContractID, "v2", &projectionv1.MentionablesProjectionSnapshot{}},
		{notificationDecisionSnapshotContractID, "v2", &projectionv1.NotificationDecisionProjectionSnapshot{}},
		{notificationSnapshotContractID, "v2", &projectionv1.NotificationProjectionSnapshot{}},
		{rbacSnapshotContractID, "v1", &projectionv1.RBACProjectionSnapshot{}},
		{reactionSnapshotContractID, "v1", &projectionv1.ReactionProjectionSnapshot{}},
		{roomDirectorySnapshotContractID, "v1", &projectionv1.RoomDirectoryProjectionSnapshot{}},
		{roomGroupLayoutSnapshotContractID, "v1", &projectionv1.RoomGroupLayoutProjectionSnapshot{}},
		{roomTimelineSnapshotContractID, "v7", &projectionv1.RoomTimelineProjectionSnapshot{}},
		{threadSnapshotContractID, "v2", &projectionv1.ThreadProjectionSnapshot{}},
		{userSnapshotContractID, "v4", &projectionv1.UserProfileProjectionSnapshot{}},
	}
	for _, tt := range tests {
		require.Equal(t, snapshotContractID(tt.semantics, tt.message), tt.contract)
		require.LessOrEqual(t, len(tt.contract), 64)
	}
}

func TestPrivacyBoundaryProjectionContractsRejectPreRequestSnapshots(t *testing.T) {
	tests := []struct {
		current string
		old     string
	}{
		{userSnapshotContractID, snapshotContractID("v2", &projectionv1.UserProfileProjectionSnapshot{})},
		{userSnapshotContractID, snapshotContractID("v3", &projectionv1.UserProfileProjectionSnapshot{})},
		{mentionablesSnapshotContractID, snapshotContractID("v1", &projectionv1.MentionablesProjectionSnapshot{})},
		{roomTimelineSnapshotContractID, snapshotContractID("v5", &projectionv1.RoomTimelineProjectionSnapshot{})},
		{roomTimelineSnapshotContractID, snapshotContractID("v6", &projectionv1.RoomTimelineProjectionSnapshot{})},
		{threadSnapshotContractID, snapshotContractID("v1", &projectionv1.ThreadProjectionSnapshot{})},
	}
	for _, tt := range tests {
		require.NotEqual(t, tt.old, tt.current)
	}
}

func TestProjectionSnapshotSchemaFingerprintIncludesReferencedType(t *testing.T) {
	fingerprint := func(thirdFieldType string) string {
		t.Helper()
		optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
		messageType := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
		field := func(name string, number int32, typeName string) *descriptorpb.FieldDescriptorProto {
			return &descriptorpb.FieldDescriptorProto{
				Name:     proto.String(name),
				Number:   proto.Int32(number),
				Label:    &optional,
				Type:     &messageType,
				TypeName: proto.String(typeName),
			}
		}
		file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
			Name:    proto.String("snapshot_fingerprint_test.proto"),
			Package: proto.String("snapshot_fingerprint_test"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("A"), Field: []*descriptorpb.FieldDescriptorProto{{
					Name: proto.String("value"), Number: proto.Int32(1), Label: &optional,
					Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				}}},
				{Name: proto.String("B"), Field: []*descriptorpb.FieldDescriptorProto{{
					Name: proto.String("value"), Number: proto.Int32(1), Label: &optional,
					Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
				}}},
				{
					Name: proto.String("Root"),
					Field: []*descriptorpb.FieldDescriptorProto{
						field("a", 1, ".snapshot_fingerprint_test.A"),
						field("b", 2, ".snapshot_fingerprint_test.B"),
						field("choice", 3, thirdFieldType),
					},
				},
			},
		}, nil)
		require.NoError(t, err)
		return snapshotSchemaFingerprint(file.Messages().ByName("Root"))
	}

	withA := fingerprint(".snapshot_fingerprint_test.A")
	withB := fingerprint(".snapshot_fingerprint_test.B")
	require.NotEqual(t, withA, withB)
}

func TestMentionablesSnapshotRetainsEncryptedSourceWithoutPlaintextHandle(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	newProjection := func() *MentionablesProjection {
		return NewMentionablesProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	}
	p := newProjection()
	dek := &evtv1.Event{Id: "K1", Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}}
	require.NoError(t, p.Apply(dek, 1))
	contentKey := &messageContentKey{epoch: 1, purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
	created := userEvent("E1", time.Unix(1_700_000_000, 0), accountCreated(t, contentKey, "E1", "U1", "SecretLogin", "Secret Name"))
	require.NoError(t, p.Apply(created, 2))

	payload, err := p.Snapshot()
	require.NoError(t, err)
	require.NotContains(t, string(payload), "SecretLogin")
	require.NotContains(t, string(payload), "Secret Name")

	restored := newProjection()
	require.NoError(t, restored.Restore(payload))
	availability := restored.Availability("secretlogin", nil)
	require.False(t, availability.Available)
	require.Equal(t, mentionableOwnerUser, availability.OwnerKind)
	require.Equal(t, "U1", availability.OwnerID)
}

func TestRoomDirectorySnapshotPreservesUnknownThreadingMode(t *testing.T) {
	const unknownMode = evtv1.RoomThreadingMode(99)
	projection := NewRoomDirectoryProjection()
	require.NoError(t, projection.Catalog.Apply(&evtv1.Event{Event: &evtv1.Event_RoomCreated{
		RoomCreated: &evtv1.RoomCreatedEvent{
			RoomId: "R1", Name: "future-threading", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL,
			ThreadingMode: unknownMode,
		},
	}}, 1))

	room, ok := projection.Catalog.Get("R1")
	require.True(t, ok)
	require.Equal(t, evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED, room.GetThreadingMode())

	payload, err := projection.Snapshot()
	require.NoError(t, err)
	assertRawMode := func(snapshotBytes []byte) {
		t.Helper()
		snapshot := &projectionv1.RoomDirectoryProjectionSnapshot{}
		require.NoError(t, proto.Unmarshal(snapshotBytes, snapshot))
		require.Len(t, snapshot.GetRooms(), 1)
		require.Equal(t, unknownMode, snapshot.GetRooms()[0].GetThreadingMode())
	}
	assertRawMode(payload)

	restored := NewRoomDirectoryProjection()
	require.NoError(t, restored.Restore(payload))
	restoredRoom, ok := restored.Catalog.Get("R1")
	require.True(t, ok)
	require.Equal(t, evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED, restoredRoom.GetThreadingMode())
	roundTrip, err := restored.Snapshot()
	require.NoError(t, err)
	assertRawMode(roundTrip)
}

func TestProjectionSnapshotsRoundTripTransactionally(t *testing.T) {
	now := time.Unix(1_700_000_000, 123).UTC()
	tests := []struct {
		name string
		new  func() snapshotProjection
		seed func(snapshotProjection)
	}{
		{"room_directory", func() snapshotProjection { return NewRoomDirectoryProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomDirectoryProjection)
			p.Catalog.rooms["R1"] = &roomCatalogEntry{name: "General", kind: evtv1.RoomKind_ROOM_KIND_CHANNEL, universal: true}
			p.Catalog.seq = 41
			p.Membership.addLocked("R1", "U1")
			expires := now.Add(time.Hour)
			p.Bans.byRoom["R1"] = map[string]RoomBan{"U2": {EventID: "B1", RoomID: "R1", UserID: "U2", ModeratorID: "U1", Reason: "spam", CreatedAt: now, ExpiresAt: &expires}}
		}},
		{"server_config", func() snapshotProjection { return NewConfigProjection() }, func(raw snapshotProjection) {
			p := raw.(*ConfigProjection)
			blocked := "admin"
			timezone := "Europe/Berlin"
			format := evtv1.TimeFormat_TIME_FORMAT_24H
			p.server.serverName = "Chatto"
			p.server.blockedUsernames = &blocked
			p.users["U1"] = &userConfigState{
				timezone: &timezone, timeFormat: &format, shareTimezone: true,
				serverModes: &evtv1.NotificationDeliveryModes{Reactions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION.Enum()},
				roomModesByRoom: map[string]*evtv1.NotificationDeliveryModes{
					"R1": {DirectMentions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION.Enum()},
				},
				roomGroupModesByGroup: map[string]*evtv1.NotificationDeliveryModes{
					"G1": {Reactions: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()},
				},
			}
		}},
		{"room_group_layout", func() snapshotProjection { return NewRoomGroupLayoutProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomGroupLayoutProjection)
			p.Groups.groups["G1"] = &roomGroupEntry{name: "Lobby", roomIDs: []string{"R1"}, entries: []*evtv1.SidebarGroupEntry{{Kind: evtv1.SidebarGroupEntry_ROOM, Id: "R1"}}, links: map[string]*evtv1.SidebarLink{"L1": {Id: "L1", Label: "Docs", Url: "https://example.test"}}}
			p.Groups.seq = 42
			p.Layout.groupIDs = []string{"G1"}
		}},
		{"notification_decisions", func() snapshotProjection { return NewNotificationDecisionProjection() }, func(raw snapshotProjection) {
			p := raw.(*NotificationDecisionProjection)
			event := &evtv1.Event{Id: "R1-created", Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL, Universal: true}}}
			if err := p.Apply(event, 41); err != nil {
				t.Fatal(err)
			}
		}},
		{"notifications", func() snapshotProjection {
			p := NewNotificationProjection()
			p.now = func() time.Time { return now }
			return p
		}, func(raw snapshotProjection) {
			p := raw.(*NotificationProjection)
			expiresAt := now.Add(time.Hour)
			occurrence := &notificationv1.NotificationOccurrence{
				Id:                         "N1",
				RecipientId:                "U1",
				SourceEventId:              "E1",
				SourceCreatedAt:            timestamppb.New(now),
				Signal:                     testNotificationSignal(notificationTestSignalReply, "R1", "E1"),
				ExpiresAt:                  timestamppb.New(expiresAt),
				NotificationStreamSequence: 41,
			}
			p.byID[occurrence.GetId()] = occurrence
			p.idsByUser[occurrence.GetRecipientId()] = map[string]struct{}{occurrence.GetId(): {}}
			p.tombstones["N2"] = notificationProjectionTombstone{recipientID: "U1", expiresAt: expiresAt, signalSequence: 42}
		}},
		{"room_timeline", func() snapshotProjection { return NewRoomTimelineProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomTimelineProjection)
			bodyEvent := &evtv1.Event{Id: "BODY1", CreatedAt: timestamppb.New(now), Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: "R1", EventId: "M1", Body: &evtv1.MessageBody{AuthorId: "U1", BodyEventId: "BODY1", EncryptionVersion: 2, ContentKeyEpoch: 1, EncryptedBody: []byte("ciphertext"), EncryptionNonce: bytes.Repeat([]byte{1}, 24)}}}}
			posted := &evtv1.Event{Id: "M1", ActorId: "U1", CreatedAt: timestamppb.New(now), Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}}}
			if err := p.Apply(bodyEvent, 40); err != nil {
				t.Fatal(err)
			}
			if err := p.Apply(posted, 41); err != nil {
				t.Fatal(err)
			}
			p.CompleteStartupReplay()
		}},
		{"call_state", func() snapshotProjection { return NewCallStateProjection() }, func(raw snapshotProjection) {
			p := raw.(*CallStateProjection)
			p.roomSeq["R1"] = 41
			p.activeCalls["R1"] = CallSession{CallID: "C1", E2EEKeyRef: "K1", StartedAt: now.Unix(), Source: evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER}
			p.rooms["R1"] = map[string]CallParticipant{"U1": {UserID: "U1", CallID: "C1", JoinedAt: now.Unix()}}
		}},
		{"assets", func() snapshotProjection { return NewAssetProjection() }, func(raw snapshotProjection) {
			p := raw.(*AssetProjection)
			p.assetCreations["A1"] = &evtv1.AssetCreatedEvent{Asset: &evtv1.AssetRecord{Id: "A1"}}
			p.assetChildren["A1"] = []string{"A2"}
			p.videoManifests["A1"] = &VideoAttachmentManifest{Started: &evtv1.AssetProcessingStartedEvent{AssetId: "A1"}}
			p.deletedAssets["A3"] = struct{}{}
			p.deletedAssetRoom["A3"] = "R1"
			p.messageOwners["A1"] = assetMessageRef{roomID: "R1", messageEventID: "M1", authorID: "U1"}
			p.messageOwners["A3"] = assetMessageRef{roomID: "R1", messageEventID: "M1", authorID: "U1"}
			p.publicLinkPreviewAssets["A4"] = struct{}{}
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"reactions", func() snapshotProjection { return NewReactionProjection() }, func(raw snapshotProjection) {
			p := raw.(*ReactionProjection)
			p.byMessage["M1"] = map[string]map[string]reactionProjectionEntry{"+1": {"U1": {AddedAtNanos: now.UnixNano(), SourceEventID: "E-reaction"}}}
			p.roomSeq["R1"] = 41
			p.messageRoom["M1"] = "R1"
			p.echoOriginal["M2"] = "M1"
			p.assetRoom["A1"] = "R1"
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"content_keys", func() snapshotProjection { return NewContentKeyProjection() }, func(raw snapshotProjection) {
			p := raw.(*ContentKeyProjection)
			p.applyDEKGeneratedLocked(&evtv1.UserDEKGeneratedEvent{UserId: "U1", Purpose: evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, Epoch: 1, ContentKeyRef: "user.U1.pii.1", WrappingKeyRef: "user.U1"})
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"rbac", func() snapshotProjection { return NewRBACProjection() }, func(raw snapshotProjection) {
			p := raw.(*RBACProjection)
			p.roles["member"] = &evtv1.Role{Name: "member", DisplayName: "Member"}
			p.assignments["U1"] = map[string]struct{}{"member": {}}
			p.decisions[rbacDecisionKey{scope: ScopeServer, subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, subject: "member", permission: PermMessagePost}] = DecisionAllow
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"mentionables", func() snapshotProjection { return newMentionablesProjectionWithDEKResolver(nil) }, func(raw snapshotProjection) {
			p := raw.(*MentionablesProjection)
			p.addOwner("moderator", mentionableOwner{kind: mentionableOwnerRole, id: "moderator"})
		}},
		{"users", func() snapshotProjection { return NewUserProjection(nil, nil) }, func(raw snapshotProjection) {
			p := raw.(*UserProjection)
			p.users["U1"] = &projectedUser{user: &evtv1.User{Id: "U1", CreatedAt: timestamppb.New(now)}, deleted: true, verifiedEmail: make(map[string]projectedVerifiedEmail)}
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
	}

	expectedContractPrefix := map[string]string{
		"room_directory": "v1-", "server_config": "v2-", "room_group_layout": "v1-",
		"notification_decisions": "v2-", "notifications": "v2-",
		"room_timeline": "v7-", "call_state": "v1-", "assets": "v3-", "reactions": "v1-",
		"content_keys": "v1-", "rbac": "v1-", "mentionables": "v2-", "users": "v4-",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.new()
			tt.seed(original)
			payload, err := original.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) == 0 {
				t.Fatal("empty snapshot payload")
			}
			restored := tt.new()
			if err := restored.Restore(payload); err != nil {
				t.Fatalf("Restore: %v", err)
			}
			roundTrip, err := restored.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTrip, payload) {
				t.Fatalf("round-trip snapshot differs\n got %x\nwant %x", roundTrip, payload)
			}
			if err := restored.Restore([]byte{0xff}); err == nil {
				t.Fatal("malformed snapshot restored successfully")
			}
			afterFailure, err := restored.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(afterFailure, payload) {
				t.Fatal("failed restore mutated projection state")
			}
			if err := restored.Restore(nil); err != nil {
				t.Fatalf("cold restore: %v", err)
			}
			empty, err := restored.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := tt.new().Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(empty, fresh) {
				t.Fatal("cold restore did not reset projection")
			}
			id := original.SnapshotContractID()
			if !strings.HasPrefix(id, expectedContractPrefix[tt.name]) {
				t.Fatalf("contract ID = %q, want prefix %q", id, expectedContractPrefix[tt.name])
			}
		})
	}
}
