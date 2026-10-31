package core

import (
	"sync"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHistoricalReactionBatch(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "batch-author", "Author", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "batch-room", "")
	require.NoError(t, err)
	created := time.Date(2020, 3, 14, 12, 0, 0, 0, time.UTC)
	post, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: created, Body: "body"})
	require.NoError(t, err)
	tombstone, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: created, Deleted: true})
	require.NoError(t, err)
	makeChange := func() HistoricalMessageChange {
		return HistoricalMessageChange{RoomID: room.Id, MessageID: post.Id, OperationKey: uuid.NewString(), Kind: "reaction", UserID: user.Id, Emoji: "❤️", Present: true}
	}
	add, remove, deleted := makeChange(), makeChange(), makeChange()
	remove.Present = false
	deleted.MessageID = tombstone.Id
	changes := []HistoricalMessageChange{add, remove, deleted}
	require.NoError(t, c.ImportHistoricalReactionChanges(ctx, changes))
	reactions, err := c.GetReactions(ctx, post.Id)
	require.NoError(t, err)
	require.Empty(t, reactions, "add then remove must keep request order")
	reactions, err = c.GetReactions(ctx, tombstone.Id)
	require.NoError(t, err)
	require.Equal(t, []ReactionSummary{{Emoji: "heart", UserIDs: []string{user.Id}}}, reactions)
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	for _, change := range changes {
		require.NoError(t, c.ImportHistoricalMessageChange(ctx, change), "batch receipts work through the old RPC")
	}
	require.NoError(t, c.ImportHistoricalReactionChanges(ctx, changes))
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	for _, tc := range []struct {
		name string
		edit func(*HistoricalMessageChange)
	}{
		{"missing user", func(in *HistoricalMessageChange) { in.UserID = "missing" }},
		{"missing actor", func(in *HistoricalMessageChange) { in.ActorID = "missing" }},
		{"missing message", func(in *HistoricalMessageChange) { in.MessageID = "missing" }},
		{"other room", func(in *HistoricalMessageChange) { in.RoomID = "missing" }},
		{"bad emoji", func(in *HistoricalMessageChange) { in.Emoji = "invalid emoji name" }},
		{"wrong operation", func(in *HistoricalMessageChange) { in.Kind = "edit" }},
		{"bad key", func(in *HistoricalMessageChange) { in.OperationKey = "invalid" }},
		{"early time", func(in *HistoricalMessageChange) { in.OccurredAt = new(created.Add(-time.Second)) }},
		{"changed retry", func(in *HistoricalMessageChange) { in.OperationKey = add.OperationKey; in.Emoji = "thumbsup" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, bad := makeChange(), makeChange()
			tc.edit(&bad)
			require.Error(t, c.ImportHistoricalReactionChanges(ctx, []HistoricalMessageChange{first, bad}))
			after, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after, "a bad later item must not commit the earlier item")
		})
	}
	duplicate := makeChange()
	require.Error(t, c.ImportHistoricalReactionChanges(ctx, []HistoricalMessageChange{duplicate, duplicate}))
	require.Error(t, c.ImportHistoricalReactionChanges(ctx, nil))
	require.Error(t, c.ImportHistoricalReactionChanges(ctx, make([]HistoricalMessageChange, MaxHistoricalReactionChanges+1)))
	// Existing single-operation receipts can be mixed with new batch items.
	newChange := makeChange()
	require.NoError(t, c.ImportHistoricalMessageChange(ctx, newChange))
	next := makeChange()
	next.Emoji = "thumbsup"
	require.NoError(t, c.ImportHistoricalReactionChanges(ctx, []HistoricalMessageChange{newChange, next}))
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventReactionAdded))
	require.NoError(t, err)
	require.Len(t, facts, 4)
	for _, fact := range facts {
		require.False(t, isDeliverableLiveEVTRoomEvent(fact))
		require.True(t, fact.GetImportContext().GetUnknownTime())
	}
}

func TestHistoricalReactionBatchConcurrentReplicas(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "batch-author", "Author", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "batch-room", "")
	require.NoError(t, err)
	post, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: time.Now().Add(-time.Hour), Body: "body"})
	require.NoError(t, err)
	replica := startReplica(t, ns, testCoreConfig())
	changes := []HistoricalMessageChange{}
	for range MaxHistoricalReactionChanges {
		changes = append(changes, HistoricalMessageChange{RoomID: room.Id, MessageID: post.Id, OperationKey: uuid.NewString(), Kind: "reaction", UserID: user.Id, Emoji: "heart", Present: true})
	}
	var wg sync.WaitGroup
	for _, writer := range []*ChattoCore{c, replica} {
		wg.Go(func() {
			if err := writer.ImportHistoricalReactionChanges(ctx, changes); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventReactionAdded))
	require.NoError(t, err)
	require.Len(t, facts, MaxHistoricalReactionChanges, "one fact per key across replicas")
}

func TestHistoricalReactionBatchRejectsLiveTargets(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, liveID := setupReactionTest(t, c, ctx)
	post, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: time.Now().Add(-time.Hour), Body: "body"})
	require.NoError(t, err)
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	changes := []HistoricalMessageChange{
		{RoomID: room.Id, MessageID: post.Id, OperationKey: uuid.NewString(), Kind: "reaction", UserID: user.Id, Emoji: "heart", Present: true},
		{RoomID: room.Id, MessageID: liveID, OperationKey: uuid.NewString(), Kind: "reaction", UserID: user.Id, Emoji: "heart", Present: true},
	}
	require.Error(t, c.ImportHistoricalReactionChanges(ctx, changes))
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
