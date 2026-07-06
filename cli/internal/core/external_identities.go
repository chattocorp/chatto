package core

import (
	"context"
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
	ExternalIdentityFlowTTL = 15 * time.Minute

	externalIdentityCreateTokenKeyPrefix = "external_identity_create."
	externalIdentityLinkTokenKeyPrefix   = "external_identity_link."
	externalIdentityLinkStartKeyPrefix   = "external_identity_link_start."

	ExternalIdentityFlowKindCreate = "create"
	ExternalIdentityFlowKindLink   = "link"
)

var (
	ErrExternalIdentityFlowNotFound  = errors.New("external identity flow not found")
	ErrExternalIdentityFlowExpired   = errors.New("external identity flow expired")
	ErrExternalIdentityFlowWrongKind = errors.New("external identity flow has the wrong kind")
	ErrExternalIdentityFlowUserBound = errors.New("external identity flow is bound to a different user")
	ErrExternalIdentityNotFound      = errors.New("external identity is not linked to this account")
	ErrExternalIdentityLastMethod    = errors.New("cannot disconnect the last sign-in method")
)

type ExternalIdentity struct {
	ProviderID   string
	ProviderType string
	Issuer       string
	Subject      string
	SubjectHash  string
}

type PendingExternalIdentityLinkStart struct {
	ProviderID   string    `json:"provider_id"`
	RedirectPath string    `json:"redirect_path,omitempty"`
	BoundUserID  string    `json:"bound_user_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type PendingExternalIdentityFlow struct {
	Kind            string    `json:"kind"`
	ProviderID      string    `json:"provider_id"`
	ProviderType    string    `json:"provider_type"`
	ProviderLabel   string    `json:"provider_label"`
	Issuer          string    `json:"issuer"`
	Subject         string    `json:"subject"`
	SubjectHash     string    `json:"subject_hash"`
	VerifiedEmail   string    `json:"verified_email,omitempty"`
	AvatarURL       string    `json:"avatar_url,omitempty"`
	LoginHint       string    `json:"login_hint,omitempty"`
	DisplayNameHint string    `json:"display_name_hint,omitempty"`
	RedirectPath    string    `json:"redirect_path,omitempty"`
	BoundUserID     string    `json:"bound_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func (c *ChattoCore) externalIdentityCreateTokenKey(token string) string {
	return c.runtimeTokenKey(externalIdentityCreateTokenKeyPrefix, token)
}

func (c *ChattoCore) externalIdentityLinkTokenKey(token string) string {
	return c.runtimeTokenKey(externalIdentityLinkTokenKeyPrefix, token)
}

func (c *ChattoCore) externalIdentityLinkStartKey(token string) string {
	return c.runtimeTokenKey(externalIdentityLinkStartKeyPrefix, token)
}

func (c *ChattoCore) CreatePendingExternalIdentityLinkStart(ctx context.Context, providerID, redirectPath, userID string) (string, error) {
	start := PendingExternalIdentityLinkStart{
		ProviderID:   strings.TrimSpace(providerID),
		RedirectPath: strings.TrimSpace(redirectPath),
		BoundUserID:  strings.TrimSpace(userID),
		CreatedAt:    time.Now(),
	}
	if start.ProviderID == "" || start.BoundUserID == "" {
		return "", ErrInvalidArgument
	}
	token := NewExternalIdentityLinkStartToken()
	data, err := json.Marshal(start)
	if err != nil {
		return "", fmt.Errorf("marshal external identity link start: %w", err)
	}
	_, err = c.storage.runtimeStateKV.Create(ctx, c.externalIdentityLinkStartKey(token), data, jetstream.KeyTTL(ExternalIdentityFlowTTL))
	if err != nil {
		return "", fmt.Errorf("store external identity link start: %w", err)
	}
	return token, nil
}

func (c *ChattoCore) ConsumePendingExternalIdentityLinkStart(ctx context.Context, token string) (*PendingExternalIdentityLinkStart, error) {
	key := c.externalIdentityLinkStartKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil, ErrExternalIdentityFlowNotFound
		}
		return nil, fmt.Errorf("get external identity link start: %w", err)
	}
	var start PendingExternalIdentityLinkStart
	if err := json.Unmarshal(entry.Value(), &start); err != nil {
		return nil, fmt.Errorf("unmarshal external identity link start: %w", err)
	}
	if time.Since(start.CreatedAt) > ExternalIdentityFlowTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrExternalIdentityFlowExpired
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return nil, fmt.Errorf("delete external identity link start: %w", err)
	}
	return &start, nil
}

func (c *ChattoCore) CreatePendingExternalIdentityCreateFlow(ctx context.Context, flow PendingExternalIdentityFlow) (string, error) {
	flow.Kind = ExternalIdentityFlowKindCreate
	token := NewExternalIdentityCreateToken()
	if err := c.storePendingExternalIdentityFlow(ctx, token, flow); err != nil {
		return "", err
	}
	return token, nil
}

