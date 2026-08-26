package connectapi

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomTimelineKeepsDMReadableWhenMessageBodyCannotHydrate(t *testing.T) {
	env := newConnectAPITestEnv(t)
	testCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx := withCaller(testCtx, env.viewer)

	participant, err := env.core.CreateUser(testCtx, core.SystemActorID, "timeline-dm-corrupt", "Timeline DM Corrupt", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	start, err := env.rooms.StartDM(ctx, connect.NewRequest(&apiv1.StartDMRequest{
		ParticipantIds: []string{participant.Id},
	}))
	if err != nil {
		t.Fatalf("StartDM: %v", err)
	}
	dm := start.Msg.GetRoom()

	bad, err := env.core.PostMessage(testCtx, core.KindDM, dm.Id, env.viewer.Id, "body that will be superseded", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage bad: %v", err)
	}
	corruptMessageBody(t, testCtx, env, dm.Id, bad.Id, env.viewer.Id)
	if _, err := env.core.GetFullMessageBody(testCtx, bad.Id); !errors.Is(err, core.ErrMessageBodyCorrupt) {
		t.Fatalf("corrupt message body hydration error = %v, want ErrMessageBodyCorrupt", err)
	}

	goodResp, err := env.messages.CreateMessage(ctx, connect.NewRequest(&apiv1.CreateMessageRequest{
		RoomId: dm.Id,
		Body:   "good DM message",
	}))
	if err != nil {
		t.Fatalf("CreateMessage good: %v", err)
	}
	good := goodResp.Msg.GetMessage()
	if good == nil {
		t.Fatalf("CreateMessage good message = nil")
	}

	resp, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{
		RoomId: dm.Id,
		Limit:  20,
	}))
	if err != nil {
		t.Fatalf("GetRoomEvents with corrupt body: %v", err)
	}

	badTimelineEvent := timelinePageEvent(resp.Msg.GetPage(), bad.Id)
	if badTimelineEvent == nil {
		t.Fatalf("corrupt message %s missing from timeline", bad.Id)
	}
	badMessage := badTimelineEvent.GetMessagePosted().GetMessage()
	if badMessage == nil {
		t.Fatalf("corrupt message payload = nil")
	}
	if badMessage.Body != nil {
		t.Fatalf("corrupt message body present = %q, want unavailable body", badMessage.GetBody())
	}
	if badMessage.GetDeletedAt() != nil {
		t.Fatalf("corrupt non-deleted message deleted_at = %v, want nil", badMessage.GetDeletedAt())
	}

	goodTimelineEvent := timelinePageEvent(resp.Msg.GetPage(), good.GetId())
	if goodTimelineEvent == nil {
		t.Fatalf("good message %s missing from timeline", good.GetId())
	}
	goodMessage := goodTimelineEvent.GetMessagePosted().GetMessage()
	if goodMessage == nil || goodMessage.GetBody() != "good DM message" {
		t.Fatalf("good message = %+v, want body", goodMessage)
	}
}

func TestRoomTimelineBodyHydrationPropagatesRequestErrors(t *testing.T) {
	env := newConnectAPITestEnv(t)

	room := env.createJoinedRoom("timeline-body-canceled")
	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "body should require hydration", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	ctx, cancel := context.WithCancel(env.ctx)
	cancel()

	h := &timelineHydrator{
		api:                  env.api,
		kind:                 core.KindChannel,
		reactionsByMessageID: make(map[string][]core.ReactionSummary),
		userIDs:              make(map[string]struct{}),
	}
	_, err = h.messagePosted(ctx, &core.RoomEvent{Event: posted}, posted.GetMessagePosted())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("messagePosted error = %v, want context cancellation", err)
	}
}

