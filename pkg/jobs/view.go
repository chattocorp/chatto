package jobs

import (
	"time"

	"github.com/chattocorp/chatto/pkg/events"
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
func (v *view) Apply(f *fact, seq uint64) error {
	if f.ID == "" || f.Kind == "" {
		return nil
	}
	v.Lock()
	defer v.Unlock()
	if f.Kind == factDeleted {
		delete(v.jobs, f.ID)
		return nil
	}
	spec := specOf(f)
	e := v.jobs[f.ID]
	if e == nil {
		// Only a fact with the specification or a success describes a job by
		// itself. Other facts without their job are leftovers.
		if spec == nil && f.Kind != factSucceeded {
			return nil
		}
		e = &entry{job: Job{ID: f.ID, Type: f.Type, CreatedAt: f.CreatedAt}}
		v.jobs[f.ID] = e
	}
	e.job.Revision = seq
	finished := e.job.Status == Succeeded || e.job.Status == Failed
	if spec != nil && (f.Kind != factFailed || e.job.Status != Succeeded) {
		e.specSeq = seq
		e.job.MaxAttempts = spec.MaxAttempts
		e.job.RetryDelay = time.Duration(spec.RetryDelayMillis) * time.Millisecond
		e.job.ExpiresAt = time.Time{}
		if spec.ExpiresAt != nil {
			e.job.ExpiresAt = *spec.ExpiresAt
		}
	}
	switch f.Kind {
	case factQueued, factRetried:
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Queued, 0, "", time.Time{}
	case factSucceeded:
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Succeeded, f.Attempts, "", time.Time{}
		e.job.MaxAttempts = f.MaxAttempts
	case factFailed:
		if e.job.Status == Succeeded {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Failed, f.Attempts, f.Failure, time.Time{}
	case factStarted:
		// A late progress fact of a repeated delivery must not reopen a
		// finished job.
		if finished {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.NextAttemptAt = Running, f.Attempts, time.Time{}
	case factAttemptFailed:
		if finished {
			return nil
		}
		e.job.Status, e.job.Attempts, e.job.Failure, e.job.NextAttemptAt = Queued, f.Attempts, f.Failure, time.Time{}
		if f.NextAttemptAt != nil {
			e.job.NextAttemptAt = *f.NextAttemptAt
		}
	default:
		// A kind that this version does not know.
		return nil
	}
	e.job.UpdatedAt = f.OccurredAt
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
