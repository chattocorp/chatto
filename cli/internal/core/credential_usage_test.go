package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestCredentialUsageRecorderKeepsMaximumTimestampAcrossConcurrentWriters(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	botID := NewUserID()
	credentialKey := incomingWebhookUsageKey(NewBotIncomingWebhookID())
	base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		usedAt := base.Add(time.Duration(i) * time.Second)
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil, nil)
			if err := recorder.writeMax(ctx, botID, credentialKey, usedAt); err != nil {
				t.Errorf("writeMax(%s): %v", usedAt, err)
			}
		}()
	}
	wg.Wait()

	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil, nil)
	lastUsed, available := recorder.LastUsed(ctx, botID)
	if !available || !lastUsed[credentialKey].Equal(base.Add(19*time.Second)) {
		t.Fatalf("LastUsed = %v, %v", lastUsed, available)
	}
}

func TestCredentialUsageRecorderCoalescesWritesButKeepsLocalObservation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil, nil)
	recorder.flushInterval = time.Hour
	botID := NewUserID()
	credentialKey := incomingWebhookUsageKey(NewBotIncomingWebhookID())
	first := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	recorder.Record(botID, credentialKey, first)
	recorder.flushDue(ctx, first)
	entry, err := c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID))
	if err != nil {
		t.Fatalf("Get first record: %v", err)
	}
	firstRevision := entry.Revision()

	recorder.Record(botID, credentialKey, second)
	recorder.flushDue(ctx, second)
	entry, err = c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID))
	if err != nil {
		t.Fatalf("Get coalesced record: %v", err)
	}
	if entry.Revision() != firstRevision {
		t.Fatalf("coalesced revision = %d, want %d", entry.Revision(), firstRevision)
	}
	lastUsed, available := recorder.LastUsed(ctx, botID)
	if !available || !lastUsed[credentialKey].Equal(second) {
		t.Fatalf("local LastUsed = %v, %v", lastUsed, available)
	}

	recorder.flushDue(ctx, first.Add(time.Hour))
	entry, err = c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID))
	if err != nil {
		t.Fatalf("Get flushed record: %v", err)
	}
	if entry.Revision() <= firstRevision {
		t.Fatalf("flushed revision = %d, want greater than %d", entry.Revision(), firstRevision)
	}

	recorder.Forget(ctx, botID, credentialKey)
	recorder.mu.RLock()
	defer recorder.mu.RUnlock()
	if len(recorder.pending) != 0 || len(recorder.observed) != 0 || len(recorder.lastFlush) != 0 {
		t.Fatalf("local state after Forget = pending %v, observed %v, lastFlush %v", recorder.pending, recorder.observed, recorder.lastFlush)
	}
}

func TestCredentialUsageRecorderRemovesWriteThatFinishesAfterForget(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	kv := &blockingCreateCredentialUsageKV{
		KeyValue: c.storage.runtimeStateKV,
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	recorder := newCredentialUsageRecorder(kv, nil, nil)
	botID := NewUserID()
	credentialKey := incomingWebhookUsageKey(NewBotIncomingWebhookID())
	usedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	recorder.Record(botID, credentialKey, usedAt)

	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		recorder.flushDue(ctx, usedAt)
	}()
	<-kv.started
	recorder.Forget(ctx, botID, credentialKey)
	close(kv.release)
	<-flushed

	if _, err := c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID)); !isRuntimeStateKeyAbsent(err) {
		t.Fatalf("persisted state after late flush = %v, want absent", err)
	}
	recorder.mu.RLock()
	defer recorder.mu.RUnlock()
	if len(recorder.pending) != 0 || len(recorder.observed) != 0 || len(recorder.lastFlush) != 0 {
		t.Fatalf("local state after Forget = pending %v, observed %v, lastFlush %v", recorder.pending, recorder.observed, recorder.lastFlush)
	}
}

