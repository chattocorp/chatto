package core

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestVideoManifestMigration_CompletedImportsWithoutOriginal(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _, original := createPostedVideoAttachment(t, core)
	thumb, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(32, 18)))
	if err != nil {
		t.Fatalf("upload thumbnail: %v", err)
	}
	variantAttachment, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "video-720p.mp4", "video/mp4", bytes.NewReader([]byte("variant")))
	if err != nil {
		t.Fatalf("upload variant: %v", err)
	}
	if err := core.DeleteAttachmentFromStorage(ctx, original); err != nil {
		t.Fatalf("delete original: %v", err)
	}

	state := &corev1.VideoProcessingState{
		Status:              corev1.VideoStatus_VIDEO_STATUS_COMPLETED,
		ThumbnailAttachment: thumb,
		DurationMs:          1234,
		Width:               640,
		Height:              360,
		Variants: []*corev1.VideoVariant{
			{
				AttachmentId: variantAttachment.Id,
				Quality:      "720p",
				Width:        640,
				Height:       360,
				Size:         variantAttachment.Size,
				Attachment:   variantAttachment,
			},
		},
	}
	writeLegacyVideoState(t, core, original.Id, state)

	if err := core.migrateVideoManifestsToES(ctx); err != nil {
		t.Fatalf("migrate video manifests: %v", err)
	}
	waitForVideoSubject(t, core, room.Id, events.EventAssetProcessingSucceeded)

	manifest, ok := core.RoomTimeline.VideoAttachmentManifest(original.Id)
	if !ok || manifest.Succeeded == nil {
		t.Fatal("expected processed manifest")
	}
	created, ok := core.RoomTimeline.AssetCreation(original.Id)
	if !ok || created.GetOriginalBinaryAvailable() {
		t.Fatal("expected original_binary_available=false on migrated asset creation")
	}
	video := manifest.Succeeded.GetVideo()
	if video == nil {
		t.Fatal("expected video metadata")
	}
	if got := video.GetThumbnailAssetId(); got != thumb.Id {
		t.Fatalf("thumbnail id = %q, want %q", got, thumb.Id)
	}
	if len(video.Variants) != 1 || video.Variants[0].GetAssetId() != variantAttachment.Id {
		t.Fatalf("expected one imported usable variant, got %#v", video.Variants)
	}
	if _, ok := core.RoomTimeline.AssetCreation(thumb.Id); !ok {
		t.Fatalf("expected imported thumbnail asset creation")
	}
	if _, ok := core.RoomTimeline.AssetCreation(variantAttachment.Id); !ok {
		t.Fatalf("expected imported variant asset creation")
	}
}

func TestVideoManifestMigration_PendingMissingOriginalImportsUnavailable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _, original := createPostedVideoAttachment(t, core)
	if err := core.DeleteAttachmentFromStorage(ctx, original); err != nil {
		t.Fatalf("delete original: %v", err)
	}
	writeLegacyVideoState(t, core, original.Id, &corev1.VideoProcessingState{
		Status: corev1.VideoStatus_VIDEO_STATUS_PENDING,
	})

	if err := core.migrateVideoManifestsToES(ctx); err != nil {
		t.Fatalf("migrate video manifests: %v", err)
	}
	waitForVideoSubject(t, core, room.Id, events.EventAssetProcessingFailed)

	manifest, ok := core.RoomTimeline.VideoAttachmentManifest(original.Id)
	if !ok || manifest.Failed == nil {
		t.Fatal("expected failed manifest")
	}
	if got := manifest.Failed.GetFailureCode(); got != corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING {
		t.Fatalf("failure code = %v, want source missing", got)
	}
}

func TestVideoManifestMigration_UntrackedMissingOriginalImportsUnavailable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _, original := createPostedVideoAttachment(t, core)
	if err := core.DeleteAttachmentFromStorage(ctx, original); err != nil {
		t.Fatalf("delete original: %v", err)
	}

	if err := core.migrateVideoManifestsToES(ctx); err != nil {
		t.Fatalf("migrate video manifests: %v", err)
	}
	waitForVideoSubject(t, core, room.Id, events.EventAssetProcessingFailed)

	manifest, ok := core.RoomTimeline.VideoAttachmentManifest(original.Id)
	if !ok || manifest.Failed == nil {
		t.Fatal("expected failed manifest")
	}
	if got := manifest.Failed.GetFailureCode(); got != corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING {
		t.Fatalf("failure code = %v, want source missing", got)
	}
}

