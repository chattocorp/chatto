package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"

	"github.com/chattocorp/chatto/pkg/events"
)

const (
	heartbeatInterval = 15 * time.Second
	// infrastructureRetry is the redelivery delay after a failed fact write.
	infrastructureRetry = 5 * time.Second
	// shutdownWriteTimeout bounds the success write after cancellation.
	shutdownWriteTimeout = 5 * time.Second
	sweepInterval        = time.Minute
	maxBackoff           = 30 * time.Minute
	// DefaultSucceededRetention is used when Options.SucceededRetention is zero.
	DefaultSucceededRetention = 3 * 24 * time.Hour
	// deletedRetention is how long the fact of a deleted job stays. Every
	// running job view must see it in this time.
	deletedRetention = time.Hour
)

// The fact names are the last subject token of each fact.
const (
	factQueued        = "queued"
	factRetried       = "retried"
	factStarted       = "started"
	factAttemptFailed = "attempt_failed"
	factSucceeded     = "succeeded"
	factFailed        = "failed"
	factDeleted       = "deleted"
)

var (
	// ErrNotFound reports a job ID that the view does not hold.
	ErrNotFound = errors.New("job not found")
	// ErrNotRetryable reports a job that is not failed, has expired, or changed
	// after the caller read it.
	ErrNotRetryable = errors.New("job cannot be retried")
	// ErrNotDeletable reports a job that is not failed or changed after the
	// caller read it.
	ErrNotDeletable = errors.New("job cannot be deleted")

	safeToken = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

// Options configures a Queue. The application owns every resource name.
type Options struct {
	// SubjectPrefix prefixes fact subjects, for example "jobs". A fact uses
	// {SubjectPrefix}.{type}.{id}.{fact}. The job stream must capture these
	// subjects and nothing else.
	SubjectPrefix string
	// SucceededRetention is how long a succeeded job stays, at least one
	// second. Zero selects DefaultSucceededRetention.
	SucceededRetention time.Duration
	// FailedRetention is how long a failed job stays, at least one second.
	// Zero keeps a failed job until a manual retry or Queue.Delete.
	FailedRetention time.Duration
	// Logger receives queue diagnostics. Nil discards them.
	Logger *slog.Logger
	// OnFailed observes a transition to failed, for application diagnostics.
	// It gets the job with its payload and must honor cancellation. The failed
	// fact stays authoritative: after a repeated delivery OnFailed can run
	// again, and after a crash it can be missing.
	OnFailed func(context.Context, Job)
}

// Queue runs durable background jobs on one job stream. The stream is a small
// event log: each job is a sequence of facts, and the state of a job is the
// fold of its facts. Enqueue and Retry append facts under optimistic
// concurrency control. Work runs jobs from a durable consumer on the queued and
// retried facts. Run keeps an in-memory view of all jobs for Get, List, Retry,
// and Delete.
type Queue struct {
	stream    jetstream.Stream
	log       *events.EncodedEventLog
	opts      Options
	logger    *slog.Logger
	view      *view
	projector *events.Projector
	now       func() time.Time
}

// New binds an already provisioned job stream. The stream must allow
// per-message TTLs and must not set a maximum age: an unfinished job stays
// until it finishes, and succeeded and failed jobs have their own retention.
// New panics when js or stream is nil and returns an error for invalid options
// or an unsuitable stream.
func New(js jetstream.JetStream, stream jetstream.Stream, opts Options) (*Queue, error) {
	if js == nil || stream == nil {
		panic("jobs: queue requires a JetStream context and a job stream")
	}
	if opts.SubjectPrefix == "" || strings.ContainsAny(opts.SubjectPrefix, "*> ") || strings.HasSuffix(opts.SubjectPrefix, ".") {
		return nil, errors.New("jobs: invalid subject prefix")
	}
	if opts.SucceededRetention == 0 {
		opts.SucceededRetention = DefaultSucceededRetention
	}
	if opts.SucceededRetention < time.Second {
		return nil, errors.New("jobs: succeeded retention must be at least one second")
	}
	if opts.FailedRetention != 0 && opts.FailedRetention < time.Second {
		return nil, errors.New("jobs: failed retention must be zero or at least one second")
	}
	config := stream.CachedInfo().Config
	if !config.AllowMsgTTL {
		return nil, errors.New("jobs: job stream must allow per-message TTLs")
	}
	if config.MaxAge != 0 {
		return nil, errors.New("jobs: job stream must not set a maximum age")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	q := &Queue{
		stream: stream, log: events.NewEncodedEventLog(js, stream, logger),
		opts: opts, logger: logger, now: time.Now,
		view: &view{subjects: []string{opts.SubjectPrefix + ".>"}, jobs: map[string]*entry{}},
	}
	var err error
	q.projector, err = events.NewDecodedProjector(js, stream, q.view, q.decodeForView, events.ProjectorOptions{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("jobs: configure job view: %w", err)
	}
	return q, nil
}

// ID derives a stable job ID from a job type and an application source
// identity. Repeated handoffs of the same source then accept the job once.
func ID(kind, source string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + source))
	return hex.EncodeToString(sum[:])
}

// FilterSubjects returns the subjects that a worker consumer of the given job
// types must receive: their queued and retried facts.
func (q *Queue) FilterSubjects(types ...string) []string {
	filters := make([]string, 0, 2*len(types))
	for _, kind := range types {
		filters = append(filters, q.subject(kind, "*", factQueued), q.subject(kind, "*", factRetried))
	}
	return filters
}

func (q *Queue) subject(kind, id, fact string) string {
	return q.opts.SubjectPrefix + "." + kind + "." + id + "." + fact
}

// jobFilter matches every fact of one job.
func (q *Queue) jobFilter(kind, id string) string {
	return q.opts.SubjectPrefix + "." + kind + "." + id + ".>"
}

func (q *Queue) decodeForView(data []byte) (events.DecodedEvent[*fact], error) {
	f, err := decodeFact(data)
	if err != nil {
		// One unreadable fact must not stop the view of all other jobs. The
		// view ignores a fact without an ID.
		q.logger.Warn("Job view ignored an unreadable fact")
		return events.DecodedEvent[*fact]{Event: new(fact)}, nil
	}
	return events.DecodedEvent[*fact]{Event: f, ID: f.ID}, nil
}

// append stores one fact of a job on the subject of its kind. messageID must
// be stable across retries of the same write. A nil expectation appends
// without a guard.
func (q *Queue) append(ctx context.Context, f *fact, messageID string, ttl time.Duration, expect *events.Expectation) (uint64, error) {
	data, err := f.encode()
	if err != nil {
		return 0, err
	}
	record := events.EncodedRecord{ID: f.Type + "." + f.ID + "." + messageID, Data: data, TTL: ttl}
	if expect != nil {
		return q.log.AppendAt(ctx, q.subject(f.Type, f.ID, f.Kind), record, *expect)
	}
	return q.log.AppendEventually(ctx, q.subject(f.Type, f.ID, f.Kind), record)
}

// Enqueue accepts a job once: it appends the queued fact only when the job has
// no facts. A repeated handoff changes nothing. Enqueue returns nil only when
// a fact of the job is stored, so sources must acknowledge their messages
// after it returns. A job whose facts are gone is unknown again: one that
// succeeded and passed its retention, or one that was deleted an hour ago.
func (q *Queue) Enqueue(ctx context.Context, spec Spec) error {
	if !safeToken.MatchString(spec.ID) || !safeToken.MatchString(spec.Type) || spec.MaxAttempts == 0 || spec.RetryDelay < time.Millisecond {
		return errors.New("jobs: invalid job specification")
	}
	now := q.now()
	stored := &factSpec{Payload: spec.Payload, MaxAttempts: spec.MaxAttempts, RetryDelayMillis: spec.RetryDelay.Milliseconds()}
	if !spec.ExpiresAt.IsZero() {
		stored.ExpiresAt = &spec.ExpiresAt
	}
	queued := &fact{ID: spec.ID, Type: spec.Type, Kind: factQueued, OccurredAt: now, CreatedAt: now, Spec: stored}
	expect := events.ExpectFilterSeq(q.jobFilter(spec.Type, spec.ID), 0)
	// The guard makes a repeated handoff safe. The message ID must not: the
	// stream would acknowledge a repeated ID in its duplicate window without
	// storing a fact, also when the facts of the job are gone.
	_, err := q.append(ctx, queued, fmt.Sprintf("%s.%d", factQueued, now.UnixNano()), 0, &expect)
	if !errors.Is(err, events.ErrConflict) {
		return err
	}
	// A conflict can come from a concurrent write that is not stored yet and
	// can still fail. Accept the handoff only when a fact of the job exists.
	_, seq, err := q.latestFact(ctx, spec.Type, spec.ID)
	if err == nil && seq == 0 {
		err = errors.New("jobs: concurrent job acceptance is not confirmed")
	}
	return err
}

// Get returns the current state of a job from the view, without its payload.
// It waits until Run has replayed the stream.
func (q *Queue) Get(ctx context.Context, id string) (Job, error) {
	if err := q.projector.WaitForStartup(ctx); err != nil {
		return Job{}, err
	}
	e, ok := q.view.get(id)
	if !ok {
		return Job{}, ErrNotFound
	}
	return e.job, nil
}

// Version identifies the state of a job that a caller read, without exposing
// a broker position. Retry and Delete act only on that state: a stale operator
// action cannot start a new attempt budget after another retry has already run
// and failed again.
func Version(job Job) string {
	return ID("job-version", fmt.Sprintf("%s:%d", job.ID, job.Revision))
}

// Retry starts one new attempt budget for a failed, unexpired job whose state
// matches expectedVersion. The job keeps its ID, payload, policy, and deadline.
// The retried fact is appended only when the failed fact is still the latest
// fact of the job, so concurrent retries start one budget. Retry needs Run.
func (q *Queue) Retry(ctx context.Context, id, expectedVersion string) (Job, error) {
	if err := q.projector.WaitForStartup(ctx); err != nil {
		return Job{}, err
	}
	e, ok := q.view.get(id)
	if !ok {
		return Job{}, ErrNotFound
	}
	now := q.now()
	if e.job.Status != Failed || e.job.Expired(now) || Version(e.job) != expectedVersion {
		return Job{}, ErrNotRetryable
	}
	// The view holds no payloads. Read the specification from the failed fact.
	msg, err := q.stream.GetMsg(ctx, e.specSeq)
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return Job{}, ErrNotRetryable
	}
	if err != nil {
		return Job{}, err
	}
	source, err := decodeFact(msg.Data)
	if err != nil || specOf(source) == nil || source.ID != id {
		return Job{}, ErrNotRetryable
	}
	retried := &fact{ID: id, Type: e.job.Type, Kind: factRetried, OccurredAt: now, CreatedAt: e.job.CreatedAt, Spec: specOf(source)}
	filter := q.jobFilter(e.job.Type, id)
	expect := events.ExpectFilterSeq(filter, e.job.Revision)
	seq, err := q.append(ctx, retried, fmt.Sprintf("%s.%d", factRetried, e.job.Revision), 0, &expect)
	if errors.Is(err, events.ErrConflict) {
		return Job{}, ErrNotRetryable
	}
	if err != nil {
		return Job{}, err
	}
	if err = q.awaitView(ctx, filter, seq); err != nil {
		return Job{}, err
	}
	if e, ok = q.view.get(id); !ok {
		return Job{}, ErrNotFound
	}
	return e.job, nil
}

