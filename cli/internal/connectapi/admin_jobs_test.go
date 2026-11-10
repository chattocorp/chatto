package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
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
	kv, err := js.KeyValue(ctx, "RUNTIME_STATE")
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		deadline := time.Now().Add(time.Hour)
		if id == "c" {
			deadline = time.Now().Add(-time.Hour)
		}
		job := &statev1.JobRecord{Id: id, Type: "unregistered-test", Status: statev1.JobExecutionStatus_JOB_EXECUTION_STATUS_FAILED, MaxAttempts: 3, Attempts: 3, Generation: 1, RetryDelayMillis: 1000, Payload: []byte("SECRET-PAYLOAD"), Owner: "SECRET-CLAIM", CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now(), ExpiresAt: timestamppb.New(deadline), Failure: "http_error"}
		data, err := proto.Marshal(job)
		require.NoError(t, err)
		_, err = kv.Put(ctx, "job."+id, data)
		require.NoError(t, err)
	}
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
	retried, err := service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "a"}))
	require.NoError(t, err)
	require.Equal(t, adminv1.JobStatus_JOB_STATUS_QUEUED, retried.Msg.Job.Status)
	require.Zero(t, retried.Msg.Job.Attempts)
	_, err = service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "a"}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	_, err = service.RetryJob(ctx, connect.NewRequest(&adminv1.RetryJobRequest{Id: "c"}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
}
