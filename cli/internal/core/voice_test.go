package core

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestLiveKitRoomName(t *testing.T) {
	tests := []struct {
		name     string
		serverID string
		spaceID  string
		roomID   string
		want     string
	}{
		{
			name:    "no server ID",
			spaceID: "space123",
			roomID:  "room456",
			want:    "space123_room456",
		},
		{
			name:     "with server ID",
			serverID: "my-instance",
			spaceID:  "space123",
			roomID:   "room456",
			want:     "my-instance.space123_room456",
		},
		{
			name:    "nanoid-style IDs without instance",
			spaceID: "V1StGXR8_Z5jdHi6B-myT",
			roomID:  "xYz9Abc_def",
			want:    "V1StGXR8_Z5jdHi6B-myT_xYz9Abc_def",
		},
		{
			name:     "nanoid-style IDs with instance",
			serverID: "prod-tenant-1",
			spaceID:  "V1StGXR8_Z5jdHi6B-myT",
			roomID:   "xYz9Abc_def",
			want:     "prod-tenant-1.V1StGXR8_Z5jdHi6B-myT_xYz9Abc_def",
		},
		{
			name:     "empty server ID treated as no prefix",
			serverID: "",
			spaceID:  "space123",
			roomID:   "room456",
			want:     "space123_room456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LiveKitRoomName(tt.serverID, tt.spaceID, tt.roomID)
			if got != tt.want {
				t.Errorf("LiveKitRoomName(%q, %q, %q) = %q, want %q", tt.serverID, tt.spaceID, tt.roomID, got, tt.want)
			}
		})
	}
}

