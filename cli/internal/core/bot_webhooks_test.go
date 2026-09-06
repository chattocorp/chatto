package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

func webhookTestBot(t *testing.T, c *ChattoCore) (string, string, string) {
	t.Helper()
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "webhook-owner", "Owner", "password123")
	require.NoError(t, err)
	bot, err := c.CreateBot(ctx, owner.GetId(), "outbound_bot", "Outbound")
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateAllow))
	room, _, err := c.FindOrCreateDM(ctx, owner.GetId(), []string{bot.User.GetId()})
	require.NoError(t, err)
	return owner.GetId(), bot.User.GetId(), room.GetId()
}
func waitWebhookOutcome(t *testing.T, c *ChattoCore, owner, bot, status string) *evtv1.Event {
	t.Helper()
	var result *BotOutboundWebhook
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var err error
		result, err = c.GetBotOutboundWebhook(ctx, owner, bot)
		return err == nil && result != nil && result.Latest != nil && result.Latest.GetBotWebhookDeliveryCompleted().GetStatus() == status
	}, 5*time.Second, 10*time.Millisecond)
	return result.Latest
}

// waitWebhookDeliveriesDrained waits for source handoff and all local delivery work.
func waitWebhookDeliveriesDrained(t *testing.T, c *ChattoCore, replicas ...*ChattoCore) {
	t.Helper()
	ctx := testContext(t)
	require.Eventually(t, func() bool {
		source, err := c.botWebhooks.sourceConsumer.Info(ctx)
		if err != nil || source.NumPending != 0 || source.NumAckPending != 0 {
			return false
		}
		for _, core := range append(replicas, c) {
			if core.botWebhooks.pending.Load() != 0 {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)
}
func requireNoWebhookOutcomes(t *testing.T, c *ChattoCore) {
	t.Helper()
	facts, _, err := c.EventPublisher.SubjectEvents(testContext(t), "evt.bot_webhook_delivery.>")
	require.NoError(t, err)
	require.Empty(t, facts, "success and skip must not append delivery facts to EVT")
	keys, err := c.storage.runtimeStateKV.Keys(testContext(t))
	if err != nil {
		require.ErrorIs(t, err, jetstream.ErrNoKeysFound)
		return
	}
	for _, key := range keys {
		require.NotContains(t, key, "bot_webhook", "webhooks must not use KV")
	}
}
func TestBotOutboundWebhookRetriesAndAcknowledgement(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 3, RetryDelay: config.Duration(50 * time.Millisecond), AllowPrivateNetworks: true}
	type receivedRequest struct {
		body    []byte
		headers http.Header
		at      time.Time
	}
	received := make(chan receivedRequest, 4)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- receivedRequest{body, r.Header.Clone(), time.Now()}
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	startCoreServices(t, c)
	owner, bot, room := webhookTestBot(t, c)
	ctx := testContext(t)
	metadata, secret, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "Bearer receiver-secret", true)
	require.NoError(t, err)
	source, err := c.PostMessage(ctx, KindDM, room, owner, "Hello @outbound_bot", nil, "", "", nil, false)
	require.NoError(t, err)
	waitWebhookDeliveriesDrained(t, c)
	require.Equal(t, int32(3), calls.Load())
	var previous time.Time
	id := botWebhookDeliveryID(bot, metadata.ID, source.GetId())
	for i := 0; i < 3; i++ {
		request := <-received
		var payload botWebhookPayload
		require.NoError(t, json.Unmarshal(request.body, &payload))
		require.Equal(t, []string{"direct_message", "mention"}, payload.Triggers)
		require.Equal(t, "Hello @outbound_bot", payload.Message.Body)
		require.Equal(t, id, payload.ID)
		require.Equal(t, "Bearer receiver-secret", request.headers.Get("Authorization"))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(request.headers.Get("Chatto-Webhook-Timestamp") + "."))
		mac.Write(request.body)
		require.Equal(t, "v1="+hex.EncodeToString(mac.Sum(nil)), request.headers.Get("Chatto-Webhook-Signature"))
		if i > 0 {
			require.GreaterOrEqual(t, request.at.Sub(previous), time.Duration(50*(1<<(i-1)))*time.Millisecond)
		}
		previous = request.at
	}
	// A repeated source handoff can send again. The receiver gets the same ID.
	data, err := proto.Marshal(source)
	require.NoError(t, err)
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RoomAggregate(room).Subject("message_posted"))
	require.NoError(t, err)
	require.NoError(t, c.botWebhooks.materialize(ctx, events.DurableDelivery{Data: data, StreamSequence: seq}))
	waitWebhookDeliveriesDrained(t, c)
	require.Equal(t, int32(4), calls.Load())
	require.Equal(t, id, (<-received).headers.Get("Chatto-Webhook-Id"))
	requireNoWebhookOutcomes(t, c)
	stored, _, _ := c.botWebhooks.projection.Projection().get(bot)
	encoded, err := proto.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), server.URL)
	require.NotContains(t, string(encoded), secret)
}
func TestBotOutboundWebhookFailureAndAccessLoss(t *testing.T) {
	for _, test := range []struct {
		name   string
		revoke bool
	}{{"exhaustion", false}, {"revoked", true}} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := newTestCore(t)
			c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 2, RetryDelay: config.Duration(100 * time.Millisecond), AllowPrivateNetworks: true}
			var calls atomic.Int32
			first := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case first <- struct{}{}:
				default:
				}
				w.WriteHeader(503)
			}))
			defer server.Close()
			c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
			startCoreServices(t, c)
			owner, bot, room := webhookTestBot(t, c)
			ctx := testContext(t)
			_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
			require.NoError(t, err)
			_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
			require.NoError(t, err)
			select {
			case <-first:
			case <-ctx.Done():
				t.Fatal("no first attempt")
			}
			if test.revoke {
				require.NoError(t, c.SetUserPermissionState(ctx, owner, bot, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateNone))
				waitWebhookDeliveriesDrained(t, c)
				requireNoWebhookOutcomes(t, c)
				require.Equal(t, int32(1), calls.Load())
			} else {
				result := waitWebhookOutcome(t, c, owner, bot, "failed").GetBotWebhookDeliveryCompleted()
				require.Equal(t, uint32(2), result.GetAttempts())
				require.Equal(t, uint32(503), result.GetHttpStatus())
				waitWebhookDeliveriesDrained(t, c)
				require.Equal(t, int32(2), calls.Load())
			}
		})
	}
}
func TestBotOutboundWebhookManagerBoundary(t *testing.T) {
	c, _ := setupTestCore(t)
	owner, bot, _ := webhookTestBot(t, c)
	ctx := testContext(t)
	stranger, err := c.CreateUser(ctx, SystemActorID, "stranger", "Stranger", "password123")
	require.NoError(t, err)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, stranger.GetId(), bot, "https://example.com/hook", "", true)
	require.Error(t, err)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, "http://localhost/hook", "", true)
	require.ErrorIs(t, err, ErrInvalidArgument)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, "https://example.com/hook", "Bearer x\r\nX-Evil: y", true)
	require.ErrorIs(t, err, ErrInvalidArgument)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, "https://example.com/hook", "", true)
	require.NoError(t, err)
	_, err = c.GetBotOutboundWebhook(ctx, stranger.GetId(), bot)
	require.Error(t, err)
	_, err = c.GetBotOutboundWebhook(ctx, bot, bot)
	require.Error(t, err)
	require.NoError(t, c.DeleteBotOutboundWebhook(ctx, owner, bot))
	w, err := c.GetBotOutboundWebhook(ctx, owner, bot)
	require.NoError(t, err)
	require.Nil(t, w)
	require.NoError(t, c.DeleteBotOutboundWebhook(ctx, owner, bot))
}

