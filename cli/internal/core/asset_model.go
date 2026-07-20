package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	ErrAssetLifecycleSkipped = errors.New("asset lifecycle event skipped")
	ErrAssetCommitUnknown    = errors.New("asset event commit status unknown")
	errAssetEventCommitted   = errors.New("asset event committed before follow-up failure")
)

const (
	assetCleanupLeaseName       = "asset_cleanup"
	assetCleanupLeaseTTL        = 45 * time.Second
	assetCleanupLeaseRenewEvery = 15 * time.Second
	assetCleanupLeaseRetryEvery = 5 * time.Second
	assetCleanupPollEvery       = 30 * time.Second
	assetCommitCheckTimeout     = 5 * time.Second
)

// derivativeContext records that an upload is a derivative of another asset.
type derivativeContext struct {
	parentAssetID  string
	derivativeRole corev1.AssetDerivativeRole
}

// AssetModel owns durable asset lifecycle facts and invariants.
//
// MediaModel owns bytes, URLs, transforms, and caches. AssetModel owns the
// event-sourced asset aggregate: creation facts, processing transitions,
// tombstones, derivative cleanup ordering, and projection read-your-writes.
type AssetModel struct {
	*ChattoCore
	cleanupLease          *lease.Lease
	cleanupConsumer       *events.IncrementalEffectConsumer
	cleanupPollEvery      time.Duration
	waitForAssetsOverride func(context.Context, events.StreamPosition) error
	cleanupStatusMu       sync.RWMutex
	cleanupPass           assetCleanupPassStatus
}

func NewAssetModel(core *ChattoCore) *AssetModel {
	model := &AssetModel{ChattoCore: core, cleanupPollEvery: assetCleanupPollEvery}
	if core != nil && core.EventPublisher != nil {
		model.cleanupConsumer = events.NewIncrementalEffectConsumerWithSubject(
			core.EventPublisher,
			events.AssetEventTypeFilter(events.EventAssetDeleted),
			model.cleanupDeletedAsset,
		)
	}
	return model
}

func (c *ChattoCore) assetLifecycle() *AssetModel {
	if c.assetModel == nil {
		c.assetModel = NewAssetModel(c)
	}
	return c.assetModel
}

// RecordUploadedAsset writes the AssetCreatedEvent for a user-uploaded binary.
func (s *AssetModel) RecordUploadedAsset(ctx context.Context, actorID, roomID string, attachment *corev1.Attachment) error {
	if actorID == "" {
		return fmt.Errorf("asset creation missing actor id")
	}
	return s.recordAssetCreated(ctx, actorID, roomID, attachment, nil, assetCreatedMetadata{})
}

// RecordUploadedPendingAttachmentAsset writes the AssetCreatedEvent for an
// attachment produced by the public chunked upload flow. The pending expiry is
// a cleanup hint until a MessageBody claims the asset ID.
func (s *AssetModel) RecordUploadedPendingAttachmentAsset(ctx context.Context, actorID, roomID string, attachment *corev1.Attachment, sha256 string, pendingExpiresAt time.Time, needsVideoProcessing bool) error {
	if actorID == "" {
		return fmt.Errorf("asset creation missing actor id")
	}
	return s.recordAssetCreated(ctx, actorID, roomID, attachment, nil, assetCreatedMetadata{
		sha256:               sha256,
		pendingExpiresAt:     pendingExpiresAt,
		needsVideoProcessing: needsVideoProcessing,
	})
}

// RecordDerivativeAsset writes the AssetCreatedEvent for a worker-generated
// derivative such as a thumbnail or transcoded variant.
func (s *AssetModel) RecordDerivativeAsset(ctx context.Context, parentAssetID string, derivativeRole corev1.AssetDerivativeRole, roomID string, attachment *corev1.Attachment) error {
	if parentAssetID == "" {
		return fmt.Errorf("derivative asset creation missing parent asset id")
	}
	deriv := &derivativeContext{parentAssetID: parentAssetID, derivativeRole: derivativeRole}
	return s.recordAssetCreated(ctx, SystemActorID, roomID, attachment, deriv, assetCreatedMetadata{})
}

