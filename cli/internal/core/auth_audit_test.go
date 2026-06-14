package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/encoding/protojson"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func countTestKVKeys(ctx context.Context, kv jetstream.KeyValue, filters ...string) (int, error) {
	lister, err := kv.ListKeysFiltered(ctx, filters...)
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for range lister.Keys() {
		count++
	}
	return count, nil
}

func TestChattoCore_RegistrationCodeAuditEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := WithAuditRequestMetadata(testContext(t), &corev1.AuditRequestMetadata{
		UserAgent: "audit-test-agent",
		IpHash:    "hashed-ip",
	})

	email := "registration-audit@example.com"
	code, err := core.CreateRegistrationCode(ctx, email)
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.AuthAggregate().Subject(events.EventRegistrationVerificationCodeIssued))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected 1 registration audit event, got %d", len(published))
	}
	payload := published[0].GetRegistrationVerificationCodeIssued()
	if payload == nil {
		t.Fatalf("expected RegistrationVerificationCodeIssued payload")
	}
	if payload.GetEmailHash() != emailHash(email) {
		t.Fatalf("email hash = %q, want %q", payload.GetEmailHash(), emailHash(email))
	}
	if payload.GetExpiresAt() == nil {
		t.Fatalf("expected expires_at")
	}
	if payload.GetRequest().GetUserAgent() != "audit-test-agent" || payload.GetRequest().GetIpHash() != "hashed-ip" {
		t.Fatalf("unexpected request metadata: %#v", payload.GetRequest())
	}

	jsonPayload, err := protojson.Marshal(published[0])
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	if strings.Contains(string(jsonPayload), email) || strings.Contains(string(jsonPayload), code) {
		t.Fatalf("audit payload leaked raw email or code: %s", jsonPayload)
	}
}

func TestChattoCore_EmailVerificationCodeAuditEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "email-audit-user", "Email Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	email := "verify-audit@example.com"
	code, err := core.CreateEmailVerificationCode(ctx, user.Id, email)
	if err != nil {
		t.Fatalf("CreateEmailVerificationCode: %v", err)
	}
	if code == "" {
		t.Fatalf("expected code")
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventEmailVerificationCodeIssued))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected 1 email verification audit event, got %d", len(published))
	}
	payload := published[0].GetEmailVerificationCodeIssued()
	if payload.GetUserId() != user.Id || payload.GetEmailHash() != emailHash(email) || payload.GetExpiresAt() == nil {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestChattoCore_PasswordResetAuditEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "password-audit-user", "Password Audit User", "oldpassword")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, user.Id, "password-audit@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}

	token, err := core.CreatePasswordResetToken(ctx, "password-audit@example.com")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}
	resetEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventPasswordResetLinkIssued))
	if err != nil {
		t.Fatalf("SubjectEvents reset link: %v", err)
	}
	if len(resetEvents) != 1 {
		t.Fatalf("expected 1 password reset link audit event, got %d", len(resetEvents))
	}
	if payload := resetEvents[0].GetPasswordResetLinkIssued(); payload.GetUserId() != user.Id || payload.GetEmailHash() != emailHash("password-audit@example.com") || payload.GetExpiresAt() == nil {
		t.Fatalf("unexpected reset link payload: %#v", payload)
	}

	unknownToken, err := core.CreatePasswordResetToken(ctx, "unknown-password-audit@example.com")
	if err != nil {
		t.Fatalf("unknown CreatePasswordResetToken: %v", err)
	}
	if unknownToken != "" {
		t.Fatalf("expected empty token for unknown email")
	}
	resetEventsAfterUnknown, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventPasswordResetLinkIssued))
	if err != nil {
		t.Fatalf("SubjectEvents after unknown: %v", err)
	}
	if len(resetEventsAfterUnknown) != len(resetEvents) {
		t.Fatalf("unknown email emitted audit event")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}
	if err := core.ResetPassword(ctx, token, string(newHash)); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	completedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventPasswordResetCompleted))
	if err != nil {
		t.Fatalf("SubjectEvents completed: %v", err)
	}
	if len(completedEvents) != 1 {
		t.Fatalf("expected 1 password reset completed audit event, got %d", len(completedEvents))
	}
	if completedEvents[0].GetPasswordResetCompleted().GetUserId() != user.Id {
		t.Fatalf("unexpected completed payload: %#v", completedEvents[0].GetPasswordResetCompleted())
	}
}

