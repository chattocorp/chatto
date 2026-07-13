package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/assets"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	assetUploadKeyPrefix             = "asset_upload."
	assetUploadTempObjectPrefix      = "asset-upload."
	defaultAssetUploadSessionTTL     = 15 * time.Minute
	defaultPendingAttachmentAssetTTL = 24 * time.Hour
	defaultAssetUploadChunkSize      = 512 * 1024
	assetUploadCleanupInterval       = 5 * time.Minute
	assetUploadOrphanChunkMaxAge     = defaultAssetUploadSessionTTL + time.Hour
	linkedImageImportKeyPrefix       = "linked_image_import."
	linkedImageImportLockKeyPrefix   = "linked_image_import_lock."
	linkedImageImportLockTTL         = time.Minute
	linkedImageImportPendingTTL      = 2 * time.Minute
	maxPendingLinkedImageImports     = 10
	linkedImageImportStatePending    = "pending"
	linkedImageImportStateCommitted  = "committed"
)

type AssetUploadStatus string

const (
	AssetUploadStatusOpen      AssetUploadStatus = "open"
	AssetUploadStatusCompleted AssetUploadStatus = "completed"
	AssetUploadStatusCancelled AssetUploadStatus = "cancelled"
)

type AssetUploadCreateInput struct {
	ActorID     string
	RoomID      string
	Filename    string
	ContentType string
	Size        int64
	SHA256      string
}

type AssetUploadChunkInput struct {
	ActorID     string
	UploadID    string
	Offset      int64
	Content     []byte
	ChunkSHA256 string
}

type AssetUploadCompleteInput struct {
	ActorID  string
	UploadID string
}

type AssetUploadCancelInput struct {
	ActorID  string
	UploadID string
}

// RemoteAttachmentImportInput describes server-fetched image bytes that should
// enter the same pending room-attachment lifecycle as a completed upload.
type RemoteAttachmentImportInput struct {
	ActorID     string
	RoomID      string
	SourceURL   string
	Filename    string
	ContentType string
	Content     []byte
	Reservation *RemoteAttachmentImportReservation
}

type RemoteAttachmentImportReservation struct {
	ActorID  string
	RoomID   string
	Key      string
	Revision uint64
}

type linkedImageImportRecord struct {
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
	Attachment []byte    `json:"attachment,omitempty"`
}

// BeginRemoteAttachmentImport reserves capacity before the network fetch, or
// returns the existing staged attachment for an idempotent repeated request.
func (m *AssetUploadModel) BeginRemoteAttachmentImport(ctx context.Context, actorID, roomID, sourceURL string) (*corev1.Attachment, *RemoteAttachmentImportReservation, error) {
	if actorID == "" || roomID == "" || sourceURL == "" {
		return nil, nil, nil
	}
	if err := m.authorizeUpload(ctx, actorID, roomID); err != nil {
		return nil, nil, err
	}
	lockRevision, err := m.acquireLinkedImageImportLock(ctx, actorID)
	if err != nil {
		return nil, nil, err
	}
	defer m.releaseLinkedImageImportLock(actorID, lockRevision)
	importKey := m.linkedImageImportKey(actorID, roomID, sourceURL)
	active, existing, err := m.activeLinkedImageImports(ctx, actorID, importKey)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		return existing, nil, nil
	}
	if active >= maxPendingLinkedImageImports {
		return nil, nil, fmt.Errorf("linked image pending import limit of %d reached: %w", maxPendingLinkedImageImports, ErrLimitExceeded)
	}
	record, err := json.Marshal(linkedImageImportRecord{State: linkedImageImportStatePending, CreatedAt: time.Now()})
	if err != nil {
		return nil, nil, fmt.Errorf("encode linked image import reservation: %w", err)
	}
	revision, err := m.core.storage.runtimeStateKV.Create(ctx, importKey, record, jetstream.KeyTTL(linkedImageImportPendingTTL))
	if err != nil {
		return nil, nil, fmt.Errorf("reserve linked image import: %w", err)
	}
	return nil, &RemoteAttachmentImportReservation{ActorID: actorID, RoomID: roomID, Key: importKey, Revision: revision}, nil
}

