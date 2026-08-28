package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

func TestMessagePostRejectsAttachmentUploadedByAnotherRoomMember(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	victim, attacker, room := setupAttachmentOwnershipUsers(t, chatto, ctx, "cross-user")
	attachment := uploadRoomAttachmentForUser(t, chatto, ctx, victim.Id, room.Id, "victim.png")

	victimPost, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: victim.Id, RoomID: room.Id, Body: "victim",
		AttachmentAssetIDs: []string{attachment.Id},
	})
	require.NoError(t, err)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: attacker.Id, RoomID: room.Id, Body: "attacker alias",
		AttachmentAssetIDs: []string{attachment.Id},
	})
	require.ErrorIs(t, err, ErrAssetNotAttachable)
	assertMessageStillOwnsAttachment(t, chatto, ctx, victimPost.Event.Id, attachment.Id)
}

func TestMessagePostRejectsAssetAlreadyAttachedBySameUser(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, _, room := setupAttachmentOwnershipUsers(t, chatto, ctx, "same-user")
	attachment := uploadRoomAttachmentForUser(t, chatto, ctx, user.Id, room.Id, "owned.png")

	first, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "first",
		AttachmentAssetIDs: []string{attachment.Id},
	})
	require.NoError(t, err)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "second",
		AttachmentAssetIDs: []string{attachment.Id},
	})
	require.ErrorIs(t, err, ErrAssetNotAttachable)
	assertMessageStillOwnsAttachment(t, chatto, ctx, first.Event.Id, attachment.Id)
}

func TestDeletingLegacyAttachmentAliasDoesNotDeleteCanonicalAsset(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	victim, attacker, room := setupAttachmentOwnershipUsers(t, chatto, ctx, "legacy-partial-delete")
	attachment := uploadRoomAttachmentForUser(t, chatto, ctx, victim.Id, room.Id, "legacy.png")
	victimPost := appendLegacyAttachmentMessage(t, chatto, ctx, victim.Id, room.Id, attachment.Id, "victim")
	attackerPost := appendLegacyAttachmentMessage(t, chatto, ctx, attacker.Id, room.Id, attachment.Id, "legacy alias")

	err := chatto.Messages().DeleteAttachment(ctx, MessageAttachmentDeleteInput{
		ActorID: attacker.Id, RoomID: room.Id, EventID: attackerPost.Id, AttachmentID: attachment.Id,
	})
	require.NoError(t, err)
	require.False(t, chatto.assetModel.AssetDeleted(attachment.Id))
	assertMessageStillOwnsAttachment(t, chatto, ctx, victimPost.Id, attachment.Id)
	assertStoredAttachmentExists(t, chatto, ctx, attachment.Id)
}

func TestDeletingLegacyMessageAliasDoesNotDeleteCanonicalAsset(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	victim, attacker, room := setupAttachmentOwnershipUsers(t, chatto, ctx, "legacy-message-delete")
	attachment := uploadRoomAttachmentForUser(t, chatto, ctx, victim.Id, room.Id, "legacy.png")
	victimPost := appendLegacyAttachmentMessage(t, chatto, ctx, victim.Id, room.Id, attachment.Id, "victim")
	attackerPost := appendLegacyAttachmentMessage(t, chatto, ctx, attacker.Id, room.Id, attachment.Id, "legacy alias")

	err := chatto.Messages().DeleteMessage(ctx, MessageDeleteInput{
		ActorID: attacker.Id, RoomID: room.Id, EventID: attackerPost.Id,
	})
	require.NoError(t, err)
	require.False(t, chatto.assetModel.AssetDeleted(attachment.Id))
	assertMessageStillOwnsAttachment(t, chatto, ctx, victimPost.Id, attachment.Id)
	assertStoredAttachmentExists(t, chatto, ctx, attachment.Id)
}

