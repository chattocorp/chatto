package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/logbridge"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

// Persisted job queue resource contract (ADR-114). Records live in
// RUNTIME_STATE under job.{id}; dispatches use jobs.{type}.{id} in JOBS.
const (
	// JobsConsumerName is the shared JOBS consumer of server replicas.
	JobsConsumerName = "chatto-jobs-v1"
	// MediaJobsConsumerName is the JOBS consumer of standalone media processes.
	MediaJobsConsumerName = "chatto-jobs-media-v1"
	jobKeyPrefix          = "job."
	jobSubjectPrefix      = "jobs"
	// JobSucceededRetention is how long succeeded job records stay.
	JobSucceededRetention = 7 * 24 * time.Hour
)

// ErrJobNotRetryable means this job is active, successful, or expired.
var ErrJobNotRetryable = errors.New("job cannot be retried")

// NewJobQueue binds Chatto's job queue contract to provisioned resources. kv
// must be the leader-bound RUNTIME_STATE bucket. onFailed may be nil.
func NewJobQueue(js jetstream.JetStream, kv jobqueue.KeyValue, logger *log.Logger, onFailed func(context.Context, jobqueue.Job)) (*jobqueue.Queue, error) {
	return jobqueue.New(js, kv, jobqueue.Options{
		KeyPrefix: jobKeyPrefix, SubjectPrefix: jobSubjectPrefix, SucceededRetention: JobSucceededRetention,
		Logger: logbridge.Slog(logger), OnFailed: onFailed,
	})
}

// JobsConsumerConfig is the application-owned configuration of a JOBS consumer.
// A standalone media consumer overlaps the shared consumer safely through
// record revision claims.
func JobsConsumerConfig(q *jobqueue.Queue, name string, types ...string) jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{Name: name, Durable: name, Description: "Durable background job dispatch", FilterSubjects: q.FilterSubjects(types...), AckPolicy: jetstream.AckExplicitPolicy, AckWait: 2 * time.Minute, MaxAckPending: 64, MaxDeliver: -1, DeliverPolicy: jetstream.DeliverAllPolicy}
}

func (c *ChattoCore) initializeJobs(ctx context.Context) error {
	var err error
	c.jobs, err = NewJobQueue(c.js, c.storage.runtimeStateKV, c.logger.WithPrefix("Jobs"), c.recordWebhookJobFailure)
	if err != nil {
		return err
	}
	// Media has its own consumer, shared with standalone media processes. A
	// media backlog, or a replica without media processing, then holds no
	// capacity of the shared consumer.
	c.jobsConsumer, err = c.storage.jobsStream.CreateOrUpdateConsumer(ctx, JobsConsumerConfig(c.jobs, JobsConsumerName, append([]string{"webhook"}, effectJobTypes()...)...))
	if err != nil {
		return err
	}
	c.mediaJobsConsumer, err = c.storage.jobsStream.CreateOrUpdateConsumer(ctx, JobsConsumerConfig(c.jobs, MediaJobsConsumerName, "media"))
	if err != nil {
		return err
	}
	// Keep the persisted source consumer identity. Older media units can still
	// process its deliveries during an upgrade; both paths use asset terminal facts.
	c.mediaSourceConsumer, err = evtstream.CreateEffectConsumer(ctx, c.storage.serverEvtStream, evtstream.EffectConsumerConfig{Name: AssetProcessingConsumerName, Description: "Durable media job handoff", FilterSubjects: []string{evtstream.AssetEventTypeFilter(evtstream.EventAssetProcessingStarted), evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted)}, AckWait: 2 * time.Minute, MaxAckPending: 64, DeliverPolicy: jetstream.DeliverAllPolicy})
	return err
}

// recordWebhookJobFailure keeps the existing endpoint diagnostics in LOG. The
// job record stays authoritative; a LOG failure is only logged.
func (c *ChattoCore) recordWebhookJobFailure(ctx context.Context, job jobqueue.Job) {
	if job.Type != "webhook" || c.botWebhooks == nil {
		return
	}
	payload := new(statev1.WebhookJob)
	if proto.Unmarshal(job.Payload, payload) != nil {
		return
	}
	request := &botWebhookDelivery{DeliveryID: job.ID, BotUserID: payload.BotUserId, WebhookID: payload.WebhookId, SourceEventID: payload.SourceEventId}
	if err := c.botWebhooks.fail(ctx, request, job.Attempts, job.Failure, 0); err != nil {
		c.logger.Warn("Could not record webhook queue failure", "delivery_id", job.ID)
	}
}

