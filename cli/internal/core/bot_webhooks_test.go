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

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
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
func TestBotOutboundWebhookRetriesAndTerminalOutcome(t *testing.T) {
	c, _ := newTestCore(t)
	c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 3, RetryDelay: config.Duration(10 * time.Millisecond), AllowPrivateNetworks: true}
	var calls atomic.Int32
	received := make(chan []byte, 4)
	signatures := make(chan http.Header, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		signatures <- r.Header.Clone()
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
	require.NotEmpty(t, metadata.ID)
	_, err = c.PostMessage(ctx, KindDM, room, owner, "Hello @outbound_bot", nil, "", "", nil, false)
	require.NoError(t, err)
	outcome := waitWebhookOutcome(t, c, owner, bot, "delivered")
	require.Equal(t, uint32(3), outcome.GetBotWebhookDeliveryCompleted().GetAttempts())
	require.Equal(t, int32(3), calls.Load())
	id := ""
	for i := 0; i < 3; i++ {
		body := <-received
		headers := <-signatures
		var payload botWebhookPayload
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, []string{"direct_message", "mention"}, payload.Triggers)
		require.Equal(t, "Hello @outbound_bot", payload.Message.Body)
		if id == "" {
			id = payload.ID
		}
		require.Equal(t, id, payload.ID)
		require.Equal(t, "Bearer receiver-secret", headers.Get("Authorization"))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(headers.Get("Chatto-Webhook-Timestamp") + "."))
		mac.Write(body)
		require.Equal(t, "v1="+hex.EncodeToString(mac.Sum(nil)), headers.Get("Chatto-Webhook-Signature"))
	}
	requests, _, err := c.EventPublisher.SubjectEvents(ctx, botWebhookAggregate(id).Subject("bot_webhook_delivery_requested"))
	require.NoError(t, err)
	require.Len(t, requests, 1)
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, botWebhookAggregate(id).Subject("bot_webhook_delivery_requested"))
	require.NoError(t, err)
	data, err := proto.Marshal(requests[0])
	require.NoError(t, err)
	require.NoError(t, c.botWebhooks.deliver(ctx, events.DurableDelivery{Data: data, StreamSequence: seq}))
	require.Equal(t, int32(3), calls.Load(), "lost acknowledgement must not resend terminal work")
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, botWebhookAggregate(id).AllEventsFilter())
	require.NoError(t, err)
	require.Len(t, facts, 2)
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
			expected := "failed"
			if test.revoke {
				require.NoError(t, c.SetUserPermissionState(ctx, owner, bot, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateNone))
				expected = "skipped"
			}
			result := waitWebhookOutcome(t, c, owner, bot, expected).GetBotWebhookDeliveryCompleted()
			if test.revoke {
				require.Equal(t, "access_lost", result.GetReason())
				require.Equal(t, int32(1), calls.Load())
			} else {
				require.Equal(t, uint32(2), result.GetAttempts())
				require.Equal(t, uint32(503), result.GetHttpStatus())
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
	for _, mode := range []string{"expiry", "replacement", "restart"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := newTestCore(t)
			c.config.BotWebhooks = config.BotWebhooksConfig{MaxAttempts: 2, RetryDelay: config.Duration(time.Minute), AllowPrivateNetworks: true}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
			defer server.Close()
			c.botWebhooks.client = newBotWebhookModel(c, c.botWebhooks.projection).client
			startCoreServices(t, c)
			owner, bot, room := webhookTestBot(t, c)
			ctx := testContext(t)
			endpoint, _, err := c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL, "", true)
			require.NoError(t, err)
			source, err := c.PostMessage(ctx, KindDM, room, owner, "Hello", nil, "", "", nil, false)
			require.NoError(t, err)
			id := botWebhookDeliveryID(bot, endpoint.ID, source.GetId())
			require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second*3, time.Millisecond*10)
			subject := botWebhookAggregate(id).Subject("bot_webhook_delivery_requested")
			records, seq, err := c.EventPublisher.SubjectEvents(ctx, subject)
			require.NoError(t, err)
			require.Len(t, records, 1)
			data, err := proto.Marshal(records[0])
			require.NoError(t, err)
			// A fresh model uses only persisted request and attempt state. The live
			// worker remains delayed for one minute and cannot participate in this test.
			recovered := newBotWebhookModel(c, c.botWebhooks.projection)
			defer recovered.client.CloseIdleConnections()
			switch mode {
			case "expiry":
				recovered.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
			case "replacement":
				_, _, err = c.ReplaceBotOutboundWebhook(ctx, owner, bot, server.URL+"/replacement", "", true)
				require.NoError(t, err)
			case "restart":
				recovered.now = func() time.Time { return time.Now().Add(time.Hour) }
			}
			require.NoError(t, recovered.deliver(ctx, events.DurableDelivery{Subject: subject, Data: data, StreamSequence: seq}))
			outcomes, _, err := c.EventPublisher.SubjectEvents(ctx, botWebhookAggregate(id).Subject("bot_webhook_delivery_completed"))
			require.NoError(t, err)
			require.Len(t, outcomes, 1)
			outcome := outcomes[0].GetBotWebhookDeliveryCompleted()
			switch mode {
			case "expiry":
				require.Equal(t, "expired", outcome.GetReason())
				require.Equal(t, int32(1), calls.Load())
			case "replacement":
				require.Equal(t, "configuration_changed", outcome.GetReason())
				require.Equal(t, int32(1), calls.Load())
			case "restart":
				require.Equal(t, uint32(2), outcome.GetAttempts())
				require.Equal(t, int32(2), calls.Load())
			}
		})
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
	waitWebhookOutcome(t, c, owner, first, "delivered")
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
	require.Zero(t, result.GetAttempts())
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
	waitWebhookOutcome(t, c, owner, bot, "delivered")
	consumer, err := c.storage.serverEvtStream.Consumer(ctx, botWebhookSourceConsumer)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, err := consumer.Info(ctx)
		return err == nil && info.NumPending == 0 && info.NumAckPending == 0
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), calls.Load())
	requests, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.bot_webhook_delivery.*.bot_webhook_delivery_requested")
	require.NoError(t, err)
	require.Len(t, requests, 1)
	require.Equal(t, []string{"mention"}, requests[0].GetBotWebhookDeliveryRequested().GetTriggers())
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
	result := waitWebhookOutcome(t, c, owner, bot, "skipped").GetBotWebhookDeliveryCompleted()
	require.Equal(t, "access_lost", result.GetReason())
	require.Equal(t, int32(1), calls.Load())
}
