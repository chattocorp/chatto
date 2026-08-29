package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	botAPIKeyPrefix                   = "cht_BK_"
	botIncomingWebhookPrefix          = "cht_IW_"
	botAPIKeyVerifierPurpose          = "bot_api_key"
	botIncomingWebhookVerifierPurpose = "bot_incoming_webhook"
	maxBotIncomingWebhooks            = 20
	maxBotIncomingWebhookNameLength   = 64
	legacyBotIncomingWebhookID        = "legacy"
)

// legacyBotAPIKeySecretBytes keeps API keys issued before the shorter format
// valid until their owner explicitly rotates them.
const legacyBotAPIKeySecretBytes = 32

// Bot is the management view of a bot account. Raw credentials are populated
// only by the command that issues them and must never be logged or persisted.
type Bot struct {
	User             *evtv1.User
	OwnerUserID      string
	APIKey           string
	APIKeyCreatedAt  time.Time
	APIKeyRotatedAt  time.Time
	IncomingWebhooks []BotIncomingWebhook
}

// BotIncomingWebhook is safe management metadata for one active credential.
type BotIncomingWebhook struct {
	ID            string
	Name          string
	CreatedAt     time.Time
	LastUsedAt    time.Time
	LastUsedState BotCredentialLastUsedState
}

// BotCredentialLastUsedState identifies whether optional credential-use
// telemetry was loaded and whether it has a value.
type BotCredentialLastUsedState uint8

const (
	BotCredentialLastUsedUnspecified BotCredentialLastUsedState = iota
	BotCredentialLastUsedNoUseRecorded
	BotCredentialLastUsedRecorded
	BotCredentialLastUsedUnavailable
)

// BotIncomingWebhookIssue contains the show-once secret returned by a create
// command. Credential must never be logged or persisted.
type BotIncomingWebhookIssue struct {
	Bot        *Bot
	WebhookID  string
	Credential string
}

func parseBotAPIKey(token string) (string, bool) {
	return parseBotCredential(token, botAPIKeyPrefix, true)
}

func parseBotIncomingWebhookCredential(token string) (botID, webhookID string, ok bool) {
	if !strings.HasPrefix(token, botIncomingWebhookPrefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(token, botIncomingWebhookPrefix), ".")
	if len(parts) == 2 {
		if !isCanonicalUserID(parts[0]) || !isCanonicalBotCredentialSecret(parts[1], false) {
			return "", "", false
		}
		return parts[0], legacyBotIncomingWebhookID, true
	}
	if len(parts) != 3 || !isCanonicalUserID(parts[0]) || !isCanonicalBotIncomingWebhookID(parts[1]) || !isCanonicalBotCredentialSecret(parts[2], false) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func parseBotCredential(token, prefix string, allowLegacyLength bool) (string, bool) {
	if !strings.HasPrefix(token, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(token, prefix)
	botID, encodedSecret, ok := strings.Cut(rest, ".")
	if !ok || !isCanonicalUserID(botID) || encodedSecret == "" || strings.Contains(encodedSecret, ".") {
		return "", false
	}
	if !isCanonicalBotCredentialSecret(encodedSecret, allowLegacyLength) {
		return "", false
	}
	return botID, true
}

func isCanonicalBotCredentialSecret(encodedSecret string, allowLegacyLength bool) bool {
	secret, err := base64.RawURLEncoding.DecodeString(encodedSecret)
	validLength := len(secret) == botAPIKeySecretBytes || allowLegacyLength && len(secret) == legacyBotAPIKeySecretBytes
	return err == nil && validLength && base64.RawURLEncoding.EncodeToString(secret) == encodedSecret
}

func isCanonicalBotIncomingWebhookID(id string) bool {
	if id == legacyBotIncomingWebhookID {
		return true
	}
	if len(id) != idLength+1 || id[0] != 'W' {
		return false
	}
	for i := 1; i < len(id); i++ {
		if !strings.ContainsRune(idAlphabet, rune(id[i])) {
			return false
		}
	}
	return true
}

func normalizedIncomingWebhookID(id string) string {
	if id == "" {
		return legacyBotIncomingWebhookID
	}
	return id
}

func (c *ChattoCore) botAPIKeyVerifier(token string) []byte {
	return c.botCredentialVerifier(botAPIKeyVerifierPurpose, token)
}

func (c *ChattoCore) botIncomingWebhookVerifier(token string) []byte {
	return c.botCredentialVerifier(botIncomingWebhookVerifierPurpose, token)
}

func (c *ChattoCore) botCredentialVerifier(purpose, token string) []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(token))
	return mac.Sum(nil)
}

