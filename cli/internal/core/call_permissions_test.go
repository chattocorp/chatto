package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/livekit/protocol/livekit"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestCallPermissionsAdmissionAndSources(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "call-permissions-user", "User", "password")
	require.NoError(t, err)
	other, err := c.CreateUser(ctx, SystemActorID, "call-permissions-other", "Other", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-permissions", "")
	require.NoError(t, err)
	require.ErrorIs(t, c.JoinVoiceCall(ctx, user.Id, room.Id), ErrNotRoomMember)
	for _, u := range []string{user.Id, other.Id} {
		_, err = c.JoinRoom(ctx, u, KindChannel, u, room.Id)
		require.NoError(t, err)
	}
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallStart))
	require.ErrorIs(t, c.JoinVoiceCall(ctx, user.Id, room.Id), ErrPermissionDenied)
	require.NoError(t, c.JoinVoiceCall(ctx, other.Id, room.Id))
	require.NoError(t, c.JoinVoiceCall(ctx, user.Id, room.Id))
	for _, permission := range []Permission{PermCallVoice, PermCallCamera, PermCallScreenShare} {
		require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, permission))
	}
	allowed, err := c.AuthorizeCall(ctx, user.Id, room.Id, false)
	require.NoError(t, err)
	require.True(t, allowed.Join)
	require.Empty(t, allowed.PublishSources())
	// Idempotent join still checks revoked authority.
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallJoin))
	require.ErrorIs(t, c.JoinVoiceCall(ctx, user.Id, room.Id), ErrPermissionDenied)
	_, _, err = c.VoiceCallRoomForMember(ctx, user.Id, room.Id)
	require.NoError(t, err, "observer reads remain available")
	require.NoError(t, c.RecordCallParticipantLeft(ctx, room.Id, user.Id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
}

func TestCallJoinRetryCannotStartWithoutPermission(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "call-retry-user", "User", "password")
	require.NoError(t, err)
	other, err := c.CreateUser(ctx, SystemActorID, "call-retry-other", "Other", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-retry", "")
	require.NoError(t, err)
	for _, u := range []string{user.Id, other.Id} {
		_, err = c.JoinRoom(ctx, u, KindChannel, u, room.Id)
		require.NoError(t, err)
	}
	require.NoError(t, c.JoinVoiceCall(ctx, other.Id, room.Id))
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallStart))
	attempts := 0
	err = c.callModel.appendParticipantTransitionAuthorized(ctx, room.Id, user.Id, true, "", evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER, func(snapshot CallRoomSnapshot) error {
		attempts++
		_, err := c.AuthorizeCall(ctx, user.Id, room.Id, snapshot.Call.CallID == "")
		if err != nil {
			return err
		}
		if attempts == 1 {
			require.NoError(t, c.RecordCallParticipantLeft(ctx, room.Id, other.Id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER))
		}
		return nil
	})
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.Equal(t, 2, attempts)
	snapshot, err := c.GetCallSnapshot(room.Id)
	require.NoError(t, err)
	require.Empty(t, snapshot.Call.CallID)
}

func TestCallTokenPermissionCombinations(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		permissions := CallPermissions{Voice: mask&1 != 0, Camera: mask&2 != 0, ScreenShare: mask&4 != 0}
		token, err := GenerateVoiceCallToken("key", "secret", "room", "user", "User", "user", "", false, "e2ee", permissions, "call")
		require.NoError(t, err)
		parsed, _, err := jwt.NewParser().ParseUnverified(token.Token, jwt.MapClaims{})
		require.NoError(t, err)
		data, err := json.Marshal(parsed.Claims.(jwt.MapClaims)["video"])
		require.NoError(t, err)
		var grant struct {
			CanPublish     bool     `json:"canPublish"`
			CanSubscribe   bool     `json:"canSubscribe"`
			CanPublishData bool     `json:"canPublishData"`
			Sources        []string `json:"canPublishSources"`
		}
		require.NoError(t, json.Unmarshal(data, &grant))
		require.Equal(t, mask != 0, grant.CanPublish)
		require.True(t, grant.CanSubscribe)
		require.False(t, grant.CanPublishData)
		var sources []string
		for _, source := range permissions.PublishSources() {
			sources = append(sources, strings.ToLower(source.String()))
		}
		require.ElementsMatch(t, sources, grant.Sources)
	}
}

func TestCallPermissionDefaultsDoNotReturnAfterClear(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	require.NoError(t, c.RevokeServerPermission(ctx, SystemActorID, RoleEveryone, PermCallVoice))
	require.NoError(t, c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermCallCamera))
	before, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	require.NoError(t, c.seedCallPermissions(ctx))
	after, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	require.Equal(t, before, after)
}

type callPermissionRoomService struct {
	fakeLiveKitRoomService
	updates   []*livekit.UpdateParticipantRequest
	removals  []*livekit.RoomParticipantIdentity
	updateErr error
}

func (s *callPermissionRoomService) UpdateParticipant(_ context.Context, request *livekit.UpdateParticipantRequest) (*livekit.ParticipantInfo, error) {
	s.updates = append(s.updates, request)
	return &livekit.ParticipantInfo{}, s.updateErr
}
func (s *callPermissionRoomService) RemoveParticipant(_ context.Context, request *livekit.RoomParticipantIdentity) (*livekit.RemoveParticipantResponse, error) {
	s.removals = append(s.removals, request)
	return &livekit.RemoveParticipantResponse{}, nil
}

