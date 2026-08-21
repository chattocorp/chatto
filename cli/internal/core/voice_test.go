package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

type fakeLiveKitParticipantLister struct {
	snapshots []liveKitParticipantSnapshot
	err       error
}

func (f fakeLiveKitParticipantLister) ListCallParticipants(ctx context.Context) ([]liveKitParticipantSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]liveKitParticipantSnapshot(nil), f.snapshots...), nil
}

type removedLiveKitParticipant struct {
	spaceID string
	roomID  string
	callID  string
	userID  string
}

type recordingLiveKitParticipantClient struct {
	removeErr error
	removed   []removedLiveKitParticipant
	snapshots []liveKitParticipantSnapshot
}

func (r *recordingLiveKitParticipantClient) ListCallParticipants(context.Context) ([]liveKitParticipantSnapshot, error) {
	return append([]liveKitParticipantSnapshot(nil), r.snapshots...), nil
}

func (r *recordingLiveKitParticipantClient) RemoveCallParticipant(_ context.Context, spaceID, roomID, callID, userID string) error {
	if r.removeErr != nil {
		return r.removeErr
	}
	r.removed = append(r.removed, removedLiveKitParticipant{
		spaceID: spaceID,
		roomID:  roomID,
		callID:  callID,
		userID:  userID,
	})
	return nil
}

type failingShredCallKeyStore struct {
	delegate kms.CallKeyStore
	err      error
}

func (f failingShredCallKeyStore) CreateCallKey(ctx context.Context, callID string) (string, string, error) {
	return f.delegate.CreateCallKey(ctx, callID)
}

func (f failingShredCallKeyStore) GetCallKey(ctx context.Context, keyRef string) (string, error) {
	return f.delegate.GetCallKey(ctx, keyRef)
}

func (f failingShredCallKeyStore) CallKeyExists(ctx context.Context, keyRef string) (bool, error) {
	return f.delegate.CallKeyExists(ctx, keyRef)
}

func (f failingShredCallKeyStore) ShredCallKey(context.Context, string) error {
	return f.err
}

type hookedCallKeyStore struct {
	keys      map[string]string
	beforeGet func(string) error
}

func (s *hookedCallKeyStore) CreateCallKey(context.Context, string) (string, string, error) {
	return "", "", errors.New("CreateCallKey is not supported")
}

func (s *hookedCallKeyStore) GetCallKey(_ context.Context, keyRef string) (string, error) {
	if s.beforeGet != nil {
		beforeGet := s.beforeGet
		s.beforeGet = nil
		if err := beforeGet(keyRef); err != nil {
			return "", err
		}
	}
	key, ok := s.keys[keyRef]
	if !ok {
		return "", errors.New("call key not found")
	}
	return key, nil
}

func (s *hookedCallKeyStore) CallKeyExists(_ context.Context, keyRef string) (bool, error) {
	_, ok := s.keys[keyRef]
	return ok, nil
}

func (s *hookedCallKeyStore) ShredCallKey(_ context.Context, keyRef string) error {
	delete(s.keys, keyRef)
	return nil
}

type recordingCallLogger struct {
	warnMessage string
	warnKeyvals []interface{}
}

type fakeLiveKitRoomService struct {
	rooms           []string
	participants    map[string][]string
	metadata        map[string]string
	participantErrs map[string]error
	removed         []livekit.RoomParticipantIdentity
	removeErr       error
}

func (f fakeLiveKitRoomService) ListRooms(context.Context, *livekit.ListRoomsRequest) (*livekit.ListRoomsResponse, error) {
	rooms := make([]*livekit.Room, 0, len(f.rooms))
	for _, name := range f.rooms {
		rooms = append(rooms, &livekit.Room{Name: name})
	}
	return &livekit.ListRoomsResponse{Rooms: rooms}, nil
}

func (f fakeLiveKitRoomService) ListParticipants(_ context.Context, req *livekit.ListParticipantsRequest) (*livekit.ListParticipantsResponse, error) {
	if err := f.participantErrs[req.GetRoom()]; err != nil {
		return nil, err
	}
	userIDs := f.participants[req.GetRoom()]
	participants := make([]*livekit.ParticipantInfo, 0, len(userIDs))
	for _, userID := range userIDs {
		participants = append(participants, &livekit.ParticipantInfo{Identity: userID, Metadata: f.metadata[userID]})
	}
	return &livekit.ListParticipantsResponse{Participants: participants}, nil
}

func (f *fakeLiveKitRoomService) RemoveParticipant(_ context.Context, req *livekit.RoomParticipantIdentity) (*livekit.RemoveParticipantResponse, error) {
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	f.removed = append(f.removed, *req)
	return &livekit.RemoveParticipantResponse{}, nil
}

func (l *recordingCallLogger) Debug(interface{}, ...interface{}) {}
func (l *recordingCallLogger) Info(interface{}, ...interface{})  {}
func (l *recordingCallLogger) Error(interface{}, ...interface{}) {}

func (l *recordingCallLogger) Warn(msg interface{}, keyvals ...interface{}) {
	l.warnMessage = msg.(string)
	l.warnKeyvals = append([]interface{}(nil), keyvals...)
}

func loggedValue(keyvals []interface{}, key string) interface{} {
	for i := 0; i+1 < len(keyvals); i += 2 {
		if keyvals[i] == key {
			return keyvals[i+1]
		}
	}
	return nil
}

func activeCallIDForTest(t *testing.T, c *ChattoCore, roomID string) string {
	t.Helper()
	active, ok := c.callModel.activeCall(roomID)
	if !ok || active.CallID == "" {
		t.Fatalf("Expected active call for room %s", roomID)
	}
	return active.CallID
}

func TestLiveKitRoomName(t *testing.T) {
	tests := []struct {
		name     string
		serverID string
		kind     RoomKind
		roomID   string
		callID   string
		want     string
	}{
		{
			name:   "channel without server ID",
			kind:   KindChannel,
			roomID: "room456",
			want:   "server_room456",
		},
		{
			name:     "with server ID",
			serverID: "my-instance",
			kind:     KindChannel,
			roomID:   "room456",
			want:     "my-instance.server_room456",
		},
		{
			name:   "channel with call ID",
			kind:   KindChannel,
			roomID: "room456",
			callID: "call789",
			want:   "server_room456@call789",
		},
		{
			name:     "DM with server ID and call ID",
			serverID: "my-instance",
			kind:     KindDM,
			roomID:   "room456",
			callID:   "call789",
			want:     "my-instance.DM_room456@call789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LiveKitRoomName(tt.serverID, tt.kind, tt.roomID, tt.callID)
			if got != tt.want {
				t.Errorf("LiveKitRoomName(%q, %q, %q, %q) = %q, want %q", tt.serverID, tt.kind, tt.roomID, tt.callID, got, tt.want)
			}
		})
	}
}

func TestParseLiveKitRoomIdentity(t *testing.T) {
	tests := []struct {
		name        string
		lkRoomName  string
		wantSpaceID string
		wantRoomID  string
		wantCallID  string
	}{
		{
			name:        "unprefixed basic",
			lkRoomName:  "space123_room456",
			wantSpaceID: "space123",
			wantRoomID:  "room456",
		},
		{
			name:        "unprefixed room ID with underscores",
			lkRoomName:  "space123_room_with_underscores",
			wantSpaceID: "space123",
			wantRoomID:  "room_with_underscores",
		},
		{
			name:        "prefixed basic",
			lkRoomName:  "my-instance.space123_room456",
			wantSpaceID: "space123",
			wantRoomID:  "room456",
		},
		{
			name:        "prefixed room ID with underscores",
			lkRoomName:  "my-instance.space123_room_with_underscores",
			wantSpaceID: "space123",
			wantRoomID:  "room_with_underscores",
		},
		{
			name:        "unprefixed with call ID",
			lkRoomName:  "space123_room456@call789",
			wantSpaceID: "space123",
			wantRoomID:  "room456",
			wantCallID:  "call789",
		},
		{
			name:        "prefixed with room underscores and call ID",
			lkRoomName:  "my-instance.space123_room_with_underscores@call789",
			wantSpaceID: "space123",
			wantRoomID:  "room_with_underscores",
			wantCallID:  "call789",
		},
		{
			name:        "empty string",
			lkRoomName:  "",
			wantSpaceID: "",
			wantRoomID:  "",
		},
		{
			name:        "no underscore",
			lkRoomName:  "nounderscore",
			wantSpaceID: "",
			wantRoomID:  "",
		},
		{
			name:        "prefixed no underscore",
			lkRoomName:  "instance.nounderscore",
			wantSpaceID: "",
			wantRoomID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpace, gotRoom, gotCall := ParseLiveKitRoomIdentity(tt.lkRoomName)
			if gotSpace != tt.wantSpaceID || gotRoom != tt.wantRoomID || gotCall != tt.wantCallID {
				t.Errorf("ParseLiveKitRoomIdentity(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.lkRoomName, gotSpace, gotRoom, gotCall, tt.wantSpaceID, tt.wantRoomID, tt.wantCallID)
			}
		})
	}
}

func TestParseLiveKitRoomServerID(t *testing.T) {
	tests := []struct {
		name       string
		lkRoomName string
		want       string
	}{
		{
			name:       "prefixed",
			lkRoomName: "my-instance.space123_room456",
			want:       "my-instance",
		},
		{
			name:       "unprefixed",
			lkRoomName: "space123_room456",
			want:       "",
		},
		{
			name:       "empty",
			lkRoomName: "",
			want:       "",
		},
		{
			name:       "k8s-style name prefix",
			lkRoomName: "prod-tenant-1.space123_room456",
			want:       "prod-tenant-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLiveKitRoomServerID(tt.lkRoomName)
			if got != tt.want {
				t.Errorf("ParseLiveKitRoomServerID(%q) = %q, want %q", tt.lkRoomName, got, tt.want)
			}
		})
	}
}

