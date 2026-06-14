package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type userDEK struct {
	epoch   int32
	purpose corev1.UserDEKPurpose
	key     []byte
}

type messageContentKey = userDEK

func messageBodyAAD(eventID, bodyEventID, roomID, authorID string, epoch int32) []byte {
	if bodyEventID == "" {
		return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00room_id=%s\x00author_id=%s\x00content_key_epoch=%d", eventID, roomID, authorID, epoch))
	}
	return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00body_event_id=%s\x00room_id=%s\x00author_id=%s\x00content_key_epoch=%d", eventID, bodyEventID, roomID, authorID, epoch))
}

func contentKeyAAD(userID string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:content-key-context:v2\x00user_id=%s\x00epoch=%d", userID, epoch))
}

func userDEKAAD(userID string, purpose corev1.UserDEKPurpose, epoch int32) []byte {
	if purpose == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return contentKeyAAD(userID, epoch)
	}
	return []byte(fmt.Sprintf("chatto:user-dek-context:v1\x00user_id=%s\x00purpose=%d\x00epoch=%d", userID, purpose, epoch))
}

func (c *ChattoCore) encryptMessageBody(ctx context.Context, body *corev1.MessageBody, roomID, eventID, bodyEventID, plaintext string) error {
	if body == nil {
		return fmt.Errorf("message body is nil")
	}
	authorID := body.GetAuthorId()
	if authorID == "" {
		return fmt.Errorf("message body author is empty")
	}
	if bodyEventID == "" {
		return fmt.Errorf("message body event ID is empty")
	}
	contentKey, err := c.ensureActiveMessageContentKey(ctx, authorID)
	if err != nil {
		return err
	}

	encrypted, err := encryption.EncryptWithContentKey(contentKey.key, []byte(plaintext), messageBodyAAD(eventID, bodyEventID, roomID, authorID, contentKey.epoch))
	if err != nil {
		return fmt.Errorf("failed to encrypt message body: %w", err)
	}
	body.EncryptedBody = encrypted.Ciphertext
	body.EncryptionNonce = encrypted.Nonce
	body.EncryptionVersion = encryption.EnvelopeVersionV2
	body.ContentKeyEpoch = contentKey.epoch
	body.BodyEventId = bodyEventID
	return nil
}

