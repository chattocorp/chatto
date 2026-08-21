package core

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

func TestChattoCore_RefreshBearerSessionRotatesAndRecoversLostResponse(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "renewable-user", "Renewable User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := first.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore replica: %v", err)
	}
	startCoreServices(t, second)

	const requestID = "lost-response-request-0001"
	rotated, err := first.RefreshBearerSession(ctx, initial.RefreshToken, requestID, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	recovered, err := second.RefreshBearerSession(ctx, initial.RefreshToken, requestID, "")
	if err != nil {
		t.Fatalf("same-request recovery on replica: %v", err)
	}
	if recovered.AccessToken != rotated.AccessToken || recovered.RefreshToken != rotated.RefreshToken {
		t.Fatalf("recovered credentials differ from committed rotation")
	}
	if got, err := second.ValidateAuthToken(ctx, recovered.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken recovered = %q, %v", got, err)
	}
}

func TestChattoCore_RefreshRetryRepairsAccessRecordAfterCommittedRotation(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "rotation-gap-user", "Rotation Gap User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	sessionID, _, ok := chattoCore.parseRefreshToken(initial.RefreshToken)
	if !ok {
		t.Fatal("initial refresh credential did not parse")
	}
	now := time.Now()
	session, entry, err := chattoCore.validateRenewableSession(ctx, sessionID, now)
	if err != nil {
		t.Fatalf("validateRenewableSession: %v", err)
	}
	session.CurrentGeneration++
	session.LastRefreshRequestID = "committed-before-process-exit"
	session.LastRotatedAt = now
	value, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal committed session: %v", err)
	}
	if _, err := chattoCore.updateRuntimeStateTokenTTL(
		ctx,
		chattoCore.renewableSessionKey(sessionID),
		value,
		entry.Revision(),
		session.ExpiresAt.Sub(now),
	); err != nil {
		t.Fatalf("commit rotation without access record: %v", err)
	}

	recovered, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, session.LastRefreshRequestID, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession recovery: %v", err)
	}
	if got, err := chattoCore.ValidateAuthToken(ctx, recovered.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken recovered = %q, %v", got, err)
	}
}

func TestChattoCore_RefreshBearerSessionReuseRevokesSession(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "reuse-user", "Reuse User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, "first-rotation-request", "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}

	if _, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, "different-request-id", ""); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("stale refresh error = %v, want ErrRefreshTokenReused", err)
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, rotated.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("rotated access after session revoke error = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := chattoCore.RefreshBearerSession(ctx, rotated.RefreshToken, "later-refresh-request", ""); !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("latest refresh after session revoke error = %v, want ErrRefreshTokenNotFound", err)
	}
}

func TestChattoCore_AccessExpiryDoesNotEndRenewableSession(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthAccessTokenTTL = time.Second
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "access-expiry-user", "Access Expiry User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	if _, err := chattoCore.ValidateAuthToken(ctx, initial.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("expired access validation error = %v, want ErrAuthTokenNotFound", err)
	}
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, "refresh-after-access-expiry", "")
	if err != nil {
		t.Fatalf("RefreshBearerSession after access expiry: %v", err)
	}
	if rotated.AccessToken == initial.AccessToken {
		t.Fatal("refresh returned the expired access token")
	}
}

func TestChattoCore_FreshAuthSurvivesConcurrentAccessRotation(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "fresh-rotation-user", "Fresh Rotation User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "unknown")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, "rotate-before-fresh-auth", "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	if err := chattoCore.MarkBearerTokenFresh(ctx, initial.AccessToken, "password", "current_password"); err != nil {
		t.Fatalf("MarkBearerTokenFresh: %v", err)
	}
	if err := chattoCore.RequireFreshAuthForBearerToken(ctx, rotated.AccessToken); err != nil {
		t.Fatalf("rotated access token did not adopt session fresh auth: %v", err)
	}
}

func TestChattoCore_ConcurrentRefreshAcrossReplicasFencesAndRevokesReuse(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "replica-refresh-user", "Replica Refresh User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := first.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore replica: %v", err)
	}
	startCoreServices(t, second)

	start := make(chan struct{})
	errorsByReplica := make([]error, 2)
	credentials := make([]BearerSessionCredentials, 2)
	var wait sync.WaitGroup
	for index, replica := range []*ChattoCore{first, second} {
		wait.Add(1)
		go func(index int, replica *ChattoCore) {
			defer wait.Done()
			<-start
			credentials[index], errorsByReplica[index] = replica.RefreshBearerSession(
				ctx,
				initial.RefreshToken,
				[]string{"parallel-request-a", "parallel-request-b"}[index],
				"",
			)
		}(index, replica)
	}
	close(start)
	wait.Wait()

	successes := 0
	reuses := 0
	for _, err := range errorsByReplica {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRefreshTokenReused), errors.Is(err, ErrRefreshTokenNotFound):
			reuses++
		default:
			t.Fatalf("parallel refresh error = %v", err)
		}
	}
	if successes > 1 || successes+reuses != 2 {
		t.Fatalf("parallel outcomes successes=%d reuses=%d, want at most one temporary winner and two terminal outcomes (%v)", successes, reuses, errorsByReplica)
	}
	for _, pair := range credentials {
		if pair.AccessToken == "" {
			continue
		}
		if _, err := first.ValidateAuthToken(ctx, pair.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
			t.Fatalf("winner remained valid after detected reuse: %v", err)
		}
	}
	if _, err := first.RefreshBearerSession(ctx, initial.RefreshToken, "post-race-request", ""); !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("initial refresh after detected concurrent reuse = %v, want ErrRefreshTokenNotFound", err)
	}
}

func TestChattoCore_RefreshBearerSessionHonorsAbsoluteExpiry(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "absolute-expiry-user", "Absolute Expiry User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	_, err = chattoCore.refreshBearerSessionAt(
		ctx,
		initial.RefreshToken,
		"after-absolute-expiry",
		"",
		initial.SessionExpiresAt.Add(time.Second),
	)
	if !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("refresh after absolute expiry error = %v, want ErrRefreshTokenNotFound", err)
	}
}

func TestChattoCore_LostResponseRecoveryEndsAtAccessExpiry(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthAccessTokenTTL = time.Minute
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "recovery-expiry-user", "Recovery Expiry User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	now := time.Now()
	rotated, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, "bounded-recovery-request", "", now)
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	_, err = chattoCore.refreshBearerSessionAt(
		ctx,
		initial.RefreshToken,
		"bounded-recovery-request",
		"",
		now.Add(chattoCore.bearerAccessTokenTTL()),
	)
	if !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("retry at access expiry error = %v, want ErrRefreshTokenReused", err)
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, rotated.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("rotated access after out-of-window retry = %v, want ErrAuthTokenNotFound", err)
	}
}
