package core

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/chattocorp/chatto/cli/internal/email"
	"github.com/chattocorp/chatto/cli/internal/encryption"
	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const jobTypeEmail = "email"

// EmailJobs stores encrypted, expiring messages and runs the email job worker.
// SendContext confirms durable queue acceptance, not transport delivery. A crash
// after transport acceptance can cause duplicate delivery.
type EmailJobs struct {
	core   *ChattoCore
	sender email.Sender
}

// NewEmailJobs binds the HTTP runtime's transport to the core-owned queue.
func (c *ChattoCore) NewEmailJobs(sender email.Sender) *EmailJobs {
	return &EmailJobs{core: c, sender: sender}
}

// IsEnabled reports whether the configured transport can accept mail.
func (m *EmailJobs) IsEnabled() bool { return m.sender.IsEnabled() }

// Send accepts a message without a request cancellation context.
func (m *EmailJobs) Send(msg email.Message) error { return m.SendContext(context.Background(), msg) }

func emailDeliveryKey(id string) string { return "email_delivery." + id }
func (m *EmailJobs) key() []byte {
	mac := hmac.New(sha256.New, []byte(m.core.config.SecretKey))
	_, _ = mac.Write([]byte("chatto:email-delivery:v1"))
	return mac.Sum(nil)
}

// SendContext encrypts content before publishing a job with only an opaque ID.
// A failed or ambiguous handoff leaves at most an expiring encrypted record.
func (m *EmailJobs) SendContext(ctx context.Context, msg email.Message) error {
	if !m.IsEnabled() {
		return email.ErrEmailDisabled
	}
	if !msg.ExpiresAt.After(time.Now()) {
		return errors.New("email delivery deadline required")
	}
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return err
	}
	id := hex.EncodeToString(idBytes)
	key := emailDeliveryKey(id)
	plain, err := proto.Marshal(&runtimestatev1.EmailDelivery{To: msg.To, FromName: msg.FromName, Subject: msg.Subject, Body: msg.Body, HtmlBody: msg.HTMLBody})
	if err != nil {
		return err
	}
	defer clear(plain)
	sealed, err := encryption.EncryptXChaCha20Poly1305(m.key(), plain, []byte(key))
	if err != nil {
		return err
	}
	data, err := proto.Marshal(&runtimestatev1.SealedEmailDelivery{Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	if err != nil {
		return err
	}
	if _, err := m.core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(time.Until(msg.ExpiresAt))); err != nil {
		return err
	}
	return m.core.jobs.Enqueue(ctx, jobs.Spec{ID: id, Type: jobTypeEmail, MaxAttempts: jobs.UnlimitedAttempts, RetryDelay: 5 * time.Second, ExpiresAt: msg.ExpiresAt})
}

// Run owns a bounded worker pool. Shutdown cancels transport calls and leaves
// unfinished deliveries to the durable consumer for another process.
func (m *EmailJobs) Run(ctx context.Context) error {
	if err := m.core.WaitForBoot(ctx); err != nil {
		return err
	}
	return m.core.jobs.Work(ctx, m.core.jobConsumers[jobTypeEmail], m.deliver, 8)
}

func (m *EmailJobs) deliver(ctx context.Context, job jobs.Job) error {
	if job.Expired(time.Now()) {
		return &jobs.Failure{Category: "expired", Permanent: true}
	}
	key := emailDeliveryKey(job.ID)
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
	// Successful delivery removes content before the job acknowledgement. A
	// repeated attempt after that removal is complete too.
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	sealed := new(runtimestatev1.SealedEmailDelivery)
	if err := proto.Unmarshal(entry.Value(), sealed); err != nil {
		return &jobs.Failure{Category: "invalid_content", Permanent: true}
	}
	plain, err := encryption.DecryptXChaCha20Poly1305(m.key(), sealed.Ciphertext, sealed.Nonce, []byte(key))
	if err != nil {
		return &jobs.Failure{Category: "content_unavailable"}
	}
	defer clear(plain)
	message := new(runtimestatev1.EmailDelivery)
	if err := proto.Unmarshal(plain, message); err != nil {
		return &jobs.Failure{Category: "invalid_content", Permanent: true}
	}
	ctx, cancelAttempt := context.WithTimeout(ctx, 30*time.Second)
	defer cancelAttempt()
	attemptCtx, cancel := context.WithDeadline(ctx, job.ExpiresAt)
	defer cancel()
	if err := m.sender.SendContext(attemptCtx, email.Message{To: message.To, FromName: message.FromName, Subject: message.Subject, Body: message.Body, HTMLBody: message.HtmlBody}); err != nil {
		return &jobs.Failure{Category: "delivery_failed"}
	}
	// Transport acceptance must survive normal shutdown or an attempt timeout.
	// Bound cleanup separately so cancellation cannot leave delivered content
	// available for another send. A crash or storage failure can still duplicate it.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelCleanup()
	return m.core.storage.runtimeStateKV.Delete(cleanupCtx, key, jetstream.LastRevision(entry.Revision()))
}
