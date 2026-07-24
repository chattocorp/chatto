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
	IntentSeq uint64    `json:"intent_seq,omitempty"`
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
	if err := c.userModel.waitForUserAuthCurrent(ctx, "bot API key status"); err != nil {
		return nil, err
	}
	intent, hasIntent := c.Users.AuthProjection().BotAPIKeyIntent(botID)
	if hasIntent && !intent.Active {
		return nil, nil
	}
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
	if hasIntent && (record.IntentSeq != intent.Sequence || record.TokenHash != intent.TokenHash) {
		return nil, nil
	}
	return &BotAPIKeyStatus{CreatedAt: record.CreatedAt}, nil
}

// RotateBotAPIKey issues the bot's first key or atomically replaces its single
// active key. Only the HMAC-derived verifier is persisted.
func (c *ChattoCore) RotateBotAPIKey(ctx context.Context, actorID, botID string) (string, *BotAPIKeyStatus, error) {
	if err := c.requireOwnedBot(ctx, actorID, botID); err != nil {
		return "", nil, err
	}
	token := NewBotAPIKey(botID)
	createdAt := time.Now().UTC()
	tokenHash := c.botAPIKeyHash(botID, token)
	key := botAPIKeyRecordKey(botID)
	intentSeq, err := c.recordBotAPIKeyRotated(ctx, actorID, botID, tokenHash)
	if err != nil {
		return "", nil, err
	}
	if intent, exists := c.Users.AuthProjection().BotAPIKeyIntent(botID); exists && intent.Sequence == intentSeq && !intent.CreatedAt.IsZero() {
		createdAt = intent.CreatedAt
	}
	record := botAPIKeyRecord{BotID: botID, TokenHash: tokenHash, CreatedAt: createdAt, IntentSeq: intentSeq}
	value, err := json.Marshal(record)
	if err != nil {
		return "", nil, fmt.Errorf("marshal bot API key: %w", err)
	}

	for attempt := 0; attempt < maxBotAPIKeyRetries; attempt++ {
		intent, exists := c.Users.AuthProjection().BotAPIKeyIntent(botID)
		if !exists || !intent.Active || intent.Sequence != intentSeq || intent.TokenHash != tokenHash {
			return "", nil, fmt.Errorf("bot API key rotation superseded: %w", events.ErrConflict)
		}
		previous, getErr := c.storage.runtimeStateKV.Get(ctx, key)
		switch {
		case errors.Is(getErr, jetstream.ErrKeyNotFound):
			_, err = c.storage.runtimeStateKV.Create(ctx, key, value)
		case getErr != nil:
			return "", nil, fmt.Errorf("get bot API key for rotation: %w", getErr)
		default:
			var previousRecord botAPIKeyRecord
			if json.Unmarshal(previous.Value(), &previousRecord) == nil && previousRecord.IntentSeq > intentSeq {
				return "", nil, fmt.Errorf("bot API key rotation superseded: %w", events.ErrConflict)
			}
			_, err = c.storage.runtimeStateKV.Update(ctx, key, value, previous.Revision())
		}
		if err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return "", nil, fmt.Errorf("store bot API key: %w", err)
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
	status, err := c.GetBotAPIKeyStatus(ctx, botID)
	if err != nil {
		return err
	}
	if status == nil {
		return nil
	}
	if _, err := c.recordBotAPIKeyRevoked(ctx, actorID, botID); err != nil {
		return err
	}
	key := botAPIKeyRecordKey(botID)
	for attempt := 0; attempt < maxBotAPIKeyRetries; attempt++ {
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
	if err := c.userModel.waitForUserAuthCurrent(ctx, "bot API key intent"); err != nil {
		return ValidatedRuntimeCredential{}, err
	}
	intent, hasIntent := c.Users.AuthProjection().BotAPIKeyIntent(botID)
	if hasIntent && !intent.Active {
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
	if hasIntent && (record.IntentSeq != intent.Sequence || record.TokenHash != intent.TokenHash) {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
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

// requireOwnedBot is stricter than general bot administration because issuing
// a credential reveals a new secret. Administrators may revoke another
// owner's credential, but only the accountable owner may receive one.
func (c *ChattoCore) requireOwnedBot(ctx context.Context, actorID, botID string) error {
	if err := c.requireManageableBot(ctx, actorID, botID); err != nil {
		return err
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil || !isBotAccount(bot) {
		return ErrNotFound
	}
	if bot.GetBot().GetOwnerId() != actorID {
		return ErrPermissionDenied
	}
	return nil
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

func (c *ChattoCore) recordBotAPIKeyRotated(ctx context.Context, actorID, botID, tokenHash string) (uint64, error) {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &corev1.BotAPIKeyRotatedEvent{UserId: botID, Request: auditRequestMetadata(ctx), TokenHash: tokenHash},
	}})
	entry := events.BatchEntry{Subject: events.UserAggregate(botID).SubjectFor(event), Event: event}
	seq, err := c.appendUserBatchAuthorized(ctx, botID, []events.BatchEntry{entry}, "", true, func() error {
		if err := c.requireOwnedBot(ctx, actorID, botID); err != nil {
			return err
		}
		intent, exists := c.Users.AuthProjection().BotAPIKeyIntent(botID)
		event.GetBotApiKeyRotated().ReplacedExisting = exists && intent.Active
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("append bot API key rotation intent: %w", err)
	}
	return seq, nil
}

func (c *ChattoCore) recordBotAPIKeyRevoked(ctx context.Context, actorID, botID string) (uint64, error) {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotApiKeyRevoked{
		BotApiKeyRevoked: &corev1.BotAPIKeyRevokedEvent{UserId: botID, Request: auditRequestMetadata(ctx)},
	}})
	entry := events.BatchEntry{Subject: events.UserAggregate(botID).SubjectFor(event), Event: event}
	seq, err := c.appendUserBatchAuthorized(ctx, botID, []events.BatchEntry{entry}, "", true, func() error {
		return c.requireManageableBot(ctx, actorID, botID)
	})
	if err != nil {
		return 0, fmt.Errorf("append bot API key revocation intent: %w", err)
	}
	return seq, nil
}
