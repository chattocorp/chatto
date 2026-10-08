package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
)

func TestMCPAdmissionBurstsAndSustainedLoad(t *testing.T) {
	now := time.Now()
	stage := newAdmissionStage(defaultAdmissionLimits())
	stage.now = func() time.Time { return now }
	first, second := sha256.Sum256([]byte("first")), sha256.Sum256([]byte("second"))
	admit := func(key [sha256.Size]byte, count int) {
		t.Helper()
		for range count {
			release, delay := stage.acquire(key)
			if release == nil || delay != 0 {
				t.Fatalf("independent allowance rejected: delay=%v", delay)
			}
			release()
		}
	}
	admit(first, 40)
	globalBefore := stage.global.TokensAt(now)
	for range 100 {
		if release, delay := stage.acquire(first); release != nil || delay != 50*time.Millisecond {
			t.Fatalf("burst overflow admitted or wrong delay: %v", delay)
		}
	}
	if stage.global.TokensAt(now) != globalBefore {
		t.Fatal("rejected caller consumed global allowance")
	}
	admit(second, 40)
	for range 10 {
		now = now.Add(time.Second)
		admit(first, 20)
		admit(second, 20)
		if release, _ := stage.acquire(first); release != nil {
			t.Fatal("sustained rate overflow admitted")
		}
	}
}

func TestMCPAdmissionGlobalCeilingsAndBoundedState(t *testing.T) {
	now := time.Now()
	limits := defaultAdmissionLimits()
	limits.globalRate, limits.globalBurst, limits.globalConcurrent, limits.maxCallers = 1, 2, 2, 2
	stage := newAdmissionStage(limits)
	stage.now = func() time.Time { return now }
	first, second, third := sha256.Sum256([]byte("first")), sha256.Sum256([]byte("second")), sha256.Sum256([]byte("third"))
	releaseFirst, _ := stage.acquire(first)
	releaseSecond, _ := stage.acquire(second)
	if releaseFirst == nil || releaseSecond == nil {
		t.Fatal("initial admission failed")
	}
	if release, _ := stage.acquire(first); release != nil {
		t.Fatal("global concurrency ceiling bypassed by an existing caller")
	}
	for range 100 {
		if release, _ := stage.acquire(third); release != nil {
			t.Fatal("global concurrency or table bound bypassed")
		}
	}
	releaseSecond()
	if release, delay := stage.acquire(first); release != nil || delay != time.Second {
		t.Fatal("global rate ceiling bypassed")
	}
	if stage.callers[first].limiter.TokensAt(now) != 39 || len(stage.callers) != 2 {
		t.Fatal("rejection consumed caller tokens or allocated state")
	}
	now = now.Add(2 * time.Minute)
	releaseThird, _ := stage.acquire(third)
	if releaseThird == nil || len(stage.callers) != 2 || stage.callers[first] == nil || stage.callers[second] != nil {
		t.Fatal("cleanup removed active state or retained idle state")
	}
	releaseFirst()
	releaseFirst() // Release must be safe when a cleanup path calls it twice.
	releaseThird()
	now = now.Add(2 * time.Minute)
	release, _ := stage.acquire(second)
	if release == nil || len(stage.callers) != 1 {
		t.Fatal("idle capacity was not recovered")
	}
	release()
}

func admissionTestHandler(a *admissionController, verifier auth.TokenVerifier, next http.Handler) http.Handler {
	return a.beforeAuthentication(auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(a.afterAuthentication(next)))
}

func admissionTestRequest(handler http.Handler, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", strings.NewReader("invalid-body-before-SDK"))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func assertAdmissionRetry(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var output toolErrorOutput
	decodeMCPResponse(t, response, &output)
	seconds, err := strconv.ParseInt(response.Header().Get("Retry-After"), 10, 64)
	if err != nil || seconds < 1 || response.Code != http.StatusTooManyRequests || response.Header().Get("Cache-Control") != "no-store" || output.Error.Code != "rate_limited" || output.Error.Retry != "after_delay" || output.Error.Outcome != "not_applied" || output.Error.RetryAfterMs != seconds*1000 {
		t.Fatalf("invalid retry contract: status=%d error=%#v", response.Code, output)
	}
	for _, private := range []string{"private-token", "private-account", "192.0.2.1"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatal("admission failure exposed private data")
		}
	}
}

