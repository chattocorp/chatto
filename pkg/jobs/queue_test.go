package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/pkg/events"
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

func testStreamConfig() jetstream.StreamConfig {
	return jetstream.StreamConfig{Name: "JOBS", Subjects: []string{"jobs.>"}, Storage: jetstream.MemoryStorage, AllowMsgTTL: true}
}

// testJS is the JetStream context of the server that the current test started.
// Tests in a package run one at a time.
var testJS jetstream.JetStream

// testQueue returns a queue on a new job stream and a worker consumer for the
// job type "work".
func testQueue(t *testing.T, opts Options) (*Queue, jetstream.Consumer) {
	t.Helper()
	js := startTestNATS(t)
	testJS = js
	ctx := t.Context()
	stream := must(js.CreateStream(ctx, testStreamConfig()))
	opts.SubjectPrefix = "jobs"
	q := must(New(js, stream, opts))
	consumer := must(stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable: "jobs-work", FilterSubjects: q.FilterSubjects("work"), AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 2 * time.Minute, MaxDeliver: -1, MaxAckPending: -1,
	}))
	return q, consumer
}

func testSpec(id string) Spec {
	return Spec{ID: id, Type: "work", Payload: []byte("ref-" + id), MaxAttempts: 3, RetryDelay: time.Millisecond}
}

// start runs fn until the returned stop function or the end of the test.
func start(t *testing.T, fn func(context.Context) error) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("run: %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func runView(t *testing.T, q *Queue) { start(t, q.Run) }

func runWorker(t *testing.T, q *Queue, c jetstream.Consumer, h Handler, concurrency int) (stop func()) {
	return start(t, func(ctx context.Context) error { return q.Work(ctx, c, h, concurrency) })
}

func awaitStatus(t *testing.T, q *Queue, id string, status Status) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		job, err := q.Get(t.Context(), id)
		if err == nil && job.Status == status {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s = %+v (%v), want status %d", id, job, err, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// factNames returns the retained fact subjects of the test stream.
func factNames(t *testing.T, q *Queue) map[string]uint64 {
	t.Helper()
	return must(q.stream.Info(t.Context(), jetstream.WithSubjectFilter("jobs.>"))).State.Subjects
}

func TestQueueRunsJobOnceAndKeepsOnlyItsSuccess(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	var calls atomic.Int32
	var seen Job
	runView(t, q)
	runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		seen = job
		calls.Add(1)
		return nil
	}, 2)
	noError(t, q.Enqueue(ctx, testSpec("one")))
	noError(t, q.Enqueue(ctx, testSpec("one")))
	job := awaitStatus(t, q, "one", Succeeded)
	if job.Attempts != 1 || job.MaxAttempts != 3 || job.Payload != nil || job.CreatedAt.IsZero() {
		t.Fatalf("job = %+v", job)
	}
	if string(seen.Payload) != "ref-one" || seen.Attempts != 1 || seen.RetryDelay != time.Millisecond {
		t.Fatalf("handler job = %+v", seen)
	}
	// A handoff after success still finds the job and accepts nothing.
	noError(t, q.Enqueue(ctx, testSpec("one")))
	time.Sleep(200 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d", calls.Load())
	}
	subjects := factNames(t, q)
	if len(subjects) != 1 || subjects["jobs.work.one.succeeded"] != 1 {
		t.Fatalf("retained facts = %v, want only the success", subjects)
	}
	msg := must(q.stream.GetLastMsgForSubject(ctx, "jobs.work.one.succeeded"))
	if msg.Header.Get(jetstream.MsgTTLHeader) == "" {
		t.Fatal("succeeded fact must expire")
	}
}

