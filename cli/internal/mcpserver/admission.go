package mcpserver

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/time/rate"
)

// Admission limits for the MCP endpoint. Each replica enforces them in its own
// memory; a deployment with N replicas admits up to N times these values.
// FDR-043 and the public MCP reference document this limit model.
const (
	// Requests without a bearer token only receive an authentication
	// challenge. Limit them per client network and in total, so that an
	// anonymous flood cannot reach the credential stages.
	anonymousNetworkRate  rate.Limit = 5
	anonymousNetworkBurst            = 20
	anonymousTotalRate    rate.Limit = 20
	anonymousTotalBurst              = 40

	// Each credentialed request that does not authenticate costs credential
	// reads. After this many recent failures, a client network cannot present
	// tokens until the allowance refills.
	failedAuthNetworkRate  = rate.Limit(1.0 / 5) // one failure every 5 seconds
	failedAuthNetworkBurst = 10

	// A credential that has not authenticated on this replica yet has its own
	// allowance before verification, so that one flooding caller cannot use
	// the total ceiling below.
	credentialRate  rate.Limit = 20
	credentialBurst            = 40

	// The total ceiling bounds verification work for credentials that have
	// not authenticated on this replica yet. Known credentials do not use it.
	credentialTotalRate  rate.Limit = 200
	credentialTotalBurst            = 400

	// Each authenticated account has its own allowance and a bound on the
	// requests that run at the same time, across all of its credentials.
	accountRate        rate.Limit = 20
	accountBurst                  = 40
	accountMaxInFlight            = 8

	// maxTrackedKeys bounds the memory of each keyed limiter and of the
	// known-credential index.
	maxTrackedKeys = 10000
)

// admission holds the MCP admission stages of one process. All configured
// MCP resources share one admission, so that a caller cannot multiply its
// allowance through host aliases.
type admission struct {
	anonymousNetwork  *keyedLimiter
	anonymousTotal    *rate.Limiter
	failedAuthNetwork *keyedLimiter
	credential        *keyedLimiter
	credentialTotal   *rate.Limiter
	account           *keyedLimiter
	known             *knownCredentials
	now               func() time.Time
}

func newAdmission() *admission {
	return &admission{
		anonymousNetwork:  newKeyedLimiter(anonymousNetworkRate, anonymousNetworkBurst, 0),
		anonymousTotal:    rate.NewLimiter(anonymousTotalRate, anonymousTotalBurst),
		failedAuthNetwork: newKeyedLimiter(failedAuthNetworkRate, failedAuthNetworkBurst, 0),
		credential:        newKeyedLimiter(credentialRate, credentialBurst, 0),
		credentialTotal:   rate.NewLimiter(credentialTotalRate, credentialTotalBurst),
		account:           newKeyedLimiter(accountRate, accountBurst, accountMaxInFlight),
		known:             &knownCredentials{accounts: make(map[string]string)},
		now:               time.Now,
	}
}

// admissionState carries the result of guard to limitAccount for one request.
type admissionState struct {
	credentialKey  string
	accountCharged bool
	authenticated  bool
	// backendError records that verification failed for a reason other than
	// an invalid credential, such as unavailable credential storage.
	backendError bool
}

type admissionStateKey struct{}

// wrap returns the MCP request chain in production order: cross-origin
// protection, the pre-authentication stages, the request deadline,
// authentication, and the per-account stage. Cross-origin rejections come
// first, so that they do not use admission allowances.
func (a *admission) wrap(authenticate func(http.Handler) http.Handler, next http.Handler) http.Handler {
	return http.NewCrossOriginProtection().Handler(a.guard(withRequestDeadline(authenticate(a.limitAccount(next)))))
}

// verifier marks backend verification errors in the admission state, so that
// guard does not count them as authentication failures. An unavailable
// credential store must not block valid callers after it recovers.
func (a *admission) verifier(inner auth.TokenVerifier) auth.TokenVerifier {
	return func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		info, err := inner(ctx, token, r)
		if err != nil && !errors.Is(err, auth.ErrInvalidToken) {
			if state, ok := r.Context().Value(admissionStateKey{}).(*admissionState); ok {
				state.backendError = true
			}
		}
		return info, err
	}
}

