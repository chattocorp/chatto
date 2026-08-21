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

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const botAPIKeyPrefix = "cht_BK_"

// legacyBotAPIKeySecretBytes keeps API keys issued before the shorter format
// valid until their owner explicitly rotates them.
const legacyBotAPIKeySecretBytes = 32

// Bot is the management view of a bot account. APIKey is populated only by
// CreateBot and RotateBotAPIKey and must never be logged or persisted.
type Bot struct {
	User            *corev1.User
	OwnerUserID     string
	APIKey          string
	APIKeyCreatedAt time.Time
	APIKeyRotatedAt time.Time
}

func parseBotAPIKey(token string) (string, bool) {
	if !strings.HasPrefix(token, botAPIKeyPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(token, botAPIKeyPrefix)
	botID, encodedSecret, ok := strings.Cut(rest, ".")
	if !ok || !isCanonicalUserID(botID) || encodedSecret == "" || strings.Contains(encodedSecret, ".") {
		return "", false
	}
	secret, err := base64.RawURLEncoding.DecodeString(encodedSecret)
	if err != nil || !validBotAPIKeySecretLength(len(secret)) || base64.RawURLEncoding.EncodeToString(secret) != encodedSecret {
		return "", false
	}
	return botID, true
}

func validBotAPIKeySecretLength(length int) bool {
	return length == botAPIKeySecretBytes || length == legacyBotAPIKeySecretBytes
}

func (c *ChattoCore) botAPIKeyVerifier(token string) []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte("bot_api_key"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(token))
	return mac.Sum(nil)
}

// ValidateBotAPIKey authenticates a bot's non-expiring API key against the
// latest verifier replayed from EVT. It returns ErrAuthTokenNotFound for every
// malformed, stale, deleted, or otherwise unusable key.
func (c *ChattoCore) ValidateBotAPIKey(ctx context.Context, token string) (*corev1.User, error) {
	user, _, err := c.ValidateBotAPIKeyCredential(ctx, token)
	return user, err
}

// ValidateBotAPIKeyCredential authenticates a bot API key and returns the
// non-secret verifier generation needed to revoke long-lived transports when
// a later durable rotation reaches this replica.
func (c *ChattoCore) ValidateBotAPIKeyCredential(ctx context.Context, token string) (*corev1.User, []byte, error) {
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

func (c *ChattoCore) requireBotManager(ctx context.Context, actorID, botID string) (*corev1.User, error) {
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

func (c *ChattoCore) botFromUser(user *corev1.User) (*Bot, error) {
	if user == nil || !user.GetIsBot() {
		return nil, ErrNotFound
	}
	credential, ok := c.userModel.botAPIKeyCredential(user.GetId())
	if !ok {
		return nil, ErrNotFound
	}
	return &Bot{
		User: user, OwnerUserID: user.GetBotOwnerUserId(),
		APIKeyCreatedAt: credential.CreatedAt, APIKeyRotatedAt: credential.RotatedAt,
	}, nil
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

// GetBot returns one bot visible to the human caller.
func (c *ChattoCore) GetBot(ctx context.Context, actorID, botID string) (*Bot, error) {
	user, err := c.requireBotManager(ctx, actorID, botID)
	if err != nil {
		return nil, err
	}
	return c.botFromUser(user)
}

// ListBots returns owned bots, or every bot for callers with bot.manage.
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

// UpdateBot changes a bot's public identity as one aggregate mutation. An OCC
// conflict is returned to the interactive caller instead of replaying stale
// edit intent after an intervening write.
func (c *ChattoCore) UpdateBot(ctx context.Context, actorID, botID string, login, displayName *string) (*Bot, error) {
	if login == nil && displayName == nil {
		return nil, fmt.Errorf("%w: at least one field is required", ErrInvalidArgument)
	}
	if _, err := c.requireBotManager(ctx, actorID, botID); err != nil {
		return nil, err
	}
	user, err := c.updateUserProfileAs(ctx, actorID, botID, login, displayName, false)
	if err != nil {
		return nil, err
	}
	return c.botFromUser(user)
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
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &corev1.BotApiKeyRotatedEvent{UserId: botID, Verifier: c.botAPIKeyVerifier(key)},
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