func TestQueueRetriesWithBackoffWithoutOccupyingTheWorker(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	var slowCalls atomic.Int32
	runView(t, q)
	runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		if job.ID == "slow" && slowCalls.Add(1) == 1 {
			return &Failure{Category: "not_yet"}
		}
		return nil
	}, 1)
	slow := testSpec("slow")
	slow.RetryDelay = 2 * time.Second
	noError(t, q.Enqueue(ctx, slow))
	waiting := awaitStatus(t, q, "slow", Queued)
	for waiting.Failure == "" {
		time.Sleep(10 * time.Millisecond)
		waiting = awaitStatus(t, q, "slow", Queued)
	}
	if waiting.Failure != "not_yet" || waiting.Attempts != 1 || !waiting.NextAttemptAt.After(time.Now()) {
		t.Fatalf("waiting job = %+v", waiting)
	}
	// The only worker is free while the first job waits.
	noError(t, q.Enqueue(ctx, testSpec("fast")))
	awaitStatus(t, q, "fast", Succeeded)
	if job := must(q.Get(ctx, "slow")); job.Status != Queued {
		t.Fatalf("slow job = %+v, want it still waiting", job)
	}
	if job := awaitStatus(t, q, "slow", Succeeded); job.Attempts != 2 {
		t.Fatalf("slow job = %+v", job)
	}
}

func TestQueueKeepsFailureAndRetriesOnce(t *testing.T) {
	var failed []Job
	var mu sync.Mutex
	q, consumer := testQueue(t, Options{OnFailed: func(_ context.Context, job Job) {
		mu.Lock()
		defer mu.Unlock()
		failed = append(failed, job)
	}})
	ctx := t.Context()
	var calls atomic.Int32
	var succeed atomic.Bool
	runView(t, q)
	runWorker(t, q, consumer, func(context.Context, Job) error {
		calls.Add(1)
		if succeed.Load() {
			return nil
		}
		return errors.New("secret detail")
	}, 2)
	spec := testSpec("bad")
	spec.MaxAttempts = 2
	noError(t, q.Enqueue(ctx, spec))
	job := awaitStatus(t, q, "bad", Failed)
	if job.Attempts != 2 || job.Failure != "attempt_failed" || calls.Load() != 2 {
		t.Fatalf("failed job = %+v, calls = %d", job, calls.Load())
	}
	mu.Lock()
	if len(failed) != 1 || string(failed[0].Payload) != "ref-bad" || failed[0].Failure != "attempt_failed" || failed[0].Attempts != 2 {
		t.Fatalf("OnFailed = %+v", failed)
	}
	mu.Unlock()
	if _, err := q.Retry(ctx, "bad", "stale"); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry with wrong version = %v", err)
	}
	if _, err := q.Retry(ctx, "missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retry missing = %v", err)
	}

	// Concurrent operator actions on the same view start one budget.
	succeed.Store(true)
	version := Version(job)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if _, err := q.Retry(ctx, "bad", version); err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrNotRetryable) {
				t.Errorf("retry = %v", err)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted retries = %d", accepted.Load())
	}
	done := awaitStatus(t, q, "bad", Succeeded)
	if done.Attempts != 1 || calls.Load() != 3 {
		t.Fatalf("retried job = %+v, calls = %d", done, calls.Load())
	}
	if _, err := q.Retry(ctx, "bad", version); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("stale retry = %v", err)
	}
}

func TestQueueStopsOnPermanentFailureAndDeadline(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	var calls atomic.Int32
	runView(t, q)
	runWorker(t, q, consumer, func(context.Context, Job) error {
		calls.Add(1)
		return &Failure{Category: "invalid_request", Permanent: true}
	}, 2)
	noError(t, q.Enqueue(ctx, testSpec("permanent")))
	if job := awaitStatus(t, q, "permanent", Failed); job.Attempts != 1 || job.Failure != "invalid_request" {
		t.Fatalf("permanent = %+v", job)
	}
	expired := testSpec("expired")
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	noError(t, q.Enqueue(ctx, expired))
	job := awaitStatus(t, q, "expired", Failed)
	if job.Attempts != 0 || job.Failure != "expired" || calls.Load() != 1 {
		t.Fatalf("expired = %+v, calls = %d", job, calls.Load())
	}
	// A manual retry never extends the deadline.
	if _, err := q.Retry(ctx, "expired", Version(job)); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry expired = %v", err)
	}
}

