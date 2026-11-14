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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
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
)

// The fact names are the last subject token of each fact.
const (
	factQueued        = "queued"
	factRetried       = "retried"
	factStarted       = "started"
	factAttemptFailed = "attempt_failed"
	factSucceeded     = "succeeded"
	factFailed        = "failed"
)

var (
	// ErrNotFound reports a job ID that the view does not hold.
	ErrNotFound = errors.New("job not found")
	// ErrNotRetryable reports a job that is not failed, has expired, or changed
	// after the caller read it.
	ErrNotRetryable = errors.New("job cannot be retried")

	safeToken = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

// Options configures a Queue. The application owns every resource name.
type Options struct {
	// SubjectPrefix prefixes fact subjects, for example "jobs". A fact uses
	// {SubjectPrefix}.{type}.{id}.{fact}. The job stream must capture these
	// subjects and nothing else.
	SubjectPrefix string
	// SucceededRetention is how long a succeeded job stays, at least one
	// second. Zero selects DefaultSucceededRetention. Other jobs stay for the
	// maximum age of the job stream.
	SucceededRetention time.Duration
	// Logger receives queue diagnostics. Nil discards them.
	Logger *slog.Logger
	// OnFailed observes each transition to failed, for application
	// diagnostics. It gets the job with its payload and must honor
	// cancellation. The failed fact stays authoritative.
	OnFailed func(context.Context, Job)
}

// Queue runs durable background jobs on one job stream. The stream is a small
// event log: each job is a sequence of facts, and the state of a job is the
// fold of its facts. Enqueue and Retry append facts under optimistic
// concurrency control. Work runs jobs from a durable consumer on the queued and
// retried facts. Run keeps an in-memory view of all jobs for Get, List, and
// Retry.
type Queue struct {
	js        jetstream.JetStream
	stream    jetstream.Stream
	log       *events.EncodedEventLog
	opts      Options
	logger    *slog.Logger
	view      *view
	projector *events.Projector
	// maxAge is the maximum age of the job stream. Zero means no limit.
	maxAge time.Duration
	now    func() time.Time
}

// New binds an already provisioned job stream. The stream must allow
// per-message TTLs and should set a maximum age, which bounds how long
// unfinished and failed jobs stay. New panics when js or stream is nil and
// returns an error for invalid options.
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
	config := stream.CachedInfo().Config
	if !config.AllowMsgTTL {
		return nil, errors.New("jobs: job stream must allow per-message TTLs")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	q := &Queue{
		js: js, stream: stream, log: events.NewEncodedEventLog(js, stream, logger),
		opts: opts, logger: logger, maxAge: config.MaxAge, now: time.Now,
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

func (q *Queue) decodeForView(data []byte) (events.DecodedEvent[*jobpb.JobFact], error) {
	fact := new(jobpb.JobFact)
	if err := proto.Unmarshal(data, fact); err != nil {
		// One unreadable fact must not stop the view of all other jobs. The
		// view ignores a fact without an ID.
		q.logger.Warn("Job view ignored an unreadable fact")
		return events.DecodedEvent[*jobpb.JobFact]{Event: new(jobpb.JobFact)}, nil
	}
	return events.DecodedEvent[*jobpb.JobFact]{Event: fact, ID: fact.Id}, nil
}

// append stores one fact of a job. messageID must be stable across retries of
// the same write. A zero expectation appends without a guard.
func (q *Queue) append(ctx context.Context, fact *jobpb.JobFact, name, messageID string, ttl time.Duration, expect *events.Expectation) (uint64, error) {
	data, err := proto.Marshal(fact)
	if err != nil {
		return 0, err
	}
	record := events.EncodedRecord{ID: fact.Type + "." + fact.Id + "." + messageID, Data: data, TTL: ttl}
	if expect != nil {
		return q.log.AppendAt(ctx, q.subject(fact.Type, fact.Id, name), record, *expect)
	}
	return q.log.AppendEventually(ctx, q.subject(fact.Type, fact.Id, name), record)
}

// Enqueue accepts a job once: it appends the queued fact only when the job has
// no facts. A repeated handoff changes nothing. A job that succeeded and
// passed its retention is unknown again, so sources must acknowledge their
// messages after Enqueue returns.
func (q *Queue) Enqueue(ctx context.Context, spec Spec) error {
	if !safeToken.MatchString(spec.ID) || !safeToken.MatchString(spec.Type) || spec.MaxAttempts == 0 || spec.RetryDelay < time.Millisecond {
		return errors.New("jobs: invalid job specification")
	}
	now := timestamppb.New(q.now())
	stored := &jobpb.JobSpec{Payload: spec.Payload, MaxAttempts: spec.MaxAttempts, RetryDelayMillis: spec.RetryDelay.Milliseconds()}
	if !spec.ExpiresAt.IsZero() {
		stored.ExpiresAt = timestamppb.New(spec.ExpiresAt)
	}
	fact := &jobpb.JobFact{Id: spec.ID, Type: spec.Type, OccurredAt: now, CreatedAt: now, Fact: &jobpb.JobFact_Queued{Queued: &jobpb.JobQueued{Spec: stored}}}
	expect := events.ExpectFilterSeq(q.jobFilter(spec.Type, spec.ID), 0)
	if _, err := q.append(ctx, fact, factQueued, factQueued, 0, &expect); err != nil && !errors.Is(err, events.ErrConflict) {
		return err
	}
	return nil
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

// RetryVersion identifies the state of a job that a caller read, without
// exposing a broker position. A stale operator action cannot start a new
// attempt budget after another retry has already run and failed again.
func RetryVersion(job Job) string {
	return ID("retry-version", fmt.Sprintf("%s:%d", job.ID, job.Revision))
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
	if e.job.Status != Failed || e.job.Expired(now) || RetryVersion(e.job) != expectedVersion {
		return Job{}, ErrNotRetryable
	}
	// The view holds no payloads. Read the specification from the fact that
	// the workers consumed.
	msg, err := q.stream.GetMsg(ctx, e.specSeq)
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return Job{}, ErrNotRetryable
	}
	if err != nil {
		return Job{}, err
	}
	source := new(jobpb.JobFact)
	if err = proto.Unmarshal(msg.Data, source); err != nil || specOf(source) == nil || source.Id != id {
		return Job{}, ErrNotRetryable
	}
	fact := &jobpb.JobFact{
		Id: id, Type: e.job.Type, OccurredAt: timestamppb.New(now), CreatedAt: timestamppb.New(e.job.CreatedAt),
		Fact: &jobpb.JobFact_Retried{Retried: &jobpb.JobRetried{Spec: specOf(source)}},
	}
	filter := q.jobFilter(e.job.Type, id)
	expect := events.ExpectFilterSeq(filter, e.job.Revision)
	seq, err := q.append(ctx, fact, factRetried, fmt.Sprintf("%s.%d", factRetried, e.job.Revision), 0, &expect)
	if errors.Is(err, events.ErrConflict) {
		return Job{}, ErrNotRetryable
	}
	if err != nil {
		return Job{}, err
	}
	if err = q.projector.WaitFor(ctx, events.SubjectPosition(filter, seq)); err != nil {
		return Job{}, err
	}
	if e, ok = q.view.get(id); !ok {
		return Job{}, ErrNotFound
	}
	return e.job, nil
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
// reads no storage, so polling is cheap. Payloads are omitted. List waits until
// Run has replayed the stream. Jobs that change while a caller pages can move
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
	jobs := q.view.list(opts.Status)
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
// once in each process that calls Get, List, or Retry. Enqueue and Work do not
// need it.
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

// sweep drops jobs whose facts the stream no longer holds. The stream removes
// facts by age without an event, so the view follows the same limits by clock.
func (q *Queue) sweep() {
	for _, job := range q.view.sweep(q.now(), q.maxAge, q.opts.SucceededRetention) {
		q.logger.Warn("Job passed the maximum age of the job stream before it finished", "job_id", job.ID, "job_type", job.Type, "attempts", job.Attempts)
	}
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

// specOf returns the specification of a queued or retried fact, or nil.
func specOf(fact *jobpb.JobFact) *jobpb.JobSpec {
	switch f := fact.GetFact().(type) {
	case *jobpb.JobFact_Queued:
		return f.Queued.GetSpec()
	case *jobpb.JobFact_Retried:
		return f.Retried.GetSpec()
	}
	return nil
}

// process performs one attempt. The delivery is a queued or retried fact; its
// delivery count is the attempt number. The consumer's acknowledgement wait is
// the claim, and the durable worker's progress heartbeat renews it.
func (q *Queue) process(ctx context.Context, d events.DurableDelivery, handler Handler) error {
	fact := new(jobpb.JobFact)
	spec := (*jobpb.JobSpec)(nil)
	if err := proto.Unmarshal(d.Data, fact); err == nil {
		spec = specOf(fact)
	}
	if spec == nil || !safeToken.MatchString(fact.Id) || !safeToken.MatchString(fact.Type) || spec.MaxAttempts == 0 {
		return events.TerminateDelivery("invalid job fact", errors.New("invalid job fact"))
	}
	job := Job{
		ID: fact.Id, Type: fact.Type, Status: Running, MaxAttempts: spec.MaxAttempts,
		Attempts:   uint32(min(d.NumDelivered, math.MaxUint32)),
		CreatedAt:  fact.GetCreatedAt().AsTime(),
		Payload:    spec.Payload,
		RetryDelay: time.Duration(spec.RetryDelayMillis) * time.Millisecond,
		Revision:   d.StreamSequence,
	}
	if spec.ExpiresAt != nil {
		job.ExpiresAt = spec.ExpiresAt.AsTime()
	}
	if job.Expired(q.now()) {
		job.Attempts--
		return q.fail(ctx, job, d, "expired")
	}
	if job.Attempts > job.MaxAttempts {
		// Deliveries that ended without an outcome used the budget, for example
		// a lost acknowledgement of the last failure.
		job.Attempts = job.MaxAttempts
		return q.fail(ctx, job, d, "attempts_exhausted")
	}
	// The started and attempt-failed facts only feed the view. A failed write
	// must not stop or repeat the attempt.
	q.note(ctx, job, d, factStarted, &jobpb.JobFact{Fact: &jobpb.JobFact_Started{Started: &jobpb.JobStarted{Attempt: job.Attempts}}})

	err := handler(ctx, job)
	if ctx.Err() != nil {
		// Shutdown. A finished attempt keeps its result, so another worker does
		// not repeat it. A failure can be an effect of the cancellation: it is
		// not recorded, and the next delivery runs the job again.
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
		return q.fail(ctx, job, d, category)
	}
	delay := Backoff(job.RetryDelay, job.Attempts)
	if !job.ExpiresAt.IsZero() {
		delay = min(delay, job.ExpiresAt.Sub(now))
	}
	q.note(ctx, job, d, factAttemptFailed, &jobpb.JobFact{Fact: &jobpb.JobFact_AttemptFailed{AttemptFailed: &jobpb.JobAttemptFailed{
		Attempt: job.Attempts, Failure: category, NextAttemptAt: timestamppb.New(now.Add(delay)),
	}}})
	return events.RetryDeliveryAfter(errors.New(category), delay)
}

// fill completes a fact of the job that delivery d runs.
func (q *Queue) fill(fact *jobpb.JobFact, job Job) *jobpb.JobFact {
	fact.Id, fact.Type = job.ID, job.Type
	fact.OccurredAt = timestamppb.New(q.now())
	fact.CreatedAt = timestamppb.New(job.CreatedAt)
	return fact
}

// note appends a fact that only the view needs.
func (q *Queue) note(ctx context.Context, job Job, d events.DurableDelivery, name string, fact *jobpb.JobFact) {
	messageID := fmt.Sprintf("%s.%d.%d", name, d.StreamSequence, job.Attempts)
	if _, err := q.append(ctx, q.fill(fact, job), name, messageID, 0, nil); err != nil && ctx.Err() == nil {
		q.logger.Warn("Job progress was not recorded", "job_id", job.ID, "fact", name)
	}
}

// succeed records success and then removes the earlier facts of the job. The
// succeeded fact expires after the retention, so a succeeded job leaves one
// small fact for a short time. A failed purge leaves the earlier facts until
// the maximum age of the stream.
func (q *Queue) succeed(ctx context.Context, job Job, d events.DurableDelivery) error {
	fact := q.fill(&jobpb.JobFact{Fact: &jobpb.JobFact_Succeeded{Succeeded: &jobpb.JobSucceeded{Attempts: job.Attempts, MaxAttempts: job.MaxAttempts}}}, job)
	seq, err := q.append(ctx, fact, factSucceeded, fmt.Sprintf("%s.%d", factSucceeded, d.StreamSequence), q.opts.SucceededRetention, nil)
	if err != nil {
		return err
	}
	if err = q.stream.Purge(ctx, jetstream.WithPurgeSubject(q.jobFilter(job.Type, job.ID)), jetstream.WithPurgeSequence(seq)); err != nil {
		q.logger.Warn("Facts of a succeeded job were not removed", "job_id", job.ID)
	}
	return nil
}

// fail records the final failure before the delivery is acknowledged. When the
// write fails, the delivery returns and reaches this outcome again.
func (q *Queue) fail(ctx context.Context, job Job, d events.DurableDelivery, category string) error {
	job.Status, job.Failure = Failed, category
	fact := q.fill(&jobpb.JobFact{Fact: &jobpb.JobFact_Failed{Failed: &jobpb.JobFailed{Attempts: job.Attempts, Failure: category}}}, job)
	if _, err := q.append(ctx, fact, factFailed, fmt.Sprintf("%s.%d", factFailed, d.StreamSequence), 0, nil); err != nil {
		return err
	}
	if q.opts.OnFailed != nil {
		q.opts.OnFailed(ctx, job)
	}
	return nil
}

// Backoff doubles the initial delay per attempt, capped at thirty minutes.
func Backoff(initial time.Duration, attempt uint32) time.Duration {
	delay := min(max(initial, time.Millisecond), maxBackoff)
	for i := uint32(1); i < attempt && delay < maxBackoff; i++ {
		delay = min(delay*2, maxBackoff)
	}
	return delay
}
