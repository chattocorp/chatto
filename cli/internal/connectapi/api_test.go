package connectapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/pb/chatto/api/v1/apiv1connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestAPIHandlers(t *testing.T) {
	api := New(nil, config.ChattoConfig{}, "test")
	handlers := api.Handlers()

	paths := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		if handler.Handler == nil {
			t.Fatalf("handler for %q is nil", handler.ServicePath)
		}
		paths = append(paths, handler.ServicePath)
	}
	sort.Strings(paths)

	want := []string{
		"/" + apiv1connect.MessageServiceName + "/",
		"/" + apiv1connect.NotificationPreferencesServiceName + "/",
		"/" + apiv1connect.ReadStateServiceName + "/",
		"/" + apiv1connect.RoomTimelineServiceName + "/",
		"/" + apiv1connect.ServerServiceName + "/",
		"/" + apiv1connect.ThreadServiceName + "/",
		"/" + apiv1connect.UserStatusServiceName + "/",
	}
	sort.Strings(want)
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("handler paths = %v, want %v", paths, want)
	}
}

func TestPrivateHandlersRequireAuth(t *testing.T) {
	api := New(nil, config.ChattoConfig{}, "test")
	mux := http.NewServeMux()
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := apiv1connect.NewMessageServiceClient(ts.Client(), ts.URL)
	_, err := client.PostMessage(context.Background(), connect.NewRequest(&apiv1.PostMessageRequest{}))
	requireConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestServerServiceGetServerPublicMetadata(t *testing.T) {
	api := New(nil, config.ChattoConfig{
		Auth: config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub provider", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub"},
			},
		},
	}, "9.8.7")
	mux := http.NewServeMux()
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := apiv1connect.NewServerServiceClient(ts.Client(), ts.URL)
	resp, err := client.GetServer(context.Background(), connect.NewRequest(&apiv1.GetServerRequest{}))
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}

	msg := resp.Msg
	if msg.Name != "Chatto" {
		t.Fatalf("Name = %q, want Chatto", msg.Name)
	}
	if msg.Version != "9.8.7" {
		t.Fatalf("Version = %q, want 9.8.7", msg.Version)
	}
	if got, want := strings.Join(msg.AuthMethods, ","), "password,oidc"; got != want {
		t.Fatalf("AuthMethods = %v, want %s", msg.AuthMethods, want)
	}
	if len(msg.AuthProviders) != 1 {
		t.Fatalf("AuthProviders len = %d, want 1", len(msg.AuthProviders))
	}
	provider := msg.AuthProviders[0]
	if provider.Id != "hub provider" {
		t.Fatalf("provider Id = %q, want hub provider", provider.Id)
	}
	if provider.LoginUrl != "/auth/providers/hub%20provider" {
		t.Fatalf("provider LoginUrl = %q, want escaped provider path", provider.LoginUrl)
	}
}

