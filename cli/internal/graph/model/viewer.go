package model

// Viewer represents the current authenticated user's instance-level permissions.
// UserID and IsConfigOwner are internal fields used by field resolvers.
type Viewer struct {
	UserID        string
	IsConfigOwner bool
}