func corruptMessageBody(t *testing.T, ctx context.Context, env *connectAPITestEnv, roomID, eventID, authorID string) {
	t.Helper()

	bodyEventID := core.NewEventID()
	bodyEvent := &corev1.Event{
		Id:        bodyEventID,
		ActorId:   authorID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessageBody{
			MessageBody: &corev1.MessageBodyEvent{
				RoomId:  roomID,
				EventId: eventID,
				Body: &corev1.MessageBody{
					CreatedAt:         timestamppb.Now(),
					AuthorId:          authorID,
					EncryptedBody:     []byte("not a valid ciphertext"),
					EncryptionNonce:   []byte("bad nonce"),
					EncryptionVersion: encryption.EnvelopeVersionV2,
					ContentKeyEpoch:   1,
					BodyEventId:       bodyEventID,
				},
			},
		},
	}
	subject := evtstream.RoomAggregate(roomID).SubjectFor(bodyEvent)
	seq, err := env.core.EventPublisher.AppendEventually(ctx, subject, bodyEvent)
	if err != nil {
		t.Fatalf("Append corrupt MessageBodyEvent: %v", err)
	}
	if err := env.core.WaitForProjectionsCurrent(ctx); err != nil {
		t.Fatalf("WaitForProjectionsCurrent after corrupt MessageBodyEvent at sequence %d: %v", seq, err)
	}
}

func TestRoomAndThreadTimelineGetThreadEventsIncludesRootAndPaginatesReplies(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	resp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
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

	olderResp, err := env.threads.GetThreadEvents(ctx, connect.NewRequest(&apiv1.GetThreadEventsRequest{
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

func TestRoomAndThreadTimelineGetThreadEventsAroundRootAndReply(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-around")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply1 := env.post(room.Id, env.viewer.Id, "reply one", root.Id)
	reply2 := env.post(room.Id, env.viewer.Id, "reply two", root.Id)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
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

	replyResp, err := env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
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

	_, err = env.threads.GetThreadEventsAround(ctx, connect.NewRequest(&apiv1.GetThreadEventsAroundRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		EventId:           "missing-anchor",
		Limit:             3,
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("missing anchor code = %v, want %v", got, connect.CodeNotFound)
	}
}

func TestRoomAndThreadTimelineGetMessageForPermalinks(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-message-link")

	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)

	req := connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	})
	if _, err := env.messages.GetMessage(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetMessage code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "message-link-outsider", "Message Link Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.messages.GetMessage(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetMessage code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := withCaller(env.ctx, env.viewer)
	rootResp, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: root.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage root: %v", err)
	}
	rootMessage := rootResp.Msg.GetMessage()
	if rootMessage.GetId() != root.Id || rootMessage.GetThreadRootEventId() != "" {
		t.Fatalf("root message = event %q thread %q, want event %q no thread", rootMessage.GetId(), rootMessage.GetThreadRootEventId(), root.Id)
	}
	if rootMessage.GetBody() != "root" {
		t.Fatalf("root body = %q, want root", rootMessage.GetBody())
	}

	replyResp, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: reply.Id,
	}))
	if err != nil {
		t.Fatalf("GetMessage reply: %v", err)
	}
	replyMessage := replyResp.Msg.GetMessage()
	if replyMessage.GetId() != reply.Id || replyMessage.GetThreadRootEventId() != root.Id {
		t.Fatalf("reply message = event %q thread %q, want event %q thread %q", replyMessage.GetId(), replyMessage.GetThreadRootEventId(), reply.Id, root.Id)
	}

	if _, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{
		RoomId:  room.Id,
		EventId: "missing-anchor",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing message code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
}

