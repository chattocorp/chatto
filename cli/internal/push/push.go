// Package push provides Web Push notification functionality.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Sender sends Web Push notifications.
type Sender struct {
	config     config.PushConfig
	logger     *log.Logger
	httpClient webpush.HTTPClient
}

const (
	pushRecordSize                       uint32 = 2048
	maxPushProviderResponseBodyBytes            = 2048
	truncatedPushProviderResponseBodyMsg        = "…"
)

// NewSender creates a new push notification sender.
// Returns nil if push is not configured.
func NewSender(cfg config.PushConfig, logger *log.Logger) *Sender {
	if !cfg.IsConfigured() {
		return nil
	}
	return &Sender{
		config: cfg,
		logger: logger,
	}
}

// Payload represents the data sent in a push notification.
type Payload struct {
	Title          string `json:"title,omitempty"`
	Body           string `json:"body,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Badge          string `json:"badge,omitempty"`
	Tag            string `json:"tag,omitempty"`
	NotificationID string `json:"notificationId,omitempty"`
	URL            string `json:"url,omitempty"`
	// Action is used for special payloads like "dismiss" to close notifications on other devices
	Action string `json:"action,omitempty"`
}

// PayloadContext provides optional context for building push payloads.
type PayloadContext struct {
	// MessagePreview is a truncated preview of the message body
	MessagePreview string
	// RoomName is the name of the room (for mentions)
	RoomName string
}

// maxPreviewLength is the maximum length for message previews
const maxPreviewLength = 100

// truncatePreview truncates a message to maxPreviewLength with ellipsis
func truncatePreview(text string) string {
	if len(text) <= maxPreviewLength {
		return text
	}
	// Find a good break point (space) near the limit
	breakPoint := maxPreviewLength
	for i := maxPreviewLength - 1; i > maxPreviewLength-20 && i > 0; i-- {
		if text[i] == ' ' {
			breakPoint = i
			break
		}
	}
	return text[:breakPoint] + "…"
}

// SendResult contains the result of a push notification send attempt.
type SendResult struct {
	Endpoint string
	Success  bool
	Error    error
	// Gone indicates the subscription is no longer valid and should be deleted
	Gone bool
}

// Send sends a push notification to a single subscription.
func (s *Sender) Send(ctx context.Context, sub *corev1.PushSubscription, payload *Payload) *SendResult {
	result := &SendResult{
		Endpoint: sub.Endpoint,
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		result.Error = fmt.Errorf("failed to marshal payload: %w", err)
		return result
	}

	// Create webpush subscription from our proto
	subscription := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256Dh,
			Auth:   sub.Auth,
		},
	}

	// Send the push notification
	resp, err := webpush.SendNotification(payloadJSON, subscription, &webpush.Options{
		Subscriber:      normalizeVAPIDSubject(s.config.VAPIDSubject),
		VAPIDPublicKey:  s.config.VAPIDPublicKey,
		VAPIDPrivateKey: s.config.VAPIDPrivateKey,
		TTL:             86400, // 24 hours
		RecordSize:      pushRecordSize,
		HTTPClient:      s.httpClient,
	})
	if err != nil {
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	// Check response status
	switch resp.StatusCode {
	case 200, 201, 202:
		// Drain body to allow connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		result.Success = true
	case 404, 410:
		// 404 Not Found or 410 Gone - subscription is no longer valid
		body, readErr := readPushProviderResponseBody(resp.Body)
		result.Gone = true
		result.Error = pushServiceStatusError("subscription expired or invalid", resp.StatusCode, body, readErr)
	default:
		body, readErr := readPushProviderResponseBody(resp.Body)
		result.Error = pushServiceStatusError("push service returned status", resp.StatusCode, body, readErr)
	}

	return result
}

func normalizeVAPIDSubject(subject string) string {
	return strings.TrimPrefix(subject, "mailto:")
}

func readPushProviderResponseBody(body io.Reader) (string, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, io.LimitReader(body, maxPushProviderResponseBodyBytes+1))
	_, _ = io.Copy(io.Discard, body)
	if err != nil {
		return "", err
	}

	responseBody := buf.Bytes()
	truncated := false
	if len(responseBody) > maxPushProviderResponseBodyBytes {
		responseBody = responseBody[:maxPushProviderResponseBodyBytes]
		truncated = true
	}

	text := strings.TrimSpace(strings.ToValidUTF8(string(responseBody), ""))
	if truncated {
		text += truncatedPushProviderResponseBodyMsg
	}
	return text, nil
}

func pushServiceStatusError(prefix string, statusCode int, body string, readErr error) error {
	if readErr != nil {
		return fmt.Errorf("%s %d (failed to read response body: %w)", prefix, statusCode, readErr)
	}
	if body == "" {
		return fmt.Errorf("%s %d", prefix, statusCode)
	}
	return fmt.Errorf("%s %d: %s", prefix, statusCode, body)
}

// SendToMany sends a push notification to multiple subscriptions.
// Returns results for each subscription.
func (s *Sender) SendToMany(ctx context.Context, subscriptions []*corev1.PushSubscription, payload *Payload) []*SendResult {
	results := make([]*SendResult, len(subscriptions))
	for i, sub := range subscriptions {
		results[i] = s.Send(ctx, sub, payload)
	}
	return results
}

// BuildPayloadFromNotification creates a push payload from a notification.
// The baseURL is used to build navigation URLs (e.g., "https://chatto.example.com").
// The optional payloadCtx provides message preview and room name for richer notifications.
func BuildPayloadFromNotification(notif *corev1.Notification, actorDisplayName, baseURL string, payloadCtx *PayloadContext) *Payload {
	payload := &Payload{
		NotificationID: notif.Id,
		Icon:           baseURL + "/icons/icon-192.png",
		Badge:          baseURL + "/icons/icon-192.png", // Badge should be monochrome, but use same for now
	}

	// Get preview from context, truncate if needed
	preview := ""
	roomName := ""
	if payloadCtx != nil {
		preview = truncatePreview(payloadCtx.MessagePreview)
		roomName = payloadCtx.RoomName
	}

	// URL prefix for the home instance. Push notifications are always generated
	// by the server the user is connected to, so the instance segment is always "-".
	// Chat URLs go straight from instance segment to room ID — no spaceId, no /dm/
	// prefix (DMs are surfaced as rooms under the primary space).
	chatPrefix := baseURL + "/chat/-"

	switch n := notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		payload.Title = fmt.Sprintf("@%s sent you a new DM", actorDisplayName)
		payload.Body = preview
		payload.Tag = "dm-" + n.DmMessage.EventId
		payload.URL = chatPrefix + "/" + n.DmMessage.RoomId

	case *corev1.Notification_Mention:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s mentioned you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s mentioned you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = "mention-" + n.Mention.EventId
		payload.URL = chatPrefix + "/" + n.Mention.RoomId
		if n.Mention.EventId != "" {
			payload.URL += "?highlight=" + n.Mention.EventId
		}

	case *corev1.Notification_Reply:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s replied to you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s replied to you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = "reply-" + n.Reply.EventId
		if n.Reply.InThread != "" {
			// Thread reply: navigate to the thread (using thread root) and highlight the replied-to message
			payload.URL = chatPrefix + "/" + n.Reply.RoomId + "/" + n.Reply.InThread + "?highlight=" + n.Reply.InReplyToId
		} else {
			// Room-level reply: navigate to room and highlight the reply message
			payload.URL = chatPrefix + "/" + n.Reply.RoomId + "?highlight=" + n.Reply.EventId
		}

	default:
		payload.Title = "New notification"
		payload.Body = "You have a new notification"
	}

	return payload
}

// NotificationTag returns the push notification tag for a notification.
// Used for dismissing notifications on other devices.
// Tags use event IDs to uniquely identify each notification.
func NotificationTag(notif *corev1.Notification) string {
	switch n := notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		return "dm-" + n.DmMessage.EventId
	case *corev1.Notification_Mention:
		return "mention-" + n.Mention.EventId
	case *corev1.Notification_Reply:
		return "reply-" + n.Reply.EventId
	default:
		return ""
	}
}