func TestConcurrentMessagePostsAttachAssetOnceAcrossReplicas(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	user, _, room := setupAttachmentOwnershipUsers(t, chatto, ctx, "attachment-race")
	attachment := uploadRoomAttachmentForUser(t, chatto, ctx, user.Id, room.Id, "race.png")

	replica, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	require.NoError(t, err)
	startCoreServices(t, replica)

	cores := []*ChattoCore{chatto, replica}
	ready := make(chan struct{}, len(cores))
	release := make(chan struct{})
	errs := make(chan error, len(cores))
	var wg sync.WaitGroup
	for index, postingCore := range cores {
		wg.Add(1)
		go func(index int, postingCore *ChattoCore) {
			defer wg.Done()
			firstAttempt := true
			_, postErr := postingCore.PostMessage(
				ctx, KindChannel, room.Id, user.Id, "racing attachment", []string{attachment.Id}, "", "", nil, false,
				withPostMessageCommitAuthorization(func(attemptCtx context.Context, _ string) error {
					if firstAttempt {
						firstAttempt = false
						ready <- struct{}{}
						select {
						case <-release:
						case <-attemptCtx.Done():
							return attemptCtx.Err()
						}
					}
					return nil
				}),
			)
			errs <- postErr
		}(index, postingCore)
	}
	for range cores {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	wg.Wait()
	close(errs)

	var succeeded, rejected int
	for postErr := range errs {
		switch {
		case postErr == nil:
			succeeded++
		case errors.Is(postErr, ErrAssetNotAttachable):
			rejected++
		default:
			t.Fatalf("concurrent post error = %v", postErr)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, rejected)
}

func setupAttachmentOwnershipUsers(t *testing.T, chatto *ChattoCore, ctx context.Context, suffix string) (*evtv1.User, *evtv1.User, *evtv1.Room) {
	t.Helper()
	victim, err := chatto.CreateUser(ctx, SystemActorID, "av-"+suffix, "Asset Victim", "password123")
	require.NoError(t, err)
	attacker, err := chatto.CreateUser(ctx, SystemActorID, "aa-"+suffix, "Asset Attacker", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, victim.Id, KindChannel, "", "ar-"+suffix, "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, victim.Id, KindChannel, victim.Id, room.Id)
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, attacker.Id, KindChannel, attacker.Id, room.Id)
	require.NoError(t, err)
	return victim, attacker, room
}

func uploadRoomAttachmentForUser(t *testing.T, chatto *ChattoCore, ctx context.Context, userID, roomID, filename string) *evtv1.Attachment {
	t.Helper()
	content := createTestPNG(32, 32)
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	upload, err := chatto.AssetUploads().CreateUpload(ctx, AssetUploadCreateInput{
		ActorID: userID, RoomID: roomID, Filename: filename, ContentType: "image/png",
		Size: int64(len(content)), SHA256: digest,
	})
	require.NoError(t, err)
	_, err = chatto.AssetUploads().UploadChunk(ctx, AssetUploadChunkInput{
		ActorID: userID, UploadID: upload.UploadID, Content: content, ChunkSHA256: digest,
	})
	require.NoError(t, err)
	_, attachment, err := chatto.AssetUploads().CompleteUpload(ctx, AssetUploadCompleteInput{
		ActorID: userID, UploadID: upload.UploadID,
	})
	require.NoError(t, err)
	return attachment
}

// appendLegacyAttachmentMessage recreates history from before explicit asset
// attachment events. It intentionally bypasses today's command validation so
// deletion behavior remains safe for data written before the fix.
func appendLegacyAttachmentMessage(t *testing.T, chatto *ChattoCore, ctx context.Context, actorID, roomID, assetID, plaintext string) *evtv1.Event {
	t.Helper()
	now := time.Now()
	eventID := NewEventID()
	bodyEventID := NewEventID()
	body := &evtv1.MessageBody{
		CreatedAt: timestamppb.New(now),
		AssetIds:  []string{assetID},
		AuthorId:  actorID,
	}
	require.NoError(t, chatto.encryptMessageBody(ctx, body, roomID, eventID, bodyEventID, plaintext))
	bodyEvent := newEvent(actorID, &evtv1.Event{
		Id: bodyEventID, CreatedAt: timestamppb.New(now),
		Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{
			RoomId: roomID, EventId: eventID, Body: body,
		}},
	})
	messageEvent := newEvent(actorID, &evtv1.Event{
		Id: eventID, CreatedAt: timestamppb.New(now),
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: roomID}},
	})

	aggregate := evtstream.RoomAggregate(roomID)
	roomFilter := aggregate.AllEventsFilter()
	roomSeq, err := chatto.EventPublisher.LastSubjectSeq(ctx, roomFilter)
	require.NoError(t, err)
	bodySubject := aggregate.SubjectFor(bodyEvent)
	messageSubject := aggregate.SubjectFor(messageEvent)
	sequences, err := chatto.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{
		{
			Subject: bodySubject, Event: bodyEvent, HasOCC: true,
			ExpectedSeq: roomSeq, FilterSubject: roomFilter,
		},
		{Subject: messageSubject, Event: messageEvent},
	})
	require.NoError(t, err)
	require.NoError(t, chatto.waitForMessageBodyAssets(ctx, bodySubject, sequences[0]))
	require.NoError(t, chatto.roomModel.waitForTimeline(ctx, events.SubjectPosition(messageSubject, sequences[1])))
	return messageEvent
}

func assertMessageStillOwnsAttachment(t *testing.T, chatto *ChattoCore, ctx context.Context, eventID, assetID string) {
	t.Helper()
	body, err := chatto.GetFullMessageBody(ctx, eventID)
	require.NoError(t, err)
	require.NotNil(t, body)
	require.Len(t, body.Attachments, 1)
	require.Equal(t, assetID, body.Attachments[0].Id)
}

func assertStoredAttachmentExists(t *testing.T, chatto *ChattoCore, ctx context.Context, assetID string) {
	t.Helper()
	store, err := chatto.mediaModel.GetAttachmentsStore(ctx)
	require.NoError(t, err)
	object, err := store.Get(ctx, assetID)
	require.NoError(t, err)
	require.NoError(t, object.Close())
}
