package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
)

const (
	leaseDuration     = time.Minute
	heartbeatInterval = 15 * time.Second
	repairInterval    = 30 * time.Second
	maxBackoff        = 30 * time.Minute
	// DefaultSucceededRetention is used when Options.SucceededRetention is zero.
	DefaultSucceededRetention = 7 * 24 * time.Hour
)

var (
	// ErrNotFound reports a job ID without a current record.
	ErrNotFound = errors.New("job not found")
	// ErrNotRetryable reports a job that is not failed, has expired, or whose
	// attempt budget changed after the caller read it.
	ErrNotRetryable = errors.New("job cannot be retried")

	safeToken = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

// KeyValue is the record store. jetstreamutil.KeyValue satisfies it: Get must
// read through the stream leader, and UpdateWithTTL lets succeeded records
// expire. The bucket must allow per-message TTLs.
type KeyValue interface {
	jetstream.KeyValue
	UpdateWithTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error)
}

var _ KeyValue = (*jetstreamutil.KeyValue)(nil)

// Options configures a Queue. The application owns every resource name.
type Options struct {
	// KeyPrefix prefixes record keys, for example "job.". It must end with a
	// dot, and no other record family in the bucket may use it.
	KeyPrefix string
	// SubjectPrefix prefixes dispatch subjects, for example "jobs". A dispatch
	// uses {SubjectPrefix}.{type}.{id}. The dispatch stream must capture these
	// subjects, keep at most one message per subject, and should set a maximum
	// age; the repair pass publishes again for unfinished jobs.
	SubjectPrefix string
	// SucceededRetention is how long a succeeded record stays. Zero selects
	// DefaultSucceededRetention. Failed and unfinished records do not expire.
	SucceededRetention time.Duration
	// Logger receives queue diagnostics. Nil discards them.
	Logger *slog.Logger
	// OnFailed observes each transition to failed, for application
	// diagnostics. It must honor cancellation. The record stays authoritative.
	OnFailed func(context.Context, Job)
}

// Queue runs durable background jobs. A key-value bucket owns current job
// state; a stream carries replayable dispatch references. A repair pass closes
// the gap between a stored record and its dispatch, including after a manual
// retry or a restore.
type Queue struct {
	js     jetstream.JetStream
	kv     KeyValue
	opts   Options
	logger *slog.Logger

	// index holds payload-free summaries of every current record. Run's
	// watcher maintains it; ready closes after the initial sync.
	mu        sync.RWMutex
	index     map[string]Job
	ready     chan struct{}
	readyOnce sync.Once
}

// New binds already provisioned resources. It panics when js or kv is nil and
// returns an error for invalid options.
func New(js jetstream.JetStream, kv KeyValue, opts Options) (*Queue, error) {
	if js == nil || kv == nil {
		panic("jobs: queue requires a JetStream context and a key-value bucket")
	}
	if !strings.HasSuffix(opts.KeyPrefix, ".") || len(opts.KeyPrefix) < 2 {
		return nil, errors.New("jobs: key prefix must end with a dot")
	}
	if opts.SubjectPrefix == "" || strings.ContainsAny(opts.SubjectPrefix, "*> ") || strings.HasSuffix(opts.SubjectPrefix, ".") {
		return nil, errors.New("jobs: invalid subject prefix")
	}
	if opts.SucceededRetention < 0 {
		return nil, errors.New("jobs: succeeded retention must not be negative")
	}
	if opts.SucceededRetention == 0 {
		opts.SucceededRetention = DefaultSucceededRetention
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Queue{js: js, kv: kv, opts: opts, logger: logger, index: map[string]Job{}, ready: make(chan struct{})}, nil
}

// ID derives a stable job ID from a job type and an application source
// identity. Repeated handoffs of the same source then accept the job once.
func ID(kind, source string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + source))
	return hex.EncodeToString(sum[:])
}

