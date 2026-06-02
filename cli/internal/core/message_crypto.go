package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type messageContentKey struct {
	epoch int32
	key   []byte
}

func messageBodyAAD(eventID, roomID, authorID string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00room_id=%s\x00author_id=%s\x00content_key_epoch=%d", eventID, roomID, authorID, epoch))
}

func contentKeyAAD(userID string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:content-key-context:v2\x00user_id=%s\x00epoch=%d", userID, epoch))
}

func (c *ChattoCore) encryptMessageBody(ctx context.Context, body *corev1.MessageBody, roomID, eventID, plaintext string) error {
	if body == nil {
		return fmt.Errorf("message body is nil")
	}
	authorID := body.GetAuthorId()
	if authorID == "" {
		return fmt.Errorf("message body author is empty")
	}
	contentKey, err := c.ensureActiveMessageContentKey(ctx, authorID)
	if err != nil {
		return err
	}

	encrypted, err := encryption.EncryptWithContentKey(contentKey.key, []byte(plaintext), messageBodyAAD(eventID, roomID, authorID, contentKey.epoch))
	if err != nil {
		return fmt.Errorf("failed to encrypt message body: %w", err)
	}
	body.EncryptedBody = encrypted.Ciphertext
	body.EncryptionNonce = encrypted.Nonce
	body.EncryptionVersion = encryption.EnvelopeVersionV2
	body.ContentKeyEpoch = contentKey.epoch
	return nil
}

func (c *ChattoCore) ensureActiveMessageContentKey(ctx context.Context, userID string) (*messageContentKey, error) {
	if event, ok := c.ContentKeys.Active(userID); ok {
		return c.unwrapMessageContentKey(ctx, event)
	}
	return c.generateInitialMessageContentKey(ctx, userID)
}

func (c *ChattoCore) unwrapMessageContentKey(ctx context.Context, event *corev1.UserContentKeyGeneratedEvent) (*messageContentKey, error) {
	if event == nil {
		return nil, fmt.Errorf("content key event is nil")
	}
	userID := event.GetUserId()
	epoch := event.GetEpoch()
	if userID == "" || epoch <= 0 {
		return nil, fmt.Errorf("invalid content key event")
	}
	kek, err := c.encryption.keyManager.GetUserKey(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if kek == nil {
		return nil, encryption.ErrKeyNotFound
	}

	key, err := encryption.UnwrapContentKey(kek, event.GetEncryptedContentKey(), event.GetContentKeyNonce(), contentKeyAAD(userID, epoch))
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap content key: %w", err)
	}
	return &messageContentKey{epoch: epoch, key: key}, nil
}

func (c *ChattoCore) generateInitialMessageContentKey(ctx context.Context, userID string) (*messageContentKey, error) {
	agg := events.UserAggregate(userID)
	filter := agg.AllEventsFilter()
	subject := agg.Subject(events.EventUserContentKeyGenerated)

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read content key OCC filter seq: %w", err)
		}
		if err := c.ContentKeysProjector.WaitForSeq(ctx, filterSeq); err != nil {
			return nil, fmt.Errorf("wait for content key projection: %w", err)
		}
		if event, ok := c.ContentKeys.Active(userID); ok {
			return c.unwrapMessageContentKey(ctx, event)
		}

		key, wrapped, err := c.newWrappedMessageContentKey(ctx, userID, 1)
		if err != nil {
			return nil, err
		}
		event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserContentKeyGenerated{
			UserContentKeyGenerated: wrapped,
		}})

		seq, err := c.EventPublisher.AppendAtFilter(ctx, subject, event, filter, filterSeq)
		if err == nil {
			if err := c.ContentKeysProjector.WaitForSeq(ctx, seq); err != nil {
				return nil, fmt.Errorf("wait for content key projection: %w", err)
			}
			return &messageContentKey{epoch: 1, key: key}, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("content key OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) newWrappedMessageContentKey(ctx context.Context, userID string, epoch int32) ([]byte, *corev1.UserContentKeyGeneratedEvent, error) {
	kek, err := c.encryption.keyManager.GetUserKey(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if kek == nil {
		return nil, nil, encryption.ErrKeyNotFound
	}
	key, err := encryption.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	wrapped, err := encryption.WrapContentKey(kek, key, contentKeyAAD(userID, epoch))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to wrap content key: %w", err)
	}
	return key, &corev1.UserContentKeyGeneratedEvent{
		UserId:              userID,
		Epoch:               epoch,
		EncryptedContentKey: wrapped.EncryptedContentKey,
		ContentKeyNonce:     wrapped.Nonce,
	}, nil
}
