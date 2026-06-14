package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var ErrAssetLifecycleSkipped = errors.New("asset lifecycle event skipped")

// derivativeContext records that an upload is a derivative of another asset.
type derivativeContext struct {
	parentAssetID  string
	derivativeRole corev1.AssetDerivativeRole
}

// AssetService owns durable asset lifecycle facts and invariants.
//
// MediaService owns bytes, URLs, transforms, and caches. AssetService owns the
// event-sourced asset aggregate: creation facts, processing transitions,
// tombstones, derivative cleanup ordering, and projection read-your-writes.
type AssetService struct {
	*ChattoCore
}

func NewAssetService(core *ChattoCore) *AssetService {
	return &AssetService{ChattoCore: core}
}

func (c *ChattoCore) assetLifecycle() *AssetService {
	if c.assetService == nil {
		c.assetService = NewAssetService(c)
	}
	return c.assetService
}

// RecordUploadedAsset writes the AssetCreatedEvent for a user-uploaded binary.
func (s *AssetService) RecordUploadedAsset(ctx context.Context, actorID, roomID string, attachment *corev1.Attachment) error {
	if actorID == "" {
		return fmt.Errorf("asset creation missing actor id")
	}
	return s.recordAssetCreated(ctx, actorID, roomID, attachment, nil)
}

// RecordDerivativeAsset writes the AssetCreatedEvent for a worker-generated
// derivative such as a thumbnail or transcoded variant.
func (s *AssetService) RecordDerivativeAsset(ctx context.Context, parentAssetID string, derivativeRole corev1.AssetDerivativeRole, roomID string, attachment *corev1.Attachment) error {
	if parentAssetID == "" {
		return fmt.Errorf("derivative asset creation missing parent asset id")
	}
	deriv := &derivativeContext{parentAssetID: parentAssetID, derivativeRole: derivativeRole}
	return s.recordAssetCreated(ctx, SystemActorID, roomID, attachment, deriv)
}

func (s *AssetService) recordAssetCreated(ctx context.Context, actorID, roomID string, attachment *corev1.Attachment, deriv *derivativeContext) error {
	created := &corev1.AssetCreatedEvent{
		Asset:                   assetFromAttachment(attachment),
		OriginalBinaryAvailable: true,
		RoomId:                  roomID,
	}
	if deriv != nil {
		created.ParentAssetId = deriv.parentAssetID
		created.DerivativeRole = deriv.derivativeRole
	} else {
		created.UserId = actorID
	}
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetCreated{AssetCreated: created},
	})
	if err := s.appendAssetEventEventually(ctx, attachment.GetId(), event); err != nil {
		return fmt.Errorf("publish asset creation event: %w", err)
	}
	return nil
}

// DeleteVideoDerivativesForAttachment deletes generated thumbnail/variant
// binaries for a processed video attachment and emits AssetDeletedEvent for
// each derivative. The durable processing manifest remains in EVT for
// audit/replay; deletion makes future signed URLs resolve to 404.
func (s *AssetService) DeleteVideoDerivativesForAttachment(ctx context.Context, actorID string, attachmentID string) {
	manifest, ok := s.Assets.VideoAttachmentManifest(attachmentID)
	if !ok || manifest == nil || manifest.Succeeded == nil {
		return
	}
	video := manifest.Succeeded.GetVideo()
	if video == nil {
		return
	}
	deleteDerivative := func(id string) {
		if id == "" {
			return
		}
		declared, ok := s.Assets.AssetCreation(id)
		if !ok {
			return
		}
		att := attachmentFromAsset(declared.GetAsset())
		if err := s.DeleteAsset(ctx, actorID, id); err != nil {
			s.logger.Warn("Failed to publish derivative asset deletion event",
				"attachment_id", id,
				"origin_attachment_id", attachmentID,
				"error", err)
			return
		}
		if err := s.media().DeleteAttachmentFromStorage(ctx, att); err != nil {
			s.logger.Warn("Failed to delete video derivative binary",
				"attachment_id", att.GetId(),
				"origin_attachment_id", attachmentID,
				"error", err)
		}
	}
	deleteDerivative(video.GetThumbnailAssetId())
	for _, variant := range video.Variants {
		deleteDerivative(variant.GetAssetId())
	}
}