func TestMCPAdmissionRejectsBeforeAuthentication(t *testing.T) {
	for _, token := range []string{"", "private-token"} {
		t.Run(fmt.Sprint(token != ""), func(t *testing.T) {
			a := newAdmissionController()
			now := time.Now()
			a.verification.now = func() time.Time { return now }
			verified := 0
			handler := admissionTestHandler(a, func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				verified++
				return nil, auth.ErrInvalidToken
			}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid credential reached tool work") }))
			for range 40 {
				if response := admissionTestRequest(handler, token); response.Code != http.StatusUnauthorized {
					t.Fatalf("initial status=%d", response.Code)
				}
			}
			assertAdmissionRetry(t, admissionTestRequest(handler, token))
			if verified != map[bool]int{false: 0, true: 40}[token != ""] || len(a.account.callers) != 0 {
				t.Fatal("early rejection reached authentication or allocated account state")
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer private-token")
	key := presentedCredentialKey(r)
	for _, header := range []string{"bearer private-token", "BEARER   private-token", " Bearer\tprivate-token "} {
		r.Header.Set("Authorization", header)
		if presentedCredentialKey(r) != key {
			t.Fatal("bearer header normalization resets a budget")
		}
	}
	for _, header := range []string{"", "Basic unrelated", "Bearer too many fields"} {
		r.Header.Set("Authorization", header)
		if presentedCredentialKey(r) != sha256.Sum256(nil) {
			t.Fatal("malformed header creates a budget")
		}
	}
}

func TestMCPAdmissionSharesAccountBudgetAcrossCredentials(t *testing.T) {
	a := newAdmissionController()
	now := time.Now()
	a.verification.now, a.account.now = func() time.Time { return now }, func() time.Time { return now }
	handler := admissionTestHandler(a, func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		user := "private-account"
		if token == "other" {
			user = "other-account"
		}
		return &auth.TokenInfo{UserID: user}, nil
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := range 40 {
		if response := admissionTestRequest(handler, fmt.Sprintf("private-token-%d", i)); response.Code != http.StatusNoContent {
			t.Fatalf("initial request %d status=%d", i, response.Code)
		}
	}
	assertAdmissionRetry(t, admissionTestRequest(handler, "fresh-credential"))
	if response := admissionTestRequest(handler, "other"); response.Code != http.StatusNoContent {
		t.Fatal("one account exhausted another account's allowance")
	}
}

func TestMCPAdmissionBoundsUniqueInvalidCredentials(t *testing.T) {
	a := newAdmissionController()
	now := time.Now()
	a.verification.now = func() time.Time { return now }
	verified := 0
	handler := admissionTestHandler(a, func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		verified++
		return nil, auth.ErrInvalidToken
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid credential reached tool work") }))
	for i := range 200 {
		if response := admissionTestRequest(handler, fmt.Sprintf("invalid-%d", i)); response.Code != http.StatusUnauthorized {
			t.Fatal("initial verification budget rejected")
		}
	}
	for i := range 100 {
		assertAdmissionRetry(t, admissionTestRequest(handler, fmt.Sprintf("overflow-%d", i)))
	}
	if verified != 200 || len(a.verification.callers) != 200 {
		t.Fatal("unique-token flood bypassed global budget or allocated rejected state")
	}
	for second := range 10 {
		now = now.Add(time.Second)
		for i := range 100 {
			if response := admissionTestRequest(handler, fmt.Sprintf("sustained-%d-%d", second, i)); response.Code != http.StatusUnauthorized {
				t.Fatal("sustained verification budget rejected")
			}
		}
		assertAdmissionRetry(t, admissionTestRequest(handler, "sustained-overflow"))
	}
}

func TestMCPAdmissionBoundsConcurrentVerification(t *testing.T) {
	a := newAdmissionController()
	entered := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	handler := admissionTestHandler(a, func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token == "busy-invalid" {
			entered <- struct{}{}
			<-ctx.Done()
		}
		return nil, auth.ErrInvalidToken
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid credential reached tool work") }))
	for range 4 {
		wg.Go(func() {
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer busy-invalid")
			handler.ServeHTTP(httptest.NewRecorder(), r)
		})
	}
	for range 4 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("verification requests did not start")
		}
	}
	assertAdmissionRetry(t, admissionTestRequest(handler, "busy-invalid"))
	if response := admissionTestRequest(handler, "independent-invalid"); response.Code != http.StatusUnauthorized {
		t.Fatal("one credential filled another credential's verification slots")
	}
	cancel()
	wg.Wait()
	if a.verification.active != 0 || a.account.active != 0 {
		t.Fatal("failed authentication leaked admission slots")
	}
}

func TestMCPAdmissionConcurrencyCancellationAndPanicCleanup(t *testing.T) {
	a := newAdmissionController()
	var started atomic.Int32
	entered := make(chan struct{}, 4)
	handler := admissionTestHandler(a, func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token == "other" {
			return &auth.TokenInfo{UserID: "other-account"}, nil
		}
		return &auth.TokenInfo{UserID: "private-account"}, nil
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.TokenInfoFromContext(r.Context()).UserID == "other-account" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		started.Add(1)
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	for i := range 4 {
		wg.Go(func() {
			r := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
			r.Header.Set("Authorization", fmt.Sprintf("Bearer private-token-%d", i))
			handler.ServeHTTP(httptest.NewRecorder(), r)
		})
	}
	for range 4 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent requests did not start")
		}
	}
	assertAdmissionRetry(t, admissionTestRequest(handler, "fifth-credential"))
	if response := admissionTestRequest(handler, "other"); response.Code != http.StatusNoContent {
		t.Fatal("busy account blocked another account")
	}
	cancel()
	wg.Wait()
	if a.verification.active != 0 || a.account.active != 0 || started.Load() != 4 {
		t.Fatal("cancellation leaked slots or excess work started")
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(ctx)
	handler.ServeHTTP(httptest.NewRecorder(), r)
	if started.Load() != 4 {
		t.Fatal("already canceled request started work")
	}
	panicHandler := admissionTestHandler(a, func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{UserID: "private-account"}, nil
	}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") }))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic handler did not run")
			}
		}()
		admissionTestRequest(panicHandler, "panic-credential")
	}()
	if a.verification.active != 0 || a.account.active != 0 {
		t.Fatal("panic leaked slots")
	}
}

// Use production credential verification and resource handlers to prove that
// origin-specific grants for one account still consume one account allowance.
func TestMCPAdmissionAcrossResourceAliases(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	c, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "admission-secret", Assets: config.AssetsConfig{SigningSecret: "admission-assets"},
	})
	if err != nil {
		t.Fatal(err)
	}
	startTestCore(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a := newAdmissionController()
	a.account.limits.callerBurst = 2
	a.account.limits.callerRate = 0
	handler := requireConfiguredHost(map[string]http.Handler{
		"chat.example":  newResourceHandler(c, "https://chat.example", "https://chat.example/mcp", "test", a),
		"alias.example": newResourceHandler(c, "https://chat.example", "https://alias.example/mcp", "test", a),
	})
	users := map[string]string{}
	grant := func(login, resource string) string {
		t.Helper()
		userID := users[login]
		if userID == "" {
			user, err := c.CreateUser(ctx, core.SystemActorID, login, "Admission User", "password")
			if err != nil {
				t.Fatal(err)
			}
			userID = user.GetId()
			users[login] = userID
		}
		generation, err := c.CurrentAuthGeneration(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		credentials, err := c.CreateOAuthBearerSessionForClientGrant(ctx, userID, "https://agent.example/client.json", resource, config.MCPOAuthScopes(), generation)
		if err != nil {
			t.Fatal(err)
		}
		return credentials.AccessToken
	}
	first := grant("admission-first", "https://chat.example/mcp")
	alias := grant("admission-first", "https://alias.example/mcp")
	other := grant("admission-other", "https://alias.example/mcp")
	call := func(host, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://"+host+"/mcp", strings.NewReader(rawToolCallBody(t, "get_server_info", map[string]any{})))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2026-07-28")
		r.Header.Set("Mcp-Method", "tools/call")
		r.Header.Set("Mcp-Name", "get_server_info")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for range 2 {
		if response := call("chat.example", first); response.Code != http.StatusOK {
			t.Fatalf("initial call status=%d", response.Code)
		}
	}
	assertAdmissionRetry(t, call("alias.example", alias))
	if response := call("alias.example", other); response.Code != http.StatusOK {
		t.Fatal("alias caller did not keep an independent allowance")
	}
}

func TestMCPAdmissionRetryRounding(t *testing.T) {
	for _, test := range []struct {
		delay   time.Duration
		seconds string
	}{{0, "1"}, {50 * time.Millisecond, "1"}, {time.Second, "1"}, {time.Second + time.Nanosecond, "2"}} {
		w := httptest.NewRecorder()
		writeAdmissionFailure(w, test.delay)
		assertAdmissionRetry(t, w)
		if w.Header().Get("Retry-After") != test.seconds {
			t.Fatal("retry delay was not rounded up")
		}
	}
}

// A context timeout does not unblock net/http body reads or socket writes.
// Use real TCP clients that withhold input or stop reading the response.
func TestMCPAdmissionSlowHTTPClientsReleaseSlots(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			a := newAdmissionController()
			done := make(chan struct{})
			var ioErr error
			handler := admissionTestHandler(a, func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{UserID: "slow-client-account"}, nil
			}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if operation == "read" {
					_, ioErr = io.ReadAll(r.Body)
				} else {
					chunk := bytes.Repeat([]byte("x"), 64*1024)
					for range 1024 {
						if _, ioErr = w.Write(chunk); ioErr != nil {
							break
						}
					}
				}
			}))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
				defer cancel()
				withRequestDeadline(handler).ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}))
			defer server.Close()
			conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if tcp, ok := conn.(*net.TCPConn); ok {
				if err := tcp.SetReadBuffer(1024); err != nil {
					t.Fatal(err)
				}
			}
			length, body := 0, ""
			if operation == "read" {
				length, body = 10, "x"
			}
			if _, err := fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: chat.example\r\nAuthorization: Bearer slow-client-token\r\nContent-Length: %d\r\n\r\n%s", length, body); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
				if ioErr == nil || a.verification.active != 0 || a.account.active != 0 {
					t.Fatal("HTTP deadline did not interrupt I/O and release slots")
				}
			case <-time.After(time.Second):
				t.Fatal("slow HTTP client kept admission slots after the deadline")
			}
		})
	}
}
