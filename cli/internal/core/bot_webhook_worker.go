package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"sync"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	logv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/log/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/nats-io/nats.go/jetstream"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	botWebhookSourceConsumer = "chatto-bot-webhook-source-v1"
	botWebhookRequestTimeout = 10 * time.Second
	botWebhookConcurrency    = 8
)

// botWebhookDelivery contains references and immutable policy, never message
// plaintext or endpoint credentials. JOBS persists its protobuf equivalent.
type botWebhookDelivery struct {
	DeliveryID, BotUserID, WebhookID, SourceEventID, RoomID string
	ConfigurationSequence                                   uint64 // Reject retries from a previous enabled period.
	Triggers                                                []string
	OccurredAt, ExpiresAt                                   time.Time
	MaxAttempts                                             uint32
	RetryDelay                                              time.Duration
}

// botWebhookAttemptFailure contains only safe categories, never response bodies.
type botWebhookAttemptFailure struct {
	reason string
	status int
}

func (e *botWebhookAttemptFailure) Error() string { return e.reason }

type botWebhookModel struct {
	core           *ChattoCore
	projection     events.ProjectionHandle[*botWebhookProjection]
	sourceConsumer jetstream.Consumer
	client         *http.Client
	now            func() time.Time
	sourceSyncMu   sync.Mutex // Coalesce projection catch-up for a committed EVT prefix.
	sourceSyncSeq  uint64     // Protected by sourceSyncMu; never persisted.
}

