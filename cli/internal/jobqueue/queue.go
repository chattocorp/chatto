// Package jobqueue runs Chatto's durable background jobs. RUNTIME_STATE owns
// current job state; JOBS carries replayable dispatch references. A repair pass
// closes the record-to-dispatch crash gap, including manual retry acceptance.
package jobqueue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	jobv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/job/v1"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

const (
	ConsumerName      = "chatto-jobs-v1"
	MediaConsumerName = "chatto-jobs-media-v1"
	leaseDuration     = time.Minute
	heartbeatInterval = 15 * time.Second
	keyPrefix         = "job."
)

const (
	Queued    = statev1.JobExecutionStatus_JOB_EXECUTION_STATUS_QUEUED
	Running   = statev1.JobExecutionStatus_JOB_EXECUTION_STATUS_RUNNING
	Succeeded = statev1.JobExecutionStatus_JOB_EXECUTION_STATUS_SUCCEEDED
	Failed    = statev1.JobExecutionStatus_JOB_EXECUTION_STATUS_FAILED
)

var (
	ErrNotFound     = errors.New("job not found")
	ErrNotRetryable = errors.New("job cannot be retried")
	safeToken       = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

// Handler performs one attempt. It must honor cancellation and tolerate repeated
// execution. Only safe error categories enter job state; errors are not logged.
type Handler func(context.Context, *statev1.JobRecord) error

// Failure carries a non-sensitive category and whether automatic retry is futile.
// Categories must be fixed tokens, never external error text.
type Failure struct {
	Category  string
	Permanent bool
}

func (e *Failure) Error() string { return e.Category }

// Queue holds broker handles only. Each Run call owns a separate worker lifecycle.
type Queue struct {
	// OnFailed observes terminal failures for application diagnostics. Set before
	// Run; it must honor cancellation. Durable job state remains authoritative.
	OnFailed func(context.Context, *statev1.JobRecord)

	js     jetstream.JetStream
	kv     jetstream.KeyValue
	logger *log.Logger
}

// New binds already-provisioned resources. kv must use leader-routed Get.
func New(js jetstream.JetStream, kv jetstream.KeyValue, logger *log.Logger) *Queue {
	return &Queue{js: js, kv: kv, logger: logger}
}

// ID produces a stable opaque identity from a work type and source identity.
func ID(kind, source string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + source))
	return hex.EncodeToString(sum[:])
}

func jobKey(id string) string                       { return keyPrefix + id }
func dispatchSubject(job *statev1.JobRecord) string { return "jobs." + job.Type + "." + job.Id }

// Enqueue accepts a job once. Repeated source handoffs never replace its payload
// or reset attempts. A failed dispatch publish is recoverable from the record.
func (q *Queue) Enqueue(ctx context.Context, job *statev1.JobRecord) error {
	if !safeToken.MatchString(job.Id) || !safeToken.MatchString(job.Type) || job.MaxAttempts == 0 || job.RetryDelayMillis <= 0 {
		return errors.New("invalid job specification")
	}
	job = proto.Clone(job).(*statev1.JobRecord)
	now := timestamppb.Now()
	job.Status, job.Generation, job.Attempts = Queued, 1, 0
	job.CreatedAt, job.UpdatedAt, job.NextAttemptAt = now, now, now
	data, err := proto.Marshal(job)
	if err != nil {
		return err
	}
	_, err = q.kv.Create(ctx, jobKey(job.Id), data)
	if err != nil {
		if !jetstreamutil.IsSequenceConflict(err) {
			return err
		}
		// A create conflict can be a write in progress. Read through the leader;
		// absence is an error so the source remains unacknowledged and retries.
		job, _, err = q.read(ctx, job.Id)
		if err != nil {
			return err
		}
	}
	if job.Status != Queued {
		return nil
	}
	return q.dispatch(ctx, job)
}

