package core

import (
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestOperatorOperationsAcrossReplicas(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	a := startReplica(t, ns, testCoreConfig())
	b := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	key := uuid.NewString()
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for _, c := range []*ChattoCore{a, b} {
		go func() {
			u, err := c.CreateHistoricalOperatorUser(ctx, key, "import-user", "Import User", "import@example.test")
			errs <- err
			if err == nil {
				ids <- u.Id
			}
		}()
	}
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	userID := <-ids
	require.Equal(t, userID, <-ids)
	_, err := a.CreateHistoricalOperatorUser(ctx, key, "changed", "Import User", "import@example.test")
	require.ErrorContains(t, err, "different arguments")
	roomKey := uuid.NewString()
	room, err := a.CreateHistoricalOperatorRoom(ctx, roomKey, "", "import-room", "")
	require.NoError(t, err)
	other, err := b.CreateHistoricalOperatorRoom(ctx, roomKey, "", "import-room", "")
	require.NoError(t, err)
	require.Equal(t, room.Id, other.Id)
	input := HistoricalMessageInput{RoomID: room.Id, AuthorID: userID, CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Body: "before", OperationKey: uuid.NewString()}
	for _, c := range []*ChattoCore{a, b} {
		go func() {
			e, err := c.ImportHistoricalMessage(ctx, input)
			errs <- err
			if err == nil {
				ids <- e.Id
			}
		}()
	}
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	message := <-ids
	require.Equal(t, message, <-ids)
	change := HistoricalMessageChange{RoomID: room.Id, MessageID: message, OperationKey: uuid.NewString(), Kind: "edit", Body: "after"}
	require.NoError(t, a.ImportHistoricalMessageChange(ctx, change))
	require.NoError(t, b.ImportHistoricalMessageChange(ctx, change))
	change.Body = "different"
	require.ErrorContains(t, b.ImportHistoricalMessageChange(ctx, change), "different arguments")
	join := HistoricalRoomChange{RoomID: room.Id, OperationKey: uuid.NewString(), Kind: "joined", Value: userID, OccurredAt: &input.CreatedAt}
	require.NoError(t, a.ImportHistoricalRoomChange(ctx, join))
	require.NoError(t, b.ImportHistoricalRoomChange(ctx, join))
	facts, _, err := a.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).AllEventsFilter())
	require.NoError(t, err)
	posts, edits, joins := 0, 0, 0
	for _, e := range facts {
		if e.GetMessagePosted() != nil {
			posts++
		}
		if e.GetMessageEdited() != nil {
			edits++
		}
		if e.GetUserJoinedRoom() != nil {
			joins++
			require.Equal(t, input.CreatedAt, e.CreatedAt.AsTime())
		}
	}
	require.Equal(t, 1, posts)
	require.Equal(t, 1, edits)
	require.Equal(t, 1, joins)
	// A fresh process reconstructs the receipt from EVT, not a local TTL cache.
	restarted := startReplica(t, ns, testCoreConfig())
	e, err := restarted.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, message, e.Id)
	deletion := HistoricalMessageChange{RoomID: room.Id, MessageID: message, OperationKey: uuid.NewString(), Kind: "deleted"}
	require.NoError(t, restarted.ImportHistoricalMessageChange(ctx, deletion))
	change.OperationKey = uuid.NewString()
	require.ErrorContains(t, restarted.ImportHistoricalMessageChange(ctx, change), "deleted")
}
