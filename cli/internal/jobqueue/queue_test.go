package jobqueue

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	jobv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/job/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

func testQueue(t *testing.T) (*Queue, jetstream.Consumer) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx := t.Context()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, Storage: jetstream.MemoryStorage, MaxMsgsPerSubject: 1})
	require.NoError(t, err)
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME_STATE"})
	require.NoError(t, err)
	bound, err := jetstreamutil.NewKeyValue(js, kv)
	require.NoError(t, err)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, ConsumerConfig(ConsumerName, []string{"webhook", "media"}))
	require.NoError(t, err)
	return New(js, bound, log.New(io.Discard)), consumer
}
func testJob(id string) *statev1.JobRecord {
	return &statev1.JobRecord{Id: id, Type: "webhook", MaxAttempts: 3, RetryDelayMillis: 1, Payload: []byte("opaque-reference")}
}
func dispatch(job *statev1.JobRecord) events.DurableDelivery {
	data, _ := proto.Marshal(&jobv1.JobDispatch{Id: job.Id, Generation: job.Generation})
	return events.DurableDelivery{Data: data}
}
func runQueue(t *testing.T, q *Queue, c jetstream.Consumer, h map[string]Handler, concurrency int) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx, c, h, concurrency) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("worker failed to stop")
		}
	})
	return cancel
}
func awaitStatus(t *testing.T, q *Queue, id string, status statev1.JobExecutionStatus) *statev1.JobRecord {
	t.Helper()
	var job *statev1.JobRecord
	require.Eventually(t, func() bool {
		var err error
		job, err = q.Get(t.Context(), id)
		return err == nil && job.Status == status
	}, 5*time.Second, 5*time.Millisecond)
	return job
}
func TestQueueRetriesWithoutOccupyingOnlyWorker(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	var calls atomic.Int32
	slow := testJob("a-slow")
	slow.RetryDelayMillis = 1000
	require.NoError(t, q.Enqueue(ctx, slow))
	require.NoError(t, q.Enqueue(ctx, testJob("b-fast")))
	runQueue(t, q, c, map[string]Handler{"webhook": func(_ context.Context, job *statev1.JobRecord) error {
		if job.Id == slow.Id && calls.Add(1) == 1 {
			return errors.New("external secret must not persist")
		}
		return nil
	}}, 1)
	awaitStatus(t, q, "b-fast", Succeeded)
	first := awaitStatus(t, q, slow.Id, Queued)
	require.Equal(t, "attempt_failed", first.Failure)
	terminal := awaitStatus(t, q, slow.Id, Succeeded)
	require.Equal(t, uint32(2), terminal.Attempts)
	// Source replay cannot reset work or alter its original payload.
	duplicate := testJob(slow.Id)
	duplicate.Payload = []byte("changed")
	require.NoError(t, q.Enqueue(ctx, duplicate))
	stored, err := q.Get(ctx, slow.Id)
	require.NoError(t, err)
	require.Equal(t, []byte("opaque-reference"), stored.Payload)
	require.Equal(t, Succeeded, stored.Status)
}
func TestQueueRetainsFailureAndManualRetryGeneration(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	job := testJob("failure")
	job.MaxAttempts = 1
	require.NoError(t, q.Enqueue(ctx, job))
	job, _ = q.Get(ctx, job.Id)
	require.NoError(t, q.process(ctx, dispatch(job), map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { return &Failure{Category: "http_error"} }}))
	failed, _ := q.Get(ctx, job.Id)
	require.Equal(t, Failed, failed.Status)
	require.Equal(t, "http_error", failed.Failure)
	retried, err := q.Retry(ctx, job.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), retried.Generation)
	require.Zero(t, retried.Attempts)
	// An old, unconfirmed source delivery cannot execute a fresh generation.
	var calls int
	h := map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { calls++; return nil }}
	require.NoError(t, q.process(ctx, dispatch(failed), h))
	require.Zero(t, calls)
	require.NoError(t, q.process(ctx, dispatch(retried), h))
	require.Equal(t, 1, calls)
	require.NoError(t, q.process(ctx, dispatch(retried), h))
	require.Equal(t, 1, calls, "lost acknowledgement repeats no completed handler")
	_, err = q.Retry(ctx, job.Id)
	require.ErrorIs(t, err, ErrNotRetryable)
}
func TestQueueConcurrentClaimsAndRetry(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	job := testJob("concurrent")
	job.MaxAttempts = 1
	require.NoError(t, q.Enqueue(ctx, job))
	job, _ = q.Get(ctx, job.Id)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	h := map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error {
		calls.Add(1)
		close(entered)
		<-release
		return &Failure{Category: "unavailable"}
	}}
	done := make(chan error, 1)
	go func() { done <- q.process(ctx, dispatch(job), h) }()
	<-entered
	require.Error(t, q.process(ctx, dispatch(job), h))
	require.Equal(t, int32(1), calls.Load())
	close(release)
	require.NoError(t, <-done)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := q.Retry(ctx, job.Id)
			if err == nil {
				accepted.Add(1)
			} else {
				require.ErrorIs(t, err, ErrNotRetryable)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), accepted.Load())
}
func TestQueueRepairsRecordWithoutDispatch(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	job := testJob("outbox")
	job.Status = Queued
	job.Generation = 1
	job.CreatedAt = timestamppb.Now()
	job.NextAttemptAt = timestamppb.Now()
	data, err := proto.Marshal(job)
	require.NoError(t, err)
	_, err = q.kv.Create(ctx, jobKey(job.Id), data)
	require.NoError(t, err)
	runQueue(t, q, c, map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { return nil }}, 1)
	awaitStatus(t, q, job.Id, Succeeded)
}
func TestQueueRecoversExpiredClaimAndPreservesDeadline(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	job := testJob("interrupted")
	require.NoError(t, q.Enqueue(ctx, job))
	job, rev, err := q.read(ctx, job.Id)
	require.NoError(t, err)
	job.Status = Running
	job.Attempts = 1
	job.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
	_, err = q.save(ctx, job, rev)
	require.NoError(t, err)
	calls := 0
	h := map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { calls++; return nil }}
	require.NoError(t, q.process(ctx, dispatch(job), h))
	require.Equal(t, 1, calls)
	expired := testJob("expired")
	expired.ExpiresAt = timestamppb.New(time.Now().Add(-time.Minute))
	require.NoError(t, q.Enqueue(ctx, expired))
	expired, _ = q.Get(ctx, expired.Id)
	require.NoError(t, q.process(ctx, dispatch(expired), h))
	require.Equal(t, 1, calls)
	expired, _ = q.Get(ctx, expired.Id)
	require.Equal(t, Failed, expired.Status)
	require.Equal(t, "expired", expired.Failure)
	_, err = q.Retry(ctx, expired.Id)
	require.ErrorIs(t, err, ErrNotRetryable)
}
func TestQueueListFiltersOtherRuntimeState(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	_, err := q.kv.Put(ctx, "other", []byte("private"))
	require.NoError(t, err)
	for _, id := range []string{"z", "b", "a"} {
		require.NoError(t, q.Enqueue(ctx, testJob(id)))
	}
	first, more, err := q.List(ctx, "", 2)
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, "a", first[0].Id)
	require.Equal(t, "b", first[1].Id)
	last, more, err := q.List(ctx, "b", 2)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, last, 1)
	require.Equal(t, "z", last[0].Id)
}

