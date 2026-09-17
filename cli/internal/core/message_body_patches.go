package core

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// Unknown fields can introduce new retention dependencies. Stop before making
// cleanup decisions rather than treating unsupported state as an empty field.
func rejectUnknownBodyFields(message protoreflect.Message) error {
	if len(message.GetUnknown()) != 0 {
		return fmt.Errorf("%w: unsupported body fields", ErrMessageBodyCorrupt)
	}
	var err error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			for i := 0; i < value.List().Len() && err == nil; i++ {
				err = rejectUnknownBodyFields(value.List().Get(i).Message())
			}
		} else {
			err = rejectUnknownBodyFields(value.Message())
		}
		return err == nil
	})
	return err
}

// buildMessageBodyPatch emits a masked body. Unselected fields stay in their
// existing payloads. Empty fields are explicitly cleared again so their old
// source can be erased without allowing replay to restore an earlier value.
func (c *ChattoCore) buildMessageBodyPatch(ctx context.Context, roomID, eventID, payloadID string, current, updated *evtv1.MessageBody, plaintext string, oldDescriptions, descriptions map[string]string) (*evtv1.MessageBodyEvent, error) {
	if err := rejectUnknownBodyFields(current.ProtoReflect()); err != nil {
		return nil, err
	}
	oldText, err := c.decryptMessageBody(ctx, eventID, roomID, current)
	if err != nil {
		return nil, err
	}
	changes := evtstream.MessageBodyFields{Text: string(oldText) != plaintext}
	changes.Attachments = messageBodyAttachmentCount(updated) == 0 || !slices.Equal(current.GetAssetIds(), updated.GetAssetIds()) || !proto.Equal(&evtv1.MessageBody{Attachments: current.GetAttachments()}, &evtv1.MessageBody{Attachments: updated.GetAttachments()})
	changes.LinkPreview = updated.GetLinkPreview() == nil || !proto.Equal(current.GetLinkPreview(), updated.GetLinkPreview())
	payload := &evtv1.MessageBody{AuthorId: current.GetAuthorId(), CreatedAt: current.GetCreatedAt(), UpdatedAt: timestamppb.Now(), BodyEventId: payloadID}
	if payload.CreatedAt == nil {
		return nil, fmt.Errorf("%w: missing body creation time", ErrMessageBodyCorrupt)
	}
	if changes.Attachments {
		payload.AssetIds, payload.Attachments = updated.GetAssetIds(), updated.GetAttachments()
	}
	if changes.LinkPreview {
		payload.LinkPreview = updated.GetLinkPreview()
	}
	attached := messageBodyAttachmentIDs(updated)
	descriptions = maps.Clone(descriptions)
	for id := range descriptions {
		if !slices.Contains(attached, id) || descriptions[id] == "" {
			delete(descriptions, id)
		}
	}
	changes.Descriptions = len(descriptions) == 0 || !maps.Equal(oldDescriptions, descriptions)
	changedValues := make(map[string]string)
	if changes.Descriptions {
		for id, value := range descriptions {
			if oldDescriptions[id] != value {
				changedValues[id] = value
			}
		}
		// A mask replaces the whole list. Preserve the encryption context of
		// unchanged descriptions copied into that list.
		for _, description := range current.GetAttachmentDescriptions() {
			id := description.GetAssetId()
			if descriptions[id] == "" || changedValues[id] != "" {
				continue
			}
			value := proto.Clone(description).(*evtv1.EncryptedAttachmentDescription)
			if value.SourceBodyEventId == "" {
				value.SourceBodyEventId = current.GetBodyEventId()
			}
			payload.AttachmentDescriptions = append(payload.AttachmentDescriptions, value)
		}
	}
	if changes.Text || len(changedValues) > 0 {
		key, err := c.ensureActiveMessageContentKey(ctx, payload.AuthorId)
		if err != nil {
			return nil, err
		}
		if changes.Text {
			sealed, err := encryption.EncryptWithContentKey(key.key, []byte(plaintext), messageBodyAAD(eventID, payloadID, roomID, payload.AuthorId, key.epoch))
			if err != nil {
				return nil, err
			}
			payload.EncryptionVersion, payload.ContentKeyEpoch = encryption.EnvelopeVersionV2, key.epoch
			payload.EncryptedBody, payload.EncryptionNonce = sealed.Ciphertext, sealed.Nonce
		}
		for _, id := range slices.Sorted(maps.Keys(changedValues)) {
			sealed, err := encryption.EncryptWithContentKey(key.key, []byte(changedValues[id]), attachmentDescriptionAAD(eventID, payloadID, roomID, payload.AuthorId, id, key.epoch))
			if err != nil {
				return nil, err
			}
			payload.AttachmentDescriptions = append(payload.AttachmentDescriptions, &evtv1.EncryptedAttachmentDescription{AssetId: id, EncryptionVersion: encryption.EnvelopeVersionV2, ContentKeyEpoch: key.epoch, EncryptedDescription: sealed.Ciphertext, EncryptionNonce: sealed.Nonce})
		}
	}
	return &evtv1.MessageBodyEvent{RoomId: roomID, EventId: eventID, Body: payload, UpdateMask: changes.Mask()}, nil
}
