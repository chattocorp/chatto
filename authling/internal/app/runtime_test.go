package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/registration"
)

func TestRuntimeCreatesAccountWithReadYourWrites(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)

	account, err := runtime.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if account.ID == "" {
		t.Fatal("created account ID is empty")
	}
	if account.CreatedAt.IsZero() {
		t.Fatal("created account timestamp is zero")
	}
	if got, ok := runtime.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("projected account = %+v, %v; want %+v, true", got, ok, account)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeAppliesConfiguredPasswordMinimumLength(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.Authentication.PasswordMinimumLength = 12
	runtime, cancel, runErrors := startTestRuntime(t, cfg)

	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "12345678901"); !errors.Is(err, accounts.ErrInvalidPassword) || err.Error() != "password must contain at least 12 characters and at most 1024 bytes" {
		t.Fatalf("eleven-character password error = %v, want configured policy error", err)
	}
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "123456789012"); err != nil {
		t.Fatalf("create account with twelve-character password: %v", err)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeRejectsCommonPasswords(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))

	for _, password := range []string{"password123", "Password123", "1234567890"} {
		if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", password); !errors.Is(err, accounts.ErrInvalidPassword) || err.Error() != "password is too common; choose a less predictable password" {
			t.Fatalf("CreateLocal password %q error = %v, want common-password policy error", password, err)
		}
	}
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "password123 is only part of this passphrase"); err != nil {
		t.Fatalf("create account with non-blocklisted passphrase: %v", err)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeReplaysAccountsAfterFullRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account before restart: %v", err)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg)
	if got := restarted.Accounts.Count(); got != 1 {
		t.Fatalf("replayed account count = %d, want 1", got)
	}
	if got, ok := restarted.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("replayed account = %+v, %v; want %+v, true", got, ok, account)
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
}

