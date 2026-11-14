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
	// specSeq is the sequence of the queued or retried fact that workers
	// consume. Zero means that the view never saw it.
	specSeq uint64
	// specAt is when that fact occurred. The stream removes it at specAt plus
	// its maximum age; the job then can neither run nor be retried.
	specAt time.Time
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
	e := v.jobs[fact.Id]
	if e == nil {
		// Only a specification or a success describes a job by itself. Other
		// facts without their job are leftovers of a job whose specification
		// passed the maximum age.
		if specOf(fact) == nil && fact.GetSucceeded() == nil {
			return nil
		}
		e = &entry{job: Job{ID: fact.Id, Type: fact.Type, CreatedAt: fact.GetCreatedAt().AsTime()}}
		v.jobs[fact.Id] = e
	}
	e.job.Revision = seq
	finished := e.job.Status == Succeeded || e.job.Status == Failed
	switch f := fact.Fact.(type) {
	case *jobpb.JobFact_Queued, *jobpb.JobFact_Retried:
		spec := specOf(fact)
		e.specSeq, e.specAt = seq, at
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Queued, 0, "", time.Time{}
		e.job.MaxAttempts = spec.GetMaxAttempts()
		e.job.RetryDelay = time.Duration(spec.GetRetryDelayMillis()) * time.Millisecond
		e.job.ExpiresAt = time.Time{}
		if spec.GetExpiresAt() != nil {
			e.job.ExpiresAt = spec.ExpiresAt.AsTime()
		}
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

// sweep drops the jobs that the stream no longer retains: a succeeded job
// after succeededRetention, any other job when its specification passes
// maxAge. Zero maxAge keeps those jobs. It returns the dropped jobs that had
// not finished.
func (v *view) sweep(now time.Time, maxAge, succeededRetention time.Duration) []Job {
	v.Lock()
	defer v.Unlock()
	var unfinished []Job
	for id, e := range v.jobs {
		switch {
		case e.job.Status == Succeeded:
			if now.Before(e.job.UpdatedAt.Add(succeededRetention)) {
				continue
			}
		case maxAge <= 0 || now.Before(e.specAt.Add(maxAge)):
			continue
		case e.job.Status != Failed:
			unfinished = append(unfinished, e.job)
		}
		delete(v.jobs, id)
	}
	return unfinished
}