func TestQueueRetryKeepsSpecificationAndRejectsStaleVersion(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	var payloads []string
	var mu sync.Mutex
	runView(t, q)
	runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, string(job.Payload))
		return &Failure{Category: "boom", Permanent: true}
	}, 1)
	noError(t, q.Enqueue(ctx, testSpec("again")))
	first := awaitStatus(t, q, "again", Failed)
	retried, err := q.Retry(ctx, "again", Version(first))
	noError(t, err)
	if retried.Status == Failed || retried.MaxAttempts != 3 || !retried.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("retried = %+v", retried)
	}
	var second Job
	for second = awaitStatus(t, q, "again", Failed); second.Revision == first.Revision; second = awaitStatus(t, q, "again", Failed) {
		time.Sleep(10 * time.Millisecond)
	}
	// The first view is stale: its budget already ran and failed again.
	if _, err = q.Retry(ctx, "again", Version(first)); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("stale retry = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 || payloads[0] != "ref-again" || payloads[1] != "ref-again" {
		t.Fatalf("payloads = %v", payloads)
	}
}

func TestQueueListOrdersByUpdateFiltersAndPages(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		if job.ID == "b" {
			return &Failure{Category: "boom", Permanent: true}
		}
		return nil
	}, 1)
	for _, id := range []string{"a", "b", "c"} {
		noError(t, q.Enqueue(ctx, testSpec(id)))
		if id == "b" {
			awaitStatus(t, q, id, Failed)
		} else {
			awaitStatus(t, q, id, Succeeded)
		}
		time.Sleep(5 * time.Millisecond)
	}
	ids := func(jobs []Job) (out string) {
		for _, job := range jobs {
			out += job.ID
		}
		return out
	}
	all, next, err := q.List(ctx, ListOptions{Limit: 10})
	noError(t, err)
	if ids(all) != "cba" || next != "" {
		t.Fatalf("all = %s, next = %q", ids(all), next)
	}
	failed, _, err := q.List(ctx, ListOptions{Limit: 10, Status: Failed})
	noError(t, err)
	if ids(failed) != "b" {
		t.Fatalf("failed = %s", ids(failed))
	}
	first, next, err := q.List(ctx, ListOptions{Limit: 2})
	noError(t, err)
	second, last, err := q.List(ctx, ListOptions{Limit: 2, Cursor: next})
	noError(t, err)
	if ids(first) != "cb" || ids(second) != "a" || next == "" || last != "" {
		t.Fatalf("pages = %s, %s; cursors = %q, %q", ids(first), ids(second), next, last)
	}
	if _, _, err = q.List(ctx, ListOptions{Limit: 2, Cursor: "bad"}); err == nil {
		t.Fatal("invalid cursor accepted")
	}
	if _, _, err = q.List(ctx, ListOptions{}); err == nil {
		t.Fatal("zero limit accepted")
	}
}

func TestQueueViewRebuildsFromRetainedFacts(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	stop := runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		if job.ID == "bad" {
			return &Failure{Category: "boom", Permanent: true}
		}
		return nil
	}, 1)
	noError(t, q.Enqueue(ctx, testSpec("good")))
	noError(t, q.Enqueue(ctx, testSpec("bad")))
	good := awaitStatus(t, q, "good", Succeeded)
	bad := awaitStatus(t, q, "bad", Failed)
	stop()
	noError(t, q.Enqueue(ctx, testSpec("waiting")))

	// A second process sees the same jobs, also the one with only its success.
	other := must(New(testJS, q.stream, Options{SubjectPrefix: "jobs"}))
	runView(t, other)
	rebuiltGood := awaitStatus(t, other, "good", Succeeded)
	if rebuiltGood.Attempts != 1 || rebuiltGood.MaxAttempts != 3 || !rebuiltGood.CreatedAt.Equal(good.CreatedAt) {
		t.Fatalf("rebuilt success = %+v, want %+v", rebuiltGood, good)
	}
	if rebuilt := awaitStatus(t, other, "bad", Failed); Version(rebuilt) != Version(bad) || rebuilt.Failure != "boom" {
		t.Fatalf("rebuilt failure = %+v, want %+v", rebuilt, bad)
	}
	awaitStatus(t, other, "waiting", Queued)
}

