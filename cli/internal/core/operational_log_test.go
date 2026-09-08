package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
	logv1 "hmans.de/chatto/internal/pb/chatto/core/log/v1"
)

func appendTestLog(t *testing.T, c *ChattoCore, bot, webhook, id string) {
	t.Helper()
	require.NoError(t, c.appendOperationalLog(testContext(t), &logv1.Entry{
		Id: id, RecordedAt: timestamppb.Now(), Severity: logv1.Severity_SEVERITY_ERROR,
		Payload: &logv1.Entry_BotWebhookDeliveryFailed{BotWebhookDeliveryFailed: &logv1.BotWebhookDeliveryFailed{
			BotUserId: bot, WebhookId: webhook, SourceEventId: "source", Reason: "http_error", Attempts: 2, HttpStatus: 503,
		}},
	}))
}

func TestLogPaginationScopeAndRetention(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	owner, bot, _ := webhookTestBot(t, c)
	endpoint, _, err := c.CreateBotOutboundWebhook(ctx, owner, bot, "Logs", "https://example.com", "", true)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		appendTestLog(t, c, bot, endpoint.ID, fmt.Sprintf("entry%d", i))
	}
	// A late replica cold-replays configuration and reads the same retained log.
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	page, err := replica.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, "")
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	require.Equal(t, "entry0", page.Entries[0].GetId())
	require.NotEmpty(t, page.NextCursor)
	originalCursor := page.NextCursor
	// A fixed pagination tail excludes later concurrent appends.
	appendTestLog(t, c, bot, endpoint.ID, "later")
	next, err := c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, page.NextCursor)
	require.NoError(t, err)
	require.Equal(t, "entry2", next.Entries[0].GetId())
	last, err := c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, next.NextCursor)
	require.NoError(t, err)
	require.Len(t, last.Entries, 1)
	require.Empty(t, last.NextCursor)
	other, _, err := c.CreateBotOutboundWebhook(ctx, owner, bot, "Other", "https://example.com/other", "", true)
	require.NoError(t, err)
	_, err = c.ListBotWebhookFailures(ctx, owner, bot, other.ID, 2, page.NextCursor)
	require.Error(t, err)
	_, err = c.ListBotWebhookFailures(ctx, bot, bot, endpoint.ID, 2, "")
	require.Error(t, err)
	_, err = c.ListBotWebhookFailures(ctx, owner, bot, "*", 2, "")
	require.Error(t, err)
	_, err = c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, page.NextCursor+"tampered")
	require.Error(t, err)
	// Expiry is enforced by JetStream, without a local cleanup worker or index.
	info, err := c.storage.logStream.Info(ctx)
	require.NoError(t, err)
	cfg := info.Config
	cfg.MaxAge = 200 * time.Millisecond
	cfg.Duplicates = 200 * time.Millisecond
	_, err = c.js.UpdateStream(ctx, cfg)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		p, err := c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, "")
		return err == nil && len(p.Entries) == 0
	}, 3*time.Second, 10*time.Millisecond)
	item, err := c.GetBotOutboundWebhook(ctx, owner, bot, endpoint.ID)
	require.NoError(t, err)
	require.Nil(t, item.Latest)
	// A cursor through removed messages ends cleanly.
	page, err = c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, page.NextCursor)
	require.NoError(t, err)
	require.Empty(t, page.Entries)
	// Once expired, the same delivery ID can be recorded again.
	appendTestLog(t, c, bot, endpoint.ID, "entry0")
	latest, err := c.latestOperationalLog(ctx, botWebhookLogFilter(bot, endpoint.ID))
	require.NoError(t, err)
	require.NotNil(t, latest)
	// An old cursor must not address a newly created stream with reused sequences.
	require.NoError(t, c.js.DeleteStream(ctx, "LOG"))
	_, err = c.js.CreateStream(ctx, cfg)
	require.NoError(t, err)
	_, err = c.ListBotWebhookFailures(ctx, owner, bot, endpoint.ID, 2, originalCursor)
	require.Error(t, err)
}

func TestLogConcurrentDuplicateAndUnavailableStorage(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	r := &botWebhookDelivery{DeliveryID: "same", BotUserID: "bot", WebhookID: "endpoint", SourceEventID: "source"}
	var group errgroup.Group
	for range 12 {
		group.Go(func() error { return c.botWebhooks.fail(ctx, r, 2, "http_error", 503) })
	}
	require.NoError(t, group.Wait())
	info, err := c.storage.logStream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), info.State.Msgs)
	require.NoError(t, c.js.DeleteStream(ctx, "LOG"))
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	require.Error(t, c.botWebhooks.fail(bounded, r, 2, "http_error", 503))
	// A missing diagnostic log must not prevent expiry from stopping HTTP.
	r.ExpiresAt = time.Now().Add(-time.Hour)
	err = c.botWebhooks.deliver(ctx, r)
	var failure *botWebhookAttemptFailure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "expired", failure.reason)
}

func TestLogUnavailableDoesNotBreakWebhookCommandsOrRepeatHTTP(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, bot, room := webhookTestBot(t, c)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
	}))
	defer server.Close()
	require.NoError(t, c.js.DeleteStream(ctx, "LOG"))
	// A successful command must not turn into an error due to missing diagnostics.
	endpoint, _, err := c.CreateBotOutboundWebhook(ctx, owner, bot, "Unavailable log", strings.Replace(server.URL, "127.0.0.1", "localhost", 1), "", true)
	require.NoError(t, err)
	require.NotNil(t, endpoint)
	c.config.BotWebhooks.MaxAttempts = 1
	_, err = c.PostMessage(ctx, KindDM, room, owner, "Test", nil, "", "", nil, false)
	require.NoError(t, err)
	waitWebhookDeliveriesDrained(t, c)
	require.Equal(t, int32(1), calls.Load(), "failed diagnostic recording must not retry HTTP")
}
