package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Run starts recoverable physical cleanup for message-owned asset deletion
// facts. Lease handover is safe because storage deletion is idempotent and a
// fresh consumer replays the durable event history.
func (s *AssetModel) Run(ctx context.Context) error {
	if s == nil || s.cleanupLease == nil {
		return fmt.Errorf("asset cleanup lease is not configured")
	}
	return s.cleanupLease.Run(ctx, s.runCleanupLoop)
}

func (s *AssetModel) runCleanupLoop(ctx context.Context) error {
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(assetCleanupHeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.writeAssetCleanupStatus(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("Failed to publish asset cleanup heartbeat", "error", err)
				}
			}
		}
	}()
	defer func() { <-heartbeatDone }()

	s.runAssetCleanupPass(ctx)
	ticker := time.NewTicker(s.cleanupPollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.runAssetCleanupPass(ctx)
		}
	}
}

func (s *AssetModel) runAssetCleanupPass(ctx context.Context) {
	s.setAssetCleanupPassStarted()
	if err := s.writeAssetCleanupStatus(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("Failed to publish asset cleanup pass start", "error", err)
	}

	err := s.consumeAssetCleanup(ctx)
	s.setAssetCleanupPassFinished(err)
	if writeErr := s.writeAssetCleanupStatus(ctx); writeErr != nil && ctx.Err() == nil {
		s.logger.Warn("Failed to publish asset cleanup pass result", "error", writeErr)
	}
	if err != nil && ctx.Err() == nil {
		s.logger.Warn("Asset cleanup pass failed", "error", err)
	}
}

func (s *AssetModel) consumeAssetCleanup(ctx context.Context) error {
	if s == nil || s.cleanupConsumer == nil || s.failedVideoCleanupConsumer == nil || s.derivativeCleanupConsumer == nil || s.processingCommitReconciliationConsumer == nil {
		return fmt.Errorf("asset cleanup consumer is not configured")
	}
	// Failed generations publish their durable cleanup intent first. Process it
	// before deletion facts so newly appended tombstones can remove bytes in the
	// same pass.
	failureErr := s.failedVideoCleanupConsumer.Consume(ctx)
	requestErr := s.derivativeCleanupConsumer.Consume(ctx)
	reconciliationErr := s.processingCommitReconciliationConsumer.Consume(ctx)
	deletionErr := s.cleanupConsumer.Consume(ctx)
	return errors.Join(failureErr, requestErr, reconciliationErr, deletionErr)
}

func (s *AssetModel) reconcileUnknownVideoProcessingCommit(ctx context.Context, subjectEvent *events.SubjectEvent) error {
	event := subjectEvent.Event
	requested := event.GetAssetProcessingCommitReconciliationRequested()
	if requested == nil || requested.GetSourceAssetId() == "" || requested.GetAttemptedEventId() == "" {
		return nil
	}
	sourceAssetID, ok := events.ParseAssetSubject(subjectEvent.Subject)
	if !ok || sourceAssetID != requested.GetSourceAssetId() {
		return fmt.Errorf(
			"asset processing reconciliation subject %q does not match payload source id %q",
			subjectEvent.Subject,
			requested.GetSourceAssetId(),
		)
	}
	createdAt := event.GetCreatedAt()
	if createdAt == nil || !createdAt.IsValid() {
		return fmt.Errorf("asset processing reconciliation %s has no valid creation time", event.GetId())
	}
	if readyAt := createdAt.AsTime().Add(assetCommitReconciliationGrace); time.Now().Before(readyAt) {
		return fmt.Errorf("asset processing reconciliation %s is waiting for commit grace period", event.GetId())
	}
	committed, err := s.assetEventIDExists(ctx, sourceAssetID, events.EventAssetProcessingSucceeded, requested.GetAttemptedEventId())
	if err != nil {
		return err
	}
	if committed {
		return nil
	}
	roomID, err := s.durableAssetRoomID(ctx, sourceAssetID)
	if err != nil {
		return err
	}
	err = s.recordAssetProcessingFailedWithCleanup(
		ctx,
		event.GetActorId(),
		roomID,
		requested.GetMessageEventId(),
		sourceAssetID,
		corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED,
		requested.GetCleanupAssetIds(),
	)
	if errors.Is(err, ErrAssetLifecycleSkipped) {
		committed, confirmErr := s.assetEventIDExists(ctx, sourceAssetID, events.EventAssetProcessingSucceeded, requested.GetAttemptedEventId())
		if confirmErr != nil {
			return confirmErr
		}
		if committed {
			return nil
		}
		return s.RecordAssetDerivativeCleanupRequested(ctx, event.GetActorId(), sourceAssetID, requested.GetCleanupAssetIds())
	}
	if err != nil {
		return err
	}
	return s.cleanupVideoDerivativeIDs(ctx, event.GetActorId(), sourceAssetID, requested.GetCleanupAssetIds())
}

func (s *AssetModel) assetEventIDExists(ctx context.Context, assetID, eventType, eventID string) (bool, error) {
	published, _, err := s.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(assetID).Subject(eventType))
	if err != nil {
		return false, fmt.Errorf("read durable %s events for asset %s: %w", eventType, assetID, err)
	}
	for _, candidate := range published {
		if candidate.GetId() == eventID {
			return true, nil
		}
	}
	return false, nil
}

func (s *AssetModel) durableAssetRoomID(ctx context.Context, assetID string) (string, error) {
	created, _, err := s.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(assetID).Subject(events.EventAssetCreated))
	if err != nil {
		return "", fmt.Errorf("read creation fact for asset %s: %w", assetID, err)
	}
	if len(created) == 0 || created[len(created)-1].GetAssetCreated().GetRoomId() == "" {
		return "", fmt.Errorf("asset %s has no durable room scope", assetID)
	}
	return created[len(created)-1].GetAssetCreated().GetRoomId(), nil
}