// Delete removes a failed job whose state matches expectedVersion, for
// example a job that can never succeed. The deleted fact is appended only when
// the failed fact is still the latest fact of the job, so a concurrent retry
// and a delete cannot both take effect. Delete needs Run.
func (q *Queue) Delete(ctx context.Context, id, expectedVersion string) error {
	if err := q.projector.WaitForStartup(ctx); err != nil {
		return err
	}
	e, ok := q.view.get(id)
	if !ok {
		return ErrNotFound
	}
	if e.job.Status != Failed || Version(e.job) != expectedVersion {
		return ErrNotDeletable
	}
	deleted := &fact{ID: id, Type: e.job.Type, Kind: factDeleted, OccurredAt: q.now(), CreatedAt: e.job.CreatedAt}
	filter := q.jobFilter(e.job.Type, id)
	expect := events.ExpectFilterSeq(filter, e.job.Revision)
	seq, err := q.append(ctx, deleted, fmt.Sprintf("%s.%d", factDeleted, e.job.Revision), deletedRetention, &expect)
	if errors.Is(err, events.ErrConflict) {
		return ErrNotDeletable
	}
	if err != nil {
		return err
	}
	// When the purge fails, the failed fact stays. The job then shows again
	// after the deleted fact expires, and an operator can delete it again.
	if err = q.purgeBefore(ctx, e.job.Type, id, seq); err != nil {
		return err
	}
	return q.awaitView(ctx, filter, seq)
}

