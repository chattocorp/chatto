package model

// Viewer represents the current authenticated user's server-level
// permissions. UserID is the only internal field — every capability
// answer is computed by the field resolvers against the permission
// engine. We intentionally don't cache "is owner/admin" on the struct:
// the previous IsConfigOwner short-circuit predated the medium-option
// removal of admin.bypass, and now that owner/admin permissions are
// enumerated, the short-circuit was redundant with the resolver walk
// while being misleadingly named.
type Viewer struct {
	UserID string
}
