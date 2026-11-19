package bleve

import (
	"sync"
	"sync/atomic"
	"time"
)

// Index write stages reported by index diagnostics.
const (
	indexWriteStageIdle     = "idle"
	indexWriteStageLockWait = "lock_wait"
	indexWriteStagePrepare  = "prepare"
	indexWriteStageCommit   = "commit"
	indexWriteStageReset    = "checkpoint_reset"
)

// indexDiagnostics records index work without the projection lock, so that
// diagnostics answer while a commit or rebuild holds that lock. Its own mutex
// protects only these fields and is never held during index work. It records
// sequences, counts, and times, never message data or query terms.
type indexDiagnostics struct {
	mu               sync.Mutex
	writeStage       string
	writeStartedAt   time.Time
	writeBatchEvents int
	writeFirstSeq    uint64
	writeLastSeq     uint64
	lastCommitAt     time.Time
	checkpointSeq    uint64
	commits          uint64
	readersWaiting   atomic.Int64
	readersActive    atomic.Int64
	readersCompleted atomic.Uint64
}

// IndexDiagnosticsSnapshot is the JSON body of the index diagnostics endpoint.
type IndexDiagnosticsSnapshot struct {
	// WriteStage is idle, lock_wait, prepare, commit, or checkpoint_reset.
	WriteStage string `json:"write_stage"`
	// WriteStartedAt is the start of the current write stage sequence.
	WriteStartedAt *time.Time `json:"write_started_at,omitempty"`
	// WriteElapsedSeconds is the age of the current write, if one runs.
	WriteElapsedSeconds float64 `json:"write_elapsed_seconds,omitempty"`
	WriteBatchEvents    int     `json:"write_batch_events,omitempty"`
	WriteFirstSeq       uint64  `json:"write_first_seq,omitempty"`
	WriteLastSeq        uint64  `json:"write_last_seq,omitempty"`
	// LastCommitAt is the completion time of the last successful commit.
	LastCommitAt *time.Time `json:"last_commit_at,omitempty"`
	// CheckpointSeq is the EVT sequence of the last committed checkpoint.
	CheckpointSeq uint64 `json:"checkpoint_seq"`
	// Commits counts successful batch commits since process start.
	Commits uint64 `json:"commits"`
	// QueriesWaiting counts queries that wait for the projection lock.
	QueriesWaiting int64 `json:"queries_waiting"`
	// QueriesActive counts queries that hold the projection lock.
	QueriesActive    int64  `json:"queries_active"`
	QueriesCompleted uint64 `json:"queries_completed"`
}

func (d *indexDiagnostics) beginWrite(stage string, events int, first, last uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writeStage = stage
	d.writeStartedAt = time.Now()
	d.writeBatchEvents = events
	d.writeFirstSeq = first
	d.writeLastSeq = last
}

func (d *indexDiagnostics) setWriteStage(stage string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writeStage = stage
}

// endWrite clears the current write. A committed checkpoint sequence above
// zero also records a successful commit.
func (d *indexDiagnostics) endWrite(committedSeq uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writeStage = indexWriteStageIdle
	d.writeStartedAt = time.Time{}
	d.writeBatchEvents = 0
	d.writeFirstSeq = 0
	d.writeLastSeq = 0
	if committedSeq > 0 {
		d.lastCommitAt = time.Now()
		d.checkpointSeq = committedSeq
		d.commits++
	}
}

func (d *indexDiagnostics) setCheckpoint(seq uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.checkpointSeq = seq
}

// beginRead records a query that waits for the projection lock. Call the
// returned function after the lock is held, and the next one after release.
func (d *indexDiagnostics) beginRead() (locked func() (done func())) {
	d.readersWaiting.Add(1)
	return func() func() {
		d.readersWaiting.Add(-1)
		d.readersActive.Add(1)
		return func() {
			d.readersActive.Add(-1)
			d.readersCompleted.Add(1)
		}
	}
}

func (d *indexDiagnostics) Snapshot(now time.Time) IndexDiagnosticsSnapshot {
	d.mu.Lock()
	snapshot := IndexDiagnosticsSnapshot{
		WriteStage:       d.writeStage,
		WriteBatchEvents: d.writeBatchEvents,
		WriteFirstSeq:    d.writeFirstSeq,
		WriteLastSeq:     d.writeLastSeq,
		CheckpointSeq:    d.checkpointSeq,
		Commits:          d.commits,
	}
	if !d.writeStartedAt.IsZero() {
		started := d.writeStartedAt
		snapshot.WriteStartedAt = &started
		snapshot.WriteElapsedSeconds = now.Sub(started).Seconds()
	}
	if !d.lastCommitAt.IsZero() {
		last := d.lastCommitAt
		snapshot.LastCommitAt = &last
	}
	d.mu.Unlock()
	if snapshot.WriteStage == "" {
		snapshot.WriteStage = indexWriteStageIdle
	}
	snapshot.QueriesWaiting = d.readersWaiting.Load()
	snapshot.QueriesActive = d.readersActive.Load()
	snapshot.QueriesCompleted = d.readersCompleted.Load()
	return snapshot
}