// guard applies the stages that run before authentication. Requests without a
// bearer token use the anonymous stage. Requests with one must pass the
// failure gate of their client network. Then a credential that authenticated
// before uses the allowance of its account; another credential uses its own
// allowance and the total credential ceiling. A request that a per-caller
// stage rejects does not use the shared ceilings. Each credentialed request
// that does not reach limitAccount counts as an authentication failure,
// except after a backend verification error.
func (a *admission) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := a.now()
		network := clientNetworkKey(r)
		token, ok := bearerToken(r)
		if !ok {
			if delay := a.anonymousNetwork.take(network, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
			if delay := take(a.anonymousTotal, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if delay := a.failedAuthNetwork.wait(network, now); delay > 0 {
			writeAdmissionFailure(w, delay)
			return
		}
		sum := sha256.Sum256([]byte(token))
		state := &admissionState{credentialKey: string(sum[:])}
		if accountID, ok := a.known.get(state.credentialKey); ok {
			// The index only selects the allowance. Verification still
			// decides authentication for this request.
			if delay := a.account.take(accountID, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
			state.accountCharged = true
		} else {
			if delay := a.credential.take(state.credentialKey, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
			if delay := take(a.credentialTotal, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), admissionStateKey{}, state)))
		if !state.authenticated && !state.backendError {
			a.known.remove(state.credentialKey)
			a.failedAuthNetwork.take(network, a.now())
		}
	})
}

// limitAccount applies the per-account stage after authentication. It must
// run inside auth.RequireBearerToken, which supplies the account ID, and
// inside guard. It records the credential as known for its account.
func (a *admission) limitAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := auth.TokenInfoFromContext(r.Context())
		state, _ := r.Context().Value(admissionStateKey{}).(*admissionState)
		if info == nil || info.UserID == "" || state == nil {
			http.Error(w, "missing authenticated account", http.StatusUnauthorized)
			return
		}
		state.authenticated = true
		a.known.put(state.credentialKey, info.UserID)
		now := a.now()
		if !state.accountCharged {
			if delay := a.account.take(info.UserID, now); delay > 0 {
				writeAdmissionFailure(w, delay)
				return
			}
		}
		release, delay := a.account.hold(info.UserID, now)
		if delay > 0 {
			writeAdmissionFailure(w, delay)
			return
		}
		defer release()
		next.ServeHTTP(w, r)
	})
}

// knownCredentials maps the hash of a credential that authenticated on this
// replica to its account. It selects the allowance of later requests with the
// same credential before verification. It does not authenticate: a failed
// request removes its entry. It keeps at most maxTrackedKeys entries and
// removes arbitrary entries when it is full; a removed credential uses the
// allowance for new credentials again.
type knownCredentials struct {
	mu       sync.Mutex
	accounts map[string]string
}

func (k *knownCredentials) get(key string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	accountID, ok := k.accounts[key]
	return accountID, ok
}

func (k *knownCredentials) put(key, accountID string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.accounts[key]; !ok && len(k.accounts) >= maxTrackedKeys {
		for other := range k.accounts {
			if len(k.accounts) <= maxTrackedKeys*3/4 {
				break
			}
			delete(k.accounts, other)
		}
	}
	k.accounts[key] = accountID
}

func (k *knownCredentials) remove(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.accounts, key)
}

// writeAdmissionFailure writes the HTTP 429 admission rejection. The response
// does not identify the stage, network, credential, or account.
func writeAdmissionFailure(w http.ResponseWriter, delay time.Duration) {
	seconds := max(1, int64(math.Ceil(delay.Seconds())))
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	f := failure("rate_limited", "MCP request rate limit exceeded. Wait before trying again.", "retry", "after_delay")
	f.RetryAfterMs = seconds * 1000
	writeHTTPFailure(w, http.StatusTooManyRequests, f)
}

// take consumes one token and returns zero, or returns the wait until a token
// is available without consuming one.
func take(limiter *rate.Limiter, now time.Time) time.Duration {
	reservation := limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return time.Second
	}
	delay := reservation.DelayFrom(now)
	if delay > 0 {
		reservation.CancelAt(now)
	}
	return delay
}

// keyedLimiter gives each key its own token bucket and, optionally, a bound on
// concurrent holders. It keeps at most maxTrackedKeys entries. When it is full,
// it removes idle entries first and then arbitrary entries that have no
// holders. When every entry has a holder, it rejects new keys. A removed entry starts again with a full bucket, which only grants
// a key more allowance; the total ceilings still apply.
type keyedLimiter struct {
	mu          sync.Mutex
	limit       rate.Limit
	burst       int
	maxInFlight int
	entries     map[string]*keyedEntry
}

