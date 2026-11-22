package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
)

// fakeAuthClock is the clock of the auth generation check window in tests.
// It starts at time.Now, so its values keep a monotonic clock reading.
type fakeAuthClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeAuthClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeAuthClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// installAuthClock gives the window of core a fake clock and removes the
// records of checks that ran before. Call it before the code under test
// validates credentials.
func installAuthClock(core *ChattoCore) *fakeAuthClock {
	clock := &fakeAuthClock{now: time.Now()}
	core.authWindow.now = clock.Now
	core.authWindow.closeAll()
	// A strict check that starts at the moment of closeAll is not recorded.
	clock.Advance(time.Nanosecond)
	return clock
}

// userLeaderReads runs fn and returns the subjects of one user whose newest
// position a client read from the EVT stream leader meanwhile. Background
// source checks read wildcard filters of the form evt.user.*.{type}; the
// helper ignores them.
func userLeaderReads(t *testing.T, nc *nats.Conn, userID string, fn func()) []string {
	t.Helper()
	sub, err := nc.SubscribeSync("$JS.API.STREAM.MSG.GET.EVT")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	fn()
	// fn has the replies to its requests, so the server has sent the
	// requests to this subscription already. The flush returns after them.
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	pending, _, err := sub.Pending()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	var got []string
	for range pending {
		msg, err := sub.NextMsg(time.Second)
		if err != nil {
			t.Fatalf("next request: %v", err)
		}
		var request struct {
			LastBySubject string `json:"last_by_subj"`
		}
		if err := json.Unmarshal(msg.Data, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if subject := request.LastBySubject; strings.HasPrefix(subject, "evt.user."+userID+".") {
			got = append(got, subject)
		}
	}
	return got
}

// authCheckCounts is the change of the auth generation check counters.
type authCheckCounts struct {
	fast   uint64
	strict map[string]uint64
}

func countAuthChecks(core *ChattoCore, fn func()) authCheckCounts {
	before := core.AuthGenerationCheckMetrics()
	fn()
	after := core.AuthGenerationCheckMetrics()
	counts := authCheckCounts{fast: after.Fast - before.Fast, strict: map[string]uint64{}}
	for reason, count := range after.Strict {
		if delta := count - before.Strict[reason]; delta != 0 {
			counts.strict[reason] = delta
		}
	}
	return counts
}

// holdProjection stops projector from applying events until the returned
// function runs. Test cleanup also releases it.
func holdProjection(t *testing.T, projector *events.Projector) func() {
	t.Helper()
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	go func() {
		done <- projector.WithReadBarrier(func(uint64) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	t.Cleanup(releaseOnce)
	return func() {
		releaseOnce()
		if err := <-done; err != nil {
			t.Errorf("read barrier: %v", err)
		}
	}
}

// passwordChangedEvent revokes every credential of the user. It does not
// delete the stored credentials, unlike SetPasswordHash, so a test can show
// what validation does with a stored credential that the event revoked.
func passwordChangedEvent(userID string) *evtv1.Event {
	return &evtv1.Event{Event: &evtv1.Event_UserPasswordHashChanged{UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{UserId: userID, PasswordHash: []byte("revoked")}}}
}

func deactivationEvent(userID string, deactivated bool) *evtv1.Event {
	return &evtv1.Event{Event: &evtv1.Event_UserAccountDeactivationChanged{UserAccountDeactivationChanged: &evtv1.UserAccountDeactivationChangedEvent{UserId: userID, Deactivated: deactivated}}}
}

func mustAppendUserEvent(t *testing.T, core *ChattoCore, userID string, event *evtv1.Event) {
	t.Helper()
	if _, err := core.appendUserEvent(testContext(t), userID, newEvent(SystemActorID, event), "", nil); err != nil {
		t.Fatalf("append user event: %v", err)
	}
}

func mustCreateCookieSession(t *testing.T, core *ChattoCore, userID string) string {
	t.Helper()
	sessionID, _, err := core.CreateCookieSession(testContext(t), userID, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	return sessionID
}

func mustValidateCookie(t *testing.T, core *ChattoCore, sessionID string) {
	t.Helper()
	if _, err := core.ValidateCookieCredential(testContext(t), sessionID); err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
}

func TestAuthGenerationWindowRecords(t *testing.T) {
	const window = authGenerationCheckWindow
	newWindow := func() (*authGenerationWindow, *fakeAuthClock) {
		clock := &fakeAuthClock{now: time.Now()}
		w := newAuthGenerationWindow()
		w.now = clock.Now
		return w, clock
	}

	t.Run("the window is half open", func(t *testing.T) {
		w, clock := newWindow()
		w.accepted("u", clock.Now(), 0)
		clock.Advance(window - time.Nanosecond)
		if !w.allows("u", 0) {
			t.Fatal("record not valid just inside the window")
		}
		clock.Advance(time.Nanosecond)
		if w.allows("u", 0) {
			t.Fatal("record valid at the window boundary")
		}
	})
	t.Run("the window starts when the strict check starts", func(t *testing.T) {
		w, clock := newWindow()
		startedAt := clock.Now()
		clock.Advance(window / 2)
		w.accepted("u", startedAt, 0)
		clock.Advance(window / 2)
		if w.allows("u", 0) {
			t.Fatal("record valid one window after the strict check started")
		}
	})
	t.Run("a strict check that started outside the window is not recorded", func(t *testing.T) {
		w, clock := newWindow()
		startedAt := clock.Now()
		clock.Advance(window)
		w.accepted("u", startedAt, 0)
		if w.size() != 0 {
			t.Fatalf("size = %d, want 0", w.size())
		}
	})
	t.Run("a clock that steps back does not extend the window", func(t *testing.T) {
		w, clock := newWindow()
		w.accepted("u", clock.Now(), 0)
		clock.Advance(-time.Second)
		if w.allows("u", 0) {
			t.Fatal("record valid before the strict check started")
		}
	})
	t.Run("a different reconnect count ends the window", func(t *testing.T) {
		w, clock := newWindow()
		w.accepted("u", clock.Now(), 3)
		if w.allows("u", 4) {
			t.Fatal("record valid after a reconnect")
		}
	})
	t.Run("a clear removes the acceptance", func(t *testing.T) {
		w, clock := newWindow()
		w.accepted("u", clock.Now(), 0)
		clock.Advance(time.Millisecond)
		w.clear("u")
		if w.allows("u", 0) {
			t.Fatal("record valid after clear")
		}
	})
	t.Run("a strict check that started before a clear cannot record", func(t *testing.T) {
		// The check started before a revocation and accepted; a later check
		// rejected. The earlier check must not make the record valid again.
		w, clock := newWindow()
		startedAt := clock.Now()
		clock.Advance(time.Millisecond)
		w.clear("u")
		w.accepted("u", startedAt, 0)
		if w.allows("u", 0) {
			t.Fatal("record valid from a strict check that started before the clear")
		}
		clock.Advance(time.Millisecond)
		w.accepted("u", clock.Now(), 0)
		if !w.allows("u", 0) {
			t.Fatal("strict check after the clear not recorded")
		}
	})
	t.Run("an older strict check does not replace a newer record", func(t *testing.T) {
		w, clock := newWindow()
		older := clock.Now()
		clock.Advance(time.Second)
		w.accepted("u", clock.Now(), 0)
		w.accepted("u", older, 0)
		clock.Advance(window - time.Second)
		if !w.allows("u", 0) {
			t.Fatal("older strict check moved the record back")
		}
	})
	t.Run("a closed gate ends every window and blocks older strict checks", func(t *testing.T) {
		w, clock := newWindow()
		startedAt := clock.Now()
		w.accepted("a", startedAt, 0)
		w.accepted("b", startedAt, 0)
		clock.Advance(time.Millisecond)
		w.closeAll()
		w.accepted("c", startedAt, 0)
		if w.allows("a", 0) || w.allows("b", 0) || w.allows("c", 0) || w.size() != 0 {
			t.Fatal("record valid after a closed gate")
		}
	})
	t.Run("expired entries are pruned and the cap holds", func(t *testing.T) {
		w, clock := newWindow()
		w.capacity = 3
		for i := range 3 {
			w.accepted(fmt.Sprint(i), clock.Now(), 0)
		}
		w.accepted("full", clock.Now(), 0)
		if w.allows("full", 0) || w.size() != 3 {
			t.Fatalf("full table recorded a user: size = %d", w.size())
		}
		w.clear("full")
		if w.size() != 3 {
			t.Fatalf("full table stored a clear: size = %d", w.size())
		}
		clock.Advance(window)
		w.accepted("full", clock.Now(), 0)
		if !w.allows("full", 0) || w.size() != 1 {
			t.Fatalf("expired entries not pruned: size = %d", w.size())
		}
	})
	t.Run("entries are pruned once each window without a full table", func(t *testing.T) {
		w, clock := newWindow()
		w.accepted("old", clock.Now(), 0)
		w.clear("rejected")
		clock.Advance(window)
		w.accepted("new", clock.Now(), 0)
		if w.size() != 1 {
			t.Fatalf("size = %d, want 1", w.size())
		}
	})
}

// The window exists and has exact bounds: inside it, validation sends no
// position read to the stream leader; at its boundary and after it, the next
// validation is strict.
func TestAuthGenerationWindowBounds(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)
	clock := installAuthClock(core)
	user, err := core.CreateUser(ctx, SystemActorID, "window-bounds", "Window Bounds", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)
	bearer, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	clock.Advance(authGenerationCheckWindow)

	validate := func(step string, wantReads bool) {
		t.Helper()
		reads := userLeaderReads(t, nc, user.Id, func() { mustValidateCookie(t, core, sessionID) })
		if got := len(reads) > 0; got != wantReads {
			t.Fatalf("%s: position reads = %v, want reads: %v", step, reads, wantReads)
		}
	}
	validate("first validation", true)
	clock.Advance(authGenerationCheckWindow - time.Nanosecond)
	validate("last moment inside the window", false)
	reads := userLeaderReads(t, nc, user.Id, func() {
		if _, err := core.ValidatePublicBearerCredential(ctx, bearer.AccessToken); err != nil {
			t.Fatalf("ValidatePublicBearerCredential: %v", err)
		}
	})
	if len(reads) != 0 {
		t.Fatalf("bearer inside the window: position reads = %v, want none", reads)
	}
	clock.Advance(time.Nanosecond)
	validate("window boundary", true)
	clock.Advance(authGenerationCheckWindow + time.Second)
	validate("after the window", true)
}

// The read budget of ADR-127: 50 validations of one credential within one
// window cost one strict check. Before ADR-127, each validation was strict;
// WithStrictAuthGenerationCheck gives that behavior for the comparison.
func TestAuthGenerationWindowLeaderReadsForFiftyValidations(t *testing.T) {
	core, nc := setupTestCore(t)
	installAuthClock(core)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "fifty-validations", "Fifty Validations", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)
	installAuthClock(core)

	measure := func(ctx context.Context) int {
		return len(userLeaderReads(t, nc, user.Id, func() {
			for range 50 {
				if _, err := core.ValidateCookieCredential(ctx, sessionID); err != nil {
					t.Fatalf("ValidateCookieCredential: %v", err)
				}
			}
		}))
	}
	strict := measure(WithStrictAuthGenerationCheck(ctx))
	installAuthClock(core)
	windowed := measure(ctx)
	t.Logf("position reads for 50 validations: strict on every call = %d, with the window = %d", strict, windowed)
	if subjects := len(userAuthGenerationSubjects(user.Id)); strict != 50*subjects || windowed != subjects {
		t.Fatalf("position reads = %d strict, %d windowed; want %d and %d", strict, windowed, 50*subjects, subjects)
	}
}

// ADR-127 changes the security guarantee on purpose. A replica whose User
// Auth projection has not applied a revocation accepts the revoked credential
// inside the window of the last strict check, without a read from the stream
// leader. The first validation after the window is strict: it waits for the
// projection and rejects. This test documents that bounded exposure.
func TestAuthGenerationWindowAcceptsRevokedCredentialOnLaggingReplicaOnlyInsideWindow(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	observer := newReplicaConn(t, ns)
	ctx := testContext(t)
	clock := installAuthClock(second)

	user, err := first.CreateUser(ctx, SystemActorID, "lagging-replica", "Lagging Replica", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, first, user.Id)
	mustValidateCookie(t, second, sessionID) // strict: starts the window

	release := holdProjection(t, second.userModel.auth.Projector())
	mustAppendUserEvent(t, first, user.Id, passwordChangedEvent(user.Id))

	clock.Advance(authGenerationCheckWindow - time.Nanosecond)
	reads := userLeaderReads(t, observer, user.Id, func() { mustValidateCookie(t, second, sessionID) })
	if len(reads) != 0 {
		t.Fatalf("inside the window: position reads = %v, want none", reads)
	}

	clock.Advance(time.Nanosecond)
	result := make(chan error, 1)
	go func() {
		_, err := second.ValidateCookieCredential(ctx, sessionID)
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("validation after the window returned %v before the projection applied the revocation", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	if err := <-result; !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("validation after the window: err = %v, want ErrCookieSessionNotFound", err)
	}
}

// When the projection has applied a revocation, validation inside the window
// rejects: the fast check sees the mismatch, and the strict check confirms
// it. The strict rejection also clears the record of the user.
func TestAuthGenerationWindowRejectsAppliedRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	installAuthClock(core)
	for name, revoke := range map[string]func(userID string) *evtv1.Event{
		"password change": passwordChangedEvent,
		"external identity unlink": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityUnlinked{UserExternalIdentityUnlinked: &evtv1.UserExternalIdentityUnlinkedEvent{UserId: userID, SubjectHash: "subject-hash"}}}
		},
		"account deletion": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: userID}}}
		},
		"deactivation": func(userID string) *evtv1.Event { return deactivationEvent(userID, true) },
		"key shredding requested": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShreddingRequested{UserKeyShreddingRequested: &evtv1.UserKeyShreddingRequestedEvent{UserId: userID}}}
		},
		"key shredded": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: userID}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := testContext(t)
			user, err := core.CreateUser(ctx, SystemActorID, strings.ReplaceAll(name, " ", "-"), "Revoked User", "password123")
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			sessionID := mustCreateCookieSession(t, core, user.Id)
			if counts := countAuthChecks(core, func() { mustValidateCookie(t, core, sessionID) }); counts.fast != 1 {
				t.Fatalf("validation inside the window: %+v, want one fast check", counts)
			}

			mustAppendUserEvent(t, core, user.Id, revoke(user.Id))
			// Wait for the projection without the auth generation check, which
			// would change the record.
			if err := core.waitForUserAuthGenerationCurrent(ctx, user.Id); err != nil {
				t.Fatalf("wait for projection: %v", err)
			}
			counts := countAuthChecks(core, func() {
				if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
					t.Fatalf("validation after applied revocation: err = %v, want ErrCookieSessionNotFound", err)
				}
			})
			if counts.fast != 0 || counts.strict["mismatch"] != 1 {
				t.Fatalf("checks = %+v, want no fast check and one strict check for the mismatch", counts)
			}
			if core.authWindow.allows(user.Id, core.natsReconnects()) {
				t.Fatal("record of the rejected user survived")
			}
		})
	}
}