func newBotWebhookModel(c *ChattoCore, p events.ProjectionHandle[*botWebhookProjection]) *botWebhookModel {
	client := linkpreview.NewSSRFSafeClientWithLocalhost(botWebhookRequestTimeout)
	// Never forward credentials or a message body through an endpoint redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &botWebhookModel{core: c, projection: p, client: client, now: time.Now}
}
func (m *botWebhookModel) initialize(ctx context.Context) error {
	var err error
	m.sourceConsumer, err = evtstream.CreateEffectConsumer(ctx, m.core.storage.serverEvtStream, evtstream.EffectConsumerConfig{
		Name: botWebhookSourceConsumer, Description: "Durable outbound webhook job handoff",
		FilterSubjects: []string{evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted)},
		AckWait:        time.Minute, MaxAckPending: botWebhookConcurrency, DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	return err
}

func (m *botWebhookModel) run(ctx context.Context) error {
	if err := m.core.WaitForBoot(ctx); err != nil {
		return err
	}
	defer m.client.CloseIdleConnections()
	worker, err := evtstream.NewEffectWorker(m.sourceConsumer, m.materialize, evtstream.EffectWorkerOptions{
		MaxConcurrent: botWebhookConcurrency, RetryDelay: 5 * time.Second, AckTimeout: 5 * time.Second,
		HeartbeatInterval: 15 * time.Second, Logger: m.core.logger.WithPrefix("BotWebhookSource"),
	})
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}

// enqueue confirms durable acceptance before the EVT source is acknowledged.
func (m *botWebhookModel) enqueue(ctx context.Context, delivery *botWebhookDelivery) error {
	return m.core.enqueueWebhookJob(ctx, delivery)
}

func botWebhookDeliveryID(botID, webhookID, eventID string) string {
	sum := sha256.Sum256([]byte(botID + "\x00" + webhookID + "\x00" + eventID))
	return hex.EncodeToString(sum[:])
}

// syncSourceEndpoints catches endpoint state up through the source message.
// Capture the EVT tail before the filtered projection barrier. All source
// messages in that prefix can then share this barrier, without four JetStream
// reads per message during a burst or replay. Delivery still checks current
// endpoint state before HTTP; this watermark is only a handoff optimization.
func (m *botWebhookModel) syncSourceEndpoints(ctx context.Context, sourceSeq uint64) error {
	m.sourceSyncMu.Lock()
	defer m.sourceSyncMu.Unlock()

	if sourceSeq <= m.sourceSyncSeq {
		return nil
	}
	tail, err := m.core.EventPublisher.LastSubjectSeq(ctx, evtstream.EventSubjectFilter())
	if err != nil {
		return err
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	m.sourceSyncSeq = tail
	return nil
}

// materialize persists each destination before acknowledging EVT. Stable job
// identities make partial fanout and lost source acknowledgement recoverable.
func (m *botWebhookModel) materialize(ctx context.Context, d events.DurableDelivery) error {
	e, err := decodeDurableCoreDelivery(d)
	if err != nil {
		return err
	}
	message := e.GetMessagePosted()
	if message == nil || message.GetHistoricalImport() {
		return nil
	}
	expiry := e.GetCreatedAt().AsTime().Add(m.core.config.BotWebhooks.ExpiryOrDefault())
	// Wait for endpoint state first. With no eligible endpoint, historical
	// replay needs no room reads. Expired eligible messages still create work
	// so the delivery worker records their terminal expiry instead of losing it.
	if err = m.syncSourceEndpoints(ctx, d.StreamSequence); err != nil {
		return err
	}
	candidates := m.projection.Projection().activeBefore(d.StreamSequence)
	if len(candidates) == 0 {
		return nil
	}
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	kind, err := m.core.FindRoomKind(ctx, message.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		config := candidate.Configuration.GetBotOutboundWebhookConfigured()
		botID, webhookID := config.GetBotUserId(), config.GetWebhookId()
		if botID == e.GetActorId() {
			continue
		}
		triggers := []string{}
		if kind == KindDM {
			member, err := m.core.RoomMembershipExists(ctx, kind, botID, message.GetRoomId())
			if err != nil {
				return err
			}
			if member {
				triggers = append(triggers, "direct_message")
			}
		}
		for _, mention := range message.GetMentions() {
			if mention.GetUserId() == botID && mention.GetDirect() != nil {
				triggers = append(triggers, "mention")
				break
			}
		}
		if len(triggers) == 0 {
			continue
		}
		if _, _, err = m.core.requireMessageReader(ctx, botID, message.GetRoomId(), e.GetId()); err != nil {
			if webhookAccessLost(err) {
				continue
			}
			return err
		}
		endpoint := m.projection.Projection().get(botID, webhookID)
		if endpoint == nil || endpoint.Sequence >= d.StreamSequence || !endpoint.Enabled {
			continue
		}
		id := botWebhookDeliveryID(botID, webhookID, e.GetId())
		request := &botWebhookDelivery{ConfigurationSequence: endpoint.Sequence, DeliveryID: id, BotUserID: botID, WebhookID: webhookID, SourceEventID: e.GetId(), RoomID: message.GetRoomId(), Triggers: triggers, OccurredAt: e.GetCreatedAt().AsTime(), ExpiresAt: expiry, MaxAttempts: uint32(m.core.config.BotWebhooks.MaxAttemptsOrDefault()), RetryDelay: m.core.config.BotWebhooks.RetryDelayOrDefault()}
		if err = m.enqueue(ctx, request); err != nil {
			return err
		}
	}
	return nil
}
func webhookAccessLost(err error) bool {
	return errors.Is(err, ErrNotRoomMember) || errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrMessageNotFound) || errors.Is(err, ErrBotOwnerPermissionCeiling)
}

// botWebhookPayload is the fixed v1 JSON contract for both activation causes.
// Message content is the currently readable version at each attempt.
type botWebhookPayload struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Triggers     []string          `json:"triggers"`
	OccurredAt   time.Time         `json:"occurred_at"`
	BotID        string            `json:"bot_id"`
	RoomID       string            `json:"room_id"`
	ThreadRootID *string           `json:"thread_root_id"`
	Message      botWebhookMessage `json:"message"`
}
type botWebhookMessage struct {
	ID       string `json:"id"`
	AuthorID string `json:"author_id"`
	Body     string `json:"body"`
}