func TestUserStatusServiceSetAndClearCustomStatus(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := auth.WithUser(env.ctx, env.viewer)
	expiresAt := timestamppb.New(time.Now().Add(time.Hour).UTC())

	setResp, err := env.status.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
		Emoji:     "🌿",
		Text:      "In focus mode",
		ExpiresAt: expiresAt,
	}))
	if err != nil {
		t.Fatalf("SetCustomStatus: %v", err)
	}
	if got := setResp.Msg.GetStatus(); got.GetEmoji() != "🌿" || got.GetText() != "In focus mode" {
		t.Fatalf("status = %+v, want focus status", got)
	}
	if got := setResp.Msg.GetStatus().GetExpiresAt(); got == nil || !got.AsTime().Equal(expiresAt.AsTime()) {
		t.Fatalf("ExpiresAt = %v, want %v", got, expiresAt)
	}

	stored, err := env.core.GetUser(ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if stored.GetCustomStatus().GetEmoji() != "🌿" {
		t.Fatalf("stored CustomStatus = %+v, want set status", stored.GetCustomStatus())
	}

	_, err = env.status.SetCustomStatus(ctx, connect.NewRequest(&apiv1.SetCustomStatusRequest{
		Emoji: "🌿",
		Text:  "   ",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SetCustomStatus blank text error = %v, want InvalidArgument", err)
	}

	clearResp, err := env.status.ClearCustomStatus(ctx, connect.NewRequest(&apiv1.ClearCustomStatusRequest{}))
	if err != nil {
		t.Fatalf("ClearCustomStatus: %v", err)
	}
	if clearResp.Msg.GetStatus() != nil {
		t.Fatalf("cleared status = %+v, want nil", clearResp.Msg.GetStatus())
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

func TestRoomTimelineServiceRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "timeline-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	req := connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id})
	if _, err := env.timeline.GetRoomEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "timeline-outsider", "Timeline Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.timeline.GetRoomEvents(auth.WithUser(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetRoomEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServicePostMessageRequiresAuthMembershipAndPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-authz")
	req := connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "hello",
	})

	if _, err := env.messages.PostMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-outsider", "Message Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.PostMessage(auth.WithUser(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if _, err := env.messages.PostMessage(auth.WithUser(env.ctx, env.viewer), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("denied PostMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestMessageServicePostMessageValidatesInput(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-validation")
	ctx := auth.WithUser(env.ctx, env.viewer)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	tests := []struct {
		name string
		req  *apiv1.PostMessageRequest
		code connect.Code
	}{
		{
			name: "missing room",
			req:  &apiv1.PostMessageRequest{Body: "hello"},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "empty body and no attachments",
			req:  &apiv1.PostMessageRequest{RoomId: room.Id, Body: "   "},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "channel echo outside thread",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "hello",
				AlsoSendToChannel: true,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "missing thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: "missing-thread-root",
			},
			code: connect.CodeNotFound,
		},
		{
			name: "thread reply as thread root",
			req: &apiv1.PostMessageRequest{
				RoomId:            room.Id,
				Body:              "reply",
				ThreadRootEventId: reply.Id,
			},
			code: connect.CodeInvalidArgument,
		},
		{
			name: "link preview URL too long",
			req: &apiv1.PostMessageRequest{
				RoomId: room.Id,
				Body:   "hello",
				LinkPreview: &apiv1.MessageLinkPreviewInput{
					Url: strings.Repeat("x", core.MaxLinkPreviewURLLength+1),
				},
			},
			code: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := env.messages.PostMessage(ctx, connect.NewRequest(tt.req)); connect.CodeOf(err) != tt.code {
				t.Fatalf("PostMessage code = %v, want %v", connect.CodeOf(err), tt.code)
			}
		})
	}
}

func TestMessageServicePostMessageInfersVideoProcessingAssetIDs(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-video")

	original, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("original video")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}

	if _, err := env.messages.PostMessage(auth.WithUser(env.ctx, env.viewer), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId:             room.Id,
		AttachmentAssetIds: []string{original.Id},
	})); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	manifest, ok := env.core.Assets.VideoAttachmentManifest(original.Id)
	if !ok || manifest.Started == nil {
		t.Fatalf("VideoAttachmentManifest = %+v, %v; want started", manifest, ok)
	}
}

func TestMessageServicePostMessageReturnsRenderableTimelineEvent(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("message-post-success")

	resp, err := env.messages.PostMessage(auth.WithUser(env.ctx, env.viewer), connect.NewRequest(&apiv1.PostMessageRequest{
		RoomId: room.Id,
		Body:   "hello over connect",
	}))
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	event := resp.Msg.GetEvent()
	if event == nil {
		t.Fatalf("PostMessage event = nil, response = %+v", resp.Msg)
	}
	message := event.GetMessagePosted()
	if message == nil {
		t.Fatalf("PostMessage payload = %T, want message_posted", event.GetEvent())
	}
	if message.Body == nil || message.GetBody() != "hello over connect" {
		t.Fatalf("message body = %q present=%v, want posted body", message.GetBody(), message.Body != nil)
	}
	if got := resp.Msg.GetIncludes().GetUsers()[env.viewer.Id]; got == nil || got.DisplayName != env.viewer.DisplayName {
		t.Fatalf("included viewer = %+v, want %q", got, env.viewer.DisplayName)
	}
}

func TestRoomTimelineServiceGetRoomEventsPaginatesWithOpaqueCursors(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-pagination")

	m1 := env.post(room.Id, env.viewer.Id, "one", "")
	m2 := env.post(room.Id, env.viewer.Id, "two", "")
	m3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := auth.WithUser(env.ctx, env.viewer)
	resp, err := env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	if !strings.HasPrefix(page.StartCursor, roomTimelineCursorSeqPrefix) || !strings.HasPrefix(page.EndCursor, roomTimelineCursorSeqPrefix) {
		t.Fatalf("cursors = %q/%q, want seq-prefixed cursors", page.StartCursor, page.EndCursor)
	}

	olderResp, err := env.timeline.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
}