// The fast check never gives a negative answer. A lagging replica that does
// not know an account yet, does not know its reactivation, or does not know
// its new auth generation, waits for its projection and accepts. It deletes
// no stored credential.
func TestAuthGenerationWindowHasNoStaleNegative(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	installAuthClock(second)

	// checkOnLaggingReplica writes on the first replica while the User Auth
	// projection of the second replica is held, and validates the returned
	// cookie session on the second replica.
	checkOnLaggingReplica := func(t *testing.T, write func() string) {
		t.Helper()
		var sessionID string
		// Check the first pass alone: the authoritative second pass would hide
		// a stale negative answer of the first pass.
		err := checkWhileHeld(t, second.userModel.auth.Projector(), func() { sessionID = write() }, func() error {
			_, err := second.validateRuntimeCredential(ctx, sessionID, AuthTokenPresentationCookie, fastCredentialRead)
			return err
		})
		if err != nil {
			t.Fatalf("first pass of validation on lagging replica: %v", err)
		}
		mustValidateCookie(t, second, sessionID)
		if _, err := first.storage.runtimeStateKV.Get(ctx, first.authTokenKey(sessionID)); err != nil {
			t.Fatalf("stored credential after validation: %v", err)
		}
	}
	newUser := func(t *testing.T, login string) string {
		t.Helper()
		user, err := first.CreateUser(ctx, SystemActorID, login, "Replica User", "")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		return user.Id
	}

	t.Run("new account", func(t *testing.T) {
		checkOnLaggingReplica(t, func() string {
			return mustCreateCookieSession(t, first, newUser(t, "stale-negative-new"))
		})
	})
	t.Run("reactivated account", func(t *testing.T) {
		userID := newUser(t, "stale-negative-reactivated")
		mustValidateCookie(t, second, mustCreateCookieSession(t, first, userID))
		mustAppendUserEvent(t, first, userID, deactivationEvent(userID, true))
		if err := second.waitForUserAuthGenerationCurrent(ctx, userID); err != nil {
			t.Fatalf("wait for projection: %v", err)
		}
		checkOnLaggingReplica(t, func() string {
			mustAppendUserEvent(t, first, userID, deactivationEvent(userID, false))
			return mustCreateCookieSession(t, first, userID)
		})
	})
	t.Run("new auth generation inside a window", func(t *testing.T) {
		userID := newUser(t, "stale-negative-generation")
		mustValidateCookie(t, second, mustCreateCookieSession(t, first, userID))
		if !second.authWindow.allows(userID, second.natsReconnects()) {
			t.Fatal("no window on the second replica")
		}
		checkOnLaggingReplica(t, func() string {
			mustAppendUserEvent(t, first, userID, passwordChangedEvent(userID))
			return mustCreateCookieSession(t, first, userID)
		})
	})
}