// DeleteMessageOwnedAssetsForUser removes every currently projected
// message-owned asset for userID, including derivative children such as video
// thumbnails and variants. AssetDeletedEvent is appended before the backing
// bytes are removed so serving paths stop resolving the asset even if storage
// cleanup is slow or partially fails.
func (s *AssetService) DeleteMessageOwnedAssetsForUser(ctx context.Context, actorID, userID string) int {
	owned := s.RoomTimeline.MessageAssetsByAuthor(userID)
	deleted := 0
	seen := make(map[string]struct{})
	type deletionTarget struct {
		assetID    string
		roomID     string
		attachment *corev1.Attachment
	}
	var targets []deletionTarget

	for _, ref := range owned {
		subtree := s.Assets.AssetSubtreeIDs(ref.AssetID)
		for i := len(subtree) - 1; i >= 0; i-- {
			assetID := subtree[i]
			if assetID == "" {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}

			declared, ok := s.Assets.AssetCreation(assetID)
			if !ok || declared == nil {
				continue
			}
			roomID := assetCreatedRoomID(declared)
			if roomID == "" {
				roomID = ref.RoomID
			}
			if roomID == "" {
				continue
			}
			targets = append(targets, deletionTarget{
				assetID:    assetID,
				roomID:     roomID,
				attachment: attachmentFromAsset(declared.GetAsset()),
			})
		}
	}

	for _, target := range targets {
		if err := s.RecordAssetDeleted(ctx, actorID, target.roomID, target.assetID); err != nil {
			s.logger.Warn("Failed to publish asset deletion event during user asset cleanup",
				"asset_id", target.assetID,
				"room_id", target.roomID,
				"user_id", userID,
				"error", err)
			continue
		}
		if target.attachment != nil {
			if err := s.media().DeleteAttachmentFromStorage(ctx, target.attachment); err != nil {
				s.logger.Warn("Failed to delete attachment during user asset cleanup",
					"asset_id", target.assetID,
					"room_id", target.roomID,
					"user_id", userID,
					"error", err)
			}
		}
		deleted++
	}
	return deleted
}

// ScheduleVideoProcessingForMessageAttachment enqueues async processing for a
// message-owned video asset. It appends a durable AssetProcessingStartedEvent,
// then calls the process-local video processing hook.
func (s *AssetService) ScheduleVideoProcessingForMessageAttachment(ctx context.Context, actorID string, roomID, messageEventID string, attachment *corev1.Attachment) error {
	if roomID == "" || messageEventID == "" || attachment == nil || attachment.GetId() == "" {
		return fmt.Errorf("video processing missing room, message, or attachment")
	}
	if manifest, ok := s.Assets.VideoAttachmentManifest(attachment.GetId()); ok && manifest != nil {
		if manifest.Succeeded != nil || manifest.Failed != nil {
			return nil
		}
	}
	if s.attachmentBinaryStatus(ctx, attachment) == AttachmentBinaryMissing {
		return s.RecordAssetProcessingFailed(ctx, actorID, roomID, messageEventID, attachment.GetId(), corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING)
	}
	if err := s.RecordAssetProcessingStarted(ctx, actorID, roomID, messageEventID, attachment.GetId()); err != nil {
		return err
	}
	if s.OnVideoProcessingRequested == nil {
		s.logger.Warn("Video processing requested but no local processor is registered",
			"asset_id", attachment.GetId(),
			"message_event_id", messageEventID)
		return nil
	}
	if err := s.OnVideoProcessingRequested(context.Background(), attachment.GetId(), messageEventID); err != nil {
		s.logger.Warn("Failed to start local video processing",
			"asset_id", attachment.GetId(),
			"message_event_id", messageEventID,
			"error", err)
	}
	return nil
}

// RecordAssetProcessingStarted appends a durable AssetProcessingStartedEvent.
func (s *AssetService) RecordAssetProcessingStarted(ctx context.Context, actorID string, roomID, messageEventID, assetID string) error {
	if roomID == "" || assetID == "" {
		return fmt.Errorf("asset processing started missing room or asset id")
	}
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{
				AssetId:        assetID,
				MessageEventId: messageEventID,
			},
		},
	})
	return s.PublishAssetProcessing(ctx, roomID, event)
}

// RecoverUnmanifestedVideoAttachments replays durable message attachments into
// the in-process video processor when they have no completed/failed manifest
// yet. If the original binary is already gone, it records a durable unavailable
// state.
func (s *AssetService) RecoverUnmanifestedVideoAttachments(ctx context.Context) {
	for _, req := range s.UnmanifestedVideoAttachments() {
		if req.Attachment == nil {
			continue
		}
		if err := s.ScheduleVideoProcessingForMessageAttachment(ctx, SystemActorID, req.RoomID, req.MessageEventID, req.Attachment); err != nil {
			s.logger.Warn("Failed to recover video processing", "attachment_id", req.Attachment.GetId(), "error", err)
		}
	}
}

