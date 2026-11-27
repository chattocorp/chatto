package core

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/email"
	"github.com/chattocorp/chatto/cli/internal/encryption"
	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type retryMailSender struct {
	*email.MockSender
	calls atomic.Int32
}

func (s *retryMailSender) SendContext(ctx context.Context, msg email.Message) error {
	if s.calls.Add(1) == 1 {
		return errors.New("secret@example.invalid code 123456 SMTP reply")
	}
	return s.MockSender.SendContext(ctx, msg)
}

func testEmailBranding(name string) func() email.Branding {
	return func() email.Branding { return email.Branding{ServerName: name, ServerURL: "https://chat.example.test"} }
}

// storedEmailDelivery decrypts the stored record of a queued email.
func storedEmailDelivery(t *testing.T, ctx context.Context, m *EmailJobs, id string) *runtimestatev1.EmailDelivery {
	t.Helper()
	key := emailDeliveryKey(id)
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
	require.NoError(t, err)
	sealed := new(runtimestatev1.SealedEmailDelivery)
	require.NoError(t, proto.Unmarshal(entry.Value(), sealed))
	plain, err := encryption.DecryptXChaCha20Poly1305(m.key(), sealed.Ciphertext, sealed.Nonce, []byte(key))
	require.NoError(t, err)
	record := new(runtimestatev1.EmailDelivery)
	require.NoError(t, proto.Unmarshal(plain, record))
	return record
}

// sealEmailDelivery replaces the stored record of a queued email.
func sealEmailDelivery(t *testing.T, ctx context.Context, m *EmailJobs, id string, record *runtimestatev1.EmailDelivery) {
	t.Helper()
	key := emailDeliveryKey(id)
	plain, err := proto.Marshal(record)
	require.NoError(t, err)
	sealed, err := encryption.EncryptXChaCha20Poly1305(m.key(), plain, []byte(key))
	require.NoError(t, err)
	data, err := proto.Marshal(&runtimestatev1.SealedEmailDelivery{Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	require.NoError(t, err)
	_, err = m.core.storage.runtimeStateKV.Put(ctx, key, data)
	require.NoError(t, err)
}

func lastEmailJobID(t *testing.T, ctx context.Context, c *ChattoCore) string {
	t.Helper()
	info, err := c.storage.jobsStream.Info(ctx)
	require.NoError(t, err)
	fact, err := c.storage.jobsStream.GetMsg(ctx, info.State.LastSeq)
	require.NoError(t, err)
	return strings.Split(fact.Subject, ".")[2]
}

func TestEmailJobsPersistPrivateContentAndRetry(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	transport := &retryMailSender{MockSender: email.NewMockSender(true)}
	sender := c.NewEmailJobs(transport, testEmailBranding("Old name"))
	delivery := email.Delivery{To: "secret@example.invalid", Content: email.RegistrationCode{Code: "123456", TTL: time.Minute}, ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, sender.Enqueue(ctx, delivery))
	require.Zero(t, transport.calls.Load(), "request must not call transport")
	info, err := c.storage.jobsStream.Info(ctx)
	require.NoError(t, err)
	raw, err := c.storage.jobsStream.GetMsg(ctx, info.State.LastSeq)
	require.NoError(t, err)
	id := strings.Split(raw.Subject, ".")[2]
	entry, err := c.storage.runtimeStateKV.Get(ctx, emailDeliveryKey(id))
	require.NoError(t, err)
	for _, secret := range []string{delivery.To, "123456"} {
		require.NotContains(t, string(raw.Data), secret)
		require.NotContains(t, string(entry.Value()), secret)
	}
	// The record holds what the email says, never a rendered message.
	record := storedEmailDelivery(t, ctx, sender, id)
	require.Equal(t, "123456", record.GetRegistrationCode().GetCode())
	require.Equal(t, time.Minute, record.GetRegistrationCode().GetTtl().AsDuration())
	require.Empty(t, record.GetSubject())
	require.Empty(t, record.GetBody())
	// A fresh core and sender can recover the accepted job without request
	// memory. It renders with the branding of the time of sending.
	replica, err := NewChattoCore(ctx, c.nc, testCoreConfig())
	require.NoError(t, err)
	recovered := replica.NewEmailJobs(transport, testEmailBranding("New name"))
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- replica.jobs.Work(workCtx, replica.jobConsumers[jobTypeEmail], recovered.deliver, 2) }()
	defer func() { cancel(); <-done }()
	require.Eventually(t, func() bool { return len(transport.Messages()) == 1 }, 10*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(2), transport.calls.Load())
	delivered := transport.Messages()[0]
	require.Equal(t, "123456 is your code for New name", delivered.Subject)
	require.Equal(t, "New name", delivered.FromName)
	require.Contains(t, delivered.HTMLBody, "123456")
	require.Eventually(t, func() bool {
		_, err := c.storage.runtimeStateKV.Get(ctx, emailDeliveryKey(id))
		return errors.Is(err, jetstream.ErrKeyNotFound)
	}, time.Second, time.Millisecond)
	require.NoError(t, recovered.deliver(ctx, jobs.Job{ID: id, ExpiresAt: delivery.ExpiresAt}))
	require.Equal(t, int32(2), transport.calls.Load(), "ack replay must not resend removed content")
	info, err = c.storage.jobsStream.Info(ctx)
	require.NoError(t, err)
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		fact, err := c.storage.jobsStream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		require.NoError(t, err)
		require.NotContains(t, string(fact.Data), delivery.To)
		require.NotContains(t, string(fact.Data), "123456")
	}
}