func TestParseParticipantMetadata(t *testing.T) {
	tests := []struct {
		name      string
		metadata  string
		wantLogin string
		wantAvURL string
	}{
		{
			name:      "full metadata",
			metadata:  `{"login":"alice","avatarUrl":"https://example.com/a.jpg"}`,
			wantLogin: "alice",
			wantAvURL: "https://example.com/a.jpg",
		},
		{
			name:      "login only",
			metadata:  `{"login":"bob"}`,
			wantLogin: "bob",
			wantAvURL: "",
		},
		{
			name:      "empty string",
			metadata:  "",
			wantLogin: "",
			wantAvURL: "",
		},
		{
			name:      "invalid JSON",
			metadata:  "not json",
			wantLogin: "",
			wantAvURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := ParseParticipantMetadata(tt.metadata)
			if md.Login != tt.wantLogin {
				t.Errorf("Login = %q, want %q", md.Login, tt.wantLogin)
			}
			if md.AvatarURL != tt.wantAvURL {
				t.Errorf("AvatarURL = %q, want %q", md.AvatarURL, tt.wantAvURL)
			}
		})
	}
}

func TestGenerateVoiceCallToken(t *testing.T) {
	apiKey := "devkey"
	apiSecret := "secret"
	roomName := "space123_room456"
	userID := "user789"
	displayName := "Test User"
	login := "testuser"
	avatarURL := "https://example.com/avatar.jpg"
	callID := "call789"

	result, err := GenerateVoiceCallToken(apiKey, apiSecret, roomName, userID, displayName, login, avatarURL, true, "e2ee-test-key", callID)
	if err != nil {
		t.Fatalf("GenerateVoiceCallToken() error = %v", err)
	}
	if result == nil {
		t.Fatal("GenerateVoiceCallToken() returned nil")
	}
	if result.Token == "" {
		t.Fatal("GenerateVoiceCallToken() returned empty token")
	}
	if result.E2EEKey != "e2ee-test-key" {
		t.Fatalf("E2EEKey = %q, want %q", result.E2EEKey, "e2ee-test-key")
	}
	if result.CallID != callID {
		t.Fatalf("CallID = %q, want %q", result.CallID, callID)
	}

	// Parse the JWT to verify claims (without full validation since we're using a test secret)
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(result.Token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("Failed to parse JWT: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("Failed to cast claims")
	}

	// Check identity (sub claim in LiveKit tokens)
	if sub, ok := claims["sub"].(string); !ok || sub != userID {
		t.Errorf("Token sub = %v, want %q", claims["sub"], userID)
	}

	// Check name
	if name, ok := claims["name"].(string); !ok || name != displayName {
		t.Errorf("Token name = %v, want %q", claims["name"], displayName)
	}

	// Check issuer (should be the API key)
	if iss, ok := claims["iss"].(string); !ok || iss != apiKey {
		t.Errorf("Token iss = %v, want %q", claims["iss"], apiKey)
	}

	// Check metadata contains login and avatarUrl
	if md, ok := claims["metadata"].(string); ok {
		if !strings.Contains(md, `"login":"testuser"`) {
			t.Errorf("Token metadata missing login: %s", md)
		}
		if !strings.Contains(md, `"avatarUrl":"https://example.com/avatar.jpg"`) {
			t.Errorf("Token metadata missing avatarUrl: %s", md)
		}
		if !strings.Contains(md, `"isBot":true`) {
			t.Errorf("Token metadata missing bot status: %s", md)
		}
		if !strings.Contains(md, `"callId":"call789"`) {
			t.Errorf("Token metadata missing callId: %s", md)
		}
	} else {
		t.Error("Token missing metadata claim")
	}

	// Check video grant
	video, ok := claims["video"].(map[string]interface{})
	if !ok {
		t.Fatal("Token missing video grant")
	}
	if roomJoin, ok := video["roomJoin"].(bool); !ok || !roomJoin {
		t.Error("Token video.roomJoin should be true")
	}
	if room, ok := video["room"].(string); !ok || room != roomName {
		t.Errorf("Token video.room = %v, want %q", video["room"], roomName)
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Error("Token missing exp claim")
	}
	nbf, ok := claims["nbf"].(float64)
	if !ok {
		t.Error("Token missing nbf claim")
	}
	if ttl := time.Duration(exp-nbf) * time.Second; ttl != VoiceCallTokenTTL {
		t.Errorf("Token TTL = %s, want %s", ttl, VoiceCallTokenTTL)
	}
}

func TestGenerateVoiceCallToken_NoAvatar(t *testing.T) {
	result, err := GenerateVoiceCallToken("key", "secret", "room", "user1", "User One", "userone", "", false, "e2ee-test-key")
	if err != nil {
		t.Fatalf("GenerateVoiceCallToken() error = %v", err)
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(result.Token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("Failed to parse JWT: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	md, ok := claims["metadata"].(string)
	if !ok {
		t.Fatal("Token missing metadata claim")
	}

	// avatarUrl should be omitted (omitempty) when empty
	if strings.Contains(md, "avatarUrl") {
		t.Errorf("Token metadata should omit empty avatarUrl: %s", md)
	}
	if !strings.Contains(md, `"login":"userone"`) {
		t.Errorf("Token metadata missing login: %s", md)
	}
}

func TestGenerateCallMediaPublisherToken(t *testing.T) {
	result, err := GenerateCallMediaPublisherToken(
		"devkey",
		"secret",
		"space123_room456@call789",
		"publisher123",
		"user789",
		"Test User",
		"e2ee-test-key",
		"call789",
		ParticipantPublisherKindGameShare,
	)
	if err != nil {
		t.Fatalf("GenerateCallMediaPublisherToken() error = %v", err)
	}
	if result.CallID != "call789" || result.E2EEKey != "e2ee-test-key" {
		t.Fatalf("GenerateCallMediaPublisherToken() = %+v", result)
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(result.Token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse JWT: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["sub"] != "publisher123" {
		t.Fatalf("Token sub = %v, want publisher123", claims["sub"])
	}
	metadata, _ := claims["metadata"].(string)
	if !strings.Contains(metadata, `"publisherKind":"game_share"`) || !strings.Contains(metadata, `"ownerIdentity":"user789"`) {
		t.Fatalf("Token metadata = %q", metadata)
	}
	if !IsCallMediaPublisher(metadata) {
		t.Fatal("IsCallMediaPublisher() = false")
	}
	video, ok := claims["video"].(map[string]interface{})
	if !ok {
		t.Fatal("Token missing video grant")
	}
	if video["canSubscribe"] != false || video["canPublishData"] != false {
		t.Fatalf("Token video grant = %+v, want subscribe/data disabled", video)
	}
	sources, _ := video["canPublishSources"].([]interface{})
	if fmt.Sprint(sources) != "[screen_share microphone]" {
		t.Fatalf("Token publish sources = %v", sources)
	}
	exp, _ := claims["exp"].(float64)
	nbf, _ := claims["nbf"].(float64)
	if ttl := time.Duration(exp-nbf) * time.Second; ttl != CallMediaPublisherTokenTTL {
		t.Fatalf("Token TTL = %s, want %s", ttl, CallMediaPublisherTokenTTL)
	}
}

// ============================================================================
// Call State Projection Tests (require embedded NATS)
// ============================================================================

func TestCallModel_ReadBoundaryReturnsDetachedState(t *testing.T) {
	projection := NewCallStateProjection()
	roomID := "room-call-model"
	callID := "call-call-model"

	if err := projection.Apply(newEvent("user1", &corev1.Event{
		Event: &corev1.Event_VoiceCallStarted{
			VoiceCallStarted: &corev1.CallStartedEvent{
				RoomId:     roomID,
				CallId:     callID,
				E2EeKeyRef: "key-ref-call-model",
			},
		},
	}), 10); err != nil {
		t.Fatalf("Apply VoiceCallStarted: %v", err)
	}
	if err := projection.Apply(newEvent("user1", &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantJoined{
			VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
				RoomId: roomID,
				CallId: callID,
			},
		},
	}), 11); err != nil {
		t.Fatalf("Apply VoiceCallParticipantJoined: %v", err)
	}

	model := &CallModel{
		callState: detachedTestProjectionHandle(projection),
		callKeys: &hookedCallKeyStore{keys: map[string]string{
			"key-ref-call-model": "key-call-model",
		}},
	}
	active, ok := model.activeCall(roomID)
	if !ok || active.CallID != callID {
		t.Fatalf("activeCall() = %#v, %v; want call %q", active, ok, callID)
	}
	active.CallID = "mutated"
	if got, ok := model.activeCall(roomID); !ok || got.CallID != callID {
		t.Fatalf("activeCall() after caller mutation = %#v, %v; want call %q", got, ok, callID)
	}

	participants := model.participants(roomID)
	if len(participants) != 1 || participants[0].UserID != "user1" {
		t.Fatalf("participants() = %#v, want user1", participants)
	}
	participants[0].UserID = "mutated"
	if got := model.participants(roomID); len(got) != 1 || got[0].UserID != "user1" {
		t.Fatalf("participants() after caller mutation = %#v, want user1", got)
	}

	snapshot := model.roomSnapshot(roomID)
	if snapshot.Seq != 11 || snapshot.Call.CallID != callID || len(snapshot.Participants) != 1 {
		t.Fatalf("roomSnapshot() = %#v, want seq 11 with active call and participant", snapshot)
	}
	snapshot.Participants[0].UserID = "mutated"
	if got := model.roomSnapshot(roomID); len(got.Participants) != 1 || got.Participants[0].UserID != "user1" {
		t.Fatalf("roomSnapshot() after caller mutation = %#v, want user1", got)
	}

	roomIDs := model.activeRoomIDs()
	if !slices.Equal(roomIDs, []string{roomID}) {
		t.Fatalf("activeRoomIDs() = %v, want %v", roomIDs, []string{roomID})
	}
	roomIDs[0] = "mutated"
	if got := model.activeRoomIDs(); !slices.Equal(got, []string{roomID}) {
		t.Fatalf("activeRoomIDs() after caller mutation = %v, want %v", got, []string{roomID})
	}

	core := &ChattoCore{callModel: model}
	active, ok, err := core.GetActiveCall(roomID)
	if err != nil || !ok || active.CallID != callID {
		t.Fatalf("GetActiveCall() = %#v, %v, %v; want call %q", active, ok, err, callID)
	}
	if _, ok, err := core.GetActiveCall("missing-room"); err != nil || ok {
		t.Fatalf("GetActiveCall(missing-room) ok, err = %v, %v; want false, nil", ok, err)
	}
	coreSnapshot, err := core.GetCallSnapshot(roomID)
	if err != nil ||
		coreSnapshot.Call.CallID != callID ||
		len(coreSnapshot.Participants) != 1 ||
		coreSnapshot.Participants[0].CallID != callID {
		t.Fatalf("GetCallSnapshot() = %#v, %v; want one consistent call generation", coreSnapshot, err)
	}
	access, err := core.GetVoiceCallAccessMaterial(context.Background(), roomID)
	if err != nil || access.CallID != callID || access.E2EEKey != "key-call-model" {
		t.Fatalf("GetVoiceCallAccessMaterial() = %#v, %v; want matching call access", access, err)
	}
}

func TestCallModel_GetAccessMaterialRejectsCallGenerationTransition(t *testing.T) {
	projection := NewCallStateProjection()
	roomID := "room-access-generation"
	oldCallID := "call-old"
	newCallID := "call-new"
	oldKeyRef := "key-ref-old"
	newKeyRef := "key-ref-new"

	if err := projection.Apply(newEvent("user1", &corev1.Event{
		Event: &corev1.Event_VoiceCallStarted{
			VoiceCallStarted: &corev1.CallStartedEvent{
				RoomId:     roomID,
				CallId:     oldCallID,
				E2EeKeyRef: oldKeyRef,
			},
		},
	}), 1); err != nil {
		t.Fatalf("Apply old VoiceCallStarted: %v", err)
	}

	keyStore := &hookedCallKeyStore{
		keys: map[string]string{
			oldKeyRef: "key-old",
			newKeyRef: "key-new",
		},
	}
	keyStore.beforeGet = func(keyRef string) error {
		if keyRef != oldKeyRef {
			return fmt.Errorf("GetCallKey key ref = %q, want captured old ref %q", keyRef, oldKeyRef)
		}
		if err := projection.Apply(newEvent("user1", &corev1.Event{
			Event: &corev1.Event_VoiceCallEnded{
				VoiceCallEnded: &corev1.CallEndedEvent{
					RoomId: roomID,
					CallId: oldCallID,
				},
			},
		}), 2); err != nil {
			return err
		}
		if err := projection.Apply(newEvent("user2", &corev1.Event{
			Event: &corev1.Event_VoiceCallStarted{
				VoiceCallStarted: &corev1.CallStartedEvent{
					RoomId:     roomID,
					CallId:     newCallID,
					E2EeKeyRef: newKeyRef,
				},
			},
		}), 3); err != nil {
			return err
		}
		return projection.Apply(newEvent("user2", &corev1.Event{
			Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
					RoomId: roomID,
					CallId: newCallID,
				},
			},
		}), 4)
	}

	model := &CallModel{
		callState: detachedTestProjectionHandle(projection),
		callKeys:  keyStore,
	}
	core := &ChattoCore{callModel: model}
	access, err := core.GetVoiceCallAccessMaterial(context.Background(), roomID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVoiceCallAccessMaterial() = %#v, %v; want call transition error", access, err)
	}

	snapshot := model.roomSnapshot(roomID)
	if snapshot.Call.CallID != newCallID ||
		len(snapshot.Participants) != 1 ||
		snapshot.Participants[0].CallID != newCallID {
		t.Fatalf("roomSnapshot() after injected transition = %#v, want only new call generation", snapshot)
	}

	model.callKeys = nil
	if _, err := model.GetAccessMaterial(context.Background(), roomID); err == nil {
		t.Fatal("GetAccessMaterial() without key store error = nil, want initialization error")
	}
	model.callKeys = &hookedCallKeyStore{keys: map[string]string{}}
	if _, err := model.GetAccessMaterial(context.Background(), roomID); err == nil {
		t.Fatal("GetAccessMaterial() without projected key error = nil, want key read error")
	}
}

func TestCallModel_GetParticipantAccessMaterialRejectsCallGenerationTransition(t *testing.T) {
	projection := NewCallStateProjection()
	roomID := "room-publisher-access-generation"
	userID := "user1"
	oldCallID := "call-old"
	newCallID := "call-new"
	oldKeyRef := "key-ref-old"
	newKeyRef := "key-ref-new"

	if err := projection.Apply(newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallStarted{
			VoiceCallStarted: &corev1.CallStartedEvent{
				RoomId:     roomID,
				CallId:     oldCallID,
				E2EeKeyRef: oldKeyRef,
			},
		},
	}), 1); err != nil {
		t.Fatalf("Apply old VoiceCallStarted: %v", err)
	}
	if err := projection.Apply(newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantJoined{
			VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
				RoomId: roomID,
				CallId: oldCallID,
			},
		},
	}), 2); err != nil {
		t.Fatalf("Apply old VoiceCallParticipantJoined: %v", err)
	}

	keyStore := &hookedCallKeyStore{
		keys: map[string]string{
			oldKeyRef: "key-old",
			newKeyRef: "key-new",
		},
	}
	keyStore.beforeGet = func(keyRef string) error {
		if keyRef != oldKeyRef {
			return fmt.Errorf("GetCallKey key ref = %q, want captured old ref %q", keyRef, oldKeyRef)
		}
		if err := projection.Apply(newEvent(userID, &corev1.Event{
			Event: &corev1.Event_VoiceCallEnded{
				VoiceCallEnded: &corev1.CallEndedEvent{RoomId: roomID, CallId: oldCallID},
			},
		}), 3); err != nil {
			return err
		}
		if err := projection.Apply(newEvent(userID, &corev1.Event{
			Event: &corev1.Event_VoiceCallStarted{
				VoiceCallStarted: &corev1.CallStartedEvent{
					RoomId:     roomID,
					CallId:     newCallID,
					E2EeKeyRef: newKeyRef,
				},
			},
		}), 4); err != nil {
			return err
		}
		return projection.Apply(newEvent(userID, &corev1.Event{
			Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{
					RoomId: roomID,
					CallId: newCallID,
				},
			},
		}), 5)
	}

	model := &CallModel{
		callState: detachedTestProjectionHandle(projection),
		callKeys:  keyStore,
	}
	access, err := model.GetParticipantAccessMaterial(context.Background(), roomID, userID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetParticipantAccessMaterial() = %#v, %v; want call transition error", access, err)
	}

	keyStore.beforeGet = nil
	if _, err := model.GetParticipantAccessMaterial(context.Background(), roomID, "not-participating"); !errors.Is(err, ErrCallParticipationRequired) {
		t.Fatalf("GetParticipantAccessMaterial() non-participant error = %v, want ErrCallParticipationRequired", err)
	}
}