// Every path that reads through the stream leader keeps the strict check on
// every call, also inside a window (ADR-127 section 3).
func TestAuthGenerationStrictPathsStayStrictInsideWindow(t *testing.T) {
	core, nc := setupTestCore(t)
	installAuthClock(core)
	const password = "password123"
	const redirectURI = "https://example.com/callback"
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	type fixture struct {
		ctx                         context.Context
		userID, login, cookie, code string
		bearer                      BearerSessionCredentials
		generation                  uint64
	}
	tests := []struct {
		name string
		// firstPass is true when the path starts with the read-only first
		// pass, which can use the fast check before the strict one.
		firstPass bool
		run       func(f fixture) error
	}{
		{name: "second pass", run: func(f fixture) error {
			_, err := core.validateRuntimeCredential(f.ctx, f.cookie, AuthTokenPresentationCookie, authoritativeCredentialRead)
			return err
		}},
		{name: "cookie renewal", run: func(f fixture) error {
			_, _, err := core.RenewCookieSession(f.ctx, f.cookie, time.Now())
			return err
		}},
		{name: "mark cookie fresh", run: func(f fixture) error {
			return core.MarkCookieSessionFresh(f.ctx, f.cookie, "password", "test")
		}},
		{name: "mark bearer fresh", run: func(f fixture) error {
			return core.MarkBearerTokenFresh(f.ctx, f.bearer.AccessToken, "password", "test")
		}},
		{name: "require fresh cookie", run: func(f fixture) error {
			return ignoreFreshAuthRequired(core.RequireFreshAuthForCookieSession(f.ctx, f.cookie))
		}},
		{name: "require fresh bearer", run: func(f fixture) error {
			return ignoreFreshAuthRequired(core.RequireFreshAuthForBearerToken(f.ctx, f.bearer.AccessToken))
		}},
		{name: "cookie privileged mode", run: func(f fixture) error {
			_, err := core.SetCookiePrivilegedMode(f.ctx, f.cookie, DeactivatePrivilegedMode)
			return err
		}},
		{name: "bearer privileged mode", firstPass: true, run: func(f fixture) error {
			_, err := core.SetBearerPrivilegedMode(f.ctx, f.bearer.AccessToken, DeactivatePrivilegedMode)
			return err
		}},
		{name: "logout", run: func(f fixture) error {
			_, _, err := core.RevokePresentedRuntimeCredentialWithReason(f.ctx, f.cookie, AuthTokenPresentationCookie, "logout")
			return err
		}},
		{name: "refresh token logout", run: func(f fixture) error {
			_, _, err := core.RevokeRefreshTokenWithReasonResult(f.ctx, f.bearer.RefreshToken, "logout")
			return err
		}},
		{name: "refresh rotation", run: func(f fixture) error {
			_, err := core.RefreshBearerSession(f.ctx, f.bearer.RefreshToken, testRefreshRequestIDA, "")
			return err
		}},
		{name: "password verification", run: func(f fixture) error {
			_, _, err := core.VerifyPasswordWithAuthGeneration(f.ctx, f.login, password)
			return err
		}},
		{name: "create cookie session", run: func(f fixture) error {
			_, _, err := core.CreateCookieSession(f.ctx, f.userID, "password_login")
			return err
		}},
		{name: "create cookie session for generation", run: func(f fixture) error {
			_, _, err := core.CreateCookieSessionForGeneration(f.ctx, f.userID, "password_login", f.generation)
			return err
		}},
		{name: "new cookie session data", run: func(f fixture) error {
			_, err := core.NewCookieSessionData(f.ctx, f.userID, "password_login")
			return err
		}},
		{name: "new cookie session data for generation", run: func(f fixture) error {
			_, err := core.NewCookieSessionDataForGeneration(f.ctx, f.userID, "password_login", f.generation)
			return err
		}},
		{name: "create bearer session", run: func(f fixture) error {
			_, err := core.CreateBearerSessionWithSource(f.ctx, f.userID, "password_login")
			return err
		}},
		{name: "create bearer session for grant", run: func(f fixture) error {
			_, err := core.CreateBearerSessionWithSourceGeneration(f.ctx, f.userID, "password_login", f.generation)
			return err
		}},
		{name: "create auth code", run: func(f fixture) error {
			_, err := core.CreateAuthCode(f.ctx, f.userID, redirectURI, GenerateCodeChallenge(verifier), "S256")
			return err
		}},
		{name: "create auth code for generation", run: func(f fixture) error {
			_, err := core.CreateAuthCodeForGeneration(f.ctx, f.userID, redirectURI, GenerateCodeChallenge(verifier), "S256", f.generation)
			return err
		}},
		{name: "exchange auth code", run: func(f fixture) error {
			_, _, err := core.ExchangeAuthCodeForClientResourceSession(f.ctx, f.code, verifier, redirectURI, "", "")
			return err
		}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testContext(t)
			login := fmt.Sprintf("strict-path-%d", i)
			user, err := core.CreateUser(ctx, SystemActorID, login, "Strict Path", password)
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			f := fixture{ctx: ctx, userID: user.Id, login: login, cookie: mustCreateCookieSession(t, core, user.Id)}
			if f.bearer, err = core.CreateBearerSessionWithSource(ctx, user.Id, "password_login"); err != nil {
				t.Fatalf("CreateBearerSessionWithSource: %v", err)
			}
			if f.code, err = core.CreateAuthCode(ctx, user.Id, redirectURI, GenerateCodeChallenge(verifier), "S256"); err != nil {
				t.Fatalf("CreateAuthCode: %v", err)
			}
			f.generation = mustCurrentAuthGeneration(t, core, user.Id)
			if counts := countAuthChecks(core, func() { mustValidateCookie(t, core, f.cookie) }); counts.fast != 1 {
				t.Fatalf("no window before the path: %+v", counts)
			}

			var reads []string
			counts := countAuthChecks(core, func() {
				reads = userLeaderReads(t, nc, user.Id, func() {
					if err := tt.run(f); err != nil {
						t.Fatalf("path: %v", err)
					}
				})
			})
			if counts.strict["path"] == 0 || len(reads) == 0 {
				t.Fatalf("checks = %+v, position reads = %v; want a strict check", counts, reads)
			}
			if !tt.firstPass && counts.fast != 0 {
				t.Fatalf("checks = %+v, want no fast check", counts)
			}
		})
	}
}

