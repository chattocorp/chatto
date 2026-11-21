package core

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// FreshAuthReverificationTTL bounds the round trip through the provider.
	FreshAuthReverificationTTL = 15 * time.Minute

	freshAuthReverificationStartKeyPrefix = "fresh_auth_reverification_start."
	// freshAuthReverificationCookieScope separates the cookie digest in a
	// start record from every other HMAC of the same cookie handle.
	freshAuthReverificationCookieScope = "fresh_auth_reverification_cookie"
	// freshAuthReverificationClockSkew allows for the provider's whole-second
	// auth_time and small clock differences between servers.
	freshAuthReverificationClockSkew = 30 * time.Second
	freshAuthReverificationSource    = "external_identity_reverification"
)

var (
	// ErrFreshAuthReverificationMismatch reports a provider identity that is
	// not linked to the account that started the re-verification, or a
	// callback in a browser without the bound cookie session.
	ErrFreshAuthReverificationMismatch = errors.New("re-verified identity does not match the current account")
	// ErrFreshAuthReverificationUnavailable reports a provider that cannot
	// re-verify the user: it is not linked to the account, or it cannot force
	// a new provider sign-in.
	ErrFreshAuthReverificationUnavailable = errors.New("provider cannot re-verify this account")
	// ErrFreshAuthReverificationStale reports a provider sign-in that did not
	// happen during this re-verification, so it does not prove presence.
	ErrFreshAuthReverificationStale = errors.New("provider did not authenticate the user again")
)

// FreshAuthReverificationStart is a consumed re-verification handoff. The
// credential binding stays unexported so that only
// CompleteFreshAuthReverification can use it.
type FreshAuthReverificationStart struct {
	// ProviderID is the configured provider that must re-verify the user.
	ProviderID string
	// RedirectPath is the internal path to return to after the callback.
	RedirectPath string
	// BoundUserID is the account that the provider identity must belong to.
	BoundUserID string
	// CreatedAt is when the start RPC ran. Provider authentication must be
	// at or after this time.
	CreatedAt time.Time

	record *runtimestatev1.FreshAuthReverificationStart
}

func (c *ChattoCore) freshAuthReverificationStartKey(token string) string {
	return c.runtimeTokenKey(freshAuthReverificationStartKeyPrefix, token)
}

// CreateFreshAuthReverificationStart stores a one-time handoff that binds a
// provider re-verification to the presented cookie session. It returns the raw
// handoff token for the provider start URL. Bearer credentials cannot
// re-verify: the callback browser cannot prove that it holds them, so a
// leaked bearer token could gain freshness from a phished provider sign-in.
func (c *ChattoCore) CreateFreshAuthReverificationStart(ctx context.Context, providerID, redirectPath, cookieHandle string) (string, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return "", ErrInvalidArgument
	}
	credential, err := c.validateRuntimeCredential(ctx, cookieHandle, AuthTokenPresentationCookie, authoritativeCredentialRead)
	if err != nil {
		return "", err
	}
	if credential.Kind != AuthTokenKindFirstPartySession {
		return "", ErrFreshAuthRequired
	}
	if err := c.requireHumanUser(ctx, credential.UserID); err != nil {
		return "", err
	}
	record := &runtimestatev1.FreshAuthReverificationStart{
		ProviderId:          providerID,
		RedirectPath:        strings.TrimSpace(redirectPath),
		BoundUserId:         credential.UserID,
		CookieSessionDigest: c.runtimeTokenHash(freshAuthReverificationCookieScope, cookieHandle),
		CreatedAt:           timestamppb.Now(),
	}
	data, err := proto.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("marshal fresh-auth re-verification start: %w", err)
	}
	token := NewFreshAuthReverificationStartToken()
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.freshAuthReverificationStartKey(token), data, jetstream.KeyTTL(FreshAuthReverificationTTL)); err != nil {
		return "", fmt.Errorf("store fresh-auth re-verification start: %w", err)
	}
	return token, nil
}

