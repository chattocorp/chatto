package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// Job types of durable effects that start from one retained stream message.
// The names are persisted in job records and JOBS subjects.
const (
	jobTypeAssetCleanup              = "asset_cleanup"
	jobTypeCallKeyCleanup            = "call_key_cleanup"
	jobTypePushSubscriptionCleanup   = "push_subscription_cleanup"
	jobTypeUserKeyShredding          = "user_key_shredding"
	jobTypeNotificationAlertDelivery = "notification_alert"
)

// effectJobOptions is the per-type queue policy of an effect job.
type effectJobOptions struct {
	// MaxAttempts is the automatic attempt budget before the job fails.
	MaxAttempts uint32
	// RetryDelay is the first backoff delay. The queue doubles it per attempt.
	RetryDelay time.Duration
	// SourceMayExpire means that stream limits can remove the source message
	// before the job runs. The job then succeeds without work, because another
	// reconciler owns the outcome. Otherwise a missing source fails the job.
	SourceMayExpire bool
}

// effectJob moves one existing durable effect onto JOBS. Its source consumer
// keeps the persisted name and filter, and acknowledges a source message only
// after the queue durably accepts a job for it. The job stores only the source
// subject and sequence. Each attempt reads the message again and runs the
// unchanged effect handler, so domain fences and idempotency stay in place.
type effectJob struct {
	core    *ChattoCore
	kind    string
	stream  jetstream.Stream
	source  *events.DurableWorker
	handle  events.DurableDeliveryHandler
	options effectJobOptions
}

// newEffectJob registers kind with the shared queue. Call it during core
// construction, before ChattoCore.Run starts the job pool.
func (c *ChattoCore) newEffectJob(kind string, stream jetstream.Stream, consumer jetstream.Consumer, handle events.DurableDeliveryHandler, options effectJobOptions) (*effectJob, error) {
	if c.effectJobs[kind] != nil {
		return nil, fmt.Errorf("duplicate effect job type %q", kind)
	}
	j := &effectJob{core: c, kind: kind, stream: stream, handle: handle, options: options}
	source, err := evtstream.NewEffectWorker(consumer, j.enqueue, evtstream.EffectWorkerOptions{
		MaxConcurrent: 8,
		RetryDelay:    5 * time.Second,
		Logger:        c.logger.WithPrefix("JobSource." + kind),
	})
	if err != nil {
		return nil, fmt.Errorf("configure %s job source: %w", kind, err)
	}
	j.source = source
	if c.effectJobs == nil {
		c.effectJobs = map[string]*effectJob{}
	}
	c.effectJobs[kind] = j
	return j, nil
}

// Run hands source messages to the queue until cancellation. The shared job
// pool performs the work after boot. The handoff itself reads no projection.
func (j *effectJob) Run(ctx context.Context) error {
	return j.source.Run(ctx)
}

func (j *effectJob) enqueue(ctx context.Context, d events.DurableDelivery) error {
	digest := sha256.Sum256(d.Data)
	payload, err := proto.Marshal(&statev1.EventEffectJob{Subject: d.Subject, Sequence: d.StreamSequence, DataSha256: digest[:]})
	if err != nil {
		return err
	}
	// The digest keeps identities distinct when a recreated stream reuses
	// sequence numbers. Redelivery of the same message keeps the same identity.
	source := fmt.Sprintf("%s:%d:%x", j.stream.CachedInfo().Config.Name, d.StreamSequence, digest)
	return j.core.jobs.Enqueue(ctx, &statev1.JobRecord{
		Id:               jobqueue.ID(j.kind, source),
		Type:             j.kind,
		Payload:          payload,
		MaxAttempts:      j.options.MaxAttempts,
		RetryDelayMillis: j.options.RetryDelay.Milliseconds(),
	})
}

// perform is the queue handler. It maps a handler termination to a permanent
// failure; other errors use the automatic retry budget.
func (j *effectJob) perform(ctx context.Context, job *statev1.JobRecord) error {
	ref := new(statev1.EventEffectJob)
	if err := proto.Unmarshal(job.Payload, ref); err != nil || ref.Subject == "" || ref.Sequence == 0 {
		return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
	}
	msg, err := j.stream.GetMsg(ctx, ref.Sequence)
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		if j.options.SourceMayExpire {
			return nil
		}
		return &jobqueue.Failure{Category: "source_missing", Permanent: true}
	}
	if err != nil {
		return err
	}
	if digest := sha256.Sum256(msg.Data); msg.Subject != ref.Subject || !bytes.Equal(digest[:], ref.DataSha256) {
		return &jobqueue.Failure{Category: "source_mismatch", Permanent: true}
	}
	err = j.handle(ctx, events.DurableDelivery{
		Subject:        msg.Subject,
		Data:           msg.Data,
		StreamSequence: msg.Sequence,
		PublishedAt:    msg.Time,
		NumDelivered:   uint64(job.Attempts),
	})
	if err == nil || ctx.Err() != nil {
		return err
	}
	// Effect handler errors follow the DurableDeliveryHandler contract: they
	// contain no secrets or PII. The queue stores only a fixed category.
	reason, terminated := events.TerminationReason(err)
	j.core.logger.Warn("Effect job attempt failed", "job_type", j.kind, "job_id", job.Id, "attempt", job.Attempts, "terminated", terminated, "reason", reason, "error", err)
	if terminated {
		return &jobqueue.Failure{Category: "invalid_source", Permanent: true}
	}
	return err
}

// effectJobTypes lists every type that the shared consumer must receive.
func effectJobTypes() []string {
	return []string{jobTypeAssetCleanup, jobTypeCallKeyCleanup, jobTypePushSubscriptionCleanup, jobTypeUserKeyShredding, jobTypeNotificationAlertDelivery}
}
