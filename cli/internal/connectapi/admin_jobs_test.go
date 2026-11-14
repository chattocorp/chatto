package connectapi

import (
	"context"
	"io"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

func TestJobsAPIRequiresPermissionAndPrivilegedMode(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &jobsService{api: env.api}
	_, err := service.ListJobs(env.ctx, connect.NewRequest(&adminv1.ListJobsRequest{}))
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
	_, err = service.GetJob(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetJobRequest{Id: "missing"}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = service.RetryJob(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.RetryJobRequest{Id: "missing"}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermServerJobsManage))
	// Authority alone is insufficient for a human bearer session.
	_, err = service.ListJobs(withBearerCredential(env.ctx, env.viewer, "unarmed"), connect.NewRequest(&adminv1.ListJobsRequest{}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	ctx := withArmedBearerCredential(env.ctx, env.viewer, "armed")
	response, err := service.ListJobs(ctx, connect.NewRequest(&adminv1.ListJobsRequest{}))
	require.NoError(t, err)
	require.Empty(t, response.Msg.Jobs)
	_, err = service.GetJob(ctx, connect.NewRequest(&adminv1.GetJobRequest{Id: "missing"}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}
func TestJobsAPISafeSummaryPaginationAndRetry(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &jobsService{api: env.api}
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermServerJobsManage))
	ctx := withArmedBearerCredential(env.ctx, env.viewer, "armed")
	js, err := jetstream.New(env.nc)
	require.NoError(t, err)
	bucket, err := js.KeyValue(ctx, "RUNTIME_STATE")
	require.NoError(t, err)
	kv, err := jetstreamutil.NewKeyValue(js, bucket)
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "JOBS")
	require.NoError(t, err)
	// A separate queue handle with Chatto's contract makes three real failed
	// jobs of a type that the server does not handle. Job c is already expired.
	queue, err := core.NewJobQueue(js, kv, log.New(io.Discard), nil)
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		deadline := time.Now().Add(time.Hour)
		if id == "c" {
			deadline = time.Now().Add(-time.Hour)
		}
		require.NoError(t, queue.Enqueue(ctx, jobqueue.Spec{ID: id, Type: "unregistered-test", MaxAttempts: 3, RetryDelay: time.Second, Payload: []byte("SECRET-PAYLOAD"), ExpiresAt: deadline}))
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, core.JobsConsumerConfig(queue, "test-unregistered", "unregistered-test"))
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- queue.Run(runCtx, consumer, map[string]jobqueue.Handler{"unregistered-test": func(context.Context, jobqueue.Job) error {
			return &jobqueue.Failure{Category: "http_error", Permanent: true}
		}}, 3)
	}()
	require.Eventually(t, func() bool {
		jobs, _, err := queue.List(ctx, "", 10)
		if err != nil || len(jobs) != 3 {
			return false
		}
		for _, job := range jobs {
			if job.Status != jobqueue.Failed {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)
	stop()
	require.NoError(t, <-done)
	first, err := service.ListJobs(ctx, connect.NewRequest(&adminv1.ListJobsRequest{PageSize: 2}))
	require.NoError(t, err)
	require.Len(t, first.Msg.Jobs, 2)
	require.NotEmpty(t, first.Msg.NextPageToken)
	encoded, err := protojson.Marshal(first.Msg)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "SECRET")
	require.NotContains(t, string(encoded), "payload")
	require.NotContains(t, string(encoded), "generation")
	next, err := service.ListJobs(ctx, connect.NewRequest(&adminv1.ListJobsRequest{PageSize: 2, PageToken: first.Msg.NextPageToken}))
	require.NoError(t, err)
	require.Len(t, next.Msg.Jobs, 1)
	require.Empty(t, next.Msg.NextPageToken)
	require.Equal(t, adminv1.RetryJobState_RETRY_JOB_STATE_EXPIRED, next.Msg.Jobs[0].GetRetryState())
	_, err = service.ListJobs(ctx, connect.NewRequest(&adminv1.ListJobsRequest{PageToken: "tampered"}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	retried, err := service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "a", RetryVersion: first.Msg.Jobs[0].RetryVersion}))
	require.NoError(t, err)
	require.Equal(t, adminv1.JobStatus_JOB_STATUS_QUEUED, retried.Msg.Job.Status)
	require.Zero(t, retried.Msg.Job.Attempts)
	_, err = service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "a", RetryVersion: first.Msg.Jobs[0].RetryVersion}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	_, err = service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "c", RetryVersion: next.Msg.Jobs[0].RetryVersion}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
}
