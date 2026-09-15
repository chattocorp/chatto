package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/encryption"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// DecryptedMessageBody is the public view of a message body with
// plaintext content. The body's encryption envelope is unwrapped by
// the resolver layer's decryptMessageBody helper.
type DecryptedMessageBody struct {
	AuthorId               string
	Body                   string
	Attachments            []*evtv1.Attachment
	AttachmentDescriptions map[string]string
	LinkPreview            *evtv1.LinkPreview
	CreatedAt              time.Time
	UpdatedAt              *time.Time
}

// GetFullMessageBody returns the decrypted message body for a message event,
// folding any subsequent edit or retract events to produce the current body.
// Returns nil if the message has been retracted or doesn't exist, or
// if the author's encryption key has been crypto-shredded.
func (c *ChattoCore) GetFullMessageBody(ctx context.Context, eventID string) (*DecryptedMessageBody, error) {
	if eventID == "" {
		return nil, nil
	}

	entry, ok := c.roomModel.timelineEntry(eventID)
	if !ok || !entry.IsMessagePost() {
		return nil, nil
	}
	body, err := c.currentMessageBody(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if body == nil {
		// Retracted message: same shape as a legacy GDPR delete —
		// resolver renders "[Message unavailable]".
		return nil, nil
	}

	plaintext, err := c.decryptMessageBody(ctx, eventID, entry.RoomID, body)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil // crypto-shredded
		}
		return nil, fmt.Errorf("failed to decrypt message body: %w", err)
	}
	descriptions, err := c.decryptAttachmentDescriptions(ctx, eventID, entry.RoomID, body)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decrypt attachment descriptions: %w", err)
	}

	result := &DecryptedMessageBody{
		AuthorId:               body.GetAuthorId(),
		Body:                   string(plaintext),
		Attachments:            c.mediaModel.MessageBodyAttachments(body),
		AttachmentDescriptions: descriptions,
		LinkPreview:            body.GetLinkPreview(),
		CreatedAt:              entry.CreatedAt,
	}
	// UpdatedAt: if EVT hydration returned a body different from the
	// original post's body, the message has been edited. The body
	// proto carries its own UpdatedAt; surface that if set, otherwise
	// derive from the most recent edit's envelope time.
	if upd := body.GetUpdatedAt(); upd != nil {
		t := upd.AsTime()
		result.UpdatedAt = &t
	}
	return result, nil
}

// decryptAttachmentDescriptions decrypts attachment-description envelopes
// without retaining plaintext in a projection. Duplicate, unsupported, or
// detached entries make the message body corrupt.
func (c *ChattoCore) decryptAttachmentDescriptions(ctx context.Context, eventID, roomID string, msg *evtv1.MessageBody) (map[string]string, error) {
	if msg == nil || len(msg.GetAttachmentDescriptions()) == 0 {
		return map[string]string{}, nil
	}
	attachmentIDs := messageBodyAttachmentIDs(msg)
	currentAssets := make(map[string]struct{}, len(attachmentIDs))
	for _, assetID := range attachmentIDs {
		currentAssets[assetID] = struct{}{}
	}
	result := make(map[string]string, len(msg.GetAttachmentDescriptions()))
	canonicalMessageEventID := c.attachmentDescriptionCanonicalEventID(eventID)
	keys := make(map[int32]*messageContentKey)
	for _, encryptedDescription := range msg.GetAttachmentDescriptions() {
		if encryptedDescription == nil {
			return nil, fmt.Errorf("%w: nil attachment description", ErrMessageBodyCorrupt)
		}
		assetID := encryptedDescription.GetAssetId()
		if _, ok := currentAssets[assetID]; !ok {
			return nil, fmt.Errorf("%w: attachment description references an absent asset", ErrMessageBodyCorrupt)
		}
		if _, duplicate := result[assetID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate attachment description", ErrMessageBodyCorrupt)
		}
		if encryptedDescription.GetEncryptionVersion() != encryption.EnvelopeVersionV2 {
			return nil, fmt.Errorf("%w: unsupported attachment description encryption version %d", ErrMessageBodyCorrupt, encryptedDescription.GetEncryptionVersion())
		}
		epoch := encryptedDescription.GetContentKeyEpoch()
		if epoch <= 0 {
			return nil, fmt.Errorf("%w: missing attachment description content key epoch", ErrMessageBodyCorrupt)
		}
		contentKey := keys[epoch]
		if contentKey == nil {
			contentKeyEvent, ok, err := c.userModel.contentKeyAtEpoch(msg.GetAuthorId(), evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, epoch)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, encryption.ErrKeyNotFound
			}
			contentKey, err = c.unwrapMessageContentKey(ctx, contentKeyEvent)
			if err != nil {
				return nil, err
			}
			keys[epoch] = contentKey
		}
		plaintext, err := encryption.DecryptWithContentKey(
			contentKey.key,
			encryptedDescription.GetEncryptedDescription(),
			encryptedDescription.GetEncryptionNonce(),
			attachmentDescriptionAAD(canonicalMessageEventID, msg.GetBodyEventId(), roomID, msg.GetAuthorId(), assetID, epoch),
		)
		if err != nil {
			return nil, messageBodyEnvelopeError(err)
		}
		result[assetID] = string(plaintext)
	}
	return result, nil
}

