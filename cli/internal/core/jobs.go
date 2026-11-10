package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// ErrJobNotRetryable means this job is active, successful, or expired.
var ErrJobNotRetryable = errors.New("job cannot be retried")

func (c *ChattoCore) initializeJobs(ctx context.Context) error {
	c.jobs = jobqueue.New(c.js, c.storage.runtimeStateKV, c.logger.WithPrefix("Jobs"))
	var err error
	c.jobsConsumer, err = c.storage.jobsStream.CreateOrUpdateConsumer(ctx, jobqueue.ConsumerConfig(jobqueue.ConsumerName, []string{"webhook", "media"}))
	if err != nil {
		return err
	}
	// Keep the persisted source consumer identity. Older media units can still
	// process its deliveries during an upgrade; both paths use asset terminal facts.
	c.mediaSourceConsumer, err = evtstream.CreateEffectConsumer(ctx, c.storage.serverEvtStream, evtstream.EffectConsumerConfig{Name: AssetProcessingConsumerName, Description: "Durable media job handoff", FilterSubjects: []string{evtstream.AssetEventTypeFilter(evtstream.EventAssetProcessingStarted), evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted)}, AckWait: 2 * time.Minute, MaxAckPending: 64, DeliverPolicy: jetstream.DeliverAllPolicy})
	return err
}

// SetMediaJobHandler registers media processing before Run starts. The handler
// owns asset projection barriers and terminal outcomes. Concurrency bounds the
// media attempts within the shared pool when processing runs in this replica.
func (c *ChattoCore) SetMediaJobHandler(handler func(context.Context, string, string) error, concurrency int) {
	slots := make(chan struct{}, max(1, concurrency))
	c.mediaJobHandler = func(ctx context.Context, assetID, messageID string) error {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			return handler(ctx, assetID, messageID)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *ChattoCore) runJobs(ctx context.Context) error {
	if err := c.WaitForBoot(ctx); err != nil {
		return err
	}
	c.jobs.OnFailed = func(ctx context.Context, job *statev1.JobRecord) {
		if job.Type != "webhook" {
			return
		}
		payload := new(statev1.WebhookJob)
		if proto.Unmarshal(job.Payload, payload) != nil {
			return
		}
		request := &botWebhookDelivery{DeliveryID: job.Id, BotUserID: payload.BotUserId, WebhookID: payload.WebhookId, SourceEventID: payload.SourceEventId}
		if err := c.botWebhooks.fail(ctx, request, job.Attempts, job.Failure, 0); err != nil {
			c.logger.Warn("Could not record webhook queue failure", "delivery_id", job.Id)
		}
	}
	handlers := map[string]jobqueue.Handler{"webhook": c.botWebhooks.performJob}
	if c.mediaJobHandler != nil {
		handlers["media"] = func(ctx context.Context, job *statev1.JobRecord) error {
			request := new(statev1.MediaJob)
			if err := proto.Unmarshal(job.Payload, request); err != nil {
				return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
			}
			if request.AssetId == "" || request.MessageEventId == "" {
				return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
			}
			return c.mediaJobHandler(ctx, request.AssetId, request.MessageEventId)
		}
		c.RecoverUnmanifestedVideoAttachments(ctx)
	}
	concurrency := c.config.JobConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	return c.jobs.Run(ctx, c.jobsConsumer, handlers, concurrency)
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
		return c.jobs.Enqueue(ctx, &statev1.JobRecord{Id: jobqueue.ID("media", event.GetId()), Type: "media", Payload: payload, MaxAttempts: 5, RetryDelayMillis: 30000})
	}, evtstream.EffectWorkerOptions{MaxConcurrent: 8, RetryDelay: 5 * time.Second, Logger: c.logger.WithPrefix("MediaJobSource")})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// ListJobs returns safe-to-map private job records after the operation's gate.
// The page token is sealed and bound to the viewer; no broker position is public.
func (c *ChattoCore) ListJobs(ctx context.Context, actorID string, limit int, token string) ([]*statev1.JobRecord, string, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerJobsManage); err != nil {
		return nil, "", err
	}
	if limit < 0 || limit > 200 {
		return nil, "", ErrInvalidArgument
	}
	if limit == 0 {
		limit = 50
	}
	after := ""
	if token != "" {
		data, err := c.OpenPublicCursor("jobs-page-v1", actorID, token)
		if err != nil {
			return nil, "", ErrInvalidArgument
		}
		after = string(data)
	}
	jobs, more, err := c.jobs.List(ctx, after, limit)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if more && len(jobs) > 0 {
		next, err = c.SealPublicCursor("jobs-page-v1", actorID, []byte(jobs[len(jobs)-1].Id))
	}
	return jobs, next, err
}

// GetJob requires the same management permission as list and retry.
func (c *ChattoCore) GetJob(ctx context.Context, actorID, id string) (*statev1.JobRecord, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerJobsManage); err != nil {
		return nil, err
	}
	job, err := c.jobs.Get(ctx, id)
	if errors.Is(err, jobqueue.ErrNotFound) {
		return nil, ErrNotFound
	}
	return job, err
}

// RetryJob accepts one new attempt generation without changing its source-time
// deadline or payload. Revision OCC serializes concurrent operator requests.
func (c *ChattoCore) RetryJob(ctx context.Context, actorID, id, expectedVersion string) (*statev1.JobRecord, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerJobsManage); err != nil {
		return nil, err
	}
	job, err := c.jobs.Retry(ctx, id, expectedVersion)
	if errors.Is(err, jobqueue.ErrNotFound) {
		return nil, ErrNotFound
	}
	if errors.Is(err, jobqueue.ErrNotRetryable) {
		return nil, ErrJobNotRetryable
	}
	if err != nil {
		return nil, fmt.Errorf("retry job: %w", err)
	}
	return job, nil
}

// enqueueWebhookJob stores only references, never content or credentials.
func (c *ChattoCore) enqueueWebhookJob(ctx context.Context, r *botWebhookDelivery) error {
	payload, err := proto.Marshal(&statev1.WebhookJob{BotUserId: r.BotUserID, WebhookId: r.WebhookID, SourceEventId: r.SourceEventID, RoomId: r.RoomID, ConfigurationSequence: r.ConfigurationSequence, Triggers: r.Triggers, OccurredAt: timestamppb.New(r.OccurredAt)})
	if err != nil {
		return err
	}
	return c.jobs.Enqueue(ctx, &statev1.JobRecord{Id: r.DeliveryID, Type: "webhook", Payload: payload, MaxAttempts: r.MaxAttempts, RetryDelayMillis: r.RetryDelay.Milliseconds(), ExpiresAt: timestamppb.New(r.ExpiresAt)})
}

func (m *botWebhookModel) performJob(ctx context.Context, job *statev1.JobRecord) error {
	payload := new(statev1.WebhookJob)
	if err := proto.Unmarshal(job.Payload, payload); err != nil {
		return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
	}
	r := &botWebhookDelivery{DeliveryID: job.Id, BotUserID: payload.BotUserId, WebhookID: payload.WebhookId, SourceEventID: payload.SourceEventId, RoomID: payload.RoomId, ConfigurationSequence: payload.ConfigurationSequence, Triggers: payload.Triggers, OccurredAt: payload.OccurredAt.AsTime(), ExpiresAt: job.ExpiresAt.AsTime(), MaxAttempts: job.MaxAttempts, RetryDelay: time.Duration(job.RetryDelayMillis) * time.Millisecond}
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
			m.core.logger.Warn("Could not record outbound webhook failure", "delivery_id", job.Id)
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