func (c *ChattoCore) CreatePendingExternalIdentityLinkFlow(ctx context.Context, flow PendingExternalIdentityFlow, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrInvalidArgument
	}
	flow.Kind = ExternalIdentityFlowKindLink
	flow.BoundUserID = userID
	token := NewExternalIdentityLinkToken()
	if err := c.storePendingExternalIdentityFlow(ctx, token, flow); err != nil {
		return "", err
	}
	return token, nil
}

func (c *ChattoCore) storePendingExternalIdentityFlow(ctx context.Context, token string, flow PendingExternalIdentityFlow) error {
	flow.ProviderID = strings.TrimSpace(flow.ProviderID)
	flow.ProviderType = strings.TrimSpace(flow.ProviderType)
	flow.Issuer = strings.TrimSpace(flow.Issuer)
	flow.Subject = strings.TrimSpace(flow.Subject)
	if flow.SubjectHash == "" && flow.Issuer != "" && flow.Subject != "" {
		flow.SubjectHash = externalIdentityHash(flow.Issuer, flow.Subject)
	}
	if flow.ProviderID == "" || flow.ProviderType == "" || flow.Issuer == "" || flow.Subject == "" || flow.SubjectHash == "" {
		return fmt.Errorf("external identity flow requires provider and identity fields")
	}
	if flow.ProviderLabel == "" {
		flow.ProviderLabel = flow.ProviderID
	}
	if flow.CreatedAt.IsZero() {
		flow.CreatedAt = time.Now()
	}

	data, err := json.Marshal(flow)
	if err != nil {
		return fmt.Errorf("marshal external identity flow: %w", err)
	}

	var key string
	switch flow.Kind {
	case ExternalIdentityFlowKindCreate:
		key = c.externalIdentityCreateTokenKey(token)
	case ExternalIdentityFlowKindLink:
		key = c.externalIdentityLinkTokenKey(token)
	default:
		return ErrExternalIdentityFlowWrongKind
	}
	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(ExternalIdentityFlowTTL))
	if err != nil {
		return fmt.Errorf("store external identity flow: %w", err)
	}
	return nil
}

func (c *ChattoCore) GetPendingExternalIdentityFlow(ctx context.Context, token string) (*PendingExternalIdentityFlow, error) {
	if flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityCreateTokenKey(token)); err == nil {
		return flow, nil
	} else if !errors.Is(err, ErrExternalIdentityFlowNotFound) {
		return nil, err
	}
	return c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityLinkTokenKey(token))
}

func (c *ChattoCore) GetPendingExternalIdentityCreateFlow(ctx context.Context, token string) (*PendingExternalIdentityFlow, error) {
	flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityCreateTokenKey(token))
	if err != nil {
		return nil, err
	}
	if flow.Kind != ExternalIdentityFlowKindCreate {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	return flow, nil
}

func (c *ChattoCore) GetPendingExternalIdentityLinkFlow(ctx context.Context, token, userID string) (*PendingExternalIdentityFlow, error) {
	flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityLinkTokenKey(token))
	if err != nil {
		return nil, err
	}
	if flow.Kind != ExternalIdentityFlowKindLink {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID != userID {
		return nil, ErrExternalIdentityFlowUserBound
	}
	return flow, nil
}

func (c *ChattoCore) getPendingExternalIdentityFlowByKey(ctx context.Context, key string) (*PendingExternalIdentityFlow, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil, ErrExternalIdentityFlowNotFound
		}
		return nil, fmt.Errorf("get external identity flow: %w", err)
	}
	var flow PendingExternalIdentityFlow
	if err := json.Unmarshal(entry.Value(), &flow); err != nil {
		return nil, fmt.Errorf("unmarshal external identity flow: %w", err)
	}
	if time.Since(flow.CreatedAt) > ExternalIdentityFlowTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrExternalIdentityFlowExpired
	}
	return &flow, nil
}

func (c *ChattoCore) DeletePendingExternalIdentityFlow(ctx context.Context, token string) error {
	var firstErr error
	for _, key := range []string{c.externalIdentityCreateTokenKey(token), c.externalIdentityLinkTokenKey(token)} {
		err := c.storage.runtimeStateKV.Delete(ctx, key)
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("delete external identity flow: %w", firstErr)
	}
	return nil
}

