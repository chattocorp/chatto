package core

import (
	"sync"
	"sync/atomic"
	"time"
)

// authGenerationCheckWindow is the auth generation check window of ADR-127.
// After a strict auth generation check accepts a user, the read-only first
// pass of request validation for that user on this replica can use the fast
// check for this time. It is a security property, not a setting: it is the
// longest time for which this replica can accept a revoked credential.
const authGenerationCheckWindow = 60 * time.Second

// authGenerationWindowCapacity limits the number of users that one replica
// tracks. When the table is full after pruning, the replica does not record
// more users. Their validations then stay strict, which is always correct.
const authGenerationWindowCapacity = 100_000

// authGenerationWindowFullPruneInterval limits how often a full table is
// scanned. Between scans, a full table records no new users.
const authGenerationWindowFullPruneInterval = time.Second

// strictAuthGenerationReason tells why an auth generation check was strict.
// The values are metric labels. They contain no user data.
type strictAuthGenerationReason int

const (
	// strictReasonPath: the caller requires the strict check (ADR-127 section 3
	// and 5): issuance, credential changes, revocation, the authoritative
	// second pass, and the realtime timer.
	strictReasonPath strictAuthGenerationReason = iota
	// strictReasonWindow: no strict check accepted the user in the current
	// window on this replica.
	strictReasonWindow
	// strictReasonGate: a gate of ADR-127 section 4 is closed.
	strictReasonGate
	// strictReasonMismatch: the local projection did not accept the
	// credential, so only a strict check can decide (ADR-127 section 2).
	strictReasonMismatch
	strictReasonCount
)

// String returns the metric label of the reason.
func (r strictAuthGenerationReason) String() string {
	switch r {
	case strictReasonPath:
		return "path"
	case strictReasonWindow:
		return "window"
	case strictReasonGate:
		return "gate"
	case strictReasonMismatch:
		return "mismatch"
	}
	return "unknown"
}

// authGenerationWindowEntry is the state of one user.
type authGenerationWindowEntry struct {
	// acceptedFrom is the start time of the newest strict check that accepted
	// the user, or zero. The start, not the end, bounds the window: the strict
	// check read the stream leader after it started, so a revocation that it
	// did not see was written after acceptedFrom.
	acceptedFrom time.Time
	// reconnects is the NATS reconnect count when that strict check started.
	// A different count means that the connection was interrupted after the
	// check, so the record has no value.
	reconnects uint64
	// clearedAt is the time when a strict check last did not accept the user,
	// or zero. A strict check that started at or before clearedAt cannot
	// record an acceptance. Thus a slow strict check that started before a
	// revocation cannot make the record valid again after a rejection.
	clearedAt time.Time
}

// authGenerationWindow is the process-local record of strict auth generation
// checks that accepted a user (ADR-127 section 7). It is acceleration state
// only: it is never persisted or shared, a replica starts without records,
// and a record that is older than authGenerationCheckWindow has no value.
//
// The type only stores records. ChattoCore.requireAuthenticationAllowed
// decides between the fast and the strict check.
//
// Time comparisons use the monotonic clock reading that time.Now adds, so a
// wall clock step does not move a window. An elapsed time below zero counts
// as outside the window.
type authGenerationWindow struct {
	// now is the clock. Tests replace it before the first check.
	now func() time.Time

	mu      sync.Mutex
	entries map[string]authGenerationWindowEntry
	// notBefore is the time when a gate was last seen closed. Every record
	// from a strict check that started at or before it has no value.
	notBefore time.Time
	lastPrune time.Time

	fastChecks   atomic.Uint64
	strictChecks [strictReasonCount]atomic.Uint64
	capacity     int
	window       time.Duration
}

func newAuthGenerationWindow() *authGenerationWindow {
	return &authGenerationWindow{
		now:      time.Now,
		entries:  make(map[string]authGenerationWindowEntry),
		capacity: authGenerationWindowCapacity,
		window:   authGenerationCheckWindow,
	}
}