func TestVideoStatusPreservesLegacyRuntimeKVWireNumbers(t *testing.T) {
	if got := int32(corev1.VideoStatus_VIDEO_STATUS_PENDING); got != 0 {
		t.Fatalf("VIDEO_STATUS_PENDING = %d, want legacy wire value 0", got)
	}
	if got := int32(corev1.VideoStatus_VIDEO_STATUS_PROCESSING); got != 1 {
		t.Fatalf("VIDEO_STATUS_PROCESSING = %d, want legacy wire value 1", got)
	}
	if got := int32(corev1.VideoStatus_VIDEO_STATUS_COMPLETED); got != 2 {
		t.Fatalf("VIDEO_STATUS_COMPLETED = %d, want legacy wire value 2", got)
	}
	if got := int32(corev1.VideoStatus_VIDEO_STATUS_FAILED); got != 3 {
		t.Fatalf("VIDEO_STATUS_FAILED = %d, want legacy wire value 3", got)
	}
}

func createPostedVideoAttachment(t *testing.T, core *ChattoCore) (*corev1.Room, *corev1.User, *corev1.Attachment) {
	t.Helper()
	ctx := testContext(t)
	legacyRuntime := ensureLegacyServerRuntimeKV(t, core)

	_ = core.storage.runtimeStateKV.Delete(ctx, videoManifestESMigrationKey)
	_ = core.storage.runtimeStateKV.Delete(ctx, assetCreationESMigrationKey)
	_ = legacyRuntime.Delete(ctx, videoManifestESMigrationKey)
	_ = legacyRuntime.Delete(ctx, assetCreationESMigrationKey)
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Video", "Video room")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	user, err := core.CreateUser(ctx, "system", "video-user", "video-user", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("join room: %v", err)
	}
	// Simulate the legacy state: write the binary without emitting an
	// AssetCreatedEvent (pre-Option-1 PostMessage emitted it inline). The
	// migration is supposed to backfill the missing AssetCreated and the
	// corresponding processing manifest.
	original, err := core.uploadAttachmentBinary(ctx, room.Id, "original.mp4", "video/mp4", bytes.NewReader([]byte("original")))
	if err != nil {
		t.Fatalf("upload original: %v", err)
	}
	legacyPost := newEvent(user.Id, &corev1.Event{
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId: room.Id,
			},
		},
	})
	legacyBody := newEvent(user.Id, &corev1.Event{
		Event: &corev1.Event_MessageBody{
			MessageBody: &corev1.MessageBodyEvent{
				RoomId:  room.Id,
				EventId: legacyPost.Id,
				Body: &corev1.MessageBody{
					AuthorId:    user.Id,
					Attachments: []*corev1.Attachment{original},
				},
			},
		},
	})
	original.MessageBodyId = legacyPost.Id
	if _, err := core.EventPublisher.AppendEventually(ctx, events.RoomAggregate(room.Id).SubjectFor(legacyBody), legacyBody); err != nil {
		t.Fatalf("append legacy body: %v", err)
	}
	if _, err := core.EventPublisher.AppendEventually(ctx, events.RoomAggregate(room.Id).SubjectFor(legacyPost), legacyPost); err != nil {
		t.Fatalf("append legacy message: %v", err)
	}
	messageSubject := events.RoomAggregate(room.Id).Subject(events.EventMessagePosted)
	_, seq, err := core.EventPublisher.SubjectEvents(ctx, messageSubject)
	if err != nil {
		t.Fatalf("read legacy message subject: %v", err)
	}
	if err := core.RoomTimelineProjector.WaitFor(ctx, events.SubjectPosition(messageSubject, seq)); err != nil {
		t.Fatalf("wait for legacy message projection: %v", err)
	}
	return room, user, original
}

func writeLegacyVideoState(t *testing.T, core *ChattoCore, attachmentID string, state *corev1.VideoProcessingState) {
	t.Helper()
	ctx := testContext(t)
	legacyRuntime := ensureLegacyServerRuntimeKV(t, core)

	data, err := proto.Marshal(state)
	if err != nil {
		t.Fatalf("marshal video state: %v", err)
	}
	if _, err := legacyRuntime.Put(ctx, videoProcessingKey(attachmentID), data); err != nil {
		t.Fatalf("put legacy video state: %v", err)
	}
}

func waitForVideoSubject(t *testing.T, core *ChattoCore, roomID, eventType string) {
	t.Helper()
	ctx := testContext(t)

	subject := events.RoomAggregate(roomID).Subject(eventType)
	published, seq, err := core.EventPublisher.SubjectEvents(ctx, subject)
	if err != nil {
		t.Fatalf("read %s: %v", subject, err)
	}
	if len(published) == 0 {
		t.Fatalf("expected event on %s", subject)
	}
	if err := core.RoomTimelineProjector.WaitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("wait for room timeline seq %d: %v", seq, err)
	}
}
