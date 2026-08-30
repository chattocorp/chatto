package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	// PendingOAuthAuthorizeTTL bounds an OAuth browser flow that has not yet
	// reached approval, denial, or an already-consented callback.
	PendingOAuthAuthorizeTTL = 15 * time.Minute

	pendingOAuthAuthorizeKeyPrefix = "oauth_authorize."
)

var ErrPendingOAuthAuthorizeNotFound = errors.New("pending OAuth authorization request not found")

// PendingOAuthAuthorize is validated authorization-request state stored in
// RUNTIME_STATE. The browser cookie carries only its opaque lookup token.
type PendingOAuthAuthorize struct {
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	State               string    `json:"state,omitempty"`
	ClientID            string    `json:"client_id,omitempty"`
	ClientName          string    `json:"client_name,omitempty"`
	ClientURI           string    `json:"client_uri,omitempty"`
	Resource            string    `json:"resource,omitempty"`
	Scopes              []string  `json:"scopes,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

func (c *ChattoCore) pendingOAuthAuthorizeKey(token string) string {
	return c.runtimeTokenKey(pendingOAuthAuthorizeKeyPrefix, token)
}

// CreatePendingOAuthAuthorize stores a validated browser authorization flow
// and returns the opaque handle that may be placed in the signed session cookie.
func (c *ChattoCore) CreatePendingOAuthAuthorize(ctx context.Context, pending PendingOAuthAuthorize) (string, error) {
	if pending.RedirectURI == "" || pending.CodeChallenge == "" || pending.CodeChallengeMethod == "" {
		return "", ErrInvalidArgument
	}
	pending.CreatedAt = time.Now()
	data, err := json.Marshal(pending)
	if err != nil {
		return "", fmt.Errorf("marshal pending OAuth authorization request: %w", err)
	}
	token := NewPendingOAuthAuthorizeToken()
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.pendingOAuthAuthorizeKey(token), data, jetstream.KeyTTL(PendingOAuthAuthorizeTTL)); err != nil {
		return "", fmt.Errorf("store pending OAuth authorization request: %w", err)
	}
	return token, nil
}

// GetPendingOAuthAuthorize reads a pending browser flow without consuming it.
func (c *ChattoCore) GetPendingOAuthAuthorize(ctx context.Context, token string) (PendingOAuthAuthorize, error) {
	pending, _, err := c.getPendingOAuthAuthorize(ctx, token)
	return pending, err
}

// ConsumePendingOAuthAuthorize atomically claims and removes a pending browser
// flow so concurrent approval, denial, or completion requests cannot reuse it.
func (c *ChattoCore) ConsumePendingOAuthAuthorize(ctx context.Context, token string) (PendingOAuthAuthorize, error) {
	pending, revision, err := c.getPendingOAuthAuthorize(ctx, token)
	if err != nil {
		return PendingOAuthAuthorize{}, err
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, c.pendingOAuthAuthorizeKey(token), jetstream.LastRevision(revision)); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) || isRuntimeStateRevisionConflict(err) {
			return PendingOAuthAuthorize{}, ErrPendingOAuthAuthorizeNotFound
		}
		return PendingOAuthAuthorize{}, fmt.Errorf("consume pending OAuth authorization request: %w", err)
	}
	return pending, nil
}

// DiscardPendingOAuthAuthorize removes an abandoned or superseded browser flow.
func (c *ChattoCore) DiscardPendingOAuthAuthorize(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	err := c.storage.runtimeStateKV.Delete(ctx, c.pendingOAuthAuthorizeKey(token))
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("discard pending OAuth authorization request: %w", err)
	}
	return nil
}

func (c *ChattoCore) getPendingOAuthAuthorize(ctx context.Context, token string) (PendingOAuthAuthorize, uint64, error) {
	if token == "" {
		return PendingOAuthAuthorize{}, 0, ErrPendingOAuthAuthorizeNotFound
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, c.pendingOAuthAuthorizeKey(token))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return PendingOAuthAuthorize{}, 0, ErrPendingOAuthAuthorizeNotFound
		}
		return PendingOAuthAuthorize{}, 0, fmt.Errorf("get pending OAuth authorization request: %w", err)
	}
	var pending PendingOAuthAuthorize
	if err := json.Unmarshal(entry.Value(), &pending); err != nil {
		return PendingOAuthAuthorize{}, 0, fmt.Errorf("unmarshal pending OAuth authorization request: %w", err)
	}
	if pending.RedirectURI == "" || pending.CodeChallenge == "" || pending.CodeChallengeMethod == "" || pending.CreatedAt.IsZero() || time.Since(pending.CreatedAt) > PendingOAuthAuthorizeTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.pendingOAuthAuthorizeKey(token))
		return PendingOAuthAuthorize{}, 0, ErrPendingOAuthAuthorizeNotFound
	}
	return pending, entry.Revision(), nil
}
