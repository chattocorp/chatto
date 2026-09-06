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
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	botWebhookSourceConsumer = "chatto-bot-webhook-source-v1"
	botWebhookRequestTimeout = 10 * time.Second
	botWebhookConcurrency    = 8
	botWebhookBuffer         = 64
)

// botWebhookDelivery is process-local work. It contains references and policy,
// never message plaintext or endpoint credentials. Restart abandons this work.
type botWebhookDelivery struct {
	DeliveryID, BotUserID, WebhookID, SourceEventID, RoomID string
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
	deliveries     chan *botWebhookDelivery
	pending        atomic.Int64 // Accepted or blocked handoffs and active deliveries; process-local only.
	sourceConsumer jetstream.Consumer
	client         *http.Client
	now            func() time.Time
}

func newBotWebhookModel(c *ChattoCore, p events.ProjectionHandle[*botWebhookProjection]) *botWebhookModel {
	client := linkpreview.NewSSRFSafeClient(botWebhookRequestTimeout)
	if c.config.BotWebhooks.AllowPrivateNetworks {
		client = &http.Client{Timeout: botWebhookRequestTimeout, Transport: &http.Transport{Proxy: nil, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: botWebhookRequestTimeout}}
	}
	// Never forward credentials or a message body through an endpoint redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &botWebhookModel{core: c, projection: p, client: client, now: time.Now, deliveries: make(chan *botWebhookDelivery, botWebhookBuffer)}
}
func (m *botWebhookModel) initialize(ctx context.Context) error {
	var err error
	m.sourceConsumer, err = evtstream.CreateEffectConsumer(ctx, m.core.storage.serverEvtStream, evtstream.EffectConsumerConfig{
		Name: botWebhookSourceConsumer, Description: "Best-effort outbound webhook handoff",
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
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return worker.Run(ctx) })
	for range botWebhookConcurrency {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case delivery := <-m.deliveries:
					m.runDelivery(ctx, delivery)
					m.pending.Add(-1)
				}
			}
		})
	}
	return g.Wait()
}

// enqueue applies backpressure without spawning goroutines. Once this handoff
// succeeds, the EVT source may be acknowledged even though HTTP has not started.
func (m *botWebhookModel) enqueue(ctx context.Context, delivery *botWebhookDelivery) error {
	m.pending.Add(1)
	select {
	case <-ctx.Done():
		m.pending.Add(-1)
		return ctx.Err()
	case m.deliveries <- delivery:
		return nil
	}
}