func TestRoomTimelineServiceHydratesMessagesWithoutClientNPlusOne(t *testing.T) {
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
	if _, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, root.Id, "thumbsup", env.viewer.Id); err != nil {
		t.Fatalf("AddReaction viewer: %v", err)
	}
	if _, err := env.core.AddReaction(env.ctx, core.KindChannel, room.Id, root.Id, "thumbsup", replier.Id); err != nil {
		t.Fatalf("AddReaction replier: %v", err)
	}

	resp, err := env.timeline.GetRoomEvents(auth.WithUser(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	if payload.Body == nil || payload.GetBody() != "root" {
		t.Fatalf("message body present/body = %v/%q, want true/root", payload.Body != nil, payload.GetBody())
	}
	if payload.ReplyCount != 1 {
		t.Fatalf("reply count = %d, want 1", payload.ReplyCount)
	}
	if got := payload.ThreadParticipantUserIds; len(got) != 1 || got[0] != replier.Id {
		t.Fatalf("thread participants = %v, want [%s]", got, replier.Id)
	}
	if len(payload.Reactions) != 1 {
		t.Fatalf("reaction summaries = %d, want 1", len(payload.Reactions))
	}
	reaction := payload.Reactions[0]
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

func TestRoomTimelineServiceGetThreadEventsIncludesRootAndPaginatesReplies(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := auth.WithUser(env.ctx, env.viewer)
	resp, err := env.timeline.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             2,
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents latest: %v", err)
	}
	page := resp.Msg.GetPage()
	gotIDs := timelinePageEventIDs(page)
	wantIDs := []string{root.Id, reply2.Id, reply3.Id}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("thread latest event IDs = %v, want %v", gotIDs, wantIDs)
	}
	if !page.HasOlder || page.HasNewer {
		t.Fatalf("thread latest HasOlder/HasNewer = %v/%v, want true/false", page.HasOlder, page.HasNewer)
	}
	if page.StartCursor == "" || page.EndCursor == "" {
		t.Fatalf("thread reply cursors are empty, want reply-window cursors")
	}

	olderResp, err := env.timeline.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             10,
		Cursor:            &apiv1.GetThreadEventsRequest_Before{Before: page.StartCursor},
	}))
	if err != nil {
		t.Fatalf("GetThreadEvents before: %v", err)
	}
	olderIDs := timelinePageEventIDs(olderResp.Msg.GetPage())
	wantOlderIDs := []string{reply1.Id}
	if strings.Join(olderIDs, ",") != strings.Join(wantOlderIDs, ",") {
		t.Fatalf("thread older event IDs = %v, want %v", olderIDs, wantOlderIDs)
	}
	if olderResp.Msg.GetPage().HasOlder {
		t.Fatalf("older thread page HasOlder = true, want false")
	}
}

func TestRoomTimelineServiceGetThreadEventsAroundRootAndReply(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-around")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := auth.WithUser(env.ctx, env.viewer)
	rootResp, err := env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           root.Id,
		Limit:             2,
	}))
	if err != nil {
		t.Fatalf("GetThreadEventsAround root: %v", err)
	}
	rootIDs := timelinePageEventIDs(rootResp.Msg.GetPage())
	wantRootIDs := []string{root.Id, reply1.Id, reply2.Id}
	if strings.Join(rootIDs, ",") != strings.Join(wantRootIDs, ",") {
		t.Fatalf("root-anchored thread IDs = %v, want %v", rootIDs, wantRootIDs)
	}
	if rootResp.Msg.TargetIndex != 0 {
		t.Fatalf("root target index = %d, want 0", rootResp.Msg.TargetIndex)
	}
	if rootResp.Msg.GetPage().HasOlder || !rootResp.Msg.GetPage().HasNewer {
		t.Fatalf("root-anchored HasOlder/HasNewer = %v/%v, want false/true", rootResp.Msg.GetPage().HasOlder, rootResp.Msg.GetPage().HasNewer)
	}

	replyResp, err := env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           reply2.Id,
		Limit:             3,
	}))
	if err != nil {
		t.Fatalf("GetThreadEventsAround reply: %v", err)
	}
	replyIDs := timelinePageEventIDs(replyResp.Msg.GetPage())
	wantReplyIDs := []string{root.Id, reply1.Id, reply2.Id, reply3.Id}
	if strings.Join(replyIDs, ",") != strings.Join(wantReplyIDs, ",") {
		t.Fatalf("reply-anchored thread IDs = %v, want %v", replyIDs, wantReplyIDs)
	}
	if replyResp.Msg.TargetIndex != 2 {
		t.Fatalf("reply target index = %d, want 2", replyResp.Msg.TargetIndex)
	}

	_, err = env.timeline.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           "missing-anchor",
		Limit:             3,
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("missing anchor code = %v, want %v", got, connect.CodeNotFound)
	}
}