type AssetUploadSession struct {
	UploadID        string            `json:"upload_id"`
	ActorID         string            `json:"actor_id"`
	RoomID          string            `json:"room_id"`
	Filename        string            `json:"filename"`
	ContentType     string            `json:"content_type"`
	Size            int64             `json:"size"`
	SHA256          string            `json:"sha256"`
	Status          AssetUploadStatus `json:"status"`
	CommittedOffset int64             `json:"committed_offset"`
	MaxChunkSize    int32             `json:"max_chunk_size"`
	ExpiresAt       time.Time         `json:"expires_at"`
	AssetID         string            `json:"asset_id,omitempty"`
	ChunkKeys       []string          `json:"chunk_keys,omitempty"`
}

type AssetUploadModel struct {
	core *ChattoCore
}

func (c *ChattoCore) AssetUploads() *AssetUploadModel {
	return &AssetUploadModel{core: c}
}

func (m *AssetUploadModel) CreateUpload(ctx context.Context, input AssetUploadCreateInput) (*AssetUploadSession, error) {
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		return nil, invalidArgument("filename is required")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if input.Size < 0 {
		return nil, invalidArgument("size must be non-negative")
	}
	if !validSHA256Hex(input.SHA256) {
		return nil, invalidArgument("sha256 must be lowercase hexadecimal SHA-256")
	}
	if err := m.checkUploadSize(contentType, input.Size); err != nil {
		return nil, err
	}
	if err := m.authorizeUpload(ctx, input.ActorID, input.RoomID); err != nil {
		return nil, err
	}

	now := time.Now()
	session := &AssetUploadSession{
		UploadID:     NewAssetID(),
		ActorID:      input.ActorID,
		RoomID:       input.RoomID,
		Filename:     filename,
		ContentType:  contentType,
		Size:         input.Size,
		SHA256:       strings.ToLower(input.SHA256),
		Status:       AssetUploadStatusOpen,
		MaxChunkSize: defaultAssetUploadChunkSize,
		ExpiresAt:    now.Add(defaultAssetUploadSessionTTL),
	}
	value, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	if _, err := m.core.storage.runtimeStateKV.Create(ctx, assetUploadKey(session.UploadID), value, jetstream.KeyTTL(time.Until(session.ExpiresAt))); err != nil {
		return nil, fmt.Errorf("create upload session: %w", err)
	}
	return session, nil
}