func TestChattoCore_AccountDeletionTokenAuditEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "delete-audit-user", "Delete Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, err := core.CreateAccountDeletionToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAccountDeletionToken: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventAccountDeletionConfirmationIssued))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected 1 account deletion audit event, got %d", len(published))
	}
	payload := published[0].GetAccountDeletionConfirmationIssued()
	if payload.GetUserId() != user.Id || payload.GetExpiresAt() == nil {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestChattoCore_LoginAndLogoutAuditEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "login-audit-user", "Login Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := core.RecordLoginSucceeded(ctx, user.Id, " Login-Audit-User "); err != nil {
		t.Fatalf("RecordLoginSucceeded: %v", err)
	}
	successEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventLoginSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents login success: %v", err)
	}
	if len(successEvents) != 1 {
		t.Fatalf("expected 1 login success audit event, got %d", len(successEvents))
	}
	success := successEvents[0].GetLoginSucceeded()
	if success.GetUserId() != user.Id || success.GetIdentifierHash() != auditIdentifierHash("login-audit-user") {
		t.Fatalf("unexpected login success payload: %#v", success)
	}

	if err := core.RecordLoginFailed(ctx, " missing-user@example.com "); err != nil {
		t.Fatalf("RecordLoginFailed: %v", err)
	}
	failedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AuthAggregate().Subject(events.EventLoginFailed))
	if err != nil {
		t.Fatalf("SubjectEvents login failed: %v", err)
	}
	if len(failedEvents) != 1 {
		t.Fatalf("expected 1 login failed audit event, got %d", len(failedEvents))
	}
	failed := failedEvents[0].GetLoginFailed()
	if failed.GetIdentifierHash() != auditIdentifierHash("missing-user@example.com") {
		t.Fatalf("unexpected login failed payload: %#v", failed)
	}
	jsonPayload, err := protojson.Marshal(failedEvents[0])
	if err != nil {
		t.Fatalf("marshal failed login event: %v", err)
	}
	if strings.Contains(string(jsonPayload), "missing-user@example.com") {
		t.Fatalf("failed-login audit payload leaked raw identifier: %s", jsonPayload)
	}

	if err := core.RecordLogoutSucceeded(ctx, user.Id); err != nil {
		t.Fatalf("RecordLogoutSucceeded: %v", err)
	}
	logoutEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventLogoutSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents logout: %v", err)
	}
	if len(logoutEvents) != 1 {
		t.Fatalf("expected 1 logout audit event, got %d", len(logoutEvents))
	}
	if logoutEvents[0].GetLogoutSucceeded().GetUserId() != user.Id {
		t.Fatalf("unexpected logout payload: %#v", logoutEvents[0].GetLogoutSucceeded())
	}
}

func TestChattoCore_BearerTokenAuditEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := WithAuditRequestMetadata(testContext(t), &corev1.AuditRequestMetadata{
		UserAgent: "bearer-audit-agent",
		IpHash:    "bearer-ip-hash",
	})
	user, err := core.CreateUser(ctx, SystemActorID, "bearer-audit-user", "Bearer Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token, err := core.CreateAuthTokenWithSource(ctx, user.Id, "test_source")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}

	issuedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventBearerTokenIssued))
	if err != nil {
		t.Fatalf("SubjectEvents bearer issued: %v", err)
	}
	if len(issuedEvents) != 1 {
		t.Fatalf("expected 1 bearer token issued event, got %d", len(issuedEvents))
	}
	issued := issuedEvents[0].GetBearerTokenIssued()
	if issued.GetUserId() != user.Id || issued.GetSource() != "test_source" || issued.GetExpiresAt() == nil {
		t.Fatalf("unexpected bearer token issued payload: %#v", issued)
	}
	if issued.GetRequest().GetUserAgent() != "bearer-audit-agent" || issued.GetRequest().GetIpHash() != "bearer-ip-hash" {
		t.Fatalf("unexpected bearer request metadata: %#v", issued.GetRequest())
	}

	if err := core.RevokeAuthTokenWithReason(ctx, token, "test_revoke"); err != nil {
		t.Fatalf("RevokeAuthTokenWithReason: %v", err)
	}
	revokedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventBearerTokenRevoked))
	if err != nil {
		t.Fatalf("SubjectEvents bearer revoked: %v", err)
	}
	if len(revokedEvents) != 1 {
		t.Fatalf("expected 1 bearer token revoked event, got %d", len(revokedEvents))
	}
	revoked := revokedEvents[0].GetBearerTokenRevoked()
	if revoked.GetUserId() != user.Id || revoked.GetReason() != "test_revoke" {
		t.Fatalf("unexpected bearer token revoked payload: %#v", revoked)
	}

	for _, event := range append(issuedEvents, revokedEvents...) {
		jsonPayload, err := protojson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal bearer audit event: %v", err)
		}
		if strings.Contains(string(jsonPayload), token) {
			t.Fatalf("bearer audit payload leaked raw token: %s", jsonPayload)
		}
	}
}

