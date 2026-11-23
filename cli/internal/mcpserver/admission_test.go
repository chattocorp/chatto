package mcpserver

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// admissionFixture mirrors the production order of the admission stages
// around a stub verifier and a manual clock.
type admissionFixture struct {
	admission *admission
	handler   http.Handler
	now       time.Time
	verified  int
	// storageDown makes the stub verifier fail with a backend error.
	storageDown bool
	// verifyGate, when set, makes the stub verifier wait for it or for the
	// request to end for tokens that start with "fake-", as a slow credential
	// read does.
	verifyGate chan struct{}
	verifying  chan struct{}
}

func newAdmissionFixture(t *testing.T, accounts map[string]string, next http.Handler) *admissionFixture {
	t.Helper()
	f := &admissionFixture{admission: newAdmission(), now: time.Unix(1_700_000_000, 0)}
	f.admission.now = func() time.Time { return f.now }
	verifier := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		f.verified++
		if f.verifyGate != nil && strings.HasPrefix(token, "fake-") {
			f.verifying <- struct{}{}
			select {
			case <-f.verifyGate:
			case <-ctx.Done():
				return nil, fmt.Errorf("read credential: %w", ctx.Err())
			}
		}
		if f.storageDown {
			return nil, errors.New("credential storage unavailable")
		}
		userID, ok := accounts[token]
		if !ok {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: userID, Expiration: time.Now().Add(time.Hour)}, nil
	}
	if next == nil {
		next = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	}
	f.handler = f.admission.wrap(auth.RequireBearerToken(f.admission.verifier(verifier), nil), next)
	return f
}

func (f *admissionFixture) serve(address, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.RemoteAddr = address + ":40000"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

// countStatus sends n requests and counts the responses with status.
func (f *admissionFixture) countStatus(n int, address, token string, status int) int {
	count := 0
	for range n {
		if f.serve(address, token).Code == status {
			count++
		}
	}
	return count
}

func TestMCPAdmissionIsolatesCallers(t *testing.T) {
	tests := []struct {
		name       string
		floodToken string
	}{
		{name: "anonymous flood", floodToken: ""},
		{name: "rejected-token flood", floodToken: "invalid"},
		{name: "flood from another bot", floodToken: "bot-a-key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newAdmissionFixture(t, map[string]string{"bot-a-key": "bot-a", "bot-b-key": "bot-b"}, nil)
			// The flood comes from a different network than the victim.
			// Rejected tokens block only their own network (see below).
			f.countStatus(5000, "198.51.100.7", test.floodToken, http.StatusOK)
			if ok := f.countStatus(26, "192.0.2.10", "bot-b-key", http.StatusOK); ok != 26 {
				t.Fatalf("victim succeeded %d of 26 calls during the flood", ok)
			}
		})
	}
}

func TestMCPAdmissionCrossOriginFloodUsesNoAllowance(t *testing.T) {
	f := newAdmissionFixture(t, map[string]string{"victim-key": "victim"}, nil)
	for i := range 5000 {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request.RemoteAddr = "198.51.100.7:40000"
		request.Header.Set("Authorization", fmt.Sprintf("Bearer fake-%d", i))
		request.Header.Set("Sec-Fetch-Site", "cross-site")
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("cross-origin request %d = %d, want 403", i, response.Code)
		}
	}
	if ok := f.countStatus(26, "192.0.2.10", "victim-key", http.StatusOK); ok != 26 {
		t.Fatalf("victim succeeded %d of 26 calls during the flood", ok)
	}
}