func TestCallModel_WaitForRejectsMissingProjector(t *testing.T) {
	ctx := testContext(t)
	pos := events.SubjectPosition(evtstream.RoomSubjectFilter(), 1)

	var nilModel *CallModel
	if err := nilModel.waitFor(ctx, pos); err == nil {
		t.Fatal("nil CallModel waitFor() error = nil, want initialization error")
	}

	model := &CallModel{}
	if err := model.waitFor(ctx, pos); err == nil {
		t.Fatal("CallModel without projector waitFor() error = nil, want initialization error")
	}
}

func TestWaitForRoomLeaveTail_PropagatesCallProjectionReadinessFailure(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)
	roomID := "room-call-readiness-failure"
	event := newEvent("user1", &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{
				RoomId: roomID,
				Name:   "Call readiness failure",
			},
		},
	})
	subject := evtstream.RoomAggregate(roomID).SubjectFor(event)
	seq, err := harness.publisher.Append(ctx, subject, event)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	directoryProjection := NewRoomDirectoryProjection()
	directoryProjector := harness.projector(directoryProjection)
	startTestProjector(t, directoryProjector)
	core := &ChattoCore{
		roomModel: newTestRoomModel(t,
			directoryProjection,
			directoryProjector,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
		callModel: &CallModel{},
	}

	if err := core.waitForRoomLeaveTail(ctx, subject, seq); err == nil {
		t.Fatal("waitForRoomLeaveTail() error = nil, want call projection readiness error")
	}
}

func TestChattoCore_CallReadsRejectMissingModel(t *testing.T) {
	core := &ChattoCore{}

	if _, err := core.GetCallParticipants("room1"); err == nil {
		t.Fatal("GetCallParticipants() error = nil, want initialization error")
	}
	if _, err := core.GetVoiceCallAccessMaterial(context.Background(), "room1"); err == nil {
		t.Fatal("GetVoiceCallAccessMaterial() error = nil, want initialization error")
	}
	if _, _, err := core.GetActiveCall("room1"); err == nil {
		t.Fatal("GetActiveCall() error = nil, want initialization error")
	}
	if _, err := core.GetCallSnapshot("room1"); err == nil {
		t.Fatal("GetCallSnapshot() error = nil, want initialization error")
	}
	if _, err := core.GetActiveCallRoomIDs(context.Background()); err == nil {
		t.Fatal("GetActiveCallRoomIDs() error = nil, want initialization error")
	}
}

