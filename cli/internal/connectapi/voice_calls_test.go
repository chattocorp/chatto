package connectapi

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestCreateCallTokenAvatarUsesCanonicalOrigin(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("voice-avatar-origin")
	env.api.config.Webserver.URL = "https://chat.example"
	env.api.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}

	var avatar bytes.Buffer
	if err := png.Encode(&avatar, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatalf("encode avatar: %v", err)
	}
	asset, err := env.core.UploadUserAvatar(env.ctx, env.viewer.Id, &avatar)
	if err != nil {
		t.Fatalf("UploadUserAvatar: %v", err)
	}
	if err := env.core.SetUserAvatar(env.ctx, env.viewer.Id, asset); err != nil {
		t.Fatalf("SetUserAvatar: %v", err)
	}

	// The caller uses a hostname alias. Other call participants read the
	// avatar URL from the token metadata, so it must use webserver.url.
	ctx := WithRequestBaseURL(withCaller(env.ctx, env.viewer), "https://alias.example")
	if _, err := env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id})); err != nil {
		t.Fatalf("JoinCall: %v", err)
	}
	tokenResp, err := env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("CreateCallToken: %v", err)
	}

	token, _, err := jwt.NewParser(jwt.WithoutClaimsValidation()).ParseUnverified(tokenResp.Msg.GetToken(), jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse call token: %v", err)
	}
	metadata, _ := token.Claims.(jwt.MapClaims)["metadata"].(string)
	avatarURL := core.ParseParticipantMetadata(metadata).AvatarURL
	if !strings.HasPrefix(avatarURL, "https://chat.example/assets/server/") {
		t.Fatalf("participant avatar URL = %q, want URL on webserver.url origin", avatarURL)
	}
}
