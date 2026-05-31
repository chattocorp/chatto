package core

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/encoding/protojson"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_RegistrationTokenAuditEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := WithAuditRequestMetadata(testContext(t), &corev1.AuditRequestMetadata{
		UserAgent: "audit-test-agent",
		IpHash:    "hashed-ip",
	})

	email := "registration-audit@example.com"
	token, err := core.CreateRegistrationToken(ctx, email)
	if err != nil {
		t.Fatalf("CreateRegistrationToken: %v", err)
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.AuthAggregate().Subject(events.EventRegistrationLinkIssued))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected 1 registration audit event, got %d", len(published))
	}
	payload := published[0].GetRegistrationLinkIssued()
	if payload == nil {
		t.Fatalf("expected RegistrationLinkIssued payload")
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
	if strings.Contains(string(jsonPayload), email) || strings.Contains(string(jsonPayload), token) {
		t.Fatalf("audit payload leaked raw email or token: %s", jsonPayload)
	}
}

func TestChattoCore_EmailVerificationTokenAuditEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "email-audit-user", "Email Audit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	email := "verify-audit@example.com"
	token, err := core.CreateEmailVerificationToken(ctx, user.Id, email)
	if err != nil {
		t.Fatalf("CreateEmailVerificationToken: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventEmailVerificationLinkIssued))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected 1 email verification audit event, got %d", len(published))
	}
	payload := published[0].GetEmailVerificationLinkIssued()
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

func TestChattoCore_AuditAppendFailureCleansNewRegistrationToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	core.EventPublisher = nil

	token, err := core.CreateRegistrationToken(ctx, "audit-failure@example.com")
	if err == nil {
		t.Fatalf("expected audit append failure")
	}
	if token != "" {
		t.Fatalf("expected no token on failure, got %q", token)
	}
	count, err := countKVKeys(ctx, core.storage.serverKV, "registration.*")
	if err != nil {
		t.Fatalf("count registration keys: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected failed issuance to clean token, found %d registration keys", count)
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
