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

func (s *HTTPServer) setupWebhookRoutes() {
	webhooks := s.router.Group("/webhooks")
	webhooks.POST("/incoming/:credential", limitLegacyRequestBody(), s.handleIncomingWebhook)
	if s.config.LiveKit.IsConfigured() {
		webhooks.POST("/livekit", s.handleLiveKitWebhook)
	}
	registerTestWebhookEndpoints(webhooks, s)
}

type incomingWebhookPayload struct {
	Text         string `json:"text"`
	Body         string `json:"body"`
	Channel      string `json:"channel"`
	RoomID       string `json:"room_id"`
	CreateThread bool   `json:"create_thread"`
}

func (s *HTTPServer) handleIncomingWebhook(c *gin.Context) {
	bot, err := s.core.ValidateBotIncomingWebhookCredential(c.Request.Context(), c.Param("credential"))
	if err != nil {
		if errors.Is(err, core.ErrAuthTokenNotFound) {
			incomingWebhookError(c, http.StatusUnauthorized, "invalid_token")
			return
		}
		incomingWebhookError(c, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}

	var payload incomingWebhookPayload
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&payload); err != nil {
		incomingWebhookError(c, http.StatusBadRequest, "invalid_payload")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		incomingWebhookError(c, http.StatusBadRequest, "invalid_payload")
		return
	}
	body, ok := matchingIncomingWebhookValue(payload.Text, payload.Body)
	if !ok || strings.TrimSpace(body) == "" {
		incomingWebhookError(c, http.StatusBadRequest, "invalid_payload")
		return
	}

	targets := append([]string(nil), c.Request.URL.Query()["room_id"]...)
	targets = append(targets, payload.RoomID, payload.Channel)
	roomID, ok := matchingIncomingWebhookValue(targets...)
	if !ok || roomID == "" {
		incomingWebhookError(c, http.StatusBadRequest, "invalid_payload")
		return
	}

	_, err = s.core.Messages().PostMessage(c.Request.Context(), core.MessagePostInput{
		ActorID: bot.GetId(), RoomID: roomID, Body: body, CreateThread: payload.CreateThread,
	})
	if err != nil {
		switch {
		case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrNotRoomMember), errors.Is(err, core.ErrPermissionDenied):
			incomingWebhookError(c, http.StatusNotFound, "channel_not_found")
		case errors.Is(err, core.ErrSlowModeActive):
			incomingWebhookError(c, http.StatusTooManyRequests, "rate_limited")
		case errors.Is(err, core.ErrRoomArchived):
			incomingWebhookError(c, http.StatusConflict, "channel_is_archived")
		case errors.Is(err, core.ErrInvalidArgument), errors.Is(err, core.ErrMessageTooLong), errors.Is(err, core.ErrDMThreadsUnsupported), errors.Is(err, core.ErrRoomThreadingPolicy):
			incomingWebhookError(c, http.StatusBadRequest, "invalid_payload")
		default:
			incomingWebhookError(c, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("ok"))
}

func matchingIncomingWebhookValue(values ...string) (string, bool) {
	selected := ""
	for _, value := range values {
		if value == "" {
			continue
		}
		if selected != "" && value != selected {
			return "", false
		}
		selected = value
	}
	return selected, true
}

func incomingWebhookError(c *gin.Context, status int, code string) {
	c.Data(status, "text/plain; charset=utf-8", []byte(code))
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
		if core.IsCallMediaPublisher(event.Participant.Metadata) {
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
		if core.IsCallMediaPublisher(event.Participant.Metadata) {
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
