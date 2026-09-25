package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
	// These are protocol expectations, independent of PublishSources.
	expected := [][]string{
		{},
		{"microphone"},
		{"camera"},
		{"microphone", "camera"},
		{"screen_share", "screen_share_audio"},
		{"microphone", "screen_share", "screen_share_audio"},
		{"camera", "screen_share", "screen_share_audio"},
		{"microphone", "camera", "screen_share", "screen_share_audio"},
	}
	for mask, sources := range expected {
		t.Run(fmt.Sprintf("media_%03b", mask), func(t *testing.T) {
			permissions := CallPermissions{Voice: mask&1 != 0, Camera: mask&2 != 0, ScreenShare: mask&4 != 0}
			token, err := GenerateVoiceCallToken("key", "secret", "room", "user", "User", "user", "", false, "e2ee", permissions, time.Time{}, "call")
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
			require.ElementsMatch(t, sources, grant.Sources)
		})
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
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallCamera))
	service.updates = nil
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Len(t, service.updates, 2)
	require.ElementsMatch(t, []livekit.TrackSource{livekit.TrackSource_SCREEN_SHARE, livekit.TrackSource_SCREEN_SHARE_AUDIO}, service.updates[0].Permission.CanPublishSources)
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallCamera))
	service.updates = nil
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Len(t, service.updates, 2)
	require.Contains(t, service.updates[0].Permission.CanPublishSources, livekit.TrackSource_CAMERA)
	require.NotContains(t, service.updates[0].Permission.CanPublishSources, livekit.TrackSource_MICROPHONE)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallScreenShare))
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Equal(t, "companion", service.removals[0].Identity)
	// Losing the last media source leaves the main participant subscribed.
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallCamera))
	service.updates = nil
	_, err = client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Len(t, service.updates, 1)
	require.False(t, service.updates[0].Permission.CanPublish)
	require.True(t, service.updates[0].Permission.CanSubscribe)
	require.False(t, service.updates[0].Permission.CanPublishData)
	require.Empty(t, service.updates[0].Permission.CanPublishSources)
	// The fake continues to list the removed companion on each scan.
	require.Len(t, service.removals, 2)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermCallJoin))
	snapshots, err := client.ListCallParticipants(ctx)
	require.NoError(t, err)
	require.Empty(t, snapshots[0].UserIDs)
	require.Len(t, service.removals, 4)
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

// Each scoped decision must affect only its matching call capability. Clear
// removes the last grant, so absence cannot pass through the default baseline.
func TestCallPermissionsScopeMatrix(t *testing.T) {
	for _, scopeKind := range []MatrixScopeKind{MatrixScopeServer, MatrixScopeGroup, MatrixScopeRoom, MatrixScopeDM} {
		t.Run(string(scopeKind), func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			user, err := c.CreateUser(ctx, SystemActorID, "call-matrix-user", "User", "password")
			require.NoError(t, err)
			room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-matrix", "")
			require.NoError(t, err)
			manager, err := c.CreateUser(ctx, SystemActorID, "call-matrix-manager", "Manager", "password")
			require.NoError(t, err)
			require.NoError(t, c.AssignServerRole(ctx, SystemActorID, manager.Id, RoleOwner))
			scope := PermissionTargetScope{Kind: scopeKind}
			switch scopeKind {
			case MatrixScopeGroup:
				group, err := c.CreateRoomGroup(ctx, SystemActorID, "Call matrix", "")
				require.NoError(t, err)
				require.NoError(t, c.MoveRoomToGroup(ctx, SystemActorID, room.Id, group.Id))
				scope.ID = group.Id
			case MatrixScopeRoom:
				scope.ID = room.Id
			case MatrixScopeDM:
				other, err := c.CreateUser(ctx, SystemActorID, "call-matrix-other", "Other", "password")
				require.NoError(t, err)
				room, _, err = c.roomCommands.StartDM(ctx, RoomStartDMInput{ActorID: user.Id, ParticipantIDs: []string{other.Id}})
				require.NoError(t, err)
			}
			if scopeKind != MatrixScopeDM {
				_, err = c.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
				require.NoError(t, err)
			}
			cases := []struct {
				permission Permission
				disable    func(*CallPermissions)
			}{
				{PermCallStart, func(p *CallPermissions) { p.Start = false }},
				{PermCallJoin, func(p *CallPermissions) { p.Join = false }},
				{PermCallVoice, func(p *CallPermissions) { p.Voice = false }},
				{PermCallCamera, func(p *CallPermissions) { p.Camera = false }},
				{PermCallScreenShare, func(p *CallPermissions) { p.ScreenShare = false }},
			}
			for _, tc := range cases {
				t.Run(string(tc.permission), func(t *testing.T) {
					require.NoError(t, c.RevokeServerPermission(ctx, SystemActorID, RoleEveryone, tc.permission))
					for _, state := range []PermissionState{PermissionStateAllow, PermissionStateDeny, PermissionStateNone} {
						t.Run(string(state), func(t *testing.T) {
							require.NoError(t, c.SetRolePermissionState(ctx, manager.Id, RoleEveryone, scope, tc.permission, state))
							expected := CallPermissions{Start: true, Join: true, Voice: true, Camera: true, ScreenShare: true}
							if state != PermissionStateAllow {
								tc.disable(&expected)
							}
							for _, starting := range []bool{false, true} {
								actual, err := c.AuthorizeCall(ctx, user.Id, room.Id, starting)
								if !expected.Join || (starting && !expected.Start) {
									require.ErrorIs(t, err, ErrPermissionDenied)
								} else {
									require.NoError(t, err)
								}
								require.Equal(t, expected, actual)
							}
						})
					}
					require.NoError(t, c.GrantServerPermission(ctx, SystemActorID, RoleEveryone, tc.permission))
				})
			}
		})
	}
}

// A call connection keeps the privileged-mode state of the session that
// requested its token. The owner override ends with that deadline.
func TestCallPermissionReconciliationGatesOwnerOverride(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "call-gated-owner", "Owner", "password")
	require.NoError(t, err)
	require.NoError(t, c.AssignOwnerRole(ctx, owner.Id))
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-gated-owner-room", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, owner.Id, KindChannel, owner.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, c.JoinVoiceCall(ctx, owner.Id, room.Id))
	snapshot, err := c.GetCallSnapshot(room.Id)
	require.NoError(t, err)
	require.NoError(t, c.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermCallJoin))

	name := LiveKitRoomName("", KindChannel, room.Id, snapshot.Call.CallID)
	scan := func(privilegedUntil time.Time) *callPermissionRoomService {
		t.Helper()
		metadata, err := json.Marshal(participantMetadata{PrivilegedUntil: privilegedUntilUnix(privilegedUntil)})
		require.NoError(t, err)
		service := &callPermissionRoomService{fakeLiveKitRoomService: fakeLiveKitRoomService{
			rooms:        []string{name},
			participants: map[string][]string{name: {owner.Id}},
			metadata:     map[string]string{owner.Id: string(metadata)},
		}}
		client := &liveKitRoomClient{service: service, core: c, apiKey: "key", apiSecret: "secret"}
		_, err = client.ListCallParticipants(ctx)
		require.NoError(t, err)
		return service
	}
	require.Empty(t, scan(time.Now().Add(time.Minute)).removals, "active privileged mode keeps the owner override")
	removed := scan(time.Now().Add(-time.Second)).removals
	require.Len(t, removed, 1, "expired privileged mode removes the owner")
	require.Equal(t, owner.Id, removed[0].Identity)
	require.Len(t, scan(time.Time{}).removals, 1, "a token without privileged mode has no owner override")
}
