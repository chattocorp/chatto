package core

import (
	"slices"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/config"
)

// MaxAtprotoDIDLength bounds an AT Protocol DID or handle hint. The AT
// Protocol specification limits DIDs to 2 KB.
const MaxAtprotoDIDLength = 2048

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