func TestQueueViewDropsOnlySucceededJobsByDefault(t *testing.T) {
	q, consumer := testQueue(t, Options{SucceededRetention: time.Minute})
	ctx := t.Context()
	runView(t, q)
	stop := runWorker(t, q, consumer, func(_ context.Context, job Job) error {
		if job.ID == "bad" {
			return &Failure{Category: "boom", Permanent: true}
		}
		return nil
	}, 1)
	noError(t, q.Enqueue(ctx, testSpec("good")))
	noError(t, q.Enqueue(ctx, testSpec("bad")))
	awaitStatus(t, q, "good", Succeeded)
	awaitStatus(t, q, "bad", Failed)
	stop()
	noError(t, q.Enqueue(ctx, testSpec("waiting")))
	awaitStatus(t, q, "waiting", Queued)

	listed := func() int {
		jobs, _, err := q.List(ctx, ListOptions{Limit: 10})
		noError(t, err)
		return len(jobs)
	}
	q.sweep()
	if listed() != 3 {
		t.Fatalf("jobs = %d, want all three before any limit", listed())
	}
	// After the succeeded retention, only the success goes. Without a failed
	// retention, the failed job stays, and an unfinished job always stays.
	q.now = func() time.Time { return time.Now().Add(365 * 24 * time.Hour) }
	q.sweep()
	if _, err := q.Get(ctx, "good"); !errors.Is(err, ErrNotFound) || listed() != 2 {
		t.Fatalf("after retention: get = %v, jobs = %d", err, listed())
	}
}

func TestQueueFailedJobIsOneFactUntilItIsDeleted(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	var calls atomic.Int32
	runView(t, q)
	runWorker(t, q, consumer, func(context.Context, Job) error {
		calls.Add(1)
		return &Failure{Category: "boom", Permanent: true}
	}, 1)
	for _, id := range []string{"dead", "kept"} {
		noError(t, q.Enqueue(ctx, testSpec(id)))
	}
	dead := awaitStatus(t, q, "dead", Failed)
	kept := awaitStatus(t, q, "kept", Failed)
	// The failed fact carries the specification; the earlier facts are gone.
	// Without a failed retention, it has no TTL.
	subjects := factNames(t, q)
	if len(subjects) != 2 || subjects["jobs.work.dead.failed"] != 1 || subjects["jobs.work.kept.failed"] != 1 {
		t.Fatalf("retained facts = %v, want one failed fact per job", subjects)
	}
	if msg := must(q.stream.GetLastMsgForSubject(ctx, "jobs.work.dead.failed")); msg.Header.Get(jetstream.MsgTTLHeader) != "" {
		t.Fatal("failed fact must not expire by default")
	}
	// A handoff of the same source finds the failed job and accepts nothing.
	noError(t, q.Enqueue(ctx, testSpec("dead")))

	if err := q.Delete(ctx, "dead", "stale"); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("delete with wrong version = %v", err)
	}
	if err := q.Delete(ctx, "missing", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v", err)
	}
	noError(t, q.Delete(ctx, "dead", Version(dead)))
	if _, err := q.Get(ctx, "dead"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted job = %v", err)
	}
	if _, err := q.Retry(ctx, "dead", Version(dead)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retry deleted = %v", err)
	}
	// Only the short-lived deleted fact stays; a new view does not see the job.
	subjects = factNames(t, q)
	if len(subjects) != 2 || subjects["jobs.work.dead.deleted"] != 1 {
		t.Fatalf("retained facts after delete = %v", subjects)
	}
	if msg := must(q.stream.GetLastMsgForSubject(ctx, "jobs.work.dead.deleted")); msg.Header.Get(jetstream.MsgTTLHeader) == "" {
		t.Fatal("deleted fact must expire")
	}
	other := must(New(testJS, q.stream, Options{SubjectPrefix: "jobs"}))
	runView(t, other)
	awaitStatus(t, other, "kept", Failed)
	if _, err := other.Get(ctx, "dead"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted job in a new view = %v", err)
	}

	// A retry and a delete of the same failure cannot both take effect.
	if _, err := q.Retry(ctx, "kept", Version(kept)); err != nil {
		t.Fatal(err)
	}
	if err := q.Delete(ctx, "kept", Version(kept)); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("delete after retry = %v", err)
	}
	// Only failed jobs can be deleted.
	noError(t, q.Enqueue(ctx, testSpec("fresh")))
	queued := testSpec("queued")
	queued.Type = "other"
	noError(t, q.Enqueue(ctx, queued))
	waiting := awaitStatus(t, q, "queued", Queued)
	if err := q.Delete(ctx, "queued", Version(waiting)); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("delete unfinished = %v", err)
	}
}

