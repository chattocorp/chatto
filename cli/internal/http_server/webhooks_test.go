package http_server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestIncomingWebhookPostsThroughBotPermissionsAndSupportsExistingDMs(t *testing.T) {
	ctx := testContext(t)
	s := setupHTTPServerTestServer(t, config.AuthConfig{})
	s.setupWebhookRoutes()
	owner, err := s.core.CreateUser(ctx, core.SystemActorID, "incoming-owner", "Incoming Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := s.core.CreateBot(ctx, owner.GetId(), "incoming_bot", "Incoming Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	webhook, err := s.core.CreateBotIncomingWebhook(ctx, owner.GetId(), bot.User.GetId(), "HTTP test")
	if err != nil {
		t.Fatalf("CreateBotIncomingWebhook: %v", err)
	}
	room, err := s.core.CreateRoom(ctx, owner.GetId(), core.KindChannel, "", "incoming-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := s.core.AddMember(ctx, owner.GetId(), core.KindChannel, room.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("AddMember bot: %v", err)
	}
	path := "/webhooks/incoming/" + webhook.Credential
	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"denied","channel":"`+room.GetId()+`"}`))
	s.router.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound || denied.Body.String() != "channel_not_found" {
		t.Fatalf("permission-denied webhook = %d %q", denied.Code, denied.Body.String())
	}
	if err := s.core.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeServer}, core.PermMessagePost, core.PermissionStateAllow); err != nil {
		t.Fatalf("grant bot message.post: %v", err)
	}
	if err := s.core.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeServer}, core.PermMessagePostInThread, core.PermissionStateAllow); err != nil {
		t.Fatalf("grant bot message.post-in-thread: %v", err)
	}

	post := func(target, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		s.router.ServeHTTP(recorder, req)
		return recorder
	}
	response := post(path+"?room_id="+room.GetId(), `{"text":"  from Slack  ","channel":"`+room.GetId()+`","create_thread":true}`)
	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("channel webhook = %d %q", response.Code, response.Body.String())
	}
	events, _, err := s.core.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.GetId()).Subject(evtstream.EventMessagePosted))
	if err != nil || len(events) != 1 {
		t.Fatalf("message events = %d, %v", len(events), err)
	}
	if body, err := s.core.GetMessageBody(ctx, events[0].GetId()); err != nil || body != "  from Slack  " {
		t.Fatalf("message body = %q, %v", body, err)
	}
	if metadata, err := s.core.GetThreadMetadata(ctx, core.KindChannel, room.GetId(), events[0].GetId()); err != nil || !metadata.Exists {
		t.Fatalf("thread metadata = %+v, %v", metadata, err)
	}

	dm, _, err := s.core.RoomCommands().StartDM(ctx, core.RoomStartDMInput{ActorID: owner.GetId(), ParticipantIDs: []string{bot.User.GetId()}})
	if err != nil {
		t.Fatalf("StartDM: %v", err)
	}
	response = post(path, `{"body":"DM reply","room_id":"`+dm.GetId()+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("DM webhook = %d %q", response.Code, response.Body.String())
	}
	response = post(path, `{"text":"threaded DM","channel":"`+dm.GetId()+`","create_thread":true}`)
	if response.Code != http.StatusBadRequest || response.Body.String() != "invalid_payload" {
		t.Fatalf("threaded DM webhook = %d %q", response.Code, response.Body.String())
	}

	response = post(path+"?room_id="+room.GetId(), `{"text":"mismatch","channel":"`+dm.GetId()+`"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched target webhook = %d %q", response.Code, response.Body.String())
	}
	response = post("/webhooks/incoming/invalid", `not-json`)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid credential webhook = %d %q", response.Code, response.Body.String())
	}
}

func TestIncomingWebhookRecordsUseAfterAuthenticationBeforePayloadValidation(t *testing.T) {
	ctx := testContext(t)
	s := setupHTTPServerTestServer(t, config.AuthConfig{})
	s.setupWebhookRoutes()
	owner, err := s.core.CreateUser(ctx, core.SystemActorID, "usage-http-owner", "Usage HTTP Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := s.core.CreateBot(ctx, owner.GetId(), "usage_http_bot", "Usage HTTP Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	webhook, err := s.core.CreateBotIncomingWebhook(ctx, owner.GetId(), bot.User.GetId(), "Invalid payload test")
	if err != nil {
		t.Fatalf("CreateBotIncomingWebhook: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhooks/incoming/"+webhook.Credential, strings.NewReader(`not-json`))
	s.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || recorder.Body.String() != "invalid_payload" {
		t.Fatalf("invalid payload response = %d %q", recorder.Code, recorder.Body.String())
	}
	managed, err := s.core.GetBot(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetBot: %v", err)
	}
	s.core.HydrateBotCredentialUsage(ctx, managed)
	if len(managed.IncomingWebhooks) != 1 || managed.IncomingWebhooks[0].LastUsedState != core.BotCredentialLastUsedRecorded || managed.IncomingWebhooks[0].LastUsedAt.IsZero() {
		t.Fatalf("last-used metadata = %+v", managed.IncomingWebhooks)
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

func TestLiveKitWebhookIgnoresCompanionPublisherMembership(t *testing.T) {
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
		Enabled: true, URL: "ws://livekit.example.test", APIKey: apiKey, APISecret: apiSecret, ServerID: serverID,
	}
	s.setupWebhookRoutes()
	if err := s.core.RecordCallParticipantJoined(ctx, roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined() error = %v", err)
	}
	active, ok, err := s.core.GetActiveCall(roomID)
	if err != nil || !ok {
		t.Fatalf("GetActiveCall() = %+v, %v", active, err)
	}

	for _, eventName := range []string{webhook.EventParticipantJoined, webhook.EventParticipantLeft} {
		event := &livekit.WebhookEvent{
			Event: eventName,
			Room:  &livekit.Room{Name: core.LiveKitRoomName(serverID, core.KindChannel, roomID, active.CallID)},
			Participant: &livekit.ParticipantInfo{
				Identity: "publisher1",
				Metadata: `{"publisherKind":"game_share","ownerIdentity":"user1"}`,
			},
		}
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, signedLiveKitWebhookRequest(t, apiKey, apiSecret, event))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s webhook status = %d", eventName, recorder.Code)
		}
	}

	participants, err := s.core.GetCallParticipants(roomID)
	if err != nil {
		t.Fatalf("GetCallParticipants() error = %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != userID {
		t.Fatalf("participants = %+v, want only owner", participants)
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
