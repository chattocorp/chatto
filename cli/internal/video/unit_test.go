package video

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

func TestStandaloneMediaWorkerConsumesSharedJobs(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := t.Context()
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, MaxMsgsPerSubject: 1})
	require.NoError(t, err)
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME_STATE"})
	require.NoError(t, err)
	queue, consumer, err := openJobs(ctx, runtimeunit.Env{NC: nc, JS: js, Logger: log.New(io.Discard)})
	require.NoError(t, err)
	payload, err := proto.Marshal(&statev1.MediaJob{AssetId: "asset", MessageEventId: "message"})
	require.NoError(t, err)
	require.NoError(t, queue.Enqueue(ctx, &statev1.JobRecord{Id: "media-job", Type: "media", Payload: payload, MaxAttempts: 5, RetryDelayMillis: 1000}))
	require.NoError(t, queue.Enqueue(ctx, &statev1.JobRecord{Id: "webhook-job", Type: "webhook", MaxAttempts: 3, RetryDelayMillis: 1000}))
	processed := make(chan *statev1.MediaJob, 1)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- queue.Run(runCtx, consumer, map[string]jobqueue.Handler{"media": func(_ context.Context, job *statev1.JobRecord) error {
			request := new(statev1.MediaJob)
			err := proto.Unmarshal(job.Payload, request)
			processed <- request
			return err
		}}, 1)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("standalone worker did not stop")
		}
	})
	select {
	case request := <-processed:
		require.Equal(t, "asset", request.AssetId)
		require.Equal(t, "message", request.MessageEventId)
	case <-time.After(3 * time.Second):
		t.Fatal("media not processed")
	}
	job, err := queue.Get(ctx, "webhook-job")
	require.NoError(t, err)
	require.Equal(t, jobqueue.Queued, job.Status)
	require.Zero(t, job.Attempts)
}