// ValidateBotIncomingWebhookCredential authenticates an action-limited
// incoming webhook credential against the latest verifier replayed from EVT.
func (c *ChattoCore) ValidateBotIncomingWebhookCredential(ctx context.Context, token string) (*evtv1.User, error) {
	botID, webhookID, ok := parseBotIncomingWebhookCredential(token)
	if !ok {
		return nil, ErrAuthTokenNotFound
	}
	agg := evtstream.UserAggregate(botID)
	if err := c.userModel.waitForUsersCurrent(ctx, "bot incoming webhook authentication", agg.AllEventsFilter()); err != nil {
		return nil, err
	}
	credential, ok := c.userModel.botIncomingWebhookCredential(botID, webhookID)
	presentedVerifier := c.botIncomingWebhookVerifier(token)
	if !ok || subtle.ConstantTimeCompare(presentedVerifier, credential.Verifier) != 1 {
		return nil, ErrAuthTokenNotFound
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil || !bot.GetIsBot() {
		return nil, ErrAuthTokenNotFound
	}
	owner, err := c.GetUser(ctx, bot.GetBotOwnerUserId())
	if err != nil || owner.GetIsBot() {
		return nil, ErrAuthTokenNotFound
	}
	c.credentialUsage.recordIfActive(botID, incomingWebhookUsageKey(webhookID), time.Now(), func() bool {
		current, exists := c.userModel.botIncomingWebhookCredential(botID, webhookID)
		return exists && subtle.ConstantTimeCompare(presentedVerifier, current.Verifier) == 1
	})
	return bot, nil
}

// ValidateBotAPIKey authenticates a bot's non-expiring API key against the
// latest verifier replayed from EVT. It returns ErrAuthTokenNotFound for every
// malformed, stale, deleted, or otherwise unusable key.
func (c *ChattoCore) ValidateBotAPIKey(ctx context.Context, token string) (*evtv1.User, error) {
	user, _, err := c.ValidateBotAPIKeyCredential(ctx, token)
	return user, err
}

// ValidateBotAPIKeyCredential authenticates a bot API key and returns the
// non-secret verifier generation needed to revoke long-lived transports when
// a later durable rotation reaches this replica.
func (c *ChattoCore) ValidateBotAPIKeyCredential(ctx context.Context, token string) (*evtv1.User, []byte, error) {
	botID, ok := parseBotAPIKey(token)
	if !ok {
		return nil, nil, ErrAuthTokenNotFound
	}
	agg := evtstream.UserAggregate(botID)
	if err := c.userModel.waitForUsersCurrent(ctx, "bot API key authentication", agg.AllEventsFilter()); err != nil {
		return nil, nil, err
	}
	credential, ok := c.userModel.botAPIKeyCredential(botID)
	presentedVerifier := c.botAPIKeyVerifier(token)
	if !ok || subtle.ConstantTimeCompare(presentedVerifier, credential.Verifier) != 1 {
		return nil, nil, ErrAuthTokenNotFound
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil || !bot.GetIsBot() {
		return nil, nil, ErrAuthTokenNotFound
	}
	owner, err := c.GetUser(ctx, bot.GetBotOwnerUserId())
	if err != nil || owner.GetIsBot() {
		return nil, nil, ErrAuthTokenNotFound
	}
	return bot, presentedVerifier, nil
}

// WatchBotAPIKeyInvalidated closes when the durable user-auth projection
// observes that verifier is no longer current for botID.
func (c *ChattoCore) WatchBotAPIKeyInvalidated(botID string, verifier []byte) (<-chan struct{}, func()) {
	if c.userModel == nil || c.userModel.auth.Projection() == nil || botID == "" || len(verifier) == 0 {
		return nil, func() {}
	}
	return c.userModel.auth.Projection().watchBotAPIKeyInvalidated(botID, verifier)
}

func (c *ChattoCore) requireHumanUser(ctx context.Context, userID string) error {
	if userID == "" {
		return ErrNotAuthenticated
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.GetIsBot() {
		return ErrHumanAccountRequired
	}
	return nil
}

func (c *ChattoCore) requireBotManager(ctx context.Context, actorID, botID string) (*evtv1.User, error) {
	if err := c.requireHumanUser(ctx, actorID); err != nil {
		return nil, err
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	if !bot.GetIsBot() {
		return nil, ErrNotFound
	}
	if bot.GetBotOwnerUserId() == actorID {
		return bot, nil
	}
	allowed, err := c.CanManageBots(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	return bot, nil
}

func (c *ChattoCore) requireBotReassignmentManager(ctx context.Context, actorID string) error {
	if err := c.requireHumanUser(ctx, actorID); err != nil {
		return err
	}
	allowed, err := c.CanManageBots(ctx, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) botFromUser(user *evtv1.User) (*Bot, error) {
	if user == nil || !user.GetIsBot() {
		return nil, ErrNotFound
	}
	credential, ok := c.userModel.botAPIKeyCredential(user.GetId())
	if !ok {
		return nil, ErrNotFound
	}
	bot := &Bot{
		User: user, OwnerUserID: user.GetBotOwnerUserId(),
		APIKeyCreatedAt: credential.CreatedAt, APIKeyRotatedAt: credential.RotatedAt,
	}
	for _, webhook := range c.userModel.botIncomingWebhookCredentials(user.GetId()) {
		bot.IncomingWebhooks = append(bot.IncomingWebhooks, BotIncomingWebhook{
			ID: webhook.ID, Name: webhook.Name, CreatedAt: webhook.CreatedAt,
		})
	}
	return bot, nil
}

// HydrateBotCredentialUsage adds optional last-use telemetry to request-local
// bot metadata. Callers must defer this read until after filtering and
// pagination, and credential-issuing paths must not call it.
func (c *ChattoCore) HydrateBotCredentialUsage(ctx context.Context, bot *Bot) {
	if bot == nil || bot.User == nil || len(bot.IncomingWebhooks) == 0 {
		return
	}
	lastUsed, available := c.credentialUsage.LastUsed(ctx, bot.User.GetId())
	for i := range bot.IncomingWebhooks {
		webhook := &bot.IncomingWebhooks[i]
		webhook.LastUsedAt = lastUsed[incomingWebhookUsageKey(webhook.ID)]
		switch {
		case !available:
			webhook.LastUsedState = BotCredentialLastUsedUnavailable
		case webhook.LastUsedAt.IsZero():
			webhook.LastUsedState = BotCredentialLastUsedNoUseRecorded
		default:
			webhook.LastUsedState = BotCredentialLastUsedRecorded
		}
	}
}

func (c *ChattoCore) credentialUsageIsActive(botID, credentialKey string) bool {
	prefix := credentialUsageWebhookKind + ":"
	if !strings.HasPrefix(credentialKey, prefix) {
		// A future credential kind must define its lifecycle check before this
		// recorder can remove its telemetry.
		return true
	}
	webhookID := strings.TrimPrefix(credentialKey, prefix)
	if webhookID == "" {
		return false
	}
	_, active := c.userModel.botIncomingWebhookCredential(botID, webhookID)
	return active
}

// CreateBot creates a passwordless bot owned by actorID and returns its raw key once.
func (c *ChattoCore) CreateBot(ctx context.Context, actorID, login, displayName string) (*Bot, error) {
	check := func() error {
		if err := c.requireHumanUser(ctx, actorID); err != nil {
			return err
		}
		allowed, err := c.CanCreateBots(ctx, actorID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPermissionDenied
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	var apiKey string
	user, err := c.createUserWithOptions(ctx, actorID, login, displayName, "", userCreationOptions{
		isBot:        true,
		botOwnerID:   actorID,
		botAPIKeyOut: &apiKey,
		authorize:    check,
	})
	if err != nil {
		return nil, err
	}
	bot, err := c.botFromUser(user)
	if err != nil {
		return nil, err
	}
	bot.APIKey = apiKey
	return bot, nil
}

// GetBot returns one bot visible to the human caller without optional
// credential-use telemetry. Response assembly can hydrate that state later.
func (c *ChattoCore) GetBot(ctx context.Context, actorID, botID string) (*Bot, error) {
	user, err := c.requireBotManager(ctx, actorID, botID)
	if err != nil {
		return nil, err
	}
	return c.botFromUser(user)
}

// ListBots returns owned bots, or every bot for callers with bot.manage. It
// leaves optional credential-use telemetry unhydrated so callers can filter
// and paginate before they read RUNTIME_STATE.
func (c *ChattoCore) ListBots(ctx context.Context, actorID string) ([]*Bot, error) {
	if err := c.requireHumanUser(ctx, actorID); err != nil {
		return nil, err
	}
	manageAll, err := c.CanManageBots(ctx, actorID)
	if err != nil {
		return nil, err
	}
	ids := c.userModel.botIDsOwnedBy(actorID)
	if manageAll {
		ids = c.userModel.botIDs()
	}
	result := make([]*Bot, 0, len(ids))
	for _, id := range ids {
		user, err := c.GetUser(ctx, id)
		if err != nil {
			continue
		}
		bot, err := c.botFromUser(user)
		if err == nil {
			result = append(result, bot)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if strings.EqualFold(result[i].User.GetLogin(), result[j].User.GetLogin()) {
			return result[i].User.GetId() < result[j].User.GetId()
		}
		return strings.ToLower(result[i].User.GetLogin()) < strings.ToLower(result[j].User.GetLogin())
	})
	return result, nil
}

// RotateBotAPIKey replaces the sole verifier. There is deliberately no retry:
// concurrent rotations conflict so two callers cannot both receive keys while
// only one remains current.
func (c *ChattoCore) RotateBotAPIKey(ctx context.Context, actorID, botID string) (*Bot, error) {
	key, err := NewBotAPIKey(botID)
	if err != nil {
		return nil, err
	}
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return nil, err
	}
	filter := evtstream.UserAggregate(botID).AllEventsFilter()
	filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUserAuthCurrent(ctx, "bot API key rotation"); err != nil {
		return nil, err
	}
	rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return nil, err
	}
	if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(evtstream.RBACSubjectFilter(), rbacSeq)); err != nil {
		return nil, err
	}
	if _, err := c.requireBotManager(ctx, actorID, botID); err != nil {
		return nil, err
	}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &evtv1.BotApiKeyRotatedEvent{UserId: botID, Verifier: c.botAPIKeyVerifier(key)},
	}})
	subject := evtstream.UserAggregate(botID).SubjectFor(event)
	seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
		Subject: subject, Event: event, HasOCC: true, ExpectedSeq: filterSeq, FilterSubject: filter,
	}}, authorizationSeq)
	if err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
		return nil, err
	}
	user, err := c.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	bot, err := c.botFromUser(user)
	if err != nil {
		return nil, err
	}
	bot.APIKey = key
	return bot, nil
}

