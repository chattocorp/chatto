package http_server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
	"hmans.de/chatto/internal/core"
)

const maxIncomingBotWebhookBodyBytes = 16 << 10

type incomingBotWebhookRequest struct {
	RoomID string `json:"room_id"`
	Body   string `json:"body"`
}

type incomingBotWebhookResponse struct {
	MessageID string `json:"message_id"`
}

func (s *HTTPServer) setupIncomingBotWebhookRoutes() {
	s.router.POST("/webhooks/bots/:botID", s.handleIncomingBotWebhook)
}

// handleIncomingBotWebhook is deliberately write-only: a bot API key can post
// a root text message to an explicitly installed channel but cannot use this
// HTTP surface to inspect rooms or messages.
func (s *HTTPServer) handleIncomingBotWebhook(c *gin.Context) {
	values := c.Request.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		c.Status(http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || strings.TrimSpace(token) != token {
		c.Status(http.StatusUnauthorized)
		return
	}
	credential, err := s.core.ValidateBotAPIKey(c.Request.Context(), token)
	if err != nil || credential.UserID != c.Param("botID") {
		c.Status(http.StatusUnauthorized)
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxIncomingBotWebhookBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request incomingBotWebhookRequest
	if err := decoder.Decode(&request); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.Status(http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.RoomID) == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	result, err := s.core.Messages().PostBotChannelMessage(c.Request.Context(), core.MessagePostInput{
		ActorID: credential.UserID,
		RoomID:  request.RoomID,
		Body:    request.Body,
	})
	if err != nil {
		switch {
		case errors.Is(err, core.ErrInvalidArgument), errors.Is(err, core.ErrMessageTooLong):
			c.Status(http.StatusBadRequest)
		case errors.Is(err, core.ErrPermissionDenied), errors.Is(err, core.ErrNotRoomMember), errors.Is(err, core.ErrNotFound):
			c.Status(http.StatusForbidden)
		default:
			c.Status(http.StatusInternalServerError)
		}
		return
	}
	if result == nil || result.Event == nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, incomingBotWebhookResponse{MessageID: result.Event.GetId()})
}

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

	// Parse the legacy LiveKit room name at the integration boundary.
	if event.Room == nil {
		c.Status(http.StatusOK)
		return
	}
	if !liveKitWebhookRoomBelongsToInstance(event.Room.Name, s.config.LiveKit.ServerID) {
		logger.Warn("Ignoring LiveKit webhook for foreign room", "room", event.Room.Name, "instance", s.config.LiveKit.ServerID)
		c.Status(http.StatusOK)
		return
	}
	legacySpaceID, roomID, callID := core.ParseLiveKitRoomIdentity(event.Room.Name)
	if legacySpaceID == "" || roomID == "" {
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
		eventCallID := callID
		if eventCallID == "" {
			eventCallID = md.CallID
		}
		if eventCallID == "" {
			logger.Warn("Ignoring LiveKit participant joined without call ID", "room", event.Room.Name)
			break
		}
		if err := s.core.HandleCallParticipantJoined(
			ctx, roomID,
			event.Participant.Identity,
			eventCallID,
		); err != nil {
			logger.Warn("Failed to handle participant joined", "error", err)
		}

	case webhook.EventParticipantLeft:
		if event.Participant == nil {
			break
		}
		if liveKitParticipantLeftIsConnectionHandoff(event.Participant) {
			break
		}
		md := core.ParseParticipantMetadata(event.Participant.Metadata)
		eventCallID := callID
		if eventCallID == "" {
			eventCallID = md.CallID
		}
		if eventCallID == "" {
			logger.Warn("Ignoring LiveKit participant left without call ID", "room", event.Room.Name)
			break
		}
		if err := s.core.HandleCallParticipantLeft(
			ctx, roomID,
			event.Participant.Identity,
			eventCallID,
		); err != nil {
			logger.Warn("Failed to handle participant left", "error", err)
		}

	case webhook.EventRoomFinished:
		if callID == "" {
			logger.Warn("Ignoring LiveKit room finished without call ID", "room", event.Room.Name)
			break
		}
		if err := s.core.HandleCallRoomFinished(ctx, roomID, callID); err != nil {
			logger.Warn("Failed to handle room finished", "error", err)
		}
	}

	c.Status(http.StatusOK)
}

func liveKitParticipantLeftIsConnectionHandoff(participant *livekit.ParticipantInfo) bool {
	if participant == nil {
		return false
	}
	// Chatto call membership is user-scoped, while LiveKit duplicate-identity
	// replacement is connection-scoped. A new tab/device taking over the same
	// user identity should not become a durable domain leave.
	return participant.GetDisconnectReason() == livekit.DisconnectReason_DUPLICATE_IDENTITY
}

func liveKitWebhookRoomBelongsToInstance(roomName, instanceID string) bool {
	roomInstanceID := core.ParseLiveKitRoomServerID(roomName)
	if instanceID == "" {
		return roomInstanceID == ""
	}
	return roomInstanceID == instanceID
}