func TestParseLiveKitRoomName(t *testing.T) {
	tests := []struct {
		name        string
		lkRoomName  string
		wantSpaceID string
		wantRoomID  string
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
			gotSpace, gotRoom := ParseLiveKitRoomName(tt.lkRoomName)
			if gotSpace != tt.wantSpaceID || gotRoom != tt.wantRoomID {
				t.Errorf("ParseLiveKitRoomName(%q) = (%q, %q), want (%q, %q)",
					tt.lkRoomName, gotSpace, gotRoom, tt.wantSpaceID, tt.wantRoomID)
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

	result, err := GenerateVoiceCallToken(apiKey, apiSecret, roomName, userID, displayName, login, avatarURL)
	if err != nil {
		t.Fatalf("GenerateVoiceCallToken() error = %v", err)
	}
	if result == nil {
		t.Fatal("GenerateVoiceCallToken() returned nil")
	}
	if result.Token == "" {
		t.Fatal("GenerateVoiceCallToken() returned empty token")
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

	// Verify exp and nbf claims exist (don't check duration — that's
	// the LiveKit library's responsibility via SetValidFor)
	if _, ok := claims["exp"].(float64); !ok {
		t.Error("Token missing exp claim")
	}
	if _, ok := claims["nbf"].(float64); !ok {
		t.Error("Token missing nbf claim")
	}
}

func TestGenerateVoiceCallToken_NoAvatar(t *testing.T) {
	result, err := GenerateVoiceCallToken("key", "secret", "room", "user1", "User One", "userone", "")
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

// ============================================================================
// Call State KV Tests (require embedded NATS)
// ============================================================================

func TestCallState_JoinAndLeave(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	// Initially no participants
	participants, err := core.GetCallParticipants(ctx, "channel", roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 0 {
		t.Errorf("Expected 0 participants, got %d", len(participants))
	}

	// Join — adds participant to KV
	err = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user1", "Alice", "alice", "https://example.com/alice.jpg")
	if err != nil {
		t.Fatalf("HandleCallParticipantJoined() error = %v", err)
	}
	if _, err := core.storage.memoryCacheKV.Get(ctx, callStateKey("channel", roomID)); err != nil {
		t.Fatalf("expected call state in MEMORY_CACHE: %v", err)
	}

	participants, err = core.GetCallParticipants(ctx, "channel", roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Expected 1 participant, got %d", len(participants))
	}
	if participants[0].UserID != "user1" {
		t.Errorf("UserID = %q, want %q", participants[0].UserID, "user1")
	}
	if participants[0].DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", participants[0].DisplayName, "Alice")
	}
	if participants[0].Login != "alice" {
		t.Errorf("Login = %q, want %q", participants[0].Login, "alice")
	}
	if participants[0].AvatarURL != "https://example.com/alice.jpg" {
		t.Errorf("AvatarURL = %q, want %q", participants[0].AvatarURL, "https://example.com/alice.jpg")
	}
	if participants[0].JoinedAt == 0 {
		t.Error("JoinedAt should not be zero")
	}

	// Leave — removes participant, deletes key when empty
	err = core.HandleCallParticipantLeft(ctx, "channel", roomID, "user1")
	if err != nil {
		t.Fatalf("HandleCallParticipantLeft() error = %v", err)
	}

	participants, err = core.GetCallParticipants(ctx, "channel", roomID)
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

	// Join twice — should not duplicate
	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user1", "Alice", "alice", "")
	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user1", "Alice", "alice", "")

	participants, _ := core.GetCallParticipants(ctx, "channel", roomID)
	if len(participants) != 1 {
		t.Errorf("Expected 1 participant (idempotent), got %d", len(participants))
	}
}

func TestCallState_LeaveNotInCall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Leave when not in a call — should be a no-op
	err := core.HandleCallParticipantLeft(ctx, "space1", "room1", "user1")
	if err != nil {
		t.Fatalf("HandleCallParticipantLeft() for absent user should not error, got %v", err)
	}
}

func TestCallState_MultipleParticipants(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := "room1"

	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user1", "Alice", "alice", "")
	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user2", "Bob", "bob", "")

	participants, _ := core.GetCallParticipants(ctx, "channel", roomID)
	if len(participants) != 2 {
		t.Fatalf("Expected 2 participants, got %d", len(participants))
	}

	// Remove one — other should remain
	_ = core.HandleCallParticipantLeft(ctx, "channel", roomID, "user1")
	participants, _ = core.GetCallParticipants(ctx, "channel", roomID)
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

	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user1", "Alice", "alice", "")
	_ = core.HandleCallParticipantJoined(ctx, "channel", roomID, "user2", "Bob", "bob", "")

	err := core.HandleCallRoomFinished(ctx, "channel", roomID)
	if err != nil {
		t.Fatalf("HandleCallRoomFinished() error = %v", err)
	}

	participants, _ := core.GetCallParticipants(ctx, "channel", roomID)
	if len(participants) != 0 {
		t.Errorf("Expected 0 participants after room finished, got %d", len(participants))
	}
}

func TestGetActiveCallRoomIDs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space1 := "space1"
	space2 := "space2"

	// No active calls initially
	ids, err := core.GetActiveCallRoomIDs(ctx, space1)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("Expected 0 room IDs, got %d", len(ids))
	}

	// Add calls in multiple rooms
	_ = core.HandleCallParticipantJoined(ctx, space1, "room1", "user1", "Alice", "alice", "")
	_ = core.HandleCallParticipantJoined(ctx, space1, "room2", "user2", "Bob", "bob", "")
	_ = core.HandleCallParticipantJoined(ctx, space2, "room3", "user3", "Carol", "carol", "")

	// Space1 should have 2 rooms
	ids, err = core.GetActiveCallRoomIDs(ctx, space1)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("Expected 2 room IDs for space1, got %d: %v", len(ids), ids)
	}

	// Space2 should have 1 room
	ids, err = core.GetActiveCallRoomIDs(ctx, space2)
	if err != nil {
		t.Fatalf("GetActiveCallRoomIDs() error = %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("Expected 1 room ID for space2, got %d: %v", len(ids), ids)
	}

	// Remove all participants from room1 — should no longer appear
	_ = core.HandleCallParticipantLeft(ctx, space1, "room1", "user1")
	ids, _ = core.GetActiveCallRoomIDs(ctx, space1)
	if len(ids) != 1 {
		t.Errorf("Expected 1 room ID after leave, got %d: %v", len(ids), ids)
	}
}
