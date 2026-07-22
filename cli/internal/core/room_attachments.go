package core

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomAttachmentItem is one current attachment as it appears in a room,
// including the message anchor needed by UI surfaces that jump back to where
// the file was posted.
type RoomAttachmentItem struct {
	Attachment        *corev1.Attachment
	MessageEventID    string
	ThreadRootEventID string
	CreatedAt         *timestamppb.Timestamp
}

// RoomAttachmentsResult is the return type for room-scoped attachment lists.
type RoomAttachmentsResult struct {
	Items      []*RoomAttachmentItem
	TotalCount int
	HasMore    bool
}

// ListRoomAttachmentsInput is the authorized room attachment list request.
type ListRoomAttachmentsInput struct {
	ActorID string
	RoomID  string
	Limit   int
	Offset  int
}

// MessageAttachmentsInput is the authorized current-message attachment request.
type MessageAttachmentsInput struct {
	ActorID string
	RoomID  string
	EventID string
}

// MessageAttachmentSet contains current attachments for one visible message.
type MessageAttachmentSet struct {
	EventID     string
	Attachments []*corev1.Attachment
}

// BatchMessageAttachmentsInput is the authorized current-message attachment
// batch request.
type BatchMessageAttachmentsInput struct {
	ActorID  string
	RoomID   string
	EventIDs []string
}

// RoomAssetInput is the authorized room-scoped asset read request.
type RoomAssetInput struct {
	ActorID string
	RoomID  string
	AssetID string
}

// BatchRoomAssetsInput is the authorized room-scoped asset batch read request.
type BatchRoomAssetsInput struct {
	ActorID  string
	RoomID   string
	AssetIDs []string
}

// ListRoomAttachments returns current message-owned attachments for a room the
// actor belongs to.
func (c *ChattoCore) ListRoomAttachments(ctx context.Context, input ListRoomAttachmentsInput) (*RoomAttachmentsResult, error) {
	_, kind, err := c.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return c.GetRoomAttachments(ctx, kind, input.RoomID, input.Limit, input.Offset)
}

// GetRoomAsset returns one room-scoped asset for a room the actor belongs to.
// Missing, deleted, and wrong-room asset IDs return ErrNotFound.
func (c *ChattoCore) GetRoomAsset(ctx context.Context, input RoomAssetInput) (*corev1.Attachment, error) {
	room, _, err := c.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return c.roomAsset(room.Id, input.AssetID)
}

// BatchGetRoomAssets returns room-scoped assets for a room the actor belongs
// to. Missing, deleted, and wrong-room asset IDs are omitted.
func (c *ChattoCore) BatchGetRoomAssets(ctx context.Context, input BatchRoomAssetsInput) ([]*corev1.Attachment, error) {
	room, _, err := c.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(input.AssetIDs))
	out := make([]*corev1.Attachment, 0, len(input.AssetIDs))
	for _, assetID := range input.AssetIDs {
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		attachment, err := c.roomAsset(room.Id, assetID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, attachment)
	}
	return out, nil
}

func (c *ChattoCore) roomAsset(roomID, assetID string) (*corev1.Attachment, error) {
	if assetID == "" {
		return nil, invalidArgument("asset_id is required")
	}
	declared, ok := c.assetLifecycle().AssetCreation(assetID)
	if !ok || declared == nil || c.assetLifecycle().AssetDeleted(assetID) {
		return nil, ErrNotFound
	}
	assetRoomID, ok := c.assetLifecycle().AssetRoomID(assetID)
	if !ok || assetRoomID != roomID {
		return nil, ErrNotFound
	}
	attachment := AttachmentFromAsset(declared.GetAsset())
	if attachment == nil {
		return nil, ErrNotFound
	}
	attachment.RoomId = roomID
	return attachment, nil
}

// MessageAttachments returns the current attachments for one visible message in
// a room the actor belongs to. Retracted, hidden, wrong-room, and non-message
// event IDs return ErrMessageNotFound so callers do not learn more than the
// timeline read path would reveal.
func (c *ChattoCore) MessageAttachments(ctx context.Context, input MessageAttachmentsInput) ([]*corev1.Attachment, error) {
	_, kind, err := c.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return c.messageAttachments(ctx, kind, input.RoomID, input.EventID)
}

// BatchMessageAttachments returns current attachments for visible messages in a
// room the actor belongs to. Missing, retracted, hidden, wrong-room, and
// non-message event IDs are omitted.
func (c *ChattoCore) BatchMessageAttachments(ctx context.Context, input BatchMessageAttachmentsInput) ([]*MessageAttachmentSet, error) {
	_, kind, err := c.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(input.EventIDs))
	out := make([]*MessageAttachmentSet, 0, len(input.EventIDs))
	for _, eventID := range input.EventIDs {
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}

		attachments, err := c.messageAttachments(ctx, kind, input.RoomID, eventID)
		if err != nil {
			if errors.Is(err, ErrMessageNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, &MessageAttachmentSet{
			EventID:     eventID,
			Attachments: attachments,
		})
	}
	return out, nil
}

func (c *ChattoCore) messageAttachments(ctx context.Context, kind RoomKind, roomID, eventID string) ([]*corev1.Attachment, error) {
	event, err := c.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.GetMessagePosted() == nil {
		return nil, ErrMessageNotFound
	}
	body, err := c.GetFullMessageBodyByEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, ErrMessageNotFound
	}
	out := make([]*corev1.Attachment, 0, len(body.Attachments))
	for _, attachment := range body.Attachments {
		if attachment == nil {
			continue
		}
		cloned := proto.Clone(attachment).(*corev1.Attachment)
		cloned.RoomId = roomID
		if cloned.MessageBodyId == "" {
			cloned.MessageBodyId = eventID
		}
		out = append(out, cloned)
	}
	return out, nil
}

// GetRoomAttachments returns current message-owned attachments in newest
// message order. It includes root messages and thread replies, reads the room
// timeline projection's current attachment-message index, and preserves
// attachment order within each message.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomAttachments(ctx context.Context, kind RoomKind, roomID string, limit int, offset int) (*RoomAttachmentsResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	items := make([]*RoomAttachmentItem, 0)
	for _, message := range c.roomModel.currentRoomAttachmentMessages(roomID) {
		if message.Entry == nil || message.Entry.Event == nil || message.Body == nil {
			continue
		}
		posted := message.Entry.Event.GetMessagePosted()
		if posted == nil {
			continue
		}
		attachments := c.MessageBodyAttachments(message.Body)
		if len(attachments) == 0 {
			continue
		}
		for _, attachment := range attachments {
			if attachment == nil {
				continue
			}
			cloned := proto.Clone(attachment).(*corev1.Attachment)
			cloned.RoomId = roomID
			if cloned.MessageBodyId == "" {
				cloned.MessageBodyId = message.Entry.Event.GetId()
			}
			items = append(items, &RoomAttachmentItem{
				Attachment:        cloned,
				MessageEventID:    message.Entry.Event.GetId(),
				ThreadRootEventID: posted.GetInThread(),
				CreatedAt:         message.Entry.Event.GetCreatedAt(),
			})
		}
	}

	page, totalCount, hasMore := paginateCoreSlice(items, limit, offset)
	return &RoomAttachmentsResult{
		Items:      page,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

func paginateCoreSlice[T any](items []T, limit int, offset int) ([]T, int, bool) {
	totalCount := len(items)
	if offset >= totalCount {
		return []T{}, totalCount, false
	}
	page := items[offset:]
	if limit > 0 && len(page) > limit {
		page = page[:limit]
	}
	return page, totalCount, offset+len(page) < totalCount
}