func TestCallState_JoinAndLeave(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	// Initially no participants
	participants, err := core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 0 {
		t.Errorf("Expected 0 participants, got %d", len(participants))
	}

	// Join — appends a durable LiveKit-observed fact
	err = core.HandleCallParticipantJoined(ctx, roomID, "user1")
	if err != nil {
		t.Fatalf("HandleCallParticipantJoined() error = %v", err)
	}
	eventsForRoom, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantJoined))
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(eventsForRoom) != 1 {
		t.Fatalf("Expected 1 durable join fact, got %d", len(eventsForRoom))
	}
	joined := eventsForRoom[0].GetVoiceCallParticipantJoined()
	if joined == nil {
		t.Fatal("Expected voice call joined event")
	}
	if joined.GetSource() != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT {
		t.Fatalf("Source = %v, want LIVEKIT", joined.GetSource())
	}
	if joined.GetCallId() == "" {
		t.Fatal("CallId should be set")
	}

	participants, err = core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant, got %d", len(participants))
	}
	if participants[0].UserID != "user1" {
		t.Errorf("UserID = %q, want %q", participants[0].UserID, "user1")
	}
	if participants[0].CallID != joined.GetCallId() {
		t.Errorf("CallID = %q, want %q", participants[0].CallID, joined.GetCallId())
	}
	if participants[0].Source != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT {
		t.Errorf("Source = %v, want LIVEKIT", participants[0].Source)
	}
	if participants[0].JoinedAt == 0 {
		t.Error("JoinedAt should not be zero")
	}

	// Leave — appends a durable LiveKit-observed fact and removes active participant
	err = core.HandleCallParticipantLeft(ctx, roomID, "user1")
	if err != nil {
		t.Fatalf("HandleCallParticipantLeft() error = %v", err)
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 4 {
		t.Fatalf("Expected 4 durable call lifecycle facts, got %d", len(callEvents))
	}
	if callEvents[0].GetVoiceCallStarted() == nil ||
		callEvents[1].GetVoiceCallParticipantJoined() == nil ||
		callEvents[2].GetVoiceCallParticipantLeft() == nil ||
		callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected start/join/leave/end call events")
	}
	if callEvents[2].GetVoiceCallParticipantLeft() == nil {
		t.Fatal("Expected voice call left event")
	}

	participants, err = core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 0 {
		t.Errorf("Expected 0 participants after leave, got %d", len(participants))
	}
}

func TestCallState_JoinIdempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	// Join twice while still active: one participant and one durable transition.
	_ = core.HandleCallParticipantJoined(ctx, roomID, "user1")
	_ = core.HandleCallParticipantJoined(ctx, roomID, "user1")

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Errorf("Expected 1 participant (idempotent), got %d", len(participants))
	}
	eventsForRoom, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantJoined))
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(eventsForRoom) != 1 {
		t.Errorf("Expected 1 durable join fact for the active transition, got %d", len(eventsForRoom))
	}
}

func TestCallState_SnapshotTracksRoomAggregateSeq(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	roomEvent := newEvent("admin1", &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: roomID, Name: "Room One"},
		},
	})
	seq, err := core.EventPublisher.AppendEventuallyAndWait(ctx, core.callModel.callState.Projector(), evtstream.RoomAggregate(roomID), roomEvent)
	if err != nil {
		t.Fatalf("append room event() error = %v", err)
	}

	snapshot := core.callModel.roomSnapshot(roomID)
	if snapshot.Seq != seq {
		t.Fatalf("RoomSnapshot().Seq = %d, want %d", snapshot.Seq, seq)
	}
	if len(snapshot.Participants) != 0 {
		t.Fatalf("RoomSnapshot().Participants = %d, want 0", len(snapshot.Participants))
	}
}

func TestCallState_SnapshotIgnoresAssetAggregateLifecycleSeq(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"
	assetID := "asset1"

	roomEvent := newEvent("admin1", &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: roomID, Name: "Room One"},
		},
	})
	roomSeq, err := core.EventPublisher.AppendEventuallyAndWait(ctx, core.callModel.callState.Projector(), evtstream.RoomAggregate(roomID), roomEvent)
	if err != nil {
		t.Fatalf("append room event() error = %v", err)
	}

	assetEvent := newEvent("user1", &corev1.Event{Event: &corev1.Event_AssetCreated{
		AssetCreated: &corev1.AssetCreatedEvent{
			RoomId: roomID,
			Asset:  &corev1.AssetRecord{Id: assetID},
		},
	}})
	assetSubject := evtstream.AssetAggregate(assetID).SubjectFor(assetEvent)
	assetSeq, err := core.EventPublisher.AppendEventually(ctx, assetSubject, assetEvent)
	if err != nil {
		t.Fatalf("append asset event error = %v", err)
	}
	if err := core.assetModel.assets.Projector().WaitFor(ctx, events.SubjectPosition(assetSubject, assetSeq)); err != nil {
		t.Fatalf("wait for asset event error = %v", err)
	}

	snapshot := core.callModel.roomSnapshot(roomID)
	if snapshot.Seq != roomSeq {
		t.Fatalf("RoomSnapshot().Seq = %d, want room seq %d", snapshot.Seq, roomSeq)
	}
	if err := core.HandleCallParticipantJoined(ctx, roomID, "user1"); err != nil {
		t.Fatalf("HandleCallParticipantJoined() after asset aggregate event error = %v", err)
	}
}

func TestCallState_LeaveNotInCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Leave when not in a call is a no-op, not a duplicate transition fact.
	roomID := "room1"
	err := core.HandleCallParticipantLeft(ctx, roomID, "user1")
	if err != nil {
		t.Fatalf("HandleCallParticipantLeft() for absent user should not error, got %v", err)
	}
	eventsForRoom, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantLeft))
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(eventsForRoom) != 0 {
		t.Fatalf("Expected no durable leave fact for absent user, got %d", len(eventsForRoom))
	}
}

func TestCallState_UserAndLiveKitReportsDoNotDuplicateTransitions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	if err := core.HandleCallParticipantJoined(ctx, roomID, "user1"); err != nil {
		t.Fatalf("HandleCallParticipantJoined() error = %v", err)
	}
	joins, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantJoined))
	if err != nil {
		t.Fatalf("SubjectEvents(joined) error = %v", err)
	}
	if len(joins) != 1 {
		t.Fatalf("Expected USER+LIVEKIT join reports to produce 1 durable transition fact, got %d", len(joins))
	}

	if err := core.RecordCallParticipantLeft(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantLeft() error = %v", err)
	}
	if err := core.HandleCallParticipantLeft(ctx, roomID, "user1"); err != nil {
		t.Fatalf("HandleCallParticipantLeft() error = %v", err)
	}
	leaves, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantLeft))
	if err != nil {
		t.Fatalf("SubjectEvents(left) error = %v", err)
	}
	if len(leaves) != 1 {
		t.Fatalf("Expected USER+LIVEKIT leave reports to produce 1 durable transition fact, got %d", len(leaves))
	}
}

func TestCallState_RejoinAfterLeaveRecordsNewTransitions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	for i := 0; i < 2; i++ {
		if err := core.HandleCallParticipantJoined(ctx, roomID, "user1"); err != nil {
			t.Fatalf("HandleCallParticipantJoined(%d) error = %v", i, err)
		}
		if err := core.HandleCallParticipantLeft(ctx, roomID, "user1"); err != nil {
			t.Fatalf("HandleCallParticipantLeft(%d) error = %v", i, err)
		}
	}

	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 8 {
		t.Fatalf("Expected two calls to produce 8 durable lifecycle facts, got %d", len(callEvents))
	}
	if callEvents[0].GetVoiceCallStarted() == nil ||
		callEvents[1].GetVoiceCallParticipantJoined() == nil ||
		callEvents[2].GetVoiceCallParticipantLeft() == nil ||
		callEvents[3].GetVoiceCallEnded() == nil ||
		callEvents[4].GetVoiceCallStarted() == nil ||
		callEvents[5].GetVoiceCallParticipantJoined() == nil ||
		callEvents[6].GetVoiceCallParticipantLeft() == nil ||
		callEvents[7].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected start/join/leave/end for each call")
	}
	firstCallID := callEvents[0].GetVoiceCallStarted().GetCallId()
	secondCallID := callEvents[4].GetVoiceCallStarted().GetCallId()
	if firstCallID == "" || secondCallID == "" || firstCallID == secondCallID {
		t.Fatalf("Call IDs should be non-empty and different, got %q and %q", firstCallID, secondCallID)
	}
}

func TestCallState_MultipleParticipants(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	_ = core.HandleCallParticipantJoined(ctx, roomID, "user1")
	_ = core.HandleCallParticipantJoined(ctx, roomID, "user2")

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 2 {
		t.Fatalf("Expected 2 participants, got %d", len(participants))
	}

	// Remove one — other should remain
	_ = core.HandleCallParticipantLeft(ctx, roomID, "user1")
	participants, _ = core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant after leave, got %d", len(participants))
	}
	if participants[0].UserID != "user2" {
		t.Errorf("Remaining participant should be user2, got %q", participants[0].UserID)
	}
}

