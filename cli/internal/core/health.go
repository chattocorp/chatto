package core

import (
	"context"
	"fmt"
	"strings"
)

// SpaceRBACHealthReport contains the results of the RBAC health check.
type SpaceRBACHealthReport struct {
	SpacesChecked     int
	SpacesInitialized int      // Spaces that needed RBAC initialization
	Errors            []string // Any errors encountered (non-fatal)
}

// SpaceRBACHealthCheck ensures all spaces have their RBAC defaults initialized.
// This is idempotent and safe to run from multiple replicas simultaneously
// (CreateDefaultRoles uses CreateRoleWithPosition which ignores ErrRoleAlreadyExists).
func (c *ChattoCore) SpaceRBACHealthCheck(ctx context.Context) (*SpaceRBACHealthReport, error) {
	report := &SpaceRBACHealthReport{}

	// Get all space IDs from INSTANCE bucket
	spaceIDs, err := c.listAllSpaceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list spaces: %w", err)
	}

	for _, spaceID := range spaceIDs {
		// Skip DM space - it doesn't use standard RBAC
		if IsDMSpace(spaceID) {
			continue
		}

		report.SpacesChecked++

		// Get the RBAC engine (creates bucket if needed)
		engine := c.storage.serverRBACEngine

		// Check if owner role exists (indicates bucket is initialized)
		exists, err := engine.RoleExists(ctx, RoleOwner)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Sprintf("space %s: failed to check owner role: %v", spaceID, err))
			continue
		}

		if !exists {
			c.logger.Info("Initializing RBAC defaults for space", "space_id", spaceID)
			// CreateDefaultRoles is idempotent (ignores ErrRoleAlreadyExists)
			if err := c.CreateDefaultRoles(ctx); err != nil {
				report.Errors = append(report.Errors,
					fmt.Sprintf("space %s: failed to create default roles: %v", spaceID, err))
				continue
			}
			report.SpacesInitialized++
		}
	}

	return report, nil
}

// listAllSpaceIDs returns all space IDs from the INSTANCE bucket.
func (c *ChattoCore) listAllSpaceIDs(ctx context.Context) ([]string, error) {
	keyLister, err := c.storage.serverKV.ListKeysFiltered(ctx, "space.*")
	if err != nil {
		return nil, err
	}

	var spaceIDs []string
	for key := range keyLister.Keys() {
		// Key format: space.{spaceID}
		parts := strings.SplitN(key, ".", 2)
		if len(parts) == 2 {
			spaceIDs = append(spaceIDs, parts[1])
		}
	}
	return spaceIDs, nil
}