func TestEmailJobsStoreEachContentKind(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	transport := email.NewMockSender(true)
	sender := c.NewEmailJobs(transport, testEmailBranding("Acme"))
	expiresAt := time.Now().Add(time.Minute)
	tests := []struct {
		content email.Content
		subject string
		has     string
	}{
		{email.RegistrationCode{Code: "111111", TTL: time.Minute}, "111111 is your code for Acme", "Welcome to Acme"},
		{email.EmailVerificationCode{Code: "222222", TTL: time.Minute}, "222222 is your code for Acme", "Verify your email address"},
		{email.PasswordReset{Token: "PRtoken", TTL: time.Hour}, "Reset your Acme password", "https://chat.example.test/reset-password?token=PRtoken"},
	}
	for _, tt := range tests {
		require.NoError(t, sender.Enqueue(ctx, email.Delivery{To: "kind@example.invalid", Content: tt.content, ExpiresAt: expiresAt}))
		require.NoError(t, sender.deliver(ctx, jobs.Job{ID: lastEmailJobID(t, ctx, c), ExpiresAt: expiresAt}))
		msg := transport.LastMessage()
		require.NotNil(t, msg)
		require.Equal(t, tt.subject, msg.Subject)
		require.Contains(t, msg.Body, tt.has)
	}
}

func TestEmailJobsSendRecordsFromOlderVersionsAsPlainText(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	transport := email.NewMockSender(true)
	sender := c.NewEmailJobs(transport, testEmailBranding("Acme"))
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, sender.Enqueue(ctx, email.Delivery{To: "old@example.invalid", Content: email.RegistrationCode{Code: "333333", TTL: time.Minute}, ExpiresAt: expiresAt}))
	id := lastEmailJobID(t, ctx, c)
	sealEmailDelivery(t, ctx, sender, id, &runtimestatev1.EmailDelivery{To: "old@example.invalid", Subject: "Old subject", Body: "Old body 333333"})
	require.NoError(t, sender.deliver(ctx, jobs.Job{ID: id, ExpiresAt: expiresAt}))
	require.Equal(t, []email.Message{{To: "old@example.invalid", Subject: "Old subject", Body: "Old body 333333"}}, transport.Messages())
}

func TestEmailJobsRejectExpiredMessages(t *testing.T) {
	c, _ := newTestCore(t)
	transport := email.NewMockSender(true)
	sender := c.NewEmailJobs(transport, testEmailBranding("Acme"))
	require.Error(t, sender.Enqueue(testContext(t), email.Delivery{To: "expired@example.invalid", Content: email.RegistrationCode{Code: "123456"}, ExpiresAt: time.Now().Add(-time.Second)}))
	require.Empty(t, transport.Messages())
}

func TestEmailJobsRejectMissingContent(t *testing.T) {
	c, _ := newTestCore(t)
	sender := c.NewEmailJobs(email.NewMockSender(true), testEmailBranding("Acme"))
	require.Error(t, sender.Enqueue(testContext(t), email.Delivery{To: "empty@example.invalid", ExpiresAt: time.Now().Add(time.Minute)}))
}

func TestEmailJobsFailClosedForExpiredOrCorruptContent(t *testing.T) {
	for _, scenario := range []string{"expired", "corrupt", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := newTestCore(t)
			ctx := testContext(t)
			transport := email.NewMockSender(true)
			sender := c.NewEmailJobs(transport, testEmailBranding("Acme"))
			expiresAt := time.Now().Add(time.Minute)
			require.NoError(t, sender.Enqueue(ctx, email.Delivery{To: "private@example.invalid", Content: email.RegistrationCode{Code: "123456", TTL: time.Minute}, ExpiresAt: expiresAt}))
			id := lastEmailJobID(t, ctx, c)
			switch scenario {
			case "corrupt":
				_, err := c.storage.runtimeStateKV.Put(ctx, emailDeliveryKey(id), []byte("corrupt"))
				require.NoError(t, err)
			case "empty":
				sealEmailDelivery(t, ctx, sender, id, &runtimestatev1.EmailDelivery{To: "private@example.invalid"})
			default:
				expiresAt = time.Now().Add(-time.Second)
			}
			err := sender.deliver(ctx, jobs.Job{ID: id, ExpiresAt: expiresAt})
			require.Error(t, err)
			require.Empty(t, transport.Messages())
			require.NotContains(t, err.Error(), "private@example.invalid")
		})
	}
}

// acceptingCanceledMail models transport acceptance followed immediately by
// shutdown, before the handler can remove its persisted content.
type acceptingCanceledMail struct {
	*email.MockSender
	cancel context.CancelFunc
}

func (s *acceptingCanceledMail) SendContext(ctx context.Context, msg email.Message) error {
	if err := s.MockSender.SendContext(ctx, msg); err != nil {
		return err
	}
	s.cancel()
	return nil
}

func TestEmailJobsRemoveAcceptedContentAfterCancellation(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	transport := &acceptingCanceledMail{MockSender: email.NewMockSender(true), cancel: cancel}
	sender := c.NewEmailJobs(transport, testEmailBranding("Acme"))
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, sender.Enqueue(ctx, email.Delivery{To: "accepted@example.invalid", Content: email.RegistrationCode{Code: "123456", TTL: time.Minute}, ExpiresAt: expiresAt}))
	job := jobs.Job{ID: lastEmailJobID(t, ctx, c), ExpiresAt: expiresAt}
	require.NoError(t, sender.deliver(attemptCtx, job))
	require.ErrorIs(t, attemptCtx.Err(), context.Canceled)
	_, err := c.storage.runtimeStateKV.Get(ctx, emailDeliveryKey(job.ID))
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
	require.NoError(t, sender.deliver(ctx, job))
	require.Len(t, transport.Messages(), 1, "redelivery after shutdown must not resend accepted mail")
}
