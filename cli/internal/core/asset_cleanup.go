package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

func (s *AssetModel) configureCleanup(ctx context.Context, stream jetstream.Stream) error {
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:            assetCleanupConsumerName,
		Durable:         assetCleanupConsumerName,
		Description:     "Shared durable queue for Chatto asset deletion",
		DeliverPolicy:   jetstream.DeliverAllPolicy,
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         assetCleanupAckWait,
		MaxDeliver:      -1,
		FilterSubject:   evtstream.AssetEventTypeFilter(evtstream.EventAssetDeleted),
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   assetCleanupMaxPending,
		MaxRequestBatch: assetCleanupMaxPending,
	})
	if err != nil {
		return fmt.Errorf("create asset cleanup consumer: %w", err)
	}
	worker, err := events.NewDurableWorker(consumer, s.processCleanupDelivery, events.DurableWorkerOptions{
		MaxConcurrent:     assetCleanupMaxPending,
		RetryDelay:        assetCleanupRetryDelay,
		AckTimeout:        assetCleanupAckTimeout,
		HeartbeatInterval: assetCleanupHeartbeat,
		Logger:            s.logger.WithPrefix("core.AssetCleanup"),
	})
	if err != nil {
		return fmt.Errorf("configure asset cleanup worker: %w", err)
	}
	s.cleanupConsumer = consumer
	s.cleanupWorker = worker
	return nil
}

// Run starts shared, recoverable physical cleanup for asset deletion facts.
func (s *AssetModel) Run(ctx context.Context) error {
	if s == nil || s.cleanupWorker == nil {
		return fmt.Errorf("asset cleanup worker is not configured")
	}
	return s.cleanupWorker.Run(ctx)
}

func (s *AssetModel) processCleanupDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	deleted := event.GetAssetDeleted()
	assetID, ok := evtstream.ParseAssetSubject(delivery.Subject)
	if !ok || deleted == nil || deleted.GetAssetId() == "" || assetID != deleted.GetAssetId() {
		return events.TerminateDelivery("invalid asset deletion request", errors.New("asset deletion subject and payload do not match"))
	}
	// HLS repair consults projected child state. A worker can receive retained
	// deletion facts while the projection is still replaying at startup, so the
	// delivery itself is the barrier that makes all earlier creation facts safe
	// to read before we decide a child was already tombstoned.
	if err := s.waitForAssets(ctx, events.SubjectPosition(delivery.Subject, delivery.StreamSequence)); err != nil {
		return fmt.Errorf("wait for asset cleanup projection boundary: %w", err)
	}
	return s.cleanupDeletedAsset(ctx, &evtstream.SubjectEvent{Subject: delivery.Subject, Event: event})
}

func (s *AssetModel) cleanupDeletedAsset(ctx context.Context, subjectEvent *evtstream.SubjectEvent) error {
	event := subjectEvent.Event
	deleted := event.GetAssetDeleted()
	if deleted == nil || deleted.GetAssetId() == "" {
		return nil
	}
	aggregateAssetID, ok := evtstream.ParseAssetSubject(subjectEvent.Subject)
	if !ok || aggregateAssetID != deleted.GetAssetId() {
		return fmt.Errorf(
			"asset deletion subject %q does not match payload id %q",
			subjectEvent.Subject,
			deleted.GetAssetId(),
		)
	}
	createdEvents, _, err := s.EventPublisher.SubjectEvents(
		ctx,
		evtstream.AssetAggregate(deleted.GetAssetId()).Subject(evtstream.EventAssetCreated),
	)
	if err != nil {
		return fmt.Errorf("read creation fact for asset %s: %w", deleted.GetAssetId(), err)
	}
	if len(createdEvents) == 0 {
		// Beta room-scoped histories cannot be located from the asset ID alone.
		return nil
	}
	if err := s.reconcileDeletedAssetHLSDerivatives(ctx, event, deleted.GetAssetId()); err != nil {
		return err
	}
	created := createdEvents[len(createdEvents)-1].GetAssetCreated()
	if created.GetAsset().GetId() != deleted.GetAssetId() {
		return fmt.Errorf(
			"asset creation id %q does not match deletion aggregate %q",
			created.GetAsset().GetId(),
			deleted.GetAssetId(),
		)
	}
	if err := s.validateCleanupStorage(deleted.GetAssetId(), created.GetAsset()); err != nil {
		return err
	}
	attachment := attachmentFromAsset(created.GetAsset())
	if attachment == nil {
		return fmt.Errorf("asset creation %s has invalid storage metadata", deleted.GetAssetId())
	}
	if err := s.mediaModel.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		return fmt.Errorf("delete asset %s from storage: %w", deleted.GetAssetId(), err)
	}
	return nil
}

