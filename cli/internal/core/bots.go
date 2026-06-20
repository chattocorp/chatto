package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const botTokenKeyPrefix = "bot_token."

var (
	ErrBotTokenNotFound     = errors.New("bot token not found")
	ErrBotTokenExpired      = errors.New("bot token expired")
	ErrBotTokenTTLTooLong   = errors.New("bot token expiry exceeds server maximum")
	ErrBotTokenIndefinite   = errors.New("indefinite bot tokens are disabled")
	ErrBotActorNotAllowed   = errors.New("bot accounts cannot manage bot accounts")
	ErrBotOwnerRequired     = errors.New("bot owner is required")
	ErrBotTokenNameRequired = errors.New("bot token name is required")
)

type BotTokenRecord struct {
	ID           string     `json:"id"`
	BotUserID    string     `json:"bot_user_id"`
	Name         string     `json:"name"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	LastUsedBy   string     `json:"last_used_by,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokedBy    string     `json:"revoked_by,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
}

type BotTokenMetadata struct {
	ID           string
	BotUserID    string
	Name         string
	CreatedBy    string
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	LastUsedAt   *time.Time
	RevokedAt    *time.Time
	RevokedBy    string
	RevokeReason string
}

func (c *ChattoCore) botTokenKey(token string) string {
	return c.runtimeTokenKey(botTokenKeyPrefix, token)
}

func IsBotUser(user *corev1.User) bool {
	return user != nil && user.GetKind() == corev1.UserKind_USER_KIND_BOT
}

func IsHumanUser(user *corev1.User) bool {
	return user != nil && !IsBotUser(user)
}

func (c *ChattoCore) ListBots(ctx context.Context) ([]*corev1.User, error) {
	users, err := c.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	var bots []*corev1.User
	for _, user := range users {
		if IsBotUser(user) {
			bots = append(bots, user)
		}
	}
	sort.Slice(bots, func(i, j int) bool {
		ti := bots[i].GetCreatedAt()
		tj := bots[j].GetCreatedAt()
		if ti != nil && tj != nil && !ti.AsTime().Equal(tj.AsTime()) {
			return ti.AsTime().Before(tj.AsTime())
		}
		return strings.ToLower(bots[i].GetLogin()) < strings.ToLower(bots[j].GetLogin())
	})
	return bots, nil
}

func (c *ChattoCore) ListBotsOwnedBy(ctx context.Context, ownerID string) ([]*corev1.User, error) {
	bots, err := c.ListBots(ctx)
	if err != nil {
		return nil, err
	}
	out := bots[:0]
	for _, bot := range bots {
		if bot.GetBotOwnerId() == ownerID {
			out = append(out, bot)
		}
	}
	return out, nil
}

func (c *ChattoCore) CreateBotToken(ctx context.Context, actorID, botUserID, name string, expiresAt *time.Time) (string, BotTokenMetadata, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", BotTokenMetadata{}, ErrBotTokenNameRequired
	}
	bot, err := c.GetUser(ctx, botUserID)
	if err != nil {
		return "", BotTokenMetadata{}, err
	}
	if !IsBotUser(bot) {
		return "", BotTokenMetadata{}, fmt.Errorf("target user is not a bot")
	}
	if err := c.validateBotTokenExpiry(time.Now(), expiresAt); err != nil {
		return "", BotTokenMetadata{}, err
	}

	now := time.Now().UTC()
	token := NewBotToken()
	record := BotTokenRecord{
		ID:        NewEventID(),
		BotUserID: botUserID,
		Name:      name,
		CreatedBy: actorID,
		CreatedAt: now,
	}
	if expiresAt != nil {
		exp := expiresAt.UTC()
		record.ExpiresAt = &exp
	}
	value, err := json.Marshal(record)
	if err != nil {
		return "", BotTokenMetadata{}, err
	}

	key := c.botTokenKey(token)
	if record.ExpiresAt != nil {
		ttl := time.Until(*record.ExpiresAt)
		if ttl <= 0 {
			return "", BotTokenMetadata{}, ErrBotTokenExpired
		}
		_, err = c.storage.runtimeStateKV.Create(ctx, key, value, jetstream.KeyTTL(ttl))
	} else {
		_, err = c.storage.runtimeStateKV.Create(ctx, key, value)
	}
	if err != nil {
		return "", BotTokenMetadata{}, fmt.Errorf("store bot token: %w", err)
	}
	return token, botTokenMetadata(record), nil
}

func (c *ChattoCore) validateBotTokenExpiry(now time.Time, expiresAt *time.Time) error {
	maxTTL := c.config.BotTokenMaxTTL
	if expiresAt == nil {
		if maxTTL > 0 {
			return ErrBotTokenIndefinite
		}
		return nil
	}
	exp := expiresAt.UTC()
	if !exp.After(now) {
		return ErrBotTokenExpired
	}
	if maxTTL > 0 && exp.After(now.Add(maxTTL)) {
		return ErrBotTokenTTLTooLong
	}
	return nil
}