func TestChattoCore_AuthCodeAuditEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "auth-code-audit-user", "Auth Code Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	issuedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventAuthCodeIssued))
	if err != nil {
		t.Fatalf("SubjectEvents auth code issued: %v", err)
	}
	if len(issuedEvents) != 1 {
		t.Fatalf("expected 1 auth code issued event, got %d", len(issuedEvents))
	}
	issued := issuedEvents[0].GetAuthCodeIssued()
	if issued.GetUserId() != user.Id || issued.GetRedirectUriHash() != auditValueHash(redirectURI) || issued.GetExpiresAt() == nil {
		t.Fatalf("unexpected auth code issued payload: %#v", issued)
	}

	token, userID, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	if userID != user.Id {
		t.Fatalf("ExchangeAuthCode userID = %q, want %q", userID, user.Id)
	}

	successEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventAuthCodeExchangeSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents auth code exchange succeeded: %v", err)
	}
	if len(successEvents) != 1 {
		t.Fatalf("expected 1 auth code exchange success event, got %d", len(successEvents))
	}
	success := successEvents[0].GetAuthCodeExchangeSucceeded()
	if success.GetUserId() != user.Id || success.GetRedirectUriHash() != auditValueHash(redirectURI) {
		t.Fatalf("unexpected auth code exchange success payload: %#v", success)
	}

	bearerEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventBearerTokenIssued))
	if err != nil {
		t.Fatalf("SubjectEvents bearer issued: %v", err)
	}
	if len(bearerEvents) != 1 {
		t.Fatalf("expected 1 bearer token issued event, got %d", len(bearerEvents))
	}
	if bearerEvents[0].GetBearerTokenIssued().GetSource() != "oauth_code_exchange" {
		t.Fatalf("unexpected bearer source: %#v", bearerEvents[0].GetBearerTokenIssued())
	}

	for _, event := range append(append(issuedEvents, successEvents...), bearerEvents...) {
		jsonPayload, err := protojson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal auth code audit event: %v", err)
		}
		payload := string(jsonPayload)
		for _, forbidden := range []string{code, token, redirectURI, verifier, challenge} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("auth code audit payload leaked %q: %s", forbidden, payload)
			}
		}
	}
}

func TestChattoCore_AuthCodeExchangeFailureAuditEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "auth-code-failure-audit-user", "Auth Code Failure Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	_, _, err = core.ExchangeAuthCode(ctx, code, "wrong-verifier", redirectURI)
	if err != ErrAuthCodeInvalidVerifier {
		t.Fatalf("ExchangeAuthCode wrong verifier error = %v, want ErrAuthCodeInvalidVerifier", err)
	}

	failedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventAuthCodeExchangeFailed))
	if err != nil {
		t.Fatalf("SubjectEvents auth code exchange failed: %v", err)
	}
	if len(failedEvents) != 1 {
		t.Fatalf("expected 1 auth code exchange failure event, got %d", len(failedEvents))
	}
	failed := failedEvents[0].GetAuthCodeExchangeFailed()
	if failed.GetUserId() != user.Id || failed.GetRedirectUriHash() != auditValueHash(redirectURI) || failed.GetReason() != "invalid_verifier" {
		t.Fatalf("unexpected auth code exchange failure payload: %#v", failed)
	}

	_, _, err = core.ExchangeAuthCode(ctx, "cht_ACnonexistent1234", "verifier", redirectURI)
	if err != ErrAuthCodeNotFound {
		t.Fatalf("unknown ExchangeAuthCode error = %v, want ErrAuthCodeNotFound", err)
	}
	failedEventsAfterUnknown, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventAuthCodeExchangeFailed))
	if err != nil {
		t.Fatalf("SubjectEvents after unknown: %v", err)
	}
	if len(failedEventsAfterUnknown) != len(failedEvents) {
		t.Fatalf("unknown auth code emitted a failure audit event")
	}

	jsonPayload, err := protojson.Marshal(failedEvents[0])
	if err != nil {
		t.Fatalf("marshal auth code failure audit event: %v", err)
	}
	payload := string(jsonPayload)
	for _, forbidden := range []string{code, redirectURI, verifier, challenge, "wrong-verifier"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("auth code failure audit payload leaked %q: %s", forbidden, payload)
		}
	}
}