func TestMCPAdmissionMultiCredentialAccountDoesNotUseSharedCeiling(t *testing.T) {
	accounts := map[string]string{"victim-key": "victim"}
	tokens := make([]string, 20)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("flood-key-%d", i)
		accounts[tokens[i]] = "flooder"
	}
	f := newAdmissionFixture(t, accounts, nil)
	for range 50 {
		for i, token := range tokens {
			f.serve(fmt.Sprintf("198.51.100.%d", i), token)
		}
	}
	// The victim credential is new, so it needs the shared ceiling.
	if ok := f.countStatus(26, "192.0.2.10", "victim-key", http.StatusOK); ok != 26 {
		t.Fatalf("victim succeeded %d of 26 calls during the flood", ok)
	}
	if remaining := f.admission.credentialTotal.TokensAt(f.now); remaining < float64(credentialTotalBurst-len(tokens)-1) {
		t.Fatalf("shared ceiling has %.0f tokens; known credentials used it", remaining)
	}
}

func TestMCPAdmissionForgetsFailedKnownCredential(t *testing.T) {
	accounts := map[string]string{"token": "user"}
	f := newAdmissionFixture(t, accounts, nil)
	if code := f.serve("192.0.2.10", "token").Code; code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", code)
	}
	delete(accounts, "token")
	if code := f.serve("192.0.2.10", "token").Code; code != http.StatusUnauthorized {
		t.Fatalf("revoked request = %d, want 401", code)
	}
	sum := sha256.Sum256([]byte("token"))
	if _, ok := f.admission.known.get(string(sum[:])); ok {
		t.Fatal("a failed credential stayed known")
	}
	if delay := f.admission.failedAuthNetwork.wait("192.0.2.10", f.now); delay != 0 {
		t.Fatalf("one failure closed the gate: %v", delay)
	}
	if tokens := f.admission.failedAuthNetwork.entries["192.0.2.10"].limiter.TokensAt(f.now); tokens != failedAuthNetworkBurst-1 {
		t.Fatalf("failure allowance = %.0f, want %d", tokens, failedAuthNetworkBurst-1)
	}
}

func TestMCPAdmissionSameNetworkFloodsDoNotBlockOtherCredentials(t *testing.T) {
	for _, floodToken := range []string{"", "bot-a-key"} {
		f := newAdmissionFixture(t, map[string]string{"bot-a-key": "bot-a", "bot-b-key": "bot-b"}, nil)
		f.countStatus(5000, "192.0.2.10", floodToken, http.StatusOK)
		if ok := f.countStatus(25, "192.0.2.10", "bot-b-key", http.StatusOK); ok != 25 {
			t.Fatalf("flood token %q: victim succeeded %d of 25 calls", floodToken, ok)
		}
	}
}

func TestMCPAdmissionBoundsRejectedTokensPerNetwork(t *testing.T) {
	f := newAdmissionFixture(t, map[string]string{"valid": "bot"}, nil)
	for i := range failedAuthNetworkBurst {
		if code := f.serve("198.51.100.7", fmt.Sprintf("guess-%d", i)).Code; code != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d, want 401", i, code)
		}
	}
	verified := f.verified
	response := f.serve("198.51.100.7", "another-guess")
	assertAdmissionFailure(t, response, 5)
	if f.verified != verified {
		t.Fatal("a blocked network reached token verification")
	}
	if code := f.serve("192.0.2.10", "valid").Code; code != http.StatusOK {
		t.Fatalf("other network = %d, want 200", code)
	}
	f.now = f.now.Add(5 * time.Second)
	if code := f.serve("198.51.100.7", "guess-again").Code; code != http.StatusUnauthorized {
		t.Fatalf("after refill = %d, want 401", code)
	}
	if code := f.serve("198.51.100.7", "guess-later").Code; code != http.StatusTooManyRequests {
		t.Fatalf("after one more rejection = %d, want 429", code)
	}
}

func TestMCPAdmissionIgnoresBackendVerificationErrors(t *testing.T) {
	f := newAdmissionFixture(t, map[string]string{"token": "user"}, nil)
	if code := f.serve("192.0.2.10", "token").Code; code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", code)
	}
	f.storageDown = true
	if failures := f.countStatus(2*failedAuthNetworkBurst, "192.0.2.10", "token", http.StatusInternalServerError); failures != 2*failedAuthNetworkBurst {
		t.Fatalf("backend errors = %d, want %d", failures, 2*failedAuthNetworkBurst)
	}
	sum := sha256.Sum256([]byte("token"))
	if _, ok := f.admission.known.get(string(sum[:])); !ok {
		t.Fatal("a backend error removed the known credential")
	}
	f.storageDown = false
	f.now = f.now.Add(time.Second)
	if code := f.serve("192.0.2.10", "token").Code; code != http.StatusOK {
		t.Fatalf("after recovery = %d, want 200", code)
	}
}