func ignoreFreshAuthRequired(err error) error {
	if errors.Is(err, ErrFreshAuthRequired) {
		return nil
	}
	return err
}

// The realtime timer and other callers can require the strict check in the
// first pass with WithStrictAuthGenerationCheck.
func TestAuthGenerationWindowStrictContext(t *testing.T) {
	core, _ := setupTestCore(t)
	installAuthClock(core)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "strict-context", "Strict Context", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)
	bearer, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	strictCtx := WithStrictAuthGenerationCheck(ctx)
	counts := countAuthChecks(core, func() {
		if _, err := core.ValidateCookieCredential(strictCtx, sessionID); err != nil {
			t.Fatalf("ValidateCookieCredential: %v", err)
		}
		if _, err := core.ValidatePublicBearerCredential(strictCtx, bearer.AccessToken); err != nil {
			t.Fatalf("ValidatePublicBearerCredential: %v", err)
		}
	})
	if counts.fast != 0 || counts.strict["path"] != 2 {
		t.Fatalf("checks = %+v, want two strict checks", counts)
	}
}

// Gate 1: while NATS recovery is not complete, every validation is strict.
// The closed gate ends the window, so the first validation after recovery is
// strict too. The test sets the recovery state that runNATSRecovery sets.
func TestAuthGenerationWindowConnectionGate(t *testing.T) {
	core, _ := setupTestCore(t)
	clock := installAuthClock(core)
	user, err := core.CreateUser(testContext(t), SystemActorID, "connection-gate", "Connection Gate", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)

	core.natsRecoveryState.Store(natsRecoveryInProgress)
	t.Cleanup(func() { core.natsRecoveryState.Store(natsRecoveryReady) })
	if counts := countAuthChecks(core, func() { mustValidateCookie(t, core, sessionID) }); counts.fast != 0 || counts.strict["gate"] != 1 {
		t.Fatalf("recovery in progress: checks = %+v, want one strict check for the gate", counts)
	}
	core.natsRecoveryState.Store(natsRecoveryReady)
	clock.Advance(time.Nanosecond) // a strict check at the moment the gate closed is not recorded
	if counts := countAuthChecks(core, func() { mustValidateCookie(t, core, sessionID) }); counts.fast != 0 || counts.strict["window"] != 1 {
		t.Fatalf("after recovery: checks = %+v, want one strict check for the window", counts)
	}
	if counts := countAuthChecks(core, func() { mustValidateCookie(t, core, sessionID) }); counts.fast != 1 {
		t.Fatalf("after a strict check: checks = %+v, want one fast check", counts)
	}
}