// SetMediaJobHandler registers media processing before Run starts. The handler
// owns asset projection barriers and terminal outcomes. Media jobs use their
// own consumer and worker pool with this concurrency, so a video backlog cannot
// delay other jobs.
func (c *ChattoCore) SetMediaJobHandler(handler func(context.Context, string, string) error, concurrency int) {
	c.mediaJobHandler = handler
	c.mediaJobConcurrency = max(1, concurrency)
}

func (c *ChattoCore) runJobs(ctx context.Context) error {
	if err := c.WaitForBoot(ctx); err != nil {
		return err
	}
	handlers := map[string]jobqueue.Handler{"webhook": c.botWebhooks.performJob}
	for kind, job := range c.effectJobs {
		handlers[kind] = job.Handle
	}
	concurrency := c.config.JobConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return c.jobs.Run(ctx) })
	g.Go(func() error { return c.jobs.Work(ctx, c.jobsConsumer, handlers, concurrency) })
	if c.mediaJobHandler != nil {
		c.RecoverUnmanifestedVideoAttachments(ctx)
		media := map[string]jobqueue.Handler{"media": func(ctx context.Context, job jobqueue.Job) error {
			request := new(statev1.MediaJob)
			if err := proto.Unmarshal(job.Payload, request); err != nil || request.AssetId == "" || request.MessageEventId == "" {
				return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
			}
			return c.mediaJobHandler(ctx, request.AssetId, request.MessageEventId)
		}}
		g.Go(func() error { return c.jobs.Work(ctx, c.mediaJobsConsumer, media, c.mediaJobConcurrency) })
	}
	return g.Wait()
}
func (c *ChattoCore) runMediaJobSource(ctx context.Context) error {
	if err := c.WaitForBoot(ctx); err != nil {
		return err
	}
	worker, err := evtstream.NewEffectWorker(c.mediaSourceConsumer, func(ctx context.Context, d events.DurableDelivery) error {
		event, err := decodeDurableCoreDelivery(d)
		if err != nil {
			return err
		}
		request := event.GetAssetProcessingStarted()
		if event.GetId() == "" || request == nil || request.GetAssetId() == "" || request.GetMessageEventId() == "" {
			return events.TerminateDelivery("invalid media request", errors.New("invalid media request"))
		}
		payload, err := proto.Marshal(&statev1.MediaJob{AssetId: request.GetAssetId(), MessageEventId: request.GetMessageEventId()})
		if err != nil {
			return err
		}
		return c.jobs.Enqueue(ctx, jobqueue.Spec{ID: jobqueue.ID("media", event.GetId()), Type: "media", Payload: payload, MaxAttempts: 5, RetryDelay: 30 * time.Second})
	}, evtstream.EffectWorkerOptions{MaxConcurrent: 8, RetryDelay: 5 * time.Second, Logger: c.logger.WithPrefix("MediaJobSource")})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// ListJobs returns safe-to-map private job records after the operation's gate.
// The page token is sealed and bound to the viewer; no broker position is public.
func (c *ChattoCore) ListJobs(ctx context.Context, actorID string, limit int, token string, status jobqueue.Status) ([]jobqueue.Job, string, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return nil, "", err
	}
	if limit < 0 || limit > 200 {
		return nil, "", ErrInvalidArgument
	}
	if limit == 0 {
		limit = 50
	}
	// The sealed cursor binds the queue position to this viewer and filter.
	purpose := fmt.Sprintf("jobs-page-v2:%d", status)
	cursor := ""
	if token != "" {
		data, err := c.OpenPublicCursor(purpose, actorID, token)
		if err != nil {
			return nil, "", ErrInvalidArgument
		}
		cursor = string(data)
	}
	jobs, next, err := c.jobs.List(ctx, jobqueue.ListOptions{Status: status, Limit: limit, Cursor: cursor})
	if err != nil {
		return nil, "", err
	}
	if next != "" {
		next, err = c.SealPublicCursor(purpose, actorID, []byte(next))
	}
	return jobs, next, err
}