func (s *AssetService) UnmanifestedVideoAttachments() []VideoProcessingRequest {
	var out []VideoProcessingRequest
	for _, owner := range s.RoomTimeline.MessageAssetOwners() {
		if owner.RoomID == "" || owner.MessageEventID == "" || owner.AssetID == "" {
			continue
		}
		if s.RoomTimeline.MessageTombstoned(owner.MessageEventID) {
			continue
		}
		declared, ok := s.Assets.AssetCreation(owner.AssetID)
		if !ok || declared == nil {
			continue
		}
		asset := declared.GetAsset()
		if asset == nil {
			continue
		}
		if _, hasManifest := s.Assets.VideoAttachmentManifest(owner.AssetID); hasManifest {
			continue
		}
		contentType := asset.GetContentType()
		if !strings.HasPrefix(contentType, "video/") && contentType != "image/gif" {
			continue
		}
		out = append(out, VideoProcessingRequest{
			RoomID:         owner.RoomID,
			MessageEventID: owner.MessageEventID,
			Attachment:     attachmentFromAsset(asset),
		})
	}
	return out
}

// PublishAssetProcessing appends a durable asset-processing event to EVT.
// Refuses events with an empty ActorId; every asset lifecycle event must be
// attributable to a user or SystemActorID.
func (s *AssetService) PublishAssetProcessing(ctx context.Context, roomID string, event *corev1.Event) error {
	if err := s.publishAssetProcessing(ctx, roomID, event); err != nil {
		if errors.Is(err, ErrAssetLifecycleSkipped) {
			return nil
		}
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	return nil
}

func (s *AssetService) publishAssetProcessing(ctx context.Context, roomID string, event *corev1.Event) error {
	if roomID == "" {
		return fmt.Errorf("asset processing event missing room id")
	}
	if event.GetActorId() == "" {
		return fmt.Errorf("asset processing event missing actor id (use SystemActorID for non-user paths)")
	}
	assetID := assetIDOfLifecycleEvent(event)
	if assetID == "" {
		return fmt.Errorf("asset processing event missing asset id")
	}
	if assetRoomID, ok := s.Assets.AssetRoomID(assetID); ok && assetRoomID != roomID {
		return fmt.Errorf("asset processing event room mismatch: asset room %s, event room %s", assetRoomID, roomID)
	}
	if err := s.appendAssetProcessingEvent(ctx, assetID, event); err != nil {
		return err
	}
	return nil
}

// RecordAssetProcessed builds and publishes a durable processed-video
// manifest for an original video attachment. If the terminal manifest is
// skipped because another terminal/deleted state already won, derivative
// outputs passed to this call are tombstoned and storage-cleaned.
func (s *AssetService) RecordAssetProcessed(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	thumbnailAssetID := ""
	if thumbnail != nil {
		thumbnailAssetID = thumbnail.GetId()
	}
	assetVariants := make([]*corev1.AssetVideoVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetAttachment() == nil {
			continue
		}
		assetVariants = append(assetVariants, &corev1.AssetVideoVariant{
			Quality: variant.GetQuality(),
			AssetId: variant.GetAttachment().GetId(),
		})
	}
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId:        attachmentID,
				MessageEventId: messageEventID,
				Video: &corev1.AssetProcessedVideo{
					DurationMs:       durationMs,
					Width:            width,
					Height:           height,
					ThumbnailAssetId: thumbnailAssetID,
					Variants:         assetVariants,
				},
			},
		},
	})
	if err := s.publishAssetProcessing(ctx, roomID, event); err != nil {
		if errors.Is(err, ErrAssetLifecycleSkipped) {
			s.cleanupVideoDerivativeOutputs(ctx, actorID, roomID, attachmentID, thumbnail, variants)
			return nil
		}
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	return nil
}

func (s *AssetService) cleanupVideoDerivativeOutputs(ctx context.Context, actorID string, roomID, originAssetID string, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) {
	s.cleanupVideoDerivativeOutput(ctx, actorID, roomID, originAssetID, thumbnail)
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		s.cleanupVideoDerivativeOutput(ctx, actorID, roomID, originAssetID, variant.GetAttachment())
	}
}

func (s *AssetService) cleanupVideoDerivativeOutput(ctx context.Context, actorID string, fallbackRoomID, originAssetID string, attachment *corev1.Attachment) {
	if attachment == nil || attachment.GetId() == "" {
		return
	}
	assetID := attachment.GetId()
	roomID := fallbackRoomID
	if projectedRoomID, ok := s.Assets.AssetRoomID(assetID); ok && projectedRoomID != "" {
		roomID = projectedRoomID
	}
	if roomID != "" {
		if err := s.RecordAssetDeleted(ctx, actorID, roomID, assetID); err != nil {
			s.logger.Warn("Failed to publish derivative asset deletion event after skipped video manifest",
				"attachment_id", assetID,
				"origin_attachment_id", originAssetID,
				"error", err)
			return
		}
	}
	if err := s.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		s.logger.Warn("Failed to delete derivative binary after skipped video manifest",
			"attachment_id", assetID,
			"origin_attachment_id", originAssetID,
			"error", err)
	}
}