func TestRoomAndThreadTimelineGetThreadEventsRequiresMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("timeline-thread-authz")
	root := env.post(room.Id, env.viewer.Id, "root", "")

	req := connect.NewRequest(&apiv1.GetThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})
	if _, err := env.threads.GetThreadEvents(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-outsider", "Thread Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.threads.GetThreadEvents(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member GetThreadEvents code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestTimelineAndAssetServicesHydrateProcessedVideoAttachments(t *testing.T) {
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
		t.Fatalf("CreateMessage: %v", err)
	}
	if err := env.core.RecordAssetProcessedWithHLS(env.ctx, core.SystemActorID, room.Id, event.Id, original.Id, 1234, 1280, 720, thumbnail, []*corev1.VideoVariant{
		{
			AttachmentId: variant.Id,
			Quality:      "720p",
			Width:        1280,
			Height:       720,
			Size:         variant.Size,
			Attachment:   variant,
		},
	}, &corev1.AssetProcessedHLS{Renditions: []*corev1.AssetHLSRendition{{Width: 1280, Height: 720, Bandwidth: 1_000_000, Segments: []*corev1.AssetHLSSegment{{AssetId: "A-segment", DurationMs: 1234}}}}}); err != nil {
		t.Fatalf("RecordAssetProcessedWithHLS: %v", err)
	}

	resp, err := env.rooms.GetRoomEvents(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoomEventsRequest{
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
	attachments := apiEvent.GetMessagePosted().GetMessage().GetAttachments()
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	if got := attachments[0].GetThumbnailAssetUrl().GetUrl(); !strings.Contains(got, "/960x400/contain") {
		t.Fatalf("attachment thumbnail URL = %q, want 960x400 contain transform", got)
	}
	processing := attachments[0].GetVideoProcessing()
	if processing == nil {
		t.Fatal("videoProcessing = nil, want completed manifest")
	}
	if processing.GetStatus() != apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_COMPLETED {
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
	if got := processing.GetHls().GetMasterPlaylistUrl().GetUrl(); !strings.Contains(got, "/assets/hls/"+original.Id+"/master.m3u8?access=") {
		t.Fatalf("videoProcessing HLS master URL = %q", got)
	}

	assetResponse, err := env.assets.GetAsset(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetAssetRequest{
		RoomId:  room.Id,
		AssetId: original.Id,
	}))
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	assetProcessing := assetResponse.Msg.GetAsset().GetVideoProcessing()
	if assetProcessing.GetStatus() != apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_COMPLETED {
		t.Fatalf("asset videoProcessing status = %v, want COMPLETED", assetProcessing.GetStatus())
	}
	if assetProcessing.GetDurationMs() != 1234 || assetProcessing.GetWidth() != 1280 || assetProcessing.GetHeight() != 720 {
		t.Fatalf("asset videoProcessing dimensions = %d/%d/%d, want 1234/1280/720", assetProcessing.GetDurationMs(), assetProcessing.GetWidth(), assetProcessing.GetHeight())
	}
	if assetProcessing.GetThumbnailAssetUrl().GetUrl() == "" || len(assetProcessing.GetVariants()) != 1 || assetProcessing.GetVariants()[0].GetAssetUrl().GetUrl() == "" {
		t.Fatalf("asset videoProcessing derivative URLs missing: %+v", assetProcessing)
	}
	if got := assetProcessing.GetHls().GetMasterPlaylistUrl().GetUrl(); !strings.Contains(got, "/assets/hls/"+original.Id+"/master.m3u8?access=") {
		t.Fatalf("asset videoProcessing HLS master URL = %q", got)
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

	_, err := h.event(env.ctx, &core.RoomEvent{Event: &corev1.Event{
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
			name: "room threading mode changed",
			event: &corev1.Event{
				Id:      "Eroom-threading-mode-changed",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_RoomThreadingModeChanged{
					RoomThreadingModeChanged: &corev1.RoomThreadingModeChangedEvent{
						RoomId: room.Id, ThreadingMode: corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED,
					},
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
		{
			name: "call started",
			event: &corev1.Event{
				Id:      "Ecall-started",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_VoiceCallStarted{
					VoiceCallStarted: &corev1.CallStartedEvent{RoomId: room.Id, CallId: "call-1"},
				},
			},
		},
		{
			name: "call ended",
			event: &corev1.Event{
				Id:      "Ecall-ended",
				ActorId: env.viewer.Id,
				Event: &corev1.Event_VoiceCallEnded{
					VoiceCallEnded: &corev1.CallEndedEvent{RoomId: room.Id, CallId: "call-1"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !core.IsVisibleRoomTimelineEntry(tt.event) {
				t.Fatalf("test event is not visible according to core")
			}
			hydrated, err := h.event(env.ctx, &core.RoomEvent{Event: tt.event})
			if err != nil {
				t.Fatalf("hydrate visible event: %v", err)
			}
			if tt.event.GetRoomThreadingModeChanged() != nil {
				change := hydrated.GetRoomThreadingModeChanged()
				if change.GetRoomId() != room.Id || change.GetThreadingMode() != apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED {
					t.Fatalf("threading mode change = %+v", change)
				}
			}
		})
	}
}

func TestRoomAndThreadServicesRequiresAuthAndMembership(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("read-state-authz")

	req := connect.NewRequest(&apiv1.MarkRoomAsReadRequest{RoomId: room.Id})
	if _, err := env.rooms.MarkRoomAsRead(env.ctx, req); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "read-state-outsider", "Read State Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.rooms.MarkRoomAsRead(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member MarkRoomAsRead code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestRoomAndThreadServicesMarkRoomAsReadAnchorsAndDoesNotRegress(t *testing.T) {
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
	roomMention := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, e1, "", notificationTestSignalDirectMention)
	roomReply := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, e2, "", notificationTestSignalReply)
	futureRoomNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, e3, "", notificationTestSignalFollowedRoom)
	threadRoot := env.post(room.Id, env.viewer.Id, "thread root", "")
	threadReply := env.post(room.Id, env.viewer.Id, "thread reply", threadRoot.Id)
	threadNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, threadReply, threadRoot.Id, notificationTestSignalReply)

	ctx := withCaller(env.ctx, reader)
	resp, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
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
	assertAPINotificationStates(t, env, ctx,
		[]string{futureRoomNotification.Id, threadNotification.Id},
		[]string{roomMention.Id, roomReply.Id},
	)

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: e1.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead stale e1: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after stale e1 = %q, %v; want %s", got, err, e2.Id)
	}
	assertAPINotificationStates(t, env, ctx,
		[]string{futureRoomNotification.Id, threadNotification.Id},
		[]string{roomMention.Id, roomReply.Id},
	)

	reply := env.post(room.Id, env.viewer.Id, "reply", e2.Id)
	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: reply.Id,
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("MarkRoomAsRead reply anchor code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after reply anchor = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != e2.Id {
		t.Fatalf("marker after missing event = %q, %v; want %s", got, err, e2.Id)
	}

	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId: room.Id,
	})); err != nil {
		t.Fatalf("MarkRoomAsRead omitted anchor: %v", err)
	}
	if got, err := env.core.GetLastReadEventID(env.ctx, core.KindChannel, reader.Id, room.Id); err != nil || got != threadRoot.Id {
		t.Fatalf("marker after omitted anchor = %q, %v; want %s", got, err, threadRoot.Id)
	}
	assertAPINotificationStates(t, env, ctx,
		[]string{threadNotification.Id},
		[]string{futureRoomNotification.Id, roomMention.Id, roomReply.Id},
	)
}

func TestRoomAndThreadServicesMarkRoomAsReadRejectsMissingAnchorWithoutLazyMarker(t *testing.T) {
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

	ctx := withCaller(env.ctx, reader)
	if _, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
		RoomId:      room.Id,
		UpToEventId: "missing-event",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("MarkRoomAsRead missing event code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
	if marker, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id); err != nil || exists || marker != "" {
		t.Fatalf("reader marker after rejected request = %q exists=%v err=%v, want absent", marker, exists, err)
	}

	resp, err := env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{
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

func TestRoomAndThreadServicesMarkThreadAsReadAnchorsAndDoesNotRegress(t *testing.T) {
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
	threadReplyNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, reply1, root.Id, notificationTestSignalReply)
	threadMentionNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, reply2, root.Id, notificationTestSignalDirectMention)
	reply3 := env.post(room.Id, env.viewer.Id, "reply three", root.Id)
	futureThreadNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, reply3, root.Id, notificationTestSignalReply)
	otherRoot := env.post(room.Id, env.viewer.Id, "other root", "")
	otherReply := env.post(room.Id, env.viewer.Id, "other reply", otherRoot.Id)
	otherThreadNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, otherReply, otherRoot.Id, notificationTestSignalReply)
	roomNotification := createReadTestOccurrence(t, env, reader.Id, env.viewer.Id, room.Id, root, "", notificationTestSignalDirectMention)

	ctx := withCaller(env.ctx, reader)
	resp, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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
	assertAPINotificationStates(t, env, ctx,
		[]string{futureThreadNotification.Id, otherThreadNotification.Id, roomNotification.Id},
		[]string{threadReplyNotification.Id, threadMentionNotification.Id},
	)

	resp, err = env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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
	assertAPINotificationStates(t, env, ctx,
		[]string{futureThreadNotification.Id, otherThreadNotification.Id, roomNotification.Id},
		[]string{threadReplyNotification.Id, threadMentionNotification.Id},
	)

	if _, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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

	if _, err := env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{
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

func createReadTestOccurrence(t *testing.T, env *connectAPITestEnv, recipientID, actorID, roomID string, event *corev1.Event, threadRootID string, reason notificationTestSignalKind) *corev1.NotificationOccurrence {
	t.Helper()
	sequence, err := env.core.GetEventSequence(env.ctx, core.KindChannel, roomID, event.GetId())
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	target := testNotificationRoomMessageTarget(roomID, event.GetId())
	if threadRootID != "" {
		target.ThreadRootEventId = &threadRootID
	}
	occurrence, _, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
		RecipientID:          recipientID,
		SourceEventID:        event.GetId(),
		SourceCreated:        event.GetCreatedAt().AsTime(),
		SourceStreamSequence: sequence,
		ActorID:              actorID,
		Signal:               testNotificationSignalWithMessage(reason, target),
		Mode:                 corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		AttentionLevel:       corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		InitiallyRead:        false,
		SkipReadLookup:       true,
	})
	if err != nil {
		t.Fatalf("Create notification occurrence: %v", err)
	}
	return occurrence
}

