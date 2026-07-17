package http_server

import (
	"sync"
	"time"
)

const (
	realtimeCatchUpMaxConcurrent        = 8
	realtimeCatchUpRateBurst            = 3
	realtimeCatchUpRateRefillInterval   = 20 * time.Second
	realtimeCatchUpLimiterStateLifetime = 24 * time.Hour
	realtimeCatchUpDefaultTimeout       = 30 * time.Second
)

type realtimeCatchUpAdmissionError struct {
	code       string
	retryAfter time.Duration
}

type realtimeCatchUpUserState struct {
	active     bool
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// realtimeCatchUpAdmission bounds expensive projection catch-up work per
// process. It is a capacity guard only: correctness and authorization never
// depend on this process-local state.
type realtimeCatchUpAdmission struct {
	mu             sync.Mutex
	global         chan struct{}
	users          map[string]*realtimeCatchUpUserState
	burst          int
	refillInterval time.Duration
	timeout        time.Duration
	now            func() time.Time
	acquisitions   uint64
}

func newRealtimeCatchUpAdmission() *realtimeCatchUpAdmission {
	return newRealtimeCatchUpAdmissionWithLimits(
		realtimeCatchUpMaxConcurrent,
		realtimeCatchUpRateBurst,
		realtimeCatchUpRateRefillInterval,
		time.Now,
	)
}

func newRealtimeCatchUpAdmissionWithLimits(maxConcurrent, burst int, refillInterval time.Duration, now func() time.Time) *realtimeCatchUpAdmission {
	return &realtimeCatchUpAdmission{
		global:         make(chan struct{}, maxConcurrent),
		users:          make(map[string]*realtimeCatchUpUserState),
		burst:          burst,
		refillInterval: refillInterval,
		timeout:        realtimeCatchUpDefaultTimeout,
		now:            now,
	}
}

// acquire admits at most one catch-up per authenticated user on this replica,
// consumes one per-user rate token, and reserves one global slot. The returned
// release function is idempotent.
func (a *realtimeCatchUpAdmission) acquire(userID string) (func(), *realtimeCatchUpAdmissionError) {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()

	a.acquisitions++
	if a.acquisitions%256 == 0 {
		a.removeStaleUsers(now)
	}

	state := a.users[userID]
	if state == nil {
		state = &realtimeCatchUpUserState{
			tokens:     float64(a.burst),
			lastRefill: now,
			lastSeen:   now,
		}
		a.users[userID] = state
	}
	if state.active {
		return nil, &realtimeCatchUpAdmissionError{code: "catch_up_in_progress", retryAfter: time.Second}
	}

	elapsed := now.Sub(state.lastRefill)
	if elapsed > 0 {
		state.tokens += float64(elapsed) / float64(a.refillInterval)
		if state.tokens > float64(a.burst) {
			state.tokens = float64(a.burst)
		}
		state.lastRefill = now
	}
	state.lastSeen = now
	if state.tokens < 1 {
		retryAfter := time.Duration((1 - state.tokens) * float64(a.refillInterval))
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return nil, &realtimeCatchUpAdmissionError{code: "catch_up_rate_limited", retryAfter: retryAfter}
	}

	select {
	case a.global <- struct{}{}:
	default:
		return nil, &realtimeCatchUpAdmissionError{code: "catch_up_server_busy", retryAfter: time.Second}
	}

	state.tokens--
	state.active = true
	var once sync.Once
	return func() {
		once.Do(func() {
			<-a.global
			a.mu.Lock()
			state.active = false
			state.lastSeen = a.now()
			a.mu.Unlock()
		})
	}, nil
}

func (a *realtimeCatchUpAdmission) removeStaleUsers(now time.Time) {
	for userID, state := range a.users {
		if !state.active && now.Sub(state.lastSeen) > realtimeCatchUpLimiterStateLifetime {
			delete(a.users, userID)
		}
	}
}
