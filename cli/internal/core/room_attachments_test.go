package core

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_GetRoomAttachmentsIncludesRootAndThreadFiles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)

	rootA := uploadRoomAttachment(t, core, ctx, room.Id, "root-a.png")
	rootB := uploadRoomAttachment(t, core, ctx, room.Id, "root-b.png")
	rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "root with files", []string{rootA.Id, rootB.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root message: %v", err)
	}

	threadAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "thread.png")
	threadEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "thread with file", []string{threadAttachment.Id}, rootEvent.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Post thread reply: %v", err)
	}

	result, err := core.GetRoomAttachments(ctx, KindChannel, room.Id, 10, 0)
	if err != nil {
		t.Fatalf("GetRoomAttachments: %v", err)
	}

	if result.TotalCount != 3 {
		t.Fatalf("TotalCount = %d, want 3", result.TotalCount)
	}
	if result.HasMore {
		t.Fatal("HasMore = true, want false")
	}
	if got := attachmentNames(result.Items); !sameStrings(got, []string{"thread.png", "root-a.png", "root-b.png"}) {
		t.Fatalf("attachment order = %v, want [thread.png root-a.png root-b.png]", got)
	}

	if result.Items[0].MessageEventID != threadEvent.Id {
		t.Fatalf("thread item messageEventId = %q, want %q", result.Items[0].MessageEventID, threadEvent.Id)
	}
	if result.Items[0].ThreadRootEventID != rootEvent.Id {
		t.Fatalf("thread item threadRootEventId = %q, want %q", result.Items[0].ThreadRootEventID, rootEvent.Id)
	}
	if result.Items[1].MessageEventID != rootEvent.Id || result.Items[1].ThreadRootEventID != "" {
		t.Fatalf("root item anchor = (%q, %q), want (%q, empty)", result.Items[1].MessageEventID, result.Items[1].ThreadRootEventID, rootEvent.Id)
	}
}

func TestChattoCore_GetRoomAttachmentsPagination(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)

	oldAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "old.png")
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "old", []string{oldAttachment.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Post old message: %v", err)
	}

	newAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "new.png")
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "new", []string{newAttachment.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Post new message: %v", err)
	}

	first, err := core.GetRoomAttachments(ctx, KindChannel, room.Id, 1, 0)
	if err != nil {
		t.Fatalf("Get first page: %v", err)
	}
	if first.TotalCount != 2 || !first.HasMore || len(first.Items) != 1 || first.Items[0].Attachment.Filename != "new.png" {
		t.Fatalf("first page = count %d hasMore %v names %v, want count 2 hasMore true [new.png]", first.TotalCount, first.HasMore, attachmentNames(first.Items))
	}

	second, err := core.GetRoomAttachments(ctx, KindChannel, room.Id, 1, 1)
	if err != nil {
		t.Fatalf("Get second page: %v", err)
	}
	if second.TotalCount != 2 || second.HasMore || len(second.Items) != 1 || second.Items[0].Attachment.Filename != "old.png" {
		t.Fatalf("second page = count %d hasMore %v names %v, want count 2 hasMore false [old.png]", second.TotalCount, second.HasMore, attachmentNames(second.Items))
	}
}

func TestChattoCore_GetRoomAttachmentsExcludesRemovedAndRetractedFiles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)

	removedAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "removed.png")
	keptAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "kept.png")
	editedEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "edit target", []string{removedAttachment.Id, keptAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post edit target: %v", err)
	}
	if err := core.DeleteAttachmentFromMessage(ctx, user.Id, KindChannel, room.Id, editedEvent.Id, removedAttachment.Id); err != nil {
		t.Fatalf("DeleteAttachmentFromMessage: %v", err)
	}

	retractedAttachment := uploadRoomAttachment(t, core, ctx, room.Id, "retracted.png")
	retractedEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "delete target", []string{retractedAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post delete target: %v", err)
	}
	if err := core.DeleteMessage(ctx, user.Id, KindChannel, room.Id, retractedEvent.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	result, err := core.GetRoomAttachments(ctx, KindChannel, room.Id, 10, 0)
	if err != nil {
		t.Fatalf("GetRoomAttachments: %v", err)
	}

	if got := attachmentNames(result.Items); !sameStrings(got, []string{"kept.png"}) {
		t.Fatalf("attachment names = %v, want [kept.png]", got)
	}
}

func TestChattoCore_GetRoomAttachmentsDoesNotDecryptNonFileMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)

	attachment := uploadRoomAttachment(t, core, ctx, room.Id, "file.png")
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "with file", []string{attachment.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Post file message: %v", err)
	}

	messageEventID := NewEventID()
	bodyEventID := NewEventID()
	createdAt := timestamppb.Now()
	corruptBody := &corev1.MessageBody{
		AuthorId:        user.Id,
		CreatedAt:       createdAt,
		BodyEventId:     bodyEventID,
		EncryptedBody:   []byte("not-valid-ciphertext"),
		EncryptionNonce: []byte("bad-nonce"),
	}
	if err := core.RoomTimeline.Apply(&corev1.Event{
		Id:        bodyEventID,
		ActorId:   user.Id,
		CreatedAt: createdAt,
		Event: &corev1.Event_MessageBody{
			MessageBody: &corev1.MessageBodyEvent{
				RoomId:  room.Id,
				EventId: messageEventID,
				Body:    corruptBody,
			},
		},
	}, 1_000_000); err != nil {
		t.Fatalf("Apply corrupt text body: %v", err)
	}
	if err := core.RoomTimeline.Apply(&corev1.Event{
		Id:        messageEventID,
		ActorId:   user.Id,
		CreatedAt: createdAt,
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId: room.Id,
			},
		},
	}, 1_000_001); err != nil {
		t.Fatalf("Apply corrupt text message: %v", err)
	}

	result, err := core.GetRoomAttachments(ctx, KindChannel, room.Id, 10, 0)
	if err != nil {
		t.Fatalf("GetRoomAttachments: %v", err)
	}
	if got := attachmentNames(result.Items); !sameStrings(got, []string{"file.png"}) {
		t.Fatalf("attachment names = %v, want [file.png]", got)
	}
}

func setupRoomAttachmentTest(t *testing.T, core *ChattoCore, ctx context.Context) (*corev1.Room, *corev1.User) {
	t.Helper()
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	user, err := core.CreateUser(ctx, "system", "filesuser", "Files User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	return room, user
}

func uploadRoomAttachment(t *testing.T, core *ChattoCore, ctx context.Context, roomID string, filename string) *corev1.Attachment {
	t.Helper()
	attachment, err := core.UploadAttachment(ctx, SystemActorID, roomID, filename, "image/png", bytes.NewReader(createTestPNG(16, 16)))
	if err != nil {
		t.Fatalf("UploadAttachment %s: %v", filename, err)
	}
	return attachment
}

func attachmentNames(items []*RoomAttachmentItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || item.Attachment == nil {
			continue
		}
		out = append(out, item.Attachment.Filename)
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
