// Package notificationcontent hydrates display content shared by notification delivery surfaces.
package notificationcontent

import (
	"context"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/core"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/cli/internal/push"
)

// PayloadContext loads best-effort display content for Web Push and native alerts.
// Callers must authorize the occurrence before hydration and recheck visibility
// after it, before returning or delivering the content. No content is cached.
func PayloadContext(ctx context.Context, chattoCore *core.ChattoCore, occurrence *notificationv1.NotificationOccurrence, logger *log.Logger) *push.PayloadContext {
	if invitation := occurrence.GetSignal().GetRoomInvitationReceived(); invitation != nil {
		room, err := chattoCore.FindRoomByID(ctx, invitation.GetRoomId())
		if err != nil {
			return nil
		}
		return &push.PayloadContext{RoomName: room.GetName()}
	}
	target := core.NotificationOccurrenceMessageReference(occurrence)
	if target == nil {
		return nil
	}
	roomID := target.GetRoomId()
	eventID := target.GetEventId()

	if eventID == "" {
		return nil
	}

	payloadCtx := &push.PayloadContext{}

	kind, err := chattoCore.FindRoomKind(ctx, roomID)
	if err != nil {
		logger.Debug("Failed to resolve room kind for push notification preview",
			"room_id", roomID, "error", err)
		return nil
	}

	// Fetch the message to get its body
	event, err := chattoCore.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		logger.Debug("Failed to fetch event for push notification preview",
			"event_id", eventID,
			"error", err)
		return nil
	}
	if event == nil {
		return nil
	}

	// Extract message body from the event
	if _, ok := event.Event.(*evtv1.Event_MessagePosted); ok {
		body, err := chattoCore.GetMessageBody(ctx, event.Id)
		if err != nil {
			logger.Debug("Failed to fetch message body for push notification preview",
				"event_id", event.Id,
				"error", err)
		} else {
			payloadCtx.MessagePreview = body
		}
	}

	if kind != core.KindDM {
		room, err := chattoCore.GetRoom(ctx, kind, roomID)
		if err != nil {
			logger.Debug("Failed to fetch room for push notification",
				"room_id", roomID,
				"error", err)
		} else if room != nil {
			payloadCtx.RoomName = room.Name
		}
	}

	return payloadCtx
}
