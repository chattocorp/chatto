package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

const (
	testRefreshRequestIDA = "00000000-0000-4000-8000-000000000001"
	testRefreshRequestIDB = "00000000-0000-4000-8000-000000000002"
	testRefreshRequestIDC = "00000000-0000-4000-8000-000000000003"
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
	sessionID, _, ok := first.parseRefreshToken(initial.RefreshToken)
	if !ok {
		t.Fatal("initial refresh credential did not parse")
	}
	sessionKey := first.renewableSessionKey(sessionID)
	assertRuntimeKVHasTTL(t, first, sessionKey)

	second, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore replica: %v", err)
	}
	startCoreServices(t, second)

	const requestID = testRefreshRequestIDA
	rotated, err := first.RefreshBearerSession(ctx, initial.RefreshToken, requestID, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	assertRuntimeKVHasTTL(t, first, sessionKey)
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
	if err := first.RevokeRefreshTokenWithReason(ctx, recovered.RefreshToken, "test"); err != nil {
		t.Fatalf("RevokeRefreshTokenWithReason: %v", err)
	}
	if _, err := first.storage.runtimeStateKV.Get(ctx, sessionKey); !isRuntimeStateKeyAbsent(err) {
		t.Fatalf("revoked renewable session lookup error = %v, want absent key", err)
	}
}

func TestChattoCore_RefreshBearerSessionRejectsReusedRequestIDForNewRotation(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "refresh-request-id-user", "Refresh Request ID User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if _, err := chattoCore.RefreshBearerSession(ctx, rotated.RefreshToken, testRefreshRequestIDA, ""); !errors.Is(err, ErrRefreshRequestIDInvalid) {
		t.Fatalf("reused request ID error = %v, want ErrRefreshRequestIDInvalid", err)
	}
}

func TestChattoCore_RefreshBearerSessionRequiresUUIDv4RecoveryNonce(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "refresh-nonce-user", "Refresh Nonce User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	for _, requestID := range []string{
		"predictable-request-id",
		"00000000-0000-3000-8000-000000000001",
		"00000000-0000-4000-7000-000000000001",
		"00000000-0000-4000-8000-000000000001\n",
	} {
		if _, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, requestID, ""); !errors.Is(err, ErrRefreshRequestIDInvalid) {
			t.Fatalf("request ID %q error = %v, want ErrRefreshRequestIDInvalid", requestID, err)
		}
	}
}

func TestChattoCore_RefreshBearerSessionStoresOnlyRecoveryNonceVerifier(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "refresh-verifier-user", "Refresh Verifier User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	if _, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, ""); err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	sessionID, _, ok := chattoCore.parseRefreshToken(initial.RefreshToken)
	if !ok {
		t.Fatal("initial refresh credential did not parse")
	}
	session, entry, err := chattoCore.loadRenewableSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("loadRenewableSession: %v", err)
	}
	if session.LastRefreshRequestVerifier != chattoCore.refreshRequestVerifier(testRefreshRequestIDA) {
		t.Fatalf("stored recovery verifier = %q", session.LastRefreshRequestVerifier)
	}
	if bytes.Contains(entry.Value(), []byte(testRefreshRequestIDA)) {
		t.Fatal("renewable session stored the raw recovery nonce")
	}
}

func TestChattoCore_RefreshBearerSessionRenewsActiveSessionWindow(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthTokenTTL = 4 * time.Hour
	chattoCore.config.AuthAccessTokenTTL = time.Hour
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "renew-window-user", "Renew Window User", "password123")
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
	session, entry, err := chattoCore.loadRenewableSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("loadRenewableSession: %v", err)
	}
	session.ExpiresAt = now.Add(30 * time.Minute)
	value, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal near-expiry session: %v", err)
	}
	if _, err := chattoCore.updateRuntimeStateUntil(ctx, entry.Key(), value, entry.Revision(), session.ExpiresAt, now); err != nil {
		t.Fatalf("store near-expiry session: %v", err)
	}

	rotated, err := chattoCore.refreshBearerSessionAt(
		ctx,
		initial.RefreshToken,
		testRefreshRequestIDA,
		"",
		now,
	)
	if err != nil {
		t.Fatalf("refreshBearerSessionAt: %v", err)
	}
	wantExpiry := now.Add(chattoCore.renewableSessionTTL())
	if !rotated.SessionExpiresAt.Equal(wantExpiry) {
		t.Fatalf("renewed session expiry = %v, want %v", rotated.SessionExpiresAt, wantExpiry)
	}
	stored, _, err := chattoCore.loadRenewableSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load renewed session: %v", err)
	}
	if !stored.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("stored renewed expiry = %v, want %v", stored.ExpiresAt, wantExpiry)
	}
	assertRuntimeKVHasTTL(t, chattoCore, chattoCore.renewableSessionKey(sessionID))

	recovered, err := chattoCore.refreshBearerSessionAt(
		ctx,
		initial.RefreshToken,
		testRefreshRequestIDA,
		"",
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("recover renewed session response: %v", err)
	}
	if recovered.AccessToken != rotated.AccessToken ||
		recovered.RefreshToken != rotated.RefreshToken ||
		!recovered.SessionExpiresAt.Equal(rotated.SessionExpiresAt) {
		t.Fatalf("recovered credentials differ from renewed rotation")
	}
}

