package core

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	botAPIKeyPrefix       = "cht_BK"
	botAPIKeyRecordPrefix = "bot_api_key."
	userIDLength          = 1 + idLength
	maxBotAPIKeyRetries   = 5
)

// BotAPIKeyStatus is the non-secret metadata for a bot's active API key.
type BotAPIKeyStatus struct {
	CreatedAt time.Time
}

type botAPIKeyRecord struct {
	BotID     string    `json:"bot_id"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
}

func botAPIKeyRecordKey(botID string) string {
	return botAPIKeyRecordPrefix + botID
}

func parseBotAPIKey(token string) (string, bool) {
	remainder, ok := strings.CutPrefix(token, botAPIKeyPrefix)
	if !ok || len(remainder) != userIDLength+idLength {
		return "", false
	}
	for _, char := range remainder {
		if !strings.ContainsRune(idAlphabet, char) {
			return "", false
		}
	}
	botID := remainder[:userIDLength]
	return botID, strings.HasPrefix(botID, "U")
}

func (c *ChattoCore) botAPIKeyHash(botID, token string) string {
	return c.runtimeTokenHash("bot_api_key."+botID, token)
}

// GetBotAPIKeyStatus returns non-secret metadata for the bot's active key.
func (c *ChattoCore) GetBotAPIKeyStatus(ctx context.Context, botID string) (*BotAPIKeyStatus, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, botAPIKeyRecordKey(botID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get bot API key: %w", err)
	}
	var record botAPIKeyRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil || record.BotID != botID || record.TokenHash == "" || record.CreatedAt.IsZero() {
		return nil, errors.New("malformed bot API key record")
	}
	return &BotAPIKeyStatus{CreatedAt: record.CreatedAt}, nil
}

// RotateBotAPIKey issues the bot's first key or atomically replaces its single
// active key. Only the HMAC-derived verifier is persisted.
func (c *ChattoCore) RotateBotAPIKey(ctx context.Context, actorID, botID string) (string, *BotAPIKeyStatus, error) {
	if err := c.requireManageableBot(ctx, actorID, botID); err != nil {
		return "", nil, err
	}
	token := NewBotAPIKey(botID)
	createdAt := time.Now().UTC()
	record := botAPIKeyRecord{BotID: botID, TokenHash: c.botAPIKeyHash(botID, token), CreatedAt: createdAt}
	value, err := json.Marshal(record)
	if err != nil {
		return "", nil, fmt.Errorf("marshal bot API key: %w", err)
	}
	key := botAPIKeyRecordKey(botID)

	for attempt := 0; attempt < maxBotAPIKeyRetries; attempt++ {
		if err := c.requireManageableBot(ctx, actorID, botID); err != nil {
			return "", nil, err
		}
		previous, getErr := c.storage.runtimeStateKV.Get(ctx, key)
		replaced := getErr == nil
		var revision uint64
		switch {
		case errors.Is(getErr, jetstream.ErrKeyNotFound):
			revision, err = c.storage.runtimeStateKV.Create(ctx, key, value)
		case getErr != nil:
			return "", nil, fmt.Errorf("get bot API key for rotation: %w", getErr)
		default:
			revision, err = c.storage.runtimeStateKV.Update(ctx, key, value, previous.Revision())
		}
		if err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return "", nil, fmt.Errorf("store bot API key: %w", err)
		}
		if err := c.recordBotAPIKeyRotated(ctx, actorID, botID, replaced); err != nil {
			c.rollbackBotAPIKeyRotation(ctx, key, previous, revision)
			return "", nil, err
		}
		return token, &BotAPIKeyStatus{CreatedAt: createdAt}, nil
	}
	return "", nil, fmt.Errorf("bot API key rotation retry exhausted: %w", jetstream.ErrKeyExists)
}

// RevokeBotAPIKey removes the bot's active key. Revoking an absent key is a
// successful no-op after the same management authorization check.
func (c *ChattoCore) RevokeBotAPIKey(ctx context.Context, actorID, botID string) error {
	if err := c.requireManageableBot(ctx, actorID, botID); err != nil {
		return err
	}
	key := botAPIKeyRecordKey(botID)
	for attempt := 0; attempt < maxBotAPIKeyRetries; attempt++ {
		if err := c.requireManageableBot(ctx, actorID, botID); err != nil {
			return err
		}
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get bot API key for revocation: %w", err)
		}
		if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return fmt.Errorf("revoke bot API key: %w", err)
		}
		if err := c.recordBotAPIKeyRevoked(ctx, actorID, botID); err != nil {
			_, _ = c.storage.runtimeStateKV.Create(context.WithoutCancel(ctx), key, entry.Value())
			return err
		}
		return nil
	}
	return fmt.Errorf("bot API key revocation retry exhausted: %w", jetstream.ErrKeyExists)
}

// ValidateBotAPIKey validates a raw bot key without refreshing or expiring it.
func (c *ChattoCore) ValidateBotAPIKey(ctx context.Context, token string) (ValidatedRuntimeCredential, error) {
	botID, ok := parseBotAPIKey(token)
	if !ok {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, botAPIKeyRecordKey(botID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
		return ValidatedRuntimeCredential{}, fmt.Errorf("get bot API key credential: %w", err)
	}
	var record botAPIKeyRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil || record.BotID != botID || record.TokenHash == "" {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	want := c.botAPIKeyHash(botID, token)
	if subtle.ConstantTimeCompare([]byte(want), []byte(record.TokenHash)) != 1 {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if err := c.userModel.waitForUsersCurrent(ctx, "bot API key authentication", events.UserAggregate(botID).AllEventsFilter()); err != nil {
		return ValidatedRuntimeCredential{}, err
	}
	ownerID, bot, active, exists := c.Users.AuthorizationIdentity(botID)
	if !exists || !bot || !active {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if err := c.userModel.waitForUsersCurrent(ctx, "bot API key owner authentication", events.UserAggregate(ownerID).AllEventsFilter()); err != nil {
		return ValidatedRuntimeCredential{}, err
	}
	_, ownerBot, ownerActive, ownerExists := c.Users.AuthorizationIdentity(ownerID)
	if !ownerExists || ownerBot || !ownerActive {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	return ValidatedRuntimeCredential{
		Handle: token, UserID: botID, Kind: AuthTokenKindBotAPIKey,
		Presentation: AuthTokenPresentationBearer, Source: "bot_api_key", CreatedAt: record.CreatedAt,
	}, nil
}

func (c *ChattoCore) requireManageableBot(ctx context.Context, actorID, botID string) error {
	if err := c.waitForBotPermissionInputs(ctx, actorID, botID, ScopeServer); err != nil {
		return err
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil || !isBotAccount(bot) {
		return ErrNotFound
	}
	allowed, err := c.CanManageBot(ctx, actorID, botID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) rollbackBotAPIKeyRotation(ctx context.Context, key string, previous jetstream.KeyValueEntry, revision uint64) {
	rollbackCtx := context.WithoutCancel(ctx)
	if previous == nil {
		_ = c.storage.runtimeStateKV.Delete(rollbackCtx, key, jetstream.LastRevision(revision))
		return
	}
	_, _ = c.storage.runtimeStateKV.Update(rollbackCtx, key, previous.Value(), revision)
}

func (c *ChattoCore) revokeBotAPIKeyForDeletion(ctx context.Context, botID string) (bool, error) {
	key := botAPIKeyRecordKey(botID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, err
	}
	return true, nil
}

func (c *ChattoCore) recordBotAPIKeyRotated(ctx context.Context, actorID, botID string, replaced bool) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &corev1.BotAPIKeyRotatedEvent{UserId: botID, ReplacedExisting: replaced, Request: auditRequestMetadata(ctx)},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(botID), event); err != nil {
		return fmt.Errorf("append bot API key rotation audit event: %w", err)
	}
	return nil
}

func (c *ChattoCore) recordBotAPIKeyRevoked(ctx context.Context, actorID, botID string) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotApiKeyRevoked{
		BotApiKeyRevoked: &corev1.BotAPIKeyRevokedEvent{UserId: botID, Request: auditRequestMetadata(ctx)},
	}})
	if err := c.appendAuthAuditEvent(ctx, events.UserAggregate(botID), event); err != nil {
		return fmt.Errorf("append bot API key revocation audit event: %w", err)
	}
	return nil
}