type botIncomingWebhookMutation int

const (
	botIncomingWebhookCreate botIncomingWebhookMutation = iota
	botIncomingWebhookRevoke
)

// CreateBotIncomingWebhook creates one named action-limited credential. The
// raw credential is returned once and is never persisted.
func (c *ChattoCore) CreateBotIncomingWebhook(ctx context.Context, actorID, botID, name string) (*BotIncomingWebhookIssue, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxBotIncomingWebhookNameLength {
		return nil, invalidArgument("incoming webhook name must contain 1 to 64 characters")
	}
	return c.mutateBotIncomingWebhook(ctx, actorID, botID, "", name, botIncomingWebhookCreate)
}

// RevokeBotIncomingWebhook irreversibly invalidates one credential. Repeated
// calls for the same well-formed ID are idempotent.
func (c *ChattoCore) RevokeBotIncomingWebhook(ctx context.Context, actorID, botID, webhookID string) (*Bot, error) {
	result, err := c.mutateBotIncomingWebhook(ctx, actorID, botID, webhookID, "", botIncomingWebhookRevoke)
	if err != nil {
		return nil, err
	}
	return result.Bot, nil
}

func (c *ChattoCore) mutateBotIncomingWebhook(ctx context.Context, actorID, botID, webhookID, name string, mutation botIncomingWebhookMutation) (*BotIncomingWebhookIssue, error) {
	if mutation == botIncomingWebhookCreate {
		webhookID = NewBotIncomingWebhookID()
	} else if !isCanonicalBotIncomingWebhookID(webhookID) {
		return nil, invalidArgument("invalid incoming webhook ID")
	}
	credential := ""
	var err error
	if mutation == botIncomingWebhookCreate {
		credential, err = NewBotIncomingWebhookCredentialForID(botID, webhookID)
		if err != nil {
			return nil, err
		}
	}
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return nil, err
	}
	filter := evtstream.UserAggregate(botID).AllEventsFilter()
	filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUserAuthCurrent(ctx, "bot incoming webhook mutation"); err != nil {
		return nil, err
	}
	rbacFilter := evtstream.RBACSubjectFilter()
	rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, rbacFilter)
	if err != nil {
		return nil, err
	}
	if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(rbacFilter, rbacSeq)); err != nil {
		return nil, err
	}
	user, err := c.requireBotManager(ctx, actorID, botID)
	if err != nil {
		return nil, err
	}
	webhooks := c.userModel.botIncomingWebhookCredentials(botID)
	_, exists := c.userModel.botIncomingWebhookCredential(botID, webhookID)
	if mutation == botIncomingWebhookCreate && len(webhooks) >= maxBotIncomingWebhooks {
		return nil, invalidArgument("a bot can have at most 20 active incoming webhooks")
	}
	if mutation == botIncomingWebhookRevoke && !exists {
		bot, err := c.botFromUser(user)
		return &BotIncomingWebhookIssue{Bot: bot, WebhookID: webhookID}, err
	}

	var event *evtv1.Event
	switch mutation {
	case botIncomingWebhookCreate:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookCreated{
			BotIncomingWebhookCreated: &evtv1.BotIncomingWebhookCreatedEvent{
				UserId: botID, WebhookId: webhookID, Name: name, Verifier: c.botIncomingWebhookVerifier(credential),
			},
		}})
	case botIncomingWebhookRevoke:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookRevoked{
			BotIncomingWebhookRevoked: &evtv1.BotIncomingWebhookRevokedEvent{UserId: botID, WebhookId: webhookID},
		}})
	default:
		return nil, fmt.Errorf("%w: unsupported incoming webhook mutation", ErrInvalidArgument)
	}
	subject := evtstream.UserAggregate(botID).SubjectFor(event)
	seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
		Subject: subject, Event: event, HasOCC: true, ExpectedSeq: filterSeq, FilterSubject: filter,
	}}, authorizationSeq)
	if err != nil {
		return nil, err
	}
	if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
		return nil, err
	}
	user, err = c.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	bot, err := c.botFromUser(user)
	if err != nil {
		return nil, err
	}
	if mutation == botIncomingWebhookCreate {
		for i := range bot.IncomingWebhooks {
			if bot.IncomingWebhooks[i].ID == webhookID {
				// The credential cannot authenticate before its show-once secret
				// leaves this command, so no recorded use is known without a
				// telemetry read.
				bot.IncomingWebhooks[i].LastUsedState = BotCredentialLastUsedNoUseRecorded
				break
			}
		}
	} else {
		c.credentialUsage.Forget(ctx, botID, incomingWebhookUsageKey(webhookID))
	}
	return &BotIncomingWebhookIssue{Bot: bot, WebhookID: webhookID, Credential: credential}, nil
}