func TestCredentialUsageRecorderRejectsInactiveObservation(t *testing.T) {
	c, _ := setupTestCore(t)
	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil, nil)
	recorder.recordIfActive(NewUserID(), incomingWebhookUsageKey(NewBotIncomingWebhookID()), time.Now(), func() bool {
		return false
	})

	recorder.mu.RLock()
	defer recorder.mu.RUnlock()
	if len(recorder.pending) != 0 || len(recorder.observed) != 0 {
		t.Fatalf("inactive observation was retained: pending %v, observed %v", recorder.pending, recorder.observed)
	}
}

func TestCredentialUsageRecorderSweepsCredentialRevokedOnAnotherReplica(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	active := true
	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil, func(string, string) bool { return active })
	recorder.sweepInterval = time.Minute
	botID := NewUserID()
	credentialKey := incomingWebhookUsageKey(NewBotIncomingWebhookID())
	usedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	recorder.Record(botID, credentialKey, usedAt)
	recorder.flushDue(ctx, usedAt)
	if _, err := c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID)); err != nil {
		t.Fatalf("Get initial usage: %v", err)
	}
	active = false
	recorder.flushDue(ctx, usedAt.Add(time.Minute))

	if _, err := c.storage.runtimeStateKV.Get(ctx, credentialUsageRuntimeStateKey(botID)); !isRuntimeStateKeyAbsent(err) {
		t.Fatalf("persisted state after remote revocation = %v, want absent", err)
	}
	recorder.mu.RLock()
	defer recorder.mu.RUnlock()
	if len(recorder.pending) != 0 || len(recorder.observed) != 0 || len(recorder.lastFlush) != 0 {
		t.Fatalf("local state after remote revocation = pending %v, observed %v, lastFlush %v", recorder.pending, recorder.observed, recorder.lastFlush)
	}
}

func TestBotCredentialUsageHydrationReportsUnavailableWithoutFailing(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "usage-owner", "Usage Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "usage_bot", "Usage Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	c.credentialUsage.kv = failingCredentialUsageKV{KeyValue: c.storage.runtimeStateKV, err: errors.New("unavailable")}

	issued, err := c.CreateBotIncomingWebhook(ctx, owner.GetId(), bot.User.GetId(), "Unavailable telemetry")
	if err != nil {
		t.Fatalf("CreateBotIncomingWebhook: %v", err)
	}
	if len(issued.Bot.IncomingWebhooks) != 1 || issued.Bot.IncomingWebhooks[0].LastUsedState != BotCredentialLastUsedNoUseRecorded {
		t.Fatalf("new incoming webhook usage = %+v", issued.Bot.IncomingWebhooks)
	}
	c.HydrateBotCredentialUsage(ctx, issued.Bot)
	if issued.Bot.APIKeys[0].LastUsedState != BotCredentialLastUsedUnavailable {
		t.Fatalf("API key usage = %+v", issued.Bot.APIKeys)
	}
	if issued.Bot.IncomingWebhooks[0].LastUsedState != BotCredentialLastUsedUnavailable {
		t.Fatalf("incoming webhook usage = %+v", issued.Bot.IncomingWebhooks)
	}
	if authenticated, err := c.ValidateBotIncomingWebhookCredential(ctx, issued.Credential); err != nil || authenticated.GetId() != bot.User.GetId() {
		t.Fatalf("ValidateBotIncomingWebhookCredential = %+v, %v", authenticated, err)
	}
}