func TestRoomTimelineServiceGetThreadEventsRequiresMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-authz")
	root := env.post(room.Id, env.viewer.Id, "root", "")

	req := connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.timeline.GetThreadEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-outsider", "Thread Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.timeline.GetThreadEvents(auth.WithUser(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestRoomTimelineServiceHydratesProcessedVideoAttachments(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-video")

	original, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("original video")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}
	thumbnail, err := env.core.UploadDerivativeAttachment(env.ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "clip.thumbnail", "application/octet-stream", bytes.NewReader([]byte("thumbnail")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment thumbnail: %v", err)
	}
	variant, err := env.core.UploadDerivativeAttachment(env.ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, room.Id, "clip-720p.mp4", "video/mp4", bytes.NewReader([]byte("variant video")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment variant: %v", err)
	}

	event, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "video", []string{original.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.RecordAssetProcessed(env.ctx, core.SystemActorID, core.KindChannel, room.Id, event.Id, original.Id, 1234, 1280, 720, thumbnail, []*corev1.VideoVariant{
		{
			AttachmentId: variant.Id,
			Quality:      "720p",
			Width:        1280,
			Height:       720,
			Size:         variant.Size,
			Attachment:   variant,
		},
	}); err != nil {
		t.Fatalf("RecordAssetProcessed: %v", err)
	}

	resp, err := env.timeline.GetRoomEvents(auth.WithUser(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: room.Id,
		Limit:  10,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	apiEvent := timelinePageEvent(resp.Msg.GetPage(), event.Id)
	if apiEvent == nil || apiEvent.GetMessagePosted() == nil {
		t.Fatalf("message event %s not found in page", event.Id)
	}
	attachments := apiEvent.GetMessagePosted().GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	processing := attachments[0].GetVideoProcessing()
	if processing == nil {
		t.Fatal("videoProcessing = nil, want completed manifest")
	}
	if processing.GetStatus() != apiv1.RoomTimelineVideoProcessingStatus_ROOM_TIMELINE_VIDEO_PROCESSING_STATUS_COMPLETED {
		t.Fatalf("videoProcessing status = %v, want COMPLETED", processing.GetStatus())
	}
	if processing.GetDurationMs() != 1234 || processing.GetWidth() != 1280 || processing.GetHeight() != 720 {
		t.Fatalf("videoProcessing dimensions = %d/%d/%d, want 1234/1280/720", processing.GetDurationMs(), processing.GetWidth(), processing.GetHeight())
	}
	if processing.GetThumbnailAssetUrl().GetUrl() == "" {
		t.Fatal("videoProcessing thumbnail URL is empty")
	}
	if len(processing.GetVariants()) != 1 || processing.GetVariants()[0].GetQuality() != "720p" {
		t.Fatalf("videoProcessing variants = %+v, want one 720p variant", processing.GetVariants())
	}
	if processing.GetVariants()[0].GetAssetUrl().GetUrl() == "" {
		t.Fatal("videoProcessing variant URL is empty")
	}
}

func TestRoomTimelineHydratorRejectsUnsupportedEvents(t *testing.T) {
	env := newConnectAPITestEnv(t)
	h := &timelineHydrator{
		api:      env.api,
		ctx:      env.ctx,
		viewerID: env.viewer.Id,
		kind:     core.KindChannel,
		userIDs:  make(map[string]struct{}),
	}

	_, err := h.event(&core.RoomEvent{Event: &corev1.Event{
		Id:      "Eunsupported",
		ActorId: env.viewer.Id,
		Event: &corev1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{RoomId: "Runsupported"},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported room timeline event") {
		t.Fatalf("unsupported event error = %v, want unsupported room timeline event", err)
	}
}

func TestRoomTimelineHydratorSupportsVisibleCoreEvents(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-visible-events")
	posted := env.post(room.Id, env.viewer.Id, "visible root", "")

	h := &timelineHydrator{
		api:      env.api,
		ctx:      env.ctx,
		viewerID: env.viewer.Id,
		kind:     core.KindChannel,
		userIDs:  make(map[string]struct{}),
	}

	tests := []struct {
		name  string
		event *corev1.Event
	}{
		{
			name:  "message posted",
			event: posted,
		},
		{
			name: "room created",
			event: &corev1.Event{
				Id:      "Eroom-created",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomCreated{
					RoomCreated: &corev1.RoomCreatedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room updated",
			event: &corev1.Event{
				Id:      "Eroom-updated",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomUpdated{
					RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room deleted",
			event: &corev1.Event{
				Id:      "Eroom-deleted",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomDeleted{
					RoomDeleted: &corev1.RoomDeletedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room archived",
			event: &corev1.Event{
				Id:      "Eroom-archived",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomArchived{
					RoomArchived: &corev1.RoomArchivedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "room unarchived",
			event: &corev1.Event{
				Id:      "Eroom-unarchived",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomUnarchived{
					RoomUnarchived: &corev1.RoomUnarchivedEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "user joined room",
			event: &corev1.Event{
				Id:      "Euser-joined",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_UserJoinedRoom{
					UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: room.Id},
				},
			},
		},
		{
			name: "user left room",
			event: &corev1.Event{
				Id:      "Euser-left",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_UserLeftRoom{
					UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: room.Id},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !core.IsVisibleRoomTimelineEntry(tt.event) {
				t.Fatalf("test event is not visible according to core")
			}
			if _, err := h.event(&core.RoomEvent{Event: tt.event}); err != nil {
				t.Fatalf("hydrate visible event: %v", err)
			}
		})
	}
}

func TestReadStateServiceRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-authz")

	req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{RoomId: room.Id})
	if _, err := env.readState.MarkRoomAsRead(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-outsider", "Read State Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.readState.MarkRoomAsRead(auth.WithUser(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestReadStateServiceMarkRoomAsReadAnchorsAndDoesNotRegress(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-room")

	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-reader", "Read State Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, reader.Id, core.KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	e1 := env.post(room.Id, env.viewer.Id, "one", "")
	if err := env.core.SetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id, e1.Id); err != nil {
		t.Fatalf("seed read marker: %v", err)
	}
	e2 := env.post(room.Id, env.viewer.Id, "two", "")
	e3 := env.post(room.Id, env.viewer.Id, "three", "")

	ctx := auth.WithUser(env.ctx, reader)
	resp, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e2.Id,
	}))
	if err != nil {
		t.Fatalf("MarkRoomAsRead e2: %v", err)
	}
	if resp.Msg.LastReadAt == nil || resp.Msg.PreviousLastReadAt == nil {
		t.Fatalf("timestamps = last %v previous %v, want both set", resp.Msg.LastReadAt, resp.Msg.PreviousLastReadAt)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after e2 = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead stale e1: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after stale e1 = %q, %v; want %s", got, err, e2.Id)
	}

	reply := env.post(room.Id, env.viewer.Id, "reply", e2.Id)
	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkRoomAsRead reply anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after reply anchor = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after missing event = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId: room.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead omitted anchor: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e3.Id {
		t.Fatalf("marker after omitted anchor = %q, %v; want %s", got, err, e3.Id)
	}
}

func TestReadStateServiceMarkRoomAsReadRejectsMissingAnchorWithoutLazyMarker(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "read-state-universal", "", core.WithUniversalRoom(true))
	if err != nil {
		t.Fatalf("CreateRoom universal: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}
	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-lazy-reader", "Read State Lazy Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}

	e1 := env.post(room.Id, env.viewer.Id, "one", "")
	e2 := env.post(room.Id, env.viewer.Id, "two", "")
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker before request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	ctx := auth.WithUser(env.ctx, reader)
	if _, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker after rejected request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	resp, err := env.readState.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	}))
	if err != nil {
		t.Fatalf("MarkRoomAsRead e1 after rejected request: %v", err)
	}
	if resp.Msg.PreviousLastReadAt != nil {
		t.Fatalf("PreviousLastReadAt after missing marker = %v, want nil", resp.Msg.PreviousLastReadAt)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e1.Id {
		t.Fatalf("marker after valid e1 = %q, %v; want %s", got, err, e1.Id)
	}
	if e2.Id == e1.Id {
		t.Fatal("test setup posted duplicate event IDs")
	}
}

func TestReadStateServiceMarkThreadAsReadAnchorsAndDoesNotRegress(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-thread")

	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-thread-reader", "Read State Thread Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, reader.Id, core.KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)

	ctx := auth.WithUser(env.ctx, reader)
	resp, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       reply2.Id,
	}))
	if err != nil {
		t.Fatalf("MarkThreadAsRead reply2: %v", err)
	}
	if resp.Msg.PreviousReadAt != nil {
		t.Fatalf("first previous read at = %v, want nil", resp.Msg.PreviousReadAt)
	}
	marker2, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after reply2: %v", err)
	}

	resp, err = env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       reply1.Id,
	}))
	if err != nil {
		t.Fatalf("MarkThreadAsRead stale reply1: %v", err)
	}
	if resp.Msg.PreviousReadAt == nil {
		t.Fatalf("second previous read at = nil, want previous marker")
	}
	markerAfter, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after stale reply1: %v", err)
	}
	if !markerAfter.Equal(marker2) {
		t.Fatalf("thread marker regressed from %v to %v", marker2, markerAfter)
	}

	otherRoot := env.post(room.Id, env.viewer.Id, "other root", "")
	otherReply := env.post(room.Id, env.viewer.Id, "other reply", otherRoot.Id)
	if _, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       otherReply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkThreadAsRead cross-thread anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	markerAfterInvalid, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after invalid anchor: %v", err)
	}
	if !markerAfterInvalid.Equal(marker2) {
		t.Fatalf("thread marker changed after invalid anchor from %v to %v", marker2, markerAfterInvalid)
	}

	if _, err := env.readState.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		UpToEventId:       "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkThreadAsRead missing anchor code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	markerAfterMissing, err := env.core.GetThreadLastOpened(env.ctx, core.KindChannel, reader.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetThreadLastOpened after missing anchor: %v", err)
	}
	if !markerAfterMissing.Equal(marker2) {
		t.Fatalf("thread marker changed after missing anchor from %v to %v", marker2, markerAfterMissing)
	}
}

func TestThreadServiceRequiresMembershipAndTogglesFollowState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("thread-follow")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	req := connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.threads.FollowThread(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-follow-outsider", "Thread Follow Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.threads.FollowThread(auth.WithUser(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := auth.WithUser(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: "missing-root",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing root FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("reply root FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}

	followResp, err := env.threads.FollowThread(ctx, req)
	if err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if !followResp.Msg.Following {
		t.Fatalf("FollowThread following = false, want true")
	}
	isFollowing, err := env.core.IsFollowingThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("IsFollowingThread after follow: %v", err)
	}
	if !isFollowing {
		t.Fatalf("core follow state = false, want true")
	}

	unfollowResp, err := env.threads.UnfollowThread(ctx, connect.NewRequest(&apiv1.UnfollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	}))
	if err != nil {
		t.Fatalf("UnfollowThread: %v", err)
	}
	if unfollowResp.Msg.Following {
		t.Fatalf("UnfollowThread following = true, want false")
	}
	isFollowing, err = env.core.IsFollowingThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("IsFollowingThread after unfollow: %v", err)
	}
	if isFollowing {
		t.Fatalf("core follow state = true, want false")
	}
}

func TestNotificationLevelMapping(t *testing.T) {
	valid := []struct {
		name string
		api  apiv1.NotificationLevel
		core corev1.NotificationLevel
	}{
		{"default clears core override", apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED},
		{"muted", apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED},
		{"normal", apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL},
		{"all messages", apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := apiNotificationLevelToCore(tt.api)
			if err != nil {
				t.Fatalf("apiNotificationLevelToCore(%v) returned error: %v", tt.api, err)
			}
			if got != tt.core {
				t.Fatalf("apiNotificationLevelToCore(%v) = %v, want %v", tt.api, got, tt.core)
			}
		})
	}

	invalid := []struct {
		name string
		api  apiv1.NotificationLevel
	}{
		{"unspecified is not user intent", apiv1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED},
		{"unknown enum", apiv1.NotificationLevel(99)},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := apiNotificationLevelToCore(tt.api)
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("apiNotificationLevelToCore(%v) error code = %v, want %v", tt.api, got, connect.CodeInvalidArgument)
			}
		})
	}

	if got := coreNotificationLevelToAPI(corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED); got != apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
		t.Fatalf("core unspecified maps to %v, want DEFAULT", got)
	}
}

func TestConnectErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code connect.Code
	}{
		{"not authenticated", core.ErrNotAuthenticated, connect.CodeUnauthenticated},
		{"permission denied", core.ErrPermissionDenied, connect.CodePermissionDenied},
		{"not room member", core.ErrNotRoomMember, connect.CodePermissionDenied},
		{"core not found", core.ErrNotFound, connect.CodeNotFound},
		{"message not found", core.ErrMessageNotFound, connect.CodeNotFound},
		{"jetstream key not found", jetstream.ErrKeyNotFound, connect.CodeNotFound},
		{"message too long", core.ErrMessageTooLong, connect.CodeInvalidArgument},
		{"invalid argument", core.ErrInvalidArgument, connect.CodeInvalidArgument},
		{"string length", &core.StringLengthError{Field: "field", Max: 10}, connect.CodeInvalidArgument},
		{"room archived", core.ErrRoomArchived, connect.CodeFailedPrecondition},
		{"unknown", errors.New("boom"), connect.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connect.CodeOf(connectError(tt.err)); got != tt.code {
				t.Fatalf("connectError code = %v, want %v", got, tt.code)
			}
		})
	}

	if err := connectError(errors.New("boom")); strings.Contains(err.Error(), "boom") {
		t.Fatalf("connectError leaked internal error: %v", err)
	}
}