// awaitView waits until the view has applied the fact at seq. A worker can
// already have finished the job and removed that fact; the wait then cannot
// confirm the position, and the caller reads the view as it is.
func (q *Queue) awaitView(ctx context.Context, filter string, seq uint64) error {
	if err := q.projector.WaitFor(ctx, events.SubjectPosition(filter, seq)); err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return err
	}
	return nil
}

// latestFact returns the name and the sequence of the latest fact of a job.
// The sequence is zero when the job has no facts.
func (q *Queue) latestFact(ctx context.Context, kind, id string) (string, uint64, error) {
	msg, err := q.stream.GetLastMsgForSubject(ctx, q.jobFilter(kind, id))
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	return msg.Subject[strings.LastIndexByte(msg.Subject, '.')+1:], msg.Sequence, nil
}

// purgeBefore removes the facts of a job before seq. Leftover facts are not
// harmless: a view would build a job from them. Callers therefore report the
// error, so that the purge runs again.
func (q *Queue) purgeBefore(ctx context.Context, kind, id string, seq uint64) error {
	if err := q.stream.Purge(ctx, jetstream.WithPurgeSubject(q.jobFilter(kind, id)), jetstream.WithPurgeSequence(seq)); err != nil {
		return fmt.Errorf("jobs: remove earlier job facts: %w", err)
	}
	return nil
}