type assetCreatedMetadata struct {
	sha256               string
	pendingExpiresAt     time.Time
	needsVideoProcessing bool
}

func (s *AssetModel) recordAssetCreated(ctx context.Context, actorID, roomID string, attachment *corev1.Attachment, deriv *derivativeContext, metadata assetCreatedMetadata) error {
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
	if metadata.sha256 != "" {
		created.Sha256 = metadata.sha256
	}
	if !metadata.pendingExpiresAt.IsZero() {
		created.PendingExpiresAt = timestamppb.New(metadata.pendingExpiresAt)
	}
	created.NeedsVideoProcessing = metadata.needsVideoProcessing
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetCreated{AssetCreated: created},
	})
	if err := s.appendAssetEventEventually(ctx, attachment.GetId(), event); err != nil {
		if errors.Is(err, errAssetEventCommitted) {
			return nil
		}
		committed, confirmErr := s.assetEventCommitted(ctx, attachment.GetId(), event)
		if confirmErr != nil {
			return errors.Join(
				fmt.Errorf("publish asset creation event: %w", err),
				fmt.Errorf("%w: %v", ErrAssetCommitUnknown, confirmErr),
			)
		}
		if committed {
			return nil
		}
		return fmt.Errorf("publish asset creation event: %w", err)
	}
	return nil
}

// DeleteVideoDerivativesForAttachment deletes generated thumbnail/variant
// binaries for a processed video attachment and emits AssetDeletedEvent for
// each derivative. The durable processing manifest remains in EVT for
// audit/replay; deletion makes future signed URLs resolve to 404.
func (s *AssetModel) DeleteVideoDerivativesForAttachment(ctx context.Context, actorID string, attachmentID string) {
	manifest, ok := s.VideoAttachmentManifest(attachmentID)
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
		declared, ok := s.AssetCreation(id)
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
	for _, assetID := range hlsDerivativeAssetIDs(video.GetHls()) {
		deleteDerivative(assetID)
	}
}

