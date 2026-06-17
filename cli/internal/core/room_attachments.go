package core

import (
	"context"

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
	for _, message := range c.rooms().currentRoomAttachmentMessages(roomID) {
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
