package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/email"
	"github.com/chattocorp/chatto/authling/internal/storage"
	"github.com/nats-io/nats.go/jetstream"
)

// queuedTestRuntime uses the production composition, including mail handoff and
// workers. Workflow tests elsewhere inject acceptance at the service seam.
func queuedTestRuntime(t *testing.T, cfg config.Config, sender email.Sender) (*Runtime, context.CancelFunc, <-chan error) {
	t.Helper()
	r, err := newRuntime(t.Context(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), sender)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	if err := r.WaitReady(t.Context()); err != nil {
		cancel()
		<-done
		r.Close()
		t.Fatal(err)
	}
	return r, cancel, done
}

func waitMail(t *testing.T, sender *capturingSender, count int) email.Message {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if sender.count() >= count {
			return sender.all()[count-1]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("mail was not delivered")
	return email.Message{}
}

func TestQueuedMailCoversAllWorkflows(t *testing.T) {
	sender := &capturingSender{}
	r, cancel, done := queuedTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, r, cancel, done)
	flow, err := r.Registration.Start(t.Context(), "queued@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(waitMail(t, sender, 1).Body)
	if err := r.Registration.Verify(t.Context(), flow, code); err != nil {
		t.Fatal(err)
	}
	const password = "a long queued mail test password"
	account, err := r.Registration.Complete(t.Context(), flow, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.PasswordReset.Start(t.Context(), "queued@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if got := waitMail(t, sender, 2).Subject; !strings.Contains(got, "password reset code") {
		t.Fatal(got)
	}
	flow, err = r.EmailChange.Start(t.Context(), account.ID, password, "changed@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	code = regexp.MustCompile(`\b[0-9]{6}\b`).FindString(waitMail(t, sender, 3).Body)
	if err := r.EmailChange.Verify(t.Context(), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	completion, err := r.EmailChange.Complete(t.Context(), account.ID, flow)
	if err != nil || completion.OldAddressNotificationFailed {
		t.Fatalf("complete: %v", err)
	}
	notice := waitMail(t, sender, 4)
	if notice.To != "queued@example.invalid" || !strings.Contains(notice.Subject, "email address changed") {
		t.Fatal("wrong security notice")
	}
}

type unavailableMail struct{ calls atomic.Int32 }

func (s *unavailableMail) SendContext(context.Context, email.Message) error {
	s.calls.Add(1)
	return errors.New("SMTP rejected private@example.invalid code 654321")
}

func TestQueuedMailRestartPrivacyAndExpiry(t *testing.T) {
	cfg := embeddedTestConfig(t)
	unavailable := &unavailableMail{}
	r, cancel, done := queuedTestRuntime(t, cfg, unavailable)
	flow, err := r.Registration.Start(t.Context(), "private@example.invalid")
	if err != nil {
		t.Fatal("SMTP outage must not reject queue acceptance:", err)
	}
	// Stop before or after an unsuccessful attempt. Accepted work must survive both.
	stopTestRuntime(t, r, cancel, done)
	sender := &capturingSender{}
	// Construction opens durable storage but does not start the mail worker.
	r, err = newRuntime(t.Context(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), sender)
	if err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.New(r.connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(t.Context(), storage.JobStreamName)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		fact, err := stream.GetMsg(t.Context(), seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"private@example.invalid", flow, "654321"} {
			if strings.Contains(string(fact.Data), secret) {
				t.Fatal("job facts exposed mail or transport error")
			}
		}
	}
	// Inspect the runtime bytes before delivery. Neither storage surface may
	// expose the address or code in plaintext.
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := stores.RuntimeState.Keys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var storedContent [][]byte
	for _, key := range keys {
		if !strings.HasPrefix(key, "email-delivery.") {
			continue
		}
		entry, err := stores.RuntimeState.Get(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		storedContent = append(storedContent, entry.Value())
		if strings.Contains(string(entry.Value()), "private@example.invalid") {
			t.Fatal("plaintext recipient in runtime state")
		}
	}
	if len(storedContent) != 1 {
		t.Fatal("expected encrypted pending mail")
	}
	// Reduce only this test's crash redelivery wait; production keeps one minute.
	consumerInfo, err := r.mailConsumer.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	consumerConfig := consumerInfo.Config
	consumerConfig.AckWait = 100 * time.Millisecond
	r.mailConsumer, err = stream.UpdateConsumer(t.Context(), consumerConfig)
	if err != nil {
		t.Fatal(err)
	}
	// A second queued message expires while the worker is offline.
	if err := r.mailSender.SendContext(t.Context(), email.Message{To: "expired@example.invalid", Subject: "expired", Body: "expired secret", ExpiresAt: time.Now().Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2100 * time.Millisecond)
	runCtx, runCancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() { runDone <- r.Run(runCtx) }()
	defer stopTestRuntime(t, r, runCancel, runDone)
	if err := r.WaitReady(t.Context()); err != nil {
		t.Fatal(err)
	}
	message := waitMail(t, sender, 1)
	if message.To != "private@example.invalid" {
		t.Fatal("expired mail was delivered")
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(message.Body)
	if err := r.Registration.Verify(t.Context(), flow, code); err != nil {
		t.Fatal("recovered code invalid:", err)
	}
	for _, data := range storedContent {
		if strings.Contains(string(data), code) {
			t.Fatal("plaintext code in runtime state")
		}
	}
	// Wait for both accepted jobs to leave the durable consumer's pending set.
	deadline := time.Now().Add(3 * time.Second)
	drained := false
	for time.Now().Before(deadline) {
		ci, err := r.mailConsumer.Info(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if ci.NumAckPending == 0 && ci.NumPending == 0 {
			drained = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !drained {
		t.Fatal("mail consumer did not finish accepted jobs")
	}
	if sender.count() != 1 {
		t.Fatal("expired job reached transport")
	}
}

func TestQueuedMailAcceptanceFailureRollsBackWorkflow(t *testing.T) {
	sender := &capturingSender{}
	r, cancel, done := queuedTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, r, cancel, done)
	js, err := jetstream.New(r.connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(t.Context(), storage.JobStreamName)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// A sealed stream still permits reads but rejects queue acceptance. This
	// models a storage rejection without changing the production sender seam.
	cfg := info.Config
	cfg.Sealed = true
	if _, err := js.UpdateStream(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Registration.Start(t.Context(), "rejected@example.invalid"); err == nil {
		t.Fatal("queue failure accepted a workflow")
	}
	if sender.count() != 0 {
		t.Fatal("queue failure reached SMTP")
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := stores.RuntimeState.Keys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "signup.") || strings.HasPrefix(key, "signup-limit.") {
			t.Fatal("rejected handoff retained signup state or delivery budget")
		}
	}
}