// ListOptions selects a page of jobs.
type ListOptions struct {
	// Status keeps only jobs in this state. Unknown keeps every job.
	Status Status
	// Limit is the page size. It must be positive.
	Limit int
	// Cursor continues after the page that returned it. Empty starts at the
	// newest job. Applications must not expose it unprotected.
	Cursor string
}

// List returns jobs ordered by last update, newest first, from the view. It
// waits for the current stream boundary, then copies and sorts the matching
// summaries without their payloads. Jobs that change while a caller pages can move
// between pages, so callers should remove duplicate IDs.
func (q *Queue) List(ctx context.Context, opts ListOptions) ([]Job, string, error) {
	if opts.Limit <= 0 {
		return nil, "", errors.New("jobs: list limit must be positive")
	}
	afterTime, afterID, err := parseCursor(opts.Cursor)
	if err != nil {
		return nil, "", err
	}
	if err = q.projector.WaitForStartup(ctx); err != nil {
		return nil, "", err
	}
	// A live change notification can come from a different replica.
	if err = q.projector.WaitForCurrent(ctx); err != nil {
		return nil, "", err
	}
	jobs := q.view.list(opts.Status, q.now(), q.opts.SucceededRetention, q.opts.FailedRetention)
	slices.SortFunc(jobs, func(a, b Job) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	start := 0
	if opts.Cursor != "" {
		start = sort.Search(len(jobs), func(i int) bool {
			if c := jobs[i].UpdatedAt.Compare(afterTime); c != 0 {
				return c < 0
			}
			return jobs[i].ID > afterID
		})
	}
	end := min(start+opts.Limit, len(jobs))
	page := jobs[start:end]
	next := ""
	if end < len(jobs) && len(page) > 0 {
		last := page[len(page)-1]
		next = fmt.Sprintf("%d:%s", last.UpdatedAt.UnixNano(), last.ID)
	}
	return page, next, nil
}

func parseCursor(cursor string) (time.Time, string, error) {
	if cursor == "" {
		return time.Time{}, "", nil
	}
	nanos, id, ok := strings.Cut(cursor, ":")
	value, err := strconv.ParseInt(nanos, 10, 64)
	if !ok || err != nil || !safeToken.MatchString(id) {
		return time.Time{}, "", errors.New("jobs: invalid list cursor")
	}
	return time.Unix(0, value), id, nil
}

// Run keeps the job view current until cancellation. It replays the job
// stream, follows new facts, and drops jobs that passed their retention. Run it
// once in each process that calls Get, List, Retry, or Delete. Enqueue and Work
// do not need it.
func (q *Queue) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := q.projector.Run(ctx); err != nil && ctx.Err() == nil {
			return fmt.Errorf("jobs: job view stopped: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := q.projector.WaitForStartup(ctx); err != nil {
			return nil
		}
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			q.sweep()
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	})
	return g.Wait()
}

