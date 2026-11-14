package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
)

func startTestNATS(t *testing.T) jetstream.JetStream {
	t.Helper()
	natsServer, err := server.NewServer(&server.Options{JetStream: true, DontListen: true, StoreDir: t.TempDir(), NoSigs: true})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	natsServer.Start()
	t.Cleanup(func() {
		natsServer.Shutdown()
		natsServer.WaitForShutdown()
	})
	if !natsServer.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server did not become ready")
	}
	connection, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(natsServer))
	if err != nil {
		t.Fatalf("connect to NATS server: %v", err)
	}
	t.Cleanup(connection.Close)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	return js
}

// must unwraps test setup results. A panic fails the calling test.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func noError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testQueue(t *testing.T) (*Queue, jetstream.Consumer) {
	t.Helper()
	js := startTestNATS(t)
	ctx := t.Context()
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, Storage: jetstream.MemoryStorage, MaxMsgsPerSubject: 1}))
	bucket := must(js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "STATE", LimitMarkerTTL: time.Hour}))
	kv := must(jetstreamutil.NewKeyValue(js, bucket))
	q := must(New(js, kv, Options{KeyPrefix: "job.", SubjectPrefix: "jobs"}))
	consumer := must(stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "jobs", FilterSubjects: q.FilterSubjects("work"), AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 2 * time.Minute, MaxDeliver: -1,
	}))
	return q, consumer
}

func testSpec(id string) Spec {
	return Spec{ID: id, Type: "work", MaxAttempts: 3, RetryDelay: time.Millisecond, Payload: []byte("opaque-reference")}
}

func dispatch(job Job) events.DurableDelivery {
	data, _ := proto.Marshal(&jobpb.JobDispatch{Id: job.ID, Generation: job.Generation})
	return events.DurableDelivery{Data: data}
}

func handlers(h Handler) map[string]Handler { return map[string]Handler{"work": h} }

func runQueue(t *testing.T, q *Queue, c jetstream.Consumer, h map[string]Handler, concurrency int) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		g, ctx := errgroup.WithContext(ctx)
		g.Go(func() error { return q.Run(ctx) })
		g.Go(func() error { return q.Work(ctx, c, h, concurrency) })
		done <- g.Wait()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("queue run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("queue did not stop")
		}
	})
}

func awaitStatus(t *testing.T, q *Queue, id string, status Status) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, err := q.Get(t.Context(), id)
		if err == nil && job.Status == status {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s = %+v, %v; want status %d", id, job, err, status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQueueRetriesWithoutOccupyingOnlyWorker(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	var calls atomic.Int32
	slow := testSpec("a-slow")
	slow.RetryDelay = time.Second
	noError(t, q.Enqueue(ctx, slow))
	noError(t, q.Enqueue(ctx, testSpec("b-fast")))
	runQueue(t, q, c, handlers(func(_ context.Context, job Job) error {
		if job.ID == slow.ID && calls.Add(1) == 1 {
			return errors.New("external secret must not persist")
		}
		return nil
	}), 1)
	awaitStatus(t, q, "b-fast", Succeeded)
	if first := awaitStatus(t, q, slow.ID, Queued); first.Failure != "attempt_failed" {
		t.Fatalf("failure = %q, want attempt_failed", first.Failure)
	}
	if terminal := awaitStatus(t, q, slow.ID, Succeeded); terminal.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", terminal.Attempts)
	}
	// Source replay cannot reset work or alter its original payload.
	duplicate := testSpec(slow.ID)
	duplicate.Payload = []byte("changed")
	noError(t, q.Enqueue(ctx, duplicate))
	stored := must(q.Get(ctx, slow.ID))
	if string(stored.Payload) != "opaque-reference" || stored.Status != Succeeded {
		t.Fatalf("stored = %+v, want unchanged succeeded job", stored)
	}
}