func (q *Queue) dispatch(ctx context.Context, job *statev1.JobRecord) error {
	data, err := proto.Marshal(&jobv1.JobDispatch{Id: job.Id, Generation: job.Generation})
	if err != nil {
		return err
	}
	_, err = q.js.Publish(ctx, dispatchSubject(job), data, jetstream.WithMsgID(fmt.Sprintf("%s:%d", job.Id, job.Generation)))
	return err
}

func (q *Queue) read(ctx context.Context, id string) (*statev1.JobRecord, uint64, error) {
	if !safeToken.MatchString(id) {
		return nil, 0, ErrNotFound
	}
	entry, err := q.kv.Get(ctx, jobKey(id))
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	job := new(statev1.JobRecord)
	if err = proto.Unmarshal(entry.Value(), job); err != nil {
		return nil, 0, err
	}
	return job, entry.Revision(), nil
}

// Get returns detached current state, including the private work payload.
// Application boundaries must map it to a safe public summary.
func (q *Queue) Get(ctx context.Context, id string) (*statev1.JobRecord, error) {
	job, _, err := q.read(ctx, id)
	return job, err
}

func (q *Queue) save(ctx context.Context, job *statev1.JobRecord, revision uint64) (uint64, error) {
	job.UpdatedAt = timestamppb.Now()
	data, err := proto.Marshal(job)
	if err != nil {
		return 0, err
	}
	return q.kv.Update(ctx, jobKey(job.Id), data, revision)
}

// RetryVersion identifies an application attempt budget without exposing broker
// revisions or dispatch generations. A stale operator action cannot start a new
// budget after another operator's retry has already run and failed again.
func RetryVersion(job *statev1.JobRecord) string {
	return ID("retry-version", fmt.Sprintf("%s:%d", job.Id, job.Generation))
}

