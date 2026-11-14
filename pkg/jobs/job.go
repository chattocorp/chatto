package jobs

import (
	"context"
	"math"
	"time"
)

// Status is the execution state of a job.
type Status int

const (
	// Unknown is a state that this version does not recognize.
	Unknown Status = iota
	// Queued jobs wait for their next attempt.
	Queued
	// Running jobs have a claimed attempt with a renewed lease.
	Running
	// Succeeded jobs are complete. They stay for the succeeded retention.
	Succeeded
	// Failed jobs exhausted their budget, expired, or failed permanently.
	// They stay until a manual retry or the maximum age of the job stream.
	Failed
)

// UnlimitedAttempts is an attempt budget without a practical limit. A job with
// it retries with capped backoff until it succeeds, fails permanently, or
// expires. Use it for work that must finish, such as erasure, when no operator
// is expected to retry it.
const UnlimitedAttempts uint32 = math.MaxUint32

// Spec describes a new job.
type Spec struct {
	// ID is a stable identity, usually from ID. Repeated handoffs with the same
	// ID accept the job once. It must be unique across job types. It uses
	// letters, digits, '_' and '-', up to 128.
	ID string
	// Type selects the handler and the dispatch subject. Same syntax as ID.
	Type string
	// Payload is an opaque application reference. It must not hold content,
	// credentials, or other data that a job listing must not reveal.
	Payload []byte
	// MaxAttempts is the automatic attempt budget. It must be positive.
	// UnlimitedAttempts retries until success, permanent failure, or expiry.
	MaxAttempts uint32
	// RetryDelay is the first backoff delay, at least one millisecond.
	RetryDelay time.Duration
	// ExpiresAt is an optional deadline. Manual retries never extend it.
	ExpiresAt time.Time
}

// Job is a detached snapshot of a job. Zero times mean "not set". Handlers get
// the job with its payload; jobs from Queue.Get and Queue.List have none.
type Job struct {
	ID, Type string
	Status   Status
	// Attempts is the number of the current or last attempt. It counts
	// deliveries, so an attempt that a shutdown interrupted also counts.
	Attempts    uint32
	MaxAttempts uint32
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// NextAttemptAt is set while a queued job waits for its next attempt.
	NextAttemptAt time.Time
	// Failure is the safe category of the last failed attempt.
	Failure    string
	Payload    []byte
	RetryDelay time.Duration
	ExpiresAt  time.Time
	// Revision is the stream sequence of the latest fact of the job. It is a
	// broker position: applications must not expose it. RetryVersion derives a
	// safe token from it.
	Revision uint64
}

// Expired reports whether the job's deadline has passed.
func (j Job) Expired(now time.Time) bool {
	return !j.ExpiresAt.IsZero() && !now.Before(j.ExpiresAt)
}

// Handler performs one attempt. It must honor cancellation and tolerate
// repeated execution. Return a *Failure to choose a safe category or to stop
// automatic retries. The queue never stores or logs other error text.
type Handler func(context.Context, Job) error

// Failure is a handler error with a non-sensitive category. Categories must be
// fixed tokens, never external error text. Permanent stops automatic retries.
type Failure struct {
	Category  string
	Permanent bool
}

func (e *Failure) Error() string { return e.Category }