// DeleteMessageOwnedAssetsForUser removes every currently projected
// message-owned asset for userID, including derivative children such as video
// thumbnails and variants. AssetDeletedEvent is appended before the backing
// bytes are removed so serving paths stop resolving the asset even if storage
// cleanup is slow or partially fails.
func (s *AssetModel) DeleteMessageOwnedAssetsForUser(ctx context.Context, actorID, userID string) int {
	owned := s.MessageAssetsByAuthor(userID)
	deleted := 0
	seen := make(map[string]struct{})
	type deletionTarget struct {
		assetID    string
		roomID     string
		attachment *corev1.Attachment
	}
	var targets []deletionTarget

	for _, ref := range owned {
		subtree := s.AssetSubtreeIDs(ref.AssetID)
		for i := len(subtree) - 1; i >= 0; i-- {
			assetID := subtree[i]
			if assetID == "" {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}

			declared, ok := s.AssetCreation(assetID)
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
func (s *AssetModel) ScheduleVideoProcessingForMessageAttachment(ctx context.Context, actorID string, roomID, messageEventID string, attachment *corev1.Attachment) error {
	if roomID == "" || messageEventID == "" || attachment == nil || attachment.GetId() == "" {
		return fmt.Errorf("video processing missing room, message, or attachment")
	}
	if manifest, ok := s.VideoAttachmentManifest(attachment.GetId()); ok && manifest != nil {
		if manifest.Succeeded != nil || manifest.Failed != nil {
			return nil
		}
	}
	if s.OnVideoProcessingRequested == nil {
		return nil
	}
	if s.attachmentBinaryStatus(ctx, attachment) == AttachmentBinaryMissing {
		return s.RecordAssetProcessingFailed(ctx, actorID, roomID, messageEventID, attachment.GetId(), corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING)
	}
	if err := s.RecordAssetProcessingStarted(ctx, actorID, roomID, messageEventID, attachment.GetId()); err != nil {
		return err
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
func (s *AssetModel) RecordAssetProcessingStarted(ctx context.Context, actorID string, roomID, messageEventID, assetID string) error {
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
func (s *AssetModel) RecoverUnmanifestedVideoAttachments(ctx context.Context) {
	for _, req := range s.UnmanifestedVideoAttachments() {
		if req.Attachment == nil {
			continue
		}
		if err := s.ScheduleVideoProcessingForMessageAttachment(ctx, SystemActorID, req.RoomID, req.MessageEventID, req.Attachment); err != nil {
			s.logger.Warn("Failed to recover video processing", "attachment_id", req.Attachment.GetId(), "error", err)
		}
	}
}

func (s *AssetModel) UnmanifestedVideoAttachments() []VideoProcessingRequest {
	var out []VideoProcessingRequest
	for _, owner := range s.MessageAssetOwners() {
		if owner.RoomID == "" || owner.MessageEventID == "" || owner.AssetID == "" {
			continue
		}
		if s.MessageTombstoned(owner.MessageEventID) {
			continue
		}
		declared, ok := s.AssetCreation(owner.AssetID)
		if !ok || declared == nil {
			continue
		}
		asset := declared.GetAsset()
		if asset == nil {
			continue
		}
		manifest, hasManifest := s.VideoAttachmentManifest(owner.AssetID)
		if hasManifest && manifest != nil && (manifest.Succeeded != nil || manifest.Failed != nil) {
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
func (s *AssetModel) PublishAssetProcessing(ctx context.Context, roomID string, event *corev1.Event) error {
	if err := s.publishAssetProcessing(ctx, roomID, event); err != nil {
		if errors.Is(err, ErrAssetLifecycleSkipped) {
			return nil
		}
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	return nil
}

func (s *AssetModel) publishAssetProcessing(ctx context.Context, roomID string, event *corev1.Event) error {
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
	if assetRoomID, ok := s.AssetRoomID(assetID); ok && assetRoomID != roomID {
		return fmt.Errorf("asset processing event room mismatch: asset room %s, event room %s", assetRoomID, roomID)
	}
	if err := s.appendAssetProcessingEvent(ctx, assetID, event); err != nil {
		return err
	}
	return nil
}

// RecordAssetProcessed builds and publishes a durable processed-video
// manifest for an original video attachment. If the terminal manifest is
// skipped because another terminal/deleted state already won, it makes a
// bounded best-effort attempt to tombstone and storage-clean the unused output.
func (s *AssetModel) RecordAssetProcessed(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant) error {
	return s.RecordAssetProcessedWithHLS(ctx, actorID, roomID, messageEventID, attachmentID, durationMs, width, height, thumbnail, variants, nil)
}

// RecordAssetProcessedWithHLS publishes a terminal video manifest containing
// the compatibility MP4 renditions and, when generated, one HLS generation.
func (s *AssetModel) RecordAssetProcessedWithHLS(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, durationMs int64, width, height int32, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant, hls *corev1.AssetProcessedHLS) error {
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
					Hls:              hls,
				},
			},
		},
	})
	if err := s.publishAssetProcessing(ctx, roomID, event); err != nil {
		if errors.Is(err, ErrAssetLifecycleSkipped) {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), assetCommitCheckTimeout)
			s.cleanupVideoDerivativeOutputs(cleanupCtx, actorID, roomID, attachmentID, thumbnail, variants, hls)
			cleanupCancel()
			return nil
		}
		if errors.Is(err, errAssetEventCommitted) {
			return nil
		}
		committed, confirmErr := s.assetEventCommitted(ctx, attachmentID, event)
		if confirmErr != nil {
			return errors.Join(
				fmt.Errorf("publish asset processing event: %w", err),
				fmt.Errorf("%w: %v", ErrAssetCommitUnknown, confirmErr),
			)
		}
		if committed {
			return nil
		}
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	return nil
}

func (s *AssetModel) assetEventCommitted(ctx context.Context, assetID string, event *corev1.Event) (bool, error) {
	confirmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), assetCommitCheckTimeout)
	defer cancel()
	eventType := events.EventTypeOf(event)
	if eventType == "" {
		return false, fmt.Errorf("resolve asset processing event type")
	}
	published, _, err := s.EventPublisher.SubjectEvents(confirmCtx, events.AssetAggregate(assetID).Subject(eventType))
	if err != nil {
		return false, fmt.Errorf("read durable asset processing events: %w", err)
	}
	for _, candidate := range published {
		if candidate.GetId() == event.GetId() {
			return true, nil
		}
	}
	return false, nil
}

func (s *AssetModel) cleanupVideoDerivativeOutputs(ctx context.Context, actorID string, roomID, originAssetID string, thumbnail *corev1.Attachment, variants []*corev1.VideoVariant, hls *corev1.AssetProcessedHLS) {
	s.cleanupVideoDerivativeOutput(ctx, actorID, roomID, originAssetID, thumbnail)
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		s.cleanupVideoDerivativeOutput(ctx, actorID, roomID, originAssetID, variant.GetAttachment())
	}
	for _, assetID := range hlsDerivativeAssetIDs(hls) {
		declared, ok := s.AssetCreation(assetID)
		if !ok || declared == nil {
			continue
		}
		s.cleanupVideoDerivativeOutput(ctx, actorID, roomID, originAssetID, attachmentFromAsset(declared.GetAsset()))
	}
}

func hlsDerivativeAssetIDs(hls *corev1.AssetProcessedHLS) []string {
	if hls == nil {
		return nil
	}
	var ids []string
	for _, rendition := range hls.GetRenditions() {
		if rendition == nil {
			continue
		}
		for _, segment := range rendition.GetSegments() {
			if segment != nil && segment.GetAssetId() != "" {
				ids = append(ids, segment.GetAssetId())
			}
		}
	}
	return ids
}

func (s *AssetModel) cleanupVideoDerivativeOutput(ctx context.Context, actorID string, fallbackRoomID, originAssetID string, attachment *corev1.Attachment) {
	if attachment == nil || attachment.GetId() == "" {
		return
	}
	assetID := attachment.GetId()
	roomID := fallbackRoomID
	if projectedRoomID, ok := s.AssetRoomID(assetID); ok && projectedRoomID != "" {
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
func (s *AssetModel) DeleteAsset(ctx context.Context, actorID, assetID string) error {
	roomID, ok := s.AssetRoomID(assetID)
	if !ok {
		return fmt.Errorf("asset deletion missing room scope")
	}
	return s.RecordAssetDeleted(ctx, actorID, roomID, assetID)
}

// RecordAssetDeleted appends a durable AssetDeletedEvent in the asset aggregate.
func (s *AssetModel) RecordAssetDeleted(ctx context.Context, actorID string, roomID, assetID string) error {
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

func (s *AssetModel) appendAssetEventEventually(ctx context.Context, assetID string, event *corev1.Event) error {
	if assetID == "" {
		return fmt.Errorf("asset event missing asset id")
	}
	subject := events.AssetAggregate(assetID).SubjectFor(event)
	seq, err := s.EventPublisher.AppendEventually(ctx, subject, event)
	if err != nil {
		return err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := s.waitForAssets(ctx, pos); err != nil {
		return errors.Join(errAssetEventCommitted, err)
	}
	return nil
}

func (s *AssetModel) appendAssetProcessingEvent(ctx context.Context, assetID string, event *corev1.Event) error {
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
			if err := s.waitForAssets(ctx, tail); err != nil {
				return err
			}
		}
		if !s.shouldAppendAssetProcessingEvent(assetID, event) {
			return ErrAssetLifecycleSkipped
		}
		subject := agg.SubjectFor(event)
		seq, err := s.EventPublisher.AppendAtFilter(ctx, subject, event, filter, tail.Seq)
		if err == nil {
			if err := s.waitForAssets(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return errors.Join(errAssetEventCommitted, err)
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
	}
	return fmt.Errorf("append asset processing event after retries: %w", events.ErrConflict)
}

func (s *AssetModel) waitForAssets(ctx context.Context, pos events.StreamPosition) error {
	if s.waitForAssetsOverride != nil {
		return s.waitForAssetsOverride(ctx, pos)
	}
	return waitForPositionAll(ctx, pos, waitForProjection("assets", s.AssetsProjector))
}

func (s *AssetModel) waitForAssetsCurrent(ctx context.Context) error {
	return waitForCurrentAll(ctx, waitForProjection("assets", s.AssetsProjector))
}

func (s *AssetModel) AssetCreation(assetID string) (*corev1.AssetCreatedEvent, bool) {
	return s.Assets.AssetCreation(assetID)
}

func (s *AssetModel) AssetRoomID(assetID string) (string, bool) {
	return s.Assets.AssetRoomID(assetID)
}

func (s *AssetModel) VideoAttachmentManifest(assetID string) (*VideoAttachmentManifest, bool) {
	return s.Assets.VideoAttachmentManifest(assetID)
}

func (s *AssetModel) AssetDeleted(assetID string) bool {
	return s.Assets.AssetDeleted(assetID)
}

func (s *AssetModel) PendingExpiredAssets(now time.Time) []*corev1.AssetCreatedEvent {
	return s.Assets.PendingExpiredAssets(now)
}

func (s *AssetModel) AssetSubtreeIDs(assetID string) []string {
	return s.Assets.AssetSubtreeIDs(assetID)
}

func (s *AssetModel) MessageAssetsByAuthor(userID string) []MessageAssetRef {
	return s.RoomTimeline.MessageAssetsByAuthor(userID)
}

func (s *AssetModel) MessageAssetOwners() []MessageAssetRef {
	return s.RoomTimeline.MessageAssetOwners()
}

func (s *AssetModel) MessageTombstoned(eventID string) bool {
	return s.RoomTimeline.MessageTombstoned(eventID)
}

func (s *AssetModel) shouldAppendAssetProcessingEvent(assetID string, event *corev1.Event) bool {
	if s.AssetDeleted(assetID) {
		return false
	}
	manifest, hasManifest := s.VideoAttachmentManifest(assetID)
	switch event.GetEvent().(type) {
	case *corev1.Event_AssetProcessingStarted:
		return !hasManifest || manifest == nil || (manifest.Succeeded == nil && manifest.Failed == nil)
	case *corev1.Event_AssetProcessingSucceeded, *corev1.Event_AssetProcessingFailed:
		return !hasManifest || manifest == nil || (manifest.Succeeded == nil && manifest.Failed == nil)
	default:
		return true
	}
}

// RecordAssetProcessingFailed builds and publishes a durable failed
// video-processing outcome.
func (s *AssetModel) RecordAssetProcessingFailed(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	err := s.recordAssetProcessingFailed(ctx, actorID, roomID, messageEventID, attachmentID, failureCode)
	if errors.Is(err, ErrAssetLifecycleSkipped) {
		return nil
	}
	return err
}

func (s *AssetModel) recordAssetProcessingFailed(ctx context.Context, actorID string, roomID, messageEventID, attachmentID string, failureCode corev1.AssetProcessingFailureCode) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:        attachmentID,
				MessageEventId: messageEventID,
				FailureCode:    failureCode,
			},
		},
	})
	if err := s.publishAssetProcessing(ctx, roomID, event); err != nil {
		if errors.Is(err, ErrAssetLifecycleSkipped) {
			return ErrAssetLifecycleSkipped
		}
		if errors.Is(err, errAssetEventCommitted) {
			return nil
		}
		committed, confirmErr := s.assetEventCommitted(ctx, attachmentID, event)
		if confirmErr != nil {
			return errors.Join(
				fmt.Errorf("publish asset processing event: %w", err),
				fmt.Errorf("%w: %v", ErrAssetCommitUnknown, confirmErr),
			)
		}
		if committed {
			return nil
		}
		return fmt.Errorf("publish asset processing event: %w", err)
	}
	return nil
}