type keyedEntry struct {
	limiter  *rate.Limiter
	inFlight int
}

func newKeyedLimiter(limit rate.Limit, burst, maxInFlight int) *keyedLimiter {
	return &keyedLimiter{limit: limit, burst: burst, maxInFlight: maxInFlight, entries: make(map[string]*keyedEntry)}
}

// take consumes one token of key. It returns zero, or the wait until a token
// is available.
func (k *keyedLimiter) take(key string, now time.Time) time.Duration {
	k.mu.Lock()
	defer k.mu.Unlock()
	entry := k.entry(key, now)
	if entry == nil {
		return time.Second
	}
	return take(entry.limiter, now)
}

// wait returns the wait until key has a token, without consuming one. A key
// without an entry has a full bucket.
func (k *keyedLimiter) wait(key string, now time.Time) time.Duration {
	k.mu.Lock()
	defer k.mu.Unlock()
	entry, ok := k.entries[key]
	if !ok {
		return 0
	}
	reservation := entry.limiter.ReserveN(now, 1)
	defer reservation.CancelAt(now)
	return reservation.DelayFrom(now)
}

// hold takes one concurrency slot of key. The caller must call release
// exactly once when the delay is zero.
func (k *keyedLimiter) hold(key string, now time.Time) (release func(), delay time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	entry := k.entry(key, now)
	if entry == nil || entry.inFlight >= k.maxInFlight {
		return nil, time.Second
	}
	entry.inFlight++
	var once sync.Once
	return func() {
		once.Do(func() {
			k.mu.Lock()
			defer k.mu.Unlock()
			entry.inFlight--
		})
	}, 0
}

// entry returns the entry of key and creates it when necessary. It returns
// nil when the limiter is full and no entry can be removed, because every
// entry has a holder; the caller then rejects the request. The caller holds
// k.mu.
func (k *keyedLimiter) entry(key string, now time.Time) *keyedEntry {
	if entry, ok := k.entries[key]; ok {
		return entry
	}
	if len(k.entries) >= maxTrackedKeys {
		k.evict(now)
		if len(k.entries) >= maxTrackedKeys {
			return nil
		}
	}
	entry := &keyedEntry{limiter: rate.NewLimiter(k.limit, k.burst)}
	k.entries[key] = entry
	return entry
}

// evict reduces the entries to three quarters of maxTrackedKeys, so that the
// scan cost is shared by many later insertions. Entries with holders stay.
func (k *keyedLimiter) evict(now time.Time) {
	target := maxTrackedKeys * 3 / 4
	for key, entry := range k.entries {
		if entry.inFlight == 0 && entry.limiter.TokensAt(now) >= float64(k.burst) {
			delete(k.entries, key)
		}
	}
	for key, entry := range k.entries {
		if len(k.entries) <= target {
			return
		}
		if entry.inFlight == 0 {
			delete(k.entries, key)
		}
	}
}

// bearerToken returns the token of a well-formed bearer Authorization header,
// with the same parsing as auth.RequireBearerToken. A request without one only
// receives an authentication challenge.
func bearerToken(r *http.Request) (string, bool) {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") {
		return "", false
	}
	return fields[1], true
}

type clientAddressContextKey struct{}

// WithClientAddress returns r with the client IP address that the HTTP server
// resolved through its trusted-proxy configuration. Admission limits key
// unauthenticated and rejected requests by this address. Without it, they use
// the remote address of the connection.
func WithClientAddress(r *http.Request, address string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), clientAddressContextKey{}, address))
}

// clientNetworkKey returns the admission key of the client network: the IPv4
// address, or the /64 prefix of an IPv6 address, because one IPv6 client
// usually controls a complete /64. The key stays in memory and is never logged.
func clientNetworkKey(r *http.Request) string {
	address, _ := r.Context().Value(clientAddressContextKey{}).(string)
	if address == "" {
		address = r.RemoteAddr
		if host, _, err := net.SplitHostPort(address); err == nil {
			address = host
		}
	}
	addr, err := netip.ParseAddr(strings.Trim(address, "[]"))
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().String()
	}
	return addr.String()
}