func TestCallPermissionReconciliation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "call-reconcile-user", "User", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-permission-reconcile", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, c.JoinVoiceCall(ctx, user.Id, room.Id))
	snapshot, err := c.GetCallSnapshot(room.Id)
	require.NoError(t, err)
	callID := snapshot.Call.CallID
	name := LiveKitRoomName("", KindChannel, room.Id, callID)
	service := &callPermissionRoomService{fakeLiveKitRoomService: fakeLiveKitRoomService{rooms: []string{name}, participants: map[string][]string{name: {user.Id, "companion"}}, metadata: map[string]string{"companion": `{"publisherKind":"game_share","ownerIdentity":"` + user.Id + `"}`}}}
	client := &liveKitRoomClient{service: service, core: c, apiKey: "key", apiSecret: "secret"}
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallVoice))
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Len(t, service.updates, 2)
	require.NotContains(t, service.updates[0].Permission.CanPublishSources, livekit.TrackSource_MICROPHONE)
	require.Contains(t, service.updates[1].Permission.CanPublishSources, livekit.TrackSource_MICROPHONE, "companion audio is controlled by screenshare")
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallScreenShare))
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Equal(t, "companion", service.removals[0].Identity)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallJoin))
	snapshots, err := client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Empty(t, snapshots[0].UserIDs)
	require.Len(t, service.removals, 3)
	// Permission-sync failures must not count toward the global listing outage.
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallJoin))
	service.updateErr = errors.New("update unavailable")
	// Exercise the reconciliation operation without replacing a running worker's
	// startup-only provider. The local model shares the authoritative state.
	model := *c.callModel
	model.livekit = client
	require.Error(t, model.ReconcileWithLiveKit(ctx))
	require.Error(t, model.ReconcileWithLiveKit(ctx))
	require.Error(t, model.ReconcileWithLiveKit(ctx))
	snapshot, err = c.GetCallSnapshot(room.Id)
	require.NoError(t, err)
	require.Equal(t, callID, snapshot.Call.CallID)
}

func TestCallPermissionsDMScopeAndBotDelegation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "call-owner", "Owner", "password")
	require.NoError(t, err)
	other, err := c.CreateUser(ctx, SystemActorID, "call-dm-other", "Other", "password")
	require.NoError(t, err)
	dm, _, err := c.roomCommands.StartDM(ctx, RoomStartDMInput{ActorID: owner.Id, ParticipantIDs: []string{other.Id}})
	require.NoError(t, err)
	require.NoError(t, c.JoinVoiceCall(ctx, owner.Id, dm.Id))
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, owner.Id, PermRoleManage))
	require.NoError(t, c.SetRolePermissionState(ctx, owner.Id, RoleEveryone, PermissionTargetScope{Kind: MatrixScopeDM}, PermCallVoice, PermissionStateDeny))
	permissions, err := c.AuthorizeCall(ctx, owner.Id, dm.Id, false)
	require.NoError(t, err)
	require.False(t, permissions.Voice)
	bot, err := c.CreateBot(ctx, owner.Id, "call_bot", "Call Bot")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-bot-room", "")
	require.NoError(t, err)
	// Add membership without implying any bot permission.
	_, err = c.AddMember(ctx, SystemActorID, KindChannel, room.Id, bot.User.Id)
	require.NoError(t, err)
	require.ErrorIs(t, c.JoinVoiceCall(ctx, bot.User.Id, room.Id), ErrPermissionDenied)
	for _, permission := range callPermissionIDs() {
		require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, bot.User.Id, permission))
	}
	require.NoError(t, c.JoinVoiceCall(ctx, bot.User.Id, room.Id))
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, owner.Id, PermCallVoice))
	permissions, err = c.AuthorizeCall(ctx, bot.User.Id, room.Id, false)
	require.NoError(t, err)
	require.False(t, permissions.Voice)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, owner.Id, PermCallJoin))
	_, err = c.AuthorizeCall(ctx, bot.User.Id, room.Id, false)
	require.ErrorIs(t, err, ErrPermissionDenied)
}

func TestCallPermissionUpgradeConcurrentAndRestart(t *testing.T) {
	h := newTestEventHarness(t)
	ctx := testContext(t)
	// Seed a pre-call-permission server using the same historical RBAC events.
	defaults := defaultRBACDecisions()
	var oldDefaults []rbacSeedDecision
	for _, decision := range defaults {
		if !strings.HasPrefix(string(decision.permission), "call.") {
			oldDefaults = append(oldDefaults, decision)
		}
	}
	entries := rbacSeedEntries(defaultRBACRoles(), nil, oldDefaults)
	entries[0].HasOCC = true
	entries[0].FilterSubject = evtstream.RBACSubjectFilter()
	_, err := h.publisher.AppendBatch(ctx, entries)
	require.NoError(t, err)
	first := &ChattoCore{EventPublisher: h.publisher}
	second := &ChattoCore{EventPublisher: h.publisher}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, replica := range []*ChattoCore{first, second} {
		group.Add(1)
		go func() { defer group.Done(); results <- replica.seedCallPermissions(ctx) }()
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	history, seq, err := h.publisher.SubjectEvents(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	count := 0
	for _, event := range history {
		if strings.HasPrefix(event.GetRbacPermissionGranted().GetPermission(), "call.") {
			count++
		}
	}
	require.Equal(t, 5, count)
	require.NoError(t, second.seedCallPermissions(ctx))
	_, after, err := h.publisher.SubjectEvents(ctx, evtstream.RBACSubjectFilter())
	require.NoError(t, err)
	require.Equal(t, seq, after)
}