func (m *AssetUploadModel) GetUpload(ctx context.Context, actorID, uploadID string) (*AssetUploadSession, error) {
	session, _, err := m.loadUpload(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session.ActorID != actorID {
		return nil, ErrPermissionDenied
	}
	return session, nil
}

func (m *AssetUploadModel) UploadChunk(ctx context.Context, input AssetUploadChunkInput) (*AssetUploadSession, error) {
	if len(input.Content) == 0 {
		return nil, invalidArgument("chunk content is required")
	}
	if !validSHA256Hex(input.ChunkSHA256) {
		return nil, invalidArgument("chunk_sha256 must be lowercase hexadecimal SHA-256")
	}
	sum := sha256.Sum256(input.Content)
	if hex.EncodeToString(sum[:]) != input.ChunkSHA256 {
		return nil, invalidArgument("chunk_sha256 does not match content")
	}
	session, revision, err := m.loadUpload(ctx, input.UploadID)
	if err != nil {
		return nil, err
	}
	if session.ActorID != input.ActorID {
		return nil, ErrPermissionDenied
	}
	if session.Status != AssetUploadStatusOpen {
		return nil, invalidArgument("upload is not open")
	}
	if input.Offset != session.CommittedOffset {
		return nil, invalidArgument("chunk offset does not match committed offset")
	}
	if int32(len(input.Content)) > session.MaxChunkSize {
		return nil, invalidArgument("chunk exceeds maximum chunk size")
	}
	if input.Offset+int64(len(input.Content)) > session.Size {
		return nil, invalidArgument("chunk exceeds declared upload size")
	}
	chunkKey := assetUploadTempObjectKey(session.UploadID, input.Offset)
	if _, err := m.core.storage.serverAssets.Put(ctx, jetstream.ObjectMeta{
		Name: chunkKey,
		Headers: map[string][]string{
			"Upload-Id": {session.UploadID},
		},
	}, bytes.NewReader(input.Content)); err != nil {
		return nil, fmt.Errorf("store upload chunk: %w", err)
	}
	session.ChunkKeys = append(session.ChunkKeys, chunkKey)
	session.CommittedOffset += int64(len(input.Content))
	if err := m.updateUpload(ctx, session, revision); err != nil {
		_ = m.core.storage.serverAssets.Delete(ctx, chunkKey)
		return nil, err
	}
	return session, nil
}

func (m *AssetUploadModel) CompleteUpload(ctx context.Context, input AssetUploadCompleteInput) (*AssetUploadSession, *corev1.Attachment, error) {
	session, revision, err := m.loadUpload(ctx, input.UploadID)
	if err != nil {
		return nil, nil, err
	}
	if session.ActorID != input.ActorID {
		return nil, nil, ErrPermissionDenied
	}
	if session.Status == AssetUploadStatusCompleted {
		declared, ok := m.core.assetLifecycle().AssetCreation(session.AssetID)
		if !ok {
			return nil, nil, ErrNotFound
		}
		attachment := attachmentFromAsset(declared.GetAsset())
		if attachment != nil {
			attachment.RoomId = session.RoomID
		}
		return session, attachment, nil
	}
	if session.Status != AssetUploadStatusOpen {
		return nil, nil, invalidArgument("upload is not open")
	}
	if session.CommittedOffset != session.Size {
		return nil, nil, invalidArgument("upload is incomplete")
	}
	if err := m.authorizeUpload(ctx, input.ActorID, session.RoomID); err != nil {
		return nil, nil, err
	}
	tmp, err := m.materializeUpload(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	attachment, animatedGIF, err := m.storeCompletedUpload(ctx, session, tmp)
	if err != nil {
		return nil, nil, err
	}
	pendingExpiresAt := time.Now().Add(defaultPendingAttachmentAssetTTL)
	needsVideoProcessing := m.core.OnVideoProcessingRequested != nil && AttachmentNeedsVideoProcessing(attachment, animatedGIF)
	if err := m.core.assetLifecycle().RecordUploadedPendingAttachmentAsset(ctx, input.ActorID, session.RoomID, attachment, session.SHA256, pendingExpiresAt, needsVideoProcessing); err != nil {
		m.core.media().DeleteAttachmentFromStorage(ctx, attachment)
		return nil, nil, err
	}
	session.Status = AssetUploadStatusCompleted
	session.AssetID = attachment.GetId()
	session.ExpiresAt = pendingExpiresAt
	if err := m.updateUpload(ctx, session, revision); err != nil {
		return nil, nil, err
	}
	m.deleteUploadChunks(ctx, session)
	return session, attachment, nil
}

// ImportRemoteAttachment validates and stores server-fetched image bytes as a
// pending room attachment. The resulting asset is claimed by a later message
// post or removed by the existing pending-asset cleanup after its TTL.
func (m *AssetUploadModel) ImportRemoteAttachment(ctx context.Context, input RemoteAttachmentImportInput) (*corev1.Attachment, error) {
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		return nil, invalidArgument("filename is required")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, invalidArgument("remote attachment must be an image")
	}
	if len(input.Content) == 0 {
		return nil, invalidArgument("remote attachment content is required")
	}
	if strings.TrimSpace(input.SourceURL) == "" {
		return nil, invalidArgument("remote attachment source URL is required")
	}
	if err := m.checkUploadSize(contentType, int64(len(input.Content))); err != nil {
		return nil, err
	}
	if err := m.authorizeUpload(ctx, input.ActorID, input.RoomID); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(input.Content)
	contentSHA256 := hex.EncodeToString(sum[:])
	reservation := input.Reservation
	if reservation == nil {
		existing, created, err := m.BeginRemoteAttachmentImport(ctx, input.ActorID, input.RoomID, input.SourceURL)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
		reservation = created
	}
	if reservation == nil || reservation.ActorID != input.ActorID || reservation.RoomID != input.RoomID || reservation.Key != m.linkedImageImportKey(input.ActorID, input.RoomID, input.SourceURL) {
		return nil, invalidArgument("remote attachment import reservation is invalid")
	}
	reservationActive := true
	defer func() {
		if reservationActive {
			m.CancelRemoteAttachmentImport(context.Background(), reservation)
		}
	}()

	session := &AssetUploadSession{
		ActorID:     input.ActorID,
		RoomID:      input.RoomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(input.Content)),
		SHA256:      contentSHA256,
	}
	attachment, animatedGIF, err := m.storeCompletedUpload(ctx, session, bytes.NewReader(input.Content))
	if err != nil {
		return nil, err
	}
	pendingExpiresAt := time.Now().Add(defaultPendingAttachmentAssetTTL)
	needsVideoProcessing := m.core.OnVideoProcessingRequested != nil && AttachmentNeedsVideoProcessing(attachment, animatedGIF)
	if err := m.core.assetLifecycle().RecordUploadedPendingAttachmentAsset(ctx, input.ActorID, input.RoomID, attachment, session.SHA256, pendingExpiresAt, needsVideoProcessing); err != nil {
		m.core.media().DeleteAttachmentFromStorage(ctx, attachment)
		return nil, err
	}
	attachmentData, err := proto.Marshal(attachment)
	if err != nil {
		_ = m.core.assetLifecycle().RecordAssetDeleted(context.Background(), SystemActorID, input.RoomID, attachment.GetId())
		_ = m.core.media().DeleteAttachmentFromStorage(context.Background(), attachment)
		return nil, fmt.Errorf("encode linked image import attachment: %w", err)
	}
	committedRecord, err := json.Marshal(linkedImageImportRecord{State: linkedImageImportStateCommitted, CreatedAt: time.Now(), Attachment: attachmentData})
	if err != nil {
		_ = m.core.assetLifecycle().RecordAssetDeleted(context.Background(), SystemActorID, input.RoomID, attachment.GetId())
		_ = m.core.media().DeleteAttachmentFromStorage(context.Background(), attachment)
		return nil, fmt.Errorf("encode committed linked image import: %w", err)
	}
	lockRevision, err := m.acquireLinkedImageImportLock(ctx, input.ActorID)
	if err != nil {
		_ = m.core.assetLifecycle().RecordAssetDeleted(context.Background(), SystemActorID, input.RoomID, attachment.GetId())
		_ = m.core.media().DeleteAttachmentFromStorage(context.Background(), attachment)
		return nil, err
	}
	defer m.releaseLinkedImageImportLock(input.ActorID, lockRevision)
	if err := m.core.storage.runtimeStateKV.Delete(ctx, reservation.Key, jetstream.LastRevision(reservation.Revision)); err != nil {
		_ = m.core.assetLifecycle().RecordAssetDeleted(context.Background(), SystemActorID, input.RoomID, attachment.GetId())
		_ = m.core.media().DeleteAttachmentFromStorage(context.Background(), attachment)
		return nil, fmt.Errorf("replace linked image import reservation: %w", err)
	}
	reservationActive = false
	if _, err := m.core.storage.runtimeStateKV.Create(ctx, reservation.Key, committedRecord, jetstream.KeyTTL(defaultPendingAttachmentAssetTTL)); err != nil {
		_ = m.core.assetLifecycle().RecordAssetDeleted(context.Background(), SystemActorID, input.RoomID, attachment.GetId())
		_ = m.core.media().DeleteAttachmentFromStorage(context.Background(), attachment)
		return nil, fmt.Errorf("commit linked image import reservation: %w", err)
	}
	return attachment, nil
}