// inWindow reports whether from is less than the window before now. The
// window boundary itself is outside.
func (w *authGenerationWindow) inWindow(from, now time.Time) bool {
	if from.IsZero() {
		return false
	}
	elapsed := now.Sub(from)
	return elapsed >= 0 && elapsed < w.window
}

// allows reports whether a strict check that started less than the window
// ago accepted the user, and nothing cleared that record since then.
// reconnects is the current NATS reconnect count.
func (w *authGenerationWindow) allows(userID string, reconnects uint64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	entry, ok := w.entries[userID]
	if !ok {
		return false
	}
	return entry.reconnects == reconnects &&
		entry.acceptedFrom.After(entry.clearedAt) &&
		entry.acceptedFrom.After(w.notBefore) &&
		w.inWindow(entry.acceptedFrom, w.now())
}

// accepted records that a strict check that started at startedAt, with the
// NATS reconnect count reconnects, accepted the user. It ignores a check that
// started outside the window, at or before a clear of the user or a closed
// gate, or before the check that the record already holds.
func (w *authGenerationWindow) accepted(userID string, startedAt time.Time, reconnects uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if !w.inWindow(startedAt, now) || !startedAt.After(w.notBefore) {
		return
	}
	entry, ok := w.entries[userID]
	if !startedAt.After(entry.clearedAt) || (ok && !startedAt.After(entry.acceptedFrom)) {
		return
	}
	if !ok && !w.makeRoom(now) {
		return
	}
	entry.acceptedFrom = startedAt
	entry.reconnects = reconnects
	w.entries[userID] = entry
}

// clear removes the acceptance of the user. Call it when a strict check did
// not accept the user, for any reason. When the table is full and has no
// entry for the user, nothing is stored: then the user has no acceptance, and
// a strict check that ends later can record only its own, true acceptance.
func (w *authGenerationWindow) clear(userID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if _, ok := w.entries[userID]; !ok && !w.makeRoom(now) {
		return
	}
	w.entries[userID] = authGenerationWindowEntry{clearedAt: now}
}

// closeAll ends the window of every user. Call it when a gate is closed.
func (w *authGenerationWindow) closeAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.notBefore = w.now()
	clear(w.entries)
}

// makeRoom prunes entries when one window passed since the last prune, or
// when the table is full and authGenerationWindowFullPruneInterval passed
// since the last prune. It reports whether a new entry fits. The rate limit
// keeps a full table from a scan on each request, because the scan holds mu.
// The caller holds mu.
func (w *authGenerationWindow) makeRoom(now time.Time) bool {
	sinceLastPrune := now.Sub(w.lastPrune)
	full := len(w.entries) >= w.capacity
	if !w.inWindow(w.lastPrune, now) || (full && sinceLastPrune >= authGenerationWindowFullPruneInterval) {
		w.prune(now)
	}
	return len(w.entries) < w.capacity
}

// prune removes entries that have no value: an acceptance outside the window
// cannot allow the fast check, and a clear outside the window cannot block an
// acceptance, because accepted ignores checks that started outside the
// window. The caller holds mu.
func (w *authGenerationWindow) prune(now time.Time) {
	w.lastPrune = now
	for userID, entry := range w.entries {
		if !w.inWindow(entry.acceptedFrom, now) && !w.inWindow(entry.clearedAt, now) {
			delete(w.entries, userID)
		}
	}
}

// size returns the number of entries.
func (w *authGenerationWindow) size() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}

// AuthGenerationCheckMetrics counts auth generation checks in this process.
// The counters contain no user data.
type AuthGenerationCheckMetrics struct {
	// Fast is the number of checks that the fast check accepted.
	Fast uint64
	// Strict is the number of strict checks, by reason label: "path",
	// "window", "gate", and "mismatch".
	Strict map[string]uint64
}

func (w *authGenerationWindow) metrics() AuthGenerationCheckMetrics {
	m := AuthGenerationCheckMetrics{Fast: w.fastChecks.Load(), Strict: make(map[string]uint64, strictReasonCount)}
	for reason := range strictReasonCount {
		m.Strict[reason.String()] = w.strictChecks[reason].Load()
	}
	return m
}
