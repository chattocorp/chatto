// Package push provides Web Push notification functionality.
package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/charmbracelet/log"
	"golang.org/x/net/idna"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/pushendpoint"
)

// Sender sends Web Push notifications.
type Sender struct {
	config           config.PushConfig
	logger           *log.Logger
	httpClient       webpush.HTTPClient
	validateEndpoint func(string) error
	requestSlots     chan struct{}
	now              func() time.Time
}

const (
	// Keep ordinary encrypted requests compact, but grow for a longer installed
	// client route without reaching push providers' common 4 KiB body ceiling.
	pushRecordSize    uint32 = 2048
	maxPushRecordSize uint32 = 3990
	// aes128gcm framing uses 86 header bytes, a delimiter, and a 16-byte tag.
	pushRecordOverhead        = 103
	declarativeWebPushValue   = 8030
	pushRequestTimeout        = 10 * time.Second
	maxConcurrentPushRequests = 16
)

// NewSender creates a new push notification sender.
// Returns nil if push is not configured.
func NewSender(cfg config.PushConfig, logger *log.Logger) *Sender {
	if !cfg.IsConfigured() {
		return nil
	}
	return &Sender{
		config:           cfg,
		logger:           logger,
		httpClient:       pushendpoint.NewHTTPClient(pushRequestTimeout),
		validateEndpoint: pushendpoint.Validate,
		requestSlots:     make(chan struct{}, maxConcurrentPushRequests),
		now:              time.Now,
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
	// TTLSeconds overrides the provider retention horizon. Notification alerts
	// set this to their remaining immutable delivery lifetime; other push types
	// retain the normal 24-hour default.
	TTLSeconds int `json:"-"`
	// DeliveryDeadline is the immutable absolute provider-retention boundary for
	// time-sensitive alerts. Sender calculates the remaining TTL only after it
	// acquires a request slot so local contention cannot extend that boundary.
	DeliveryDeadline time.Time `json:"-"`
	// Action is empty for regular user-visible notifications. Control pushes set
	// it to a command such as "dismiss" and do not display a new notification.
	Action string `json:"action,omitempty"`
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
		Action         string                   `json:"action,omitempty"`
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
		Action:         p.Action,
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
	return p.Action == "" && p.Title != "" && p.URL != ""
}

func (p Payload) isUserVisibleNotification() bool {
	return p.Action == ""
}

// deliveryUrgency keeps visible notifications prompt on sleeping mobile
// devices without using high-priority delivery for silent control pushes.
func (p Payload) deliveryUrgency() webpush.Urgency {
	if p.isUserVisibleNotification() {
		return webpush.UrgencyHigh
	}
	return webpush.UrgencyNormal
}

func (p Payload) deliveryTTL(now time.Time) (int, bool) {
	if !p.DeliveryDeadline.IsZero() {
		remaining := p.DeliveryDeadline.Sub(now)
		if remaining <= 0 {
			return 0, false
		}
		// Truncation is intentional: the provider must never retain the payload
		// beyond the absolute deadline. Zero means immediate delivery only.
		return int(remaining / time.Second), true
	}
	if p.TTLSeconds > 0 {
		return p.TTLSeconds, true
	}
	return 24 * 60 * 60, true
}

// PayloadContext provides optional context for building push payloads.
type PayloadContext struct {
	// MessagePreview is a truncated preview of the message body
	MessagePreview string
	// RoomName is the name of the room (for mentions)
	RoomName string
}

// maxPreviewLength is the maximum number of Unicode code points in a message
// preview, including the ellipsis added when truncating.
const maxPreviewLength = 100

// truncatePreview truncates a message to maxPreviewLength Unicode code points,
// preferring a nearby whitespace boundary and including the ellipsis in the
// limit. It never slices through a UTF-8 encoding.
func truncatePreview(text string) string {
	if utf8.RuneCountInString(text) <= maxPreviewLength {
		return text
	}
	runes := []rune(text)
	contentLimit := maxPreviewLength - 1
	breakPoint := contentLimit
	for i := contentLimit - 1; i > contentLimit-20 && i > 0; i-- {
		if unicode.IsSpace(runes[i]) {
			breakPoint = i
			break
		}
	}
	return string(runes[:breakPoint]) + "…"
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
		Endpoint: sub.GetEndpoint(),
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validateEndpoint(sub.GetEndpoint()); err != nil {
		result.Error = errors.New("push delivery failed: invalid endpoint")
		return result
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
	ttl, deliverable := payload.deliveryTTL(s.now())
	if !deliverable {
		result.Error = context.DeadlineExceeded
		return result
	}

	// Marshal payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		result.Error = fmt.Errorf("failed to marshal payload: %w", err)
		return result
	}
	recordSize, err := recordSizeForPayload(len(payloadJSON))
	if err != nil {
		result.Error = err
		return result
	}

	// Create webpush subscription from our proto
	subscription := &webpush.Subscription{
		Endpoint: sub.GetEndpoint(),
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
		TTL:             ttl,
		Urgency:         payload.deliveryUrgency(),
		RecordSize:      recordSize,
		HTTPClient:      s.httpClient,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			result.Error = context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			result.Error = context.DeadlineExceeded
		default:
			result.Error = errors.New("push delivery request failed")
		}
		return result
	}
	defer resp.Body.Close()

	// Check response status
	switch resp.StatusCode {
	case 200, 201, 202:
		drainPushProviderResponseBody(resp.Body)
		result.Success = true
	case 404, 410:
		// 404 Not Found or 410 Gone - subscription is no longer valid
		drainPushProviderResponseBody(resp.Body)
		result.Gone = true
		result.Error = pushServiceStatusError("subscription expired or invalid", resp.StatusCode)
	default:
		drainPushProviderResponseBody(resp.Body)
		result.Error = pushServiceStatusError("push service returned status", resp.StatusCode)
	}

	return result
}