func TestChattoCore_RefreshBearerSessionKeepsCurrentWindowBeforeRenewalQuarter(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "keep-window-user", "Keep Window User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}

	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}
	if !rotated.SessionExpiresAt.Equal(initial.SessionExpiresAt) {
		t.Fatalf("ordinary refresh changed session expiry from %v to %v", initial.SessionExpiresAt, rotated.SessionExpiresAt)
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
	session.LastRefreshRequestVerifier = chattoCore.refreshRequestVerifier(testRefreshRequestIDA)
	session.LastRotatedAt = now
	value, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal committed session: %v", err)
	}
	if _, err := chattoCore.updateRuntimeStateUntil(
		ctx,
		chattoCore.renewableSessionKey(sessionID),
		value,
		entry.Revision(),
		session.ExpiresAt,
		now,
	); err != nil {
		t.Fatalf("commit rotation without access record: %v", err)
	}

	recovered, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession recovery: %v", err)
	}
	if got, err := chattoCore.ValidateAuthToken(ctx, recovered.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken recovered = %q, %v", got, err)
	}
}

func TestChattoCore_RefreshRetryKeepsOriginalAccessExpiryAndPhysicalTTL(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	chattoCore.config.AuthAccessTokenTTL = time.Minute
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "rotation-ttl-user", "Rotation TTL User", "password123")
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
	issuedAt := now.Add(-30 * time.Second)
	session, entry, err := chattoCore.validateRenewableSession(ctx, sessionID, now)
	if err != nil {
		t.Fatalf("validateRenewableSession: %v", err)
	}
	session.CurrentGeneration++
	session.LastRefreshRequestVerifier = chattoCore.refreshRequestVerifier(testRefreshRequestIDA)
	session.LastRotatedAt = issuedAt
	value, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal committed session: %v", err)
	}
	if _, err := chattoCore.updateRuntimeStateUntil(ctx, entry.Key(), value, entry.Revision(), session.ExpiresAt, now); err != nil {
		t.Fatalf("commit rotation without access record: %v", err)
	}

	recovered, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now)
	if err != nil {
		t.Fatalf("recover access record: %v", err)
	}
	wantExpiry := issuedAt.Add(time.Minute)
	if !recovered.AccessTokenExpiresAt.Equal(wantExpiry) {
		t.Fatalf("recovered access expiry = %v, want %v", recovered.AccessTokenExpiresAt, wantExpiry)
	}
	accessEntry, err := chattoCore.storage.runtimeStateKV.Get(ctx, chattoCore.authTokenKey(recovered.AccessToken))
	if err != nil {
		t.Fatalf("get recovered access record: %v", err)
	}
	stream, err := chattoCore.js.Stream(ctx, "KV_RUNTIME_STATE")
	if err != nil {
		t.Fatalf("get RUNTIME_STATE stream: %v", err)
	}
	msg, err := stream.GetMsg(ctx, accessEntry.Revision())
	if err != nil {
		t.Fatalf("get recovered access message: %v", err)
	}
	physicalTTL, err := time.ParseDuration(msg.Header.Get("Nats-TTL"))
	if err != nil {
		t.Fatalf("parse recovered access TTL: %v", err)
	}
	if physicalTTL > 31*time.Second || physicalTTL < 29*time.Second {
		t.Fatalf("recovered physical TTL = %v, want about 30s", physicalTTL)
	}
}

func TestChattoCore_RefreshRetrySurvivesFreshAuthMetadataChange(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "refresh-fresh-retry-user", "Refresh Fresh Retry User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "unknown")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	now := time.Now()
	rotated, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now)
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if err := chattoCore.MarkBearerTokenFresh(ctx, rotated.AccessToken, "password", "current_password"); err != nil {
		t.Fatalf("MarkBearerTokenFresh: %v", err)
	}
	recovered, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now.Add(time.Second))
	if err != nil {
		t.Fatalf("recover after fresh-auth change: %v", err)
	}
	if recovered.AccessToken != rotated.AccessToken || recovered.RefreshToken != rotated.RefreshToken {
		t.Fatal("fresh-auth change altered deterministic recovery credentials")
	}
}

func TestChattoCore_RefreshRetryAllowsSmallReplicaClockSkewWithoutRevocation(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "refresh-skew-user", "Refresh Skew User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	now := time.Now()
	rotated, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now)
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	recovered, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now.Add(-time.Second))
	if err != nil {
		t.Fatalf("recover with small negative skew: %v", err)
	}
	if recovered.AccessToken != rotated.AccessToken || recovered.RefreshToken != rotated.RefreshToken {
		t.Fatal("clock-skew recovery returned different credentials")
	}

	if _, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now.Add(-2*time.Minute)); err == nil || errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("large negative skew error = %v, want retryable non-reuse error", err)
	}
	if _, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now.Add(time.Second)); err != nil {
		t.Fatalf("large clock-skew attempt revoked session: %v", err)
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
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}

	if _, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDB, ""); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("stale refresh error = %v, want ErrRefreshTokenReused", err)
	}
	if _, err := chattoCore.ValidateAuthToken(ctx, rotated.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("rotated access after session revoke error = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := chattoCore.RefreshBearerSession(ctx, rotated.RefreshToken, testRefreshRequestIDC, ""); !errors.Is(err, ErrRefreshTokenNotFound) {
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
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
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
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
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
				[]string{testRefreshRequestIDA, testRefreshRequestIDB}[index],
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
	if _, err := first.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDC, ""); !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("initial refresh after detected concurrent reuse = %v, want ErrRefreshTokenNotFound", err)
	}
}

func TestChattoCore_RefreshBearerSessionRejectsExpiredWindow(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "expired-window-user", "Expired Window User", "password123")
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
		testRefreshRequestIDA,
		"",
		initial.SessionExpiresAt.Add(time.Second),
	)
	if !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("refresh after window expiry error = %v, want ErrRefreshTokenNotFound", err)
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
	rotated, err := chattoCore.refreshBearerSessionAt(ctx, initial.RefreshToken, testRefreshRequestIDA, "", now)
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	_, err = chattoCore.refreshBearerSessionAt(
		ctx,
		initial.RefreshToken,
		testRefreshRequestIDA,
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