func TestMCPAdmissionBoundsConcurrentNewCredentialsPerNetwork(t *testing.T) {
	f := newAdmissionFixture(t, map[string]string{"victim-key": "victim"}, nil)
	f.verifyGate = make(chan struct{})
	f.verifying = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, failedAuthNetworkMaxInFlight)
	for i := range failedAuthNetworkMaxInFlight {
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
		request.RemoteAddr = "198.51.100.7:40000"
		request.Header.Set("Authorization", fmt.Sprintf("Bearer fake-%d", i))
		go func() {
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, request)
			done <- response.Code
		}()
		<-f.verifying
	}
	assertAdmissionFailure(t, f.serve("198.51.100.7", "fake-extra"), 1)

	// Another network still verifies new credentials.
	if code := f.serve("192.0.2.10", "victim-key").Code; code != http.StatusOK {
		t.Fatalf("other network = %d, want 200", code)
	}

	// Callers that disconnect during verification still count as failures.
	cancel()
	for range failedAuthNetworkMaxInFlight {
		<-done
	}
	if delay := f.admission.failedAuthNetwork.wait("198.51.100.7", f.now); delay == 0 {
		t.Fatal("cancelled verifications did not count as failures")
	}
}

func TestMCPAdmissionAccountLimitSpansCredentials(t *testing.T) {
	f := newAdmissionFixture(t, map[string]string{"token-1": "user", "token-2": "user"}, nil)
	ok := f.countStatus(accountBurst/2, "192.0.2.10", "token-1", http.StatusOK) +
		f.countStatus(accountBurst, "192.0.2.10", "token-2", http.StatusOK)
	if ok != accountBurst {
		t.Fatalf("account admitted %d requests, want burst %d", ok, accountBurst)
	}
	// Sustained load receives the refill rate.
	f.now = f.now.Add(time.Second)
	if ok := f.countStatus(100, "192.0.2.10", "token-1", http.StatusOK); ok != int(accountRate) {
		t.Fatalf("sustained account requests = %d, want %d", ok, int(accountRate))
	}
}

func TestMCPAdmissionAnonymousTotalCeiling(t *testing.T) {
	f := newAdmissionFixture(t, nil, nil)
	unauthorized := 0
	for i := range anonymousTotalBurst + 10 {
		if f.serve(fmt.Sprintf("203.0.113.%d", i), "").Code == http.StatusUnauthorized {
			unauthorized++
		}
	}
	if unauthorized != anonymousTotalBurst {
		t.Fatalf("anonymous requests admitted = %d, want %d", unauthorized, anonymousTotalBurst)
	}
	assertAdmissionFailure(t, f.serve("203.0.113.200", ""), 1)
}

func TestMCPAdmissionAccountConcurrency(t *testing.T) {
	entered := make(chan struct{})
	// Only requests of "user" hold their slot until the test releases them.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.TokenInfoFromContext(r.Context()).UserID == "user" {
			entered <- struct{}{}
			<-r.Context().Done()
		}
		w.WriteHeader(http.StatusOK)
	})
	f := newAdmissionFixture(t, map[string]string{"token": "user", "other": "other-user"}, next)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, accountMaxInFlight)
	for range accountMaxInFlight {
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
		request.RemoteAddr = "192.0.2.10:40000"
		request.Header.Set("Authorization", "Bearer token")
		go func() {
			f.handler.ServeHTTP(httptest.NewRecorder(), request)
			done <- struct{}{}
		}()
		<-entered
	}
	assertAdmissionFailure(t, f.serve("192.0.2.10", "token"), 1)

	// Another account keeps its own slots.
	if code := f.serve("192.0.2.10", "other").Code; code != http.StatusOK {
		t.Fatalf("other account = %d, want 200", code)
	}

	// Cancelled requests release their slots.
	cancel()
	for range accountMaxInFlight {
		<-done
	}
	release, delay := f.admission.account.hold("user", f.now)
	if delay > 0 {
		t.Fatalf("slot after cancellation: wait %v", delay)
	}
	release()
}