// reconcileDeletedAssetHLSDerivatives repairs mixed-version deletion. An older
// replica can read an additive HLS manifest as MP4-only and tombstone the source
// without tombstoning the HLS children. The durable cleanup consumer can still
// recover those child IDs from the source aggregate after an upgrade and append
// their deletion facts before removing the source bytes.
func (s *AssetModel) reconcileDeletedAssetHLSDerivatives(ctx context.Context, sourceEvent *corev1.Event, sourceAssetID string) error {
	processedEvents, _, err := s.EventPublisher.SubjectEvents(
		ctx,
		evtstream.AssetAggregate(sourceAssetID).Subject(evtstream.EventAssetProcessingSucceeded),
	)
	if err != nil {
		return fmt.Errorf("read processing manifest for deleted asset %s: %w", sourceAssetID, err)
	}
	if len(processedEvents) == 0 {
		return nil
	}
	processed := processedEvents[len(processedEvents)-1].GetAssetProcessingSucceeded()
	if processed.GetAssetId() != sourceAssetID {
		return fmt.Errorf("processing manifest id %q does not match deleted asset %q", processed.GetAssetId(), sourceAssetID)
	}
	hls := processed.GetVideo().GetHls()
	if hls == nil {
		return nil
	}

	type derivativeRef struct {
		assetID string
		role    corev1.AssetDerivativeRole
	}
	var refs []derivativeRef
	for _, rendition := range hls.GetRenditions() {
		if rendition == nil {
			continue
		}
		for _, segment := range rendition.GetSegments() {
			if segment == nil {
				continue
			}
			refs = append(refs, derivativeRef{
				assetID: segment.GetAssetId(),
				role:    corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT,
			})
		}
	}

	actorID := sourceEvent.GetActorId()
	if actorID == "" {
		actorID = SystemActorID
	}
	for _, ref := range refs {
		if ref.assetID == "" {
			continue
		}
		declared, ok := s.AssetCreation(ref.assetID)
		if !ok {
			// The child was already tombstoned by an HLS-aware replica.
			continue
		}
		if declared.GetParentAssetId() != sourceAssetID || declared.GetDerivativeRole() != ref.role {
			return fmt.Errorf("deleted asset %s has invalid HLS derivative reference %s", sourceAssetID, ref.assetID)
		}
		if err := s.DeleteAsset(ctx, actorID, ref.assetID); err != nil {
			return fmt.Errorf("tombstone HLS derivative %s of deleted asset %s: %w", ref.assetID, sourceAssetID, err)
		}
	}
	return nil
}

func (s *AssetModel) validateCleanupStorage(assetID string, asset *corev1.AssetRecord) error {
	switch {
	case asset.GetNats() != nil:
		if asset.GetNats().GetKey() != assetID {
			return fmt.Errorf("asset %s has non-canonical NATS key %q", assetID, asset.GetNats().GetKey())
		}
	case asset.GetS3() != nil:
		if s.s3Client == nil {
			return fmt.Errorf("asset %s uses S3 but no S3 client is configured", assetID)
		}
		validKey := false
		for _, candidate := range legacyAttachmentS3KeyCandidates(assetID) {
			if asset.GetS3().GetKey() == candidate {
				validKey = true
				break
			}
		}
		if !validKey {
			return fmt.Errorf("asset %s has non-canonical S3 key %q", assetID, asset.GetS3().GetKey())
		}
		if asset.GetS3().GetBucket() != "" && asset.GetS3().GetBucket() != s.s3Client.Bucket() {
			return fmt.Errorf("asset %s has unexpected S3 bucket %q", assetID, asset.GetS3().GetBucket())
		}
	}
	return nil
}
