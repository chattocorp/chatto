package core

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
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
// generation-relevant event of the user, whichever replica wrote it. This
// covers the event types that the per-user wait added: account creation and
// both key-shredding events.
func TestCurrentAuthGenerationFollowsOtherReplica(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)

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
			user, err := first.CreateUser(ctx, SystemActorID, strings.ReplaceAll(name, " ", "-"), "Replica User", "")
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			// The other replica knows the new account immediately. An unknown
			// user counts as revoked.
			if generation, err := second.CurrentAuthGeneration(ctx, user.Id); err != nil || generation != 0 {
				t.Fatalf("new account on other replica: generation = %d, err = %v, want 0, nil", generation, err)
			}
			if _, err := first.appendUserEvent(ctx, user.Id, newEvent(SystemActorID, revoke(user.Id)), "", nil); err != nil {
				t.Fatalf("append event: %v", err)
			}
			if _, err := second.CurrentAuthGeneration(ctx, user.Id); !errors.Is(err, ErrAuthenticationRevoked) {
				t.Fatalf("after event on other replica: err = %v, want ErrAuthenticationRevoked", err)
			}
		})
	}
}

// Every validation of a runtime credential runs the auth generation check, so
// its cost in requests to the stream leader matters. It reads one position
// for each generation-relevant subject of the user and no subject of another
// user.
func TestCurrentAuthGenerationReadsOnlySubjectsOfTheUser(t *testing.T) {
	core, nc := setupTestCore(t)
	userID := createGenerationZeroPasswordUser(t, core, "read-budget-user")
	createGenerationZeroPasswordUser(t, core, "read-budget-other-user")

	// Background checks also read stream positions, with wildcard subjects.
	// Count only the requests that name this user.
	requests := make(chan string, 64)
	sub, err := nc.Subscribe("$JS.API.STREAM.MSG.GET.EVT", func(msg *nats.Msg) {
		var request struct {
			LastBySubject string `json:"last_by_subj"`
		}
		if err := json.Unmarshal(msg.Data, &request); err == nil && strings.Contains(request.LastBySubject, userID) {
			requests <- request.LastBySubject
		}
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	mustCurrentAuthGeneration(t, core, userID)
	// The subscription is on another connection than the core. A request to
	// the server from this connection passes the pending deliveries.
	if err := sub.Unsubscribe(); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	close(requests)
	var got []string
	for subject := range requests {
		got = append(got, subject)
	}
	want := userAuthGenerationSubjects(userID)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("position reads for the user = %v, want one for each of %v", got, want)
	}
}
