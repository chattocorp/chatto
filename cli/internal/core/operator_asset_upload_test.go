package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"hmans.de/chatto/internal/evtstream"
)

func TestOperatorAssetUploadOwnershipAndPublicIsolation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "operator-asset-author", "Asset Author", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "operator-asset-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	content := []byte("operator attachment")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	input := AssetUploadCreateInput{
		ActorID: author.GetId(), Operator: true, RoomID: room.GetId(), Filename: "export.txt",
		ContentType: "text/plain", Size: int64(len(content)), SHA256: digest,
	}
	if _, err := chatto.AssetUploads().CreateUpload(ctx, AssetUploadCreateInput{
		ActorID: author.GetId(), RoomID: room.GetId(), Filename: input.Filename,
		ContentType: input.ContentType, Size: input.Size, SHA256: input.SHA256,
	}); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("public upload without membership = %v, want not room member", err)
	}
	upload, err := chatto.AssetUploads().CreateUpload(ctx, input)
	if err != nil {
		t.Fatalf("operator CreateUpload: %v", err)
	}
	if _, err := chatto.AssetUploads().GetUpload(ctx, author.GetId(), upload.UploadID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("public GetUpload of operator session = %v, want permission denied", err)
	}
	if _, err := chatto.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID: author.GetId(), UploadID: upload.UploadID, Content: content, ChunkSHA256: digest,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("public UploadChunk of operator session = %v, want permission denied", err)
	}
	if _, err := chatto.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID: SystemActorID, Operator: true, UploadID: upload.UploadID,
		Content: content, ChunkSHA256: digest,
	}); err != nil {
		t.Fatalf("operator UploadChunk: %v", err)
	}
	if _, _, err := chatto.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID: author.GetId(), UploadID: upload.UploadID,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("public CompleteUpload of operator session = %v, want permission denied", err)
	}
	if _, err := chatto.AssetUploads().CancelUpload(ctx, AssetUploadCancelInput{
		ActorID: author.GetId(), UploadID: upload.UploadID,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("public CancelUpload of operator session = %v, want permission denied", err)
	}
	_, attachment, err := chatto.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID: SystemActorID, Operator: true, UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("operator CompleteUpload: %v", err)
	}
	created, ok := chatto.assetModel.AssetCreation(attachment.GetId())
	if !ok || created.GetUserId() != author.GetId() || created.GetRoomId() != room.GetId() {
		t.Fatalf("created asset = %+v, found = %t", created, ok)
	}
	events, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.AssetAggregate(attachment.GetId()).Subject(evtstream.EventAssetCreated))
	if err != nil || len(events) != 1 || events[0].GetActorId() != SystemActorID {
		t.Fatalf("asset creation events = %+v, err = %v", events, err)
	}
	reader, _, err := chatto.GetAttachmentReader(ctx, attachment)
	if err != nil {
		t.Fatalf("GetAttachmentReader: %v", err)
	}
	stored, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("stored bytes = %q, err = %v", stored, err)
	}
	if _, err := chatto.AddMember(ctx, SystemActorID, KindChannel, room.GetId(), author.GetId()); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "", []string{attachment.GetId()}, "", "", nil, false); err != nil {
		t.Fatalf("attach operator asset to author message: %v", err)
	}
	if _, err := chatto.GetRoomAsset(ctx, RoomAssetInput{
		ActorID: author.GetId(), RoomID: room.GetId(), AssetID: attachment.GetId(),
	}); err != nil {
		t.Fatalf("read attached asset through room access: %v", err)
	}
	publicUpload, err := chatto.AssetUploads().CreateUpload(ctx, AssetUploadCreateInput{
		ActorID: author.GetId(), RoomID: room.GetId(), Filename: "public.txt",
		ContentType: "text/plain", Size: 0,
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	})
	if err != nil {
		t.Fatalf("public CreateUpload: %v", err)
	}
	if _, _, err := chatto.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID: SystemActorID, Operator: true, UploadID: publicUpload.UploadID,
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("operator CompleteUpload of public session = %v, want permission denied", err)
	}
}

func TestOperatorAssetUploadTargetsAndCancellation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "operator-asset-target", "Asset Target", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "operator-asset-target-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	content := []byte("cancel me")
	sum := sha256.Sum256(content)
	input := AssetUploadCreateInput{
		ActorID: author.GetId(), Operator: true, RoomID: room.GetId(), Filename: "cancel.txt",
		ContentType: "text/plain", Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:]),
	}
	for _, tc := range []struct {
		name  string
		input AssetUploadCreateInput
	}{
		{name: "missing author", input: func() AssetUploadCreateInput { v := input; v.ActorID = "missing"; return v }()},
		{name: "missing room", input: func() AssetUploadCreateInput { v := input; v.RoomID = "missing"; return v }()},
		{name: "over size limit", input: func() AssetUploadCreateInput {
			v := input
			v.Size = int64(chatto.AssetsConfig().MaxUploadSize) + 1
			return v
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := chatto.AssetUploads().CreateUpload(ctx, tc.input); err == nil {
				t.Fatal("CreateUpload succeeded for invalid target")
			}
		})
	}
	upload, err := chatto.AssetUploads().CreateUpload(ctx, input)
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if _, err := chatto.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID: SystemActorID, Operator: true, UploadID: upload.UploadID,
		Content: content, ChunkSHA256: input.SHA256,
	}); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	if _, err := chatto.AssetUploads().CancelUpload(ctx, AssetUploadCancelInput{
		ActorID: SystemActorID, Operator: true, UploadID: upload.UploadID,
	}); err != nil {
		t.Fatalf("CancelUpload: %v", err)
	}
	if _, _, err := chatto.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID: SystemActorID, Operator: true, UploadID: upload.UploadID,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CompleteUpload after cancel = %v, want not found", err)
	}
	if _, err := chatto.ArchiveRoom(ctx, SystemActorID, KindChannel, room.GetId()); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	if _, err := chatto.AssetUploads().CreateUpload(ctx, input); !errors.Is(err, ErrRoomArchived) {
		t.Fatalf("CreateUpload archived room = %v, want archived", err)
	}
}
