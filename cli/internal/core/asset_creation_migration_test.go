package core

import (
	"bytes"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAssetCreationMigration_BackfillsMessageAttachments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.storage.serverRuntimeKV.Delete(ctx, assetCreationESMigrationKey); err != nil {
		t.Fatalf("delete migration sentinel: %v", err)
	}
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Files", "Files room")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	user, err := core.CreateUser(ctx, "system", "asset-user", "asset-user", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	attachment, err := core.UploadAttachment(ctx, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}

	legacyPost := newEvent(user.Id, &corev1.Event{
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId: room.Id,
				Body: &corev1.MessageBody{
					AuthorId:    user.Id,
					Attachments: []*corev1.Attachment{attachment},
				},
			},
		},
	})
	legacyPost.GetMessagePosted().MessageBodyId = legacyPost.Id
	legacyPost.GetMessagePosted().EventId = legacyPost.Id
	attachment.MessageBodyId = legacyPost.Id
	if _, err := core.EventPublisher.AppendEventually(ctx, events.RoomAggregate(room.Id).SubjectFor(legacyPost), legacyPost); err != nil {
		t.Fatalf("append legacy message: %v", err)
	}

	before, err := core.verifyAssetCreationsInEVT(ctx)
	if err != nil {
		t.Fatalf("verify before migration: %v", err)
	}
	if before.MissingCreations != 1 {
		t.Fatalf("missing creations before migration = %d, want 1", before.MissingCreations)
	}

	if err := core.migrateAssetCreationsToES(ctx); err != nil {
		t.Fatalf("migrate asset creations: %v", err)
	}
	waitForAssetCreationSubject(t, core, room.Id)

	declared, ok := core.RoomTimeline.AssetCreation(attachment.Id)
	if !ok {
		t.Fatal("expected projected asset creation")
	}
	if declared.GetRoomId() != room.Id || declared.GetMessageEventId() != legacyPost.Id {
		t.Fatalf("asset creation = %+v, want room/message owner", declared)
	}
	if got := declared.GetAsset().GetMessageBodyId(); got != legacyPost.Id {
		t.Fatalf("created asset MessageBodyId = %q, want %q", got, legacyPost.Id)
	}

	after, err := core.verifyAssetCreationsInEVT(ctx)
	if err != nil {
		t.Fatalf("verify after migration: %v", err)
	}
	if after.MissingCreations != 0 || after.DanglingProcessingOutcomes != 0 {
		t.Fatalf("verification after migration = %+v, want no inconsistencies", after)
	}
}

func waitForAssetCreationSubject(t *testing.T, core *ChattoCore, roomID string) {
	t.Helper()
	ctx := testContext(t)

	subject := events.RoomAggregate(roomID).Subject(events.EventAssetCreated)
	published, seq, err := core.EventPublisher.SubjectEvents(ctx, subject)
	if err != nil {
		t.Fatalf("read %s: %v", subject, err)
	}
	if len(published) == 0 {
		t.Fatalf("expected event on %s", subject)
	}
	if err := core.RoomTimelineProjector.WaitForSeq(ctx, seq); err != nil {
		t.Fatalf("wait for room timeline seq %d: %v", seq, err)
	}
}
