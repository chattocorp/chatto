package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	runtimev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	botWebhookSourceConsumer   = "chatto-bot-webhook-source-v1"
	botWebhookDeliveryConsumer = "chatto-bot-webhook-delivery-v1"
	botWebhookRequestTimeout   = 10 * time.Second
	botWebhookConcurrency      = 8
)

var errBotWebhookRetry = errors.New("outbound webhook delivery pending")

type botWebhookModel struct {
	core             *ChattoCore
	projection       events.ProjectionHandle[*botWebhookProjection]
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
	for _, item := range []struct {
		name, filter string
		target       *jetstream.Consumer
	}{
		{botWebhookSourceConsumer, evtstream.RoomEventTypeFilter(evtstream.EventMessagePosted), &m.sourceConsumer},
		{botWebhookDeliveryConsumer, "evt.bot_webhook_delivery.*.bot_webhook_delivery_requested", &m.deliveryConsumer},
	} {
		*item.target, err = evtstream.CreateEffectConsumer(ctx, m.core.storage.serverEvtStream, evtstream.EffectConsumerConfig{Name: item.name, Description: "Outbound bot webhook worker", FilterSubjects: []string{item.filter}, AckWait: time.Minute, MaxAckPending: botWebhookConcurrency, DeliverPolicy: jetstream.DeliverAllPolicy})
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
// the message. OCC makes partial fan-out and source redelivery idempotent.
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
		request := &evtv1.BotWebhookDeliveryRequestedEvent{DeliveryId: id, BotUserId: botID, WebhookId: webhookID, SourceEventId: e.GetId(), RoomId: message.GetRoomId(), Triggers: triggers, OccurredAt: e.GetCreatedAt(), ExpiresAt: timestamppb.New(expiry), MaxAttempts: uint32(m.core.config.BotWebhooks.MaxAttemptsOrDefault()), RetryDelayMs: m.core.config.BotWebhooks.RetryDelayOrDefault().Milliseconds()}
		event := newEvent("", &evtv1.Event{Event: &evtv1.Event_BotWebhookDeliveryRequested{BotWebhookDeliveryRequested: request}})
		agg := botWebhookAggregate(id)
		_, err = m.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: agg.SubjectFor(event), Event: event, HasOCC: true, ExpectedSeq: 0, FilterSubject: agg.AllEventsFilter()}})
		if err != nil && !errors.Is(err, events.ErrConflict) {
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
	e, err := decodeDurableCoreDelivery(d)
	if err != nil {
		return err
	}
	r := e.GetBotWebhookDeliveryRequested()
	if r == nil || r.GetDeliveryId() == "" || r.GetExpiresAt() == nil || r.GetMaxAttempts() == 0 {
		return events.TerminateDelivery("invalid outbound webhook request", nil)
	}
	agg := botWebhookAggregate(r.GetDeliveryId())
	terminal, err := m.core.EventPublisher.LastSubjectSeq(ctx, agg.Subject("bot_webhook_delivery_completed"))
	if err != nil {
		return err
	}
	if terminal != 0 {
		return m.cleanupAttempt(ctx, r.GetDeliveryId())
	}
	key := "bot_webhook_attempt." + r.GetDeliveryId()
	state, revision, err := m.readAttempt(ctx, key)
	if err != nil {
		return err
	}
	complete := func(status, reason string, httpStatus int) error {
		return m.complete(ctx, r, d.StreamSequence, state.GetAttempts(), status, reason, httpStatus)
	}
	if !m.now().Before(r.GetExpiresAt().AsTime()) {
		return complete("failed", "expired", 0)
	}
	if err = m.core.WaitForProjectionsCurrent(ctx); err != nil {
		return err
	}
	cfg, _, _ := m.projection.Projection().get(r.GetBotUserId())
	if cfg == nil || !cfg.GetBotOutboundWebhookConfigured().GetEnabled() || cfg.GetBotOutboundWebhookConfigured().GetWebhookId() != r.GetWebhookId() {
		return complete("skipped", "configuration_changed", 0)
	}
	if _, err = m.core.GetUser(ctx, r.GetBotUserId()); err != nil {
		if webhookAccessLost(err) {
			return complete("skipped", "access_lost", 0)
		}
		return err
	}
	creds, err := m.credentials(ctx, cfg)
	if err != nil {
		return err
	}
	if err = validateBotWebhookURL(creds.URL, m.core.config.BotWebhooks.AllowPrivateNetworks); err != nil {
		return complete("skipped", "destination_policy", 0)
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
			return complete("skipped", "access_lost", 0)
		}
		return err
	}
	body, err := m.core.GetFullMessageBody(ctx, r.GetSourceEventId())
	if err != nil {
		return err
	}
	if body == nil {
		return complete("skipped", "message_unavailable", 0)
	}
	if next := time.UnixMilli(state.GetNextAttemptUnixMs()); m.now().Before(next) {
		return events.RetryDeliveryAfter(errBotWebhookRetry, time.Until(next))
	}
	if state.GetAttempts() >= r.GetMaxAttempts() {
		return complete("failed", "attempt_limit", 0)
	}
	state.Attempts++
	// Reserve before HTTP. A crash consumes this attempt because the outcome
	// cannot be known. CAS plus a request-timeout grace prevents normal overlap.
	state.NextAttemptUnixMs = m.now().Add(2*botWebhookRequestTimeout + webhookRetryDelay(r, state.Attempts)).UnixMilli()
	data, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	if revision == 0 {
		revision, err = m.core.storage.runtimeStateKV.Create(ctx, key, data)
	} else {
		revision, err = m.core.storage.runtimeStateKV.Update(ctx, key, data, revision)
	}
	if err != nil {
		return events.RetryDeliveryAfter(errBotWebhookRetry, time.Second)
	}
	// A stale worker can reserve after a terminal worker has removed KV state.
	// Recheck EVT after CAS, before sending, to close that cleanup race.
	terminal, err = m.core.EventPublisher.LastSubjectSeq(ctx, agg.Subject("bot_webhook_delivery_completed"))
	if err != nil {
		return err
	}
	if terminal != 0 {
		return m.cleanupAttempt(ctx, r.GetDeliveryId())
	}
	if err = m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	current, _, _ := m.projection.Projection().get(r.GetBotUserId())
	if current == nil || !current.GetBotOutboundWebhookConfigured().GetEnabled() || current.GetBotOutboundWebhookConfigured().GetWebhookId() != r.GetWebhookId() {
		return complete("skipped", "configuration_changed", 0)
	}
	err = m.core.authorizeAtStableInputs(ctx, func() error {
		_, _, err := m.core.requireMessageReader(ctx, r.GetBotUserId(), r.GetRoomId(), r.GetSourceEventId())
		return err
	})
	if err != nil {
		if webhookAccessLost(err) {
			return complete("skipped", "access_lost", 0)
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
		return complete("failed", "invalid_request", 0)
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
		return complete("delivered", "", status)
	}
	reason := "http_error"
	if sendErr != nil {
		reason = "transport_error"
	}
	if !m.now().Before(r.GetExpiresAt().AsTime()) {
		return complete("failed", "expired", status)
	}
	if state.GetAttempts() >= r.GetMaxAttempts() {
		return complete("failed", reason, status)
	}
	delay := webhookRetryDelay(r, state.Attempts)
	state.NextAttemptUnixMs = m.now().Add(delay).UnixMilli()
	data, err = proto.Marshal(state)
	if err != nil {
		return err
	}
	if _, err = m.core.storage.runtimeStateKV.Update(ctx, key, data, revision); err != nil {
		return err
	}
	return events.RetryDeliveryAfter(errBotWebhookRetry, delay)
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func webhookRetryDelay(r *evtv1.BotWebhookDeliveryRequestedEvent, attempt uint32) time.Duration {
	delay := time.Duration(r.GetRetryDelayMs()) * time.Millisecond
	for i := uint32(1); i < attempt && delay < 30*time.Minute; i++ {
		delay *= 2
	}
	return min(delay, 30*time.Minute)
}
func (m *botWebhookModel) readAttempt(ctx context.Context, key string) (*runtimev1.BotWebhookAttempt, uint64, error) {
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return &runtimev1.BotWebhookAttempt{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	state := &runtimev1.BotWebhookAttempt{}
	if err = proto.Unmarshal(entry.Value(), state); err != nil {
		return nil, 0, fmt.Errorf("decode outbound webhook attempt state")
	}
	return state, entry.Revision(), nil
}
func (m *botWebhookModel) complete(ctx context.Context, r *evtv1.BotWebhookDeliveryRequestedEvent, requestSeq uint64, attempts uint32, status, reason string, httpStatus int) error {
	x := &evtv1.BotWebhookDeliveryCompletedEvent{DeliveryId: r.GetDeliveryId(), BotUserId: r.GetBotUserId(), WebhookId: r.GetWebhookId(), SourceEventId: r.GetSourceEventId(), Status: status, Reason: reason, Attempts: attempts, HttpStatus: uint32(httpStatus)}
	event := newEvent("", &evtv1.Event{Event: &evtv1.Event_BotWebhookDeliveryCompleted{BotWebhookDeliveryCompleted: x}})
	agg := botWebhookAggregate(r.GetDeliveryId())
	subject := agg.SubjectFor(event)
	seqs, err := m.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: event, HasOCC: true, ExpectedSeq: requestSeq, FilterSubject: agg.AllEventsFilter()}})
	if errors.Is(err, events.ErrConflict) {
		seq, readErr := m.core.EventPublisher.LastSubjectSeq(ctx, subject)
		if readErr != nil {
			return readErr
		}
		if seq != 0 {
			return m.cleanupAttempt(ctx, r.GetDeliveryId())
		}
		return err
	}
	if err != nil {
		return err
	}
	if err = m.projection.Projector().WaitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
		return err
	}
	return m.cleanupAttempt(ctx, r.GetDeliveryId())
}
func (m *botWebhookModel) cleanupAttempt(ctx context.Context, id string) error {
	err := m.core.storage.runtimeStateKV.Delete(ctx, "bot_webhook_attempt."+id)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return nil
	}
	return err
}