// Gate 1 also covers a reconnect that a replica sees before its recovery
// starts: the reconnect counter differs from the recovered one. The test
// forces a real reconnect of the replica's connection.
func TestAuthGenerationWindowReconnectEndsWindow(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	nc := newReplicaConn(t, ns)
	core, err := NewChattoCore(testContext(t), nc, testCoreConfig())
	if err != nil {
		t.Fatalf("create core: %v", err)
	}
	startCoreServices(t, core)
	installAuthClock(core)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "reconnect-gate", "Reconnect Gate", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)
	if !core.authWindow.allows(user.Id, core.natsReconnects()) {
		t.Fatal("no window before the reconnect")
	}

	reconnected := make(chan struct{})
	nc.SetReconnectHandler(func(*nats.Conn) { close(reconnected) })
	if err := nc.ForceReconnect(); err != nil {
		t.Fatalf("ForceReconnect: %v", err)
	}
	select {
	case <-reconnected:
	case <-ctx.Done():
		t.Fatal("no reconnect")
	}
	// The reconnect counter changed, so the record is invalid whatever the
	// state of the recovery.
	if core.authWindow.allows(user.Id, core.natsReconnects()) {
		t.Fatal("record valid after a reconnect")
	}
	if counts := countAuthChecks(core, func() { _, _ = core.ValidateCookieCredential(ctx, sessionID) }); counts.fast != 0 {
		t.Fatalf("after reconnect: checks = %+v, want no fast check", counts)
	}
}