// sweep drops jobs whose fact the stream no longer holds. The stream removes
// an expired fact without an event, so the view follows the same retention by
// clock.
func (q *Queue) sweep() {
	q.view.sweep(q.now(), q.opts.SucceededRetention, q.opts.FailedRetention)
}

// Work runs jobs from consumer with handler until cancellation. The
// application owns the consumer: it must filter to FilterSubjects of the job
// types that handler performs, use explicit acknowledgement, allow unlimited
// deliveries, and set an acknowledgement wait of at least one minute. A job
// that waits for a retry holds an unacknowledged delivery, so the consumer
// should not limit pending acknowledgements; concurrency bounds running jobs.
func (q *Queue) Work(ctx context.Context, consumer jetstream.Consumer, handler Handler, concurrency int) error {
	if handler == nil {
		panic("jobs: work requires a handler")
	}
	worker, err := events.NewDurableWorker(consumer, func(ctx context.Context, d events.DurableDelivery) error {
		return q.process(ctx, d, handler)
	}, events.DurableWorkerOptions{MaxConcurrent: concurrency, RetryDelay: infrastructureRetry, AckTimeout: 5 * time.Second, HeartbeatInterval: heartbeatInterval, Logger: q.logger})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// process performs one attempt. The delivery is a queued or retried fact; its
// delivery count is the attempt number. The consumer's acknowledgement wait is
// the claim, and the durable worker's progress heartbeat renews it.
//
// A delivery can repeat after its outcome is stored: the acknowledgement was
// lost, or the purge after the outcome failed. process then only finishes the
// purge. A delivery whose earlier deliveries ended without a stored outcome
// runs the handler again, also beyond the attempt budget, because the result
// of those deliveries is unknown.
func (q *Queue) process(ctx context.Context, d events.DurableDelivery, handler Handler) error {
	var spec *factSpec
	// Workers consume only queued and retried facts.
	delivered, err := decodeFact(d.Data)
	if err == nil && delivered.Kind != factFailed {
		spec = specOf(delivered)
	}
	if spec == nil || !safeToken.MatchString(delivered.ID) || !safeToken.MatchString(delivered.Type) || spec.MaxAttempts == 0 {
		return events.TerminateDelivery("invalid job fact", errors.New("invalid job fact"))
	}
	job := Job{
		ID: delivered.ID, Type: delivered.Type, Status: Running, MaxAttempts: spec.MaxAttempts,
		Attempts:   uint32(min(d.NumDelivered, math.MaxUint32)),
		CreatedAt:  delivered.CreatedAt,
		Payload:    spec.Payload,
		RetryDelay: time.Duration(spec.RetryDelayMillis) * time.Millisecond,
		Revision:   d.StreamSequence,
	}
	if spec.ExpiresAt != nil {
		job.ExpiresAt = *spec.ExpiresAt
	}
	// The job already has an outcome, or an operator started a new attempt
	// budget after this fact. Remove what the outcome or the retry replaced.
	latest, seq, err := q.latestFact(ctx, job.Type, job.ID)
	if err != nil {
		return err
	}
	if seq > d.StreamSequence && (latest == factSucceeded || latest == factFailed || latest == factDeleted || latest == factRetried) {
		return q.purgeBefore(ctx, job.Type, job.ID, seq)
	}
	if job.Expired(q.now()) {
		job.Attempts--
		return q.fail(ctx, job, spec, d, "expired")
	}
	// The started and attempt-failed facts only feed the view. A failed write
	// must not stop or repeat the attempt.
	q.note(ctx, d, q.newFact(job, factStarted))

	err = handler(ctx, job)
	if ctx.Err() != nil {
		// Shutdown. A finished attempt records its result, so that the next
		// delivery does not repeat it. A failure can be an effect of the
		// cancellation: it is not recorded, and the next delivery runs the job
		// again.
		if err == nil {
			writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownWriteTimeout)
			defer cancel()
			if q.succeed(writeCtx, job, d) != nil {
				q.logger.Warn("Job result was not recorded before shutdown; the job runs again", "job_id", job.ID)
			}
		}
		return ctx.Err()
	}
	if err == nil {
		return q.succeed(ctx, job, d)
	}
	// Only safe failure categories enter job state and logs; handler error
	// text never does.
	category, permanent := "attempt_failed", false
	if failure, ok := errors.AsType[*Failure](err); ok {
		if safeToken.MatchString(failure.Category) {
			category = failure.Category
		}
		permanent = failure.Permanent
	}
	now := q.now()
	if permanent || job.Attempts >= job.MaxAttempts || job.Expired(now) {
		return q.fail(ctx, job, spec, d, category)
	}
	delay := Backoff(job.RetryDelay, job.Attempts)
	if !job.ExpiresAt.IsZero() {
		delay = min(delay, job.ExpiresAt.Sub(now))
	}
	next := now.Add(delay)
	attemptFailed := q.newFact(job, factAttemptFailed)
	attemptFailed.Failure, attemptFailed.NextAttemptAt = category, &next
	q.note(ctx, d, attemptFailed)
	return events.RetryDeliveryAfter(errors.New(category), delay)
}