func (m *AssetUploadModel) CancelRemoteAttachmentImport(ctx context.Context, reservation *RemoteAttachmentImportReservation) {
	if reservation == nil || reservation.Key == "" || reservation.Revision == 0 {
		return
	}
	if err := m.core.storage.runtimeStateKV.Delete(ctx, reservation.Key, jetstream.LastRevision(reservation.Revision)); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
		m.core.logger.Warn("Failed to cancel linked image import reservation", "error", err)
	}
}

func (m *AssetUploadModel) acquireLinkedImageImportLock(ctx context.Context, actorID string) (uint64, error) {
	revision, err := m.core.storage.runtimeStateKV.Create(ctx, m.linkedImageImportLockKey(actorID), []byte{1}, jetstream.KeyTTL(linkedImageImportLockTTL))
	if errors.Is(err, jetstream.ErrKeyExists) {
		return 0, fmt.Errorf("linked image import already in progress: %w", ErrLimitExceeded)
	}
	if err != nil {
		return 0, fmt.Errorf("acquire linked image import lock: %w", err)
	}
	return revision, nil
}

func (m *AssetUploadModel) releaseLinkedImageImportLock(actorID string, revision uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.core.storage.runtimeStateKV.Delete(ctx, m.linkedImageImportLockKey(actorID), jetstream.LastRevision(revision)); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
		m.core.logger.Warn("Failed to release linked image import lock", "error", err)
	}
}