func assertAPINotificationStates(t *testing.T, env *connectAPITestEnv, ctx context.Context, unreadIDs []string, readIDs []string) {
	t.Helper()
	resp, err := env.notifications.ListNotificationOccurrences(ctx, connect.NewRequest(&apiv1.ListNotificationOccurrencesRequest{
		Page: &apiv1.PageRequest{Limit: 100},
	}))
	if err != nil {
		t.Fatalf("ListNotificationOccurrences: %v", err)
	}
	states := map[string]bool{}
	for _, notification := range resp.Msg.GetOccurrences() {
		states[notification.GetId()] = notification.GetUnread()
	}
	for _, id := range unreadIDs {
		if !states[id] {
			t.Fatalf("notification %s state = %v, want unread; got %v", id, states[id], states)
		}
	}
	for _, id := range readIDs {
		if states[id] {
			t.Fatalf("notification %s state = %v, want read; got %v", id, states[id], states)
		}
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
	if _, err := env.threads.FollowThread(withCaller(env.ctx, outsider), req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-member FollowThread code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
	}

	ctx := withCaller(env.ctx, env.viewer)
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
	if state := followResp.Msg.GetState(); state.GetRoomId() != room.Id || state.GetThreadRootEventId() != root.Id || !state.GetFollowing() {
		t.Fatalf("FollowThread state = %+v, want current followed thread", state)
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
	if state := unfollowResp.Msg.GetState(); state.GetRoomId() != room.Id || state.GetThreadRootEventId() != root.Id || state.GetFollowing() {
		t.Fatalf("UnfollowThread state = %+v, want current unfollowed thread", state)
	}
	isFollowing, err = env.core.IsFollowingThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("IsFollowingThread after unfollow: %v", err)
	}
	if isFollowing {
		t.Fatalf("core follow state = true, want false")
	}
}

func TestThreadServiceListFollowedThreadsReturnsHydratedPage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("followed-thread-list")
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-list-participant", "Thread List Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, participant.Id, core.KindChannel, participant.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom participant: %v", err)
	}
	root := env.post(room.Id, env.viewer.Id, "root body", "")
	env.post(room.Id, participant.Id, "reply body", root.Id)

	if _, err := env.threads.ListFollowedThreads(env.ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListFollowedThreads code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}

	resp, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if resp.Msg.GetPage().GetTotalCount() != 1 || resp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListFollowedThreads page metadata = total %d hasMore %v, want total 1 hasMore false", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore())
	}
	if len(resp.Msg.GetThreads()) != 1 {
		t.Fatalf("ListFollowedThreads returned %d threads, want 1", len(resp.Msg.GetThreads()))
	}

	thread := resp.Msg.GetThreads()[0]
	if thread.GetRoom().GetId() != room.Id || thread.GetRoom().GetName() != room.Name || thread.GetThread().GetThreadRootEventId() != root.Id {
		t.Fatalf("followed thread identity = room %q name %q root %q, want room %q name %q root %q", thread.GetRoom().GetId(), thread.GetRoom().GetName(), thread.GetThread().GetThreadRootEventId(), room.Id, room.Name, root.Id)
	}
	if thread.GetThread().GetReplyCount() != 1 || !thread.GetThread().GetViewerState().GetHasUnread() || thread.GetThread().GetLastReplyAt() == nil {
		t.Fatalf("followed thread metadata = replies %d unread %v lastReplyAt %v, want replies 1 unread true lastReplyAt set", thread.GetThread().GetReplyCount(), thread.GetThread().GetViewerState().GetHasUnread(), thread.GetThread().GetLastReplyAt())
	}
	rootMessage := thread.GetRootMessage()
	if rootMessage == nil || rootMessage.GetId() != root.Id {
		t.Fatalf("root message = %+v, want hydrated message %s", rootMessage, root.Id)
	}
	if got := rootMessage.GetBody(); got != "root body" {
		t.Fatalf("root message body = %q, want root body", got)
	}
	users := resp.Msg.GetIncludes().GetUsers()
	if users[env.viewer.Id] == nil || users[participant.Id] == nil {
		t.Fatalf("includes users missing viewer or participant: got %d included users", len(users))
	}
}

