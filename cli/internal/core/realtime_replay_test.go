package core

import (
	"bytes"
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRealtimeCursorRoundTrip(t *testing.T) {
	identity := "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cursor, err := encodeRealtimeCursor(identity, 42)
	if err != nil {
		t.Fatalf("encodeRealtimeCursor: %v", err)
	}
	decoded, err := decodeRealtimeCursor(cursor)
	if err != nil {
		t.Fatalf("decodeRealtimeCursor: %v", err)
	}
	if decoded.Version != realtimeCursorVersion || decoded.StreamIdentity != identity || decoded.Sequence != 42 {
		t.Fatalf("decoded cursor = %+v", decoded)
	}
	if _, err := decodeRealtimeCursor("not-a-cursor"); !errors.Is(err, ErrRealtimeCursorInvalid) {
		t.Fatalf("invalid cursor error = %v, want ErrRealtimeCursorInvalid", err)
	}
}

func TestPlanRealtimeReplayReplaysAuthorizedReactionGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, messageEventID := setupReactionTest(t, chatto, ctx)

	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if len(before.Events) != 0 || before.StartCursor == "" || before.BoundaryCursor == "" {
		t.Fatalf("initial replay plan = %+v", before)
	}

	if added, err := chatto.AddReaction(ctx, KindChannel, room.Id, messageEventID, "thumbsup", user.Id); err != nil || !added {
		t.Fatalf("AddReaction = %v, %v", added, err)
	}
	if removed, err := chatto.RemoveReaction(ctx, KindChannel, room.Id, messageEventID, "thumbsup", user.Id); err != nil || !removed {
		t.Fatalf("RemoveReaction = %v, %v", removed, err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if replay.StartCursor != before.BoundaryCursor {
		t.Fatalf("start cursor changed: got %q want %q", replay.StartCursor, before.BoundaryCursor)
	}
	if len(replay.Events) != 2 {
		t.Fatalf("replayed events = %d, want 2", len(replay.Events))
	}
	if got := replay.Events[0].EVTEvent().GetReactionAdded(); got == nil || got.GetMessageEventId() != messageEventID {
		t.Fatalf("first replay event = %T, want reaction_added for %q", replay.Events[0].EVTEvent().GetEvent(), messageEventID)
	}
	if got := replay.Events[1].EVTEvent().GetReactionRemoved(); got == nil || got.GetMessageEventId() != messageEventID {
		t.Fatalf("second replay event = %T, want reaction_removed for %q", replay.Events[1].EVTEvent().GetEvent(), messageEventID)
	}
	if replay.Events[0].DeliverySeq() >= replay.Events[1].DeliverySeq() || replay.Events[1].DeliverySeq() > replay.BoundarySequence {
		t.Fatalf("replay sequences = %d, %d through %d", replay.Events[0].DeliverySeq(), replay.Events[1].DeliverySeq(), replay.BoundarySequence)
	}

	outsider, err := chatto.CreateUser(ctx, SystemActorID, "replay-outsider", "Replay Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderReplay, err := chatto.PlanRealtimeReplay(ctx, outsider.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("outsider PlanRealtimeReplay: %v", err)
	}
	for _, event := range outsiderReplay.Events {
		if event.EVTEvent().GetReactionAdded() != nil || event.EVTEvent().GetReactionRemoved() != nil {
			t.Fatalf("outsider replayed unauthorized reaction event: %T", event.EVTEvent().GetEvent())
		}
	}
}

func TestPlanRealtimeReplayReplaysAuthorizedAssetLifecycleGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	attachment, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "replay-asset.txt", "text/plain", bytes.NewReader([]byte("asset")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.RecordAssetProcessingStarted(ctx, SystemActorID, KindChannel, room.Id, message.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetProcessingStarted: %v", err)
	}
	if err := chatto.RecordAssetProcessingFailed(ctx, SystemActorID, KindChannel, room.Id, message.Id, attachment.Id, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		t.Fatalf("RecordAssetProcessingFailed: %v", err)
	}
	if err := chatto.RecordAssetDeleted(ctx, SystemActorID, KindChannel, room.Id, attachment.Id); err != nil {
		t.Fatalf("RecordAssetDeleted: %v", err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if replay.Reset || len(replay.Events) != 3 {
		t.Fatalf("asset replay plan = %+v, want three incremental events", replay)
	}
	if replay.Events[0].EVTEvent().GetAssetProcessingStarted() == nil || replay.Events[1].EVTEvent().GetAssetProcessingFailed() == nil || replay.Events[2].EVTEvent().GetAssetDeleted() == nil {
		t.Fatalf("asset replay events = %T, %T, %T", replay.Events[0].EVTEvent().GetEvent(), replay.Events[1].EVTEvent().GetEvent(), replay.Events[2].EVTEvent().GetEvent())
	}
	for i, event := range replay.Events {
		if event.DeliverySeq() == 0 || event.DeliverySeq() > replay.BoundarySequence {
			t.Fatalf("asset replay event %d sequence = %d through %d", i, event.DeliverySeq(), replay.BoundarySequence)
		}
	}

	outsider, err := chatto.CreateUser(ctx, SystemActorID, "asset-replay-outsider", "Asset Replay Outsider", "password123")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	outsiderReplay, err := chatto.PlanRealtimeReplay(ctx, outsider.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("outsider PlanRealtimeReplay: %v", err)
	}
	for _, event := range outsiderReplay.Events {
		if isAssetLifecycleEvent(event.EVTEvent()) {
			t.Fatalf("outsider replayed unauthorized asset event: %T", event.EVTEvent().GetEvent())
		}
	}
}

func TestPlanRealtimeReplayReplaysLegacyRoomScopedAssetLifecycleGap(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	attachment, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "legacy-replay.txt", "text/plain", bytes.NewReader([]byte("asset")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "legacy asset lifecycle", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	before, err := chatto.PlanRealtimeReplay(ctx, user.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	legacy := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_AssetProcessingStarted{
		AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{AssetId: attachment.Id, MessageEventId: message.Id},
	}})
	legacySubject := events.RoomAggregate(room.Id).SubjectFor(legacy)
	if _, err := chatto.EventPublisher.AppendEventually(ctx, legacySubject, legacy); err != nil {
		t.Fatalf("append legacy asset event: %v", err)
	}

	replay, err := chatto.PlanRealtimeReplay(ctx, user.Id, before.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if replay.Reset || len(replay.Events) != 1 || replay.Events[0].EVTEvent().GetAssetProcessingStarted() == nil {
		t.Fatalf("legacy asset replay plan = %+v, want one processing-started event", replay)
	}
}

func TestAssetEventTimelineTargetResolvesDeletedProcessedDerivative(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, chatto, ctx)
	original, err := chatto.UploadAttachment(ctx, user.Id, room.Id, "original.mp4", "video/mp4", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("UploadAttachment original: %v", err)
	}
	thumbnail, err := chatto.UploadDerivativeAttachment(ctx, original.Id, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumbnail.bin", "application/octet-stream", bytes.NewReader([]byte("thumbnail")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "processed video", []string{original.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chatto.RecordAssetProcessed(ctx, SystemActorID, KindChannel, room.Id, message.Id, original.Id, 1000, 640, 360, thumbnail, nil); err != nil {
		t.Fatalf("RecordAssetProcessed: %v", err)
	}
	if err := chatto.RecordAssetDeleted(ctx, SystemActorID, KindChannel, room.Id, thumbnail.Id); err != nil {
		t.Fatalf("RecordAssetDeleted thumbnail: %v", err)
	}

	roomID, messageEventID, ok := chatto.AssetEventTimelineTarget(&corev1.Event{
		Event: &corev1.Event_AssetDeleted{AssetDeleted: &corev1.AssetDeletedEvent{AssetId: thumbnail.Id}},
	})
	if !ok || roomID != room.Id || messageEventID != message.Id {
		t.Fatalf("AssetEventTimelineTarget = %q, %q, %v; want %q, %q, true", roomID, messageEventID, ok, room.Id, message.Id)
	}
}

func TestPlanRealtimeReplayResetsForDifferentStreamIncarnation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	cursor, err := encodeRealtimeCursor("evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 0)
	if err != nil {
		t.Fatalf("encodeRealtimeCursor: %v", err)
	}
	plan, err := chatto.PlanRealtimeReplay(ctx, "user", cursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 || plan.StartCursor != plan.BoundaryCursor {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want compacted reset", plan)
	}
}

func TestPlanRealtimeReplayResetsAfterUserKeyShredding(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chatto.CreateUser(ctx, SystemActorID, "replay-shred-viewer", "Replay Shred Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	author, err := chatto.CreateUser(ctx, SystemActorID, "replay-shred-author", "Replay Shred Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, viewer.Id, KindChannel, "", "replay-shred-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{viewer.Id, author.Id} {
		if _, err := chatto.JoinRoom(ctx, viewer.Id, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %q: %v", userID, err)
		}
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.Id, author.Id, "must be purged", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.DeleteUser(ctx, viewer.Id, author.Id); err != nil {
		t.Fatalf("DeleteUser author: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want compacted reset", plan)
	}
}

func TestPlanRealtimeReplayResetsAfterViewerLosesRoomVisibility(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, room, _ := setupReactionTest(t, chatto, ctx)

	boundary, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, "")
	if err != nil {
		t.Fatalf("initial PlanRealtimeReplay: %v", err)
	}
	if err := chatto.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	plan, err := chatto.PlanRealtimeReplay(ctx, viewer.Id, boundary.BoundaryCursor)
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}
	if !plan.Reset || len(plan.Events) != 0 || plan.StartCursor != plan.BoundaryCursor {
		t.Fatalf("PlanRealtimeReplay plan = %+v, want compacted authorization reset", plan)
	}
}

func TestRealtimeReplayRequiresResetForServerProjectionAggregates(t *testing.T) {
	for _, subject := range []string{
		"evt.config.server.server_name_changed",
		"evt.group.G1.room_group_updated",
		"evt.layout.default.room_moved",
	} {
		if !realtimeReplayRequiresReset(subject) {
			t.Fatalf("realtimeReplayRequiresReset(%q) = false", subject)
		}
	}
	if realtimeReplayRequiresReset("evt.room.R1.message_posted") {
		t.Fatal("room message unexpectedly requires reset")
	}
}

func TestRealtimeReplayRoomSubject(t *testing.T) {
	roomID, ok := realtimeReplayRoomSubject(events.RoomAggregate("R1").SubjectFor(&corev1.Event{
		Event: &corev1.Event_ReactionAdded{ReactionAdded: &corev1.ReactionAddedEvent{}},
	}))
	if !ok || roomID != "R1" {
		t.Fatalf("realtimeReplayRoomSubject = %q, %v", roomID, ok)
	}
	if _, ok := realtimeReplayRoomSubject("evt.user.U1.custom_status_set"); ok {
		t.Fatal("user subject accepted as room replay subject")
	}
}
