package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DecryptedMessageBody is the public view of a message body with
// plaintext content. The body's encryption envelope is unwrapped by
// the resolver layer's decryptMessageBody helper.
type DecryptedMessageBody struct {
	AuthorId    string
	Body        string
	Attachments []*corev1.Attachment
	LinkPreview *corev1.LinkPreview
	CreatedAt   time.Time
	UpdatedAt   *time.Time
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
	if !ok {
		return nil, nil
	}
	posted := entry.Event.GetMessagePosted()
	if posted == nil {
		return nil, nil
	}

	body, retracted, _ := c.roomModel.latestBody(eventID)
	if retracted || body == nil {
		// Retracted message: same shape as a legacy GDPR delete —
		// resolver renders "[Message unavailable]".
		return nil, nil
	}

	plaintext, err := c.decryptMessageBody(ctx, eventID, posted.GetRoomId(), body)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil // crypto-shredded
		}
		return nil, fmt.Errorf("failed to decrypt message body: %w", err)
	}

	result := &DecryptedMessageBody{
		AuthorId:    body.GetAuthorId(),
		Body:        string(plaintext),
		Attachments: c.mediaModel.MessageBodyAttachments(body),
		LinkPreview: body.GetLinkPreview(),
		CreatedAt:   entry.Event.GetCreatedAt().AsTime(),
	}
	// UpdatedAt: if LatestBody returned a body different from the
	// original post's body, the message has been edited. The body
	// proto carries its own UpdatedAt; surface that if set, otherwise
	// derive from the most recent edit's envelope time.
	if upd := body.GetUpdatedAt(); upd != nil {
		t := upd.AsTime()
		result.UpdatedAt = &t
	}
	return result, nil
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
func (c *ChattoCore) decryptMessageBody(ctx context.Context, eventID, roomID string, msg *corev1.MessageBody) ([]byte, error) {
	if msg.GetEncryptionVersion() >= encryption.EnvelopeVersionV2 || msg.GetContentKeyEpoch() > 0 {
		version := msg.GetEncryptionVersion()
		if version != encryption.EnvelopeVersionV2 {
			return nil, fmt.Errorf("%w: unsupported message body encryption version %d", ErrMessageBodyCorrupt, version)
		}
		epoch := msg.GetContentKeyEpoch()
		if epoch <= 0 {
			return nil, fmt.Errorf("%w: missing content key epoch for v%d message body", ErrMessageBodyCorrupt, version)
		}
		contentKeyEvent, ok := c.ContentKeys.Get(msg.GetAuthorId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, epoch)
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