// A retry may be accepted even when dispatch publication fails. Recovery must
// use the accepted generation instead of rejecting or resetting that retry.
func TestQueueRepairsAcceptedRetryAfterPublishFailure(t *testing.T) {
	q, consumer := testQueue(t)
	ctx := t.Context()
	require.NoError(t, q.Enqueue(ctx, testJob("retry-outbox")))
	job, rev, err := q.read(ctx, "retry-outbox")
	require.NoError(t, err)
	job.Status = Failed
	_, err = q.save(ctx, job, rev)
	require.NoError(t, err)
	publisher := &failNextPublish{JetStream: q.js}
	publisher.fail.Store(true)
	q.js = publisher
	accepted, err := q.Retry(ctx, job.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), accepted.Generation)
	runQueue(t, q, consumer, map[string]Handler{"webhook": func(_ context.Context, attempt *statev1.JobRecord) error {
		require.Equal(t, uint64(2), attempt.Generation)
		return nil
	}}, 1)
	awaitStatus(t, q, job.Id, Succeeded)
}

type failNextPublish struct {
	jetstream.JetStream
	fail atomic.Bool
}

func (p *failNextPublish) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if p.fail.Swap(false) {
		return nil, errors.New("simulated dispatch outage")
	}
	return p.JetStream.Publish(ctx, subject, data, opts...)
}
func TestQueueGracefulShutdownReleasesClaim(t *testing.T) {
	q, _ := testQueue(t)
	require.NoError(t, q.Enqueue(t.Context(), testJob("shutdown")))
	job, err := q.Get(t.Context(), "shutdown")
	require.NoError(t, err)
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- q.process(ctx, dispatch(job), map[string]Handler{"webhook": func(ctx context.Context, _ *statev1.JobRecord) error { close(entered); <-ctx.Done(); return ctx.Err() }})
	}()
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	released, err := q.Get(t.Context(), job.Id)
	require.NoError(t, err)
	require.Equal(t, Queued, released.Status)
	require.Nil(t, released.LeaseUntil)
	require.NoError(t, q.process(t.Context(), dispatch(released), map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { return nil }}))
	awaitStatus(t, q, job.Id, Succeeded)
}
func TestQueueLostOwnershipCannotPersistCompletion(t *testing.T) {
	q, _ := testQueue(t)
	require.NoError(t, q.Enqueue(t.Context(), testJob("fenced")))
	job, err := q.Get(t.Context(), "fenced")
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- q.process(t.Context(), dispatch(job), map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { close(entered); <-release; return nil }})
	}()
	<-entered
	claimed, rev, err := q.read(t.Context(), job.Id)
	require.NoError(t, err)
	claimed.Owner = "replacement-owner"
	_, err = q.save(t.Context(), claimed, rev)
	require.NoError(t, err)
	close(release)
	require.Error(t, <-done)
	current, err := q.Get(t.Context(), job.Id)
	require.NoError(t, err)
	require.Equal(t, Running, current.Status)
	require.Equal(t, "replacement-owner", current.Owner)
}

func TestQueueRetryWaitDoesNotOutliveOriginalDeadline(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	job := testJob("deadline-wait")
	job.ExpiresAt = timestamppb.New(time.Now().Add(time.Minute))
	job.RetryDelayMillis = int64((30 * time.Minute) / time.Millisecond)
	require.NoError(t, q.Enqueue(ctx, job))
	job, err := q.Get(ctx, job.Id)
	require.NoError(t, err)
	require.Error(t, q.process(ctx, dispatch(job), map[string]Handler{"webhook": func(context.Context, *statev1.JobRecord) error { return &Failure{Category: "unavailable"} }}))
	pending, err := q.Get(ctx, job.Id)
	require.NoError(t, err)
	require.Equal(t, Queued, pending.Status)
	require.True(t, pending.NextAttemptAt.AsTime().Equal(job.ExpiresAt.AsTime()))
}