func (s *AssetModel) cleanupFailedVideoDerivatives(ctx context.Context, subjectEvent *events.SubjectEvent) error {
	event := subjectEvent.Event
	failed := event.GetAssetProcessingFailed()
	if failed == nil || failed.GetAssetId() == "" {
		return nil
	}
	sourceAssetID, ok := events.ParseAssetSubject(subjectEvent.Subject)
	if !ok || sourceAssetID != failed.GetAssetId() {
		return fmt.Errorf(
			"asset processing failure subject %q does not match payload id %q",
			subjectEvent.Subject,
			failed.GetAssetId(),
		)
	}
	return s.cleanupVideoDerivativeIDs(ctx, event.GetActorId(), sourceAssetID, failed.GetCleanupAssetIds())
}

func (s *AssetModel) cleanupRequestedVideoDerivatives(ctx context.Context, subjectEvent *events.SubjectEvent) error {
	event := subjectEvent.Event
	requested := event.GetAssetDerivativeCleanupRequested()
	if requested == nil || requested.GetSourceAssetId() == "" {
		return nil
	}
	sourceAssetID, ok := events.ParseAssetSubject(subjectEvent.Subject)
	if !ok || sourceAssetID != requested.GetSourceAssetId() {
		return fmt.Errorf(
			"asset derivative cleanup subject %q does not match payload source id %q",
			subjectEvent.Subject,
			requested.GetSourceAssetId(),
		)
	}
	return s.cleanupVideoDerivativeIDs(ctx, event.GetActorId(), sourceAssetID, requested.GetCleanupAssetIds())
}

func (s *AssetModel) cleanupVideoDerivativeIDs(ctx context.Context, actorID, sourceAssetID string, cleanupAssetIDs []string) error {
	if actorID == "" {
		actorID = SystemActorID
	}
	sourceCreatedEvents, _, err := s.EventPublisher.SubjectEvents(
		ctx,
		events.AssetAggregate(sourceAssetID).Subject(events.EventAssetCreated),
	)
	if err != nil {
		return fmt.Errorf("read creation fact for failed source asset %s: %w", sourceAssetID, err)
	}
	if len(sourceCreatedEvents) == 0 {
		return fmt.Errorf("failed source asset %s has no canonical creation fact", sourceAssetID)
	}
	sourceCreated := sourceCreatedEvents[len(sourceCreatedEvents)-1].GetAssetCreated()
	if sourceCreated.GetAsset().GetId() != sourceAssetID {
		return fmt.Errorf("failed source creation id %q does not match aggregate %q", sourceCreated.GetAsset().GetId(), sourceAssetID)
	}
	sourceRoomID := sourceCreated.GetRoomId()
	for _, assetID := range cleanupAssetIDs {
		if assetID == "" {
			continue
		}
		deletedEvents, _, err := s.EventPublisher.SubjectEvents(
			ctx,
			events.AssetAggregate(assetID).Subject(events.EventAssetDeleted),
		)
		if err != nil {
			return fmt.Errorf("read deletion facts for failed derivative %s: %w", assetID, err)
		}
		if len(deletedEvents) > 0 {
			continue
		}
		createdEvents, _, err := s.EventPublisher.SubjectEvents(
			ctx,
			events.AssetAggregate(assetID).Subject(events.EventAssetCreated),
		)
		if err != nil {
			return fmt.Errorf("read creation fact for failed derivative %s: %w", assetID, err)
		}
		if len(createdEvents) == 0 {
			return fmt.Errorf("failed derivative %s has no canonical creation fact", assetID)
		}
		declared := createdEvents[len(createdEvents)-1].GetAssetCreated()
		if declared.GetAsset().GetId() != assetID {
			return fmt.Errorf("failed derivative creation id %q does not match aggregate %q", declared.GetAsset().GetId(), assetID)
		}
		if declared.GetParentAssetId() != sourceAssetID {
			return fmt.Errorf("failed asset %s has invalid cleanup derivative %s", sourceAssetID, assetID)
		}
		roomID := declared.GetRoomId()
		if roomID == "" {
			roomID = sourceRoomID
		}
		if roomID == "" {
			return fmt.Errorf("failed derivative %s has no durable room scope", assetID)
		}
		if err := s.RecordAssetDeleted(ctx, actorID, roomID, assetID); err != nil {
			return fmt.Errorf("tombstone failed derivative %s of asset %s: %w", assetID, sourceAssetID, err)
		}
	}
	return nil
}

func (s *AssetModel) cleanupDeletedAsset(ctx context.Context, subjectEvent *events.SubjectEvent) error {
	event := subjectEvent.Event
	deleted := event.GetAssetDeleted()
	if deleted == nil || deleted.GetAssetId() == "" {
		return nil
	}
	aggregateAssetID, ok := events.ParseAssetSubject(subjectEvent.Subject)
	if !ok || aggregateAssetID != deleted.GetAssetId() {
		return fmt.Errorf(
			"asset deletion subject %q does not match payload id %q",
			subjectEvent.Subject,
			deleted.GetAssetId(),
		)
	}
	createdEvents, _, err := s.EventPublisher.SubjectEvents(
		ctx,
		events.AssetAggregate(deleted.GetAssetId()).Subject(events.EventAssetCreated),
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
	if err := s.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
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
		events.AssetAggregate(sourceAssetID).Subject(events.EventAssetProcessingSucceeded),
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
