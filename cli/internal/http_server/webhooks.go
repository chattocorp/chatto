package http_server

import (
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
	"hmans.de/chatto/internal/core"
)

func (s *HTTPServer) setupWebhookRoutes() {
	if !s.config.LiveKit.IsConfigured() {
		return
	}

	webhooks := s.router.Group("/webhooks")
	webhooks.POST("/livekit", s.handleLiveKitWebhook)
	registerTestWebhookEndpoints(webhooks, s)
}

func (s *HTTPServer) handleLiveKitWebhook(c *gin.Context) {
	logger := log.WithPrefix("webhook.livekit")

	webhookKey, webhookSecret := s.config.LiveKit.WebhookKeyPair()
	provider := auth.NewSimpleKeyProvider(webhookKey, webhookSecret)
	event, err := webhook.ReceiveWebhookEvent(c.Request, provider)
	if err != nil {
		logger.Warn("Webhook validation failed", "error", err)
		c.Status(http.StatusUnauthorized)
		return
	}

	// Extract space and room IDs from the LiveKit room name
	if event.Room == nil {
		c.Status(http.StatusOK)
		return
	}
	if !liveKitWebhookRoomBelongsToInstance(event.Room.Name, s.config.LiveKit.ServerID) {
		logger.Warn("Ignoring LiveKit webhook for foreign room", "room", event.Room.Name, "instance", s.config.LiveKit.ServerID)
		c.Status(http.StatusOK)
		return
	}
	spaceID, roomID := core.ParseLiveKitRoomName(event.Room.Name)
	if spaceID == "" || roomID == "" {
		logger.Warn("Unrecognized LiveKit room name", "name", event.Room.Name)
		c.Status(http.StatusOK)
		return
	}

	ctx := c.Request.Context()

	switch event.Event {
	case webhook.EventParticipantJoined:
		if event.Participant == nil {
			break
		}
		md := core.ParseParticipantMetadata(event.Participant.Metadata)
		if err := s.core.HandleCallParticipantJoined(
			ctx, spaceID, roomID,
			event.Participant.Identity,
			event.Participant.Name,
			md.Login, md.AvatarURL,
		); err != nil {
			logger.Warn("Failed to handle participant joined", "error", err)
		}

	case webhook.EventParticipantLeft:
		if event.Participant == nil {
			break
		}
		if err := s.core.HandleCallParticipantLeft(
			ctx, spaceID, roomID,
			event.Participant.Identity,
		); err != nil {
			logger.Warn("Failed to handle participant left", "error", err)
		}

	case webhook.EventRoomFinished:
		if err := s.core.HandleCallRoomFinished(ctx, spaceID, roomID); err != nil {
			logger.Warn("Failed to handle room finished", "error", err)
		}
	}

	c.Status(http.StatusOK)
}

func liveKitWebhookRoomBelongsToInstance(roomName, instanceID string) bool {
	roomInstanceID := core.ParseLiveKitRoomServerID(roomName)
	if instanceID == "" {
		return roomInstanceID == ""
	}
	return roomInstanceID == instanceID
}
