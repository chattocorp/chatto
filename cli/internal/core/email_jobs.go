package core

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/email"
	"github.com/chattocorp/chatto/cli/internal/encryption"
	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const jobTypeEmail = "email"

// EmailJobs stores encrypted, expiring email content and runs the email job
// worker. It stores what an email says, not the rendered message: the worker
// renders it with the branding of the time of sending. Enqueue confirms durable
// queue acceptance, not transport delivery. A crash after transport acceptance
// can cause duplicate delivery.
type EmailJobs struct {
	core     *ChattoCore
	sender   email.Sender
	branding func() email.Branding
}

// Verify EmailJobs implements email.Queue at compile time.
var _ email.Queue = (*EmailJobs)(nil)

// NewEmailJobs binds the HTTP runtime's transport and branding to the
// core-owned queue. The worker calls branding for each email it renders.
func (c *ChattoCore) NewEmailJobs(sender email.Sender, branding func() email.Branding) *EmailJobs {
	return &EmailJobs{core: c, sender: sender, branding: branding}
}

// IsEnabled reports whether the configured transport can accept mail.
func (m *EmailJobs) IsEnabled() bool { return m.sender.IsEnabled() }

func emailDeliveryKey(id string) string { return "email_delivery." + id }
func (m *EmailJobs) key() []byte {
	mac := hmac.New(sha256.New, []byte(m.core.config.SecretKey))
	_, _ = mac.Write([]byte("chatto:email-delivery:v1"))
	return mac.Sum(nil)
}

// Enqueue encrypts content before publishing a job with only an opaque ID.
// A failed or ambiguous handoff leaves at most an expiring encrypted record.
func (m *EmailJobs) Enqueue(ctx context.Context, delivery email.Delivery) error {
	if !m.IsEnabled() {
		return email.ErrEmailDisabled
	}
	if !delivery.ExpiresAt.After(time.Now()) {
		return errors.New("email delivery deadline required")
	}
	record, err := emailDeliveryRecord(delivery)
	if err != nil {
		return err
	}
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return err
	}
	id := hex.EncodeToString(idBytes)
	key := emailDeliveryKey(id)
	plain, err := proto.Marshal(record)
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
	if _, err := m.core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(time.Until(delivery.ExpiresAt))); err != nil {
		return err
	}
	return m.core.jobs.Enqueue(ctx, jobs.Spec{ID: id, Type: jobTypeEmail, MaxAttempts: jobs.UnlimitedAttempts, RetryDelay: 5 * time.Second, ExpiresAt: delivery.ExpiresAt})
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
	record := new(runtimestatev1.EmailDelivery)
	if err := proto.Unmarshal(plain, record); err != nil {
		return &jobs.Failure{Category: "invalid_content", Permanent: true}
	}
	message, err := m.render(record)
	if err != nil {
		return &jobs.Failure{Category: "invalid_content", Permanent: true}
	}
	ctx, cancelAttempt := context.WithTimeout(ctx, 30*time.Second)
	defer cancelAttempt()
	attemptCtx, cancel := context.WithDeadline(ctx, job.ExpiresAt)
	defer cancel()
	if err := m.sender.SendContext(attemptCtx, message); err != nil {
		return &jobs.Failure{Category: "delivery_failed"}
	}
	// Transport acceptance must survive normal shutdown or an attempt timeout.
	// Bound cleanup separately so cancellation cannot leave delivered content
	// available for another send. A crash or storage failure can still duplicate it.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelCleanup()
	return m.core.storage.runtimeStateKV.Delete(cleanupCtx, key, jetstream.LastRevision(entry.Revision()))
}

// emailDeliveryRecord converts accepted content to its stored form.
func emailDeliveryRecord(delivery email.Delivery) (*runtimestatev1.EmailDelivery, error) {
	record := &runtimestatev1.EmailDelivery{To: delivery.To, Locale: delivery.Locale}
	switch content := delivery.Content.(type) {
	case email.RegistrationCode:
		record.Content = &runtimestatev1.EmailDelivery_RegistrationCode{RegistrationCode: &runtimestatev1.EmailCode{Code: content.Code, Ttl: durationpb.New(content.TTL)}}
	case email.EmailVerificationCode:
		record.Content = &runtimestatev1.EmailDelivery_EmailVerificationCode{EmailVerificationCode: &runtimestatev1.EmailCode{Code: content.Code, Ttl: durationpb.New(content.TTL)}}
	case email.PasswordReset:
		record.Content = &runtimestatev1.EmailDelivery_PasswordReset{PasswordReset: &runtimestatev1.PasswordResetEmail{Token: content.Token, Ttl: durationpb.New(content.TTL)}}
	default:
		return nil, fmt.Errorf("unsupported email content %T", delivery.Content)
	}
	return record, nil
}

// render builds the message for a stored record with the current branding.
// A record from a version before content holds a rendered plain-text email,
// which is sent as it is.
func (m *EmailJobs) render(record *runtimestatev1.EmailDelivery) (email.Message, error) {
	var content email.Content
	switch stored := record.Content.(type) {
	case *runtimestatev1.EmailDelivery_RegistrationCode:
		content = email.RegistrationCode{Code: stored.RegistrationCode.GetCode(), TTL: stored.RegistrationCode.GetTtl().AsDuration()}
	case *runtimestatev1.EmailDelivery_EmailVerificationCode:
		content = email.EmailVerificationCode{Code: stored.EmailVerificationCode.GetCode(), TTL: stored.EmailVerificationCode.GetTtl().AsDuration()}
	case *runtimestatev1.EmailDelivery_PasswordReset:
		content = email.PasswordReset{Token: stored.PasswordReset.GetToken(), TTL: stored.PasswordReset.GetTtl().AsDuration()}
	default:
		if record.GetBody() == "" {
			return email.Message{}, errors.New("email record has no content")
		}
		return email.Message{To: record.GetTo(), Subject: record.GetSubject(), Body: record.GetBody()}, nil
	}
	return email.Render(m.branding(), record.GetLocale(), record.GetTo(), content)
}
