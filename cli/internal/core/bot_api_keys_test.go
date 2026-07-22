package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"hmans.de/chatto/internal/events"
)

func TestBotAPIKeyRotationValidationAndRevocation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "keyowner", "Key Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "keyed_bot", "Keyed Bot", "Uses its key for API access")
	if err != nil {
		t.Fatal(err)
	}

	first, status, err := c.RotateBotAPIKey(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatalf("RotateBotAPIKey first: %v", err)
	}
	if !strings.HasPrefix(first, botAPIKeyPrefix+bot.GetId()) || status == nil || status.CreatedAt.IsZero() {
		t.Fatalf("first key/status = %q, %+v", first, status)
	}
	credential, err := c.ValidatePresentedRuntimeCredential(ctx, first, AuthTokenPresentationBearer)
	if err != nil {
		t.Fatalf("validate first key: %v", err)
	}
	if credential.UserID != bot.GetId() || credential.Kind != AuthTokenKindBotAPIKey {
		t.Fatalf("credential = %+v", credential)
	}

	entry, err := c.storage.runtimeStateKV.Get(ctx, botAPIKeyRecordKey(bot.GetId()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entry.Value()), first) {
		t.Fatal("stored bot API key record contains raw key")
	}
	var record botAPIKeyRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil || record.TokenHash == "" {
		t.Fatalf("stored record = %+v, err = %v", record, err)
	}
	stream, err := c.js.Stream(ctx, "KV_RUNTIME_STATE")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := stream.GetMsg(ctx, entry.Revision())
	if err != nil {
		t.Fatal(err)
	}
	if ttl := msg.Header.Get("Nats-TTL"); ttl != "" {
		t.Fatalf("bot API key TTL = %q, want indefinite", ttl)
	}

	second, _, err := c.RotateBotAPIKey(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatalf("RotateBotAPIKey second: %v", err)
	}
	if second == first {
		t.Fatal("rotation returned the previous key")
	}
	if _, err := c.ValidateBotAPIKey(ctx, first); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("old key validation = %v, want not found", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, second); err != nil {
		t.Fatalf("new key validation: %v", err)
	}
	if _, _, err := c.RotateBotAPIKey(ctx, bot.GetId(), bot.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("bot self-rotation = %v, want permission denied", err)
	}

	rotations, _, err := c.EventPublisher.SubjectEvents(ctx, events.UserAggregate(bot.GetId()).Subject(events.EventBotAPIKeyRotated))
	if err != nil {
		t.Fatal(err)
	}
	if len(rotations) != 2 || rotations[0].GetBotApiKeyRotated().GetReplacedExisting() || !rotations[1].GetBotApiKeyRotated().GetReplacedExisting() {
		t.Fatalf("rotation audit events = %+v", rotations)
	}
	if rotations[0].GetActorId() != owner.GetId() || strings.Contains(rotations[0].String(), first) {
		t.Fatalf("unsafe rotation audit event = %+v", rotations[0])
	}

	if err := c.RevokeBotAPIKey(ctx, owner.GetId(), bot.GetId()); err != nil {
		t.Fatalf("RevokeBotAPIKey: %v", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, second); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("revoked key validation = %v, want not found", err)
	}
	if status, err := c.GetBotAPIKeyStatus(ctx, bot.GetId()); err != nil || status != nil {
		t.Fatalf("status after revoke = %+v, %v", status, err)
	}
	revocations, _, err := c.EventPublisher.SubjectEvents(ctx, events.UserAggregate(bot.GetId()).Subject(events.EventBotAPIKeyRevoked))
	if err != nil || len(revocations) != 1 || revocations[0].GetActorId() != owner.GetId() {
		t.Fatalf("revocation audit events = %+v, %v", revocations, err)
	}
}

func TestBotAPIKeyDeletionCleanup(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "deletekeyowner", "Delete Key Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "deletekey_bot", "Delete Key Bot", "Deleted with its key")
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := c.RotateBotAPIKey(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteBot(ctx, owner.GetId(), bot.GetId()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, key); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("deleted bot key validation = %v, want not found", err)
	}
}
