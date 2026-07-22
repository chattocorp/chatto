package core

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/encryption"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type snapshotProjection interface {
	SnapshotContractID() string
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

func TestMentionablesSnapshotRetainsEncryptedSourceWithoutPlaintextHandle(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	newProjection := func() *MentionablesProjection {
		return NewMentionablesProjection(staticProjectionKeyWrapper{key: key}, staticProjectionDEKStore{})
	}
	p := newProjection()
	dek := &corev1.Event{Id: "K1", Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{UserId: "U1", Epoch: 1, Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, ContentKeyRef: "dek.test"}}}
	require.NoError(t, p.Apply(dek, 1))
	contentKey := &messageContentKey{epoch: 1, purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, key: key}
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

func TestV2ProjectionSnapshotsRoundTripTransactionally(t *testing.T) {
	now := time.Unix(1_700_000_000, 123).UTC()
	tests := []struct {
		name string
		new  func() snapshotProjection
		seed func(snapshotProjection)
	}{
		{"room_directory", func() snapshotProjection { return NewRoomDirectoryProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomDirectoryProjection)
			p.Catalog.rooms["R1"] = &roomCatalogEntry{name: "General", kind: corev1.RoomKind_ROOM_KIND_CHANNEL, universal: true}
			p.Catalog.seq = 41
			p.Membership.addLocked("R1", "U1")
			expires := now.Add(time.Hour)
			p.Bans.byRoom["R1"] = map[string]RoomBan{"U2": {EventID: "B1", RoomID: "R1", UserID: "U2", ModeratorID: "U1", Reason: "spam", CreatedAt: now, ExpiresAt: &expires}}
		}},
		{"server_config", func() snapshotProjection { return NewConfigProjection() }, func(raw snapshotProjection) {
			p := raw.(*ConfigProjection)
			blocked := "admin"
			timezone := "Europe/Berlin"
			format := corev1.TimeFormat_TIME_FORMAT_24H
			level := corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
			p.server.serverName = "Chatto"
			p.server.blockedUsernames = &blocked
			p.users["U1"] = &userConfigState{timezone: &timezone, timeFormat: &format, serverLevel: &level, roomLevelByRoom: map[string]corev1.NotificationLevel{"R1": corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES}}
		}},
		{"room_group_layout", func() snapshotProjection { return NewRoomGroupLayoutProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomGroupLayoutProjection)
			p.Groups.groups["G1"] = &roomGroupEntry{name: "Lobby", roomIDs: []string{"R1"}, entries: []*corev1.SidebarGroupEntry{{Kind: corev1.SidebarGroupEntry_ROOM, Id: "R1"}}, links: map[string]*corev1.SidebarLink{"L1": {Id: "L1", Label: "Docs", Url: "https://example.test"}}}
			p.Groups.seq = 42
			p.Layout.groupIDs = []string{"G1"}
		}},
		{"room_timeline", func() snapshotProjection { return NewRoomTimelineProjection() }, func(raw snapshotProjection) {
			p := raw.(*RoomTimelineProjection)
			bodyEvent := &corev1.Event{Id: "BODY1", CreatedAt: timestamppb.New(now), Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{RoomId: "R1", EventId: "M1", Body: &corev1.MessageBody{AuthorId: "U1", BodyEventId: "BODY1", EncryptionVersion: 2, ContentKeyEpoch: 1, EncryptedBody: []byte("ciphertext"), EncryptionNonce: bytes.Repeat([]byte{1}, 24)}}}}
			posted := &corev1.Event{Id: "M1", ActorId: "U1", CreatedAt: timestamppb.New(now), Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}}}
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
			p.activeCalls["R1"] = CallSession{CallID: "C1", E2EEKeyRef: "K1", StartedAt: now.Unix(), Source: corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER}
			p.rooms["R1"] = map[string]CallParticipant{"U1": {UserID: "U1", CallID: "C1", JoinedAt: now.Unix()}}
		}},
		{"assets", func() snapshotProjection { return NewAssetProjection() }, func(raw snapshotProjection) {
			p := raw.(*AssetProjection)
			p.assetCreations["A1"] = &corev1.AssetCreatedEvent{Asset: &corev1.AssetRecord{Id: "A1"}}
			p.assetChildren["A1"] = []string{"A2"}
			p.videoManifests["A1"] = &VideoAttachmentManifest{Started: &corev1.AssetProcessingStartedEvent{AssetId: "A1"}}
			p.deletedAssets["A3"] = struct{}{}
			p.deletedAssetRoom["A3"] = "R1"
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"reactions", func() snapshotProjection { return NewReactionProjection() }, func(raw snapshotProjection) {
			p := raw.(*ReactionProjection)
			p.byMessage["M1"] = map[string]map[string]int64{"+1": {"U1": now.UnixNano()}}
			p.roomSeq["R1"] = 41
			p.messageRoom["M1"] = "R1"
			p.echoOriginal["M2"] = "M1"
			p.assetRoom["A1"] = "R1"
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"content_keys", func() snapshotProjection { return NewContentKeyProjection() }, func(raw snapshotProjection) {
			p := raw.(*ContentKeyProjection)
			p.applyDEKGeneratedLocked(&corev1.UserDEKGeneratedEvent{UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, Epoch: 1, ContentKeyRef: "user.U1.pii.1", WrappingKeyRef: "user.U1"})
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"rbac", func() snapshotProjection { return NewRBACProjection() }, func(raw snapshotProjection) {
			p := raw.(*RBACProjection)
			p.roles["member"] = &corev1.Role{Name: "member", DisplayName: "Member"}
			p.assignments["U1"] = map[string]struct{}{"member": {}}
			p.decisions[rbacDecisionKey{scope: ScopeServer, subjectKind: corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, subject: "member", permission: PermMessagePost}] = DecisionAllow
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
		{"mentionables", func() snapshotProjection { return newMentionablesProjectionWithDEKResolver(nil) }, func(raw snapshotProjection) {
			p := raw.(*MentionablesProjection)
			p.addOwner("moderator", mentionableOwner{kind: mentionableOwnerRole, id: "moderator"})
		}},
		{"users", func() snapshotProjection { return NewUserProjection(nil, nil) }, func(raw snapshotProjection) {
			p := raw.(*UserProjection)
			p.users["U1"] = &projectedUser{user: &corev1.User{Id: "U1", Kind: corev1.UserKind_USER_KIND_HUMAN, CreatedAt: timestamppb.New(now)}, deleted: true, verifiedEmail: make(map[string]projectedVerifiedEmail)}
			p.replayGuard.highestSeq = 41
			p.replayGuard.completeReplay()
		}},
	}

	expectedContract := map[string]string{
		"room_directory": "v1", "server_config": "v1", "room_group_layout": "v1",
		"room_timeline": "v1", "call_state": "v1", "assets": "v1", "reactions": "v1",
		"content_keys": "v1", "rbac": "v1", "mentionables": "v1", "users": "v4",
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
			if id != expectedContract[tt.name] {
				t.Fatalf("contract ID = %q, want %q", id, expectedContract[tt.name])
			}
		})
	}
}