func (m *AssetUploadModel) activeLinkedImageImports(ctx context.Context, actorID, requestedKey string) (int, *corev1.Attachment, error) {
	prefix := m.linkedImageImportActorPrefix(actorID)
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("list linked image imports: %w", err)
	}

	active := 0
	for key := range lister.Keys() {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return 0, nil, fmt.Errorf("read linked image import: %w", err)
		}
		var record linkedImageImportRecord
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
				return 0, nil, fmt.Errorf("delete invalid linked image import: %w", err)
			}
			continue
		}
		switch record.State {
		case linkedImageImportStatePending:
			active++
			if key == requestedKey {
				return 0, nil, fmt.Errorf("linked image import already in progress: %w", ErrLimitExceeded)
			}
			continue
		case linkedImageImportStateCommitted:
			attachment := &corev1.Attachment{}
			if len(record.Attachment) > 0 && proto.Unmarshal(record.Attachment, attachment) == nil && attachment.GetId() != "" {
				active++
				if key == requestedKey {
					return active, attachment, nil
				}
				continue
			}
		}
		if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
			return 0, nil, fmt.Errorf("delete stale linked image import: %w", err)
		}
	}
	return active, nil, nil
}

// ReleaseLinkedImageImports removes quota/idempotency entries after a message
// successfully claims their assets. The durable attachment lifecycle remains
// authoritative; this runtime index is only staging coordination.
func (m *AssetUploadModel) ReleaseLinkedImageImports(ctx context.Context, actorID string, attachments []*corev1.Attachment) {
	assetIDs := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		if attachment != nil && attachment.GetId() != "" {
			assetIDs[attachment.GetId()] = struct{}{}
		}
	}
	if actorID == "" || len(assetIDs) == 0 {
		return
	}
	lockRevision, err := m.acquireLinkedImageImportLock(ctx, actorID)
	if err != nil {
		m.core.logger.Warn("Failed to acquire linked image import lock for claim cleanup", "error", err)
		return
	}
	defer m.releaseLinkedImageImportLock(actorID, lockRevision)

	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, m.linkedImageImportActorPrefix(actorID)+"*")
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return
	}
	if err != nil {
		m.core.logger.Warn("Failed to list linked image imports for claim cleanup", "error", err)
		return
	}
	for key := range lister.Keys() {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			continue
		}
		var record linkedImageImportRecord
		attachment := &corev1.Attachment{}
		if json.Unmarshal(entry.Value(), &record) != nil || record.State != linkedImageImportStateCommitted || proto.Unmarshal(record.Attachment, attachment) != nil {
			continue
		}
		if _, claimed := assetIDs[attachment.GetId()]; !claimed {
			continue
		}
		if err := m.core.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
			m.core.logger.Warn("Failed to release claimed linked image import", "asset_id", attachment.GetId(), "error", err)
		}
	}
}

