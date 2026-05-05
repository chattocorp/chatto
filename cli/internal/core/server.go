package core

import (
	"context"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ResolvePrimarySpaceID returns the space ID to treat as this deployment's
// primary (future Server) for the migration described in ADR-027 and issue #330.
//
// The configuredID argument comes from config.ServerConfig.PrimarySpaceID
// (env: CHATTO_SERVER_PRIMARY_SPACE_ID).
//
// Resolution rules:
//
//   - If configuredID is set, the named space must exist; we return its ID.
//     A configured-but-missing primary is a faulty config and the caller (run.go)
//     fails the boot rather than silently picking something else.
//   - If configuredID is unset and there are zero user-facing spaces, returns
//     ("", nil). This is the fresh-install case — no primary exists yet, and
//     that's not an error.
//   - If configuredID is unset and there is exactly one user-facing space,
//     returns its ID (auto-derive). Covers the common single-space upgrade
//     path with no operator action.
//   - If configuredID is unset and there are two or more user-facing spaces,
//     returns an error: the operator must pick one explicitly.
//
// "User-facing" here means: excluding the well-known DM hidden space
// (DMSpaceID), which is created at core initialization for every deployment
// and would otherwise mask the fresh-install case.
func (c *ChattoCore) ResolvePrimarySpaceID(ctx context.Context, configuredID string) (string, error) {
	if configuredID != "" {
		if _, err := c.GetSpace(ctx, configuredID); err != nil {
			return "", fmt.Errorf("configured server.primary_space_id %q does not exist: %w", configuredID, err)
		}
		return configuredID, nil
	}

	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list spaces while resolving primary space: %w", err)
	}

	userFacing := userFacingSpaces(spaces)
	switch len(userFacing) {
	case 0:
		return "", nil
	case 1:
		return userFacing[0].Id, nil
	default:
		ids := make([]string, 0, len(userFacing))
		for _, s := range userFacing {
			ids = append(ids, s.Id)
		}
		return "", fmt.Errorf("multiple spaces present (%v) and server.primary_space_id is unset; set it explicitly to one of these IDs (see ADR-027)", ids)
	}
}

// JoinPrimarySpaceIfAvailable joins the user to the deployment's primary space
// (per ADR-027 / issue #330). Used by signup flows so a newly-created user is a
// server member by default — a "server" is just the primary space dressed up.
//
// configuredID is the operator-configured server.primary_space_id (may be empty,
// in which case the resolver auto-derives or treats the install as fresh).
//
// Best-effort by design: if no primary space resolves yet (fresh install) or the
// resolver errors transiently, we log and continue rather than failing the
// signup. Membership is reconciled later when the operator sets the primary, or
// in the phase-4 data migration. JoinSpace is idempotent so retrying is safe.
func (c *ChattoCore) JoinPrimarySpaceIfAvailable(ctx context.Context, userID, configuredID string) {
	primaryID, err := c.ResolvePrimarySpaceID(ctx, configuredID)
	if err != nil {
		c.logger.Warn("auto-join primary space skipped: resolver error", "user_id", userID, "error", err)
		return
	}
	if primaryID == "" {
		return
	}
	if _, err := c.JoinSpace(ctx, userID, primaryID); err != nil {
		c.logger.Warn("auto-join primary space failed", "user_id", userID, "space_id", primaryID, "error", err)
	}
}

func userFacingSpaces(spaces []*corev1.Space) []*corev1.Space {
	out := make([]*corev1.Space, 0, len(spaces))
	for _, s := range spaces {
		if IsDMSpace(s.Id) {
			continue
		}
		out = append(out, s)
	}
	return out
}
