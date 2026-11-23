package core

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

// createGenerationZeroPasswordUser returns a user whose auth generation stays
// 0 after the first password, because adding a first password preserves
// existing credentials.
func createGenerationZeroPasswordUser(t *testing.T, core *ChattoCore, login string) string {
	t.Helper()
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, login, login, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.SetInitialPasswordHash(ctx, user.Id, "oldpassword123"); err != nil {
		t.Fatalf("SetInitialPasswordHash: %v", err)
	}
	if generation := mustCurrentAuthGeneration(t, core, user.Id); generation != 0 {
		t.Fatalf("auth generation = %d, want 0", generation)
	}
	return user.Id
}

// A login that checks the old password while a reset is in progress can store
// a generation-0 cookie session after the password event. The new generation
// must revoke that session, whatever its CreatedAt time.
func TestChattoCore_PasswordChangeRevokesGenerationZeroSessionFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createGenerationZeroPasswordUser(t, core, "generation-zero-user")
	if err := core.SetPasswordHash(ctx, userID, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	now := time.Now()
	data, err := json.Marshal(AuthTokenData{
		UserID:       userID,
		Kind:         AuthTokenKindFirstPartySession,
		Presentation: AuthTokenPresentationCookie,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	sessionID := NewAuthToken()
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(sessionID), data, jetstream.KeyTTL(time.Hour)); err != nil {
		t.Fatalf("store session: %v", err)
	}

	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("generation-0 session after password change: err = %v, want ErrCookieSessionNotFound", err)
	}
}

// A logout that presents a credential from before a password change must not
// count as a live logout, because the handler then terminates every current
// session of the user.
func TestChattoCore_LogoutWithStaleCredentialAfterPasswordChangeIsNotLive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "stale-logout-user", "Stale Logout User", "oldpassword123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	stale, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource stale: %v", err)
	}
	staleRefresh, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource stale refresh: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	current, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource current: %v", err)
	}

	if userID, revoked, err := core.RevokePresentedRuntimeCredentialWithReason(ctx, stale.AccessToken, AuthTokenPresentationBearer, "logout"); err != nil || revoked || userID != "" {
		t.Fatalf("stale access logout = (%q, %v, %v), want (\"\", false, nil)", userID, revoked, err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.authTokenKey(stale.AccessToken)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("stale access record still stored: %v", err)
	}
	if userID, revoked, err := core.RevokeRefreshTokenWithReasonResult(ctx, staleRefresh.RefreshToken, "logout"); err != nil || revoked || userID != "" {
		t.Fatalf("stale refresh logout = (%q, %v, %v), want (\"\", false, nil)", userID, revoked, err)
	}

	if userID, revoked, err := core.RevokeRefreshTokenWithReasonResult(ctx, current.RefreshToken, "logout"); err != nil || !revoked || userID != user.Id {
		t.Fatalf("current refresh logout = (%q, %v, %v), want (%q, true, nil)", userID, revoked, err, user.Id)
	}
}

// A replica must not decide from a projection that has not applied the newest
// generation-relevant event of the user, whichever replica wrote it. The test
// stops the User Auth projection of the second replica, so a check that does
// not wait for the event reads the old state and fails here. It covers the
// event types that only the earlier, broader wait covered: account creation
// and both key-shredding events.
func TestCurrentAuthGenerationWaitsForEventFromOtherReplica(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)

	type result struct {
		generation uint64
		err        error
	}
	// checkGeneration writes on the first replica while the User Auth
	// projection of the second replica cannot apply events. Then it checks
	// the auth generation of the user that write returns on the second
	// replica.
	checkGeneration := func(t *testing.T, write func() string) result {
		t.Helper()
		var userID string
		return checkWhileHeld(t, second.userModel.auth.Projector(), func() { userID = write() }, func() result {
			generation, err := second.CurrentAuthGeneration(ctx, userID)
			return result{generation, err}
		})
	}

	for name, revoke := range map[string]func(userID string) *evtv1.Event{
		"account deleted": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: userID}}}
		},
		"key shredding requested": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShreddingRequested{UserKeyShreddingRequested: &evtv1.UserKeyShreddingRequestedEvent{UserId: userID}}}
		},
		"key shredded": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: userID}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			// An unknown user counts as revoked, so the check must wait for
			// the account creation too.
			var userID string
			created := checkGeneration(t, func() string {
				user, err := first.CreateUser(ctx, SystemActorID, strings.ReplaceAll(name, " ", "-"), "Replica User", "")
				if err != nil {
					t.Fatalf("CreateUser: %v", err)
				}
				userID = user.Id
				return userID
			})
			if created.err != nil || created.generation != 0 {
				t.Fatalf("new account: generation = %d, err = %v, want 0, nil", created.generation, created.err)
			}
			revoked := checkGeneration(t, func() string {
				if _, err := first.appendUserEvent(ctx, userID, newEvent(SystemActorID, revoke(userID)), "", nil); err != nil {
					t.Fatalf("append event: %v", err)
				}
				return userID
			})
			if !errors.Is(revoked.err, ErrAuthenticationRevoked) {
				t.Fatalf("after event on other replica: err = %v, want ErrAuthenticationRevoked", revoked.err)
			}
		})
	}
}

// Every validation of a runtime credential runs the auth generation check, so
// its cost in requests to the stream leader matters. It reads one position
// for each generation-relevant subject of the user, and no position of a
// subject of another user.
func TestCurrentAuthGenerationReadsOnlySubjectsOfTheUser(t *testing.T) {
	core, nc := setupTestCore(t)
	userID := createGenerationZeroPasswordUser(t, core, "read-budget-user")
	createGenerationZeroPasswordUser(t, core, "read-budget-other-user")

	got := userLeaderReads(t, nc, userID, func() { mustCurrentAuthGeneration(t, core, userID) })
	var want []string
	for _, eventType := range []string{
		"account_created", "password_hash_changed", "external_identity_unlinked", "account_deleted",
		"account_deactivation_changed", "user_key_shredding_requested", "user_key_shredded",
	} {
		want = append(want, "evt.user."+userID+"."+eventType)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("position reads of one user's subjects = %v, want %v", got, want)
	}
}
