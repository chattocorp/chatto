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
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	webhookv1 "hmans.de/chatto/internal/pb/chatto/core/webhook/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	botWebhookStreamName       = "BOT_WEBHOOKS"
	botWebhookJobFilter        = "bot_webhook.>"
	botWebhookSourceConsumer   = "chatto-bot-webhook-source-v1"
	botWebhookDeliveryConsumer = "chatto-bot-webhook-delivery-v1"
	botWebhookRequestTimeout   = 10 * time.Second
	botWebhookConcurrency      = 8
)

var errBotWebhookRetry = errors.New("outbound webhook delivery pending")

type botWebhookModel struct {
	core             *ChattoCore
	projection       events.ProjectionHandle[*botWebhookProjection]
	queue            jetstream.Stream
	sourceConsumer   jetstream.Consumer
	deliveryConsumer jetstream.Consumer
	client           *http.Client
	now              func() time.Time
}

func newBotWebhookModel(c *ChattoCore, p events.ProjectionHandle[*botWebhookProjection]) *botWebhookModel {
	client := linkpreview.NewSSRFSafeClient(botWebhookRequestTimeout)
	if c.config.BotWebhooks.AllowPrivateNetworks {
		client = &http.Client{Timeout: botWebhookRequestTimeout, Transport: &http.Transport{Proxy: nil, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: botWebhookRequestTimeout}}
	}
	// Never forward credentials or a message body through an endpoint redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &botWebhookModel{core: c, projection: p, client: client, now: time.Now}
}
func (m *botWebhookModel) initialize(ctx context.Context) error {
	var err error
	// WorkQueue retention removes acknowledged jobs. No MaxAge: the worker
	// must record expiry failures, including after an extended server outage.
	m.queue, err = m.core.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: botWebhookStreamName, Subjects: []string{botWebhookJobFilter},
		Description: "Pending outbound bot webhook deliveries",
		Retention:   jetstream.WorkQueuePolicy, Storage: jetstream.FileStorage,
		Replicas: m.core.config.Replicas, Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return err
	}
	for _, item := range []struct {
		stream       jetstream.Stream
		name, filter string
		target       *jetstream.Consumer
	}{
		{m.core.storage.serverEvtStream, botWebhookSourceConsumer, evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted), &m.sourceConsumer},
		{m.queue, botWebhookDeliveryConsumer, botWebhookJobFilter, &m.deliveryConsumer},
	} {
		*item.target, err = evtstream.CreateEffectConsumer(ctx, item.stream, evtstream.EffectConsumerConfig{Name: item.name, Description: "Outbound bot webhook worker", FilterSubjects: []string{item.filter}, AckWait: time.Minute, MaxAckPending: botWebhookConcurrency, DeliverPolicy: jetstream.DeliverAllPolicy})
		if err != nil {
			return err
		}
	}
	return nil
}
func (m *botWebhookModel) run(ctx context.Context) error {
	if err := m.core.WaitForBoot(ctx); err != nil {
		return err
	}
	defer m.client.CloseIdleConnections()
	g, ctx := errgroup.WithContext(ctx)
	for _, item := range []struct {
		consumer jetstream.Consumer
		handler  events.DurableDeliveryHandler
	}{{m.sourceConsumer, m.materialize}, {m.deliveryConsumer, m.deliver}} {
		worker, err := evtstream.NewEffectWorker(item.consumer, item.handler, evtstream.EffectWorkerOptions{MaxConcurrent: botWebhookConcurrency, RetryDelay: time.Second * 5, AckTimeout: time.Second * 5, HeartbeatInterval: time.Second * 15, Logger: m.core.logger.WithPrefix("BotWebhookWorker")})
		if err != nil {
			return err
		}
		g.Go(func() error { return worker.Run(ctx) })
	}
	return g.Wait()
}
func botWebhookAggregate(id string) evtstream.Aggregate {
	return evtstream.Aggregate{Type: "bot_webhook_delivery", ID: id}
}
func botWebhookDeliveryID(botID, webhookID, eventID string) string {
	sum := sha256.Sum256([]byte(botID + "\x00" + webhookID + "\x00" + eventID))
	return hex.EncodeToString(sum[:])
}