func recordSizeForPayload(payloadLength int) (uint32, error) {
	required := payloadLength + pushRecordOverhead
	if required > int(maxPushRecordSize) {
		return 0, errors.New("push delivery failed: payload too large")
	}
	if required <= int(pushRecordSize) {
		return pushRecordSize, nil
	}
	return uint32(required), nil
}

func normalizeVAPIDSubject(subject string) string {
	return strings.TrimPrefix(subject, "mailto:")
}

func drainPushProviderResponseBody(body io.Reader) {
	// Provider bodies are never trusted or surfaced. A small bounded drain keeps
	// normal connections reusable without letting an endpoint stream arbitrary
	// data until the request timeout.
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4096))
}

func pushServiceStatusError(prefix string, statusCode int) error {
	return fmt.Errorf("%s %d", prefix, statusCode)
}

// EndpointLogID returns a stable, opaque identifier for a push endpoint.
func EndpointLogID(endpoint string) string {
	hash := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(hash[:8])
}

// SendToMany sends a push notification to multiple subscriptions.
// Returns results for each subscription.
func (s *Sender) SendToMany(ctx context.Context, subscriptions []*corev1.PushSubscription, payload *Payload) []*SendResult {
	return s.SendToManyMapped(ctx, subscriptions, func(*corev1.PushSubscription) *Payload {
		return payload
	})
}

// SendToManyMapped sends a destination-specific payload to each subscription.
// It is used when click routes differ between installed web clients.
func (s *Sender) SendToManyMapped(
	ctx context.Context,
	subscriptions []*corev1.PushSubscription,
	payloadFor func(*corev1.PushSubscription) *Payload,
) []*SendResult {
	if len(subscriptions) > pushendpoint.MaxSubscriptionsPerUser {
		subscriptions = subscriptions[:pushendpoint.MaxSubscriptionsPerUser]
	}
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
				results[i] = s.Send(ctx, subscriptions[i], payloadFor(subscriptions[i]))
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
	segments := []string{}
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
	return buildPayloadFromOccurrence(
		occurrence,
		actorDisplayName,
		baseURL,
		buildAppURL(baseURL, []string{"chat", "-"}, "", ""),
		payloadCtx,
	)
}

