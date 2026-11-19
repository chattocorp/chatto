package core

import (
	"cmp"
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
)

const brandingCleanupConsumerName = "chatto-branding-cleanup-v1"

// The source acknowledges a retirement only after JOBS accepts the work.
func (c *ChattoCore) configureBrandingCleanup(ctx context.Context) error {
	stream := c.storage.serverEvtStream
	subject := evtstream.ConfigSubjectAggregate(ConfigSubjectServer).Subject(evtstream.EventServerBrandingAssetRetired)
	consumer, err := evtstream.CreateEffectConsumer(ctx, stream, evtstream.EffectConsumerConfig{
		Name:           brandingCleanupConsumerName,
		Description:    "Hand off retired server branding assets for cleanup",
		FilterSubjects: []string{subject},
		AckWait:        assetCleanupAckWait,
		MaxAckPending:  assetCleanupMaxPending,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return err
	}
	_, err = c.newEffectJob(jobTypeBrandingCleanup, stream, consumer, c.cleanupRetiredBranding, jobqueue.EffectOptions{
		MaxAttempts: jobqueue.UnlimitedAttempts,
		RetryDelay:  assetCleanupRetryDelay,
	})
	return err
}

// Both steps tolerate an object removed by an interrupted attempt.
func (c *ChattoCore) cleanupRetiredBranding(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	asset := event.GetServerBrandingAssetRetired().GetAsset()
	key := cmp.Or(asset.GetNats().GetKey(), asset.GetS3().GetKey())
	id, namespaced, valid := serverAssetRequestKey(key)
	if !valid || id != asset.GetId() {
		return events.TerminateDelivery("invalid branding asset", errors.New("invalid storage key"))
	}
	switch {
	case asset.GetNats() != nil:
		err = c.storage.serverAssets.Delete(ctx, key)
		if err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
			return err
		}
	case asset.GetS3() != nil && !namespaced:
		if c.s3Client == nil {
			return errors.New("S3 storage unavailable")
		}
		if err := c.s3Client.DeleteObjectFromBucket(ctx, asset.GetS3().GetBucket(), S3KeyServerAsset(id)); err != nil {
			return err
		}
	default:
		return events.TerminateDelivery("invalid branding asset", errors.New("unknown storage"))
	}
	return c.deleteCachedResizesForServerAsset(ctx, key, "branding", "server")
}