// Retry atomically accepts one new generation. Dispatch failure after acceptance
// is repaired in the background; callers can safely read the accepted state.
func (q *Queue) Retry(ctx context.Context, id, expectedVersion string) (*statev1.JobRecord, error) {
	job, rev, err := q.read(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status != Failed || Expired(job, time.Now()) || RetryVersion(job) != expectedVersion {
		return nil, ErrNotRetryable
	}
	job.Generation++
	job.Attempts = 0
	job.Status = Queued
	job.Failure = ""
	job.Owner = ""
	job.LeaseUntil = nil
	job.NextAttemptAt = timestamppb.Now()
	if _, err = q.save(ctx, job, rev); err != nil {
		if jetstreamutil.IsSequenceConflict(err) {
			return nil, ErrNotRetryable
		}
		return nil, err
	}
	if err = q.dispatch(ctx, job); err != nil {
		q.logger.Warn("Job retry dispatch awaits repair", "job_id", job.Id)
	}
	return job, nil
}

// Expired preserves the original deadline across automatic and manual retries.
func Expired(job *statev1.JobRecord, now time.Time) bool {
	return job.ExpiresAt != nil && !now.Before(job.ExpiresAt.AsTime())
}

// List returns a bounded page in stable ID order. The current v1 index lists
// only keys; it does not hydrate every job or create a process-wide job cache.
func (q *Queue) List(ctx context.Context, after string, limit int) ([]*statev1.JobRecord, bool, error) {
	keys, err := q.keys(ctx)
	if err != nil {
		return nil, false, err
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	out := make([]*statev1.JobRecord, 0, limit)
	for _, key := range keys {
		id := strings.TrimPrefix(key, keyPrefix)
		if id <= after {
			continue
		}
		if len(out) == limit {
			return out, true, nil
		}
		job, _, err := q.read(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		out = append(out, job)
	}
	return out, false, nil
}
func (q *Queue) keys(ctx context.Context) ([]string, error) {
	lister, err := q.kv.ListKeysFiltered(ctx, keyPrefix+">")
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer lister.Stop()
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return keys, nil
}

// ConsumerConfig is application-owned and versioned with the queue contract.
// A standalone media consumer overlaps the main consumer safely through record
// revision claims. Limits retention preserves dispatches for replay/recovery.
func ConsumerConfig(name string, types []string) jetstream.ConsumerConfig {
	filters := make([]string, 0, len(types))
	for _, kind := range types {
		filters = append(filters, "jobs."+kind+".*")
	}
	return jetstream.ConsumerConfig{Name: name, Durable: name, Description: "Durable background job dispatch", FilterSubjects: filters, AckPolicy: jetstream.AckExplicitPolicy, AckWait: 2 * time.Minute, MaxAckPending: 64, MaxDeliver: -1, DeliverPolicy: jetstream.DeliverAllPolicy}
}

// Run processes jobs and repairs unfinished handoffs until cancellation. Handler
// registration is fixed before startup. Workers never sleep through retry delays.
func (q *Queue) Run(ctx context.Context, consumer jetstream.Consumer, handlers map[string]Handler, concurrency int) error {
	worker, err := evtstream.NewEffectWorker(consumer, func(ctx context.Context, d events.DurableDelivery) error { return q.process(ctx, d, handlers) }, evtstream.EffectWorkerOptions{MaxConcurrent: concurrency, RetryDelay: 5 * time.Second, AckTimeout: 5 * time.Second, HeartbeatInterval: heartbeatInterval, Logger: q.logger})
	if err != nil {
		return err
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return worker.Run(ctx) })
	g.Go(func() error { return q.repair(ctx, handlers) })
	return g.Wait()
}

// repair keeps only unfinished work in memory. One filtered watcher closes
// enqueue/retry and backup gaps without scanning all terminal jobs every tick.
// The index is a hint: dispatch consumers still claim authoritative revisions.
func (q *Queue) repair(ctx context.Context, handlers map[string]Handler) error {
	watcher, err := q.kv.Watch(ctx, keyPrefix+">", jetstream.IgnoreDeletes())
	if err != nil {
		return err
	}
	defer watcher.Stop()
	active := map[string]*statev1.JobRecord{}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	publish := func(job *statev1.JobRecord) {
		if handlers[job.Type] == nil {
			return
		}
		if job.Status == Queued || (job.Status == Running && !time.Now().Before(job.LeaseUntil.AsTime())) {
			if err := q.dispatch(ctx, job); err != nil && ctx.Err() == nil {
				q.logger.Warn("Job dispatch awaits repair", "job_id", job.Id)
			}
		}
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
				return errors.New("job repair watcher closed")
			}
			if entry == nil {
				continue
			}
			job := new(statev1.JobRecord)
			if err := proto.Unmarshal(entry.Value(), job); err != nil {
				return errors.New("invalid job record")
			}
			if job.Status == Queued || job.Status == Running {
				active[job.Id] = job
				publish(job)
			} else {
				delete(active, job.Id)
			}
		case <-ticker.C:
			for _, job := range active {
				publish(job)
			}
		}
	}
}