func (m *AssetUploadModel) linkedImageImportActorPrefix(actorID string) string {
	digest := sha256.Sum256([]byte(actorID))
	return linkedImageImportKeyPrefix + hex.EncodeToString(digest[:]) + "."
}

func (m *AssetUploadModel) linkedImageImportKey(actorID, roomID, sourceURL string) string {
	digest := sha256.Sum256([]byte(roomID + "\x00" + sourceURL))
	return m.linkedImageImportActorPrefix(actorID) + hex.EncodeToString(digest[:])
}

func (m *AssetUploadModel) linkedImageImportLockKey(actorID string) string {
	digest := sha256.Sum256([]byte(actorID))
	return linkedImageImportLockKeyPrefix + hex.EncodeToString(digest[:])
}

func (m *AssetUploadModel) CancelUpload(ctx context.Context, input AssetUploadCancelInput) (*AssetUploadSession, error) {
	session, revision, err := m.loadUpload(ctx, input.UploadID)
	if err != nil {
		return nil, err
	}
	if session.ActorID != input.ActorID {
		return nil, ErrPermissionDenied
	}
	if session.Status == AssetUploadStatusCompleted {
		return nil, invalidArgument("completed uploads cannot be cancelled")
	}
	session.Status = AssetUploadStatusCancelled
	if err := m.updateUpload(ctx, session, revision); err != nil {
		return nil, err
	}
	m.deleteUploadChunks(ctx, session)
	_ = m.core.storage.runtimeStateKV.Delete(ctx, assetUploadKey(session.UploadID))
	return session, nil
}

func (m *AssetUploadModel) RunCleanup(ctx context.Context) error {
	ticker := time.NewTicker(assetUploadCleanupInterval)
	defer ticker.Stop()
	for {
		if err := m.CleanupExpired(ctx); err != nil {
			m.core.logger.Warn("Asset upload cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *AssetUploadModel) CleanupExpired(ctx context.Context) error {
	now := time.Now()
	if err := m.cleanupExpiredUploadSessions(ctx, now); err != nil {
		return err
	}
	if err := m.cleanupOrphanUploadChunks(ctx, now); err != nil {
		return err
	}
	if err := m.cleanupExpiredPendingAssets(ctx, now); err != nil {
		return err
	}
	return nil
}

func (m *AssetUploadModel) cleanupExpiredUploadSessions(ctx context.Context, now time.Time) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, assetUploadKeyPrefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list asset upload sessions: %w", err)
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	for _, key := range keys {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				continue
			}
			return fmt.Errorf("load asset upload session for cleanup: %w", err)
		}
		var session AssetUploadSession
		if err := json.Unmarshal(entry.Value(), &session); err != nil {
			m.core.logger.Warn("Deleting malformed asset upload session", "upload_key", key, "error", err)
			_ = m.core.storage.runtimeStateKV.Delete(ctx, key)
			continue
		}
		expired := !session.ExpiresAt.After(now)
		if session.Status == AssetUploadStatusOpen && !expired {
			continue
		}
		if session.Status == AssetUploadStatusCompleted && !expired {
			continue
		}
		m.deleteUploadChunks(ctx, &session)
		_ = m.core.storage.runtimeStateKV.Delete(ctx, key)
	}
	return nil
}

func (m *AssetUploadModel) cleanupOrphanUploadChunks(ctx context.Context, now time.Time) error {
	objects, err := m.core.storage.serverAssets.List(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return nil
		}
		return fmt.Errorf("list asset upload chunks: %w", err)
	}
	cutoff := now.Add(-assetUploadOrphanChunkMaxAge)
	for _, info := range objects {
		if info == nil || !strings.HasPrefix(info.Name, assetUploadTempObjectPrefix) || info.ModTime.After(cutoff) {
			continue
		}
		if err := m.core.storage.serverAssets.Delete(ctx, info.Name); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
			m.core.logger.Warn("Failed to delete orphan asset upload chunk", "chunk_key", info.Name, "error", err)
		}
	}
	return nil
}