// materialize confirms all destination-specific requests before acknowledging
// the message. Stable publish IDs deduplicate source retries within the
// queue's duplicate window. HTTP receivers must still tolerate repeats.
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
		request := &webhookv1.BotWebhookDelivery{DeliveryId: id, BotUserId: botID, WebhookId: webhookID, SourceEventId: e.GetId(), RoomId: message.GetRoomId(), Triggers: triggers, OccurredAt: e.GetCreatedAt(), ExpiresAt: timestamppb.New(expiry), MaxAttempts: uint32(m.core.config.BotWebhooks.MaxAttemptsOrDefault()), RetryDelayMs: m.core.config.BotWebhooks.RetryDelayOrDefault().Milliseconds()}
		data, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		if _, err = m.core.js.Publish(ctx, "bot_webhook."+id, data, jetstream.WithMsgID(id)); err != nil {
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

func (m *botWebhookModel) deliver(ctx context.Context, d events.DurableDelivery) error {
	r := &webhookv1.BotWebhookDelivery{}
	if err := proto.Unmarshal(d.Data, r); err != nil || r.GetDeliveryId() == "" || r.GetExpiresAt() == nil || r.GetMaxAttempts() == 0 {
		return events.TerminateDelivery("invalid outbound webhook request", nil)
	}
	// JetStream owns pending state and the delivery count. A worker delivery
	// counts even if authorization or infrastructure prevents an HTTP attempt.
	attempt := max(d.NumDelivered, 1)
	failed := func(reason string, status int) error {
		return m.fail(ctx, r, uint32(min(attempt, uint64(r.GetMaxAttempts()))), reason, status)
	}
	terminal, err := m.core.EventPublisher.LastSubjectSeq(ctx, botWebhookAggregate(r.GetDeliveryId()).Subject("bot_webhook_delivery_completed"))
	if err != nil {
		return err
	}
	if terminal != 0 {
		return nil
	}
	if !m.now().Before(r.GetExpiresAt().AsTime()) {
		return failed("expired", 0)
	}
	if attempt > uint64(r.GetMaxAttempts()) {
		return failed("attempt_limit", 0)
	}
	if err = m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	cfg, _, _ := m.projection.Projection().get(r.GetBotUserId())
	if cfg == nil || !cfg.GetBotOutboundWebhookConfigured().GetEnabled() || cfg.GetBotOutboundWebhookConfigured().GetWebhookId() != r.GetWebhookId() {
		return nil
	}
	if _, err = m.core.GetUser(ctx, r.GetBotUserId()); err != nil {
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
		message, err = m.core.roomTimelineReads.GetMessage(ctx, r.GetBotUserId(), r.GetRoomId(), r.GetSourceEventId())
		return err
	})
	if err != nil {
		if webhookAccessLost(err) {
			return nil
		}
		return err
	}
	body, err := m.core.GetFullMessageBody(ctx, r.GetSourceEventId())
	if err != nil {
		return err
	}
	if body == nil {
		return nil
	}
	if err = m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	current, _, _ := m.projection.Projection().get(r.GetBotUserId())
	if current == nil || !current.GetBotOutboundWebhookConfigured().GetEnabled() || current.GetBotOutboundWebhookConfigured().GetWebhookId() != r.GetWebhookId() {
		return nil
	}
	err = m.core.authorizeAtStableInputs(ctx, func() error {
		_, _, err := m.core.requireMessageReader(ctx, r.GetBotUserId(), r.GetRoomId(), r.GetSourceEventId())
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
	payload := botWebhookPayload{Version: 1, ID: r.GetDeliveryId(), Type: "message.created", Triggers: r.GetTriggers(), OccurredAt: r.GetOccurredAt().AsTime(), BotID: r.GetBotUserId(), RoomID: r.GetRoomId(), ThreadRootID: thread, Message: botWebhookMessage{ID: r.GetSourceEventId(), AuthorID: message.Event.GetActorId(), Body: body.Body}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The deadline also bounds an in-flight HTTP request.
	sendCtx, cancel := context.WithDeadline(ctx, minTime(m.now().Add(botWebhookRequestTimeout), r.GetExpiresAt().AsTime()))
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, creds.URL, bytes.NewReader(encoded))
	if err != nil {
		return failed("invalid_request", 0)
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
	req.Header.Set("Chatto-Webhook-Id", r.GetDeliveryId())
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
	if !m.now().Before(r.GetExpiresAt().AsTime()) {
		return failed("expired", status)
	}
	if attempt >= uint64(r.GetMaxAttempts()) {
		return failed(reason, status)
	}
	// A delayed NAK releases this worker. Wake by expiry even when the next
	// exponential interval would otherwise outlast the delivery lifetime.
	delay := min(webhookRetryDelay(r, attempt), r.GetExpiresAt().AsTime().Sub(m.now()))
	return events.RetryDeliveryAfter(errBotWebhookRetry, delay)
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func webhookRetryDelay(r *webhookv1.BotWebhookDelivery, attempt uint64) time.Duration {
	delay := time.Duration(r.GetRetryDelayMs()) * time.Millisecond
	for i := uint64(1); i < attempt && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Minute)
}

// fail records terminal failures only. OCC prevents duplicate failure facts
// when publishing succeeded but acknowledging the work queue did not.
func (m *botWebhookModel) fail(ctx context.Context, r *webhookv1.BotWebhookDelivery, attempts uint32, reason string, httpStatus int) error {
	x := &evtv1.BotWebhookDeliveryCompletedEvent{DeliveryId: r.GetDeliveryId(), BotUserId: r.GetBotUserId(), WebhookId: r.GetWebhookId(), SourceEventId: r.GetSourceEventId(), Status: "failed", Reason: reason, Attempts: attempts, HttpStatus: uint32(httpStatus)}
	event := newEvent("", &evtv1.Event{Event: &evtv1.Event_BotWebhookDeliveryCompleted{BotWebhookDeliveryCompleted: x}})
	agg := botWebhookAggregate(r.GetDeliveryId())
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
