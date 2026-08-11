package http_server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestIncomingBotWebhookIsAuthenticatedInstalledAndWriteOnly(t *testing.T) {
	ctx := testContext(t)
	s := setupHTTPServerTestServer(t, config.AuthConfig{})
	s.setupIncomingBotWebhookRoutes()
	owner, err := s.core.CreateUser(ctx, core.SystemActorID, "webhook-owner", "Webhook Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.core.GrantUserPermission(ctx, core.SystemActorID, owner.GetId(), core.PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := s.core.CreateBot(ctx, core.SystemActorID, owner.GetId(), "webhook_bot", "Webhook Bot", "Posts incoming deployment updates.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.SetBotCapabilities(ctx, core.SystemActorID, bot.GetId(), []string{string(core.ApplicationCapabilityMessageWrite)}); err != nil {
		t.Fatal(err)
	}
	key, _, err := s.core.RotateBotAPIKey(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatal(err)
	}
	room, err := s.core.CreateRoom(ctx, owner.GetId(), core.KindChannel, "", "webhook-room", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.core.JoinRoom(ctx, owner.GetId(), core.KindChannel, owner.GetId(), room.GetId()); err != nil {
		t.Fatal(err)
	}
	if err := s.core.GrantUserRoomPermission(ctx, core.SystemActorID, room.GetId(), owner.GetId(), core.PermRoomManage); err != nil {
		t.Fatal(err)
	}

	postToRoom := func(path, bearer, roomID string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(incomingBotWebhookRequest{RoomID: roomID, Body: "deployment complete @webhook_bot"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, req)
		return recorder
	}
	post := func(path, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		return postToRoom(path, bearer, room.GetId())
	}

	if got := post("/webhooks/bots/"+bot.GetId(), ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d", got.Code)
	}
	if got := post("/webhooks/bots/wrong-bot", key); got.Code != http.StatusUnauthorized {
		t.Fatalf("path/key mismatch status = %d", got.Code)
	}
	if got := post("/webhooks/bots/"+bot.GetId(), key); got.Code != http.StatusForbidden {
		t.Fatalf("not-installed status = %d, body = %s", got.Code, got.Body.String())
	}
	if got := postToRoom("/webhooks/bots/"+bot.GetId(), key, "unknown-room"); got.Code != http.StatusForbidden {
		t.Fatalf("unknown-room status = %d, body = %s", got.Code, got.Body.String())
	}
	if _, err := s.core.RoomCommands().AddMember(ctx, core.RoomUserInput{
		ActorID: owner.GetId(), RoomID: room.GetId(), UserID: bot.GetId(),
	}); err != nil {
		t.Fatalf("install bot: %v", err)
	}
	created := post("/webhooks/bots/"+bot.GetId(), key)
	if created.Code != http.StatusCreated {
		t.Fatalf("installed status = %d, body = %s", created.Code, created.Body.String())
	}
	var response incomingBotWebhookResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil || response.MessageID == "" {
		t.Fatalf("response = %+v, error = %v", response, err)
	}
	event, err := s.core.GetRoomEventByEventID(ctx, core.KindChannel, room.GetId(), response.MessageID)
	if err != nil || event.GetActorId() != bot.GetId() {
		t.Fatalf("created event = %+v, error = %v", event, err)
	}
	if got := event.GetMessagePosted().GetDirectMentionedBotIds(); len(got) != 0 {
		t.Fatalf("bot-authored webhook minted bot invitations: %v", got)
	}
	if err := s.core.DenyUserRoomPermission(ctx, core.SystemActorID, room.GetId(), owner.GetId(), core.PermMessagePost); err != nil {
		t.Fatal(err)
	}
	if got := post("/webhooks/bots/"+bot.GetId(), key); got.Code != http.StatusForbidden {
		t.Fatalf("owner-denied status = %d, body = %s", got.Code, got.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/webhooks/bots/"+bot.GetId(), nil)
	get.Header.Set("Authorization", "Bearer "+key)
	getRecorder := httptest.NewRecorder()
	s.router.ServeHTTP(getRecorder, get)
	if getRecorder.Code != http.StatusNotFound {
		t.Fatalf("webhook read method status = %d, want 404", getRecorder.Code)
	}
}

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

	if err := s.core.RecordCallParticipantJoined(ctx, roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	active, ok, err := s.core.GetActiveCall(roomID)
	if err != nil {
		t.Fatalf("GetActiveCall() error = %v", err)
	}
	if !ok || active.CallID == "" {
		t.Fatalf("expected active call for room %s", roomID)
	}
	if _, err := s.core.GetVoiceCallE2EEKey(ctx, roomID); err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() before webhook error = %v", err)
	}

	event := &livekit.WebhookEvent{
		Event: webhook.EventParticipantLeft,
		Room: &livekit.Room{
			Name: core.LiveKitRoomName(serverID, core.KindChannel, roomID, active.CallID),
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

	participants, err := s.core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != userID {
		t.Fatalf("participants after duplicate identity leave = %+v, want user still active", participants)
	}
	if got, ok, err := s.core.GetActiveCall(roomID); err != nil || !ok || got.CallID != active.CallID {
		t.Fatalf("active call after duplicate identity leave = %+v, %v; want call %q active", got, ok, active.CallID)
	}
	if _, err := s.core.GetVoiceCallE2EEKey(ctx, roomID); err != nil {
		t.Fatalf("GetVoiceCallE2EEKey() after duplicate identity leave error = %v", err)
	}

	leftEvents, _, err := s.core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallParticipantLeft))
	if err != nil {
		t.Fatalf("SubjectEvents(call_left) error = %v", err)
	}
	if len(leftEvents) != 0 {
		t.Fatalf("call_left events after duplicate identity leave = %d, want 0", len(leftEvents))
	}
	endedEvents, _, err := s.core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventCallEnded))
	if err != nil {
		t.Fatalf("SubjectEvents(call_ended) error = %v", err)
	}
	if len(endedEvents) != 0 {
		t.Fatalf("call_ended events after duplicate identity leave = %d, want 0", len(endedEvents))
	}
}

func TestLiveKitWebhookParticipantLeftUsesParsedRoomID(t *testing.T) {
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

	if err := s.core.RecordCallParticipantJoined(ctx, roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	active, ok, err := s.core.GetActiveCall(roomID)
	if err != nil {
		t.Fatalf("GetActiveCall() error = %v", err)
	}
	if !ok || active.CallID == "" {
		t.Fatalf("expected active call for room %s", roomID)
	}

	event := &livekit.WebhookEvent{
		Event: webhook.EventParticipantLeft,
		Room: &livekit.Room{
			Name: core.LiveKitRoomName(serverID, core.KindChannel, roomID, active.CallID),
		},
		Participant: &livekit.ParticipantInfo{Identity: userID},
	}
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, signedLiveKitWebhookRequest(t, apiKey, apiSecret, event))
	if recorder.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	participants, err := s.core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants after leave = %+v, want none", participants)
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
