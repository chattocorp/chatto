package core

// MaxAtprotoDIDLength bounds an AT Protocol DID or handle hint. The AT
// Protocol specification limits DIDs to 2 KB.
const MaxAtprotoDIDLength = 2048

// AtprotoDIDForUser returns the DID of the AT Protocol identity that userID
// linked for sign-in, or "" without one. The public user resource publishes
// this DID; clients verify it against the user's account link record
// (FDR-056).
func (c *ChattoCore) AtprotoDIDForUser(userID string) string {
	return c.userModel.auth.Projection().AtprotoDID(userID)
}
