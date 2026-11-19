package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
)

const brandingCleanupConsumerName = "chatto-branding-cleanup-v1"

// The source acknowledges a branding change only after JOBS accepts the work.
func (c *ChattoCore) configureBrandingCleanup(ctx context.Context) error {
	stream := c.storage.serverEvtStream
	config := evtstream.ConfigSubjectAggregate(ConfigSubjectServer)
	consumer, err := evtstream.CreateEffectConsumer(ctx, stream, evtstream.EffectConsumerConfig{
		Name:        brandingCleanupConsumerName,
		Description: "Hand off retired server branding assets for cleanup",
		FilterSubjects: []string{
			config.Subject(evtstream.EventServerLogoSet), config.Subject(evtstream.EventServerLogoCleared),
			config.Subject(evtstream.EventServerBannerSet), config.Subject(evtstream.EventServerBannerCleared),
		},
		AckWait:       assetCleanupAckWait,
		MaxAckPending: assetCleanupMaxPending,
		DeliverPolicy: jetstream.DeliverNewPolicy,
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
	if delivery.Subject != evtstream.ConfigSubjectAggregate(ConfigSubjectServer).SubjectFor(event) {
		return events.TerminateDelivery("invalid branding change", errors.New("config subject and event do not match"))
	}
	var setType, clearType string
	var current *evtv1.AssetRecord
	switch event.GetEvent().(type) {
	case *evtv1.Event_ServerLogoSet, *evtv1.Event_ServerLogoCleared:
		setType, clearType = evtstream.EventServerLogoSet, evtstream.EventServerLogoCleared
		current = event.GetServerLogoSet().GetAsset()
	case *evtv1.Event_ServerBannerSet, *evtv1.Event_ServerBannerCleared:
		setType, clearType = evtstream.EventServerBannerSet, evtstream.EventServerBannerCleared
		current = event.GetServerBannerSet().GetAsset()
	default:
		return events.TerminateDelivery("invalid branding change", errors.New("unexpected config event"))
	}
	asset, err := c.previousBrandingAsset(ctx, delivery.StreamSequence, setType, clearType)
	if err != nil {
		return err
	}
	if asset == nil || ServerAssetDeliveryKey(asset) == ServerAssetDeliveryKey(current) {
		return nil
	}
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

// previousBrandingAsset folds the matching set and clear facts strictly before
// the triggering change. A delayed or repeated job sees the same predecessor
// regardless of later writes or the current state of the config projection.
func (c *ChattoCore) previousBrandingAsset(ctx context.Context, before uint64, setType, clearType string) (*evtv1.AssetRecord, error) {
	config := evtstream.ConfigSubjectAggregate(ConfigSubjectServer)
	setSubject, clearSubject := config.Subject(setType), config.Subject(clearType)
	var previous []byte
	var after uint64
history:
	for {
		page, err := c.EventPublisher.SubjectRecordsAfterPage(ctx, config.AllEventsFilter(), after, 128, 1<<20)
		if err != nil {
			return nil, err
		}
		for _, record := range page.Records {
			if record.Sequence >= before {
				break history
			}
			switch record.Subject {
			case clearSubject:
				previous = nil
			case setSubject:
				previous = record.Data
			}
		}
		if !page.More {
			return nil, fmt.Errorf("branding history did not reach source sequence %d", before)
		}
		after = page.LastSequence
	}

	if previous == nil {
		return nil, nil
	}
	event, err := decodeDurableCoreDelivery(events.DurableDelivery{Data: previous})
	if err != nil {
		return nil, err
	}
	asset := cmp.Or(event.GetServerLogoSet().GetAsset(), event.GetServerBannerSet().GetAsset())
	if evtstream.EventTypeOf(event) != setType || asset == nil {
		return nil, events.TerminateDelivery("invalid branding change", errors.New("invalid preceding set event"))
	}
	return asset, nil
}