func TestCallState_RoomFinished(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	_ = core.HandleCallParticipantJoined(ctx, roomID, "user1")
	_ = core.HandleCallParticipantJoined(ctx, roomID, "user2")

	err := core.HandleCallRoomFinished(ctx, roomID)
	if err != nil {
		t.Fatalf("HandleCallRoomFinished() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Errorf("Expected 0 participants after room finished, got %d", len(participants))
	}
}

func TestCallState_StaleLiveKitEventsForOldCallAreIgnored(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined(first) error = %v", err)
	}
	firstCallID := activeCallIDForTest(t, core, roomID)
	if err := core.RecordCallParticipantLeft(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantLeft(first) error = %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined(second) error = %v", err)
	}
	secondCallID := activeCallIDForTest(t, core, roomID)
	if firstCallID == secondCallID {
		t.Fatalf("Expected distinct call IDs, got %q", firstCallID)
	}

	if err := core.HandleCallParticipantJoined(ctx, roomID, "user2", firstCallID); err != nil {
		t.Fatalf("stale HandleCallParticipantJoined() error = %v", err)
	}
	if err := core.HandleCallParticipantLeft(ctx, roomID, "user1", firstCallID); err != nil {
		t.Fatalf("stale HandleCallParticipantLeft() error = %v", err)
	}
	if err := core.HandleCallRoomFinished(ctx, roomID, firstCallID); err != nil {
		t.Fatalf("stale HandleCallRoomFinished() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 || participants[0].UserID != "user1" || participants[0].CallID != secondCallID {
		t.Fatalf("Expected only user1 in second call, got %#v", participants)
	}
	active, ok := core.callModel.activeCall(roomID)
	if !ok || active.CallID != secondCallID {
		t.Fatalf("Expected active second call %q, got %#v ok=%v", secondCallID, active, ok)
	}
}

func TestGetActiveCallRoomIDs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// No active calls initially
	ids, err := core.GetActiveCallRoomIDs(ctx)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Expected 0 room IDs, got %d", len(ids))
	}

	// Add calls in both channel and DM rooms.
	_ = core.HandleCallParticipantJoined(ctx, "channel-room", "user1")
	_ = core.HandleCallParticipantJoined(ctx, "dm-room", "user2")

	ids, err = core.GetActiveCallRoomIDs(ctx)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs after joins: %v", err)
	}
	if want := []string{"channel-room", "dm-room"}; !slices.Equal(ids, want) {
		t.Fatalf("active room IDs = %v, want %v", ids, want)
	}

	// Remove the channel participant; the DM call remains visible.
	_ = core.HandleCallParticipantLeft(ctx, "channel-room", "user1")
	ids, err = core.GetActiveCallRoomIDs(ctx)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs after leave: %v", err)
	}
	if want := []string{"dm-room"}; !slices.Equal(ids, want) {
		t.Fatalf("active room IDs after leave = %v, want %v", ids, want)
	}
}

func TestCallState_UserIntentFacts(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	participants, err := core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant, got %d", len(participants))
	}
	if participants[0].Source != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER {
		t.Fatalf("Source = %v, want USER", participants[0].Source)
	}

	if err := core.RecordCallParticipantLeft(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantLeft() error = %v", err)
	}
	participants, _ = core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants after explicit leave, got %d", len(participants))
	}
}

func TestCallState_UserLeftRoomEventRemovesParticipantOnReplay(t *testing.T) {
	projection := NewCallStateProjection()
	roomID := "room-replay-leave"
	userID := "user-replay-leave"

	if err := projection.Apply(&corev1.Event{
		ActorId: userID,
		Event: &corev1.Event_VoiceCallStarted{
			VoiceCallStarted: &corev1.CallStartedEvent{RoomId: roomID, CallId: "call-replay"},
		},
	}, 1); err != nil {
		t.Fatalf("Apply CallStarted: %v", err)
	}
	if err := projection.Apply(&corev1.Event{
		ActorId: userID,
		Event: &corev1.Event_VoiceCallParticipantJoined{
			VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{RoomId: roomID, CallId: "call-replay"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply CallParticipantJoined: %v", err)
	}
	if err := projection.Apply(&corev1.Event{
		ActorId: userID,
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: roomID},
		},
	}, 3); err != nil {
		t.Fatalf("Apply UserLeftRoom: %v", err)
	}

	if participants := projection.Participants(roomID); len(participants) != 0 {
		t.Fatalf("participants after UserLeftRoom replay = %#v, want none", participants)
	}
	if _, ok := projection.ActiveCall(roomID); ok {
		t.Fatal("active call still exists after final participant UserLeftRoom replay")
	}
}

func TestCallState_RoomLeaveRemovesFinalCallParticipant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "call-room-leaver", "Call Room Leaver", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-room-leave", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	active := activeCallIDForTest(t, core, room.Id)
	session, _ := core.callModel.activeCall(room.Id)
	keyRef := session.E2EEKeyRef
	recorder := &recordingLiveKitParticipantClient{}
	core.callModel.livekit = recorder

	if err := core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	participants, err := core.GetCallParticipants(room.Id)
	if err != nil {
		t.Fatalf("GetCallParticipants: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants after room leave = %#v, want none", participants)
	}
	if _, ok := core.callModel.activeCall(room.Id); ok {
		t.Fatal("active call still exists after final room-leaving participant")
	}
	exists, err := core.encryption.callKeys.CallKeyExists(ctx, keyRef)
	if err != nil {
		t.Fatalf("CallKeyExists: %v", err)
	}
	if exists {
		t.Fatal("call key still exists after final room-leaving participant")
	}
	if len(recorder.removed) != 1 || recorder.removed[0].roomID != room.Id || recorder.removed[0].callID != active || recorder.removed[0].userID != user.Id {
		t.Fatalf("LiveKit removals = %#v, want one removal for user call", recorder.removed)
	}

	events, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	var sawCallLeft, sawCallEnded bool
	for _, event := range events {
		if left := event.GetVoiceCallParticipantLeft(); left != nil {
			sawCallLeft = left.GetSource() == corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER
		}
		if ended := event.GetVoiceCallEnded(); ended != nil {
			sawCallEnded = ended.GetSource() == corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER
		}
	}
	if !sawCallLeft || !sawCallEnded {
		t.Fatalf("room leave call events source USER: left=%v ended=%v", sawCallLeft, sawCallEnded)
	}
}

func TestCallState_RemoveMemberRemovesOnlyTargetFromCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	manager, err := core.CreateUser(ctx, SystemActorID, "call-kick-manager", "Call Kick Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "call-kick-target", "Call Kick Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	other, err := core.CreateUser(ctx, SystemActorID, "call-kick-other", "Call Kick Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-kick-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, user := range []*corev1.User{target, other} {
		if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", user.Id, err)
		}
		if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
			t.Fatalf("RecordCallParticipantJoined %s: %v", user.Id, err)
		}
	}
	callID := activeCallIDForTest(t, core, room.Id)
	recorder := &recordingLiveKitParticipantClient{}
	core.callModel.livekit = recorder

	removed, err := core.RemoveMember(ctx, manager.Id, KindChannel, room.Id, target.Id)
	if err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if !removed {
		t.Fatal("RemoveMember removed=false, want true")
	}

	participants, _ := core.GetCallParticipants(room.Id)
	if len(participants) != 1 || participants[0].UserID != other.Id {
		t.Fatalf("participants after RemoveMember = %#v, want only other", participants)
	}
	active, ok := core.callModel.activeCall(room.Id)
	if !ok || active.CallID != callID {
		t.Fatalf("active call after RemoveMember = %#v ok=%v, want same call %s", active, ok, callID)
	}
	if len(recorder.removed) != 1 || recorder.removed[0].userID != target.Id {
		t.Fatalf("LiveKit removals = %#v, want target removal", recorder.removed)
	}
}

func TestCallState_BanMemberRemovesTargetFromCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	moderator, err := core.CreateUser(ctx, SystemActorID, "call-ban-moderator", "Call Ban Moderator", "password")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "call-ban-target", "Call Ban Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-ban-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, target.Id, KindChannel, target.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom target: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, target.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	recorder := &recordingLiveKitParticipantClient{}
	core.callModel.livekit = recorder

	if _, err := core.BanMember(ctx, moderator.Id, KindChannel, room.Id, target.Id, "moderation test", nil); err != nil {
		t.Fatalf("BanMember: %v", err)
	}

	participants, _ := core.GetCallParticipants(room.Id)
	if len(participants) != 0 {
		t.Fatalf("participants after BanMember = %#v, want none", participants)
	}
	if _, ok := core.callModel.activeCall(room.Id); ok {
		t.Fatal("active call still exists after banned final participant")
	}
	if len(recorder.removed) != 1 || recorder.removed[0].userID != target.Id {
		t.Fatalf("LiveKit removals = %#v, want target removal", recorder.removed)
	}
}

func TestCallState_DeleteUserRemovesUserFromRoomCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "call-delete-user", "Call Delete User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-delete-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	recorder := &recordingLiveKitParticipantClient{}
	core.callModel.livekit = recorder

	if err := core.DeleteUser(ctx, user.Id, user.Id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	participants, _ := core.GetCallParticipants(room.Id)
	if len(participants) != 0 {
		t.Fatalf("participants after DeleteUser = %#v, want none", participants)
	}
	if len(recorder.removed) != 1 || recorder.removed[0].userID != user.Id {
		t.Fatalf("LiveKit removals = %#v, want deleted user removal", recorder.removed)
	}
}

func TestCallState_LiveKitRemovalFailureDoesNotFailRoomLeave(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "call-livekit-fail-user", "Call LiveKit Fail User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-livekit-fail-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	core.callModel.livekit = &recordingLiveKitParticipantClient{removeErr: errors.New("livekit unavailable")}

	if err := core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom with LiveKit removal failure: %v", err)
	}
	if isMember, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id); err != nil || isMember {
		t.Fatalf("RoomMembershipExists after LiveKit removal failure = %v, %v; want false, nil", isMember, err)
	}
}

