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
			recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil)
			if err := recorder.writeMax(ctx, botID, credentialKey, usedAt); err != nil {
				t.Errorf("writeMax(%s): %v", usedAt, err)
			}
		}()
	}
	wg.Wait()

	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil)
	lastUsed, available := recorder.LastUsed(ctx, botID)
	if !available || !lastUsed[credentialKey].Equal(base.Add(19*time.Second)) {
		t.Fatalf("LastUsed = %v, %v", lastUsed, available)
	}
}

func TestCredentialUsageRecorderCoalescesWritesButKeepsLocalObservation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	recorder := newCredentialUsageRecorder(c.storage.runtimeStateKV, nil)
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
}

func TestBotManagementReportsUnavailableCredentialUsageWithoutFailing(t *testing.T) {
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
	if len(issued.Bot.IncomingWebhooks) != 1 || issued.Bot.IncomingWebhooks[0].LastUsedAvailable {
		t.Fatalf("incoming webhook usage = %+v", issued.Bot.IncomingWebhooks)
	}
	if authenticated, err := c.ValidateBotIncomingWebhookCredential(ctx, issued.Credential); err != nil || authenticated.GetId() != bot.User.GetId() {
		t.Fatalf("ValidateBotIncomingWebhookCredential = %+v, %v", authenticated, err)
	}
}

type failingCredentialUsageKV struct {
	jetstream.KeyValue
	err error
}

func (kv failingCredentialUsageKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, fmt.Errorf("credential usage read: %w", kv.err)
}