// newFact returns a fact of the given kind about the current attempt of job.
func (q *Queue) newFact(job Job, kind string) *fact {
	return &fact{ID: job.ID, Type: job.Type, Kind: kind, OccurredAt: q.now(), CreatedAt: job.CreatedAt, Attempts: job.Attempts}
}

// note appends a fact that only the view needs.
func (q *Queue) note(ctx context.Context, d events.DurableDelivery, f *fact) {
	messageID := fmt.Sprintf("%s.%d.%d", f.Kind, d.StreamSequence, f.Attempts)
	if _, err := q.append(ctx, f, messageID, 0, nil); err != nil && ctx.Err() == nil {
		q.logger.Warn("Job progress was not recorded", "job_id", f.ID, "fact", f.Kind)
	}
}

// succeed records success and then removes the earlier facts of the job. The
// succeeded fact expires after the retention, so a succeeded job leaves one
// small fact for a short time. When a step fails, the delivery returns: the
// next delivery runs the job again, or finishes the purge.
func (q *Queue) succeed(ctx context.Context, job Job, d events.DurableDelivery) error {
	succeeded := q.newFact(job, factSucceeded)
	succeeded.MaxAttempts = job.MaxAttempts
	seq, err := q.append(ctx, succeeded, fmt.Sprintf("%s.%d", factSucceeded, d.StreamSequence), q.opts.SucceededRetention, nil)
	if err != nil {
		return err
	}
	return q.purgeBefore(ctx, job.Type, job.ID, seq)
}

// fail records the final failure before the delivery is acknowledged, and then
// removes the earlier facts of the job. The failed fact carries the
// specification, so a failed job is one fact that a manual retry can read.
// When the write fails, the delivery returns and the job runs again. When the
// purge fails, the next delivery finishes it.
func (q *Queue) fail(ctx context.Context, job Job, spec *factSpec, d events.DurableDelivery, category string) error {
	job.Status, job.Failure = Failed, category
	failed := q.newFact(job, factFailed)
	failed.Failure, failed.Spec = category, spec
	seq, err := q.append(ctx, failed, fmt.Sprintf("%s.%d", factFailed, d.StreamSequence), q.opts.FailedRetention, nil)
	if err != nil {
		return err
	}
	if q.opts.OnFailed != nil {
		q.opts.OnFailed(ctx, job)
	}
	return q.purgeBefore(ctx, job.Type, job.ID, seq)
}

// Backoff doubles the initial delay per attempt, capped at thirty minutes.
func Backoff(initial time.Duration, attempt uint32) time.Duration {
	delay := min(max(initial, time.Millisecond), maxBackoff)
	for i := uint32(1); i < attempt && delay < maxBackoff; i++ {
		delay = min(delay*2, maxBackoff)
	}
	return delay
}