func TestBotAPIKeyAuthenticationRecordsAndRevocationForgetsUsage(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "api-key-usage-owner", "API-key Usage Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "api_key_usage_bot", "API-key Usage Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	issued, err := c.CreateBotAPIKey(ctx, owner.GetId(), bot.User.GetId(), "Production")
	if err != nil {
		t.Fatalf("CreateBotAPIKey: %v", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, issued.Credential); err != nil {
		t.Fatalf("ValidateBotAPIKey: %v", err)
	}
	managed, err := c.GetBot(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetBot: %v", err)
	}
	c.HydrateBotCredentialUsage(ctx, managed)
	found := false
	for _, key := range managed.APIKeys {
		if key.ID == issued.KeyID {
			found = true
			if key.LastUsedState != BotCredentialLastUsedRecorded || key.LastUsedAt.IsZero() {
				t.Fatalf("recorded API-key usage = %+v", key)
			}
		}
	}
	if !found {
		t.Fatalf("issued key %q missing from %+v", issued.KeyID, managed.APIKeys)
	}
	if _, err := c.RevokeBotAPIKey(ctx, owner.GetId(), bot.User.GetId(), issued.KeyID); err != nil {
		t.Fatalf("RevokeBotAPIKey: %v", err)
	}
	lastUsed, available := c.credentialUsage.LastUsed(ctx, bot.User.GetId())
	if !available {
		t.Fatal("credential usage became unavailable")
	}
	if _, exists := lastUsed[botAPIKeyUsageKey(issued.KeyID)]; exists {
		t.Fatalf("revoked API-key usage remained: %+v", lastUsed)
	}
}

func TestBotConstructionAndCredentialIssuanceSkipUsageTelemetryReads(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "deferred-usage-owner", "Deferred Usage Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	usageKV := &countingCredentialUsageKV{KeyValue: c.storage.runtimeStateKV}
	c.credentialUsage.kv = usageKV

	bot, err := c.CreateBot(ctx, owner.GetId(), "deferred_usage_bot", "Deferred Usage Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	issued, err := c.CreateBotIncomingWebhook(ctx, owner.GetId(), bot.User.GetId(), "Production")
	if err != nil {
		t.Fatalf("CreateBotIncomingWebhook: %v", err)
	}
	if issued.Bot.IncomingWebhooks[0].LastUsedState != BotCredentialLastUsedNoUseRecorded || !issued.Bot.IncomingWebhooks[0].LastUsedAt.IsZero() {
		t.Fatalf("new webhook usage = %+v, want no recorded use", issued.Bot.IncomingWebhooks[0])
	}
	key, err := c.CreateBotAPIKey(ctx, owner.GetId(), bot.User.GetId(), "Production")
	if err != nil {
		t.Fatalf("CreateBotAPIKey: %v", err)
	}
	if got := key.Bot.IncomingWebhooks[0].LastUsedState; got != BotCredentialLastUsedUnspecified {
		t.Fatalf("unhydrated API-key issuance webhook state = %v, want unspecified", got)
	}
	if _, err := c.GetBot(ctx, owner.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("GetBot: %v", err)
	}
	bots, err := c.ListBots(ctx, owner.GetId())
	if err != nil {
		t.Fatalf("ListBots: %v", err)
	}
	if got := usageKV.GetCount(); got != 0 {
		t.Fatalf("usage KV reads before hydration = %d, want 0", got)
	}

	c.HydrateBotCredentialUsage(ctx, bots[0])
	if got := usageKV.GetCount(); got != 1 {
		t.Fatalf("usage KV reads after one hydration = %d, want 1", got)
	}
	if got := bots[0].IncomingWebhooks[0]; got.LastUsedState != BotCredentialLastUsedNoUseRecorded || !got.LastUsedAt.IsZero() {
		t.Fatalf("missing usage record = %+v, want no recorded use", got)
	}
}

type failingCredentialUsageKV struct {
	jetstream.KeyValue
	err error
}

type blockingCreateCredentialUsageKV struct {
	jetstream.KeyValue
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (kv *blockingCreateCredentialUsageKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	kv.once.Do(func() {
		close(kv.started)
		<-kv.release
	})
	return kv.KeyValue.Create(ctx, key, value, opts...)
}

type countingCredentialUsageKV struct {
	jetstream.KeyValue
	mu       sync.Mutex
	getCount int
}

func (kv *countingCredentialUsageKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.mu.Lock()
	kv.getCount++
	kv.mu.Unlock()
	return kv.KeyValue.Get(ctx, key)
}

func (kv *countingCredentialUsageKV) GetCount() int {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	return kv.getCount
}

func (kv failingCredentialUsageKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, fmt.Errorf("credential usage read: %w", kv.err)
}