func TestCallState_RoomLeaveQueuesCommittedKeyCleanupAfterImmediateFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "call-shred-retry-user", "Call Shred Retry User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-shred-retry-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	session, _ := core.callModel.activeCall(room.Id)
	recorder := &recordingLiveKitParticipantClient{}
	core.callModel.livekit = recorder
	workingKeys := core.encryption.callKeys
	shredErr := errors.New("kms shred unavailable")
	core.callModel.callKeys = failingShredCallKeyStore{delegate: workingKeys, err: shredErr}

	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if !errors.Is(err, shredErr) {
		t.Fatalf("LeaveRoom error = %v, want shred error", err)
	}
	if len(recorder.removed) != 1 || recorder.removed[0].userID != user.Id {
		t.Fatalf("LiveKit removals = %#v, want cleanup despite shred failure", recorder.removed)
	}
	if isMember, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id); err != nil || isMember {
		t.Fatalf("RoomMembershipExists = %v, %v; want false, nil", isMember, err)
	}

	core.callModel.callKeys = workingKeys
	info, err := core.callModel.keyCleanupConsumer.Info(ctx)
	if err != nil {
		t.Fatalf("call-key cleanup consumer: %v", err)
	}
	if info.NumPending+uint64(info.NumAckPending) == 0 {
		t.Fatalf("call-key cleanup consumer = %+v; want durable retry work", info)
	}
	if exists, err := workingKeys.CallKeyExists(ctx, session.E2EEKeyRef); err != nil || !exists {
		t.Fatalf("CallKeyExists before durable retry = %v, %v; want true, nil", exists, err)
	}
}

func TestCallState_DurableWorkerDiscoversLaterReplicaKeyCleanup(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	core, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	workingKeys := core.encryption.callKeys
	const callID = "C-later-replica"
	keyRef, _, err := workingKeys.CreateCallKey(ctx, callID)
	if err != nil {
		t.Fatalf("CreateCallKey: %v", err)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_VoiceCallEnded{VoiceCallEnded: &corev1.CallEndedEvent{
		RoomId: "R-later-replica", CallId: callID,
	}}})
	if _, err := core.EventPublisher.AppendEventually(ctx, evtstream.RoomAggregate("R-later-replica").Subject(evtstream.EventCallEnded), event); err != nil {
		t.Fatalf("append call-ended fact: %v", err)
	}
	if exists, err := workingKeys.CallKeyExists(ctx, keyRef); err != nil || !exists {
		t.Fatalf("CallKeyExists after replica failure = %v, %v; want true, nil", exists, err)
	}
	info, err := core.callModel.keyCleanupConsumer.Info(ctx)
	if err != nil || info.NumPending == 0 {
		t.Fatalf("call-key cleanup queue before worker = %+v, %v; want pending work", info, err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- core.callModel.Run(workerCtx) }()
	waitForRecoveryTest(t, 5*time.Second, func() bool {
		exists, err := workingKeys.CallKeyExists(ctx, keyRef)
		return err == nil && !exists
	}, "durable call-key cleanup")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("holder cleanup shutdown: %v", err)
	}
}

func TestCallState_ReconciliationCleansHistoricalMembershipOnlyLeave(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "call-legacy-leave-user", "Call Legacy Leave User", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "call-legacy-leave-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := core.RecordCallParticipantJoined(ctx, room.Id, user.Id, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}
	session, _ := core.callModel.activeCall(room.Id)

	legacyLeave := newEvent(user.Id, &corev1.Event{Event: &corev1.Event_UserLeftRoom{
		UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: room.Id},
	}})
	seq, err := core.EventPublisher.AppendEventually(ctx, evtstream.RoomAggregate(room.Id).SubjectFor(legacyLeave), legacyLeave)
	if err != nil {
		t.Fatalf("AppendEventually legacy leave: %v", err)
	}
	if err := core.callModel.waitFor(ctx, events.SubjectPosition(evtstream.RoomAggregate(room.Id).AllEventsFilter(), seq)); err != nil {
		t.Fatalf("WaitFor CallState: %v", err)
	}
	if _, ok := core.callModel.activeCall(room.Id); ok {
		t.Fatal("historical membership leave did not clear projected call")
	}

	recorder := &recordingLiveKitParticipantClient{snapshots: []liveKitParticipantSnapshot{{
		LegacySpaceID: LegacySpaceIDForRoomKind(KindChannel),
		RoomID:        room.Id,
		CallID:        session.CallID,
		UserIDs:       []string{user.Id},
	}}}
	core.callModel.livekit = recorder
	workingKeys := core.encryption.callKeys
	shredErr := errors.New("kms shred unavailable")
	core.callModel.callKeys = failingShredCallKeyStore{delegate: workingKeys, err: shredErr}
	if err := core.callModel.reconcileWithLiveKit(ctx, func() (context.Context, context.CancelFunc) {
		return context.WithCancel(ctx)
	}); !errors.Is(err, shredErr) {
		t.Fatalf("reconcileWithLiveKit error = %v, want shred error", err)
	}
	if len(recorder.removed) != 1 || recorder.removed[0].spaceID != LegacySpaceIDForRoomKind(KindChannel) || recorder.removed[0].userID != user.Id {
		t.Fatalf("LiveKit removals = %#v, want historical participant eviction", recorder.removed)
	}
	if exists, err := workingKeys.CallKeyExists(ctx, session.E2EEKeyRef); err != nil || !exists {
		t.Fatalf("CallKeyExists after failed cleanup = %v, %v; want true, nil", exists, err)
	}
	ended, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventCallEnded))
	if err != nil {
		t.Fatalf("SubjectEvents call ended: %v", err)
	}
	if len(ended) == 0 || ended[len(ended)-1].GetVoiceCallEnded().GetCallId() != session.CallID {
		t.Fatalf("durable call-ended recovery facts = %#v, want call %s", ended, session.CallID)
	}

	core.callModel.callKeys = workingKeys
	info, err := core.callModel.keyCleanupConsumer.Info(ctx)
	if err != nil {
		t.Fatalf("call-key cleanup consumer: %v", err)
	}
	if info.NumPending+uint64(info.NumAckPending) == 0 {
		t.Fatalf("call-key cleanup consumer = %+v; want durable retry work", info)
	}
}

func TestCallState_ReconciliationCorrectsProjection(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.callModel.ReconcileRoomParticipants(ctx, roomID, []string{"user1"}); err != nil {
		t.Fatalf("ReconcileRoomParticipants(join) error = %v", err)
	}
	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant after reconcile join, got %d", len(participants))
	}
	if participants[0].Source != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION {
		t.Fatalf("Source = %v, want RECONCILIATION", participants[0].Source)
	}

	if err := core.callModel.ReconcileRoomParticipants(ctx, roomID, nil); err != nil {
		t.Fatalf("ReconcileRoomParticipants(leave) error = %v", err)
	}
	participants, _ = core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected 0 participants after reconcile leave, got %d", len(participants))
	}
}

func TestCallState_ReconcileWithLiveKitClosesRoomMissingFromLiveKit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	started := callEvents[0].GetVoiceCallStarted()
	if started == nil || started.GetE2EeKeyRef() == "" {
		t.Fatalf("Expected started event with key ref")
	}

	core.callModel.livekit = fakeLiveKitParticipantLister{}
	if err := core.callModel.ReconcileWithLiveKit(ctx); err != nil {
		t.Fatalf("ReconcileWithLiveKit() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected missing LiveKit room to clear participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected missing LiveKit room to end active call")
	}
	callEvents, _, err = core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() after reconcile error = %v", err)
	}
	if len(callEvents) != 4 ||
		callEvents[2].GetVoiceCallParticipantLeft() == nil ||
		callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected start/join/left/end after missing-room reconcile, got %d events", len(callEvents))
	}
	if got := callEvents[3].GetVoiceCallEnded().GetSource(); got != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION {
		t.Fatalf("Ended source = %v, want RECONCILIATION", got)
	}
	exists, err := core.encryption.callKeys.CallKeyExists(ctx, started.GetE2EeKeyRef())
	if err != nil {
		t.Fatalf("CallKeyExists() error = %v", err)
	}
	if exists {
		t.Fatal("Expected missing-room reconcile to shred ended call key")
	}
}

func TestCallState_ReconcileWithLiveKitClosesObservedEmptyRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callID := activeCallIDForTest(t, core, roomID)
	core.callModel.livekit = fakeLiveKitParticipantLister{
		snapshots: []liveKitParticipantSnapshot{{RoomID: roomID, CallID: callID}},
	}
	if err := core.callModel.ReconcileWithLiveKit(ctx); err != nil {
		t.Fatalf("ReconcileWithLiveKit() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected observed empty LiveKit room to clear participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected observed empty LiveKit room to end active call")
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 4 || callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected observed empty room to append CallEndedEvent, got %d events", len(callEvents))
	}
}

func TestCallState_ReconcileWithLiveKitIgnoresRoomWithoutProjectedActiveCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	core.callModel.livekit = fakeLiveKitParticipantLister{
		snapshots: []liveKitParticipantSnapshot{{RoomID: roomID, CallID: "old-call", UserIDs: []string{"user1"}}},
	}
	if err := core.callModel.ReconcileWithLiveKit(ctx); err != nil {
		t.Fatalf("ReconcileWithLiveKit() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected LiveKit room without projected active call to be ignored, got %#v", participants)
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected no active call to be created from LiveKit snapshot")
	}
}

