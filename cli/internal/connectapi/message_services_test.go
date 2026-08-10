package connectapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/core/linkpreview"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMessageServiceFetchLinkPreviewRequiresAuthMapsPreviewAndPostsToken(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.messages.FetchLinkPreview(env.ctx, connect.NewRequest(&apiv1.FetchLinkPreviewRequest{Url: "https://example.test"})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated FetchLinkPreview code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	restoreLocalhost := linkpreview.AllowLocalhostForTesting()
	defer restoreLocalhost()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
<meta property="og:title" content="Connect Preview">
<meta property="og:description" content="Connect preview description">
<meta property="og:site_name" content="Connect Site">
<meta property="og:image" content="` + serverURL + `/preview.png">
</head>
<body>hello</body>
</html>`))
		case "/preview.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(connectAPITestPNG())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	resp, err := env.messages.FetchLinkPreview(
		withCaller(env.ctx, env.viewer),
		connect.NewRequest(&apiv1.FetchLinkPreviewRequest{Url: server.URL + "/article"}),
	)
	if err != nil {
		t.Fatalf("FetchLinkPreview: %v", err)
	}
	preview := resp.Msg.GetPreview()
	if preview == nil {
		t.Fatal("FetchLinkPreview preview = nil")
	}
	if preview.GetUrl() != server.URL+"/article" ||
		preview.GetTitle() != "Connect Preview" ||
		preview.GetDescription() != "Connect preview description" ||
		preview.GetSiteName() != "Connect Site" {
		t.Fatalf("preview = %+v", preview)
	}
	if preview.GetImageAssetId() == "" {
		t.Fatalf("ImageAssetId is empty")
	}
	if !strings.Contains(preview.GetImageUrl(), preview.GetImageAssetId()) {
		t.Fatalf("ImageUrl %q does not contain asset id %q", preview.GetImageUrl(), preview.GetImageAssetId())
	}
	if !strings.Contains(preview.GetImageUrl(), "/assets/server/"+core.PublicServerAssetObjectPrefix) {
		t.Fatalf("ImageUrl %q does not use the public NATS namespace", preview.GetImageUrl())
	}
	if resp.Msg.GetPreviewToken() == "" {
		t.Fatalf("PreviewToken is empty")
	}

	room := env.createJoinedRoom("message-preview-token")
	createResp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:           room.Id,
		Body:             "message with preview",
		LinkPreviewToken: resp.Msg.GetPreviewToken(),
	}))
	if err != nil {
		t.Fatalf("CreateMessage with preview token: %v", err)
	}
	message := createResp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil")
	}
	body, err := env.core.GetFullMessageBody(env.ctx, message.GetId())
	if err != nil {
		t.Fatalf("GetFullMessageBody: %v", err)
	}
	stored := body.LinkPreview
	if stored == nil || stored.GetTitle() != "Connect Preview" || stored.GetDescription() != "Connect preview description" || stored.GetImageAssetId() == "" {
		t.Fatalf("stored link preview = %+v", stored)
	}
}

func TestAbsolutizeAssetURL(t *testing.T) {
	t.Run("uses configured webserver URL first", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://configured.example.com/chatto"},
		}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://request.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "/assets/logo.png"), "https://configured.example.com/assets/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})

	t.Run("falls back to request base URL", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://remote.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "/assets/logo.png"), "https://remote.example.com/assets/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})

	t.Run("keeps already absolute URLs", func(t *testing.T) {
		api := New(nil, config.ChattoConfig{}, "test")
		ctx := WithRequestBaseURL(context.Background(), "https://remote.example.com")

		if got, want := api.absolutizeAssetURL(ctx, "https://cdn.example.com/logo.png"), "https://cdn.example.com/logo.png"; got != want {
			t.Fatalf("absolutizeAssetURL = %q, want %q", got, want)
		}
	})
}

func TestRoomAndThreadTimelineRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "timeline-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	req := connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id})
	if _, err := env.rooms.GetRoomEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-outsider", "Timeline Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.GetRoomEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceCreateMessageRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-authz")
	req := connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "hello",
	})

	if _, err := env.messages.CreateMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-outsider", "Message Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.CreateMessage(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied CreateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceRejectsDMThreads(t *testing.T) {
	env := newConnectAPITestEnv(t)
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-dm-thread-participant", "DM Thread Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{participant.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	root, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: dm.Id,
		Body:   "DM root",
	}))
	if err != nil {
		t.Fatalf("CreateMessage root: %v", err)
	}
	rootID := root.Msg.GetMessage().GetId()

	_, err = env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:            dm.Id,
		Body:              "forbidden thread reply",
		ThreadRootEventId: rootID,
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("CreateMessage DM thread code = %v, want invalid argument", got)
	}

	flat, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:    dm.Id,
		Body:      "allowed flat reply",
		InReplyTo: rootID,
	}))
	if err != nil {
		t.Fatalf("CreateMessage flat DM reply: %v", err)
	}
	if got := flat.Msg.GetMessage(); got.GetThreadRootEventId() != "" || got.GetInReplyTo() != rootID {
		t.Fatalf("flat DM reply = %+v, want reply attribution without thread", got)
	}
}

func TestMessageServiceAddAndRemoveRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-authz")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	req := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})

	if _, err := env.messages.AddReaction(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reaction-outsider", "Reaction Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.AddReaction(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageReact); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.AddReaction(withCaller(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceAddAndRemoveResponseSemantics(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-response")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	ctx := withCaller(env.ctx, env.viewer)

	addReq := connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	addResp, err := env.messages.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("AddReaction: %v", err)
	}
	if !addResp.Msg.Added {
		t.Fatal("AddReaction Added = false, want true")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("AddReaction reaction = %+v, want thumbsup count 1 hasReacted", got)
	}

	addResp, err = env.messages.AddReaction(ctx, addReq)
	if err != nil {
		t.Fatalf("duplicate AddReaction: %v", err)
	}
	if addResp.Msg.Added {
		t.Fatal("duplicate AddReaction Added = true, want false")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("duplicate AddReaction reaction = %+v, want unchanged thumbsup count 1 hasReacted", got)
	}

	removeReq := connect.NewRequest(&apiv1.RemoveReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "thumbsup",
	})
	removeResp, err := env.messages.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("RemoveReaction: %v", err)
	}
	if !removeResp.Msg.Removed {
		t.Fatal("RemoveReaction Removed = false, want true")
	}
	if removeResp.Msg.GetReaction() != nil {
		t.Fatalf("RemoveReaction reaction = %+v, want nil after last reaction removed", removeResp.Msg.GetReaction())
	}

	removeResp, err = env.messages.RemoveReaction(ctx, removeReq)
	if err != nil {
		t.Fatalf("duplicate RemoveReaction: %v", err)
	}
	if removeResp.Msg.Removed {
		t.Fatal("duplicate RemoveReaction Removed = true, want false")
	}
	if removeResp.Msg.GetReaction() != nil {
		t.Fatalf("duplicate RemoveReaction reaction = %+v, want nil", removeResp.Msg.GetReaction())
	}
}

func TestMessageServiceAddReactionMapsPerUserMessageLimit(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-limit")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")
	ctx := withCaller(env.ctx, env.viewer)
	emojis := []string{
		"100", "blush", "clap", "eyes", "fire", "heart", "heart_eyes", "kissing_heart", "laughing", "muscle",
		"ok_hand", "pray", "raised_hands", "rocket", "smile", "star", "tada", "thinking", "thumbsup", "wave", "wink",
	}

	for _, emoji := range emojis[:core.MaxReactionsPerUserPerMessage] {
		resp, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
			RoomId: room.Id, MessageEventId: event.Id, Emoji: emoji,
		}))
		if err != nil {
			t.Fatalf("AddReaction %q: %v", emoji, err)
		}
		if !resp.Msg.GetAdded() {
			t.Fatalf("AddReaction %q added = false, want true", emoji)
		}
	}
	_, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId: room.Id, MessageEventId: event.Id, Emoji: emojis[core.MaxReactionsPerUserPerMessage],
	}))
	if got := connect.CodeOf(err); got != connect.CodeResourceExhausted {
		t.Fatalf("AddReaction above limit code = %v, want %v", got, connect.CodeResourceExhausted)
	}
}

func TestMessageServiceReactionOnEchoCanonicalizesToOriginal(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-echo")
	ctx := withCaller(env.ctx, env.viewer)

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "reply", nil, root.Id, root.Id, nil, true)
	if err != nil {
		t.Fatalf("PostMessage reply with echo: %v", err)
	}
	echoID, ok := env.core.ChannelEchoEventID(reply.Id)
	if !ok {
		t.Fatal("expected channel echo for reply")
	}

	addResp, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: echoID,
		Emoji:          "thumbsup",
	}))
	if err != nil {
		t.Fatalf("AddReaction via echo: %v", err)
	}
	if !addResp.Msg.GetAdded() {
		t.Fatal("AddReaction via echo Added = false, want true")
	}
	if got := addResp.Msg.GetReaction(); got.GetEmoji() != "thumbsup" || got.GetCount() != 1 || !got.GetHasReacted() {
		t.Fatalf("AddReaction via echo reaction = %+v, want thumbsup count 1 hasReacted", got)
	}

	dupResp, err := env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: reply.Id,
		Emoji:          "thumbsup",
	}))
	if err != nil {
		t.Fatalf("duplicate AddReaction via original: %v", err)
	}
	if dupResp.Msg.GetAdded() {
		t.Fatal("duplicate AddReaction via original Added = true, want false")
	}

	roomResp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	echoEvent := timelinePageEvent(roomResp.Msg.GetPage(), echoID)
	if echoEvent == nil || echoEvent.GetMessagePosted() == nil {
		t.Fatalf("echo event %s missing from room page", echoID)
	}
	if got := echoEvent.GetMessagePosted().GetMessage().GetReactions(); len(got) != 1 || got[0].GetEmoji() != "thumbsup" || got[0].GetCount() != 1 || !got[0].GetHasReacted() {
		t.Fatalf("echo reactions = %+v, want thumbsup count 1 hasReacted", got)
	}

	threadResp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents: %v", err)
	}
	replyEvent := timelinePageEvent(threadResp.Msg.GetPage(), reply.Id)
	if replyEvent == nil || replyEvent.GetMessagePosted() == nil {
		t.Fatalf("reply event %s missing from thread page", reply.Id)
	}
	if got := replyEvent.GetMessagePosted().GetMessage().GetReactions(); len(got) != 1 || got[0].GetEmoji() != "thumbsup" || got[0].GetCount() != 1 || !got[0].GetHasReacted() {
		t.Fatalf("reply reactions = %+v, want thumbsup count 1 hasReacted", got)
	}
}

func TestMessageServiceValidatesEmoji(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("reaction-validation")
	event := env.post(room.Id, env.viewer.Id, "react to me", "")

	_, err := env.messages.AddReaction(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.AddReactionRequest{
		RoomId:         room.Id,
		MessageEventId: event.Id,
		Emoji:          "totally_bogus",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid emoji AddReaction code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func TestMessageServiceCreateMessageValidatesInput(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-validation")
	ctx := withCaller(env.ctx, env.viewer)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	otherRoom := env.createJoinedRoom("message-post-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

	tests := []struct {
		name string
		req  *apiv1.CreateMessageRequest
		code connect.Code
	}{
		{
			name: "missing room",
			req:  &apiv1.CreateMessageRequest{Body: "hello"},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "empty body and no attachments",
			req:  &apiv1.CreateMessageRequest{RoomId: room.Id, Body: "   "},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "channel echo outside thread",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "hello",
				AlsoSendToChannel: true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "create thread for thread reply",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "invalid",
				ThreadRootEventId: root.Id,
				CreateThread:      true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "create thread while implicitly inheriting a thread",
			req: &apiv1.CreateMessageRequest{
				RoomId:       room.Id,
				Body:         "invalid",
				InReplyTo:    reply.Id,
				CreateThread: true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: "missing-reply-target",
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply",
				InReplyTo: otherRoomMessage.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "invalid link preview token",
			req: &apiv1.CreateMessageRequest{
				RoomId:           room.Id,
				Body:             "hello",
				LinkPreviewToken: "not-a-token",
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := env.messages.CreateMessage(ctx, connect.NewRequest(tt.req)); connect.CodeOf(err) != tt.code {
				t.Fatalf("CreateMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
		})
	}
}

func TestMessageServiceCreateMessageInfersVideoProcessingAssetIDs(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Video.Enabled = true
	env.core.VideoUploadsEnabled = true
	room := env.createJoinedRoom("message-post-video")
	assetID := env.uploadAttachmentAsset(t, room.Id, "clip.mp4", "video/mp4", []byte("original video"))

	if _, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{assetID},
	})); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	manifest := env.core.GetAssetState(assetID).VideoManifest
	if manifest == nil || manifest.Started == nil {
		t.Fatalf("VideoAttachmentManifest = %+v; want started", manifest)
	}
}

func TestMessageServiceCreateMessageReturnsRenderableMessage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-success")

	resp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "hello over connect",
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil, response = %+v", resp.Msg)
	}
	if message.Body == nil || message.GetBody() != "hello over connect" {
		t.Fatalf("message body = %q present=%v, want posted body", message.GetBody(), message.Body != nil)
	}
	if message.GetThread() != nil {
		t.Fatalf("message thread = %+v, want nil for an ordinary root", message.GetThread())
	}
}

func TestMessageServiceCreateMessageReturnsCreatedEmptyThread(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-thread-root")
	ctx := withCaller(env.ctx, env.viewer)

	resp, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:       room.Id,
		Body:         "discuss in the thread",
		CreateThread: true,
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	thread := resp.Msg.GetMessage().GetThread()
	if thread == nil {
		t.Fatalf("thread = %+v, want existing empty thread", thread)
	}
	if thread.GetReplyCount() != 0 {
		t.Fatalf("reply count = %d, want 0", thread.GetReplyCount())
	}
	if state := thread.GetViewerState(); state == nil || !state.GetIsFollowing() {
		t.Fatalf("viewer state = %+v, want following", state)
	}

	followed, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if len(followed.Msg.GetThreads()) != 1 {
		t.Fatalf("followed threads = %d, want 1", len(followed.Msg.GetThreads()))
	}
	followedSummary := followed.Msg.GetThreads()[0].GetThread()
	if followedSummary == nil || followedSummary.GetReplyCount() != 0 {
		t.Fatalf("followed thread summary = %+v, want existing empty thread", followedSummary)
	}
}

func TestMessageServiceCreateMessageRequiresThreadPostPermissionToCreateThread(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("thread-root-permission")
	ctx := withCaller(env.ctx, env.viewer)

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePostInThread); err != nil {
		t.Fatalf("DenyRoomPermission thread post: %v", err)
	}

	if _, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: room.Id,
		Body:   "ordinary roots remain allowed",
	})); err != nil {
		t.Fatalf("CreateMessage ordinary root: %v", err)
	}

	_, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:       room.Id,
		Body:         "thread creation must be denied",
		CreateThread: true,
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreateMessage explicit thread code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceCreateMessageUploadsAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-upload")
	assetID := env.uploadAttachmentAsset(t, room.Id, "note.txt", "text/plain", []byte("uploaded over connect"))

	resp, err := env.messages.CreateMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{assetID},
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage message = nil, response = %+v", resp.Msg)
	}
	attachments := message.GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}
	if attachments[0].GetFilename() != "note.txt" || attachments[0].GetContentType() != "text/plain" {
		t.Fatalf("attachment = %+v, want note.txt text/plain", attachments[0])
	}
	if attachments[0].GetId() == "" {
		t.Fatal("attachment id is empty")
	}
}

func TestMessageServiceCreateMessageAttachmentPreflightDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-upload-preflight")
	ctx := withCaller(env.ctx, env.viewer)

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageAttach); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before denied post: %v", err)
	}
	sum := sha256.Sum256([]byte("denied upload"))
	_, err = env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len("denied upload")),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied attachment CreateUpload code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after denied post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after denied attachment = %d, want unchanged %d", after, before)
	}
}

func TestMessageServiceCreateMessageBroadMentionWithAttachment(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-broad-mention")
	ctx := withCaller(env.ctx, env.viewer)

	const targetCount = 12
	for i := 0; i < targetCount; i++ {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "large-mention-target-"+strconv.Itoa(i), "Large Mention Target", "password")
		if err != nil {
			t.Fatalf("CreateUser target %d: %v", i, err)
		}
		if _, err := env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom target %d: %v", i, err)
		}
	}

	assetID := env.uploadAttachmentAsset(t, room.Id, "note.txt", "text/plain", []byte("broad mention upload"))
	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before post: %v", err)
	}
	resp, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:             room.Id,
		Body:               "@all please review this attachment",
		AttachmentAssetIds: []string{assetID},
	}))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	message := resp.Msg.GetMessage()
	if message == nil {
		t.Fatalf("CreateMessage response = %+v, want message", resp.Msg)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after message = %d, want unchanged %d", after, before)
	}
}

func TestMessageServiceCreateMessageValidationPreflightDoesNotCreateAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-validation")
	ctx := withCaller(env.ctx, env.viewer)

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	otherRoom := env.createJoinedRoom("upload-validation-other")
	otherRoomMessage := env.post(otherRoom.Id, env.viewer.Id, "other room", "")

	tests := []struct {
		name string
		req  *apiv1.CreateMessageRequest
		code connect.Code
	}{
		{
			name: "missing thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.CreateMessageRequest{
				RoomId:            room.Id,
				Body:              "reply with file",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: "missing-reply-target",
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "other room in-reply-to target",
			req: &apiv1.CreateMessageRequest{
				RoomId:    room.Id,
				Body:      "reply with file",
				InReplyTo: otherRoomMessage.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "invalid link preview token",
			req: &apiv1.CreateMessageRequest{
				RoomId:             room.Id,
				Body:               "message with bad preview and file",
				AttachmentAssetIds: []string{"missing"},
				LinkPreviewToken:   "not-a-token",
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, err := env.core.GetAssetCount(env.ctx)
			if err != nil {
				t.Fatalf("GetAssetCount before post: %v", err)
			}
			_, err = env.messages.CreateMessage(ctx, connect.NewRequest(tt.req))
			if connect.CodeOf(err) != tt.code {
				t.Fatalf("CreateMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
			after, err := env.core.GetAssetCount(env.ctx)
			if err != nil {
				t.Fatalf("GetAssetCount after post: %v", err)
			}
			if after != before {
				t.Fatalf("asset count after invalid attachment post = %d, want unchanged %d", after, before)
			}
		})
	}
}

func TestMessageServiceCreateMessageRejectsVideoUploadWhenProcessingDisabled(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-upload-video-disabled")
	ctx := withCaller(env.ctx, env.viewer)

	before, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount before video post: %v", err)
	}
	sum := sha256.Sum256([]byte("video"))
	_, err = env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "clip.mp4",
		ContentType: "video/mp4",
		Size:        int64(len("video")),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("video upload CreateUpload code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	after, err := env.core.GetAssetCount(env.ctx)
	if err != nil {
		t.Fatalf("GetAssetCount after video post: %v", err)
	}
	if after != before {
		t.Fatalf("asset count after rejected video = %d, want unchanged %d", after, before)
	}
}

func TestAssetUploadServiceChunkResumeCompleteAndCancel(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-flow")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("first chunk and second chunk")
	first := content[:11]
	second := content[11:]
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	uploadID := created.Msg.GetUpload().GetUploadId()
	if uploadID == "" || created.Msg.GetUpload().GetMaxChunkSize() <= 0 {
		t.Fatalf("created upload = %+v, want id and limits", created.Msg.GetUpload())
	}

	firstSum := sha256.Sum256(first)
	chunkResp, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Content:     first,
		ChunkSha256: hex.EncodeToString(firstSum[:]),
	}))
	if err != nil {
		t.Fatalf("UploadChunk first: %v", err)
	}
	if got := chunkResp.Msg.GetUpload().GetCommittedOffset(); got != int64(len(first)) {
		t.Fatalf("committed offset after first chunk = %d, want %d", got, len(first))
	}
	resume, err := env.assetUploads.GetUpload(ctx, connect.NewRequest(&apiv1.GetUploadRequest{UploadId: uploadID}))
	if err != nil {
		t.Fatalf("GetUpload: %v", err)
	}
	if got := resume.Msg.GetUpload().GetCommittedOffset(); got != int64(len(first)) {
		t.Fatalf("resume committed offset = %d, want %d", got, len(first))
	}

	secondSum := sha256.Sum256(second)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Offset:      int64(len(first)),
		Content:     second,
		ChunkSha256: hex.EncodeToString(secondSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk second: %v", err)
	}
	completed, err := env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{UploadId: uploadID}))
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if completed.Msg.GetUpload().GetStatus() != apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_COMPLETED {
		t.Fatalf("completed upload status = %v, want completed", completed.Msg.GetUpload().GetStatus())
	}
	if completed.Msg.GetAsset().GetId() == "" || completed.Msg.GetAsset().GetFilename() != "note.txt" {
		t.Fatalf("completed asset = %+v, want note.txt asset id", completed.Msg.GetAsset())
	}

	cancelContent := []byte("cancel me")
	cancelSum := sha256.Sum256(cancelContent)
	cancelCreated, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "cancel.txt",
		ContentType: "text/plain",
		Size:        int64(len(cancelContent)),
		Sha256:      hex.EncodeToString(cancelSum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload cancel: %v", err)
	}
	cancelResp, err := env.assetUploads.CancelUpload(ctx, connect.NewRequest(&apiv1.CancelUploadRequest{
		UploadId: cancelCreated.Msg.GetUpload().GetUploadId(),
	}))
	if err != nil {
		t.Fatalf("CancelUpload: %v", err)
	}
	if cancelResp.Msg.GetUpload().GetStatus() != apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_CANCELLED {
		t.Fatalf("cancel status = %v, want cancelled", cancelResp.Msg.GetUpload().GetStatus())
	}
	if _, err := env.assetUploads.GetUpload(ctx, connect.NewRequest(&apiv1.GetUploadRequest{
		UploadId: cancelCreated.Msg.GetUpload().GetUploadId(),
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetUpload after cancel code = %v, want not_found", connect.CodeOf(err))
	}
}

func TestAssetUploadServiceDoesNotRequireThreadPostPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-thread-permission")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("thread attachment")
	sum := sha256.Sum256(content)

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePostInThread); err != nil {
		t.Fatalf("DenyRoomPermission thread post: %v", err)
	}

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "thread.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload with thread posting denied: %v", err)
	}
	if created.Msg.GetUpload().GetUploadId() == "" {
		t.Fatal("CreateUpload upload id is empty")
	}

	_, err = env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId:            room.Id,
		Body:              "thread reply",
		ThreadRootEventId: root.Id,
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreateMessage thread reply code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestAssetUploadServiceCompleteRechecksAttachmentPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("upload-complete-permission")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("attachment permission revoked")
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "revoked.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	chunkSum := sha256.Sum256(content)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    created.Msg.GetUpload().GetUploadId(),
		Content:     content,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageAttach); err != nil {
		t.Fatalf("DenyRoomPermission attach: %v", err)
	}

	_, err = env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: created.Msg.GetUpload().GetUploadId(),
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CompleteUpload after attach permission revoked code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestAssetUploadServiceRejectsChecksumOffsetAndIncompleteComplete(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("asset-upload-validation")
	ctx := withCaller(env.ctx, env.viewer)
	content := []byte("validated content")
	sum := sha256.Sum256(content)

	created, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      room.Id,
		Filename:    "note.txt",
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	uploadID := created.Msg.GetUpload().GetUploadId()
	if _, err := env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: uploadID,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("incomplete CompleteUpload code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Content:     []byte("bad"),
		ChunkSha256: strings.Repeat("0", sha256.Size*2),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad checksum UploadChunk code = %v, want invalid_argument", connect.CodeOf(err))
	}
	chunk := []byte("valid")
	chunkSum := sha256.Sum256(chunk)
	if _, err := env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    uploadID,
		Offset:      1,
		Content:     chunk,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad offset UploadChunk code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestMessageServiceUpdateMessageAuthorAndRBAC(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-update-rbac")
	authorCtx := withCaller(env.ctx, env.viewer)
	original := env.post(room.Id, env.viewer.Id, "original", "")

	if _, err := env.messages.UpdateMessage(env.ctx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    stringPtr("ignored"),
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-outsider", "Message Update Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    stringPtr("ignored"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-other", "Message Update Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, other), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    stringPtr("ignored"),
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member without manage UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	authorResp, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: original.Id,
		Body:    stringPtr("author edit"),
	}))
	if err != nil {
		t.Fatalf("author UpdateMessage: %v", err)
	}
	if authorResp.Msg.GetMessage().GetBody() != "author edit" {
		t.Fatalf("author UpdateMessage response = %+v, want hydrated edited event", authorResp.Msg)
	}
	if body, err := env.core.GetMessageBody(env.ctx, original.Id); err != nil || body != "author edit" {
		t.Fatalf("body after author edit = %q, %v; want author edit, nil", body, err)
	}

	echo := false
	if _, err := env.messages.UpdateMessage(authorCtx, connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:            room.Id,
		EventId:           original.Id,
		Body:              stringPtr("invalid echo edit"),
		AlsoSendToChannel: &echo,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("root echo-state UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	moderator, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-update-moderator", "Message Update Moderator", "password")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom moderator: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, moderator.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission moderator manage: %v", err)
	}
	moderated := env.post(room.Id, env.viewer.Id, "moderated original", "")
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:  room.Id,
		EventId: moderated.Id,
		Body:    stringPtr("moderator edit"),
	})); err != nil {
		t.Fatalf("moderator UpdateMessage: %v", err)
	}
	if body, err := env.core.GetMessageBody(env.ctx, moderated.Id); err != nil || body != "moderator edit" {
		t.Fatalf("body after moderator edit = %q, %v; want moderator edit, nil", body, err)
	}

	echo = true
	if _, err := env.messages.UpdateMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId:            room.Id,
		EventId:           moderated.Id,
		Body:              stringPtr("moderator echo edit"),
		AlsoSendToChannel: &echo,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("moderator echo UpdateMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServiceDeleteMessageAuthorAndRBAC(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-delete-rbac")
	target := env.post(room.Id, env.viewer.Id, "delete target", "")

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-delete-other", "Message Delete Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if _, err := env.messages.DeleteMessage(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: target.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("member without manage DeleteMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	moderator, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-delete-moderator", "Message Delete Moderator", "password")
	if err != nil {
		t.Fatalf("CreateUser moderator: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, moderator.Id, core.KindChannel, moderator.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom moderator: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, moderator.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission moderator manage: %v", err)
	}
	resp, err := env.messages.DeleteMessage(withCaller(env.ctx, moderator), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: target.Id,
	}))
	if err != nil {
		t.Fatalf("moderator DeleteMessage: %v", err)
	}
	if !resp.Msg.Deleted {
		t.Fatal("moderator DeleteMessage Deleted = false, want true")
	}
	if body, err := env.core.GetMessageBody(env.ctx, target.Id); err != nil || body != "" {
		t.Fatalf("body after moderator delete = %q, %v; want empty, nil", body, err)
	}

	own := env.post(room.Id, env.viewer.Id, "own delete target", "")
	if _, err := env.messages.DeleteMessage(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteMessageRequest{
		RoomId:  room.Id,
		EventId: own.Id,
	})); err != nil {
		t.Fatalf("author DeleteMessage: %v", err)
	}
}

func TestMessageServiceDeleteAttachmentAndLinkPreviewAuthorOnly(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-partial-delete")

	attachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "note.txt", "text/plain", bytes.NewReader([]byte("note")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	attachmentEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "with attachment", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage attachment: %v", err)
	}
	previewURL := "https://example.test/preview"
	previewEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "with preview", nil, "", "", &corev1.LinkPreview{
		Url:   previewURL,
		Title: "Preview",
	}, false)
	if err != nil {
		t.Fatalf("CreateMessage preview: %v", err)
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-partial-other", "Message Partial Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, other.Id, core.PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission other manage: %v", err)
	}
	if _, err := env.messages.DeleteAttachment(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteAttachmentRequest{
		RoomId:       room.Id,
		EventId:      attachmentEvent.Id,
		AttachmentId: attachment.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-author DeleteAttachment code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
	if _, err := env.messages.DeleteLinkPreview(withCaller(env.ctx, other), connect.NewRequest(&apiv1.DeleteLinkPreviewRequest{
		RoomId:  room.Id,
		EventId: previewEvent.Id,
		Url:     previewURL,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-author DeleteLinkPreview code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if _, err := env.messages.DeleteAttachment(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteAttachmentRequest{
		RoomId:       room.Id,
		EventId:      attachmentEvent.Id,
		AttachmentId: attachment.Id,
	})); err != nil {
		t.Fatalf("author DeleteAttachment: %v", err)
	}
	body, err := env.core.GetFullMessageBody(env.ctx, attachmentEvent.Id)
	if err != nil {
		t.Fatalf("GetFullMessageBody attachment: %v", err)
	}
	if len(body.Attachments) != 0 {
		t.Fatalf("attachments after delete = %d, want 0", len(body.Attachments))
	}

	if _, err := env.messages.DeleteLinkPreview(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.DeleteLinkPreviewRequest{
		RoomId:  room.Id,
		EventId: previewEvent.Id,
		Url:     previewURL,
	})); err != nil {
		t.Fatalf("author DeleteLinkPreview: %v", err)
	}
	body, err = env.core.GetFullMessageBody(env.ctx, previewEvent.Id)
	if err != nil {
		t.Fatalf("GetFullMessageBody preview: %v", err)
	}
	if body.LinkPreview != nil {
		t.Fatalf("link preview after delete = %+v, want nil", body.LinkPreview)
	}
}

func TestRoomServiceUpdateTypingIndicatorRequiresMembershipOnly(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-typing")
	req := connect.NewRequest(&apiv1.UpdateTypingIndicatorRequest{RoomId: room.Id})

	if _, err := env.rooms.UpdateTypingIndicator(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated UpdateTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-typing-outsider", "Message Typing Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.UpdateTypingIndicator(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("outsider UpdateTypingIndicator code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission post: %v", err)
	}
	resp, err := env.rooms.UpdateTypingIndicator(withCaller(env.ctx, env.viewer), req)
	if err != nil {
		t.Fatalf("member UpdateTypingIndicator with post denied: %v", err)
	}
	if !resp.Msg.Updated {
		t.Fatal("UpdateTypingIndicator Updated = false, want true")
	}
}

func TestRoomAndThreadTimelineGetRoomEventsPaginatesWithOpaqueCursors(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-pagination")

	m1 := env.post(room.Id, env.viewer.Id, "one", "")
	m2 := env.post(room.Id, env.viewer.Id, "two", "")
	m3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  2,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents latest: %v", err)
	}
	page := resp.Msg.GetPage()
	if len(page.Events) != 2 {
		t.Fatalf("latest event count = %d, want 2", len(page.Events))
	}
	if got := []string{page.Events[0].Id, page.Events[1].Id}; got[0] != m2.Id || got[1] != m3.Id {
		t.Fatalf("latest event IDs = %v, want [%s %s]", got, m2.Id, m3.Id)
	}
	if !page.HasOlder || page.HasNewer {
		t.Fatalf("latest page HasOlder/HasNewer = %v/%v, want true/false", page.HasOlder, page.HasNewer)
	}
	if page.StartCursor == "" || page.EndCursor == "" {
		t.Fatalf("cursors = %q/%q, want non-empty cursors", page.StartCursor, page.EndCursor)
	}
	if !strings.HasPrefix(page.StartCursor, roomTimelineCursorOpaquePrefix) || !strings.HasPrefix(page.EndCursor, roomTimelineCursorOpaquePrefix) {
		t.Fatalf("cursors = %q/%q, want opaque cursors", page.StartCursor, page.EndCursor)
	}

	olderResp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
		Cursor: &apiv1.GetRoomEventsRequest_Before{Before: page.StartCursor},
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents before: %v", err)
	}
	if !timelinePageContains(olderResp.Msg.GetPage(), m1.Id) {
		t.Fatalf("older page does not contain first message %s", m1.Id)
	}
	if olderResp.Msg.GetPage().HasNewer != true {
		t.Fatalf("older page HasNewer = false, want true")
	}

	startSeq, err := env.api.parseRoomTimelineCursor(env.viewer.Id, room.Id, "", page.StartCursor)
	if err != nil {
		t.Fatalf("parse emitted start cursor: %v", err)
	}
	if startSeq == 0 {
		t.Fatal("decoded emitted cursor has zero internal position")
	}
}

func TestRoomTimelineCursorFormatIsOpaqueAndVersioned(t *testing.T) {
	env := newConnectAPITestEnv(t)
	cursor, err := env.api.formatRoomTimelineCursor(env.viewer.Id, "room-1", "", 42)
	if err != nil {
		t.Fatalf("formatRoomTimelineCursor: %v", err)
	}
	if cursor == "" {
		t.Fatal("formatRoomTimelineCursor returned empty cursor")
	}
	if !strings.HasPrefix(cursor, roomTimelineCursorOpaquePrefix) || strings.Contains(cursor, "42") {
		t.Fatalf("cursor %q exposes raw sequence", cursor)
	}
	seq, err := env.api.parseRoomTimelineCursor(env.viewer.Id, "room-1", "", cursor)
	if err != nil {
		t.Fatalf("parse opaque cursor: %v", err)
	}
	if seq != 42 {
		t.Fatalf("opaque cursor seq = %d, want 42", seq)
	}
	second, err := env.api.formatRoomTimelineCursor(env.viewer.Id, "room-1", "", 42)
	if err != nil {
		t.Fatalf("format second cursor: %v", err)
	}
	if second == cursor {
		t.Fatal("identical timeline positions produced identical ciphertext")
	}
	envelope, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, roomTimelineCursorOpaquePrefix))
	if err != nil {
		t.Fatalf("decode cursor envelope: %v", err)
	}
	sequenceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sequenceBytes, 42)
	if bytes.Contains(envelope, sequenceBytes) {
		t.Fatal("cursor envelope exposes raw JetStream sequence bytes")
	}
	tamperedEnvelope := append([]byte(nil), envelope...)
	tamperedEnvelope[len(tamperedEnvelope)-1] ^= 1
	tampered := roomTimelineCursorOpaquePrefix + base64.RawURLEncoding.EncodeToString(tamperedEnvelope)
	if _, err := env.api.parseRoomTimelineCursor(env.viewer.Id, "room-1", "", tampered); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("tampered cursor code = %v, want invalid_argument", connect.CodeOf(err))
	}
	for _, invalid := range []string{"bad", "seq:42", "tl:not-base64", "tl:AQ"} {
		if _, err := env.api.parseRoomTimelineCursor(env.viewer.Id, "room-1", "", invalid); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("parse invalid cursor %q code = %v, want invalid_argument", invalid, connect.CodeOf(err))
		}
	}
}

func TestRoomTimelineCursorsAreBoundToViewerAndResource(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("cursor-scope-room")
	otherRoom := env.createJoinedRoom("cursor-scope-other")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	env.post(room.Id, env.viewer.Id, "reply", root.Id)
	env.post(room.Id, env.viewer.Id, "latest", "")

	ctx := withCaller(env.ctx, env.viewer)
	roomPage, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  2,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	roomCursor := roomPage.Msg.GetPage().GetStartCursor()
	if roomCursor == "" {
		t.Fatal("room cursor is empty")
	}

	_, err = env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: otherRoom.Id,
		Limit:  2,
		Cursor: &apiv1.GetRoomEventsRequest_Before{Before: roomCursor},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	_, err = env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             2,
		Cursor:            &apiv1.GetThreadEventsRequest_Before{Before: roomCursor},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	threadPage, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             2,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents: %v", err)
	}
	threadCursor := threadPage.Msg.GetPage().GetEndCursor()
	if threadCursor == "" {
		t.Fatal("thread cursor is empty")
	}
	otherRoot := env.post(room.Id, env.viewer.Id, "other root", "")
	_, err = env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: otherRoot.Id,
		Limit:             2,
		Cursor:            &apiv1.GetThreadEventsRequest_Before{Before: threadCursor},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)

	otherViewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "cursor-other-viewer", "Other Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser other viewer: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, otherViewer.Id, core.KindChannel, otherViewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other viewer: %v", err)
	}
	_, err = env.rooms.GetRoomEvents(withCaller(env.ctx, otherViewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  2,
		Cursor: &apiv1.GetRoomEventsRequest_Before{Before: roomCursor},
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestRoomMessageAndAssetServicesListAttachmentsGetMessagesAndGetAssets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("attachment-list")

	rootAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "root.txt", "text/plain", bytes.NewReader([]byte("root")))
	if err != nil {
		t.Fatalf("UploadAttachment root: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "root file", []string{rootAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage root: %v", err)
	}
	threadAttachment, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "thread.png", "image/png", bytes.NewReader(connectAPITestPNG()))
	if err != nil {
		t.Fatalf("UploadAttachment thread: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "thread file", []string{threadAttachment.Id}, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage reply: %v", err)
	}
	empty, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "no files", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("CreateMessage empty: %v", err)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.rooms.ListRoomAttachments(env.ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 10},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoomAttachments code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	resp, err := env.rooms.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{
		RoomId: room.Id,
		Page:   &apiv1.PageRequest{Limit: 1},
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  120,
			Height: 120,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_COVER,
		},
	}))
	if err != nil {
		t.Fatalf("ListRoomAttachments: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 2 || !resp.Msg.GetPage().GetHasMore() || len(resp.Msg.GetAttachments()) != 1 {
		t.Fatalf("ListRoomAttachments count/hasMore/attachments = %d/%v/%d, want 2/true/1", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore(), len(resp.Msg.GetAttachments()))
	}
	first := resp.Msg.GetAttachments()[0]
	if first.MessageEventId != reply.Id || first.ThreadRootEventId != root.Id {
		t.Fatalf("first message/thread IDs = %q/%q, want %q/%q", first.MessageEventId, first.ThreadRootEventId, reply.Id, root.Id)
	}
	if first.GetAttachment().GetId() != threadAttachment.Id || first.GetAttachment().GetFilename() != "thread.png" {
		t.Fatalf("first attachment = %+v, want thread.png", first.GetAttachment())
	}
	if first.GetAttachment().GetAssetUrl().GetUrl() == "" || first.GetAttachment().GetThumbnailAssetUrl().GetUrl() == "" {
		t.Fatalf("attachment asset URLs missing: %+v", first.GetAttachment())
	}
	if first.GetCreatedAt() == nil {
		t.Fatal("created_at missing")
	}

	get, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	getAttachments := get.Msg.GetMessage().GetAttachments()
	if len(getAttachments) != 1 {
		t.Fatalf("GetMessage attachments = %d, want 1", len(getAttachments))
	}
	fresh := getAttachments[0]
	if fresh.GetId() != threadAttachment.Id {
		t.Fatalf("GetMessage attachment ID = %q, want %q", fresh.GetId(), threadAttachment.Id)
	}
	if fresh.GetAssetUrl().GetUrl() == "" || fresh.GetAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh asset URL missing: %+v", fresh.GetAssetUrl())
	}
	if fresh.GetThumbnailAssetUrl().GetUrl() == "" || fresh.GetThumbnailAssetUrl().GetExpiresAt() == nil {
		t.Fatalf("fresh thumbnail URL missing: %+v", fresh.GetThumbnailAssetUrl())
	}

	asset, err := env.assets.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{
		RoomId:  room.Id,
		AssetId: threadAttachment.Id,
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  64,
			Height: 64,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN,
		},
	}))
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got := asset.Msg.GetAsset().GetThumbnailAssetUrl().GetUrl(); !strings.Contains(got, "/64x64/contain") {
		t.Fatalf("GetAsset thumbnail URL = %q, want 64x64 contain transform", got)
	}

	batch, err := env.messages.BatchGetMessages(ctx, connect.NewRequest(&apiv1.BatchGetMessagesRequest{
		RoomId:   room.Id,
		EventIds: []string{reply.Id, "missing-event", root.Id, reply.Id, empty.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetMessages: %v", err)
	}
	if got := batch.Msg.GetMessages(); len(got) != 3 {
		t.Fatalf("BatchGetMessages messages = %d, want 3", len(got))
	}
	if batch.Msg.Messages[0].GetId() != reply.Id || len(batch.Msg.Messages[0].GetAttachments()) != 1 {
		t.Fatalf("batch first = %+v, want reply with one attachment", batch.Msg.Messages[0])
	}
	if batch.Msg.Messages[1].GetId() != root.Id ||
		len(batch.Msg.Messages[1].GetAttachments()) != 1 ||
		batch.Msg.Messages[1].GetAttachments()[0].GetId() != rootAttachment.Id {
		t.Fatalf("batch second = %+v, want root attachment", batch.Msg.Messages[1])
	}
	if batch.Msg.Messages[2].GetId() != empty.Id || len(batch.Msg.Messages[2].GetAttachments()) != 0 {
		t.Fatalf("batch third = %+v, want empty message with no attachments", batch.Msg.Messages[2])
	}

	assets, err := env.assets.BatchGetAssets(ctx, connect.NewRequest(&apiv1.BatchGetAssetsRequest{
		RoomId:   room.Id,
		AssetIds: []string{threadAttachment.Id, "missing-asset", rootAttachment.Id, threadAttachment.Id},
		Thumbnail: &apiv1.ImageTransformOptions{
			Width:  64,
			Height: 64,
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN,
		},
	}))
	if err != nil {
		t.Fatalf("BatchGetAssets: %v", err)
	}
	if got := assets.Msg.GetAssets(); len(got) != 2 || got[0].GetId() != threadAttachment.Id || got[1].GetId() != rootAttachment.Id {
		t.Fatalf("BatchGetAssets assets = %+v, want thread then root attachments", got)
	}
}

func TestRoomAndThreadTimelineHydratesMessagesWithoutClientNPlusOne(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-hydration")
	replier, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-replier", "Timeline Replier", "password")
	if err != nil {
		t.Fatalf("CreateUser replier: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, replier.Id, core.KindChannel, replier.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom replier: %v", err)
	}

	root := env.post(room.Id, env.viewer.Id, "root", "")
	env.post(room.Id, replier.Id, "reply", root.Id)
	if _, err := env.core.ReactionModel().AddReaction(env.ctx, core.ReactionMutationInput{
		ActorID: env.viewer.Id, RoomID: room.Id, MessageEventID: root.Id, Emoji: "thumbsup",
	}); err != nil {
		t.Fatalf("AddReaction viewer: %v", err)
	}
	if _, err := env.core.ReactionModel().AddReaction(env.ctx, core.ReactionMutationInput{
		ActorID: replier.Id, RoomID: room.Id, MessageEventID: root.Id, Emoji: "thumbsup",
	}); err != nil {
		t.Fatalf("AddReaction replier: %v", err)
	}

	resp, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	event := timelinePageEvent(resp.Msg.GetPage(), root.Id)
	if event == nil {
		t.Fatalf("root message %s not found in page", root.Id)
	}
	payload := event.GetMessagePosted()
	if payload == nil {
		t.Fatalf("root payload = nil, want message_posted")
	}
	message := payload.GetMessage()
	if message.Body == nil || message.GetBody() != "root" {
		t.Fatalf("message body present/body = %v/%q, want true/root", message.Body != nil, message.GetBody())
	}
	thread := message.GetThread()
	if thread.GetReplyCount() != 1 {
		t.Fatalf("reply count = %d, want 1", thread.GetReplyCount())
	}
	if got := thread.GetParticipantCount(); got != 1 {
		t.Fatalf("thread participant count = %d, want 1", got)
	}
	if got := thread.GetParticipantPreviewUserIds(); len(got) != 1 || got[0] != replier.Id {
		t.Fatalf("thread participant preview = %v, want [%s]", got, replier.Id)
	}
	if len(message.Reactions) != 1 {
		t.Fatalf("reaction summaries = %d, want 1", len(message.Reactions))
	}
	reaction := message.Reactions[0]
	if reaction.Emoji != "thumbsup" || reaction.Count != 2 || !reaction.HasReacted {
		t.Fatalf("reaction = %+v, want thumbsup count 2 hasReacted true", reaction)
	}
	if resp.Msg.GetPage().Includes.GetUsers()[env.viewer.Id].DisplayName != "Timeline Viewer" {
		t.Fatalf("viewer include missing or wrong: %+v", resp.Msg.GetPage().Includes.GetUsers()[env.viewer.Id])
	}
	if resp.Msg.GetPage().Includes.GetUsers()[replier.Id].DisplayName != "Timeline Replier" {
		t.Fatalf("replier include missing or wrong: %+v", resp.Msg.GetPage().Includes.GetUsers()[replier.Id])
	}
}

func TestRoomAndThreadTimelineExposeDeletedAt(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-tombstone-retention")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	ctx := withCaller(env.ctx, env.viewer)
	beforeDelete, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents before delete: %v", err)
	}
	if got := timelinePageEvent(beforeDelete.Msg.GetPage(), root.Id).GetMessagePosted().GetMessage().GetDeletedAt(); got != nil {
		t.Fatalf("active root deleted_at = %v, want nil", got)
	}
	if got := timelinePageEvent(beforeDelete.Msg.GetPage(), reply.Id).GetMessagePosted().GetMessage().GetDeletedAt(); got != nil {
		t.Fatalf("active reply deleted_at = %v, want nil", got)
	}

	if err := env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindChannel, room.Id, reply.Id); err != nil {
		t.Fatalf("DeleteMessage reply: %v", err)
	}
	replyState, err := env.core.RoomTimelineReads().MessageHydrationState(reply.Id)
	if err != nil {
		t.Fatalf("MessageHydrationState reply: %v", err)
	}
	if !replyState.HasDeletedAt {
		t.Fatal("reply projection deleted_at is missing")
	}
	replyDeletedAt := replyState.DeletedAt

	thread, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents after delete: %v", err)
	}
	replyMessage := timelinePageEvent(thread.Msg.GetPage(), reply.Id).GetMessagePosted().GetMessage()
	if got := replyMessage.GetDeletedAt(); got == nil || !got.AsTime().Equal(replyDeletedAt) {
		t.Fatalf("deleted reply deleted_at = %v, want %v", got, replyDeletedAt)
	}

	if err := env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindChannel, room.Id, root.Id); err != nil {
		t.Fatalf("DeleteMessage root: %v", err)
	}
	rootState, err := env.core.RoomTimelineReads().MessageHydrationState(root.Id)
	if err != nil {
		t.Fatalf("MessageHydrationState root: %v", err)
	}
	if !rootState.HasDeletedAt {
		t.Fatal("root projection deleted_at is missing")
	}
	rootDeletedAt := rootState.DeletedAt
	afterDelete, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents after delete: %v", err)
	}
	rootMessage := timelinePageEvent(afterDelete.Msg.GetPage(), root.Id).GetMessagePosted().GetMessage()
	if got := rootMessage.GetDeletedAt(); got == nil || !got.AsTime().Equal(rootDeletedAt) {
		t.Fatalf("deleted root deleted_at = %v, want %v", got, rootDeletedAt)
	}
}

func TestThreadTimelineExposesChannelEchoIdentity(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-channel-echo")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "reply", nil, root.Id, root.Id, nil, true)
	if err != nil {
		t.Fatalf("PostMessage reply with echo: %v", err)
	}
	echoID, ok := env.core.ChannelEchoEventID(reply.Id)
	if !ok {
		t.Fatal("expected channel echo for reply")
	}

	resp, err := env.threads.GetThreadEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents: %v", err)
	}
	message := timelinePageEvent(resp.Msg.GetPage(), reply.Id).GetMessagePosted().GetMessage()
	if got := message.GetChannelEchoEventId(); got != echoID {
		t.Fatalf("channel_echo_event_id = %q, want %q", got, echoID)
	}
}

func TestRoomTimelineExposesAccountKeyShredDeletedAt(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-account-key-shred")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-shredded-author", "Timeline Shredded Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	posted := env.post(room.Id, author.Id, "message before account deletion", "")

	if err := env.core.DeleteUser(env.ctx, env.viewer.Id, author.Id); err != nil {
		t.Fatalf("DeleteUser author: %v", err)
	}
	state, err := env.core.RoomTimelineReads().MessageHydrationState(posted.Id)
	if err != nil {
		t.Fatalf("MessageHydrationState account-shredded message: %v", err)
	}
	if !state.HasDeletedAt {
		t.Fatal("projection account-shred deleted_at is missing")
	}
	deletedAt := state.DeletedAt

	resp, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents after account deletion: %v", err)
	}
	timelineEvent := timelinePageEvent(resp.Msg.GetPage(), posted.Id)
	if timelineEvent == nil {
		t.Fatalf("account-shredded message %s missing from timeline", posted.Id)
	}
	message := timelineEvent.GetMessagePosted().GetMessage()
	if message == nil {
		t.Fatal("account-shredded message payload is nil")
	}
	if got := message.GetDeletedAt(); got == nil || !got.AsTime().Equal(deletedAt) {
		t.Fatalf("account-shredded message deleted_at = %v, want %v", got, deletedAt)
	}
}
