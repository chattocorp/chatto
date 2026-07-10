package core

import (
	"context"
	"fmt"
)

type AdminDiagnostics struct {
	Connection   *ConnectionInfo
	Account      *AccountInfo
	Stats        *ServerStats
	JetStream    *JetStreamStats
	Projections  []ProjectionAdminState
	AssetCleanup AssetCleanupAdminStatus
}

func (c *ChattoCore) GetAdminDiagnostics(ctx context.Context, actorID string) (*AdminDiagnostics, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	isOwner, err := c.IsServerOwner(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("check owner role: %w", err)
	}
	if !isOwner {
		return nil, ErrPermissionDenied
	}

	accountInfo, err := c.GetAccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get account info: %w", err)
	}
	stats, err := c.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get server stats: %w", err)
	}
	jetStreamStats, err := c.GetJetStreamStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get NATS stats: %w", err)
	}
	projections, err := c.ProjectionAdminStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("projection states: %w", err)
	}
	assetCleanup, err := c.assetLifecycle().AdminCleanupStatus(ctx)
	if err != nil {
		c.logger.Warn("Failed to read asset cleanup diagnostics", "error", err)
		assetCleanup = AssetCleanupAdminStatus{Health: AssetCleanupHealthUnavailable}
	}

	return &AdminDiagnostics{
		Connection:   c.GetConnectionInfo(),
		Account:      accountInfo,
		Stats:        stats,
		JetStream:    jetStreamStats,
		Projections:  projections,
		AssetCleanup: assetCleanup,
	}, nil
}