func requireConnectCode(t testing.TB, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("connect code = %v, want %v (err = %v)", got, want, err)
	}
}

type connectAPITestEnv struct {
	ctx       context.Context
	core      *core.ChattoCore
	nc        *nats.Conn
	api       *API
	messages  *messageService
	readState *readStateService
	timeline  *roomTimelineService
	status    *userStatusService
	threads   *threadService
	viewer    *corev1.User
}

func newConnectAPITestEnv(t *testing.T) *connectAPITestEnv {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startConnectAPITestCore(t, c)

	viewer, err := c.CreateUser(ctx, core.SystemActorID, "timeline-viewer", "Timeline Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	api := New(c, config.ChattoConfig{}, "test")
	return &connectAPITestEnv{
		ctx:       ctx,
		core:      c,
		nc:        nc,
		api:       api,
		messages:  &messageService{api: api},
		readState: &readStateService{api: api},
		timeline:  &roomTimelineService{api: api},
		status:    &userStatusService{api: api},
		threads:   &threadService{api: api},
		viewer:    viewer,
	}
}

func startConnectAPITestCore(t *testing.T, c *core.ChattoCore) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func (e *connectAPITestEnv) createJoinedRoom(name string) *corev1.Room {
	room, err := e.core.CreateRoom(e.ctx, e.viewer.Id, core.KindChannel, "", name, "")
	if err != nil {
		panic(err)
	}
	if _, err := e.core.JoinRoom(e.ctx, e.viewer.Id, core.KindChannel, e.viewer.Id, room.Id); err != nil {
		panic(err)
	}
	return room
}

func (e *connectAPITestEnv) post(roomID, actorID, body, inReplyTo string) *corev1.Event {
	event, err := e.core.PostMessage(e.ctx, core.KindChannel, roomID, actorID, body, nil, inReplyTo, "", nil, false)
	if err != nil {
		panic(err)
	}
	return event
}

func timelinePageContains(page *apiv1.RoomTimelinePage, eventID string) bool {
	return timelinePageEvent(page, eventID) != nil
}

func timelinePageEvent(page *apiv1.RoomTimelinePage, eventID string) *apiv1.RoomTimelineEvent {
	for _, event := range page.Events {
		if event.Id == eventID {
			return event
		}
	}
	return nil
}

func timelinePageEventIDs(page *apiv1.RoomTimelinePage) []string {
	ids := make([]string, 0, len(page.Events))
	for _, event := range page.Events {
		ids = append(ids, event.Id)
	}
	return ids
}
