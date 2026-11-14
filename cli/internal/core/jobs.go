package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// Persisted job queue resource contract (ADR-123). The JOBS stream holds the
// facts of each job on jobs.{type}.{id}.{fact}.
const (
	jobSubjectPrefix = "jobs"
	jobTypeWebhook   = "webhook"
	jobTypeMedia     = "media"
	// JobRetention is the maximum age of the JOBS stream. Unfinished and failed
	// jobs stay this long at most.
	JobRetention = 30 * 24 * time.Hour
	// JobSucceededRetention is how long succeeded jobs stay.
	JobSucceededRetention = 3 * 24 * time.Hour
)

// ErrJobNotRetryable means this job is active, successful, or expired.
var ErrJobNotRetryable = errors.New("job cannot be retried")

// JobsConsumerName returns the durable JOBS consumer of one job type. Each
// type has its own consumer and worker pool, so one type cannot delay another.
func JobsConsumerName(kind string) string {
	return "chatto-jobs-" + strings.ReplaceAll(kind, "_", "-") + "-v1"
}

// jobTypes lists every job type. Each has a JOBS consumer.
func jobTypes() []string {
	return append([]string{jobTypeWebhook, jobTypeMedia, jobTypeNotificationAlertDelivery}, effectJobTypes()...)
}

// NewJobQueue binds Chatto's job queue contract to the JOBS stream. onFailed
// may be nil.
func NewJobQueue(js jetstream.JetStream, stream jetstream.Stream, logger *log.Logger, onFailed func(context.Context, jobqueue.Job)) (*jobqueue.Queue, error) {
	return jobqueue.New(js, stream, jobqueue.Options{
		SubjectPrefix: jobSubjectPrefix, SucceededRetention: JobSucceededRetention,
		Logger: logbridge.Slog(logger), OnFailed: onFailed,
	})
}

// JobsConsumerConfig is the application-owned configuration of the JOBS
// consumer of one job type. A job that waits for a retry holds an
// unacknowledged delivery, so pending acknowledgements have no limit; the
// worker pool bounds running jobs.
func JobsConsumerConfig(q *jobqueue.Queue, kind string) jetstream.ConsumerConfig {
	name := JobsConsumerName(kind)
	return jetstream.ConsumerConfig{Name: name, Durable: name, Description: "Durable background jobs", FilterSubjects: q.FilterSubjects(kind), AckPolicy: jetstream.AckExplicitPolicy, AckWait: 2 * time.Minute, MaxAckPending: -1, MaxDeliver: -1, DeliverPolicy: jetstream.DeliverAllPolicy}
}

func (c *ChattoCore) initializeJobs(ctx context.Context) error {
	var err error
	c.jobs, err = NewJobQueue(c.js, c.storage.jobsStream, c.logger.WithPrefix("Jobs"), c.recordWebhookJobFailure)
	if err != nil {
		return err
	}
	c.jobConsumers = map[string]jetstream.Consumer{}
	for _, kind := range jobTypes() {
		c.jobConsumers[kind], err = c.storage.jobsStream.CreateOrUpdateConsumer(ctx, JobsConsumerConfig(c.jobs, kind))
		if err != nil {
			return fmt.Errorf("create %s job consumer: %w", kind, err)
		}
	}
	// Keep the persisted source consumer identity. Older media units can still
	// process its deliveries during an upgrade; both paths use asset terminal facts.
	c.mediaSourceConsumer, err = evtstream.CreateEffectConsumer(ctx, c.storage.serverEvtStream, evtstream.EffectConsumerConfig{Name: AssetProcessingConsumerName, Description: "Durable media job handoff", FilterSubjects: []string{evtstream.AssetEventTypeFilter(evtstream.EventAssetProcessingStarted), evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted)}, AckWait: 2 * time.Minute, MaxAckPending: 64, DeliverPolicy: jetstream.DeliverAllPolicy})
	return err
}