func TestChattoCore_AuditAppendFailureCleansNewRegistrationCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	core.EventPublisher = nil

	code, err := core.CreateRegistrationCode(ctx, "audit-failure@example.com")
	if err == nil {
		t.Fatalf("expected audit append failure")
	}
	if code != "" {
		t.Fatalf("expected no code on failure, got %q", code)
	}
	count, err := countTestKVKeys(ctx, core.storage.runtimeStateKV, "email_otp.*")
	if err != nil {
		t.Fatalf("count registration keys: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected failed issuance to clean code, found %d registration keys", count)
	}
}

func TestChattoCore_AuditAppendFailureCleansNewAuthRuntimeTokens(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "auth-runtime-cleanup-user", "Auth Runtime Cleanup User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	core.EventPublisher = nil
	if token, err := core.CreateAuthTokenWithSource(ctx, user.Id, "test_source"); err == nil {
		t.Fatalf("expected bearer token audit append failure")
	} else if token != "" {
		t.Fatalf("expected no bearer token on failure, got %q", token)
	}
	sessionCount, err := countTestKVKeys(ctx, core.storage.runtimeStateKV, "session.*")
	if err != nil {
		t.Fatalf("count session keys: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected failed bearer issuance to clean token, found %d session keys", sessionCount)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	if code, err := core.CreateAuthCode(ctx, user.Id, "https://example.com/callback", challenge, "S256"); err == nil {
		t.Fatalf("expected auth code audit append failure")
	} else if code != "" {
		t.Fatalf("expected no auth code on failure, got %q", code)
	}
	grantCount, err := countTestKVKeys(ctx, core.storage.runtimeStateKV, "grant.*")
	if err != nil {
		t.Fatalf("count grant keys: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("expected failed auth code issuance to clean code, found %d grant keys", grantCount)
	}
}

func TestChattoCore_BearerRevocationAuditFailureKeepsToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "revocation-audit-failure-user", "Revocation Audit Failure User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := core.CreateAuthTokenWithSource(ctx, user.Id, "test_source")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}

	core.EventPublisher = nil
	if err := core.RevokeAuthTokenWithReason(ctx, token, "test_revoke"); err == nil {
		t.Fatalf("expected revocation audit append failure")
	}

	userID, err := core.ValidateAuthToken(ctx, token)
	if err != nil {
		t.Fatalf("expected token to remain valid after failed revocation audit: %v", err)
	}
	if userID != user.Id {
		t.Fatalf("ValidateAuthToken userID = %q, want %q", userID, user.Id)
	}
}

func TestAuditRequestMetadataContextCopiesAndDefaults(t *testing.T) {
	ctx := testContext(t)
	if got := auditRequestMetadata(ctx); got == nil {
		t.Fatalf("missing metadata should produce empty metadata")
	} else if got.GetUserAgent() != "" || got.GetIpHash() != "" {
		t.Fatalf("empty metadata has fields: %#v", got)
	}

	metadata := &corev1.AuditRequestMetadata{UserAgent: "ua", IpHash: "ip"}
	ctx = WithAuditRequestMetadata(ctx, metadata)
	metadata.UserAgent = "mutated"
	got := AuditRequestMetadataFromContext(ctx)
	if got.GetUserAgent() != "ua" {
		t.Fatalf("metadata was not copied into context: %#v", got)
	}
	got.UserAgent = "mutated-again"
	if again := AuditRequestMetadataFromContext(ctx); again.GetUserAgent() != "ua" {
		t.Fatalf("metadata read was not copied: %#v", again)
	}
}