func TestThreadServiceListFollowedThreadsFiltersMembershipLoss(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("followed-loss")
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-loss-participant", "Thread Loss Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, participant.Id, core.KindChannel, participant.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom participant: %v", err)
	}
	root := env.post(room.Id, env.viewer.Id, "root body", "")
	env.post(room.Id, participant.Id, "reply body", root.Id)

	ctx := withCaller(env.ctx, env.viewer)
	if _, err := env.threads.FollowThread(ctx, connect.NewRequest(&apiv1.FollowThreadRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
	})); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if err := env.core.LeaveRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom viewer: %v", err)
	}

	resp, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if got := len(resp.Msg.GetThreads()); got != 0 {
		t.Fatalf("ListFollowedThreads returned %d threads after membership loss, want 0", got)
	}
	if resp.Msg.GetPage().GetTotalCount() != 0 || resp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListFollowedThreads page metadata = total %d hasMore %v, want total 0 hasMore false", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore())
	}
}

func TestThreadServiceListFollowedThreadsFiltersOtherRoomKinds(t *testing.T) {
	env := newConnectAPITestEnv(t)
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "thread-dm-participant", "Thread DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{participant.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "root body", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	if err := env.core.FollowThread(env.ctx, core.KindDM, env.viewer.Id, dm.Id, root.Id); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}

	resp, err := env.threads.ListFollowedThreads(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListFollowedThreadsRequest{
		Page: &apiv1.PageRequest{Limit: 20},
	}))
	if err != nil {
		t.Fatalf("ListFollowedThreads: %v", err)
	}
	if got := len(resp.Msg.GetThreads()); got != 0 {
		t.Fatalf("ListFollowedThreads returned %d DM threads in the channel list, want 0", got)
	}
	if resp.Msg.GetPage().GetTotalCount() != 0 || resp.Msg.GetPage().GetHasMore() {
		t.Fatalf("ListFollowedThreads page metadata = total %d hasMore %v, want total 0 hasMore false", resp.Msg.GetPage().GetTotalCount(), resp.Msg.GetPage().GetHasMore())
	}
}

func TestFollowedThreadsResponseOmitsUnavailableRooms(t *testing.T) {
	env := newConnectAPITestEnv(t)
	page := &core.FollowedThreadsPage{
		Threads: []*core.FollowedThread{{
			SpaceID:           core.LegacySpaceIDForRoomKind(core.KindChannel),
			RoomID:            "missing-room",
			ThreadRootEventID: "missing-root",
		}},
		TotalCount: 1,
	}

	resp, err := followedThreadsResponse(env.ctx, env.api, env.viewer.Id, page)
	if err != nil {
		t.Fatalf("followedThreadsResponse: %v", err)
	}
	if got := len(resp.GetThreads()); got != 0 {
		t.Fatalf("followedThreadsResponse returned %d unavailable threads, want 0", got)
	}
}