func TestBotOutboundWebhookExpiryAndReplacement(t *testing.T) {
	for _, mode := range []string{"expiry", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := newTestCore(t)
			c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 5, RetryDelay: config.Duration(time.Second), Expiry: config.Duration(200 * time.Millisecond), AllowPrivateNetworks: true}
			if mode == "replacement" {
				c.config.BotWebhooks.Expiry = config.Duration(time.Hour)
				c.config.BotWebhooks.RetryDelay = config.Duration(200 * time.Millisecond)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
			defer server.Close()
			c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
			startCoreServices(t, c)
			owner, bot, room := webhookTestBot(t, c)
			ctx := testContext(t)
			_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
			require.NoError(t, err)
			_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
			require.NoError(t, err)
			require.Eventually(t, func() bool { return calls.Load() == 1 }, 3*time.Second, 10*time.Millisecond)
			if mode == "replacement" {
				_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL+"/replacement", "", true)
				require.NoError(t, err)
			}
			waitWebhookDeliveriesDrained(t, c)
			require.Equal(t, int32(1), calls.Load())
			if mode == "replacement" {
				requireNoWebhookOutcomes(t, c)
				return
			}
			facts, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.bot_webhook_delivery.>")
			require.NoError(t, err)
			require.Len(t, facts, 1)
			require.Equal(t, "expired", facts[0].GetBotWebhookDeliveryCompleted().GetReason())
		})
	}
}