func TestQueueRetainsFailureAndManualRetryGeneration(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	spec := testSpec("failure")
	spec.MaxAttempts = 1
	noError(t, q.Enqueue(ctx, spec))
	job := must(q.Get(ctx, spec.ID))
	noError(t, q.process(ctx, dispatch(job), handlers(func(context.Context, Job) error { return &Failure{Category: "http_error"} })))
	failed := must(q.Get(ctx, job.ID))
	if failed.Status != Failed || failed.Failure != "http_error" {
		t.Fatalf("failed = %+v", failed)
	}
	retried := must(q.Retry(ctx, job.ID, RetryVersion(job)))
	if retried.Generation != 2 || retried.Attempts != 0 {
		t.Fatalf("retried = %+v", retried)
	}
	// An old, unconfirmed dispatch cannot execute a fresh generation.
	var calls int
	h := handlers(func(context.Context, Job) error { calls++; return nil })
	noError(t, q.process(ctx, dispatch(failed), h))
	if calls != 0 {
		t.Fatal("old generation ran")
	}
	noError(t, q.process(ctx, dispatch(retried), h))
	noError(t, q.process(ctx, dispatch(retried), h))
	if calls != 1 {
		t.Fatalf("calls = %d; a lost acknowledgement must not repeat a completed handler", calls)
	}
	if _, err := q.Retry(ctx, job.ID, RetryVersion(job)); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry succeeded job = %v", err)
	}
}

func TestQueueConcurrentClaimsAndRetry(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	spec := testSpec("concurrent")
	spec.MaxAttempts = 1
	noError(t, q.Enqueue(ctx, spec))
	job := must(q.Get(ctx, spec.ID))
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	h := handlers(func(context.Context, Job) error {
		calls.Add(1)
		close(entered)
		<-release
		return &Failure{Category: "unavailable"}
	})
	done := make(chan error, 1)
	go func() { done <- q.process(ctx, dispatch(job), h) }()
	<-entered
	// A concurrent dispatch acknowledges without a second attempt.
	noError(t, q.process(ctx, dispatch(job), h))
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
	close(release)
	noError(t, <-done)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := q.Retry(ctx, job.ID, RetryVersion(job)); err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrNotRetryable) {
				t.Errorf("retry error = %v", err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted retries = %d, want 1", accepted.Load())
	}
}

func TestQueueRepairsRecordWithoutDispatch(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	now := timestamppb.Now()
	data := must(proto.Marshal(&jobpb.JobRecord{
		Id: "outbox", Type: "work", Status: jobpb.JobStatus_JOB_STATUS_QUEUED, Generation: 1,
		MaxAttempts: 3, RetryDelayMillis: 1, CreatedAt: now, NextAttemptAt: now,
	}))
	must(q.kv.Create(ctx, q.key("outbox"), data))
	runQueue(t, q, c, handlers(func(context.Context, Job) error { return nil }), 1)
	awaitStatus(t, q, "outbox", Succeeded)
}

func TestQueueRecoversExpiredClaimAndPreservesDeadline(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	noError(t, q.Enqueue(ctx, testSpec("interrupted")))
	record, revision, err := q.read(ctx, "interrupted")
	noError(t, err)
	record.Status = jobpb.JobStatus_JOB_STATUS_RUNNING
	record.Attempts = 1
	record.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
	must(q.save(ctx, record, revision))
	calls := 0
	h := handlers(func(context.Context, Job) error { calls++; return nil })
	noError(t, q.process(ctx, dispatch(jobFromRecord(record)), h))
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	expiredSpec := testSpec("expired")
	expiredSpec.ExpiresAt = time.Now().Add(-time.Minute)
	noError(t, q.Enqueue(ctx, expiredSpec))
	noError(t, q.process(ctx, dispatch(must(q.Get(ctx, "expired"))), h))
	expired := must(q.Get(ctx, "expired"))
	if calls != 1 || expired.Status != Failed || expired.Failure != "expired" {
		t.Fatalf("expired = %+v, calls = %d", expired, calls)
	}
	if _, err := q.Retry(ctx, expired.ID, RetryVersion(expired)); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry expired = %v", err)
	}
}