// runDelivery owns bounded retries and cancellable waits within one worker slot.
// Shutdown discards unfinished work. Failure recording is also best effort.
func (m *botWebhookModel) runDelivery(ctx context.Context, delivery *botWebhookDelivery) {
	for attempt := uint64(1); ctx.Err() == nil; attempt++ {
		err := m.deliver(ctx, delivery)
		if err == nil || ctx.Err() != nil {
			return
		}
		reason, status := "internal_error", 0
		var failure *botWebhookAttemptFailure
		if errors.As(err, &failure) {
			reason, status = failure.reason, failure.status
		}
		expired := !m.now().Before(delivery.ExpiresAt)
		if expired {
			reason = "expired"
		}
		if expired || attempt >= uint64(delivery.MaxAttempts) || reason == "invalid_request" {
			if err := m.fail(ctx, delivery, uint32(attempt), reason, status); err != nil && ctx.Err() == nil {
				m.core.logger.Warn("Could not record outbound webhook failure", "delivery_id", delivery.DeliveryID)
			}
			return
		}
		timer := time.NewTimer(max(0, min(webhookRetryDelay(delivery, attempt), delivery.ExpiresAt.Sub(m.now()))))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func botWebhookAggregate(id string) evtstream.Aggregate {
	return evtstream.Aggregate{Type: "bot_webhook_delivery", ID: id}
}
func botWebhookDeliveryID(botID, webhookID, eventID string) string {
	sum := sha256.Sum256([]byte(botID + "\x00" + webhookID + "\x00" + eventID))
	return hex.EncodeToString(sum[:])
}

// materialize hands destinations to the bounded process-local pool before
// acknowledging EVT. Partial handoff or lost source acknowledgement can repeat
// requests; stable delivery IDs let receivers detect duplicates.
func (m *botWebhookModel) materialize(ctx context.Context, d events.DurableDelivery) error {
	e, err := decodeDurableCoreDelivery(d)
	if err != nil {
		return err
	}
	message := e.GetMessagePosted()
	if message == nil {
		return nil
	}
	expiry := e.GetCreatedAt().AsTime().Add(m.core.config.BotWebhooks.ExpiryOrDefault())
	// Wait for endpoint state first. With no eligible endpoint, historical
	// replay needs no room reads. Expired eligible messages still create work
	// so the delivery worker records their terminal expiry instead of losing it.
	if err = m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	candidates := m.projection.Projection().activeBefore(d.StreamSequence)
	if len(candidates) == 0 {
		return nil
	}
	if err = m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	kind, err := m.core.FindRoomKind(ctx, message.GetRoomId())
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, botID := range candidates {
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
		cfg, seq, _ := m.projection.Projection().get(botID)
		if cfg == nil || seq >= d.StreamSequence || !cfg.GetBotOutboundWebhookConfigured().GetEnabled() {
			continue
		}
		webhookID := cfg.GetBotOutboundWebhookConfigured().GetWebhookId()
		id := botWebhookDeliveryID(botID, webhookID, e.GetId())
		request := &botWebhookDelivery{DeliveryID: id, BotUserID: botID, WebhookID: webhookID, SourceEventID: e.GetId(), RoomID: message.GetRoomId(), Triggers: triggers, OccurredAt: e.GetCreatedAt().AsTime(), ExpiresAt: expiry, MaxAttempts: uint32(m.core.config.BotWebhooks.MaxAttemptsOrDefault()), RetryDelay: m.core.config.BotWebhooks.RetryDelayOrDefault()}
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
	terminal, err := m.core.EventPublisher.LastSubjectSeq(ctx, botWebhookAggregate(r.DeliveryID).Subject("bot_webhook_delivery_completed"))
	if err != nil {
		return err
	}
	if terminal != 0 {
		return nil
	}
	if !m.now().Before(r.ExpiresAt) {
		return &botWebhookAttemptFailure{reason: "expired"}
	}
	if err = m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	cfg, _, _ := m.projection.Projection().get(r.BotUserID)
	if cfg == nil || !cfg.GetBotOutboundWebhookConfigured().GetEnabled() || cfg.GetBotOutboundWebhookConfigured().GetWebhookId() != r.WebhookID {
		return nil
	}
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
	if err = validateBotWebhookURL(creds.URL, m.core.config.BotWebhooks.AllowPrivateNetworks); err != nil {
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
	current, _, _ := m.projection.Projection().get(r.BotUserID)
	if current == nil || !current.GetBotOutboundWebhookConfigured().GetEnabled() || current.GetBotOutboundWebhookConfigured().GetWebhookId() != r.WebhookID {
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
func webhookRetryDelay(r *botWebhookDelivery, attempt uint64) time.Duration {
	delay := r.RetryDelay
	for i := uint64(1); i < attempt && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Minute)
}

// fail records terminal failures only. OCC prevents duplicate failure facts
// when the EVT source is redelivered after a partial or repeated handoff.
func (m *botWebhookModel) fail(ctx context.Context, r *botWebhookDelivery, attempts uint32, reason string, httpStatus int) error {
	x := &evtv1.BotWebhookDeliveryCompletedEvent{DeliveryId: r.DeliveryID, BotUserId: r.BotUserID, WebhookId: r.WebhookID, SourceEventId: r.SourceEventID, Status: "failed", Reason: reason, Attempts: attempts, HttpStatus: uint32(httpStatus)}
	event := newEvent("", &evtv1.Event{Event: &evtv1.Event_BotWebhookDeliveryCompleted{BotWebhookDeliveryCompleted: x}})
	agg := botWebhookAggregate(r.DeliveryID)
	subject := agg.SubjectFor(event)
	seqs, err := m.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: event, HasOCC: true, ExpectedSeq: 0, FilterSubject: agg.AllEventsFilter()}})
	if errors.Is(err, events.ErrConflict) {
		return nil
	}
	if err != nil {
		return err
	}
	return m.projection.Projector().WaitFor(ctx, events.SubjectPosition(subject, seqs[0]))
}