func (m *botWebhookModel) deliver(ctx context.Context, r *botWebhookDelivery) error {
	if !m.now().Before(r.ExpiresAt) {
		return &botWebhookAttemptFailure{reason: "expired"}
	}
	if err := m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	var err error
	endpoint := m.projection.Projection().get(r.BotUserID, r.WebhookID)
	if endpoint == nil || !endpoint.Enabled || endpoint.Sequence != r.ConfigurationSequence {
		return nil
	}
	cfg := endpoint.Configuration
	if _, err = m.core.GetUser(ctx, r.BotUserID); err != nil {
		if webhookAccessLost(err) {
			return nil
		}
		return err
	}
	creds, err := m.credentials(ctx, cfg)
	if err != nil {
		return err
	}
	if err = validateBotWebhookURL(creds.URL); err != nil {
		return nil
	}
	// Stable-input authorization includes current owner authority and membership.
	var message *MessageReadResult
	err = m.core.authorizeAtStableInputs(ctx, func() error {
		var err error
		message, err = m.core.roomTimelineReads.GetMessage(ctx, r.BotUserID, r.RoomID, r.SourceEventID)
		return err
	})
	if err != nil {
		if webhookAccessLost(err) {
			return nil
		}
		return err
	}
	body, err := m.core.GetFullMessageBody(ctx, r.SourceEventID)
	if err != nil {
		return err
	}
	if body == nil {
		return nil
	}
	if err = m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	current := m.projection.Projection().get(r.BotUserID, r.WebhookID)
	if current == nil || !current.Enabled || current.Sequence != r.ConfigurationSequence {
		return nil
	}
	err = m.core.authorizeAtStableInputs(ctx, func() error {
		_, _, err := m.core.requireMessageReader(ctx, r.BotUserID, r.RoomID, r.SourceEventID)
		return err
	})
	if err != nil {
		if webhookAccessLost(err) {
			return nil
		}
		return err
	}
	var thread *string
	if id := message.Event.GetMessagePosted().GetInThread(); id != "" {
		thread = &id
	}
	payload := botWebhookPayload{Version: 1, ID: r.DeliveryID, Type: "message.created", Triggers: r.Triggers, OccurredAt: r.OccurredAt, BotID: r.BotUserID, RoomID: r.RoomID, ThreadRootID: thread, Message: botWebhookMessage{ID: r.SourceEventID, AuthorID: message.Event.GetActorId(), Body: body.Body}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The deadline also bounds an in-flight HTTP request.
	sendCtx, cancel := context.WithDeadline(ctx, minTime(m.now().Add(botWebhookRequestTimeout), r.ExpiresAt))
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, creds.URL, bytes.NewReader(encoded))
	if err != nil {
		return &botWebhookAttemptFailure{reason: "invalid_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Chatto-Webhook/1")
	if creds.Authorization != "" {
		req.Header.Set("Authorization", creds.Authorization)
	}
	timestamp := strconv.FormatInt(m.now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(creds.SigningSecret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(encoded)
	req.Header.Set("Chatto-Webhook-Id", r.DeliveryID)
	req.Header.Set("Chatto-Webhook-Timestamp", timestamp)
	req.Header.Set("Chatto-Webhook-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))
	response, sendErr := m.client.Do(req)
	status := 0
	if response != nil {
		status = response.StatusCode
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		response.Body.Close()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if sendErr == nil && status >= 200 && status < 300 {
		return nil
	}
	reason := "http_error"
	if sendErr != nil {
		reason = "transport_error"
	}
	return &botWebhookAttemptFailure{reason: reason, status: status}
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// fail records a diagnostic outcome with a bounded storage attempt. Failure to
// record it must never cause another HTTP attempt or an unbounded retry loop.
func (m *botWebhookModel) fail(ctx context.Context, r *botWebhookDelivery, attempts uint32, reason string, httpStatus int) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return m.core.appendOperationalLog(ctx, &logv1.Entry{
		Id: r.DeliveryID, RecordedAt: timestamppb.New(m.now()), Severity: logv1.Severity_SEVERITY_ERROR,
		Payload: &logv1.Entry_BotWebhookDeliveryFailed{BotWebhookDeliveryFailed: &logv1.BotWebhookDeliveryFailed{
			BotUserId: r.BotUserID, WebhookId: r.WebhookID, SourceEventId: r.SourceEventID,
			Attempts: attempts, HttpStatus: uint32(httpStatus), Reason: botWebhookLogReason(reason),
		}},
	})
}