func TestBotOutboundWebhookRestartDiscardsRetryState(t *testing.T) {
	c, nc := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 2, RetryDelay: config.Duration(time.Second), AllowPrivateNetworks: true}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	runCtx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(runCtx) }()
	require.NoError(t, c.WaitForBoot(testContext(t)))
	owner, bot, room := webhookTestBot(t, c)
	ctx := testContext(t)
	_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
	require.NoError(t, err)
	_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, err := c.botWebhooks.sourceConsumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0 && c.botWebhooks.pending.Load() == 1 && calls.Load() == 1
	}, 3*time.Second, 10*time.Millisecond)
	// EVT is already acknowledged while the delivery waits in memory. Shutdown
	// abandons its retry; a new process must not recover that accepted work.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("core did not stop")
	}
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	waitWebhookDeliveriesDrained(t, replica)
	require.Never(t, func() bool { return calls.Load() != 1 }, 1200*time.Millisecond, 10*time.Millisecond)
	requireNoWebhookOutcomes(t, replica)
}

func TestBotOutboundWebhookBackoff(t *testing.T) {
	job := &botWebhookDelivery{RetryDelay: 30 * time.Second}
	for i, want := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		require.Equal(t, want, webhookRetryDelay(job, uint64(i+1)))
	}
}

func TestBotOutboundWebhookRedirectDoesNotForwardSecrets(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 1, AllowPrivateNetworks: true}
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	startCoreServices(t, c)
	owner, bot, room := webhookTestBot(t, c)
	ctx := testContext(t)
	_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, redirect.URL, "Bearer secret", true)
	require.NoError(t, err)
	_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
	require.NoError(t, err)
	result := waitWebhookOutcome(t, c, owner, bot, "failed")
	require.Equal(t, uint32(307), result.GetBotWebhookDeliveryCompleted().GetHttpStatus())
	require.Zero(t, forwarded.Load())
}

