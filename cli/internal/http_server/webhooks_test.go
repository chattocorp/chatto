package http_server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
	"google.golang.org/protobuf/encoding/protojson"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestLiveKitWebhookDuplicateIdentityLeaveDoesNotEndCall(t *testing.T) {
	const (
		apiKey    = "devkey"
		apiSecret = "devsecret"
		serverID  = "test-server"
		roomID    = "room1"
		userID    = "user1"
	)
	ctx := testContext(t)
	s := setupHTTPServerTestServer(t, config.AuthConfig{})
	s.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.example.test",
		APIKey:    apiKey,
		APISecret: apiSecret,
		ServerID:  serverID,
	}
	s.setupWebhookRoutes()

	if err := s.core.RecordCallParticipantJoined(ctx, core.KindChannel, roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	active, ok := s.core.CallState.ActiveCall(roomID)
	if !ok || active.CallID == "" {
		t.Fatalf("expected active call for room %s", roomID)
	}
	if _, err := s.core.GetVoiceCallE2EEKey(ctx, roomID); err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() before webhook error = %v", err)
	}

	event := &livekit.WebhookEvent{
		Event: webhook.EventParticipantLeft,
		Room: &livekit.Room{
			Name: core.LiveKitRoomName(serverID, core.LegacySpaceIDForRoomKind(core.KindChannel), roomID, active.CallID),
		},
		Participant: &livekit.ParticipantInfo{
			Identity:         userID,
			DisconnectReason: livekit.DisconnectReason_DUPLICATE_IDENTITY,
		},
	}
	req := signedLiveKitWebhookRequest(t, apiKey, apiSecret, event)
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	participants, err := s.core.GetCallParticipants(ctx, core.LegacySpaceIDForRoomKind(core.KindChannel), roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != userID {
		t.Fatalf("participants after duplicate identity leave = %+v, want user still active", participants)
	}
	if got, ok := s.core.CallState.ActiveCall(roomID); !ok || got.CallID != active.CallID {
		t.Fatalf("active call after duplicate identity leave = %+v, %v; want call %q active", got, ok, active.CallID)
	}
	if _, err := s.core.GetVoiceCallE2EEKey(ctx, roomID); err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() after duplicate identity leave error = %v", err)
	}

	leftEvents, _, err := s.core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(roomID).Subject(events.EventCallParticipantLeft))
	if err != nil {
		t.Fatalf("SubjectEvents(call_left) error = %v", err)
	}
	if len(leftEvents) != 0 {
		t.Fatalf("call_left events after duplicate identity leave = %d, want 0", len(leftEvents))
	}
	endedEvents, _, err := s.core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(roomID).Subject(events.EventCallEnded))
	if err != nil {
		t.Fatalf("SubjectEvents(call_ended) error = %v", err)
	}
	if len(endedEvents) != 0 {
		t.Fatalf("call_ended events after duplicate identity leave = %d, want 0", len(endedEvents))
	}
}

func signedLiveKitWebhookRequest(t *testing.T, apiKey, apiSecret string, event *livekit.WebhookEvent) *http.Request {
	t.Helper()
	body, err := protojson.Marshal(event)
	if err != nil {
		t.Fatalf("marshal webhook event: %v", err)
	}
	sum := sha256.Sum256(body)
	hash := base64.StdEncoding.EncodeToString(sum[:])
	token, err := auth.NewAccessToken(apiKey, apiSecret).
		SetValidFor(5 * time.Minute).
		SetSha256(hash).
		ToJWT()
	if err != nil {
		t.Fatalf("sign webhook event: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/livekit", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/webhook+json")
	return req
}

func TestLiveKitWebhookRoomBelongsToInstance(t *testing.T) {
	tests := []struct {
		name       string
		roomName   string
		instanceID string
		want       bool
	}{
		{
			name:       "matching hosted instance prefix",
			roomName:   "foo.channel_room",
			instanceID: "foo",
			want:       true,
		},
		{
			name:       "foreign hosted instance prefix",
			roomName:   "bar.channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "unprefixed room rejected for hosted instance",
			roomName:   "channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "legacy unprefixed room accepted without instance ID",
			roomName:   "channel_room",
			instanceID: "",
			want:       true,
		},
		{
			name:       "prefixed room rejected without instance ID",
			roomName:   "foo.channel_room",
			instanceID: "",
			want:       false,
		},
		{
			name:       "prefix must match exactly",
			roomName:   "foobar.channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "empty room rejected for hosted instance",
			roomName:   "",
			instanceID: "foo",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := liveKitWebhookRoomBelongsToInstance(tt.roomName, tt.instanceID)
			if got != tt.want {
				t.Fatalf("liveKitWebhookRoomBelongsToInstance(%q, %q) = %v, want %v", tt.roomName, tt.instanceID, got, tt.want)
			}
		})
	}
}