func (c *ChattoCore) ValidateBotToken(ctx context.Context, token string) (string, error) {
	key := c.botTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", ErrBotTokenNotFound
		}
		return "", err
	}
	var record BotTokenRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		return "", err
	}
	if record.RevokedAt != nil {
		return "", ErrBotTokenNotFound
	}
	now := time.Now().UTC()
	if record.ExpiresAt != nil && !record.ExpiresAt.After(now) {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", ErrBotTokenExpired
	}
	if _, err := c.GetUser(ctx, record.BotUserID); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", ErrBotTokenNotFound
	}
	record.LastUsedAt = &now
	value, _ := json.Marshal(record)
	if record.ExpiresAt != nil {
		ttl := time.Until(*record.ExpiresAt)
		if ttl > 0 {
			_, _ = c.updateRuntimeStateTokenTTL(ctx, key, value, entry.Revision(), ttl)
		}
	} else {
		_, _ = c.storage.runtimeStateKV.Update(ctx, key, value, entry.Revision())
	}
	return record.BotUserID, nil
}

func (c *ChattoCore) ValidateAPIAuthToken(ctx context.Context, token string) (string, error) {
	userID, err := c.ValidateAuthToken(ctx, token)
	if err == nil {
		user, loadErr := c.GetUser(ctx, userID)
		if loadErr == nil && IsBotUser(user) {
			return "", ErrAuthTokenNotFound
		}
		return userID, loadErr
	}
	if userID, botErr := c.ValidateBotToken(ctx, token); botErr == nil {
		return userID, nil
	}
	return "", err
}

func (c *ChattoCore) ListBotTokens(ctx context.Context, botUserID string) ([]BotTokenMetadata, error) {
	_, err := c.GetUser(ctx, botUserID)
	if err != nil {
		return nil, err
	}
	records, _, err := c.listBotTokenRecords(ctx, botUserID)
	if err != nil {
		return nil, err
	}
	out := make([]BotTokenMetadata, 0, len(records))
	for _, record := range records {
		out = append(out, botTokenMetadata(record))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (c *ChattoCore) RevokeBotToken(ctx context.Context, actorID, botUserID, tokenID, reason string) error {
	if tokenID == "" {
		return ErrBotTokenNotFound
	}
	records, keys, err := c.listBotTokenRecords(ctx, botUserID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, record := range records {
		if record.ID != tokenID {
			continue
		}
		record.RevokedAt = &now
		record.RevokedBy = actorID
		record.RevokeReason = reason
		value, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if record.ExpiresAt != nil {
			if ttl := time.Until(*record.ExpiresAt); ttl > 0 {
				entry, err := c.storage.runtimeStateKV.Get(ctx, keys[i])
				if err != nil {
					if errors.Is(err, jetstream.ErrKeyNotFound) {
						return ErrBotTokenNotFound
					}
					return err
				}
				if _, err := c.updateRuntimeStateTokenTTL(ctx, keys[i], value, entry.Revision(), ttl); err != nil {
					return err
				}
			} else if err := c.storage.runtimeStateKV.Delete(ctx, keys[i]); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				return err
			}
		} else {
			if _, err := c.storage.runtimeStateKV.Put(ctx, keys[i], value); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrBotTokenNotFound
}

func (c *ChattoCore) RevokeAllBotTokensForUser(ctx context.Context, userID string, reason string) (int, error) {
	records, keys, err := c.listBotTokenRecords(ctx, userID)
	if err != nil {
		return 0, err
	}
	revoked := 0
	for i, record := range records {
		if record.BotUserID != userID {
			continue
		}
		if err := c.storage.runtimeStateKV.Delete(ctx, keys[i]); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return revoked, err
		}
		revoked++
	}
	return revoked, nil
}

func (c *ChattoCore) listBotTokenRecords(ctx context.Context, botUserID string) ([]BotTokenRecord, []string, error) {
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, botTokenKeyPrefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var records []BotTokenRecord
	var keys []string
	for key := range lister.Keys() {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, nil, err
		}
		var record BotTokenRecord
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			c.logger.Warn("Skipping malformed bot token", "key", key, "error", err)
			continue
		}
		if botUserID != "" && record.BotUserID != botUserID {
			continue
		}
		records = append(records, record)
		keys = append(keys, key)
	}
	return records, keys, nil
}

func botTokenMetadata(record BotTokenRecord) BotTokenMetadata {
	return BotTokenMetadata{
		ID:           record.ID,
		BotUserID:    record.BotUserID,
		Name:         record.Name,
		CreatedBy:    record.CreatedBy,
		CreatedAt:    record.CreatedAt,
		ExpiresAt:    cloneTimePtr(record.ExpiresAt),
		LastUsedAt:   cloneTimePtr(record.LastUsedAt),
		RevokedAt:    cloneTimePtr(record.RevokedAt),
		RevokedBy:    record.RevokedBy,
		RevokeReason: record.RevokeReason,
	}
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := t.UTC()
	return &cp
}
