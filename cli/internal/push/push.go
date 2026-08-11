// Package push provides Web Push notification functionality.
package push

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Sender sends Web Push notifications.
type Sender struct {
	config       config.PushConfig
	logger       *log.Logger
	httpClient   webpush.HTTPClient
	requestSlots chan struct{}
}

const (
	pushRecordSize                       uint32 = 2048
	maxPushProviderResponseBodyBytes            = 2048
	truncatedPushProviderResponseBodyMsg        = "…"
	declarativeWebPushValue                     = 8030
	pushRequestTimeout                          = 10 * time.Second
	maxConcurrentPushRequests                   = 16
)

// NewSender creates a new push notification sender.
// Returns nil if push is not configured.
func NewSender(cfg config.PushConfig, logger *log.Logger) *Sender {
	if !cfg.IsConfigured() {
		return nil
	}
	return &Sender{
		config:       cfg,
		logger:       logger,
		httpClient:   &http.Client{Timeout: pushRequestTimeout},
		requestSlots: make(chan struct{}, maxConcurrentPushRequests),
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
	AppBadge       string `json:"-"`
}

type declarativeNotification struct {
	Title    string                       `json:"title"`
	Body     string                       `json:"body,omitempty"`
	Navigate string                       `json:"navigate"`
	Tag      string                       `json:"tag,omitempty"`
	Icon     string                       `json:"icon,omitempty"`
	Badge    string                       `json:"badge,omitempty"`
	AppBadge string                       `json:"app_badge,omitempty"`
	Data     *declarativeNotificationData `json:"data,omitempty"`
}

type declarativeNotificationData struct {
	NotificationID string `json:"notificationId,omitempty"`
	URL            string `json:"url,omitempty"`
}

func (p Payload) MarshalJSON() ([]byte, error) {
	type payloadJSON struct {
		Title          string                   `json:"title,omitempty"`
		Body           string                   `json:"body,omitempty"`
		Icon           string                   `json:"icon,omitempty"`
		Badge          string                   `json:"badge,omitempty"`
		Tag            string                   `json:"tag,omitempty"`
		NotificationID string                   `json:"notificationId,omitempty"`
		URL            string                   `json:"url,omitempty"`
		WebPush        int                      `json:"web_push,omitempty"`
		Mutable        bool                     `json:"mutable,omitempty"`
		AppBadge       string                   `json:"app_badge,omitempty"`
		Notification   *declarativeNotification `json:"notification,omitempty"`
	}

	out := payloadJSON{
		Title:          p.Title,
		Body:           p.Body,
		Icon:           p.Icon,
		Badge:          p.Badge,
		Tag:            p.Tag,
		NotificationID: p.NotificationID,
		URL:            p.URL,
		AppBadge:       p.AppBadge,
	}
	if p.declarativeNotificationEligible() {
		out.WebPush = declarativeWebPushValue
		out.Mutable = true
		out.Notification = &declarativeNotification{
			Title:    p.Title,
			Body:     p.Body,
			Navigate: p.URL,
			Tag:      p.Tag,
			Icon:     p.Icon,
			Badge:    p.Badge,
			AppBadge: p.AppBadge,
			Data: &declarativeNotificationData{
				NotificationID: p.NotificationID,
				URL:            p.URL,
			},
		}
	}
	return json.Marshal(out)
}

func (p Payload) declarativeNotificationEligible() bool {
	return p.Title != "" && p.URL != ""
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
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, pushRequestTimeout)
	defer cancel()

	select {
	case s.requestSlots <- struct{}{}:
		defer func() { <-s.requestSlots }()
	case <-requestCtx.Done():
		result.Error = requestCtx.Err()
		return result
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
	resp, err := webpush.SendNotificationWithContext(requestCtx, payloadJSON, subscription, &webpush.Options{
		Subscriber:      normalizeVAPIDSubject(s.config.VAPIDSubject),
		VAPIDPublicKey:  s.config.VAPIDPublicKey,
		VAPIDPrivateKey: s.config.VAPIDPrivateKey,
		TTL:             86400, // 24 hours
		Urgency:         webpush.UrgencyHigh,
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

// EndpointLogID returns a stable, opaque identifier for a push endpoint.
func EndpointLogID(endpoint string) string {
	hash := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(hash[:8])
}

// SendToMany sends a push notification to multiple subscriptions.
// Returns results for each subscription.
func (s *Sender) SendToMany(ctx context.Context, subscriptions []*corev1.PushSubscription, payload *Payload) []*SendResult {
	results := make([]*SendResult, len(subscriptions))
	if len(subscriptions) == 0 {
		return results
	}

	workerCount := min(len(subscriptions), maxConcurrentPushRequests)
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for i := range jobs {
				results[i] = s.Send(ctx, subscriptions[i], payload)
			}
		}()
	}
	for i := range subscriptions {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	return results
}

func buildAppURL(baseURL string, segments []string, queryKey, queryValue string) string {
	raw, err := url.JoinPath(baseURL, segments...)
	if err != nil {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if queryKey != "" && queryValue != "" {
		query := u.Query()
		query.Set(queryKey, queryValue)
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func buildNotificationURL(baseURL, roomID, threadRootID, highlightEventID string) string {
	segments := []string{"chat", "-"}
	if roomID != "" {
		segments = append(segments, roomID)
	}
	if threadRootID != "" {
		segments = append(segments, threadRootID)
	}
	return buildAppURL(baseURL, segments, "highlight", highlightEventID)
}

// BuildPayloadFromOccurrence creates a push payload from a notification occurrence.
// The baseURL is used to build navigation URLs (e.g., "https://chatto.example.com").
// The optional payloadCtx provides message preview and room name for richer notifications.
func BuildPayloadFromOccurrence(occurrence *corev1.NotificationOccurrence, actorDisplayName, baseURL string, payloadCtx *PayloadContext) *Payload {
	payload := &Payload{
		NotificationID: occurrence.GetId(),
		Icon:           buildAppURL(baseURL, []string{"icons", "icon-192.png"}, "", ""),
		Badge:          buildAppURL(baseURL, []string{"icons", "icon-192.png"}, "", ""), // Badge should be monochrome, but use same for now
	}

	// Get preview from context, truncate if needed
	preview := ""
	roomName := ""
	if payloadCtx != nil {
		preview = truncatePreview(payloadCtx.MessagePreview)
		roomName = payloadCtx.RoomName
	}

	target := occurrence.GetTarget()
	if target == nil {
		payload.Title = "New notification"
		payload.Body = "You have a new notification"
		return payload
	}

	switch {
	case occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE):
		payload.Title = fmt.Sprintf("@%s sent you a new DM", actorDisplayName)
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(baseURL, target.GetRoomId(), "", "")

	case occurrenceHasMentionReason(occurrence):
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s mentioned you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s mentioned you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(baseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())

	case occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REPLY):
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s replied to you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s replied to you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(baseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())

	default:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s posted in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s posted a message", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(baseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())
	}

	return payload
}

// OccurrenceTag returns the stable native-notification tag for an occurrence.
func OccurrenceTag(occurrence *corev1.NotificationOccurrence) string {
	eventID := occurrence.GetTarget().GetEventId()
	switch {
	case occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MESSAGE):
		return "dm-" + eventID
	case occurrenceHasMentionReason(occurrence):
		return "mention-" + eventID
	case occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_REPLY):
		return "reply-" + eventID
	case occurrence.GetTarget() != nil:
		return "room-message-" + eventID
	default:
		return ""
	}
}

func occurrenceHasReason(occurrence *corev1.NotificationOccurrence, reason corev1.NotificationReason) bool {
	for _, match := range occurrence.GetReasons() {
		if match.GetReason() == reason {
			return true
		}
	}
	return false
}

func occurrenceHasMentionReason(occurrence *corev1.NotificationOccurrence) bool {
	return occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION) ||
		occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_ROLE_MENTION) ||
		occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_HERE) ||
		occurrenceHasReason(occurrence, corev1.NotificationReason_NOTIFICATION_REASON_ALL)
}