// Gate 2. A test cannot make the User Auth projector stall without waiting
// for DefaultProjectorStallAfter. A projector failure also stops ChattoCore.Run,
// which ends a test core. Thus this test checks the gate decision function
// with the status that the projector reports.
func TestUserAuthProjectorGate(t *testing.T) {
	for name, tt := range map[string]struct {
		status events.ProjectorStatus
		open   bool
	}{
		"current":         {events.ProjectorStatus{Started: true, StartupComplete: true}, true},
		"stalled":         {events.ProjectorStatus{Started: true, StartupComplete: true, Stalled: true}, false},
		"failed":          {events.ProjectorStatus{Started: true, StartupComplete: true, Err: events.ErrProjectionFailed}, false},
		"startup running": {events.ProjectorStatus{Started: true}, false},
		"not yet started": {events.ProjectorStatus{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := userAuthProjectorGateOpen(tt.status); got != tt.open {
				t.Fatalf("gate open = %v, want %v", got, tt.open)
			}
		})
	}
}

// Many concurrent validations of one user, while a revocation arrives, end
// with the revocation applied, a rejection, and no record of the user.
func TestAuthGenerationWindowConcurrentValidationAndRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	clock := installAuthClock(core)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "concurrent-window", "Concurrent Window", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := mustCreateCookieSession(t, core, user.Id)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			<-start
			for range 10 {
				_, err := core.ValidateCookieCredential(ctx, sessionID)
				if err != nil && !errors.Is(err, ErrCookieSessionNotFound) {
					t.Errorf("ValidateCookieCredential: %v", err)
				}
				clock.Advance(time.Millisecond)
			}
		})
	}
	wg.Go(func() {
		<-start
		mustAppendUserEvent(t, core, user.Id, passwordChangedEvent(user.Id))
	})
	close(start)
	wg.Wait()

	if err := core.waitForUserAuthGenerationCurrent(ctx, user.Id); err != nil {
		t.Fatalf("wait for projection: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("validation after revocation: err = %v, want ErrCookieSessionNotFound", err)
	}
	if core.authWindow.allows(user.Id, core.natsReconnects()) {
		t.Fatal("record of the revoked user survived")
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.authTokenKey(sessionID)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("revoked credential still stored: %v", err)
	}
}

