package bleve

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (p *Projection) decryptBody(ctx context.Context, eventID, roomID string, body *corev1.MessageBody) ([]byte, error) {
	return p.decryptBodyWithDEKs(ctx, eventID, roomID, body, p.deks)
}

func (p *Projection) decryptBodyWithDEKs(ctx context.Context, eventID, roomID string, body *corev1.MessageBody, deks map[string]*corev1.UserDEKGeneratedEvent) ([]byte, error) {
	if body.GetEncryptionVersion() >= encryption.EnvelopeVersionV2 || body.GetContentKeyEpoch() > 0 {
		if body.GetEncryptionVersion() != encryption.EnvelopeVersionV2 || body.GetContentKeyEpoch() <= 0 {
			return nil, fmt.Errorf("invalid message body encryption envelope")
		}
		dek := deks[dekKey(body.GetAuthorId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, body.GetContentKeyEpoch())]
		if dek == nil {
			dek = deks[dekKey(body.GetAuthorId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED, body.GetContentKeyEpoch())]
		}
		if dek == nil || p.keyWrapper == nil || p.dekStore == nil {
			return nil, encryption.ErrKeyNotFound
		}
		stored, err := p.dekStore.Get(ctx, dek.GetContentKeyRef())
		if err != nil {
			return nil, err
		}
		keyRef := stored.GetWrappingKeyRef()
		if keyRef == "" {
			keyRef = kms.LegacyUserKeyRef(dek.GetUserId())
		}
		key, err := p.keyWrapper.UnwrapContentKey(ctx, keyRef, kms.WrappedContentKey{
			EncryptedContentKey: stored.GetEncryptedContentKey(),
			Nonce:               stored.GetContentKeyNonce(),
			Algorithm:           stored.GetWrappingAlgorithm(),
			Metadata:            stored.GetWrappingMetadata(),
		}, encryption.UserDEKAAD(dek.GetUserId(), dek.GetPurpose(), dek.GetEpoch()))
		if err != nil {
			return nil, err
		}
		return encryption.DecryptWithContentKey(key, body.GetEncryptedBody(), body.GetEncryptionNonce(), encryption.MessageBodyAAD(eventID, body.GetBodyEventId(), roomID, body.GetAuthorId(), body.GetContentKeyEpoch()))
	}
	if p.legacyKeys == nil {
		return nil, encryption.ErrKeyNotFound
	}
	key, err := p.legacyKeys.LegacyUserKey(ctx, body.GetAuthorId())
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		return nil, encryption.ErrKeyNotFound
	}
	plaintext, err := encryption.Decrypt(key, body.GetEncryptedBody(), body.GetEncryptionNonce())
	if errors.Is(err, encryption.ErrDecryptionFailed) || errors.Is(err, encryption.ErrInvalidNonceSize) {
		return nil, fmt.Errorf("invalid message body encryption envelope: %w", err)
	}
	return plaintext, err
}
