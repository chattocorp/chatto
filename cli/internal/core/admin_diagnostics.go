package core

import (
	"context"
	"fmt"
)

type AdminDiagnostics struct {
	Connection           *ConnectionInfo
	Account              *AccountInfo
	Stats                *ServerStats
	JetStream            *JetStreamStats
	Projections          []ProjectionAdminState
	ProjectionsAvailable bool
	AssetCleanup         AssetCleanupAdminStatus
	DurableWorkers       []DurableWorkerAdminStatus
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
		c.logger.Warn("Failed to read JetStream account diagnostics", "error", err)
		accountInfo = nil
	}
	stats, err := c.GetStats(ctx)
	if err != nil {
		c.logger.Warn("Failed to read server statistics", "error", err)
		stats = nil
	}
	jetStreamStats, err := c.GetJetStreamStats(ctx)
	if err != nil {
		c.logger.Warn("Failed to read JetStream diagnostics", "error", err)
		jetStreamStats = nil
	}
	projections, err := c.ProjectionAdminStates(ctx)
	projectionsAvailable := err == nil
	if err != nil {
		c.logger.Warn("Failed to read projection diagnostics", "error", err)
		projections = nil
	}
	assetCleanup, err := c.assetModel.AdminCleanupStatus(ctx)
	if err != nil {
		c.logger.Warn("Failed to read asset cleanup diagnostics", "error", err)
		assetCleanup = AssetCleanupAdminStatus{Health: AssetCleanupHealthUnavailable}
	}

	return &AdminDiagnostics{
		Connection:           c.GetConnectionInfo(),
		Account:              accountInfo,
		Stats:                stats,
		JetStream:            jetStreamStats,
		Projections:          projections,
		ProjectionsAvailable: projectionsAvailable,
		AssetCleanup:         assetCleanup,
		DurableWorkers:       durableWorkerAdminStatuses(jetStreamStats, c.VideoUploadsEnabled),
	}, nil
}
