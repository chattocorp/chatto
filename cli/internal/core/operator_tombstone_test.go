package core

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

func TestHistoricalTombstoneConcurrentReplayAndReplies(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	user, err := c.CreateUser(testContext(t), SystemActorID, "tombstone-author", "Author", "")
	require.NoError(t, err)
	room, err := c.CreateRoom(testContext(t), user.Id, KindChannel, "", "historical", "")
	require.NoError(t, err)
	post := MessagePostInput{RoomID: room.Id, ActorID: user.Id}
	ctx := testContext(t)
	replica, err := NewChattoCore(ctx, newReplicaConn(t, ns), c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	input := HistoricalMessageInput{RoomID: post.RoomID, AuthorID: post.ActorID,
		CreatedAt: time.Unix(1500000000, 123456000).UTC(), Deleted: true, CreateThread: true, IdempotencyKey: uuid.NewString()}
	type outcome struct {
		event *evtv1.Event
		err   error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			event, err := []*ChattoCore{c, replica}[i%2].ImportHistoricalMessage(ctx, input)
			results <- outcome{event, err}
		})
	}
	wg.Wait()
	close(results)
	var rootID string
	for result := range results {
		require.NoError(t, result.err)
		if rootID == "" {
			rootID = result.event.Id
		}
		require.Equal(t, rootID, result.event.Id)
		require.Equal(t, input.CreatedAt, result.event.CreatedAt.AsTime())
	}
	for eventType, count := range map[string]int{evtstream.EventMessagePosted: 1, evtstream.EventMessageRetracted: 1,
		evtstream.EventThreadCreated: 1, evtstream.EventMessageBody: 0} {
		stored, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(post.RoomID).Subject(eventType))
		require.NoError(t, err)
		require.Len(t, stored, count)
		for _, event := range stored {
			if event.GetMessageRetracted() != nil {
				require.True(t, isDeliverableLiveEVTRoomEvent(event))
			} else {
				require.False(t, isDeliverableLiveEVTRoomEvent(event))
			}
		}
	}
	require.True(t, c.roomModel.messageTombstoned(rootID))
	reply, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: post.RoomID, AuthorID: post.ActorID,
		CreatedAt: input.CreatedAt.Add(time.Second), Body: "surviving reply", ThreadRootEventID: rootID})
	require.NoError(t, err)
	page, err := c.GetThreadReplyEvents(ctx, KindChannel, post.RoomID, rootID, 10, nil, nil)
	require.NoError(t, err)
	require.Len(t, page.Events, 1)
	require.Equal(t, reply.Id, page.Events[0].Id)
	restarted, err := NewChattoCore(ctx, newReplicaConn(t, ns), c.config)
	require.NoError(t, err)
	startCoreServices(t, restarted)
	replayed, err := restarted.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, rootID, replayed.Id)
	require.True(t, restarted.roomModel.messageTombstoned(rootID))
	require.Equal(t, c.GetThreadMetadata(rootID), restarted.GetThreadMetadata(rootID))
	changed := input
	changed.Deleted, changed.Body = false, "restored"
	_, err = c.ImportHistoricalMessage(ctx, changed)
	require.ErrorIs(t, err, ErrMessageIdempotencyConflict)
}

func TestHistoricalTombstoneContentValidationAndLazyThread(t *testing.T) {
	t.Parallel()
	c, _, post := setupMessageRetry(t)
	ctx := testContext(t)
	input := HistoricalMessageInput{RoomID: post.RoomID, AuthorID: post.ActorID, CreatedAt: time.Now().UTC(), Deleted: true}
	for _, change := range []func(*HistoricalMessageInput){
		func(v *HistoricalMessageInput) { v.Body = "discard me" },
		func(v *HistoricalMessageInput) { v.AttachmentAssetIDs = []string{"unknown"} },
		func(v *HistoricalMessageInput) { v.LinkPreview = &evtv1.LinkPreview{Url: "https://example.org"} },
		func(v *HistoricalMessageInput) { v.EditedAt = &v.CreatedAt },
	} {
		invalid := input
		change(&invalid)
		_, err := c.ImportHistoricalMessage(ctx, invalid)
		require.ErrorContains(t, err, "deleted imports must not contain")
	}
	root, err := c.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	input.ThreadRootEventID = root.Id
	reply, err := c.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.True(t, c.roomModel.messageTombstoned(reply.Id))
	require.True(t, c.GetThreadMetadata(root.Id).Exists)
	input.Deleted, input.Body = false, "visible"
	_, err = c.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
}