// recordWebhookJobFailure keeps the existing endpoint diagnostics in LOG. The
// failed job fact stays authoritative; a LOG failure is only logged.
func (c *ChattoCore) recordWebhookJobFailure(ctx context.Context, job jobqueue.Job) {
	if job.Type != jobTypeWebhook || c.botWebhooks == nil {
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
// owns asset projection barriers and terminal outcomes. concurrency is the size
// of the media worker pool of this process.
func (c *ChattoCore) SetMediaJobHandler(handler func(context.Context, string, string) error, concurrency int) {
	c.mediaJobHandler = handler
	c.mediaJobConcurrency = max(1, concurrency)
}

func (c *ChattoCore) runJobs(ctx context.Context) error {
	if err := c.WaitForBoot(ctx); err != nil {
		return err
	}
	concurrency := c.config.JobConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return c.jobs.Run(ctx) })
	work := func(kind string, handler jobqueue.Handler, concurrency int) {
		g.Go(func() error { return c.jobs.Work(ctx, c.jobConsumers[kind], handler, concurrency) })
	}
	work(jobTypeWebhook, c.botWebhooks.performJob, concurrency)
	for _, kind := range effectJobTypes() {
		work(kind, c.effectJobs[kind].Handle, concurrency)
	}
	// Alerts keep the pool size of the worker that they replace, so they meet
	// their delivery deadline.
	work(jobTypeNotificationAlertDelivery, c.effectJobs[jobTypeNotificationAlertDelivery].Handle, notificationAlertMaxPending)
	if c.mediaJobHandler != nil {
		c.RecoverUnmanifestedVideoAttachments(ctx)
		work(jobTypeMedia, MediaJobHandler(c.logger.WithPrefix("MediaJobs"), c.mediaJobHandler), c.mediaJobConcurrency)
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
		// Media retries until it has a terminal outcome, as the earlier media
		// worker did: missing tools or an object-store outage must not fail
		// attachments permanently. Unprocessable media records a terminal failure.
		return c.jobs.Enqueue(ctx, jobqueue.Spec{ID: jobqueue.ID(jobTypeMedia, event.GetId()), Type: jobTypeMedia, Payload: payload, MaxAttempts: jobqueue.UnlimitedAttempts, RetryDelay: 30 * time.Second})
	}, evtstream.EffectWorkerOptions{MaxConcurrent: 8, RetryDelay: 5 * time.Second, Logger: c.logger.WithPrefix("MediaJobSource")})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// MediaJobHandler decodes a media job and runs process for its asset. Server
// replicas and standalone media processes share it. The queue stores only a
// failure category, so the handler logs the cause of each failed attempt.
func MediaJobHandler(logger *log.Logger, process func(ctx context.Context, assetID, messageID string) error) jobqueue.Handler {
	return func(ctx context.Context, job jobqueue.Job) error {
		request := new(statev1.MediaJob)
		if err := proto.Unmarshal(job.Payload, request); err != nil || request.AssetId == "" || request.MessageEventId == "" {
			return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
		}
		err := process(ctx, request.AssetId, request.MessageEventId)
		if err != nil && ctx.Err() == nil {
			logger.Warn("Media processing attempt failed; the job retries", "job_id", job.ID, "asset_id", request.AssetId, "attempt", job.Attempts, "error", err)
		}
		return err
	}
}

// ListJobs returns payload-free jobs after the operation's gate.
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

// RetryJob starts one new attempt budget without changing the deadline or
// payload of the job. OCC on the facts of the job serializes concurrent
// operator requests.
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
	return c.jobs.Enqueue(ctx, jobqueue.Spec{ID: r.DeliveryID, Type: jobTypeWebhook, Payload: payload, MaxAttempts: r.MaxAttempts, RetryDelay: r.RetryDelay, ExpiresAt: r.ExpiresAt})
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
		// LOG is diagnostic only. The failed job fact is the durable outcome.
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