func (m *AssetUploadModel) cleanupExpiredPendingAssets(ctx context.Context, now time.Time) error {
	claimed := make(map[string]struct{})
	for _, owner := range m.core.assetLifecycle().MessageAssetOwners() {
		if owner.AssetID != "" && !m.core.assetLifecycle().MessageTombstoned(owner.MessageEventID) {
			claimed[owner.AssetID] = struct{}{}
		}
	}
	for _, declared := range m.core.assetLifecycle().PendingExpiredAssets(now) {
		asset := declared.GetAsset()
		if asset == nil || asset.GetId() == "" {
			continue
		}
		if _, ok := claimed[asset.GetId()]; ok {
			continue
		}
		roomID := declared.GetRoomId()
		if roomID == "" {
			if projectedRoomID, ok := m.core.assetLifecycle().AssetRoomID(asset.GetId()); ok {
				roomID = projectedRoomID
			}
		}
		if roomID == "" {
			continue
		}
		attachment := attachmentFromAsset(asset)
		if attachment == nil {
			continue
		}
		attachment.RoomId = roomID
		if err := m.core.assetLifecycle().RecordAssetDeleted(ctx, SystemActorID, roomID, asset.GetId()); err != nil {
			return fmt.Errorf("record expired pending asset deletion: %w", err)
		}
		if err := m.core.media().DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			m.core.logger.Warn("Failed to delete expired pending attachment binary", "attachment_id", asset.GetId(), "error", err)
		}
	}
	return nil
}

func (m *AssetUploadModel) checkUploadSize(contentType string, size int64) error {
	maxSize := m.core.AssetsConfig().MaxUploadSize
	if strings.HasPrefix(contentType, "video/") && m.core.VideoMaxUploadSize > 0 {
		maxSize = m.core.VideoMaxUploadSize
	}
	if size > maxSize {
		return fmt.Errorf("attachment exceeds maximum size of %d bytes: %w", maxSize, ErrInvalidArgument)
	}
	return nil
}

func (m *AssetUploadModel) authorizeUpload(ctx context.Context, actorID, roomID string) error {
	room, kind, err := m.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return err
	}
	if room.Archived {
		return ErrRoomArchived
	}
	canAttach, err := m.core.CanAttachFiles(ctx, actorID, kind, room.Id)
	if err != nil {
		return err
	}
	if !canAttach {
		return ErrPermissionDenied
	}
	return nil
}

func (m *AssetUploadModel) loadUpload(ctx context.Context, uploadID string) (*AssetUploadSession, uint64, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return nil, 0, invalidArgument("upload_id is required")
	}
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, assetUploadKey(uploadID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("load upload session: %w", err)
	}
	var session AssetUploadSession
	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return nil, 0, fmt.Errorf("decode upload session: %w", err)
	}
	if session.ExpiresAt.Before(time.Now()) && session.Status == AssetUploadStatusOpen {
		session.Status = AssetUploadStatusCancelled
		return &session, entry.Revision(), ErrNotFound
	}
	return &session, entry.Revision(), nil
}

func (m *AssetUploadModel) updateUpload(ctx context.Context, session *AssetUploadSession, revision uint64) error {
	value, err := json.Marshal(session)
	if err != nil {
		return err
	}
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = time.Second
	}
	if _, err := m.core.updateRuntimeStateTokenTTL(ctx, assetUploadKey(session.UploadID), value, revision, ttl); err != nil {
		return fmt.Errorf("update upload session: %w", err)
	}
	return nil
}

