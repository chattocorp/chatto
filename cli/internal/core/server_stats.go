package core

import (
	"context"
	"fmt"
)

// ============================================================================
// Statistics
// ============================================================================

// ServerStats contains aggregate counts surfaced in the admin dashboard.
type ServerStats struct {
	UserCount        int
	ChannelRoomCount int
	DMRoomCount      int
}

// GetStats returns deployment-level counts: registered users, channel rooms,
// DM rooms. Per-space breakdowns went away with the Space tier (ADR-030).
func (c *ChattoCore) GetStats(ctx context.Context) (*ServerStats, error) {
	stats := &ServerStats{}
	stats.UserCount = c.userModel.userCount()

	channelRooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to list channel rooms: %w", err)
	}
	stats.ChannelRoomCount = len(channelRooms)

	dmRooms, err := c.ListRooms(ctx, KindDM)
	if err != nil {
		return nil, fmt.Errorf("failed to list dm rooms: %w", err)
	}
	stats.DMRoomCount = len(dmRooms)

	return stats, nil
}