func TestBotOutboundWebhookFanoutAcrossReplicas(t *testing.T) {
	c, nc := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 2, RetryDelay: config.Duration(10 * time.Millisecond), AllowPrivateNetworks: true}
	var good, bad atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good" {
			good.Add(1)
			w.WriteHeader(204)
		} else {
			bad.Add(1)
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	startCoreServices(t, c)
	replica, err := NewChattoCore(testContext(t), nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	owner, first, _ := webhookTestBot(t, c)
	ctx := testContext(t)
	second, err := c.CreateBot(ctx, owner, "second_bot", "Second")
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner, second.User.GetId(), PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateAllow))
	room, _, err := c.FindOrCreateDM(ctx, owner, []string{first, second.User.GetId()})
	require.NoError(t, err)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, first, server.URL+"/good", "", true)
	require.NoError(t, err)
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, second.User.GetId(), server.URL+"/bad", "", true)
	require.NoError(t, err)
	_, err = c.PostMessage(ctx, KindDM, room.GetId(), owner, "Activate both bots", nil, "", "", nil, false)
	require.NoError(t, err)
	waitWebhookDeliveriesDrained(t, c, replica)
	waitWebhookOutcome(t, c, owner, second.User.GetId(), "failed")
	require.Equal(t, int32(1), good.Load())
	require.Equal(t, int32(2), bad.Load())
}

func TestBotOutboundWebhookSourceExpiryRecordsFailure(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{AllowPrivateNetworks: true}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	c.botWebhooks.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	startCoreServices(t, c)
	owner, bot, room := webhookTestBot(t, c)
	ctx := testContext(t)
	_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
	require.NoError(t, err)
	_, err = c.PostMessage(ctx, KindDM, room, owner, "Expired before materialization", nil, "", "", nil, false)
	require.NoError(t, err)
	result := waitWebhookOutcome(t, c, owner, bot, "failed").GetBotWebhookDeliveryCompleted()
	require.Equal(t, "expired", result.GetReason())
	require.Equal(t, uint32(1), result.GetAttempts())
	require.Zero(t, calls.Load())
}

func TestBotOutboundWebhookChannelSelection(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{AllowPrivateNetworks: true}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	startCoreServices(t, c)
	owner, bot, _ := webhookTestBot(t, c)
	ctx := testContext(t)
	room, err := c.CreateRoom(ctx, owner, KindChannel, "", "webhooks", "")
	require.NoError(t, err)
	_, err = c.AddMember(ctx, owner, KindChannel, room.GetId(), bot)
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner, bot, PermissionTargetScope{Kind: MatrixScopeRoom, ID: room.GetId()}, PermMessageReadInteractions, PermissionStateAllow))
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
	require.NoError(t, err)
	for _, body := range []string{"Ordinary channel message", "@all broadcast", "Hello @outbound_bot"} {
		_, err = c.PostMessage(ctx, KindChannel, room.GetId(), owner, body, nil, "", "", nil, false)
		require.NoError(t, err)
	}
	_, err = c.PostMessage(ctx, KindChannel, room.GetId(), bot, "Self mention @outbound_bot", nil, "", "", nil, false)
	require.NoError(t, err)
	waitWebhookDeliveriesDrained(t, c)
	consumer, err := c.storage.serverEvtStream.Consumer(ctx, botWebhookSourceConsumer)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), calls.Load())
	requireNoWebhookOutcomes(t, c)
}

func TestBotOutboundWebhookConcurrentReplacementReturnsOwnSecret(t *testing.T) {
	c, _ := setupTestCore(t)
	owner, bot, _ := webhookTestBot(t, c)
	ctx := testContext(t)
	type response struct {
		webhook *BotOutboundWebhook
		secret  string
		err     error
	}
	responses := make(chan response, 8)
	for i := 0; i < 8; i++ {
		go func() {
			w, s, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, "https://example.com/hook", "", false)
			responses <- response{w, s, err}
		}()
	}
	var succeeded []response
	for i := 0; i < 8; i++ {
		r := <-responses
		if errors.Is(r.err, events.ErrConflict) {
			continue
		}
		require.NoError(t, r.err)
		succeeded = append(succeeded, r)
	}
	require.GreaterOrEqual(t, len(succeeded), 2)
	records, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.user."+bot+".bot_outbound_webhook_configured")
	require.NoError(t, err)
	secrets := map[string]string{}
	for _, record := range records {
		creds, err := c.botWebhooks.credentials(ctx, record)
		require.NoError(t, err)
		secrets[record.GetBotOutboundWebhookConfigured().GetWebhookId()] = creds.SigningSecret
	}
	for _, r := range succeeded {
		if secrets[r.webhook.ID] != r.secret {
			t.Fatal("replacement paired another configuration with its signing secret")
		}
	}
}

