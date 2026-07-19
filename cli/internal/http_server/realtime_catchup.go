package http_server

import (
	"sync"
	"time"
)

const (
	realtimeCatchUpMaxConcurrent         = 8
	realtimeCatchUpRateBurst             = 3
	realtimeCatchUpRateRefillInterval    = 20 * time.Second
	realtimeCatchUpGeneralRateBurst      = 20
	realtimeCatchUpGeneralRefillInterval = time.Second
	realtimeCatchUpLimiterStateLifetime  = 24 * time.Hour
	realtimeCatchUpDefaultTimeout        = 30 * time.Second
	realtimeHydrationRateBurst           = 20
	realtimeHydrationRefillInterval      = time.Second
)

type realtimeCatchUpAdmissionError struct {
	code       string
	retryAfter time.Duration
}

type realtimeCatchUpUserState struct {
	active            bool
	tokens            float64
	lastRefill        time.Time
	generalTokens     float64
	generalLastRefill time.Time
	lastSeen          time.Time
	hydrationActive   bool
	hydrationTokens   float64
	hydrationRefillAt time.Time
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

// acquire admits at most one catch-up per authenticated user on this replica
// and reserves one global slot. Metered stale-cursor replay attempts consume a
// per-user replay token. Cursorless compacted bootstraps cannot request history,
// and a cursor already at the current EVT boundary cannot request replay work,
// so both use a separate, more permissive general catch-up token bucket. The
// returned release function is idempotent.
func (a *realtimeCatchUpAdmission) acquire(userID string, metered bool) (func(), *realtimeCatchUpAdmissionError) {
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
			tokens:            float64(a.burst),
			lastRefill:        now,
			generalTokens:     realtimeCatchUpGeneralRateBurst,
			generalLastRefill: now,
			lastSeen:          now,
			hydrationTokens:   realtimeHydrationRateBurst,
			hydrationRefillAt: now,
		}
		a.users[userID] = state
	}
	if state.active || state.hydrationActive {
		return nil, &realtimeCatchUpAdmissionError{code: "catch_up_in_progress", retryAfter: time.Second}
	}

	a.refill(state, now)
	a.refillGeneral(state, now)
	state.lastSeen = now
	availableTokens := state.generalTokens
	retryInterval := realtimeCatchUpGeneralRefillInterval
	if metered {
		availableTokens = state.tokens
		retryInterval = a.refillInterval
	}
	if availableTokens < 1 {
		retryAfter := time.Duration((1 - availableTokens) * float64(retryInterval))
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

	if metered {
		state.tokens--
	} else {
		state.generalTokens--
	}
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

// acquireHydration bounds post-bootstrap room hydration across every socket
// owned by one user and shares the process-wide expensive-work semaphore with
// replay/reset catch-up. The limit is capacity protection only; authorization
// remains authoritative in the room projection assembler.
func (a *realtimeCatchUpAdmission) acquireHydration(userID string) (func(), *realtimeCatchUpAdmissionError) {
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
			tokens: float64(a.burst), lastRefill: now,
			generalTokens: realtimeCatchUpGeneralRateBurst, generalLastRefill: now,
			hydrationTokens: realtimeHydrationRateBurst, hydrationRefillAt: now, lastSeen: now,
		}
		a.users[userID] = state
	}
	if state.active || state.hydrationActive {
		return nil, &realtimeCatchUpAdmissionError{code: "room_hydration_in_progress", retryAfter: time.Second}
	}
	elapsed := now.Sub(state.hydrationRefillAt)
	if elapsed > 0 {
		state.hydrationTokens += float64(elapsed) / float64(realtimeHydrationRefillInterval)
		if state.hydrationTokens > realtimeHydrationRateBurst {
			state.hydrationTokens = realtimeHydrationRateBurst
		}
		state.hydrationRefillAt = now
	}
	state.lastSeen = now
	if state.hydrationTokens < 1 {
		retryAfter := time.Duration((1 - state.hydrationTokens) * float64(realtimeHydrationRefillInterval))
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return nil, &realtimeCatchUpAdmissionError{code: "room_hydration_rate_limited", retryAfter: retryAfter}
	}
	select {
	case a.global <- struct{}{}:
	default:
		return nil, &realtimeCatchUpAdmissionError{code: "room_hydration_server_busy", retryAfter: time.Second}
	}
	state.hydrationTokens--
	state.hydrationActive = true
	var once sync.Once
	return func() {
		once.Do(func() {
			<-a.global
			a.mu.Lock()
			state.hydrationActive = false
			state.lastSeen = a.now()
			a.mu.Unlock()
		})
	}, nil
}

// consumeReplayToken charges an already-active unmetered admission when EVT
// advanced after current-boundary classification but before replay planning.
// The caller must reject the catch-up before delivery when this returns an
// error, so every actual cursor gap remains rate-bounded despite that race.
func (a *realtimeCatchUpAdmission) consumeReplayToken(userID string) *realtimeCatchUpAdmissionError {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()

	state := a.users[userID]
	if state == nil || !state.active {
		return &realtimeCatchUpAdmissionError{code: "catch_up_in_progress", retryAfter: time.Second}
	}
	a.refill(state, now)
	state.lastSeen = now
	if state.tokens < 1 {
		retryAfter := time.Duration((1 - state.tokens) * float64(a.refillInterval))
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return &realtimeCatchUpAdmissionError{code: "catch_up_rate_limited", retryAfter: retryAfter}
	}
	state.tokens--
	return nil
}

func (a *realtimeCatchUpAdmission) refill(state *realtimeCatchUpUserState, now time.Time) {
	elapsed := now.Sub(state.lastRefill)
	if elapsed <= 0 {
		return
	}
	state.tokens += float64(elapsed) / float64(a.refillInterval)
	if state.tokens > float64(a.burst) {
		state.tokens = float64(a.burst)
	}
	state.lastRefill = now
}

func (a *realtimeCatchUpAdmission) refillGeneral(state *realtimeCatchUpUserState, now time.Time) {
	elapsed := now.Sub(state.generalLastRefill)
	if elapsed <= 0 {
		return
	}
	state.generalTokens += float64(elapsed) / float64(realtimeCatchUpGeneralRefillInterval)
	if state.generalTokens > realtimeCatchUpGeneralRateBurst {
		state.generalTokens = realtimeCatchUpGeneralRateBurst
	}
	state.generalLastRefill = now
}

func (a *realtimeCatchUpAdmission) removeStaleUsers(now time.Time) {
	for userID, state := range a.users {
		if !state.active && !state.hydrationActive && now.Sub(state.lastSeen) > realtimeCatchUpLimiterStateLifetime {
			delete(a.users, userID)
		}
	}
}