// FilterSubjects returns the dispatch subjects of the given job types, for an
// application-owned consumer configuration.
func (q *Queue) FilterSubjects(types ...string) []string {
	filters := make([]string, 0, len(types))
	for _, kind := range types {
		filters = append(filters, q.opts.SubjectPrefix+"."+kind+".*")
	}
	return filters
}

func (q *Queue) key(id string) string { return q.opts.KeyPrefix + id }
func (q *Queue) subject(r *jobpb.JobRecord) string {
	return q.opts.SubjectPrefix + "." + r.Type + "." + r.Id
}

// Enqueue accepts a job once. A repeated handoff never replaces its payload or
// resets its attempts. When the dispatch publish fails after the record is
// stored, the repair pass publishes it later.
func (q *Queue) Enqueue(ctx context.Context, spec Spec) error {
	if !safeToken.MatchString(spec.ID) || !safeToken.MatchString(spec.Type) || spec.MaxAttempts == 0 || spec.RetryDelay < time.Millisecond {
		return errors.New("jobs: invalid job specification")
	}
	now := timestamppb.Now()
	record := &jobpb.JobRecord{
		Id: spec.ID, Type: spec.Type, Payload: spec.Payload,
		MaxAttempts: spec.MaxAttempts, RetryDelayMillis: spec.RetryDelay.Milliseconds(),
		Status: jobpb.JobStatus_JOB_STATUS_QUEUED, Generation: 1,
		CreatedAt: now, UpdatedAt: now, NextAttemptAt: now,
	}
	if !spec.ExpiresAt.IsZero() {
		record.ExpiresAt = timestamppb.New(spec.ExpiresAt)
	}
	data, err := proto.Marshal(record)
	if err != nil {
		return err
	}
	if _, err = q.kv.Create(ctx, q.key(spec.ID), data); err != nil {
		if !jetstreamutil.IsSequenceConflict(err) {
			return err
		}
		// A create conflict can be a write in progress. Read through the leader;
		// absence is an error, so the caller keeps its source and retries.
		if record, _, err = q.read(ctx, spec.ID); err != nil {
			return err
		}
	}
	if record.Status != jobpb.JobStatus_JOB_STATUS_QUEUED {
		return nil
	}
	return q.dispatch(ctx, record)
}

func (q *Queue) dispatch(ctx context.Context, r *jobpb.JobRecord) error {
	data, err := proto.Marshal(&jobpb.JobDispatch{Id: r.Id, Generation: r.Generation})
	if err != nil {
		return err
	}
	// Each attempt budget and attempt count gets its own message ID. Repeated
	// publications for the same due attempt collapse in the duplicate window.
	_, err = q.js.Publish(ctx, q.subject(r), data, jetstream.WithMsgID(fmt.Sprintf("%s:%d:%d", r.Id, r.Generation, r.Attempts)))
	return err
}

func (q *Queue) read(ctx context.Context, id string) (*jobpb.JobRecord, uint64, error) {
	if !safeToken.MatchString(id) {
		return nil, 0, ErrNotFound
	}
	entry, err := q.kv.Get(ctx, q.key(id))
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	record := new(jobpb.JobRecord)
	if err = proto.Unmarshal(entry.Value(), record); err != nil {
		return nil, 0, err
	}
	return record, entry.Revision(), nil
}

// Get returns current state, including the private payload. Applications must
// map it to a safe summary before they show it.
func (q *Queue) Get(ctx context.Context, id string) (Job, error) {
	record, _, err := q.read(ctx, id)
	if err != nil {
		return Job{}, err
	}
	return jobFromRecord(record), nil
}

// save writes a revision-checked update. A succeeded record gets the retention
// TTL; source consumers have acknowledged its work, so expiry cannot enqueue it
// again.
func (q *Queue) save(ctx context.Context, r *jobpb.JobRecord, revision uint64) (uint64, error) {
	r.UpdatedAt = timestamppb.Now()
	data, err := proto.Marshal(r)
	if err != nil {
		return 0, err
	}
	if r.Status == jobpb.JobStatus_JOB_STATUS_SUCCEEDED {
		return q.kv.UpdateWithTTL(ctx, q.key(r.Id), data, revision, q.opts.SucceededRetention)
	}
	return q.kv.Update(ctx, q.key(r.Id), data, revision)
}