func TestBotOutboundWebhookMembershipLossIsTerminal(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 2, RetryDelay: config.Duration(200 * time.Millisecond), AllowPrivateNetworks: true}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	startCoreServices(t, c)
	owner, bot, _ := webhookTestBot(t, c)
	ctx := testContext(t)
	room, err := c.CreateRoom(ctx, owner, KindChannel, "", "membership-test", "")
	require.NoError(t, err)
	_, err = c.AddMember(ctx, owner, KindChannel, room.GetId(), bot)
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner, bot, PermissionTargetScope{Kind: MatrixScopeRoom, ID: room.GetId()}, PermMessageReadInteractions, PermissionStateAllow))
	_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
	require.NoError(t, err)
	_, err = c.PostMessage(ctx, KindChannel, room.GetId(), owner, "Hello @outbound_bot", nil, "", "", nil, false)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second*3, time.Millisecond*10)
	require.NoError(t, c.LeaveRoom(ctx, bot, KindChannel, bot, room.GetId()))
	waitWebhookDeliveriesDrained(t, c)
	requireNoWebhookOutcomes(t, c)
	require.Equal(t, int32(1), calls.Load())
}

// A full handoff buffer blocks the source handler and remains cancellable.
func TestBotOutboundWebhookBoundedHandoff(t *testing.T) {
	m := &botWebhookModel{deliveries: make(chan *botWebhookDelivery, botWebhookBuffer)}
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	for range botWebhookBuffer {
		require.NoError(t, m.enqueue(ctx, &botWebhookDelivery{}))
	}
	done := make(chan error, 1)
	go func() { done <- m.enqueue(ctx, &botWebhookDelivery{}) }()
	require.Eventually(t, func() bool { return m.pending.Load() == botWebhookBuffer+1 }, time.Second, time.Millisecond)
	select {
	case <-done:
		t.Fatal("handoff bypassed full buffer")
	default:
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, int64(botWebhookBuffer), m.pending.Load())
}

func TestBotOutboundWebhookFailureIsIdempotent(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	delivery := &botWebhookDelivery{DeliveryID: "terminal-test", BotUserID: "bot", WebhookID: "endpoint", SourceEventID: "source"}
	require.NoError(t, c.botWebhooks.fail(ctx, delivery, 2, "http_error", 503))
	require.NoError(t, c.botWebhooks.fail(ctx, delivery, 2, "http_error", 503))
	require.NoError(t, c.botWebhooks.deliver(ctx, delivery))
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, botWebhookAggregate(delivery.DeliveryID).AllEventsFilter())
	require.NoError(t, err)
	require.Len(t, facts, 1)
}

func TestBotOutboundWebhookPoolBoundsHTTPAndCancelsOnShutdown(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{AllowPrivateNetworks: true}
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
	ctx := testContext(t)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(runCtx) }()
	require.NoError(t, c.WaitForBoot(ctx))
	owner, bot, room := webhookTestBot(t, c)
	_, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
	require.NoError(t, err)
	for range botWebhookConcurrency + 1 {
		_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
		require.NoError(t, err)
	}
	// All source events can be acknowledged while HTTP requests remain blocked.
	require.Eventually(t, func() bool {
		info, err := c.botWebhooks.sourceConsumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0 && calls.Load() == botWebhookConcurrency
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, int64(botWebhookConcurrency+1), c.botWebhooks.pending.Load())
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("active webhook requests prevented shutdown")
	}
	require.Equal(t, int32(botWebhookConcurrency), calls.Load())
	requireNoWebhookOutcomes(t, c)
}