func TestQueueFailedRetentionExpiresFailedJobs(t *testing.T) {
	q, consumer := testQueue(t, Options{FailedRetention: time.Minute})
	ctx := t.Context()
	runView(t, q)
	runWorker(t, q, consumer, func(context.Context, Job) error { return &Failure{Category: "boom", Permanent: true} }, 1)
	noError(t, q.Enqueue(ctx, testSpec("bad")))
	awaitStatus(t, q, "bad", Failed)
	if msg := must(q.stream.GetLastMsgForSubject(ctx, "jobs.work.bad.failed")); msg.Header.Get(jetstream.MsgTTLHeader) == "" {
		t.Fatal("failed fact must expire with a failed retention")
	}
	q.sweep()
	if _, err := q.Get(ctx, "bad"); err != nil {
		t.Fatalf("failed job before its retention = %v", err)
	}
	q.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	q.sweep()
	if _, err := q.Get(ctx, "bad"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed job after its retention = %v", err)
	}
}

func TestQueueShutdownRepeatsInterruptedAttemptAndKeepsFinishedOne(t *testing.T) {
	q, consumer := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	started := make(chan string, 4)
	var calls atomic.Int32
	handler := func(ctx context.Context, job Job) error {
		calls.Add(1)
		started <- job.ID
		<-ctx.Done()
		if job.ID == "finished" {
			// The effect completed just before the cancellation.
			return nil
		}
		return ctx.Err()
	}
	stop := runWorker(t, q, consumer, handler, 2)
	noError(t, q.Enqueue(ctx, testSpec("interrupted")))
	noError(t, q.Enqueue(ctx, testSpec("finished")))
	<-started
	<-started
	stop()
	if job := awaitStatus(t, q, "finished", Succeeded); job.Attempts != 1 {
		t.Fatalf("finished = %+v", job)
	}
	if job := must(q.Get(ctx, "interrupted")); job.Status != Running {
		t.Fatalf("interrupted = %+v, want no recorded outcome", job)
	}
	// Another worker takes the interrupted job over. The interrupted delivery
	// counts as an attempt.
	runWorker(t, q, consumer, func(context.Context, Job) error { return nil }, 1)
	if job := awaitStatus(t, q, "interrupted", Succeeded); job.Attempts != 2 || calls.Load() != 2 {
		t.Fatalf("interrupted = %+v, calls = %d", job, calls.Load())
	}
}

// failNextPurge makes the next purge of the wrapped stream fail.
type failNextPurge struct {
	jetstream.Stream
	fail bool
}