func (c *ChattoCore) ensureActiveMessageContentKey(ctx context.Context, userID string) (*messageContentKey, error) {
	return c.ensureActiveUserDEK(ctx, userID, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
}

func (c *ChattoCore) ensureActiveUserPIIDEK(ctx context.Context, userID string) (*userDEK, error) {
	return c.ensureActiveUserDEK(ctx, userID, corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
}

func (c *ChattoCore) ensureActiveUserDEK(ctx context.Context, userID string, purpose corev1.UserDEKPurpose) (*userDEK, error) {
	if event, ok := c.ContentKeys.Active(userID, purpose); ok {
		return c.unwrapUserDEK(ctx, event, purpose)
	}
	return c.generateInitialUserDEK(ctx, userID, purpose)
}

func (c *ChattoCore) unwrapMessageContentKey(ctx context.Context, event *corev1.UserDEKGeneratedEvent) (*messageContentKey, error) {
	return c.unwrapUserDEK(ctx, event, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
}

func (c *ChattoCore) unwrapUserDEK(ctx context.Context, event *corev1.UserDEKGeneratedEvent, purpose corev1.UserDEKPurpose) (*userDEK, error) {
	if event == nil {
		return nil, fmt.Errorf("DEK event is nil")
	}
	userID := event.GetUserId()
	epoch := event.GetEpoch()
	contentKeyRef := event.GetContentKeyRef()
	if userID == "" || epoch <= 0 || contentKeyRef == "" {
		return nil, fmt.Errorf("invalid DEK event")
	}
	eventPurpose := event.GetPurpose()
	if eventPurpose != corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED && purpose != corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED && eventPurpose != purpose {
		return nil, fmt.Errorf("DEK purpose mismatch: event has %s, want %s", eventPurpose.String(), purpose.String())
	}
	if c.encryption.keyWrapper == nil {
		return nil, encryption.ErrKeyNotFound
	}
	if c.encryption.contentKeys == nil {
		return nil, encryption.ErrKeyNotFound
	}
	stored, err := c.encryption.contentKeys.Get(ctx, contentKeyRef)
	if err != nil {
		return nil, fmt.Errorf("failed to load DEK: %w", err)
	}
	keyRef := stored.WrappingKeyRef
	if keyRef == "" {
		keyRef = kms.LegacyUserKeyRef(userID)
	}
	key, err := c.encryption.keyWrapper.UnwrapContentKey(ctx, keyRef, kms.WrappedContentKey{
		EncryptedContentKey: stored.EncryptedContentKey,
		Nonce:               stored.ContentKeyNonce,
		Algorithm:           stored.WrappingAlgorithm,
		Metadata:            stored.WrappingMetadata,
	}, userDEKAAD(userID, eventPurpose, epoch))
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap DEK: %w", err)
	}
	return &userDEK{epoch: epoch, purpose: eventPurpose, key: key}, nil
}

func (c *ChattoCore) generateInitialUserDEK(ctx context.Context, userID string, purpose corev1.UserDEKPurpose) (*userDEK, error) {
	agg := events.UserAggregate(userID)
	filter := agg.AllEventsFilter()
	subject := agg.Subject(events.EventUserDEKGenerated)

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read DEK OCC filter seq: %w", err)
		}
		if err := c.userService.waitForContentKeysCurrent(ctx, userID); err != nil {
			return nil, err
		}
		if event, ok := c.ContentKeys.Active(userID, purpose); ok {
			return c.unwrapUserDEK(ctx, event, purpose)
		}

		keyRef, err := c.encryption.keyWrapper.CreateKey(ctx, userID)
		if err != nil {
			return nil, err
		}
		createdKey := true

		key, wrapped, err := c.newWrappedUserDEK(ctx, userID, keyRef, 1, purpose)
		if err != nil {
			if createdKey {
				_ = c.encryption.keyWrapper.ShredKey(context.WithoutCancel(ctx), keyRef)
			}
			return nil, err
		}
		event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: wrapped,
		}})

		seq, err := c.EventPublisher.AppendAtFilter(ctx, subject, event, filter, filterSeq)
		if err == nil {
			if err := c.userService.waitForContentKeys(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return nil, fmt.Errorf("wait for DEK projection: %w", err)
			}
			return &userDEK{epoch: 1, purpose: purpose, key: key}, nil
		}
		_ = c.encryption.contentKeys.Shred(context.WithoutCancel(ctx), wrapped.GetContentKeyRef())
		if createdKey {
			_ = c.encryption.keyWrapper.ShredKey(context.WithoutCancel(ctx), keyRef)
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
	return nil, fmt.Errorf("DEK OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) newWrappedMessageContentKey(ctx context.Context, userID, keyRef string, epoch int32) ([]byte, *corev1.UserDEKGeneratedEvent, error) {
	return c.newWrappedUserDEK(ctx, userID, keyRef, epoch, corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY)
}

func (c *ChattoCore) newWrappedUserDEK(ctx context.Context, userID, keyRef string, epoch int32, purpose corev1.UserDEKPurpose) ([]byte, *corev1.UserDEKGeneratedEvent, error) {
	if c.encryption.keyWrapper == nil {
		return nil, nil, encryption.ErrKeyNotFound
	}
	if c.encryption.contentKeys == nil {
		return nil, nil, encryption.ErrKeyNotFound
	}
	key, err := encryption.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	wrapped, err := c.encryption.keyWrapper.WrapContentKey(ctx, keyRef, key, userDEKAAD(userID, purpose, epoch))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to wrap DEK: %w", err)
	}
	stored := &corev1.UserDataEncryptionKey{
		EncryptedContentKey: wrapped.EncryptedContentKey,
		ContentKeyNonce:     wrapped.Nonce,
		WrappingAlgorithm:   wrapped.Algorithm,
		WrappingMetadata:    wrapped.Metadata,
		WrappingKeyRef:      keyRef,
	}
	contentKeyRef, err := c.encryption.contentKeys.Create(ctx, stored)
	if err != nil {
		return nil, nil, err
	}
	return key, &corev1.UserDEKGeneratedEvent{
		UserId:            userID,
		Epoch:             epoch,
		Purpose:           purpose,
		ContentKeyRef:     contentKeyRef,
		WrappingAlgorithm: stored.WrappingAlgorithm,
		WrappingMetadata:  stored.WrappingMetadata,
		WrappingKeyRef:    stored.WrappingKeyRef,
	}, nil
}