func (c *ChattoCore) attachmentDescriptionCanonicalEventID(eventID string) string {
	entry, ok := c.roomModel.timelineEntry(eventID)
	if ok && entry.EchoOfEventID != "" {
		return entry.EchoOfEventID
	}
	return eventID
}

func (c *ChattoCore) currentMessageBody(ctx context.Context, eventID string) (*evtv1.MessageBody, error) {
	for attempt := 0; attempt < maxTimelineHydrationAttempts; attempt++ {
		reference, retracted, known := c.roomModel.latestBodyReference(eventID)
		if !known || retracted || reference.StreamSeq == 0 {
			return nil, nil
		}
		body, err := c.timelineHydrator.body(ctx, reference)
		if err != nil {
			if !c.roomModel.timeline.Projection().BodyReferenceCurrent(reference) {
				continue
			}
			return nil, err
		}
		if c.roomModel.timeline.Projection().BodyReferenceCurrent(reference) {
			return body, nil
		}
	}
	return nil, errTimelineReadPlanStale
}

func (c *ChattoCore) hydrateCurrentMessageBodies(ctx context.Context, references []TimelineBodyReference) ([]*evtv1.MessageBody, error) {
	if len(references) == 0 {
		return nil, nil
	}
	bodies, err := c.timelineHydrator.bodies(ctx, references)
	if err != nil {
		for _, reference := range references {
			if !c.roomModel.timeline.Projection().BodyReferenceCurrent(reference) {
				return nil, errTimelineReadPlanStale
			}
		}
		return nil, err
	}
	for _, reference := range references {
		if !c.roomModel.timeline.Projection().BodyReferenceCurrent(reference) {
			// Callers select a fresh reference set because edits can also change
			// attachment pagination. Returning a stale-plan marker prevents the
			// old selection from being reused.
			return nil, errTimelineReadPlanStale
		}
	}
	return bodies, nil
}

// GetMessageBody is a thin wrapper returning just the plaintext body
// text. Same semantics as GetFullMessageBody: retracted or crypto-
// shredded messages return empty string.
func (c *ChattoCore) GetMessageBody(ctx context.Context, eventID string) (string, error) {
	body, err := c.GetFullMessageBody(ctx, eventID)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	return body.Body, nil
}

// decryptMessageBody decrypts an encrypted message body. Legacy bodies are
// decrypted directly with the author's per-user key. V2 bodies resolve the
// author's message-body DEK epoch and authenticate the event context as AAD.
// Bodies carried by MessageBodyEvent additionally bind the body event envelope
// ID into AAD so payloads cannot be replayed under a different body event.
func (c *ChattoCore) decryptMessageBody(ctx context.Context, eventID, roomID string, msg *evtv1.MessageBody) ([]byte, error) {
	if msg.GetEncryptionVersion() >= encryption.EnvelopeVersionV2 || msg.GetContentKeyEpoch() > 0 {
		version := msg.GetEncryptionVersion()
		if version != encryption.EnvelopeVersionV2 {
			return nil, fmt.Errorf("%w: unsupported message body encryption version %d", ErrMessageBodyCorrupt, version)
		}
		epoch := msg.GetContentKeyEpoch()
		if epoch <= 0 {
			return nil, fmt.Errorf("%w: missing content key epoch for v%d message body", ErrMessageBodyCorrupt, version)
		}
		contentKeyEvent, ok, err := c.userModel.contentKeyAtEpoch(msg.GetAuthorId(), evtv1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, epoch)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, encryption.ErrKeyNotFound
		}
		contentKey, err := c.unwrapMessageContentKey(ctx, contentKeyEvent)
		if err != nil {
			return nil, err
		}
		plaintext, err := encryption.DecryptWithContentKey(
			contentKey.key,
			msg.GetEncryptedBody(),
			msg.GetEncryptionNonce(),
			messageBodyAAD(eventID, msg.GetBodyEventId(), roomID, msg.GetAuthorId(), epoch),
		)
		if err != nil {
			return nil, messageBodyEnvelopeError(err)
		}
		return plaintext, nil
	}

	if c.encryption.legacyKeys == nil {
		return nil, encryption.ErrKeyNotFound
	}
	key, err := c.encryption.legacyKeys.LegacyUserKey(ctx, msg.GetAuthorId())
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, encryption.ErrKeyNotFound
	}
	plaintext, err := encryption.Decrypt(key, msg.GetEncryptedBody(), msg.GetEncryptionNonce())
	if err != nil {
		return nil, messageBodyEnvelopeError(err)
	}
	return plaintext, nil
}

func messageBodyEnvelopeError(err error) error {
	if errors.Is(err, encryption.ErrDecryptionFailed) ||
		errors.Is(err, encryption.ErrInvalidNonceSize) {
		return fmt.Errorf("%w: %w", ErrMessageBodyCorrupt, err)
	}
	return err
}