func TestLiveKitRoomClientListCallParticipantsTreatsRoomNotFoundAsEmpty(t *testing.T) {
	room1 := LiveKitRoomName("", KindChannel, "room1", "C1")
	room2 := LiveKitRoomName("", KindChannel, "room2", "C2")
	client := &liveKitRoomClient{
		service: &fakeLiveKitRoomService{
			rooms:        []string{room1, room2},
			participants: map[string][]string{room2: []string{"user2"}},
			participantErrs: map[string]error{
				room1: twirp.NotFoundError("room not found"),
			},
		},
		apiKey:    "key",
		apiSecret: "secret",
	}

	snapshots, err := client.ListCallParticipants(testContext(t))
	if err != nil {
		t.Fatalf("ListCallParticipants() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("Expected two snapshots, got %#v", snapshots)
	}
	if snapshots[0].RoomID != "room1" || snapshots[0].CallID != "C1" || len(snapshots[0].UserIDs) != 0 {
		t.Fatalf("First snapshot = %#v, want empty room1 C1 snapshot", snapshots[0])
	}
	if snapshots[1].RoomID != "room2" || snapshots[1].CallID != "C2" || len(snapshots[1].UserIDs) != 1 || snapshots[1].UserIDs[0] != "user2" {
		t.Fatalf("Second snapshot = %#v, want room2 C2 user2 snapshot", snapshots[1])
	}
}

func TestLiveKitRoomClientExcludesCompanionPublishersFromCallMembership(t *testing.T) {
	room := LiveKitRoomName("", KindChannel, "room1", "C1")
	client := &liveKitRoomClient{
		service: &fakeLiveKitRoomService{
			rooms:        []string{room},
			participants: map[string][]string{room: {"user1", "publisher1"}},
			metadata: map[string]string{
				"publisher1": `{"publisherKind":"game_share","ownerIdentity":"user1"}`,
			},
		},
		apiKey:    "key",
		apiSecret: "secret",
	}

	snapshots, err := client.ListCallParticipants(testContext(t))
	if err != nil {
		t.Fatalf("ListCallParticipants() error = %v", err)
	}
	if len(snapshots) != 1 || !slices.Equal(snapshots[0].UserIDs, []string{"user1"}) {
		t.Fatalf("UserIDs = %#v, want only user1", snapshots)
	}
	if !slices.Equal(snapshots[0].ParticipantIdentities, []string{"publisher1", "user1"}) {
		t.Fatalf("ParticipantIdentities = %#v, want publisher and user", snapshots[0].ParticipantIdentities)
	}
}

func TestCallState_ReconcileWithLiveKitErrorDefersActiveCallCleanupBeforeThreshold(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	started := callEvents[0].GetVoiceCallStarted()
	if started == nil || started.GetE2EeKeyRef() == "" {
		t.Fatalf("Expected started event with key ref")
	}

	liveKitErr := errors.New("livekit unavailable")
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	for i := 1; i < callReconcileListFailureThreshold; i++ {
		err = core.callModel.ReconcileWithLiveKit(ctx)
		if !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(%d) error = %v, want %v", i, err, liveKitErr)
		}
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected failed LiveKit reconciliation below threshold to keep participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); !ok {
		t.Fatal("Expected failed LiveKit reconciliation below threshold to keep active call")
	}
	callEvents, _, err = core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 2 {
		t.Fatalf("Expected failed LiveKit reconciliation below threshold to append no leave/end facts, got %d events", len(callEvents))
	}
}

func TestCallState_ReconcileWithLiveKitErrorEndsActiveCallsAtThreshold(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	started := callEvents[0].GetVoiceCallStarted()
	if started == nil || started.GetE2EeKeyRef() == "" {
		t.Fatalf("Expected started event with key ref")
	}

	liveKitErr := errors.New("livekit unavailable")
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	for i := 1; i <= callReconcileListFailureThreshold; i++ {
		err = core.callModel.ReconcileWithLiveKit(ctx)
		if !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(%d) error = %v, want %v", i, err, liveKitErr)
		}
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected failed LiveKit reconciliation at threshold to clear participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected failed LiveKit reconciliation at threshold to end active call")
	}
	callEvents, _, err = core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 4 ||
		callEvents[2].GetVoiceCallParticipantLeft() == nil ||
		callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected failed LiveKit reconciliation to append start/join/left/end, got %d events", len(callEvents))
	}
	if got := callEvents[3].GetVoiceCallEnded().GetSource(); got != corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION {
		t.Fatalf("Ended source = %v, want RECONCILIATION", got)
	}
	exists, err := core.encryption.callKeys.CallKeyExists(ctx, started.GetE2EeKeyRef())
	if err != nil {
		t.Fatalf("CallKeyExists() error = %v", err)
	}
	if exists {
		t.Fatal("Expected failed LiveKit reconciliation to shred ended call key")
	}
}

func TestCallState_ReconcileWithLiveKitErrorEndsAllActiveRoomsAtThreshold(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	rooms := []struct {
		roomID string
		userID string
	}{
		{roomID: "room1", userID: "user1"},
		{roomID: "room2", userID: "user2"},
	}

	for _, room := range rooms {
		if err := core.RecordCallParticipantJoined(ctx, room.roomID, room.userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
			t.Fatalf("RecordCallParticipantJoined(%s) error = %v", room.roomID, err)
		}
	}

	liveKitErr := errors.New("livekit unavailable")
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	var err error
	for i := 1; i <= callReconcileListFailureThreshold; i++ {
		err = core.callModel.ReconcileWithLiveKit(ctx)
		if !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(%d) error = %v, want %v", i, err, liveKitErr)
		}
	}

	for _, room := range rooms {
		participants, _ := core.GetCallParticipants(room.roomID)
		if len(participants) != 0 {
			t.Fatalf("Expected room %s participants to clear, got %d", room.roomID, len(participants))
		}
		if _, ok := core.callModel.activeCall(room.roomID); ok {
			t.Fatalf("Expected room %s active call to end", room.roomID)
		}
		callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.roomID).AllEventsFilter())
		if err != nil {
			t.Fatalf("SubjectEvents(%s) error = %v", room.roomID, err)
		}
		if len(callEvents) != 4 || callEvents[3].GetVoiceCallEnded() == nil {
			t.Fatalf("Expected room %s to append CallEndedEvent, got %d events", room.roomID, len(callEvents))
		}
	}
}

func TestCallState_ReconcileWithLiveKitTimeoutUsesFreshCleanupContext(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}

	listCtx, cancelList := context.WithCancel(ctx)
	cancelList()
	cleanupContextCreated := false
	core.callModel.livekit = fakeLiveKitParticipantLister{err: context.DeadlineExceeded}

	var err error
	for i := 1; i <= callReconcileListFailureThreshold; i++ {
		err = core.callModel.reconcileWithLiveKit(listCtx, func() (context.Context, context.CancelFunc) {
			cleanupContextCreated = true
			return context.WithCancel(ctx)
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("reconcileWithLiveKit(%d) error = %v, want context deadline exceeded", i, err)
		}
	}
	if !cleanupContextCreated {
		t.Fatal("Expected reconciliation failure to create a cleanup context")
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 0 {
		t.Fatalf("Expected cleanup with fresh context to clear participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected cleanup with fresh context to end active call")
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 4 || callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected cleanup with fresh context to append CallEndedEvent, got %d events", len(callEvents))
	}
}

func TestCallState_ReconcileBestEffortLogsDeferredLiveKitFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"
	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}

	liveKitErr := errors.New("livekit unavailable")
	logger := &recordingCallLogger{}
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	core.callModel.reconcileLease = nil
	core.callModel.logger = logger

	core.callModel.reconcileBestEffort(ctx)

	if logger.warnMessage != "LiveKit listing failed; active-call cleanup deferred" {
		t.Fatalf("Warn message = %q", logger.warnMessage)
	}
	if got := loggedValue(logger.warnKeyvals, "error"); !errors.Is(got.(error), liveKitErr) {
		t.Fatalf("Logged error = %v, want %v", got, liveKitErr)
	}
	if got := loggedValue(logger.warnKeyvals, "consecutive_failures"); got != 1 {
		t.Fatalf("Logged consecutive_failures = %v, want 1", got)
	}
	if got := loggedValue(logger.warnKeyvals, "threshold"); got != callReconcileListFailureThreshold {
		t.Fatalf("Logged threshold = %v, want %d", got, callReconcileListFailureThreshold)
	}
}

func TestCallState_ReconcileBestEffortKeepsLeaseOnDeferredLiveKitFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"
	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}

	liveKitErr := errors.New("livekit unavailable")
	logger := &recordingCallLogger{}
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	core.callModel.reconcileLease = &lease.Lease{}
	core.callModel.logger = logger

	err := core.callModel.reconcileBestEffort(ctx)
	if err != nil {
		t.Fatalf("reconcileBestEffort() error = %v", err)
	}
	if logger.warnMessage != "LiveKit listing failed; active-call cleanup deferred" {
		t.Fatalf("Warn message = %q", logger.warnMessage)
	}
	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected deferred reconciliation below threshold to keep participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); !ok {
		t.Fatal("Expected deferred reconciliation below threshold to keep active call")
	}
}

func TestCallState_ReconcileBestEffortLogsLiveKitFailureCleanupSummaryAtThreshold(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	rooms := []struct {
		roomID string
		userID string
	}{
		{roomID: "room1", userID: "user1"},
		{roomID: "room2", userID: "user2"},
	}
	for _, room := range rooms {
		if err := core.RecordCallParticipantJoined(ctx, room.roomID, room.userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
			t.Fatalf("RecordCallParticipantJoined(%s) error = %v", room.roomID, err)
		}
	}

	liveKitErr := errors.New("livekit unavailable")
	logger := &recordingCallLogger{}
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	core.callModel.logger = logger

	for i := 1; i <= callReconcileListFailureThreshold; i++ {
		core.callModel.reconcileBestEffort(ctx)
	}

	if logger.warnMessage != "LiveKit listing failed; threshold reached and ended projected active calls" {
		t.Fatalf("Warn message = %q", logger.warnMessage)
	}
	if got := loggedValue(logger.warnKeyvals, "error"); !errors.Is(got.(error), liveKitErr) {
		t.Fatalf("Logged error = %v, want %v", got, liveKitErr)
	}
	if got := loggedValue(logger.warnKeyvals, "consecutive_failures"); got != callReconcileListFailureThreshold {
		t.Fatalf("Logged consecutive_failures = %v, want %d", got, callReconcileListFailureThreshold)
	}
	if got := loggedValue(logger.warnKeyvals, "threshold"); got != callReconcileListFailureThreshold {
		t.Fatalf("Logged threshold = %v, want %d", got, callReconcileListFailureThreshold)
	}
	if got := loggedValue(logger.warnKeyvals, "active_rooms"); got != 2 {
		t.Fatalf("Logged active_rooms = %v, want 2", got)
	}
	if got := loggedValue(logger.warnKeyvals, "ended_rooms"); got != 2 {
		t.Fatalf("Logged ended_rooms = %v, want 2", got)
	}
	if got := loggedValue(logger.warnKeyvals, "cleanup_errors"); got != 0 {
		t.Fatalf("Logged cleanup_errors = %v, want 0", got)
	}
}