// BuildPayloadFromOccurrenceForSubscription creates a payload whose click
// target opens the server in the web client that owns this subscription.
func BuildPayloadFromOccurrenceForSubscription(
	occurrence *corev1.NotificationOccurrence,
	actorDisplayName, serverBaseURL string,
	subscription *corev1.PushSubscription,
	payloadCtx *PayloadContext,
) *Payload {
	return buildPayloadFromOccurrence(
		occurrence,
		actorDisplayName,
		serverBaseURL,
		NavigationBaseURL(subscription, serverBaseURL),
		payloadCtx,
	)
}

// NavigationBaseURL reconstructs the client route for a subscription. Records
// without a usable client host fall back to this server's bundled app route.
func NavigationBaseURL(subscription *corev1.PushSubscription, serverBaseURL string) string {
	legacyURL := buildAppURL(serverBaseURL, []string{"chat", "-"}, "", "")
	if subscription == nil || subscription.ClientHost == "" {
		return legacyURL
	}

	serverURL, err := url.Parse(serverBaseURL)
	if err != nil || serverURL.Scheme == "" || serverURL.Hostname() == "" {
		return legacyURL
	}
	clientURL, err := url.Parse(serverURL.Scheme + "://" + subscription.ClientHost)
	if err != nil || clientURL.Host != subscription.ClientHost || clientURL.Hostname() == "" {
		return legacyURL
	}
	clientHostname, err := canonicalHostname(clientURL.Hostname())
	if err != nil {
		return legacyURL
	}
	clientURL.Host = hostnameWithOptionalPort(clientHostname, clientURL.Port())

	if sameOriginHost(clientURL, serverURL) {
		return buildAppURL(clientURL.String(), []string{"chat", "-"}, "", "")
	}

	clientURL.Scheme = "https"
	if isLoopbackHostname(clientHostname) {
		clientURL.Scheme = "http"
	}
	serverRouteHostname, err := browserRouteHostname(serverURL.Hostname())
	if err != nil {
		return legacyURL
	}
	return buildAppURL(clientURL.String(), []string{"chat", serverRouteHostname}, "", "")
}

func sameOriginHost(clientURL, serverURL *url.URL) bool {
	clientHostname, clientErr := canonicalHostname(clientURL.Hostname())
	serverHostname, serverErr := canonicalHostname(serverURL.Hostname())
	if clientErr != nil || serverErr != nil || clientHostname != serverHostname {
		return false
	}
	return effectivePort(clientURL) == effectivePort(serverURL)
}

func canonicalHostname(hostname string) (string, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		return strings.ToLower(ip.String()), nil
	}
	value, err := idna.Lookup.ToASCII(strings.ToLower(hostname))
	if err != nil {
		return "", err
	}
	return strings.ToLower(value), nil
}

func browserRouteHostname(hostname string) (string, error) {
	value, err := canonicalHostname(hostname)
	if err != nil {
		return "", err
	}
	if strings.Contains(value, ":") {
		return "[" + value + "]", nil
	}
	return value, nil
}