// The window starts when the strict check starts, not when its wait for the
// projection ends. A revocation that the check did not see was written after
// it started, so the exposure stays below one window from that moment.
func TestAuthGenerationWindowStartsWithTheStrictCheck(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	clock := installAuthClock(second)
	user, err := first.CreateUser(ctx, SystemActorID, "window-start", "Window Start", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := second.waitForUserAuthGenerationCurrent(ctx, user.Id); err != nil {
		t.Fatalf("wait for projection: %v", err)
	}

	// The strict check on the second replica waits for a new auth generation
	// while the clock moves half a window.
	release := holdProjection(t, second.userModel.auth.Projector())
	mustAppendUserEvent(t, first, user.Id, passwordChangedEvent(user.Id))
	sessionID := mustCreateCookieSession(t, first, user.Id)
	result := make(chan error, 1)
	go func() {
		_, err := second.ValidateCookieCredential(ctx, sessionID)
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("validation returned %v before the projection applied the new generation", err)
	case <-time.After(100 * time.Millisecond):
	}
	clock.Advance(authGenerationCheckWindow / 2)
	release()
	if err := <-result; err != nil {
		t.Fatalf("validation: %v", err)
	}

	clock.Advance(authGenerationCheckWindow/2 - time.Nanosecond)
	if counts := countAuthChecks(second, func() { mustValidateCookie(t, second, sessionID) }); counts.fast != 1 {
		t.Fatalf("inside the window: checks = %+v, want one fast check", counts)
	}
	clock.Advance(time.Nanosecond)
	if counts := countAuthChecks(second, func() { mustValidateCookie(t, second, sessionID) }); counts.fast != 0 {
		t.Fatalf("one window after the strict check started: checks = %+v, want a strict check", counts)
	}
}