// GetJob requires the same management permission as list and retry.
func (c *ChattoCore) GetJob(ctx context.Context, actorID, id string) (jobqueue.Job, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return jobqueue.Job{}, err
	}
	job, err := c.jobs.Get(ctx, id)
	if errors.Is(err, jobqueue.ErrNotFound) {
		return jobqueue.Job{}, ErrNotFound
	}
	return job, err
}

// RetryJob accepts one new attempt generation without changing its source-time
// deadline or payload. Revision OCC serializes concurrent operator requests.
func (c *ChattoCore) RetryJob(ctx context.Context, actorID, id, expectedVersion string) (jobqueue.Job, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return jobqueue.Job{}, err
	}
	job, err := c.jobs.Retry(ctx, id, expectedVersion)
	if errors.Is(err, jobqueue.ErrNotFound) {
		return jobqueue.Job{}, ErrNotFound
	}
	if errors.Is(err, jobqueue.ErrNotRetryable) {
		return jobqueue.Job{}, ErrJobNotRetryable
	}
	if err != nil {
		return jobqueue.Job{}, fmt.Errorf("retry job: %w", err)
	}
	return job, nil
}

// enqueueWebhookJob stores only references, never content or credentials.
func (c *ChattoCore) enqueueWebhookJob(ctx context.Context, r *botWebhookDelivery) error {
	payload, err := proto.Marshal(&statev1.WebhookJob{BotUserId: r.BotUserID, WebhookId: r.WebhookID, SourceEventId: r.SourceEventID, RoomId: r.RoomID, ConfigurationSequence: r.ConfigurationSequence, Triggers: r.Triggers, OccurredAt: timestamppb.New(r.OccurredAt)})
	if err != nil {
		return err
	}
	return c.jobs.Enqueue(ctx, jobqueue.Spec{ID: r.DeliveryID, Type: "webhook", Payload: payload, MaxAttempts: r.MaxAttempts, RetryDelay: r.RetryDelay, ExpiresAt: r.ExpiresAt})
}

func (m *botWebhookModel) performJob(ctx context.Context, job jobqueue.Job) error {
	payload := new(statev1.WebhookJob)
	if err := proto.Unmarshal(job.Payload, payload); err != nil {
		return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
	}
	r := &botWebhookDelivery{DeliveryID: job.ID, BotUserID: payload.BotUserId, WebhookID: payload.WebhookId, SourceEventID: payload.SourceEventId, RoomID: payload.RoomId, ConfigurationSequence: payload.ConfigurationSequence, Triggers: payload.Triggers, OccurredAt: payload.OccurredAt.AsTime(), ExpiresAt: job.ExpiresAt, MaxAttempts: job.MaxAttempts, RetryDelay: job.RetryDelay}
	err := m.deliver(ctx, r)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	reason, status := "attempt_failed", 0
	if failure, ok := errors.AsType[*botWebhookAttemptFailure](err); ok {
		reason, status = failure.reason, failure.status
	}
	if !m.now().Before(r.ExpiresAt) {
		reason = "expired"
	}
	permanent := reason == "expired" || reason == "invalid_request"
	if permanent || job.Attempts >= job.MaxAttempts {
		// LOG is diagnostic only. The queue record is the durable failure outcome.
		if logErr := m.fail(ctx, r, job.Attempts, reason, status); logErr != nil {
			m.core.logger.Warn("Could not record outbound webhook failure", "delivery_id", job.ID)
		}
	}
	return &jobqueue.Failure{Category: reason, Permanent: permanent}
}

// ProcessMediaJob fences processing on current asset state and confirms a
// terminal outcome. A failed conversion is a terminal asset outcome; job success
// means the handler completed, not that the media conversion succeeded.
func (c *ChattoCore) ProcessMediaJob(ctx context.Context, assetID, messageID string, process func(context.Context, string, string) error) error {
	if err := c.assetModel.waitForAssetsCurrent(ctx); err != nil {
		return err
	}
	terminal := func() bool {
		state := c.GetAssetState(assetID)
		return state.Deleted || (state.VideoManifest != nil && (state.VideoManifest.Succeeded != nil || state.VideoManifest.Failed != nil))
	}
	if terminal() {
		return nil
	}
	err := process(ctx, assetID, messageID)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if terminal() {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("media processing has no terminal outcome")
}