func TestMCPAdmissionFailureContract(t *testing.T) {
	f := newAdmissionFixture(t, nil, nil)
	f.countStatus(anonymousNetworkBurst, "192.0.2.10", "", http.StatusUnauthorized)
	assertAdmissionFailure(t, f.serve("192.0.2.10", ""), 1)
}

func assertAdmissionFailure(t *testing.T, response *httptest.ResponseRecorder, retryAfterSeconds int) {
	t.Helper()
	var output toolErrorOutput
	decodeMCPResponse(t, response, &output)
	if response.Code != http.StatusTooManyRequests ||
		response.Header().Get("Retry-After") != strconv.Itoa(retryAfterSeconds) ||
		response.Header().Get("Cache-Control") != "no-store" ||
		output.Error.Code != "rate_limited" || output.Error.Retry != "after_delay" ||
		output.Error.RetryAfterMs != int64(retryAfterSeconds)*1000 || output.Error.Outcome != "not_applied" {
		t.Fatalf("admission failure = %d Retry-After=%q %#v", response.Code, response.Header().Get("Retry-After"), output)
	}
}

func TestKeyedLimiterStaysBounded(t *testing.T) {
	limiter := newKeyedLimiter(1, 1, 1)
	now := time.Unix(1_700_000_000, 0)
	held, _ := limiter.hold("held", now)
	defer held()
	for i := range maxTrackedKeys * 2 {
		limiter.take(strconv.Itoa(i), now)
	}
	if len(limiter.entries) > maxTrackedKeys {
		t.Fatalf("entries = %d, want at most %d", len(limiter.entries), maxTrackedKeys)
	}
	if _, ok := limiter.entries["held"]; !ok {
		t.Fatal("eviction removed an entry with a holder")
	}
}

func TestKeyedLimiterRejectsNewKeysWhenAllEntriesAreHeld(t *testing.T) {
	limiter := newKeyedLimiter(1, 1, 1)
	now := time.Unix(1_700_000_000, 0)
	for i := range maxTrackedKeys {
		if _, delay := limiter.hold(strconv.Itoa(i), now); delay > 0 {
			t.Fatalf("hold %d: wait %v", i, delay)
		}
	}
	if _, delay := limiter.hold("new", now); delay == 0 {
		t.Fatal("a full limiter admitted a new key")
	}
	if delay := limiter.take("new", now); delay == 0 {
		t.Fatal("a full limiter admitted a new key")
	}
	if len(limiter.entries) != maxTrackedKeys {
		t.Fatalf("entries = %d, want %d", len(limiter.entries), maxTrackedKeys)
	}
}

func TestClientNetworkKey(t *testing.T) {
	tests := []struct {
		remote, resolved, want string
	}{
		{remote: "192.0.2.10:1234", want: "192.0.2.10"},
		{remote: "[2001:db8:1:2:3::4]:1234", want: "2001:db8:1:2::/64"},
		{remote: "[::ffff:192.0.2.10]:1234", want: "192.0.2.10"},
		{remote: "10.0.0.1:1234", resolved: "2001:db8:1:2:ffff::1", want: "2001:db8:1:2::/64"},
		{remote: "not-an-address", want: ""},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request.RemoteAddr = test.remote
		if test.resolved != "" {
			request = WithClientAddress(request, test.resolved)
		}
		if got := clientNetworkKey(request); got != test.want {
			t.Errorf("clientNetworkKey(%q, %q) = %q, want %q", test.remote, test.resolved, got, test.want)
		}
	}
}