func (f *failNextPurge) Purge(ctx context.Context, opts ...jetstream.StreamPurgeOpt) error {
	if f.fail {
		f.fail = false
		return errors.New("purge failed")
	}
	return f.Stream.Purge(ctx, opts...)
}

// queuedDelivery returns the queued fact of a job as a delivery.
func queuedDelivery(t *testing.T, q *Queue, id string, delivered uint64) events.DurableDelivery {
	t.Helper()
	return eventsDelivery(must(q.stream.GetLastMsgForSubject(t.Context(), "jobs.work."+id+".queued")), delivered)
}

func TestQueueRunsDeliveryBeyondTheAttemptBudget(t *testing.T) {
	q, _ := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	// Earlier deliveries ended without a stored outcome, for example because
	// the outcome write failed. Their result is unknown, so the job runs.
	noError(t, q.Enqueue(ctx, testSpec("lost-success")))
	noError(t, q.process(ctx, queuedDelivery(t, q, "lost-success", 4), func(context.Context, Job) error { return nil }))
	if job := awaitStatus(t, q, "lost-success", Succeeded); job.Attempts != 4 {
		t.Fatalf("job = %+v", job)
	}
	noError(t, q.Enqueue(ctx, testSpec("lost-failure")))
	noError(t, q.process(ctx, queuedDelivery(t, q, "lost-failure", 4), func(context.Context, Job) error { return &Failure{Category: "boom"} }))
	if job := awaitStatus(t, q, "lost-failure", Failed); job.Attempts != 4 || job.Failure != "boom" {
		t.Fatalf("job = %+v", job)
	}
}

func TestQueueRepeatedDeliveryAfterOutcomeDoesNotRunAgain(t *testing.T) {
	q, _ := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	var calls atomic.Int32
	handler := func(_ context.Context, job Job) error {
		calls.Add(1)
		if job.ID == "bad" {
			return &Failure{Category: "boom", Permanent: true}
		}
		return nil
	}
	deliveries := map[string]events.DurableDelivery{}
	for id, status := range map[string]Status{"good": Succeeded, "bad": Failed} {
		noError(t, q.Enqueue(ctx, testSpec(id)))
		delivery := queuedDelivery(t, q, id, 1)
		deliveries[id] = delivery
		noError(t, q.process(ctx, delivery, handler))
		awaitStatus(t, q, id, status)
		// The acknowledgement was lost; the same fact arrives again.
		delivery.NumDelivered = 2
		noError(t, q.process(ctx, delivery, handler))
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want one per job", calls.Load())
	}
	// A delivery of the old budget after a manual retry does not run either.
	_, err := q.Retry(ctx, "bad", Version(awaitStatus(t, q, "bad", Failed)))
	noError(t, err)
	old := deliveries["bad"]
	old.NumDelivered = 3
	noError(t, q.process(ctx, old, handler))
	if job := must(q.Get(ctx, "bad")); job.Status != Queued || calls.Load() != 2 {
		t.Fatalf("job = %+v, calls = %d", job, calls.Load())
	}
}