func TestQueueListOrdersByUpdateAndFiltersStatus(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	must(q.kv.Put(ctx, "other", []byte("private")))
	for _, id := range []string{"z-first", "a-second", "m-failed"} {
		spec := testSpec(id)
		spec.MaxAttempts = 1
		noError(t, q.Enqueue(ctx, spec))
		time.Sleep(2 * time.Millisecond)
	}
	runQueue(t, q, c, handlers(func(_ context.Context, job Job) error {
		if job.ID == "m-failed" {
			return &Failure{Category: "broken", Permanent: true}
		}
		return nil
	}), 1)
	awaitStatus(t, q, "m-failed", Failed)
	awaitStatus(t, q, "a-second", Succeeded)
	awaitStatus(t, q, "z-first", Succeeded)
	var all []Job
	cursor := ""
	for {
		page, next, err := q.List(ctx, ListOptions{Limit: 2, Cursor: cursor})
		noError(t, err)
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(all) != 3 {
		t.Fatalf("listed %d jobs, want 3 without other keys", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].UpdatedAt.After(all[i-1].UpdatedAt) {
			t.Fatalf("jobs not newest first: %+v", all)
		}
	}
	for _, job := range all {
		if job.Payload != nil {
			t.Fatal("list exposed a payload")
		}
	}
	failed, next, err := q.List(ctx, ListOptions{Status: Failed, Limit: 10})
	noError(t, err)
	if next != "" || len(failed) != 1 || failed[0].ID != "m-failed" {
		t.Fatalf("failed filter = %+v, %q", failed, next)
	}
	if _, _, err := q.List(ctx, ListOptions{Limit: 1, Cursor: "garbage"}); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}

// A job that waits for a retry must not hold consumer capacity. With one
// pending dispatch allowed, a later job still runs while the first one waits.
func TestQueueRetryWaitHoldsNoConsumerCapacity(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	stream := must(q.js.Stream(ctx, "JOBS"))
	consumer := must(stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "narrow", FilterSubjects: q.FilterSubjects("work"), AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 2 * time.Minute, MaxDeliver: -1, MaxAckPending: 1,
	}))
	waiting := testSpec("a-waiting")
	waiting.RetryDelay = time.Hour
	noError(t, q.Enqueue(ctx, waiting))
	runQueue(t, q, consumer, handlers(func(_ context.Context, job Job) error {
		if job.ID == waiting.ID {
			return &Failure{Category: "unavailable"}
		}
		return nil
	}), 1)
	deadline := time.Now().Add(5 * time.Second)
	for job := must(q.Get(ctx, waiting.ID)); job.Attempts != 1; job = must(q.Get(ctx, waiting.ID)) {
		if time.Now().After(deadline) {
			t.Fatalf("first attempt did not run: %+v", job)
		}
		time.Sleep(5 * time.Millisecond)
	}
	noError(t, q.Enqueue(ctx, testSpec("b-later")))
	awaitStatus(t, q, "b-later", Succeeded)
}

