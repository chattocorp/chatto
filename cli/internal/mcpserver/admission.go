package mcpserver

import (
	"crypto/sha256"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/time/rate"
)

// admissionLimits apply independently to each process. They protect work and
// memory, not domain correctness or credential validity. No requests are queued.
type admissionLimits struct {
	callerRate       rate.Limit
	callerBurst      int
	callerConcurrent int
	globalRate       rate.Limit
	globalBurst      int
	globalConcurrent int
	maxCallers       int
	idleLifetime     time.Duration
}

func defaultAdmissionLimits() admissionLimits {
	return admissionLimits{
		callerRate: 20, callerBurst: 40, callerConcurrent: 4,
		globalRate: 100, globalBurst: 200, globalConcurrent: 32,
		maxCallers: 8192, idleLifetime: time.Minute,
	}
}

// admissionController is shared by all MCP URL aliases in one handler. The
// first stage groups presented credentials; the second groups verified accounts
// across credential renewal and grants. Neither stage caches authentication.
type admissionController struct {
	verification *admissionStage
	account      *admissionStage
}

func newAdmissionController() *admissionController {
	return &admissionController{
		verification: newAdmissionStage(defaultAdmissionLimits()),
		account:      newAdmissionStage(defaultAdmissionLimits()),
	}
}

type admissionCaller struct {
	limiter  *rate.Limiter
	active   int
	lastSeen time.Time
}

// admissionStage holds only digests and bounded counters. Its lock covers
// admission decisions and cleanup; it never covers authentication or tool work.
// Slots stay occupied until the handler exits, also after request cancellation.
type admissionStage struct {
	mu          sync.Mutex
	limits      admissionLimits
	global      *rate.Limiter
	callers     map[[sha256.Size]byte]*admissionCaller
	active      int
	nextCleanup time.Time
	now         func() time.Time
}

func newAdmissionStage(limits admissionLimits) *admissionStage {
	return &admissionStage{
		limits: limits, global: rate.NewLimiter(limits.globalRate, limits.globalBurst),
		callers: make(map[[sha256.Size]byte]*admissionCaller), now: time.Now,
	}
}

// acquire consumes both rate budgets only on admission. It returns an
// idempotent release function, or a minimum retry delay when work cannot start.
func (s *admissionStage) acquire(key [sha256.Size]byte) (func(), time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !now.Before(s.nextCleanup) {
		for key, caller := range s.callers {
			if caller.active == 0 && now.Sub(caller.lastSeen) >= s.limits.idleLifetime {
				delete(s.callers, key)
			}
		}
		s.nextCleanup = now.Add(10 * time.Second)
	}
	caller := s.callers[key]
	if caller == nil {
		if len(s.callers) >= s.limits.maxCallers {
			return nil, time.Second
		}
		caller = &admissionCaller{limiter: rate.NewLimiter(s.limits.callerRate, s.limits.callerBurst)}
	}
	if caller.active >= s.limits.callerConcurrent || s.active >= s.limits.globalConcurrent {
		return nil, time.Second
	}
	local, global := caller.limiter.ReserveN(now, 1), s.global.ReserveN(now, 1)
	delay := max(local.DelayFrom(now), global.DelayFrom(now))
	if !local.OK() || !global.OK() || delay > 0 {
		local.CancelAt(now)
		global.CancelAt(now)
		if !local.OK() || !global.OK() {
			return nil, time.Second
		}
		return nil, delay
	}
	s.callers[key] = caller
	caller.active++
	caller.lastSeen = now
	s.active++
	return sync.OnceFunc(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		caller.active--
		caller.lastSeen = s.now()
		s.active--
	}), 0
}

// presentedCredentialKey uses the SDK's bearer parsing rules. Missing and
// malformed headers share one anonymous budget. Header spelling and whitespace
// cannot create new budgets for the same token. No IP or raw token is retained.
func presentedCredentialKey(r *http.Request) [sha256.Size]byte {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) == 2 && strings.EqualFold(fields[0], "bearer") {
		return sha256.Sum256([]byte(fields[1]))
	}
	return sha256.Sum256(nil)
}

// beforeAuthentication bounds even invalid-token verification and body work.
// These slots cover the complete request so expensive calls remain bounded even
// if authentication succeeds. A single credential cannot fill the global pool.
func (a *admissionController) beforeAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Err() != nil {
			return
		}
		release, delay := a.verification.acquire(presentedCredentialKey(r))
		if release == nil {
			writeAdmissionFailure(w, delay)
			return
		}
		defer release()
		next.ServeHTTP(w, r)
	})
}

// afterAuthentication must run inside RequireBearerToken. Only the verified
// account chooses this budget; client headers and MCP tool hints cannot do so.
// All requests count, including discovery and malformed or forbidden calls.
func (a *admissionController) afterAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Err() != nil {
			return
		}
		token := auth.TokenInfoFromContext(r.Context())
		if token == nil || token.UserID == "" {
			http.Error(w, "MCP authentication required", http.StatusUnauthorized)
			return
		}
		release, delay := a.account.acquire(sha256.Sum256([]byte(token.UserID)))
		if release == nil {
			writeAdmissionFailure(w, delay)
			return
		}
		defer release()
		next.ServeHTTP(w, r)
	})
}

// writeAdmissionFailure rounds up both retry representations to the same
// positive whole-second delay. Rejection happens before a tool can run.
func writeAdmissionFailure(w http.ResponseWriter, delay time.Duration) {
	seconds := max(1, int64(math.Ceil(delay.Seconds())))
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	f := failure("rate_limited", "MCP request capacity exceeded. Wait before trying again.", "retry", "after_delay")
	f.RetryAfterMs = seconds * 1000
	writeHTTPFailure(w, http.StatusTooManyRequests, f)
}
