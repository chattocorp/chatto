package jobs

import (
	"time"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
)

// entry is the view state of one job.
type entry struct {
	// job has no payload. Its Revision is the sequence of the latest fact of
	// the job, also of a fact that changed nothing else.
	job Job
	// specSeq is the sequence of the latest fact that carries the
	// specification: queued, retried, or failed. Zero means that the view
	// never saw one.
	specSeq uint64
}

// view is the in-memory projection of the job stream: one payload-free summary
// per job. It is derived state and is rebuilt from the retained facts on each
// start.
type view struct {
	events.MemoryProjection
	subjects []string
	jobs     map[string]*entry
}

// Subjects implements events.SubjectProjection.
func (v *view) Subjects() []string { return v.subjects }

// Apply folds one fact into its job. Facts arrive in stream order.
func (v *view) Apply(fact *jobpb.JobFact, seq uint64) error {
	if fact.GetId() == "" || fact.GetFact() == nil {
		return nil
	}
	at := fact.GetOccurredAt().AsTime()
	v.Lock()
	defer v.Unlock()
	if fact.GetDeleted() != nil {
		delete(v.jobs, fact.Id)
		return nil
	}
	e := v.jobs[fact.Id]
	if e == nil {
		// Only a fact with the specification or a success describes a job by
		// itself. Other facts without their job are leftovers.
		if specOf(fact) == nil && fact.GetSucceeded() == nil {
			return nil
		}
		e = &entry{job: Job{ID: fact.Id, Type: fact.Type, CreatedAt: fact.GetCreatedAt().AsTime()}}
		v.jobs[fact.Id] = e
	}
	e.job.Revision = seq
	finished := e.job.Status == Succeeded || e.job.Status == Failed
	if spec := specOf(fact); spec != nil && (fact.GetFailed() == nil || e.job.Status != Succeeded) {
		e.specSeq = seq
		e.job.MaxAttempts = spec.GetMaxAttempts()
		e.job.RetryDelay = time.Duration(spec.GetRetryDelayMillis()) * time.Millisecond
		e.job.ExpiresAt = time.Time{}
		if spec.GetExpiresAt() != nil {
			e.job.ExpiresAt = spec.ExpiresAt.AsTime()
		}
	}
	switch f := fact.Fact.(type) {
	case *jobpb.JobFact_Queued, *jobpb.JobFact_Retried:
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Queued, 0, "", time.Time{}
	case *jobpb.JobFact_Succeeded:
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Succeeded, f.Succeeded.Attempts, "", time.Time{}
		e.job.MaxAttempts = f.Succeeded.MaxAttempts
	case *jobpb.JobFact_Failed:
		if e.job.Status == Succeeded {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Failed, f.Failed.Attempts, f.Failed.Failure, time.Time{}
	case *jobpb.JobFact_Started:
		// A late progress fact of a repeated delivery must not reopen a
		// finished job.
		if finished {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.NextAttemptAt = Running, f.Started.Attempt, time.Time{}
	case *jobpb.JobFact_AttemptFailed:
		if finished {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.Failure = Queued, f.AttemptFailed.Attempt, f.AttemptFailed.Failure
		e.job.NextAttemptAt = f.AttemptFailed.GetNextAttemptAt().AsTime()
	default:
		return nil
	}
	e.job.UpdatedAt = at
	return nil
}

func (v *view) get(id string) (entry, bool) {
	v.RLock()
	defer v.RUnlock()
	e := v.jobs[id]
	if e == nil {
		return entry{}, false
	}
	return *e, true
}

// list returns the jobs in status, in no order. Unknown keeps every job.
func (v *view) list(status Status) []Job {
	v.RLock()
	defer v.RUnlock()
	jobs := make([]Job, 0, len(v.jobs))
	for _, e := range v.jobs {
		if status == Unknown || e.job.Status == status {
			jobs = append(jobs, e.job)
		}
	}
	return jobs
}

// sweep drops the finished jobs whose fact the stream no longer retains: a
// succeeded job after succeededRetention, and a failed job after
// failedRetention. Zero failedRetention keeps failed jobs.
func (v *view) sweep(now time.Time, succeededRetention, failedRetention time.Duration) {
	v.Lock()
	defer v.Unlock()
	for id, e := range v.jobs {
		retention := time.Duration(0)
		switch e.job.Status {
		case Succeeded:
			retention = succeededRetention
		case Failed:
			retention = failedRetention
		}
		if retention > 0 && !now.Before(e.job.UpdatedAt.Add(retention)) {
			delete(v.jobs, id)
		}
	}
}