func TestQueueIndexDropsExpiredRecords(t *testing.T) {
	js := startTestNATS(t)
	ctx := t.Context()
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, Storage: jetstream.MemoryStorage, MaxMsgsPerSubject: 1}))
	bucket := must(js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "STATE", LimitMarkerTTL: time.Hour}))
	kv := must(jetstreamutil.NewKeyValue(js, bucket))
	q := must(New(js, kv, Options{KeyPrefix: "job.", SubjectPrefix: "jobs", SucceededRetention: time.Second}))
	consumer := must(stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Durable: "jobs", FilterSubjects: q.FilterSubjects("work"), AckPolicy: jetstream.AckExplicitPolicy, MaxDeliver: -1}))
	noError(t, q.Enqueue(ctx, testSpec("short-lived")))
	runQueue(t, q, consumer, handlers(func(context.Context, Job) error { return nil }), 1)
	awaitStatus(t, q, "short-lived", Succeeded)
	deadline := time.Now().Add(10 * time.Second)
	for {
		jobs, _, err := q.List(ctx, ListOptions{Limit: 10})
		noError(t, err)
		if len(jobs) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expired job still listed: %+v", jobs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A retry may be accepted even when dispatch publication fails. Recovery must
// use the accepted generation instead of rejecting or resetting that retry.
func TestQueueRepairsAcceptedRetryAfterPublishFailure(t *testing.T) {
	q, consumer := testQueue(t)
	ctx := t.Context()
	noError(t, q.Enqueue(ctx, testSpec("retry-outbox")))
	record, revision, err := q.read(ctx, "retry-outbox")
	noError(t, err)
	record.Status = jobpb.JobStatus_JOB_STATUS_FAILED
	must(q.save(ctx, record, revision))
	publisher := &failNextPublish{JetStream: q.js}
	publisher.fail.Store(true)
	q.js = publisher
	accepted := must(q.Retry(ctx, "retry-outbox", RetryVersion(jobFromRecord(record))))
	if accepted.Generation != 2 {
		t.Fatalf("generation = %d", accepted.Generation)
	}
	runQueue(t, q, consumer, handlers(func(_ context.Context, attempt Job) error {
		if attempt.Generation != 2 {
			t.Errorf("attempt generation = %d", attempt.Generation)
		}
		return nil
	}), 1)
	awaitStatus(t, q, "retry-outbox", Succeeded)
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
	noError(t, q.Enqueue(t.Context(), testSpec("shutdown")))
	job := must(q.Get(t.Context(), "shutdown"))
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- q.process(ctx, dispatch(job), handlers(func(ctx context.Context, _ Job) error { close(entered); <-ctx.Done(); return ctx.Err() }))
	}()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("process = %v", err)
	}
	record, _, err := q.read(t.Context(), job.ID)
	noError(t, err)
	// The interrupted attempt does not count against the budget.
	if record.Status != jobpb.JobStatus_JOB_STATUS_QUEUED || record.LeaseUntil != nil || record.Attempts != 0 {
		t.Fatalf("released = %+v", record)
	}
	noError(t, q.process(t.Context(), dispatch(jobFromRecord(record)), handlers(func(context.Context, Job) error { return nil })))
	awaitStatus(t, q, job.ID, Succeeded)
}

func TestQueueLostOwnershipCannotPersistCompletion(t *testing.T) {
	q, _ := testQueue(t)
	noError(t, q.Enqueue(t.Context(), testSpec("fenced")))
	job := must(q.Get(t.Context(), "fenced"))
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- q.process(t.Context(), dispatch(job), handlers(func(context.Context, Job) error { close(entered); <-release; return nil }))
	}()
	<-entered
	claimed, revision, err := q.read(t.Context(), job.ID)
	noError(t, err)
	claimed.Owner = "replacement-owner"
	must(q.save(t.Context(), claimed, revision))
	close(release)
	if err := <-done; err == nil {
		t.Fatal("stale owner persisted completion")
	}
	current, _, err := q.read(t.Context(), job.ID)
	noError(t, err)
	if current.Status != jobpb.JobStatus_JOB_STATUS_RUNNING || current.Owner != "replacement-owner" {
		t.Fatalf("current = %+v", current)
	}
}

func TestQueueRetryWaitDoesNotOutliveOriginalDeadline(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	spec := testSpec("deadline-wait")
	spec.ExpiresAt = time.Now().Add(time.Minute).Truncate(time.Microsecond)
	spec.RetryDelay = 30 * time.Minute
	noError(t, q.Enqueue(ctx, spec))
	job := must(q.Get(ctx, spec.ID))
	// A failed attempt acknowledges its dispatch; Run publishes the next one.
	noError(t, q.process(ctx, dispatch(job), handlers(func(context.Context, Job) error { return &Failure{Category: "unavailable"} })))
	pending := must(q.Get(ctx, job.ID))
	if pending.Status != Queued || !pending.NextAttemptAt.Equal(job.ExpiresAt) {
		t.Fatalf("pending = %+v, want next attempt at deadline %v", pending, job.ExpiresAt)
	}
}