func (m *AssetUploadModel) materializeUpload(ctx context.Context, session *AssetUploadSession) (*os.File, error) {
	tmp, err := os.CreateTemp("", "chatto-asset-upload-*")
	if err != nil {
		return nil, fmt.Errorf("create upload temp file: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	chunkKeys := append([]string(nil), session.ChunkKeys...)
	sort.Slice(chunkKeys, func(i, j int) bool {
		return chunkOffset(chunkKeys[i]) < chunkOffset(chunkKeys[j])
	})
	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)
	for _, key := range chunkKeys {
		obj, err := m.core.storage.serverAssets.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("read upload chunk: %w", err)
		}
		if _, err := io.Copy(w, obj); err != nil {
			obj.Close()
			return nil, fmt.Errorf("copy upload chunk: %w", err)
		}
		if err := obj.Close(); err != nil {
			return nil, fmt.Errorf("close upload chunk: %w", err)
		}
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != session.SHA256 {
		return nil, invalidArgument("sha256 does not match uploaded content")
	}
	if pos, err := tmp.Seek(0, io.SeekStart); err != nil || pos != 0 {
		return nil, fmt.Errorf("rewind upload temp file: %w", err)
	}
	cleanup = false
	return tmp, nil
}

func (m *AssetUploadModel) storeCompletedUpload(ctx context.Context, session *AssetUploadSession, reader io.ReadSeeker) (*corev1.Attachment, bool, error) {
	attachmentID := NewAssetID()
	contentType := session.ContentType
	isImage := strings.HasPrefix(contentType, "image/")
	var content []byte
	var size int64
	var width, height int32
	var animatedGIF bool

	if isImage {
		result, err := assets.ProcessAttachmentImageWithConfig(reader, m.core.AssetsConfig())
		if err != nil {
			return nil, false, fmt.Errorf("failed to process image: %w", err)
		}
		content = result.Original
		size = int64(len(content))
		width = int32(result.Width)
		height = int32(result.Height)
		animatedGIF = contentType == "image/gif" && assets.IsAnimatedGIF(content)
		reader = bytes.NewReader(content)
	} else {
		size = session.Size
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return nil, false, fmt.Errorf("rewind upload temp file: %w", err)
		}
	}

	var storage *corev1.DeprecatedAsset
	if m.core.ShouldUseS3() {
		s3Key := S3KeyAttachment(attachmentID)
		if _, err := m.core.s3Client.PutObject(ctx, s3Key, reader, size, contentType); err != nil {
			return nil, false, fmt.Errorf("failed to upload attachment to S3: %w", err)
		}
		storage = &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_S3{
				S3: &corev1.S3Asset{Key: s3Key, Bucket: proto.String(m.core.s3Client.Bucket())},
			},
		}
	} else {
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return nil, false, fmt.Errorf("rewind upload temp file: %w", err)
		}
		if _, err := m.core.storage.serverAssets.Put(ctx, jetstream.ObjectMeta{
			Name: attachmentID,
			Headers: map[string][]string{
				"Content-Type": {contentType},
				"Filename":     {session.Filename},
				"Room-Id":      {session.RoomID},
			},
		}, reader); err != nil {
			return nil, false, fmt.Errorf("failed to store attachment: %w", err)
		}
		storage = &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_Nats{
				Nats: &corev1.NATSAsset{Key: attachmentID},
			},
		}
	}

	return &corev1.Attachment{
		Id:          attachmentID,
		RoomId:      session.RoomID,
		Filename:    session.Filename,
		ContentType: contentType,
		Size:        size,
		Width:       width,
		Height:      height,
		Storage:     storage,
	}, animatedGIF, nil
}

func (m *AssetUploadModel) deleteUploadChunks(ctx context.Context, session *AssetUploadSession) {
	for _, key := range session.ChunkKeys {
		if err := m.core.storage.serverAssets.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
			m.core.logger.Warn("Failed to delete asset upload chunk", "upload_id", session.UploadID, "error", err)
		}
	}
}

func assetUploadKey(uploadID string) string {
	return assetUploadKeyPrefix + uploadID
}

func assetUploadTempObjectKey(uploadID string, offset int64) string {
	return fmt.Sprintf("%s%s.%020d.%s", assetUploadTempObjectPrefix, uploadID, offset, NewAssetID())
}

func chunkOffset(key string) int64 {
	parts := strings.Split(strings.TrimPrefix(key, assetUploadTempObjectPrefix), ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if len(parts[i]) != 20 {
			continue
		}
		offset, err := strconv.ParseInt(parts[i], 10, 64)
		if err == nil {
			return offset
		}
	}
	return 0
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