// DeleteAsset appends an AssetDeletedEvent for a projected asset.
func (s *AssetService) DeleteAsset(ctx context.Context, actorID, assetID string) error {
	roomID, ok := s.Assets.AssetRoomID(assetID)
	if !ok {
		return fmt.Errorf("asset deletion missing room scope")
	}
	return s.RecordAssetDeleted(ctx, actorID, roomID, assetID)
}

// RecordAssetDeleted appends a durable AssetDeletedEvent in the asset aggregate.
func (s *AssetService) RecordAssetDeleted(ctx context.Context, actorID string, roomID, assetID string) error {
	if roomID == "" || assetID == "" {
		return fmt.Errorf("asset deletion missing room or asset id")
	}
	if actorID == "" {
		return fmt.Errorf("asset deletion missing actor id (use SystemActorID for non-user paths)")
	}
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: assetID},
		},
	})
	if err := s.appendAssetEventEventually(ctx, assetID, event); err != nil {
		return fmt.Errorf("publish asset deletion event: %w", err)
	}
	return nil
}

func (s *AssetService) appendAssetEventEventually(ctx context.Context, assetID string, event *corev1.Event) error {
	if assetID == "" {
		return fmt.Errorf("asset event missing asset id")
	}
	subject := events.AssetAggregate(assetID).SubjectFor(event)
	seq, err := s.EventPublisher.AppendEventually(ctx, subject, event)
	if err != nil {
		return err
	}
	pos := events.SubjectPosition(subject, seq)
	return waitForPositionAll(ctx, pos, waitForProjection("assets", s.AssetsProjector))
}

func (s *AssetService) appendAssetProcessingEvent(ctx context.Context, assetID string, event *corev1.Event) error {
	if assetID == "" {
		return fmt.Errorf("asset event missing asset id")
	}
	for attempt := 0; attempt < 5; attempt++ {
		agg := events.AssetAggregate(assetID)
		filter := agg.AllEventsFilter()
		tail, err := s.EventPublisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return err
		}
		if !tail.IsZero() {
			if err := waitForPositionAll(ctx, tail, waitForProjection("assets", s.AssetsProjector)); err != nil {
				return err
			}
		}
		if !s.shouldAppendAssetProcessingEvent(assetID, event) {
			return ErrAssetLifecycleSkipped
		}
		subject := agg.SubjectFor(event)
		seq, err := s.EventPublisher.AppendAtFilter(ctx, subject, event, filter, tail.Seq)
		if err == nil {
			return waitForPositionAll(ctx, events.SubjectPosition(subject, seq), waitForProjection("assets", s.AssetsProjector))
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("append asset processing event after retries: %w", events.ErrConflict)
}

func (s *AssetService) shouldAppendAssetProcessingEvent(assetID string, event *corev1.Event) bool {
	if s.Assets.AssetDeleted(assetID) {
		return false
	}
	manifest, hasManifest := s.Assets.VideoAttachmentManifest(assetID)
	switch event.GetEvent().(type) {
	case *corev1.Event_AssetProcessingStarted:
		return !hasManifest || manifest == nil || (manifest.Succeeded == nil && manifest.Failed == nil)
	case *corev1.Event_AssetProcessingSucceeded, *corev1.Event_AssetProcessingFailed:
		return !hasManifest || manifest == nil || (manifest.Succeeded == nil && manifest.Failed == nil)
	default:
		return true
	}
}

func assetIDOfLifecycleEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_AssetCreated:
		if ev.AssetCreated.GetAsset() == nil {
			return ""
		}
		return ev.AssetCreated.GetAsset().GetId()
	case *corev1.Event_AssetProcessingStarted:
		return ev.AssetProcessingStarted.GetAssetId()
	case *corev1.Event_AssetProcessingSucceeded:
		return ev.AssetProcessingSucceeded.GetAssetId()
	case *corev1.Event_AssetProcessingFailed:
		return ev.AssetProcessingFailed.GetAssetId()
	case *corev1.Event_AssetDeleted:
		return ev.AssetDeleted.GetAssetId()
	default:
		return ""
	}
}

// RecordAssetProcessingFailed builds and publishes a durable failed
// video-processing outcome.
func (s *AssetService) RecordAssetProcessingFailed(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:        attachmentID,
				MessageEventId: messageEventID,
				FailureCode:    failureCode,
			},
		},
	})
	return s.PublishAssetProcessing(ctx, roomID, event)
}
