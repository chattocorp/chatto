package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func TestVoiceCallIntentResolversRecordCallFacts(t *testing.T) {
	env := setupTestResolver(t)
	env.resolver.livekitConfig = configuredLiveKitForTest()

	mutation := env.resolver.Mutation()
	input := model.VoiceCallIntentInput{RoomID: env.testRoom.Id}

	ok, err := mutation.JoinVoiceCall(env.authContext(), input)
	if err != nil {
		t.Fatalf("JoinVoiceCall() error = %v", err)
	}
	if !ok {
		t.Fatal("JoinVoiceCall() returned false with LiveKit configured")
	}

	participants, err := env.core.GetCallParticipants(env.ctx, core.LegacySpaceIDForRoomKind(core.KindChannel), env.testRoom.Id)
	if err != nil {
		t.Fatalf("GetCallParticipants() after join error = %v", err)
	}
	if len(participants) != 1 || participants[0].UserID != env.testUser.Id {
		t.Fatalf("participants after join = %#v, want only %s", participants, env.testUser.Id)
	}

	ok, err = mutation.LeaveVoiceCall(env.authContext(), input)
	if err != nil {
		t.Fatalf("LeaveVoiceCall() error = %v", err)
	}
	if !ok {
		t.Fatal("LeaveVoiceCall() returned false with LiveKit configured")
	}

	participants, err = env.core.GetCallParticipants(env.ctx, core.LegacySpaceIDForRoomKind(core.KindChannel), env.testRoom.Id)
	if err != nil {
		t.Fatalf("GetCallParticipants() after leave error = %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants after leave = %#v, want none", participants)
	}
}

func TestVoiceCallIntentResolversRequireRoomMembership(t *testing.T) {
	env := setupTestResolver(t)
	env.resolver.livekitConfig = configuredLiveKitForTest()

	nonMember := env.createVerifiedUser(t, "nonmember", "Non Member", "password123")
	mutation := env.resolver.Mutation()
	input := model.VoiceCallIntentInput{RoomID: env.testRoom.Id}

	if _, err := mutation.JoinVoiceCall(env.authContextForUser(nonMember), input); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("JoinVoiceCall() error = %v, want ErrNotRoomMember", err)
	}
	if _, err := mutation.LeaveVoiceCall(env.authContextForUser(nonMember), input); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("LeaveVoiceCall() error = %v, want ErrNotRoomMember", err)
	}
}

func configuredLiveKitForTest() config.LiveKitConfig {
	return config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}
}