func hostnameWithOptionalPort(hostname, port string) string {
	if port != "" {
		return net.JoinHostPort(hostname, port)
	}
	if strings.Contains(hostname, ":") {
		return "[" + hostname + "]"
	}
	return hostname
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func isLoopbackHostname(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func buildPayloadFromOccurrence(
	occurrence *corev1.NotificationOccurrence,
	actorDisplayName, serverBaseURL, navigationBaseURL string,
	payloadCtx *PayloadContext,
) *Payload {
	payload := &Payload{
		NotificationID: occurrence.GetId(),
		Icon:           buildAppURL(serverBaseURL, []string{"icons", "icon-192.png"}, "", ""),
		Badge:          buildAppURL(serverBaseURL, []string{"icons", "icon-192.png"}, "", ""), // Badge should be monochrome, but use same for now
	}

	// Get preview from context, truncate if needed
	preview := ""
	roomName := ""
	if payloadCtx != nil {
		preview = truncatePreview(payloadCtx.MessagePreview)
		roomName = payloadCtx.RoomName
	}

	target := occurrenceMessageReference(occurrence)
	if target == nil {
		payload.Title = "New notification"
		payload.Body = "You have a new notification"
		return payload
	}

	switch {
	case occurrence.GetSignal().GetDirectMessageReceived() != nil:
		payload.Title = fmt.Sprintf("@%s sent you a new DM", actorDisplayName)
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(navigationBaseURL, target.GetRoomId(), "", "")

	case occurrenceHasMentionReason(occurrence):
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s mentioned you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s mentioned you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(navigationBaseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())

	case occurrence.GetSignal().GetReactionReceived() != nil:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s reacted to your message in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s reacted to your message", actorDisplayName)
		}
		payload.Body = preview
		if emoji := occurrence.GetSignal().GetReactionReceived().GetEmoji(); emoji != "" {
			payload.Body = ":" + emoji + ":"
			if preview != "" {
				payload.Body += " · " + preview
			}
		}
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(navigationBaseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())

	case occurrence.GetSignal().GetReplyReceived() != nil:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s replied to you in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s replied to you", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(navigationBaseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())

	default:
		if roomName != "" {
			payload.Title = fmt.Sprintf("@%s posted in #%s", actorDisplayName, roomName)
		} else {
			payload.Title = fmt.Sprintf("@%s posted a message", actorDisplayName)
		}
		payload.Body = preview
		payload.Tag = OccurrenceTag(occurrence)
		payload.URL = buildNotificationURL(navigationBaseURL, target.GetRoomId(), target.GetThreadRootEventId(), target.GetEventId())
	}

	return payload
}

// OccurrenceTag returns the stable native-notification tag for an occurrence.
func OccurrenceTag(occurrence *corev1.NotificationOccurrence) string {
	target := occurrenceMessageReference(occurrence)
	if target == nil {
		return ""
	}
	eventID := target.GetEventId()
	switch {
	case occurrence.GetSignal().GetDirectMessageReceived() != nil:
		return "dm-" + eventID
	case occurrenceHasMentionReason(occurrence):
		return "mention-" + eventID
	case occurrence.GetSignal().GetReplyReceived() != nil:
		return "reply-" + eventID
	case occurrence.GetSignal().GetReactionReceived() != nil:
		return "reaction-" + eventID
	case target != nil:
		return "room-message-" + eventID
	default:
		return ""
	}
}

func occurrenceMessageReference(occurrence *corev1.NotificationOccurrence) *corev1.NotificationMessageReference {
	if occurrence == nil || occurrence.GetSignal() == nil {
		return nil
	}
	switch payload := occurrence.GetSignal().GetKind().(type) {
	case *corev1.NotificationSignal_DirectMessageReceived:
		return payload.DirectMessageReceived.GetMessage()
	case *corev1.NotificationSignal_DirectMentionReceived:
		return payload.DirectMentionReceived.GetMessage()
	case *corev1.NotificationSignal_ReplyReceived:
		return payload.ReplyReceived.GetMessage()
	case *corev1.NotificationSignal_RoleMentionReceived:
		return payload.RoleMentionReceived.GetMessage()
	case *corev1.NotificationSignal_HereMentionReceived:
		return payload.HereMentionReceived.GetMessage()
	case *corev1.NotificationSignal_AllMentionReceived:
		return payload.AllMentionReceived.GetMessage()
	case *corev1.NotificationSignal_FollowedThreadActivity:
		return payload.FollowedThreadActivity.GetMessage()
	case *corev1.NotificationSignal_FollowedRoomActivity:
		return payload.FollowedRoomActivity.GetMessage()
	case *corev1.NotificationSignal_ReactionReceived:
		return payload.ReactionReceived.GetMessage()
	default:
		return nil
	}
}

func occurrenceHasMentionReason(occurrence *corev1.NotificationOccurrence) bool {
	if occurrence == nil || occurrence.GetSignal() == nil {
		return false
	}
	return occurrence.GetSignal().GetDirectMentionReceived() != nil ||
		occurrence.GetSignal().GetRoleMentionReceived() != nil ||
		occurrence.GetSignal().GetHereMentionReceived() != nil ||
		occurrence.GetSignal().GetAllMentionReceived() != nil
}
