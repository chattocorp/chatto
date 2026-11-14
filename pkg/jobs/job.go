package jobs

import (
	"context"
	"time"

	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
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
	// Succeeded jobs are complete. Their records expire after the retention.
	Succeeded
	// Failed jobs exhausted their budget, expired, or failed permanently.
	// They stay until a manual retry.
	Failed
)

// Spec describes a new job.
type Spec struct {
	// ID is a stable identity, usually from ID. Repeated handoffs with the same
	// ID accept the job once. It uses letters, digits, '_' and '-', up to 128.
	ID string
	// Type selects the handler and the dispatch subject. Same syntax as ID.
	Type string
	// Payload is an opaque application reference. It must not hold content,
	// credentials, or other data that a job listing must not reveal.
	Payload []byte
	// MaxAttempts is the automatic attempt budget. It must be positive.
	MaxAttempts uint32
	// RetryDelay is the first backoff delay, at least one millisecond.
	RetryDelay time.Duration
	// ExpiresAt is an optional deadline. Manual retries never extend it.
	ExpiresAt time.Time
}

// Job is a detached snapshot of a job record. Zero times mean "not set".
type Job struct {
	ID, Type    string
	Status      Status
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
	// Generation counts attempt budgets. A manual retry starts a new one.
	Generation uint64
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

func jobFromRecord(r *jobpb.JobRecord) Job {
	job := Job{
		ID: r.Id, Type: r.Type, Attempts: r.Attempts, MaxAttempts: r.MaxAttempts,
		Failure: r.Failure, Payload: append([]byte(nil), r.Payload...),
		RetryDelay: time.Duration(r.RetryDelayMillis) * time.Millisecond, Generation: r.Generation,
	}
	switch r.Status {
	case jobpb.JobStatus_JOB_STATUS_QUEUED:
		job.Status = Queued
	case jobpb.JobStatus_JOB_STATUS_RUNNING:
		job.Status = Running
	case jobpb.JobStatus_JOB_STATUS_SUCCEEDED:
		job.Status = Succeeded
	case jobpb.JobStatus_JOB_STATUS_FAILED:
		job.Status = Failed
	}
	if r.CreatedAt != nil {
		job.CreatedAt = r.CreatedAt.AsTime()
	}
	if r.UpdatedAt != nil {
		job.UpdatedAt = r.UpdatedAt.AsTime()
	}
	if r.NextAttemptAt != nil {
		job.NextAttemptAt = r.NextAttemptAt.AsTime()
	}
	if r.ExpiresAt != nil {
		job.ExpiresAt = r.ExpiresAt.AsTime()
	}
	return job
}