func TestQueueFinishesFailedPurgeOnTheNextDelivery(t *testing.T) {
	q, _ := testQueue(t, Options{})
	ctx := t.Context()
	runView(t, q)
	purge := &failNextPurge{Stream: q.stream, fail: true}
	q.stream = purge
	var calls atomic.Int32
	handler := func(context.Context, Job) error { calls.Add(1); return nil }
	noError(t, q.Enqueue(ctx, testSpec("one")))
	delivery := queuedDelivery(t, q, "one", 1)
	// The success is stored, but its earlier facts stay. The delivery returns.
	if err := q.process(ctx, delivery, handler); err == nil {
		t.Fatal("process must report the failed purge")
	}
	if subjects := factNames(t, q); len(subjects) != 3 {
		t.Fatalf("facts after failed purge = %v", subjects)
	}
	delivery.NumDelivered = 2
	noError(t, q.process(ctx, delivery, handler))
	subjects := factNames(t, q)
	if len(subjects) != 1 || subjects["jobs.work.one.succeeded"] != 1 || calls.Load() != 1 {
		t.Fatalf("facts = %v, calls = %d", subjects, calls.Load())
	}
	if job := awaitStatus(t, q, "one", Succeeded); job.Attempts != 1 {
		t.Fatalf("job = %+v", job)
	}

	// A delete reports a failed purge; the failed fact then stays.
	noError(t, q.Enqueue(ctx, testSpec("two")))
	noError(t, q.process(ctx, queuedDelivery(t, q, "two", 1), func(context.Context, Job) error {
		return &Failure{Category: "boom", Permanent: true}
	}))
	failed := awaitStatus(t, q, "two", Failed)
	purge.fail = true
	if err := q.Delete(ctx, "two", Version(failed)); err == nil {
		t.Fatal("delete must report the failed purge")
	}
}