func TestVerifiedEmailRegistrationCreatesAccountOnlyAfterConfirmation(t *testing.T) {
	cfg := embeddedTestConfig(t)
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, cfg, sender)

	flow, err := runtime.Registration.Start(testContext(t), " Person@Example.com ")
	if err != nil {
		t.Fatalf("start registration: %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("account exists before email confirmation")
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if code == "" {
		t.Fatal("verification email has no six-digit code")
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "999999"
	}
	if err := runtime.Registration.Verify(testContext(t), flow, wrongCode); !errors.Is(err, registration.ErrInvalidCode) {
		t.Fatalf("wrong code error = %v, want ErrInvalidCode", err)
	}
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatalf("verify code: %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("account exists before password completion")
	}
	account, err := runtime.Registration.Complete(testContext(t), flow, "a long secure passphrase")
	if err != nil {
		t.Fatalf("complete registration: %v", err)
	}
	if account.ID == "" || runtime.Accounts.Count() != 1 {
		t.Fatalf("created account = %+v, count = %d", account, runtime.Accounts.Count())
	}
	authenticated, err := runtime.Authentication.Login(testContext(t), " PERSON@example.COM ", "a long secure passphrase")
	if err != nil || authenticated != account {
		t.Fatalf("authenticated account = %+v, error = %v; want %+v", authenticated, err, account)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "person@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("wrong-password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "absent@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("absent-account error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := runtime.Registration.Complete(testContext(t), flow, "a long secure passphrase"); !errors.Is(err, registration.ErrInvalidFlow) {
		t.Fatalf("reused flow error = %v, want ErrInvalidFlow", err)
	}
	stopTestRuntime(t, runtime, cancel, runErrors)

	restarted, cancelRestarted, restartErrors := startTestRuntime(t, cfg, sender)
	if !restarted.Accounts.HasEmail("person@example.com") {
		t.Fatal("replayed account does not claim normalized email")
	}
	messageCount := sender.count()
	duplicateFlow, err := restarted.Registration.Start(testContext(t), "person@example.com")
	if err != nil {
		t.Fatalf("start duplicate registration: %v", err)
	}
	if sender.count() != messageCount+1 {
		t.Fatal("duplicate registration did not follow the same email-delivery path")
	}
	duplicateCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := restarted.Registration.Verify(testContext(t), duplicateFlow, duplicateCode); err != nil {
		t.Fatalf("verify duplicate flow: %v", err)
	}
	if _, err := restarted.Registration.Complete(testContext(t), duplicateFlow, "another sufficiently long password"); !errors.Is(err, accounts.ErrEmailClaimed) {
		t.Fatalf("duplicate completion error = %v, want ErrEmailClaimed", err)
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartErrors)
}

func TestBrowserSessionSurvivesRestartAndCanBeRevoked(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	token, created, err := first.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := first.Sessions.Validate(testContext(t), token); err != nil || got != created {
		t.Fatalf("validated session = %+v, error = %v; want %+v", got, err, created)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg)
	if got, err := restarted.Sessions.Validate(testContext(t), token); err != nil || got.AccountID != account.ID {
		t.Fatalf("restarted session = %+v, error = %v; want account %q", got, err, account.ID)
	}
	if err := restarted.Sessions.Revoke(testContext(t), token); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Sessions.Validate(testContext(t), token); err == nil {
		t.Fatal("revoked session still validates")
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
}

func TestLoginThrottlesAfterTenFailedAttempts(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "limited@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Registration.Complete(testContext(t), flow, "a sufficiently long throttle password"); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := runtime.Authentication.Login(testContext(t), "limited@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
			t.Fatalf("failed login error = %v, want ErrInvalidCredentials", err)
		}
	}
	if _, err := runtime.Authentication.Login(testContext(t), "limited@example.com", "a sufficiently long throttle password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("throttled valid login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestRegistrationExhaustsFlowAfterFiveWrongCodes(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "attempts@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	wrong := "000000"
	if code == wrong {
		wrong = "999999"
	}
	for range 5 {
		if err := runtime.Registration.Verify(testContext(t), flow, wrong); !errors.Is(err, registration.ErrInvalidCode) {
			t.Fatalf("wrong-code error = %v", err)
		}
	}
	if err := runtime.Registration.Verify(testContext(t), flow, code); !errors.Is(err, registration.ErrInvalidCode) {
		t.Fatalf("exhausted flow error = %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("exhausted flow created an account")
	}
}

func TestConcurrentVerifiedFlowsCannotClaimSameEmailTwice(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)

	flows := make([]string, 2)
	for i := range flows {
		flow, err := runtime.Registration.Start(testContext(t), "race@example.com")
		if err != nil {
			t.Fatalf("start flow %d: %v", i, err)
		}
		flows[i] = flow
	}
	messages := sender.all()
	for i, flow := range flows {
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(messages[i].Body)
		if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
			t.Fatalf("verify flow %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	errorsByFlow := make(chan error, 2)
	for _, flow := range flows {
		go func() {
			<-start
			_, err := runtime.Registration.Complete(testContext(t), flow, "a sufficiently long race password")
			errorsByFlow <- err
		}()
	}
	close(start)
	var successes, claimed int
	for range 2 {
		err := <-errorsByFlow
		switch {
		case err == nil:
			successes++
		case errors.Is(err, accounts.ErrEmailClaimed):
			claimed++
		default:
			t.Fatalf("concurrent completion error = %v", err)
		}
	}
	if successes != 1 || claimed != 1 || runtime.Accounts.Count() != 1 {
		t.Fatalf("successes=%d claimed=%d accounts=%d, want 1/1/1", successes, claimed, runtime.Accounts.Count())
	}
}

func TestVerifiedFlowAllowsOnlyOneConcurrentCompletion(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "single-flow@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := runtime.Registration.Complete(testContext(t), flow, "one deliberately long completion password")
			results <- err
		}()
	}
	close(start)
	var successes, rejected int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, registration.ErrInvalidFlow) {
			rejected++
		} else {
			t.Fatalf("completion error = %v", err)
		}
	}
	if successes != 1 || rejected != 1 || runtime.Accounts.Count() != 1 {
		t.Fatalf("successes=%d rejected=%d accounts=%d", successes, rejected, runtime.Accounts.Count())
	}
}

func embeddedTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		NATS: config.NATSConfig{
			Embedded: config.EmbeddedNATSConfig{
				Enabled: true,
				DataDir: t.TempDir(),
			},
		},
	}
}

func startTestRuntime(
	t *testing.T,
	cfg config.Config,
	senders ...email.Sender,
) (*Runtime, context.CancelFunc, <-chan error) {
	t.Helper()
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var runtime *Runtime
	var err error
	if len(senders) == 0 {
		runtime, err = New(testContext(t), cfg, logger)
	} else {
		runtime, err = newRuntime(testContext(t), cfg, logger, senders[0])
	}
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() {
		runErrors <- runtime.Run(runContext)
	}()
	if err := runtime.WaitReady(testContext(t)); err != nil {
		cancel()
		<-runErrors
		runtime.Close()
		t.Fatalf("wait for runtime readiness: %v", err)
	}
	return runtime, cancel, runErrors
}

type capturingSender struct {
	mu       sync.Mutex
	messages []email.Message
}

func (s *capturingSender) SendContext(_ context.Context, message email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return nil
}
func (s *capturingSender) last() email.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[len(s.messages)-1]
}
func (s *capturingSender) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.messages) }
func (s *capturingSender) all() []email.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]email.Message(nil), s.messages...)
}

func stopTestRuntime(
	t *testing.T,
	runtime *Runtime,
	cancel context.CancelFunc,
	runErrors <-chan error,
) {
	t.Helper()
	cancel()
	select {
	case err := <-runErrors:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("runtime shutdown error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
