package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
)

func TestMediaSourceHandsCurrentAndLegacyEventsToSharedPool(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	processed := make(chan string, 4)
	c.SetMediaJobHandler(func(_ context.Context, assetID, messageID string) error {
		processed <- assetID + ":" + messageID
		return nil
	}, 1)
	startCoreServices(t, c)
	for i, subject := range []string{"evt.asset.asset-current.asset_processing_started", "evt.room.room-legacy.asset_processing_started"} {
		id := []string{"current", "legacy"}[i]
		event := &evtv1.Event{Id: id, CreatedAt: timestamppb.Now(), Event: &evtv1.Event_AssetProcessingStarted{AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{AssetId: "asset-" + id, MessageEventId: "message-" + id}}}
		data, err := proto.Marshal(event)
		require.NoError(t, err)
		_, err = c.js.Publish(ctx, subject, data)
		require.NoError(t, err)
		select {
		case result := <-processed:
			require.Equal(t, "asset-"+id+":message-"+id, result)
		case <-time.After(3 * time.Second):
			t.Fatal("media work was not dispatched")
		}
		var job *statev1.JobRecord
		require.Eventually(t, func() bool {
			job, err = c.jobs.Get(ctx, jobqueue.ID("media", id))
			return err == nil && job.Status == jobqueue.Succeeded
		}, time.Second, time.Millisecond)
		// Replayed source work cannot recreate a completed job.
		consumer, err := c.storage.serverEvtStream.Consumer(ctx, AssetProcessingConsumerName)
		require.NoError(t, err)
		info, err := consumer.Info(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{evtstream.AssetEventTypeFilter(evtstream.EventAssetProcessingStarted), evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted)}, info.Config.FilterSubjects)
	}
	jobs, _, err := c.jobs.List(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}
