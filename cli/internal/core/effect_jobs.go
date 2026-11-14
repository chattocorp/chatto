package core

import (
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/cli/internal/logbridge"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
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

// effectJobTypes lists every effect type that the shared consumer must receive.
func effectJobTypes() []string {
	return []string{jobTypeAssetCleanup, jobTypeCallKeyCleanup, jobTypePushSubscriptionCleanup, jobTypeUserKeyShredding, jobTypeNotificationAlertDelivery}
}

// newEffectJob moves one existing durable effect consumer onto the shared
// queue and registers its handler. Call it during core construction, after
// initializeJobs and before ChattoCore.Run starts the job pool.
func (c *ChattoCore) newEffectJob(kind string, stream jetstream.Stream, consumer jetstream.Consumer, handle events.DurableDeliveryHandler, options jobqueue.EffectOptions) (*jobqueue.Effect, error) {
	if c.effectJobs[kind] != nil {
		return nil, fmt.Errorf("duplicate effect job type %q", kind)
	}
	options.Logger = logbridge.Slog(c.logger.WithPrefix("JobEffect." + kind))
	effect, err := jobqueue.NewEffect(c.jobs, kind, stream, consumer, handle, options)
	if err != nil {
		return nil, err
	}
	if c.effectJobs == nil {
		c.effectJobs = map[string]*jobqueue.Effect{}
	}
	c.effectJobs[kind] = effect
	return effect, nil
}