func (c *ChattoCore) CreateUserForExternalIdentity(ctx context.Context, login, displayName string, flow *PendingExternalIdentityFlow) (*corev1.User, error) {
	if flow == nil || flow.Kind != ExternalIdentityFlowKindCreate {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	if displayName == "" {
		displayName = login
	}
	user, err := c.CreateUser(ctx, SystemActorID, login, displayName, "")
	if err != nil {
		return nil, err
	}
	rollback := true
	defer func() {
		if rollback {
			c.rollbackUserCreation(ctx, user)
		}
	}()
	if flow.VerifiedEmail != "" {
		if err := c.AddVerifiedEmailDirect(ctx, user.Id, flow.VerifiedEmail); err != nil {
			return nil, fmt.Errorf("failed to add provider verified email: %w", err)
		}
	}
	if err := c.LinkExternalIdentity(ctx, flow.ProviderID, flow.ProviderType, flow.Issuer, flow.Subject, user.Id); err != nil {
		return nil, err
	}
	if flow.AvatarURL != "" {
		if err := c.ImportUserAvatarFromURL(ctx, user.Id, flow.AvatarURL); err != nil {
			c.logger.Warn("Failed to import provider avatar", "provider_id", flow.ProviderID, "provider_type", flow.ProviderType, "user_id", user.Id, "error", err)
		}
	}
	rollback = false
	return user, nil
}

func (c *ChattoCore) LinkPendingExternalIdentity(ctx context.Context, userID string, flow *PendingExternalIdentityFlow) (ExternalIdentity, error) {
	if flow == nil || flow.Kind != ExternalIdentityFlowKindLink {
		return ExternalIdentity{}, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID != userID {
		return ExternalIdentity{}, ErrExternalIdentityFlowUserBound
	}
	if err := c.LinkExternalIdentity(ctx, flow.ProviderID, flow.ProviderType, flow.Issuer, flow.Subject, userID); err != nil {
		return ExternalIdentity{}, err
	}
	return ExternalIdentity{
		ProviderID:   flow.ProviderID,
		ProviderType: flow.ProviderType,
		Issuer:       flow.Issuer,
		Subject:      flow.Subject,
		SubjectHash:  flow.SubjectHash,
	}, nil
}

func (c *ChattoCore) ConfirmPendingExternalIdentityLink(ctx context.Context, flow *PendingExternalIdentityFlow) (ExternalIdentity, error) {
	if flow == nil || flow.Kind != ExternalIdentityFlowKindLink {
		return ExternalIdentity{}, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID == "" {
		return ExternalIdentity{}, ErrExternalIdentityFlowUserBound
	}
	if err := c.LinkExternalIdentity(ctx, flow.ProviderID, flow.ProviderType, flow.Issuer, flow.Subject, flow.BoundUserID); err != nil {
		return ExternalIdentity{}, err
	}
	return ExternalIdentity{
		ProviderID:   flow.ProviderID,
		ProviderType: flow.ProviderType,
		Issuer:       flow.Issuer,
		Subject:      flow.Subject,
		SubjectHash:  flow.SubjectHash,
	}, nil
}

func (c *ChattoCore) ExternalIdentitiesForUser(ctx context.Context, userID string) ([]ExternalIdentity, error) {
	if err := c.userModel.waitForUsersCurrent(ctx, "external identities", events.UserAggregate(userID).AllEventsFilter()); err != nil {
		return nil, err
	}
	return c.Users.ExternalIdentities(userID), nil
}

// DisconnectExternalIdentity removes a linked provider identity from a user.
// It refuses to remove the last available sign-in method for passwordless
// accounts so users created through SSO cannot lock themselves out.
func (c *ChattoCore) DisconnectExternalIdentity(ctx context.Context, userID, subjectHash string) error {
	userID = strings.TrimSpace(userID)
	subjectHash = strings.TrimSpace(subjectHash)
	if userID == "" || subjectHash == "" {
		return ErrInvalidArgument
	}
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserExternalIdentityUnlinked{
		UserExternalIdentityUnlinked: &corev1.UserExternalIdentityUnlinkedEvent{
			UserId:      userID,
			SubjectHash: subjectHash,
		},
	}})
	_, err := c.appendUserEvent(ctx, userID, event, events.UserSubjectFilter(), func() error {
		if _, ok := c.Users.Get(userID); !ok {
			return ErrNotFound
		}
		identities := c.Users.ExternalIdentities(userID)
		found := false
		for _, identity := range identities {
			if identity.SubjectHash == subjectHash {
				found = true
				break
			}
		}
		if !found {
			return ErrExternalIdentityNotFound
		}
		if _, hasPassword := c.Users.PasswordHash(userID); !hasPassword && len(identities) <= 1 {
			return ErrExternalIdentityLastMethod
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := c.RevokeRuntimeCredentialsForUser(ctx, userID, "external_identity_disconnected"); err != nil {
		c.logger.Warn("Failed to clean up runtime credentials after external identity disconnect", "user_id", userID, "error", err)
	}
	if err := c.PublishSessionTerminated(ctx, userID, "external_identity_disconnected"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", userID, "reason", "external_identity_disconnected", "error", err)
	}
	return nil
}