func (q *Queue) process(ctx context.Context, d events.DurableDelivery, handlers map[string]Handler) error {
	dispatch := new(jobv1.JobDispatch)
	if err := proto.Unmarshal(d.Data, dispatch); err != nil || !safeToken.MatchString(dispatch.Id) || dispatch.Generation == 0 {
		return events.TerminateDelivery("invalid job dispatch", errors.New("invalid job dispatch"))
	}
	job, revision, err := q.read(ctx, dispatch.Id)
	if err != nil {
		return err
	}
	if dispatch.Generation != job.Generation || job.Status == Succeeded || job.Status == Failed {
		return nil
	}
	handler := handlers[job.Type]
	if handler == nil {
		return events.RetryDeliveryAfter(errors.New("job handler unavailable"), 30*time.Second)
	}
	now := time.Now()
	if job.Status == Running && now.Before(job.LeaseUntil.AsTime()) {
		return events.RetryDeliveryAfter(errors.New("job already claimed"), time.Until(job.LeaseUntil.AsTime()))
	}
	if job.Status == Queued && now.Before(job.NextAttemptAt.AsTime()) {
		return events.RetryDeliveryAfter(errors.New("job retry pending"), time.Until(job.NextAttemptAt.AsTime()))
	}
	if Expired(job, now) || job.Attempts >= job.MaxAttempts {
		job.Status = Failed
		job.Owner = ""
		job.LeaseUntil = nil
		job.NextAttemptAt = nil
		if Expired(job, now) {
			job.Failure = "expired"
		} else if job.Failure == "" {
			job.Failure = "attempts_exhausted"
		}
		_, err = q.save(ctx, job, revision)
		if err == nil && q.OnFailed != nil {
			q.OnFailed(ctx, proto.Clone(job).(*statev1.JobRecord))
		}
		return err
	}
	job.Status = Running
	job.Attempts++
	job.Owner = fmt.Sprintf("%d", now.UnixNano())
	job.NextAttemptAt = nil
	job.LeaseUntil = timestamppb.New(now.Add(leaseDuration))
	revision, err = q.save(ctx, job, revision)
	if err != nil {
		return err
	}
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	// Never share a mutable protobuf with the handler and the heartbeat loop.
	attempt := proto.Clone(job).(*statev1.JobRecord)
	go func() { result <- handler(attemptCtx, attempt) }()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			<-result
			q.release(ctx, job, revision)
			return ctx.Err()
		case <-ticker.C:
			job.LeaseUntil = timestamppb.New(time.Now().Add(leaseDuration))
			revision, err = q.save(ctx, job, revision)
			if err != nil {
				cancel()
				<-result
				return err
			}
		case err = <-result:
			if ctx.Err() != nil {
				q.release(ctx, job, revision)
				return ctx.Err()
			}
			job.Owner = ""
			job.LeaseUntil = nil
			job.NextAttemptAt = nil
			if err == nil {
				job.Status = Succeeded
				job.Failure = ""
			} else {
				category, permanent := "attempt_failed", false
				if failure, ok := errors.AsType[*Failure](err); ok {
					if safeToken.MatchString(failure.Category) {
						category = failure.Category
					}
					permanent = failure.Permanent
				}
				job.Failure = category
				if permanent || job.Attempts >= job.MaxAttempts || Expired(job, time.Now()) {
					job.Status = Failed
				} else {
					job.Status = Queued
					next := time.Now().Add(Backoff(time.Duration(job.RetryDelayMillis)*time.Millisecond, job.Attempts))
					if job.ExpiresAt != nil && next.After(job.ExpiresAt.AsTime()) {
						next = job.ExpiresAt.AsTime()
					}
					job.NextAttemptAt = timestamppb.New(next)
				}
			}
			if _, err = q.save(ctx, job, revision); err != nil {
				return err
			}
			if job.Status == Failed && q.OnFailed != nil {
				q.OnFailed(ctx, proto.Clone(job).(*statev1.JobRecord))
			}
			if job.Status == Queued {
				return events.RetryDeliveryAfter(errors.New("job attempt failed"), time.Until(job.NextAttemptAt.AsTime()))
			}
			return nil
		}
	}
}

// Backoff doubles the initial delay with a thirty-minute cap and no overflow.
func Backoff(initial time.Duration, attempt uint32) time.Duration {
	delay := min(max(initial, time.Millisecond), 30*time.Minute)
	for i := uint32(1); i < attempt && delay < 30*time.Minute; i++ {
		delay = min(delay*2, 30*time.Minute)
	}
	return delay
}

// release makes graceful shutdown handoff prompt. A hard crash instead waits for
// lease expiry. Revision fencing prevents release from replacing a newer owner.
func (q *Queue) release(ctx context.Context, job *statev1.JobRecord, revision uint64) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	job.Status = Queued
	job.Owner = ""
	job.LeaseUntil = nil
	job.NextAttemptAt = timestamppb.Now()
	if _, err := q.save(releaseCtx, job, revision); err != nil {
		q.logger.Warn("Job shutdown handoff awaits lease expiry", "job_id", job.Id)
	}
}
