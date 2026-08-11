package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/evtstream"
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
	firstRecord := append([]byte(nil), entry.Value()...)
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
	currentEntry, err := c.storage.runtimeStateKV.Get(ctx, botAPIKeyRecordKey(bot.GetId()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.storage.runtimeStateKV.Update(ctx, botAPIKeyRecordKey(bot.GetId()), firstRecord, currentEntry.Revision()); err != nil {
		t.Fatalf("restore stale runtime key record: %v", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, first); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("stale runtime key validation = %v, want not found", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, second); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("key with mismatched runtime intent = %v, want not found", err)
	}
	if status, err := c.GetBotAPIKeyStatus(ctx, bot.GetId()); err != nil || status != nil {
		t.Fatalf("status with mismatched runtime intent = %+v, %v; want no active key", status, err)
	}
	second, _, err = c.RotateBotAPIKey(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatalf("recover rotation after stale runtime record: %v", err)
	}
	if _, _, err := c.RotateBotAPIKey(ctx, bot.GetId(), bot.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("bot self-rotation = %v, want permission denied", err)
	}
	admin, err := c.CreateUser(ctx, SystemActorID, "keyadmin", "Key Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, admin.GetId(), RoleAdmin); err != nil {
		t.Fatal(err)
	}
	details, err := c.GetAdminMemberDetails(ctx, admin.GetId(), bot.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if details.ViewerCanAssignRoles || details.ViewerCanManageUserPermissions || len(details.AvailablePermissions) != 0 {
		t.Fatalf("bot RBAC management flags = %+v", details)
	}
	if err := c.AdminAssignServerRole(ctx, admin.GetId(), bot.GetId(), RoleModerator); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("assign role to bot = %v, want invalid argument", err)
	}
	if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, bot.GetId(), RoleModerator); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("operator role assignment to bot = %v, want invalid argument", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, bot.GetId(), PermMessagePost); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("direct server grant to bot = %v, want invalid argument", err)
	}
	if err := c.DenyUserPermission(ctx, SystemActorID, bot.GetId(), PermMessagePost); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("direct server deny to bot = %v, want invalid argument", err)
	}
	if _, err := c.GetUserPermissionMatrix(ctx, admin.GetId(), bot.GetId()); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("get bot permission matrix = %v, want invalid argument", err)
	}
	if err := c.SetUserPermissionState(ctx, admin.GetId(), bot.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("set bot permission = %v, want invalid argument", err)
	}
	if _, _, err := c.RotateBotAPIKey(ctx, admin.GetId(), bot.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("admin rotation = %v, want permission denied", err)
	}

	rotations, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(bot.GetId()).Subject(evtstream.EventBotAPIKeyRotated))
	if err != nil {
		t.Fatal(err)
	}
	if len(rotations) != 3 || rotations[0].GetBotApiKeyRotated().GetReplacedExisting() || !rotations[1].GetBotApiKeyRotated().GetReplacedExisting() || !rotations[2].GetBotApiKeyRotated().GetReplacedExisting() {
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
	revocations, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(bot.GetId()).Subject(evtstream.EventBotAPIKeyRevoked))
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

func TestBotAPIKeyIntentLinearizesRotationAndRevocation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "key-race-owner", "Key Race Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "key_race_bot", "Key Race Bot", "Exercises credential interleavings")
	if err != nil {
		t.Fatal(err)
	}
	keyName := botAPIKeyRecordKey(bot.GetId())

	// Rotation publishes its durable intent before materialising KV. A revoke
	// in that window must still win, and a late stale KV write stays unusable.
	staleToken := NewBotAPIKey(bot.GetId())
	staleHash := c.botAPIKeyHash(bot.GetId(), staleToken)
	staleSeq, err := c.recordBotAPIKeyRotated(ctx, owner.GetId(), bot.GetId(), staleHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeBotAPIKey(ctx, owner.GetId(), bot.GetId()); err != nil {
		t.Fatal(err)
	}
	revokedIntent, ok := c.userModel.botAPIKeyIntent(bot.GetId())
	if !ok || revokedIntent.Active || revokedIntent.Sequence <= staleSeq {
		t.Fatalf("revoked intent = %+v, exists %v", revokedIntent, ok)
	}
	staleValue, err := json.Marshal(botAPIKeyRecord{
		BotID: bot.GetId(), TokenHash: staleHash, CreatedAt: time.Now().UTC(), IntentSeq: staleSeq,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, keyName, staleValue); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, staleToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("late stale key validation = %v, want not found", err)
	}
	if err := c.deleteBotAPIKeyRecordThrough(ctx, keyName, revokedIntent.Sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := c.storage.runtimeStateKV.Get(ctx, keyName); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("stale key cleanup = %v, want key not found", err)
	}

	// Conversely, cleanup from an older revoke must not delete a newer
	// rotation that won the event order.
	newToken := NewBotAPIKey(bot.GetId())
	newHash := c.botAPIKeyHash(bot.GetId(), newToken)
	newSeq, err := c.recordBotAPIKeyRotated(ctx, owner.GetId(), bot.GetId(), newHash)
	if err != nil {
		t.Fatal(err)
	}
	newValue, err := json.Marshal(botAPIKeyRecord{
		BotID: bot.GetId(), TokenHash: newHash, CreatedAt: time.Now().UTC(), IntentSeq: newSeq,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, keyName, newValue); err != nil {
		t.Fatal(err)
	}
	if err := c.deleteBotAPIKeyRecordThrough(ctx, keyName, revokedIntent.Sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, newToken); err != nil {
		t.Fatalf("newer rotation after older cleanup: %v", err)
	}
}
