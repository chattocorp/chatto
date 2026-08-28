package core

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestAuthorizedRoomAttachmentReadsRequireMessageRead(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, chatto, ctx)
	attachment := uploadRoomAttachment(t, chatto, ctx, user.Id, room.Id, "private.png")
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "private file", []string{attachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyRoomPermission message.read.interactions: %v", err)
	}

	reads := map[string]func() error{
		"room list": func() error {
			_, err := chatto.ListRoomAttachments(ctx, ListRoomAttachmentsInput{ActorID: user.Id, RoomID: room.Id})
			return err
		},
		"asset": func() error {
			_, err := chatto.GetRoomAsset(ctx, RoomAssetInput{ActorID: user.Id, RoomID: room.Id, AssetID: attachment.Id})
			return err
		},
		"message attachments": func() error {
			_, err := chatto.MessageAttachments(ctx, MessageAttachmentsInput{ActorID: user.Id, RoomID: room.Id, EventID: message.Id})
			return err
		},
	}
	for name, read := range reads {
		if err := read(); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("%s error = %v, want ErrPermissionDenied", name, err)
		}
	}
}

func TestAuthorizedDMAttachmentReadsIgnoreMessageRead(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	reader, err := chatto.CreateUser(ctx, SystemActorID, "dm-attachment-reader", "DM Attachment Reader", "password123")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	author, err := chatto.CreateUser(ctx, SystemActorID, "dm-attachment-author", "DM Attachment Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	dm, _, err := chatto.FindOrCreateDM(ctx, reader.GetId(), []string{author.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	attachment := uploadRoomAttachment(t, chatto, ctx, author.GetId(), dm.GetId(), "direct-message.png")
	message, err := chatto.PostMessage(ctx, KindDM, dm.GetId(), author.GetId(), "direct message file", []string{attachment.GetId()}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, dm.GetId(), reader.GetId(), PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	roomAttachments, err := chatto.ListRoomAttachments(ctx, ListRoomAttachmentsInput{ActorID: reader.GetId(), RoomID: dm.GetId()})
	if err != nil {
		t.Fatalf("ListRoomAttachments after DM denial: %v", err)
	}
	if got := attachmentNames(roomAttachments.Items); !sameStrings(got, []string{"direct-message.png"}) {
		t.Fatalf("attachment names = %v, want [direct-message.png]", got)
	}
	if _, err := chatto.GetRoomAsset(ctx, RoomAssetInput{ActorID: reader.GetId(), RoomID: dm.GetId(), AssetID: attachment.GetId()}); err != nil {
		t.Fatalf("GetRoomAsset after DM denial: %v", err)
	}
	if assets, err := chatto.BatchGetRoomAssets(ctx, BatchRoomAssetsInput{ActorID: reader.GetId(), RoomID: dm.GetId(), AssetIDs: []string{attachment.GetId()}}); err != nil || len(assets) != 1 {
		t.Fatalf("BatchGetRoomAssets after DM denial = %d assets, %v; want 1, nil", len(assets), err)
	}
	if attachments, err := chatto.MessageAttachments(ctx, MessageAttachmentsInput{ActorID: reader.GetId(), RoomID: dm.GetId(), EventID: message.GetId()}); err != nil || len(attachments) != 1 {
		t.Fatalf("MessageAttachments after DM denial = %d attachments, %v; want 1, nil", len(attachments), err)
	}
	if sets, err := chatto.BatchMessageAttachments(ctx, BatchMessageAttachmentsInput{ActorID: reader.GetId(), RoomID: dm.GetId(), EventIDs: []string{message.GetId()}}); err != nil || len(sets) != 1 || len(sets[0].Attachments) != 1 {
		t.Fatalf("BatchMessageAttachments after DM denial = %+v, %v; want one attachment set", sets, err)
	}
}

func TestAuthorizedRoomAttachmentReadsUseThreadInteractions(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	room, author := setupRoomAttachmentTest(t, chatto, ctx)
	reader, err := chatto.CreateUser(ctx, SystemActorID, "interaction-attachment-reader", "Interaction Attachment Reader", "password123")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	if _, err := chatto.JoinRoom(ctx, reader.GetId(), KindChannel, reader.GetId(), room.GetId()); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}
	visibleAsset := uploadRoomAttachment(t, chatto, ctx, author.GetId(), room.GetId(), "visible-thread.png")
	visibleRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "visible attachment root", []string{visibleAsset.GetId()}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage visible root: %v", err)
	}
	hiddenAsset := uploadRoomAttachment(t, chatto, ctx, author.GetId(), room.GetId(), "hidden-thread.png")
	hiddenRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "hidden attachment root", []string{hiddenAsset.GetId()}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage hidden root: %v", err)
	}
	pendingAsset := uploadRoomAttachment(t, chatto, ctx, author.GetId(), room.GetId(), "pending.png")
	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission message.read: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read.interactions: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "attachment ping @interaction-attachment-reader", nil, visibleRoot.GetId(), "", nil, false); err != nil {
		t.Fatalf("PostMessage mention: %v", err)
	}

	page, err := chatto.ListRoomAttachments(ctx, ListRoomAttachmentsInput{ActorID: reader.GetId(), RoomID: room.GetId(), Limit: 20})
	if err != nil || len(page.Items) != 1 || page.Items[0].Attachment.GetId() != visibleAsset.GetId() {
		t.Fatalf("ListRoomAttachments = %+v, %v; want visible thread asset", page, err)
	}
	if _, err := chatto.GetRoomAsset(ctx, RoomAssetInput{ActorID: reader.GetId(), RoomID: room.GetId(), AssetID: visibleAsset.GetId()}); err != nil {
		t.Fatalf("GetRoomAsset visible: %v", err)
	}
	for name, assetID := range map[string]string{"hidden": hiddenAsset.GetId(), "pending": pendingAsset.GetId()} {
		if _, err := chatto.GetRoomAsset(ctx, RoomAssetInput{ActorID: reader.GetId(), RoomID: room.GetId(), AssetID: assetID}); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("GetRoomAsset %s error = %v, want ErrPermissionDenied", name, err)
		}
	}
	assets, err := chatto.BatchGetRoomAssets(ctx, BatchRoomAssetsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), AssetIDs: []string{hiddenAsset.GetId(), visibleAsset.GetId(), pendingAsset.GetId()},
	})
	if err != nil || len(assets) != 1 || assets[0].GetId() != visibleAsset.GetId() {
		t.Fatalf("BatchGetRoomAssets = %+v, %v; want only visible asset", assets, err)
	}
	attachments, err := chatto.MessageAttachments(ctx, MessageAttachmentsInput{ActorID: reader.GetId(), RoomID: room.GetId(), EventID: visibleRoot.GetId()})
	if err != nil || len(attachments) != 1 || attachments[0].GetId() != visibleAsset.GetId() {
		t.Fatalf("MessageAttachments visible root = %+v, %v", attachments, err)
	}
	sets, err := chatto.BatchMessageAttachments(ctx, BatchMessageAttachmentsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), EventIDs: []string{hiddenRoot.GetId(), visibleRoot.GetId()},
	})
	if err != nil || len(sets) != 1 || sets[0].EventID != visibleRoot.GetId() {
		t.Fatalf("BatchMessageAttachments = %+v, %v; want only visible root", sets, err)
	}
}

