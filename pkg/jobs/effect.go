package jobs

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jobs/internal/jobpb"
)

// EffectOptions is the queue policy of one effect type.
type EffectOptions struct {
	// MaxAttempts is the automatic attempt budget. It must be positive.
	MaxAttempts uint32
	// RetryDelay is the first backoff delay, at least one millisecond.
	RetryDelay time.Duration
	// SourceMayExpire means that stream limits can remove a source message
	// before its job runs, and that another process owns that outcome. The job
	// then succeeds without work. Otherwise a missing source fails the job.
	SourceMayExpire bool
	// Accept selects the source messages that need a job. Nil accepts every
	// message. Rejected messages are acknowledged without a job. An error
	// retries the source message.
	Accept func(events.DurableDelivery) (bool, error)
	// SourceConcurrency bounds concurrent handoffs. Zero selects eight.
	SourceConcurrency int
	// Logger receives handoff diagnostics and failed attempts. Nil discards
	// them. Handler errors follow the DurableDeliveryHandler contract and must
	// not contain secrets or personal data.
	Logger *slog.Logger
}

// Effect moves an existing durable effect onto a Queue. Its source consumer
// keeps its persisted name and filter and acknowledges a source message only
// after the queue accepts a job for it. The job stores only the source subject,
// sequence, and a message digest. Each attempt reads the message again and runs
// the unchanged handler, so its fences and idempotency checks stay in place.
type Effect struct {
	queue   *Queue
	kind    string
	stream  jetstream.Stream
	source  *events.DurableWorker
	handle  events.DurableDeliveryHandler
	options EffectOptions
	logger  *slog.Logger
}

// NewEffect binds handle to the queue as job type kind. stream must be the
// stream of consumer; the job identity includes its name. Register Handle with
// Queue.Run under the same type, and run RunSource until shutdown.
func NewEffect(queue *Queue, kind string, stream jetstream.Stream, consumer jetstream.Consumer, handle events.DurableDeliveryHandler, options EffectOptions) (*Effect, error) {
	if queue == nil || stream == nil || consumer == nil || handle == nil {
		panic("jobs: effect requires a queue, a stream, a consumer, and a handler")
	}
	if !safeToken.MatchString(kind) || options.MaxAttempts == 0 || options.RetryDelay < time.Millisecond {
		return nil, errors.New("jobs: invalid effect options")
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Effect{queue: queue, kind: kind, stream: stream, handle: handle, options: options, logger: logger}
	source, err := events.NewDurableWorker(consumer, e.enqueue, events.DurableWorkerOptions{
		MaxConcurrent: cmp.Or(options.SourceConcurrency, 8),
		RetryDelay:    5 * time.Second,
		Logger:        logger,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: configure %s source: %w", kind, err)
	}
	e.source = source
	return e, nil
}


// Type returns the job type of this effect.
func (e *Effect) Type() string { return e.kind }

// RunSource hands source messages to the queue until cancellation. The queue's
// workers perform the effect.
func (e *Effect) RunSource(ctx context.Context) error {
	return e.source.Run(ctx)
}

func (e *Effect) enqueue(ctx context.Context, d events.DurableDelivery) error {
	if e.options.Accept != nil {
		accepted, err := e.options.Accept(d)
		if err != nil || !accepted {
			return err
		}
	}
	digest := sha256.Sum256(d.Data)
	payload, err := proto.Marshal(&jobpb.EffectSource{Subject: d.Subject, Sequence: d.StreamSequence, DataSha256: digest[:]})
	if err != nil {
		return err
	}
	// The digest keeps identities distinct when a recreated stream reuses
	// sequence numbers. Redelivery of the same message keeps the same identity.
	source := fmt.Sprintf("%s:%d:%x", e.stream.CachedInfo().Config.Name, d.StreamSequence, digest)
	return e.queue.Enqueue(ctx, Spec{
		ID: ID(e.kind, source), Type: e.kind, Payload: payload,
		MaxAttempts: e.options.MaxAttempts, RetryDelay: e.options.RetryDelay,
	})
}

// Handle is the queue handler of this effect. A handler error created with
// events.TerminateDelivery fails the job permanently; other errors use the
// automatic retry budget.
func (e *Effect) Handle(ctx context.Context, job Job) error {
	ref := new(jobpb.EffectSource)
	if err := proto.Unmarshal(job.Payload, ref); err != nil || ref.Subject == "" || ref.Sequence == 0 {
		return &Failure{Category: "invalid_payload", Permanent: true}
	}
	msg, err := e.stream.GetMsg(ctx, ref.Sequence)
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		if e.options.SourceMayExpire {
			return nil
		}
		return &Failure{Category: "source_missing", Permanent: true}
	}
	if err != nil {
		return err
	}
	if digest := sha256.Sum256(msg.Data); msg.Subject != ref.Subject || !bytes.Equal(digest[:], ref.DataSha256) {
		return &Failure{Category: "source_mismatch", Permanent: true}
	}
	err = e.handle(ctx, events.DurableDelivery{
		Subject:        msg.Subject,
		Data:           msg.Data,
		StreamSequence: msg.Sequence,
		PublishedAt:    msg.Time,
		NumDelivered:   uint64(job.Attempts),
	})
	if err == nil || ctx.Err() != nil {
		return err
	}
	reason, terminated := events.TerminationReason(err)
	e.logger.Warn("Effect job attempt failed", "job_type", e.kind, "job_id", job.ID, "attempt", job.Attempts, "terminated", terminated, "reason", reason, "error", err)
	if terminated {
		return &Failure{Category: "invalid_source", Permanent: true}
	}
	return err
}