func TestViewAppliesFactsInOrder(t *testing.T) {
	v := &view{jobs: map[string]*entry{}}
	at := time.Now().Truncate(time.Second)
	fact := func(f *jobpb.JobFact, offset time.Duration) *jobpb.JobFact {
		f.Id, f.Type = "j", "work"
		f.OccurredAt, f.CreatedAt = timestamppb.New(at.Add(offset)), timestamppb.New(at)
		return f
	}
	// Leftover facts of a job without its specification describe no job.
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Started{Started: &jobpb.JobStarted{Attempt: 1}}}, 0), 1))
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Failed{Failed: &jobpb.JobFailed{Attempts: 1}}}, 0), 2))
	if _, ok := v.get("j"); ok {
		t.Fatal("leftover facts created a job")
	}
	spec := &jobpb.JobSpec{MaxAttempts: 2, RetryDelayMillis: 5}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Queued{Queued: &jobpb.JobQueued{Spec: spec}}}, 0), 3))
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Started{Started: &jobpb.JobStarted{Attempt: 1}}}, time.Second), 4))
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Failed{Failed: &jobpb.JobFailed{Attempts: 1, Failure: "boom"}}}, 2*time.Second), 5))
	// A late progress fact of a repeated delivery changes only the revision.
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Started{Started: &jobpb.JobStarted{Attempt: 2}}}, 3*time.Second), 6))
	e, _ := v.get("j")
	if e.job.Status != Failed || e.job.Attempts != 1 || e.job.Failure != "boom" || e.job.Revision != 6 || e.specSeq != 3 || !e.job.UpdatedAt.Equal(at.Add(2*time.Second)) {
		t.Fatalf("after failure = %+v", e)
	}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Retried{Retried: &jobpb.JobRetried{Spec: spec}}}, 4*time.Second), 7))
	e, _ = v.get("j")
	if e.job.Status != Queued || e.job.Attempts != 0 || e.job.Failure != "" || e.specSeq != 7 || !e.job.CreatedAt.Equal(at) {
		t.Fatalf("after retry = %+v", e)
	}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_AttemptFailed{AttemptFailed: &jobpb.JobAttemptFailed{Attempt: 1, Failure: "slow", NextAttemptAt: timestamppb.New(at.Add(time.Minute))}}}, 5*time.Second), 8))
	e, _ = v.get("j")
	if e.job.Status != Queued || e.job.Failure != "slow" || !e.job.NextAttemptAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("after failed attempt = %+v", e)
	}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Succeeded{Succeeded: &jobpb.JobSucceeded{Attempts: 2, MaxAttempts: 2}}}, 6*time.Second), 9))
	// A failure after the success is a leftover of a repeated delivery.
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Failed{Failed: &jobpb.JobFailed{Attempts: 3}}}, 7*time.Second), 10))
	e, _ = v.get("j")
	if e.job.Status != Succeeded || e.job.Attempts != 2 || e.job.Failure != "" || !e.job.NextAttemptAt.IsZero() {
		t.Fatalf("after success = %+v", e)
	}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Deleted{Deleted: &jobpb.JobDeleted{}}}, 8*time.Second), 11))
	if _, ok := v.get("j"); ok {
		t.Fatal("deleted job stays in the view")
	}
	// A success or a failure describes a job by itself, after the purge of its
	// earlier facts.
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Succeeded{Succeeded: &jobpb.JobSucceeded{Attempts: 1, MaxAttempts: 2}}}, 9*time.Second), 12))
	e, _ = v.get("j")
	if e.job.Status != Succeeded || e.job.Type != "work" || !e.job.CreatedAt.Equal(at) || e.job.MaxAttempts != 2 {
		t.Fatalf("lone success = %+v", e)
	}
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Deleted{Deleted: &jobpb.JobDeleted{}}}, 10*time.Second), 13))
	noError(t, v.Apply(fact(&jobpb.JobFact{Fact: &jobpb.JobFact_Failed{Failed: &jobpb.JobFailed{Attempts: 2, Failure: "boom", Spec: spec}}}, 11*time.Second), 14))
	e, _ = v.get("j")
	if e.job.Status != Failed || e.specSeq != 14 || e.job.MaxAttempts != 2 || e.job.RetryDelay != 5*time.Millisecond {
		t.Fatalf("lone failure = %+v", e)
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	js := startTestNATS(t)
	ctx := t.Context()
	stream := must(js.CreateStream(ctx, testStreamConfig()))
	for name, opts := range map[string]Options{
		"empty prefix":    {},
		"wildcard prefix": {SubjectPrefix: "jobs.>"},
		"trailing dot":    {SubjectPrefix: "jobs."},
		"short retention": {SubjectPrefix: "jobs", SucceededRetention: time.Millisecond},
		"short failed":    {SubjectPrefix: "jobs", FailedRetention: time.Millisecond},
	} {
		if _, err := New(js, stream, opts); err == nil {
			t.Errorf("%s: New accepted invalid options", name)
		}
	}
	plain := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "PLAIN", Subjects: []string{"plain.>"}, Storage: jetstream.MemoryStorage}))
	if _, err := New(js, plain, Options{SubjectPrefix: "plain"}); err == nil {
		t.Error("New accepted a stream without per-message TTLs")
	}
	aged := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "AGED", Subjects: []string{"aged.>"}, Storage: jetstream.MemoryStorage, MaxAge: time.Hour, AllowMsgTTL: true}))
	if _, err := New(js, aged, Options{SubjectPrefix: "aged"}); err == nil {
		t.Error("New accepted a stream that removes unfinished jobs by age")
	}
	q := must(New(js, stream, Options{SubjectPrefix: "jobs"}))
	for name, spec := range map[string]Spec{
		"bad id":      {ID: "a.b", Type: "work", MaxAttempts: 1, RetryDelay: time.Second},
		"bad type":    {ID: "a", Type: "wo rk", MaxAttempts: 1, RetryDelay: time.Second},
		"no attempts": {ID: "a", Type: "work", RetryDelay: time.Second},
		"no delay":    {ID: "a", Type: "work", MaxAttempts: 1},
	} {
		if err := q.Enqueue(ctx, spec); err == nil {
			t.Errorf("%s: Enqueue accepted an invalid specification", name)
		}
	}
	if got := q.FilterSubjects("a", "b"); len(got) != 4 || got[0] != "jobs.a.*.queued" || got[3] != "jobs.b.*.retried" {
		t.Fatalf("FilterSubjects = %v", got)
	}
	if Backoff(time.Second, 1) != time.Second || Backoff(time.Second, 3) != 4*time.Second || Backoff(time.Minute, 30) != 30*time.Minute {
		t.Fatal("Backoff must double from the initial delay to the cap")
	}
}