// RetryVersion identifies the attempt budget that a caller read, without
// exposing revisions or generations. A stale operator action cannot start a
// new budget after another retry has already run and failed again.
func RetryVersion(job Job) string {
	return ID("retry-version", fmt.Sprintf("%s:%d", job.ID, job.Generation))
}

// Retry starts one new attempt budget for a failed, unexpired job whose budget
// matches expectedVersion. The job keeps its ID, payload, and deadline. When
// the dispatch publish fails, the repair pass publishes it later.
func (q *Queue) Retry(ctx context.Context, id, expectedVersion string) (Job, error) {
	record, revision, err := q.read(ctx, id)
	if err != nil {
		return Job{}, err
	}
	job := jobFromRecord(record)
	if job.Status != Failed || job.Expired(time.Now()) || RetryVersion(job) != expectedVersion {
		return Job{}, ErrNotRetryable
	}
	record.Generation++
	record.Attempts = 0
	record.Status = jobpb.JobStatus_JOB_STATUS_QUEUED
	record.Failure = ""
	record.Owner = ""
	record.LeaseUntil = nil
	record.NextAttemptAt = timestamppb.Now()
	if _, err = q.save(ctx, record, revision); err != nil {
		if jetstreamutil.IsSequenceConflict(err) {
			return Job{}, ErrNotRetryable
		}
		return Job{}, err
	}
	if err = q.dispatch(ctx, record); err != nil {
		q.logger.Warn("Job retry dispatch awaits repair", "job_id", record.Id)
	}
	return jobFromRecord(record), nil
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

// List returns jobs ordered by last update, newest first, from the in-memory
// index. It reads no storage, so polling is cheap. Payloads are omitted. The
// index is current once Run has synchronized it; List waits until then. Jobs
// that change while a caller pages can move between pages, so callers should
// remove duplicate IDs.
func (q *Queue) List(ctx context.Context, opts ListOptions) ([]Job, string, error) {
	if opts.Limit <= 0 {
		return nil, "", errors.New("jobs: list limit must be positive")
	}
	afterTime, afterID, err := parseCursor(opts.Cursor)
	if err != nil {
		return nil, "", err
	}
	select {
	case <-q.ready:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	q.mu.RLock()
	jobs := make([]Job, 0, len(q.index))
	for _, job := range q.index {
		if opts.Status == Unknown || job.Status == opts.Status {
			jobs = append(jobs, job)
		}
	}
	q.mu.RUnlock()
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

// Run maintains the queue in this process until cancellation: one filtered
// watcher keeps the List index and publishes dispatches for due jobs, including
// after a lost publish, a retry delay, an expired lease, or a restore. Run it
// once per process that enqueues, works, or lists jobs.
func (q *Queue) Run(ctx context.Context) error {
	watcher, err := q.kv.Watch(ctx, q.opts.KeyPrefix+">")
	if err != nil {
		return err
	}
	defer func() { _ = watcher.Stop() }()
	// active holds unfinished records; the index is a hint and consumers still
	// claim authoritative revisions.
	active := map[string]*jobpb.JobRecord{}
	timer := time.NewTimer(repairInterval)
	defer timer.Stop()
	publishDue := func() {
		now := time.Now()
		next := now.Add(repairInterval)
		for _, record := range active {
			if due := dueAt(record); due.After(now) {
				next = minTime(next, due)
				continue
			}
			if err := q.dispatch(ctx, record); err != nil && ctx.Err() == nil {
				q.logger.Warn("Job dispatch awaits repair", "job_id", record.Id)
			}
		}
		timer.Reset(time.Until(next))
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case entry, ok := <-watcher.Updates():
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("jobs: watcher closed")
			}
			if entry == nil {
				q.readyOnce.Do(func() { close(q.ready) })
				publishDue()
				continue
			}
			id := strings.TrimPrefix(entry.Key(), q.opts.KeyPrefix)
			if entry.Operation() != jetstream.KeyValuePut {
				delete(active, id)
				q.mu.Lock()
				delete(q.index, id)
				q.mu.Unlock()
				continue
			}
			record := new(jobpb.JobRecord)
			if err := proto.Unmarshal(entry.Value(), record); err != nil {
				return errors.New("jobs: invalid job record")
			}
			summary := jobFromRecord(record)
			summary.Payload = nil
			q.mu.Lock()
			q.index[record.Id] = summary
			q.mu.Unlock()
			if record.Status == jobpb.JobStatus_JOB_STATUS_QUEUED || record.Status == jobpb.JobStatus_JOB_STATUS_RUNNING {
				active[record.Id] = record
			} else {
				delete(active, record.Id)
			}
			select {
			case <-q.ready:
				publishDue()
			default:
			}
		case <-timer.C:
			publishDue()
		}
	}
}

// dueAt is when an unfinished job needs a dispatch: its next attempt, or the
// end of an abandoned claim.
func dueAt(r *jobpb.JobRecord) time.Time {
	if r.Status == jobpb.JobStatus_JOB_STATUS_RUNNING {
		return r.LeaseUntil.AsTime()
	}
	if r.NextAttemptAt == nil {
		return time.Time{}
	}
	return r.NextAttemptAt.AsTime()
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// Work processes dispatches from consumer until cancellation. The application
// owns the consumer: it must filter to FilterSubjects of the handled types,
// use explicit acknowledgement, and allow unlimited deliveries. Run must run in
// the same deployment. A dispatch is acknowledged when an attempt ends,
// including a failed attempt, so jobs that wait for a retry hold no consumer
// capacity; Run publishes them again when they are due.
func (q *Queue) Work(ctx context.Context, consumer jetstream.Consumer, handlers map[string]Handler, concurrency int) error {
	worker, err := events.NewDurableWorker(consumer, func(ctx context.Context, d events.DurableDelivery) error {
		return q.process(ctx, d, handlers)
	}, events.DurableWorkerOptions{MaxConcurrent: concurrency, RetryDelay: 5 * time.Second, AckTimeout: 5 * time.Second, HeartbeatInterval: heartbeatInterval, Logger: q.logger})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

func (q *Queue) process(ctx context.Context, d events.DurableDelivery, handlers map[string]Handler) error {
	dispatch := new(jobpb.JobDispatch)
	if err := proto.Unmarshal(d.Data, dispatch); err != nil || !safeToken.MatchString(dispatch.Id) || dispatch.Generation == 0 {
		return events.TerminateDelivery("invalid job dispatch", errors.New("invalid job dispatch"))
	}
	record, revision, err := q.read(ctx, dispatch.Id)
	if err != nil {
		return err
	}
	if dispatch.Generation != record.Generation || record.Status == jobpb.JobStatus_JOB_STATUS_SUCCEEDED || record.Status == jobpb.JobStatus_JOB_STATUS_FAILED {
		return nil
	}
	handler := handlers[record.Type]
	if handler == nil {
		// A consumer that this process cannot serve, for example during a
		// rolling upgrade. Holding the dispatch would block other jobs; Run on a
		// capable process publishes it again.
		q.logger.Warn("Job type has no handler in this process", "job_type", record.Type)
		return nil
	}
	now := time.Now()
	if record.Status == jobpb.JobStatus_JOB_STATUS_RUNNING && now.Before(record.LeaseUntil.AsTime()) {
		// Another worker owns the attempt. Run publishes again if its lease ends.
		return nil
	}
	if record.Status == jobpb.JobStatus_JOB_STATUS_QUEUED && now.Before(record.NextAttemptAt.AsTime()) {
		return events.RetryDeliveryAfter(errors.New("job retry pending"), time.Until(record.NextAttemptAt.AsTime()))
	}
	if jobFromRecord(record).Expired(now) || record.Attempts >= record.MaxAttempts {
		if jobFromRecord(record).Expired(now) {
			record.Failure = "expired"
		} else if record.Failure == "" {
			record.Failure = "attempts_exhausted"
		}
		return q.fail(ctx, record, revision)
	}
	record.Status = jobpb.JobStatus_JOB_STATUS_RUNNING
	record.Attempts++
	record.Owner = fmt.Sprintf("%d", now.UnixNano())
	record.NextAttemptAt = nil
	record.LeaseUntil = timestamppb.New(now.Add(leaseDuration))
	if revision, err = q.save(ctx, record, revision); err != nil {
		return err
	}
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	// The handler gets a detached copy; the heartbeat loop keeps the record.
	attempt := jobFromRecord(record)
	go func() { result <- handler(attemptCtx, attempt) }()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			<-result
			q.release(ctx, record, revision)
			return ctx.Err()
		case <-ticker.C:
			record.LeaseUntil = timestamppb.New(time.Now().Add(leaseDuration))
			if revision, err = q.save(ctx, record, revision); err != nil {
				cancel()
				<-result
				return err
			}
		case err = <-result:
			if ctx.Err() != nil {
				q.release(ctx, record, revision)
				return ctx.Err()
			}
			return q.complete(ctx, record, revision, err)
		}
	}
}

// complete records one finished attempt. Only safe failure categories enter
// job state; handler error text never does.
func (q *Queue) complete(ctx context.Context, record *jobpb.JobRecord, revision uint64, err error) error {
	record.Owner = ""
	record.LeaseUntil = nil
	record.NextAttemptAt = nil
	if err == nil {
		record.Status = jobpb.JobStatus_JOB_STATUS_SUCCEEDED
		record.Failure = ""
		_, err = q.save(ctx, record, revision)
		return err
	}
	category, permanent := "attempt_failed", false
	if failure, ok := errors.AsType[*Failure](err); ok {
		if safeToken.MatchString(failure.Category) {
			category = failure.Category
		}
		permanent = failure.Permanent
	}
	record.Failure = category
	if permanent || record.Attempts >= record.MaxAttempts || jobFromRecord(record).Expired(time.Now()) {
		return q.fail(ctx, record, revision)
	}
	record.Status = jobpb.JobStatus_JOB_STATUS_QUEUED
	next := time.Now().Add(Backoff(time.Duration(record.RetryDelayMillis)*time.Millisecond, record.Attempts))
	if record.ExpiresAt != nil && next.After(record.ExpiresAt.AsTime()) {
		next = record.ExpiresAt.AsTime()
	}
	record.NextAttemptAt = timestamppb.New(next)
	// Acknowledge the dispatch; Run publishes the next attempt when it is due.
	_, err = q.save(ctx, record, revision)
	return err
}

func (q *Queue) fail(ctx context.Context, record *jobpb.JobRecord, revision uint64) error {
	record.Status = jobpb.JobStatus_JOB_STATUS_FAILED
	record.Owner = ""
	record.LeaseUntil = nil
	record.NextAttemptAt = nil
	if _, err := q.save(ctx, record, revision); err != nil {
		return err
	}
	if q.opts.OnFailed != nil {
		q.opts.OnFailed(ctx, jobFromRecord(record))
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

// release makes a graceful shutdown hand over promptly. After a crash, another
// consumer waits for the lease to expire. Revision fencing keeps release from
// replacing a newer owner.
func (q *Queue) release(ctx context.Context, record *jobpb.JobRecord, revision uint64) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	record.Status = jobpb.JobStatus_JOB_STATUS_QUEUED
	record.Owner = ""
	record.LeaseUntil = nil
	record.NextAttemptAt = timestamppb.Now()
	if _, err := q.save(releaseCtx, record, revision); err != nil {
		q.logger.Warn("Job shutdown handoff awaits lease expiry", "job_id", record.Id)
	}
}
