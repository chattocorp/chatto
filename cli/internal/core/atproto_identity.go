package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/nats-io/nats.go/jetstream"
)

// atprotoServiceTokenKeyPrefix stores the IDs of AT Protocol service tokens
// that a sign-in already used. Each record expires with its token.
const atprotoServiceTokenKeyPrefix = "atproto_service_token."

// ErrAtprotoServiceTokenReplayed reports a service token that a sign-in already
// used, on this or another replica.
var ErrAtprotoServiceTokenReplayed = errors.New("AT Protocol service token was already used")

// AtprotoDIDForUser returns the DID of the AT Protocol identity that userID
// linked for sign-in, or "" without one. The public user resource publishes
// this DID; clients verify it against the user's account link record
// (FDR-056). With several such identities, the one of the lowest provider ID
// wins, so the result is stable.
func (c *ChattoCore) AtprotoDIDForUser(userID string) string {
	identities := slices.Clone(c.userModel.externalIdentities(userID))
	slices.SortFunc(identities, func(a, b ExternalIdentity) int {
		if n := strings.Compare(a.ProviderID, b.ProviderID); n != 0 {
			return n
		}
		return strings.Compare(a.SubjectHash, b.SubjectHash)
	})
	for _, identity := range identities {
		if identity.ProviderType == config.AuthProviderTypeATProto && strings.HasPrefix(identity.Subject, "did:") {
			return identity.Subject
		}
	}
	return ""
}

// ClaimAtprotoServiceToken records the token ID jti until expiresAt, so the
// cluster accepts each service token at most once. A token that a replica is
// still recording also counts as used: rejection is the safe outcome.
func (c *ChattoCore) ClaimAtprotoServiceToken(ctx context.Context, jti string, expiresAt time.Time) error {
	if strings.TrimSpace(jti) == "" {
		return fmt.Errorf("%w: service token has no ID", ErrInvalidArgument)
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return fmt.Errorf("%w: service token expired", ErrInvalidArgument)
	}
	// Keep the record a little longer than the token, for clock skew.
	ttl += time.Minute
	_, err := c.storage.runtimeStateKV.Create(ctx, c.runtimeTokenKey(atprotoServiceTokenKeyPrefix, jti), []byte{1}, jetstream.KeyTTL(ttl))
	if err == nil {
		return nil
	}
	if errors.Is(err, jetstream.ErrKeyExists) || jetstreamutil.IsSequenceConflict(err) {
		return ErrAtprotoServiceTokenReplayed
	}
	return fmt.Errorf("record AT Protocol service token: %w", err)
}