// ReassignBotOwner changes the human account responsible for a bot without
// changing its configured permission allowlist or active API key. The command
// is fenced against bot changes, authorization changes, and owner deletion.
func (c *ChattoCore) ReassignBotOwner(ctx context.Context, actorID, botID, ownerUserID string) (*Bot, error) {
	// Resolve once before building subject filters so malformed public IDs can
	// never become NATS wildcards. Every decision is repeated inside the OCC
	// loop before the durable fact commits.
	if err := c.requireBotReassignmentManager(ctx, actorID); err != nil {
		return nil, err
	}
	if _, err := c.GetUser(ctx, botID); err != nil {
		return nil, err
	}
	if _, err := c.GetUser(ctx, ownerUserID); err != nil {
		return nil, err
	}

	botFilter := evtstream.UserAggregate(botID).AllEventsFilter()
	actorFilter := evtstream.UserAggregate(actorID).AllEventsFilter()
	ownerFilter := evtstream.UserAggregate(ownerUserID).AllEventsFilter()
	rbacFilter := evtstream.RBACSubjectFilter()

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return nil, fmt.Errorf("read authorization fence seq: %w", err)
		}
		botSeq, err := c.EventPublisher.LastSubjectSeq(ctx, botFilter)
		if err != nil {
			return nil, fmt.Errorf("read bot OCC filter seq: %w", err)
		}
		if err := c.userModel.waitForUsersCurrent(ctx, "bot owner reassignment", actorFilter, botFilter, ownerFilter); err != nil {
			return nil, err
		}
		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, rbacFilter)
		if err != nil {
			return nil, fmt.Errorf("read RBAC projection position: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(rbacFilter, rbacSeq)); err != nil {
			return nil, fmt.Errorf("wait for RBAC projection: %w", err)
		}

		if err := c.requireBotReassignmentManager(ctx, actorID); err != nil {
			return nil, err
		}
		bot, err := c.GetUser(ctx, botID)
		if err != nil {
			return nil, err
		}
		if !bot.GetIsBot() {
			return nil, ErrNotFound
		}
		owner, err := c.GetUser(ctx, ownerUserID)
		if err != nil {
			return nil, err
		}
		if owner.GetIsBot() {
			return nil, ErrHumanAccountRequired
		}
		if bot.GetBotOwnerUserId() == ownerUserID {
			return c.botFromUser(bot)
		}

		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotOwnerReassigned{
			BotOwnerReassigned: &evtv1.BotOwnerReassignedEvent{
				UserId:              botID,
				PreviousOwnerUserId: bot.GetBotOwnerUserId(),
				OwnerUserId:         ownerUserID,
			},
		}})
		subject := evtstream.UserAggregate(botID).SubjectFor(event)
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, []evtstream.BatchEntry{{
			Subject: subject, Event: event, HasOCC: true, ExpectedSeq: botSeq, FilterSubject: botFilter,
		}}, authorizationSeq)
		if err == nil {
			position := events.SubjectPosition(subject, seqs[0])
			if err := c.userModel.waitForUsers(ctx, position); err != nil {
				return nil, fmt.Errorf("wait for reassigned bot projection: %w", err)
			}
			if err := c.userModel.waitForUserAuth(ctx, position); err != nil {
				return nil, fmt.Errorf("wait for reassigned bot auth projection: %w", err)
			}
			updated, err := c.GetUser(ctx, botID)
			if err != nil {
				return nil, err
			}
			return c.botFromUser(updated)
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("bot owner reassignment OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

// DeleteBot permanently deletes a managed bot. Repeated calls are idempotent.
func (c *ChattoCore) DeleteBot(ctx context.Context, actorID, botID string) (bool, error) {
	if _, err := c.requireBotManager(ctx, actorID, botID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if err := c.DeleteUser(ctx, actorID, botID); err != nil {
		return false, err
	}
	return true, nil
}

// setBotUserPermissionState applies bot-specific management authorization and
// owner-ceiling checks before writing through the canonical user RBAC path.
func (c *ChattoCore) setBotUserPermissionState(ctx context.Context, actorID, botID string, scope PermissionTargetScope, perm Permission, state PermissionState) error {
	_, err := c.requireBotManager(ctx, actorID, botID)
	if err != nil {
		return err
	}
	if state == PermissionStateDeny {
		return fmt.Errorf("%w: bot permissions are an allowlist and do not support explicit denials", ErrInvalidArgument)
	}
	if !botPermissionDelegable(perm) {
		return fmt.Errorf("%w: permission %s cannot be delegated to a bot", ErrInvalidArgument, perm)
	}
	normalized := normalizePermissionScope(scope)
	validateScope := func() error {
		switch normalized.Kind {
		case MatrixScopeServer:
			normalized.ID = ""
		case MatrixScopeGroup:
			if normalized.ID == "" {
				return fmt.Errorf("%w: group scope requires an ID", ErrInvalidArgument)
			}
			// Capture and apply the shared group tail before trusting this
			// replica's projection. A concurrent deletion after this point
			// advances the authorization fence and conflicts the RBAC append.
			position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupAggregate(normalized.ID).AllEventsFilter())
			if err != nil {
				return fmt.Errorf("read room-group aggregate tail: %w", err)
			}
			if err := c.roomModel.waitForGroupLayout(ctx, position); err != nil {
				return fmt.Errorf("wait for room-group projection: %w", err)
			}
			if _, err := c.GetRoomGroup(ctx, normalized.ID); err != nil {
				return fmt.Errorf("%w: group scope %q does not exist", ErrInvalidArgument, normalized.ID)
			}
		case MatrixScopeRoom:
			if normalized.ID == "" {
				return fmt.Errorf("%w: room scope requires an ID", ErrInvalidArgument)
			}
			position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(normalized.ID).AllEventsFilter())
			if err != nil {
				return fmt.Errorf("read room aggregate tail: %w", err)
			}
			if err := c.roomModel.waitForDirectory(ctx, position); err != nil {
				return fmt.Errorf("wait for room directory projection: %w", err)
			}
			// Room-to-group membership is owned by evt.group.>, so room-scoped
			// inherited authority also requires the complete group-layout tail.
			groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
			if err != nil {
				return fmt.Errorf("read room-group projection position: %w", err)
			}
			if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
				return fmt.Errorf("wait for room-group projection: %w", err)
			}
			if _, err := c.GetRoom(ctx, KindChannel, normalized.ID); err != nil {
				return fmt.Errorf("%w: room scope %q does not exist", ErrInvalidArgument, normalized.ID)
			}
		default:
			return fmt.Errorf("%w: unsupported permission scope %q", ErrInvalidArgument, normalized.Kind)
		}
		return nil
	}
	check := func() error {
		// applyUserPermissionState invokes this again inside its mutation path so
		// neither manager authority nor the referenced scope is trusted from the
		// earlier request-level check alone.
		if err := validateScope(); err != nil {
			return err
		}
		currentBot, err := c.requireBotManager(ctx, actorID, botID)
		if err != nil {
			return err
		}
		if state == PermissionStateAllow {
			var decision DecisionKind
			switch normalized.Kind {
			case MatrixScopeGroup:
				decision, err = c.PermResolver().ResolveGroup(ctx, currentBot.GetBotOwnerUserId(), KindChannel, normalized.ID, perm)
			case MatrixScopeRoom:
				decision, err = c.PermResolver().Resolve(ctx, currentBot.GetBotOwnerUserId(), KindChannel, normalized.ID, perm)
			default:
				decision, err = c.PermResolver().Resolve(ctx, currentBot.GetBotOwnerUserId(), KindChannel, "", perm)
			}
			if err != nil {
				return err
			}
			if decision != DecisionAllow {
				return ErrBotOwnerPermissionCeiling
			}
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	var coreScope PermissionScope
	switch normalized.Kind {
	case MatrixScopeGroup:
		coreScope = ScopeGroup
	case MatrixScopeRoom:
		coreScope = ScopeRoom
	default:
		coreScope = ScopeServer
		normalized.ID = ""
	}
	if err := c.applyUserPermissionState(ctx, actorID, coreScope, normalized.ID, botID, perm, state, check); err != nil {
		return err
	}
	return nil
}

func botPermissionDelegable(perm Permission) bool {
	if _, known := GetPermissionMetadata(perm); !known {
		return false
	}
	return perm != PermBotCreate && perm != PermBotManage && perm != PermUserDeleteSelf
}