func TestChattoCore_GetRoomAttachmentsIncludesRootAndThreadFiles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, user := setupRoomAttachmentTest(t, core, ctx)

	rootA := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "root-a.png")
	rootB := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "root-b.png")
	rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "root with files", []string{rootA.Id, rootB.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root message: %v", err)
	}

	threadAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "thread.png")
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

	oldAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "old.png")
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "old", []string{oldAttachment.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Post old message: %v", err)
	}

	newAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "new.png")
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

	removedAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "removed.png")
	keptAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "kept.png")
	editedEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "edit target", []string{removedAttachment.Id, keptAttachment.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post edit target: %v", err)
	}
	if err := core.DeleteAttachmentFromMessage(ctx, user.Id, KindChannel, room.Id, editedEvent.Id, removedAttachment.Id); err != nil {
		t.Fatalf("DeleteAttachmentFromMessage: %v", err)
	}

	retractedAttachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "retracted.png")
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

	attachment := uploadRoomAttachment(t, core, ctx, user.Id, room.Id, "file.png")
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "with file", []string{attachment.Id}, "", "", nil, false); err != nil {
		t.Fatalf("Post file message: %v", err)
	}

	messageEventID := NewEventID()
	bodyEventID := NewEventID()
	createdAt := timestamppb.Now()
	corruptBody := &evtv1.MessageBody{
		AuthorId:        user.Id,
		CreatedAt:       createdAt,
		BodyEventId:     bodyEventID,
		EncryptedBody:   []byte("not-valid-ciphertext"),
		EncryptionNonce: []byte("bad-nonce"),
	}
	if err := core.roomModel.timeline.Projection().Apply(&evtv1.Event{
		Id:        bodyEventID,
		ActorId:   user.Id,
		CreatedAt: createdAt,
		Event: &evtv1.Event_MessageBody{
			MessageBody: &evtv1.MessageBodyEvent{
				RoomId:  room.Id,
				EventId: messageEventID,
				Body:    corruptBody,
			},
		},
	}, 1_000_000); err != nil {
		t.Fatalf("Apply corrupt text body: %v", err)
	}
	if err := core.roomModel.timeline.Projection().Apply(&evtv1.Event{
		Id:        messageEventID,
		ActorId:   user.Id,
		CreatedAt: createdAt,
		Event: &evtv1.Event_MessagePosted{
			MessagePosted: &evtv1.MessagePostedEvent{
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

func setupRoomAttachmentTest(t *testing.T, core *ChattoCore, ctx context.Context) (*evtv1.Room, *evtv1.User) {
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

func uploadRoomAttachment(t *testing.T, core *ChattoCore, ctx context.Context, actorID, roomID, filename string) *evtv1.Attachment {
	t.Helper()
	attachment, err := core.UploadAttachment(ctx, actorID, roomID, filename, "image/png", bytes.NewReader(createTestPNG(16, 16)))
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
