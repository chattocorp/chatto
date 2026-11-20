package bleve

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

const (
	// queryProbeInterval is the time between probe starts. A blocked probe is
	// detected at the first interval after it exceeds queryProbeTimeout.
	queryProbeInterval = 10 * time.Second
	// queryProbeTimeout bounds one probe. A probe that runs longer makes the
	// provider unhealthy even when the read path does not honor cancellation.
	queryProbeTimeout = 5 * time.Second
	// queryProbeRoomID matches no room. The probe thus reads the real index
	// through the normal query path but cannot return message data.
	queryProbeRoomID = "chatto-search-query-probe"
)

// queryProbeRequest is the fixed synthetic query. It contains no user data.
func queryProbeRequest() *searchv1.QueryRequest {
	return &searchv1.QueryRequest{
		RoomIds:  []string{queryProbeRoomID},
		Order:    searchv1.SearchOrder_SEARCH_ORDER_NEWEST,
		PageSize: 1,
	}
}

// queryProbeSnapshot is a consistent copy of the probe state.
type queryProbeSnapshot struct {
	LastResult          searchv1.QueryProbeResult
	LastCompletedAt     time.Time
	LastSucceededAt     time.Time
	LastDuration        time.Duration
	ConsecutiveFailures uint32
	// InFlightSince is the start time of a probe that has not completed, or
	// zero when no probe runs.
	InFlightSince time.Time
	// Successes, Failures, and Timeouts count completed probes since start.
	Successes uint64
	Failures  uint64
	Timeouts  uint64
	// Timeout is the probe timeout.
	Timeout time.Duration
}

// Healthy reports whether the last completed probe succeeded and no running
// probe has exceeded its timeout.
func (s queryProbeSnapshot) Healthy(now time.Time) bool {
	return s.LastResult == searchv1.QueryProbeResult_QUERY_PROBE_RESULT_SUCCEEDED && !s.overdue(now)
}

// Failing reports whether the last completed probe failed or a running probe
// has exceeded its timeout. Before the first probe completes, a provider is
// neither healthy nor failing.
func (s queryProbeSnapshot) Failing(now time.Time) bool {
	switch s.LastResult {
	case searchv1.QueryProbeResult_QUERY_PROBE_RESULT_FAILED, searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT:
		return true
	}
	return s.overdue(now)
}

func (s queryProbeSnapshot) overdue(now time.Time) bool {
	return !s.InFlightSince.IsZero() && now.Sub(s.InFlightSince) > s.Timeout
}

func (s queryProbeSnapshot) proto() *searchv1.QueryProbeStatus {
	status := &searchv1.QueryProbeStatus{
		LastResult:          s.LastResult,
		ConsecutiveFailures: s.ConsecutiveFailures,
	}
	if !s.LastCompletedAt.IsZero() {
		status.LastCompletedAt = timestamppb.New(s.LastCompletedAt)
		status.LastDuration = durationpb.New(s.LastDuration)
	}
	if !s.LastSucceededAt.IsZero() {
		status.LastSucceededAt = timestamppb.New(s.LastSucceededAt)
	}
	if !s.InFlightSince.IsZero() {
		status.InFlightSince = timestamppb.New(s.InFlightSince)
	}
	return status
}

// queryProbe runs a synthetic query through the provider's index read path.
// At most one probe runs at a time: Start does not start a probe while the
// previous one is still running, even when that probe is blocked and ignores
// its context. A blocked index thus holds at most one probe goroutine.
type queryProbe struct {
	query   func(context.Context) error
	timeout time.Duration
	now     func() time.Time

	mu       sync.Mutex
	started  bool
	snapshot queryProbeSnapshot
}

func newQueryProbe(query func(context.Context) error) *queryProbe {
	return &queryProbe{query: query, timeout: queryProbeTimeout, now: time.Now}
}

// Start begins one probe unless a probe already runs. It returns a channel
// that closes when the started probe completes, or nil when it did not start
// a probe.
func (p *queryProbe) Start(ctx context.Context) <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.snapshot.InFlightSince.IsZero() {
		return nil
	}
	p.started = true
	started := p.now()
	p.snapshot.InFlightSince = started
	done := make(chan struct{})
	go func() {
		defer close(done)
		probeContext, cancel := context.WithTimeout(ctx, p.timeout)
		err := p.query(probeContext)
		cancel()
		p.complete(started, err)
	}()
	return done
}

func (p *queryProbe) complete(started time.Time, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	completed := p.now()
	s := &p.snapshot
	s.InFlightSince = time.Time{}
	s.LastCompletedAt = completed
	s.LastDuration = completed.Sub(started)
	switch {
	case err == nil && s.LastDuration <= p.timeout:
		s.LastResult = searchv1.QueryProbeResult_QUERY_PROBE_RESULT_SUCCEEDED
		s.LastSucceededAt = completed
		s.ConsecutiveFailures = 0
		s.Successes++
		return
	case err == nil, errors.Is(err, context.DeadlineExceeded):
		s.LastResult = searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT
		s.Timeouts++
	default:
		s.LastResult = searchv1.QueryProbeResult_QUERY_PROBE_RESULT_FAILED
		s.Failures++
	}
	s.ConsecutiveFailures++
}

// Snapshot returns the current probe state. The bool is false before the
// first probe starts.
func (p *queryProbe) Snapshot() (queryProbeSnapshot, bool) {
	if p == nil {
		return queryProbeSnapshot{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshot := p.snapshot
	snapshot.Timeout = p.timeout
	return snapshot, p.started
}
