package email

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/chattocorp/chatto/authling/internal/runtimejson"
	"github.com/chattocorp/chatto/authling/internal/storage"
	"github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
)

// JobType selects Authling's independent mail worker pool.
const JobType = "email"

// QueueSender accepts encrypted, expiring mail for durable asynchronous delivery.
// A crash after SMTP acceptance can cause a duplicate; SMTP is not transactional.
type QueueSender struct {
	kv        storage.KeyValue
	key       []byte
	queue     *jobs.Queue
	transport Sender
}

// NewQueueSender binds the runtime store and workflow encryption key to the queue.
func NewQueueSender(kv storage.KeyValue, key []byte, queue *jobs.Queue, transport Sender) *QueueSender {
	return &QueueSender{kv: kv, key: append([]byte(nil), key...), queue: queue, transport: transport}
}

func deliveryKey(id string) string  { return "email-delivery." + id }
func deliveryAAD(key string) []byte { return []byte("authling:email-delivery:v1\x00" + key) }

// SendContext returns after queue acceptance. Storage failures remain request
// failures; SMTP failures retry in the worker without releasing admission limits.
func (s *QueueSender) SendContext(ctx context.Context, message Message) error {
	if !message.ExpiresAt.After(time.Now()) {
		return errors.New("email delivery deadline required")
	}
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return err
	}
	id := hex.EncodeToString(idBytes)
	key := deliveryKey(id)
	data, err := runtimejson.Seal(s.key, deliveryAAD(key), message, runtimejson.LowercaseFields)
	if err != nil {
		return err
	}
	if _, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(time.Until(message.ExpiresAt))); err != nil {
		return err
	}
	// Keep the content on ambiguous publish errors. It expires even if no job
	// was accepted, and an accepted job still has the content it needs.
	return s.queue.Enqueue(ctx, jobs.Spec{ID: id, Type: JobType, MaxAttempts: jobs.UnlimitedAttempts, RetryDelay: 5 * time.Second, ExpiresAt: message.ExpiresAt})
}

// Deliver performs one bounded attempt and removes content after SMTP accepts it.
// A replay after content removal does not send again. Job facts contain no PII.
func (s *QueueSender) Deliver(ctx context.Context, job jobs.Job) error {
	key := deliveryKey(job.ID)
	entry, err := s.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var message Message
	if err := runtimejson.Open(s.key, deliveryAAD(key), entry.Value(), &message); err != nil {
		return &jobs.Failure{Category: "content_unavailable"}
	}
	if !message.ExpiresAt.After(time.Now()) {
		return &jobs.Failure{Category: "expired", Permanent: true}
	}
	ctx, cancelAttempt := context.WithTimeout(ctx, 30*time.Second)
	defer cancelAttempt()
	attemptCtx, cancel := context.WithDeadline(ctx, message.ExpiresAt)
	defer cancel()
	if err := s.transport.SendContext(attemptCtx, message); err != nil {
		return &jobs.Failure{Category: "delivery_failed"}
	}
	// Transport acceptance must survive normal shutdown or an attempt timeout.
	// Bound cleanup separately so cancellation cannot leave delivered content
	// available for another send. A crash or storage failure can still duplicate it.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelCleanup()
	_, err = s.kv.DeleteAt(cleanupCtx, key, entry.Revision())
	return err
}