// PeekFreshAuthReverificationStart reads a handoff without consuming it. The
// provider start uses it to reject an unknown token before the redirect.
func (c *ChattoCore) PeekFreshAuthReverificationStart(ctx context.Context, token string) (*FreshAuthReverificationStart, error) {
	start, _, err := c.loadFreshAuthReverificationStart(ctx, token)
	return start, err
}

// ConsumeFreshAuthReverificationStart reads and deletes a handoff. A token
// completes at most one re-verification.
func (c *ChattoCore) ConsumeFreshAuthReverificationStart(ctx context.Context, token string) (*FreshAuthReverificationStart, error) {
	start, entry, err := c.loadFreshAuthReverificationStart(ctx, token)
	if err != nil {
		return nil, err
	}
	// The revision check lets only one concurrent callback consume the token.
	if err := c.storage.runtimeStateKV.Delete(ctx, entry.Key(), jetstream.LastRevision(entry.Revision())); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || jetstreamutil.IsSequenceConflict(err) {
			return nil, ErrExternalIdentityFlowNotFound
		}
		return nil, fmt.Errorf("delete fresh-auth re-verification start: %w", err)
	}
	return start, nil
}

func (c *ChattoCore) loadFreshAuthReverificationStart(ctx context.Context, token string) (*FreshAuthReverificationStart, jetstream.KeyValueEntry, error) {
	if token == "" {
		return nil, nil, ErrExternalIdentityFlowNotFound
	}
	key := c.freshAuthReverificationStartKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil, ErrExternalIdentityFlowNotFound
		}
		return nil, nil, fmt.Errorf("get fresh-auth re-verification start: %w", err)
	}
	var record runtimestatev1.FreshAuthReverificationStart
	if err := proto.Unmarshal(entry.Value(), &record); err != nil || record.GetBoundUserId() == "" || record.GetProviderId() == "" || record.GetCookieSessionDigest() == "" || record.GetCreatedAt() == nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, nil, ErrExternalIdentityFlowNotFound
	}
	createdAt := record.GetCreatedAt().AsTime()
	if time.Since(createdAt) > FreshAuthReverificationTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, nil, ErrExternalIdentityFlowExpired
	}
	return &FreshAuthReverificationStart{
		ProviderID:   record.GetProviderId(),
		RedirectPath: record.GetRedirectPath(),
		BoundUserID:  record.GetBoundUserId(),
		CreatedAt:    createdAt,
		record:       &record,
	}, entry, nil
}

// CompleteFreshAuthReverification marks the bound cookie session fresh after
// a provider verified issuer and subject with a sign-in at authTime. The
// identity must belong to the bound account, authTime must not be earlier
// than the start, and cookieHandle, the cookie session that the callback
// request presented, must be the bound one. It never creates a session.
func (c *ChattoCore) CompleteFreshAuthReverification(ctx context.Context, start *FreshAuthReverificationStart, issuer, subject string, authTime time.Time, cookieHandle string) error {
	if start == nil || start.record == nil {
		return ErrExternalIdentityFlowNotFound
	}
	user, _, err := c.GetUserByExternalIdentityForAuthentication(ctx, issuer, subject)
	if err != nil {
		return err
	}
	if user == nil || user.Id != start.BoundUserID {
		return ErrFreshAuthReverificationMismatch
	}
	if authTime.IsZero() || authTime.Before(start.CreatedAt.Add(-freshAuthReverificationClockSkew)) {
		return ErrFreshAuthReverificationStale
	}
	if cookieHandle == "" || !hmac.Equal(
		[]byte(c.runtimeTokenHash(freshAuthReverificationCookieScope, cookieHandle)),
		[]byte(start.record.GetCookieSessionDigest()),
	) {
		return ErrFreshAuthReverificationMismatch
	}
	credential, err := c.validateRuntimeCredential(ctx, cookieHandle, AuthTokenPresentationCookie, authoritativeCredentialRead)
	if err != nil {
		return err
	}
	if credential.UserID != start.BoundUserID {
		return ErrFreshAuthReverificationMismatch
	}
	return c.MarkCookieSessionFresh(ctx, cookieHandle, "external_identity", freshAuthReverificationSource)
}