// Interleaving: two operators view failed budget 1, the first retries it, and
// budget 2 fails before the second request arrives. An ID-only retry would
// start budget 3. The original version must reject that stale action.
func TestQueueRejectsStaleRetryAfterNewBudgetAlreadyFailed(t *testing.T) {
	q, _ := testQueue(t)
	ctx := t.Context()
	spec := testSpec("stale-retry")
	spec.MaxAttempts = 1
	noError(t, q.Enqueue(ctx, spec))
	job := must(q.Get(ctx, spec.ID))
	fail := handlers(func(context.Context, Job) error { return &Failure{Category: "http_error"} })
	noError(t, q.process(ctx, dispatch(job), fail))
	version := RetryVersion(job)
	accepted := must(q.Retry(ctx, job.ID, version))
	noError(t, q.process(ctx, dispatch(accepted), fail))
	newer := must(q.Get(ctx, job.ID))
	if newer.Status != Failed || RetryVersion(newer) == version {
		t.Fatalf("newer = %+v", newer)
	}
	if _, err := q.Retry(ctx, job.ID, version); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("stale retry = %v", err)
	}
	current := must(q.Get(ctx, job.ID))
	if current.Generation != 2 {
		t.Fatalf("generation = %d", current.Generation)
	}
	must(q.Retry(ctx, job.ID, RetryVersion(current)))
}

func TestQueueExpiresOnlySucceededRecords(t *testing.T) {
	q, c := testQueue(t)
	ctx := t.Context()
	ok, failed := testSpec("expires-ok"), testSpec("keeps-failed")
	failed.MaxAttempts = 1
	noError(t, q.Enqueue(ctx, ok))
	noError(t, q.Enqueue(ctx, failed))
	runQueue(t, q, c, handlers(func(_ context.Context, job Job) error {
		if job.ID == failed.ID {
			return &Failure{Category: "broken", Permanent: true}
		}
		return nil
	}), 2)
	awaitStatus(t, q, ok.ID, Succeeded)
	awaitStatus(t, q, failed.ID, Failed)
	stream := must(q.js.Stream(ctx, "KV_STATE"))
	ttl := func(id string) string {
		msg := must(stream.GetLastMsgForSubject(ctx, "$KV.STATE."+q.key(id)))
		return msg.Header.Get(jetstream.MsgTTLHeader)
	}
	if ttl(ok.ID) == "" {
		t.Fatal("succeeded record must expire")
	}
	if ttl(failed.ID) != "" {
		t.Fatal("failed record must stay for inspection")
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	js := startTestNATS(t)
	bucket := must(js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: "STATE"}))
	kv := must(jetstreamutil.NewKeyValue(js, bucket))
	for _, opts := range []Options{
		{KeyPrefix: "job", SubjectPrefix: "jobs"},
		{KeyPrefix: "job.", SubjectPrefix: "jobs."},
		{KeyPrefix: "job.", SubjectPrefix: "jobs.>"},
		{KeyPrefix: "job.", SubjectPrefix: "jobs", SucceededRetention: -time.Second},
	} {
		if _, err := New(js, kv, opts); err == nil {
			t.Errorf("New(%+v) succeeded", opts)
		}
	}
}

type countingPublisher struct {
	jetstream.JetStream
	count atomic.Int32
}

func (p *countingPublisher) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	p.count.Add(1)
	return p.JetStream.Publish(ctx, subject, data, opts...)
}

// A record update publishes only that record. A backlog of due jobs waiting for
// a worker must not be published again on every change.
func TestQueueUpdatePublishesOnlyTheChangedJob(t *testing.T) {
	q, _ := testQueue(t)
	ctx, cancel := context.WithCancel(t.Context())
	publisher := &countingPublisher{JetStream: q.js}
	q.js = publisher
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	for i := range 20 {
		noError(t, q.Enqueue(t.Context(), testSpec(fmt.Sprintf("backlog-%02d", i))))
	}
	waitForListed := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			jobs, _, err := q.List(t.Context(), ListOptions{Limit: 100})
			noError(t, err)
			if len(jobs) == n {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("listed %d jobs, want %d", len(jobs), n)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitForListed(20)
	time.Sleep(100 * time.Millisecond)
	before := publisher.count.Load()
	noError(t, q.Enqueue(t.Context(), testSpec("backlog-new")))
	waitForListed(21)
	time.Sleep(100 * time.Millisecond)
	// One publish from Enqueue and at most one from the watcher.
	if published := publisher.count.Load() - before; published > 2 {
		t.Fatalf("one enqueue caused %d publishes behind a backlog of 20", published)
	}
}
