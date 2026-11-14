package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
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
		var job jobqueue.Job
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

// Job completion follows the asset lifecycle, rather than merely trusting a
// nil processor return. A terminal failed conversion is completed work.
func TestMediaJobRequiresTerminalAssetOutcome(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	calls := 0
	err := c.ProcessMediaJob(ctx, "asset-terminal", "message", func(context.Context, string, string) error { calls++; return nil })
	require.ErrorContains(t, err, "no terminal outcome")
	require.Equal(t, 1, calls)
	err = c.ProcessMediaJob(ctx, "asset-terminal", "message", func(ctx context.Context, _, _ string) error {
		calls++
		event := &evtv1.Event{Id: "terminal-failure", CreatedAt: timestamppb.Now(), Event: &evtv1.Event_AssetProcessingFailed{AssetProcessingFailed: &evtv1.AssetProcessingFailedEvent{AssetId: "asset-terminal"}}}
		data, err := proto.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = c.js.Publish(ctx, "evt.room.room.asset_processing_failed", data); err != nil {
			return err
		}
		if err = c.assetModel.waitForAssetsCurrent(ctx); err != nil {
			return err
		}
		return errors.New("conversion failed after outcome persistence")
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.NoError(t, c.ProcessMediaJob(ctx, "asset-terminal", "message", func(context.Context, string, string) error {
		t.Error("terminal asset must not be processed again")
		return nil
	}))
}
