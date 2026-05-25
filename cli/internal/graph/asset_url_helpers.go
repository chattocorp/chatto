package graph

import (
	"context"

	"hmans.de/chatto/internal/graph/auth"
)

// callerID returns the authenticated user's ID from the GraphQL
// context, or "" if no user is attached. Used by attachment URL
// resolvers to bake the caller's identity into the signed URL.
//
// Lives outside events.resolvers.go so gqlgen's regeneration of
// resolver files doesn't strip it — the codegen rewrites resolver
// files and drops free-standing helpers that don't match its
// expected shape.
func callerID(ctx context.Context) string {
	if u := auth.ForContext(ctx); u != nil {
		return u.Id
	}
	return ""
}
