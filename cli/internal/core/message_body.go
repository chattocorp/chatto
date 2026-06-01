package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

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

// GetFullMessageBody returns the decrypted message body for a message.
//
// `messageBodyKey` is the legacy compound key `{userId}.{eventId}`
// retained for API compatibility with the pre-ES resolver layer. We
// extract the eventId from the second segment, look the message up in
// the RoomTimelineProjection, and fold any subsequent edit / retract
// events to produce the current body. The userId prefix on the key is
// not consulted — the projection knows the author from the event
// envelope.
//
// Returns nil if the message has been retracted or doesn't exist, or
// if the author's encryption key has been crypto-shredded.
func (c *ChattoCore) GetFullMessageBody(ctx context.Context, kind RoomKind, messageBodyKey string) (*DecryptedMessageBody, error) {
	eventID := eventIDFromBodyKey(messageBodyKey)
	if eventID == "" {
		return nil, nil
	}
	return c.GetFullMessageBodyByEventID(ctx, eventID)
}

// GetFullMessageBodyByEventID is the canonical post-cutover body
// accessor — look the message up directly by its envelope event id
// without the legacy compound key indirection.
func (c *ChattoCore) GetFullMessageBodyByEventID(ctx context.Context, eventID string) (*DecryptedMessageBody, error) {
	if eventID == "" {
		return nil, nil
	}

	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return nil, nil
	}
	posted := entry.Event.GetMessagePosted()
	if posted == nil {
		return nil, nil
	}

	body, retracted, _ := c.RoomTimeline.LatestBody(eventID)
	if retracted || body == nil {
		// Retracted message: same shape as a legacy GDPR delete —
		// resolver renders "[Message unavailable]".
		return nil, nil
	}

	plaintext, err := c.decryptMessageBody(ctx, body)
	if err != nil {
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil // crypto-shredded
		}
		return nil, fmt.Errorf("failed to decrypt message body: %w", err)
	}

	result := &DecryptedMessageBody{
		AuthorId:    body.GetAuthorId(),
		Body:        string(plaintext),
		Attachments: c.MessageBodyAttachments(body),
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
// text. Same semantics as GetFullMessageBody — retracted / crypto-
// shredded messages return empty string.
func (c *ChattoCore) GetMessageBody(ctx context.Context, kind RoomKind, messageBodyKey string) (string, error) {
	body, err := c.GetFullMessageBody(ctx, kind, messageBodyKey)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	return body.Body, nil
}

// GetMessageAuthorID returns the author of a message by its compound
// body key. Used by the GraphQL layer to check ownership before
// edit / delete.
func (c *ChattoCore) GetMessageAuthorID(ctx context.Context, kind RoomKind, messageBodyID string) (string, error) {
	eventID := eventIDFromBodyKey(messageBodyID)
	if eventID == "" {
		return "", nil
	}
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return "", nil
	}
	return entry.Event.GetActorId(), nil
}

// decryptMessageBody decrypts an encrypted message body using the
// author's per-user encryption key.
func (c *ChattoCore) decryptMessageBody(ctx context.Context, msg *corev1.MessageBody) ([]byte, error) {
	key, err := c.encryption.keyManager.GetUserKey(ctx, msg.GetAuthorId())
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, encryption.ErrKeyNotFound
	}
	return encryption.Decrypt(key, msg.GetEncryptedBody(), msg.GetEncryptionNonce())
}

// (eventIDFromBodyKey is defined in core.go and shared with the
// publish path. Body keys have the legacy format {userId}.{eventId};
// the projection-backed body readers in this file consult it to
// extract the event-id portion.)

// deleteUserMessageBodiesInSpace deletes all message bodies authored
// by a user in a specific space's legacy SERVER_BODIES bucket.
//
// This is retained for GDPR / account-deletion cleanup of the legacy
// bucket during the migration window. Once ADR-035 phase 7
// decommissions SERVER_BODIES, this becomes a no-op and can be
// removed alongside the bucket. New posts (post-cutover) don't write
// to SERVER_BODIES at all, so this function only touches data
// imported from before the cutover.
//
// Returns the number of legacy body entries deleted.
func (c *ChattoCore) deleteUserMessageBodiesInSpace(ctx context.Context, userID string, kind RoomKind) (int, error) {
	bucket := c.storage.serverBodiesKV
	if bucket == nil {
		return 0, nil
	}

	lister, err := bucket.ListKeysFiltered(ctx, userID+".>")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list message body keys: %w", err)
	}

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	deleted := 0
	for _, key := range keys {
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			c.logger.Debug("Failed to get message body during deletion", "key", key, "error", err)
			continue
		}

		var messageBody corev1.MessageBody
		if err := proto.Unmarshal(entry.Value(), &messageBody); err != nil {
			c.logger.Debug("Failed to unmarshal message body during deletion", "key", key, "error", err)
			continue
		}

		for _, attachment := range c.MessageBodyAttachments(&messageBody) {
			if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
				c.logger.Warn("Failed to delete attachment during user deletion",
					"attachment_id", attachment.Id,
					"message_body_key", key,
					"error", err)
			}
		}

		if err := bucket.Delete(ctx, key); err != nil {
			c.logger.Warn("Failed to delete message body during user deletion", "key", key, "error", err)
			continue
		}
		deleted++
	}
	return deleted, nil
}