func TestCallState_ReconcileWithLiveKitErrorReportsCleanupFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	liveKitErr := errors.New("livekit unavailable")
	shredErr := errors.New("kms shred unavailable")
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	core.callModel.callKeys = failingShredCallKeyStore{
		delegate: core.encryption.callKeys,
		err:      shredErr,
	}

	var err error
	for i := 1; i <= callReconcileListFailureThreshold; i++ {
		err = core.callModel.ReconcileWithLiveKit(ctx)
		if !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(%d) error = %v, want LiveKit error", i, err)
		}
	}
	if !errors.Is(err, shredErr) {
		t.Fatalf("ReconcileWithLiveKit() error = %v, want cleanup error", err)
	}

	callEvents, seq, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 4 || callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected cleanup failure to still append CallEndedEvent, got %d events", len(callEvents))
	}
	if err := core.callModel.waitFor(ctx, events.SubjectPosition(evtstream.RoomAggregate(roomID).AllEventsFilter(), seq)); err != nil {
		t.Fatalf("CallStateProjector.WaitFor() error = %v", err)
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected active call to clear even when key shredding fails")
	}
}

func TestCallState_ReconcileWithLiveKitSuccessResetsListFailureCounter(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callID := activeCallIDForTest(t, core, roomID)

	liveKitErr := errors.New("livekit unavailable")
	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	for i := 1; i < callReconcileListFailureThreshold; i++ {
		if err := core.callModel.ReconcileWithLiveKit(ctx); !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(failure %d) error = %v, want %v", i, err, liveKitErr)
		}
	}

	core.callModel.livekit = fakeLiveKitParticipantLister{
		snapshots: []liveKitParticipantSnapshot{{RoomID: roomID, CallID: callID, UserIDs: []string{"user1"}}},
	}
	if err := core.callModel.ReconcileWithLiveKit(ctx); err != nil {
		t.Fatalf("ReconcileWithLiveKit(success) error = %v", err)
	}

	core.callModel.livekit = fakeLiveKitParticipantLister{err: liveKitErr}
	for i := 1; i < callReconcileListFailureThreshold; i++ {
		if err := core.callModel.ReconcileWithLiveKit(ctx); !errors.Is(err, liveKitErr) {
			t.Fatalf("ReconcileWithLiveKit(second failure %d) error = %v, want %v", i, err, liveKitErr)
		}
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected failures after reset below threshold to keep participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); !ok {
		t.Fatal("Expected failures after reset below threshold to keep active call")
	}
}

func TestCallState_ReconcileWithLiveKitSuccessOnAnotherReplicaResetsListFailureCounter(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callID := activeCallIDForTest(t, core, roomID)

	liveKitErr := errors.New("livekit unavailable")
	failingReplica := NewCallModel(
		core.EventPublisher,
		core.callModel.callState,
		core.encryption.callKeys,
		fakeLiveKitParticipantLister{err: liveKitErr},
		nil,
		core.storage.memoryCacheKV,
		nil,
	)
	successfulReplica := NewCallModel(
		core.EventPublisher,
		core.callModel.callState,
		core.encryption.callKeys,
		fakeLiveKitParticipantLister{snapshots: []liveKitParticipantSnapshot{{RoomID: roomID, CallID: callID, UserIDs: []string{"user1"}}}},
		nil,
		core.storage.memoryCacheKV,
		nil,
	)

	for i := 1; i < callReconcileListFailureThreshold; i++ {
		if err := failingReplica.ReconcileWithLiveKit(ctx); !errors.Is(err, liveKitErr) {
			t.Fatalf("failing replica ReconcileWithLiveKit(%d) error = %v, want %v", i, err, liveKitErr)
		}
	}
	if err := successfulReplica.ReconcileWithLiveKit(ctx); err != nil {
		t.Fatalf("successful replica ReconcileWithLiveKit() error = %v", err)
	}
	if err := failingReplica.ReconcileWithLiveKit(ctx); !errors.Is(err, liveKitErr) {
		t.Fatalf("failing replica ReconcileWithLiveKit(after success) error = %v, want %v", err, liveKitErr)
	}

	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected one new failure after another replica's success to keep participants, got %d", len(participants))
	}
	if _, ok := core.callModel.activeCall(roomID); !ok {
		t.Fatal("Expected one new failure after another replica's success to keep active call")
	}
}

func TestCallState_CallEndedCommitsWhenKeyShredFails(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	started := callEvents[0].GetVoiceCallStarted()
	if started == nil || started.GetE2EeKeyRef() == "" {
		t.Fatalf("Expected started event with key ref")
	}

	shredErr := errors.New("kms shred unavailable")
	core.callModel.callKeys = failingShredCallKeyStore{
		delegate: core.encryption.callKeys,
		err:      shredErr,
	}
	err = core.RecordCallParticipantLeft(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER)
	if !errors.Is(err, shredErr) {
		t.Fatalf("RecordCallParticipantLeft() error = %v, want shred error", err)
	}

	callEvents, seq, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() after leave error = %v", err)
	}
	if len(callEvents) != 4 || callEvents[3].GetVoiceCallEnded() == nil {
		t.Fatalf("Expected committed CallEndedEvent despite shred failure, got %d events", len(callEvents))
	}
	if err := core.callModel.waitFor(ctx, events.SubjectPosition(evtstream.RoomAggregate(roomID).AllEventsFilter(), seq)); err != nil {
		t.Fatalf("CallStateProjector.WaitFor() error = %v", err)
	}
	if _, ok := core.callModel.activeCall(roomID); ok {
		t.Fatal("Expected committed CallEndedEvent to clear active call")
	}
	exists, err := core.encryption.callKeys.CallKeyExists(ctx, started.GetE2EeKeyRef())
	if err != nil {
		t.Fatalf("CallKeyExists() error = %v", err)
	}
	if !exists {
		t.Fatal("Expected call key to remain when shredding fails")
	}
}

func TestCallState_ReconciliationRechecksAfterConflict(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"
	calls := 0

	err := core.callModel.reconcileRoomParticipants(ctx, roomID, []string{"user1"}, func(ctx context.Context, roomID, userID string, joined bool) error {
		calls++
		if calls != 1 {
			t.Fatalf("appendEvent called %d times, want 1", calls)
		}
		if !joined {
			t.Fatal("Expected reconciliation to append a join correction")
		}
		if err := core.callModel.AppendJoined(ctx, roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION); err != nil {
			t.Fatalf("AppendJoined() error = %v", err)
		}
		return events.ErrConflict
	})
	if err != nil && !errors.Is(err, events.ErrConflict) {
		t.Fatalf("reconcileRoomParticipants() error = %v", err)
	}
	if err != nil {
		t.Fatalf("Expected conflict to be resolved by projection recheck, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("appendEvent called %d times, want 1", calls)
	}
	participants, _ := core.GetCallParticipants(roomID)
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant after simulated concurrent correction, got %d", len(participants))
	}
}

func TestVoiceCallE2EEKey_PerCallAndShreddedOnEnd(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	if _, err := core.GetVoiceCallE2EEKey(ctx, roomID); err == nil {
		t.Fatal("GetVoiceCallE2EEKey() before call should error")
	}

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	key1, err := core.GetVoiceCallE2EEKey(ctx, roomID)
	if err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() error = %v", err)
	}
	key2, err := core.GetVoiceCallE2EEKey(ctx, roomID)
	if err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() second error = %v", err)
	}
	if key1 == "" {
		t.Fatal("E2EE key should not be empty")
	}
	if key1 != key2 {
		t.Fatalf("E2EE key should be reused within the active call")
	}

	callEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	if err != nil {
		t.Fatalf("SubjectEvents() error = %v", err)
	}
	if len(callEvents) != 2 {
		t.Fatalf("Expected start+join facts after starting call, got %d", len(callEvents))
	}
	started := callEvents[0].GetVoiceCallStarted()
	if started == nil || started.GetE2EeKeyRef() == "" {
		t.Fatalf("Expected call started event with E2EE key ref")
	}
	exists, err := core.encryption.callKeys.CallKeyExists(ctx, started.GetE2EeKeyRef())
	if err != nil {
		t.Fatalf("CallKeyExists() error = %v", err)
	}
	if !exists {
		t.Fatal("Call key should exist while call is active")
	}

	if err := core.RecordCallParticipantLeft(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantLeft() error = %v", err)
	}
	exists, err = core.encryption.callKeys.CallKeyExists(ctx, started.GetE2EeKeyRef())
	if err != nil {
		t.Fatalf("CallKeyExists() after leave error = %v", err)
	}
	if exists {
		t.Fatal("Call key should be shredded when the call ends")
	}

	if err := core.RecordCallParticipantJoined(ctx, roomID, "user1", corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() second call error = %v", err)
	}
	key3, err := core.GetVoiceCallE2EEKey(ctx, roomID)
	if err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() second call error = %v", err)
	}
	if key3 == "" || key3 == key1 {
		t.Fatalf("New call should get a fresh E2EE key")
	}
}
