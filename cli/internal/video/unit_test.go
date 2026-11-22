package video

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/core"
	jobv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/job/v1"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

func TestStandaloneMediaWorkerRunsOnlyMediaJobs(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := t.Context()
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, AllowMsgTTL: true})
	require.NoError(t, err)
	queue, consumer, err := openJobs(ctx, runtimeunit.Env{NC: nc, JS: js, Logger: log.New(io.Discard)})
	require.NoError(t, err)
	payload, err := proto.Marshal(&jobv1.MediaJob{AssetId: "asset", MessageEventId: "message"})
	require.NoError(t, err)
	require.NoError(t, queue.Enqueue(ctx, jobqueue.Spec{ID: "media-job", Type: "media", Payload: payload, MaxAttempts: 5, RetryDelay: time.Second}))
	require.NoError(t, queue.Enqueue(ctx, jobqueue.Spec{ID: "webhook-job", Type: "webhook", MaxAttempts: 3, RetryDelay: time.Second}))
	processed := make(chan *jobv1.MediaJob, 1)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- queue.Work(runCtx, consumer, func(_ context.Context, job jobqueue.Job) error {
			request := new(jobv1.MediaJob)
			err := proto.Unmarshal(job.Payload, request)
			processed <- request
			return err
		}, 1)
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
	// The media consumer never receives the job of another type.
	stream, err := js.Stream(ctx, "JOBS")
	require.NoError(t, err)
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter("jobs.webhook.>"))
	require.NoError(t, err)
	require.Equal(t, map[string]uint64{"jobs.webhook.webhook-job.queued": 1}, info.State.Subjects)
}

// mediaStartupProbe matches the JSON log fragment used by deployed worker probes.
type mediaStartupProbe struct{ ready context.CancelFunc }

func (probe mediaStartupProbe) Write(line []byte) (int, error) {
	if bytes.Contains(line, []byte(`"msg":"Asset-processing worker started"`)) {
		probe.ready()
	}
	return len(line), nil
}

func TestMediaStartupProbeRequiresJobConsumer(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVT", Subjects: []string{"evt.>"}})
	require.NoError(t, err)
	_, err = js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "SERVER_ASSETS"})
	require.NoError(t, err)

	runCtx, stopOnReady := context.WithCancel(ctx)
	defer stopOnReady()
	logger := log.New(&mediaStartupProbe{ready: stopOnReady})
	logger.SetFormatter(log.JSONFormatter)
	env := runtimeunit.Env{NC: nc, JS: js, Logger: logger}
	// No jobs are submitted, so neither media tool will be invoked.
	env.Config.AssetProcessing.FFmpegPath, err = os.Executable()
	require.NoError(t, err)
	env.Config.AssetProcessing.FFprobePath = env.Config.AssetProcessing.FFmpegPath

	require.ErrorIs(t, (Unit{}).Run(runCtx, env), jetstream.ErrStreamNotFound)
	require.NoError(t, runCtx.Err(), "a failed startup must not signal readiness")

	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, AllowMsgTTL: true})
	require.NoError(t, err)
	require.NoError(t, (Unit{}).Run(runCtx, env))
	require.ErrorIs(t, runCtx.Err(), context.Canceled, "the probe must see the startup message")
	_, err = stream.Consumer(ctx, core.JobsConsumerName(core.JobTypeMedia))
	require.NoError(t, err)
}
