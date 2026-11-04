package connectapi

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// assembleRoomGroup returns the same resource as the main directory reads.
// ArchiveAll makes a manager's successful layout mutation useful for refresh.
func (s *adminRoomLayoutService) assembleRoomGroup(ctx context.Context, userID, groupID string) (*apiv1.RoomGroup, error) {
	group, err := s.api.core.RoomDirectoryReads().GetRoomGroup(ctx, userID, groupID, core.RoomDirectoryGroupOptions{ArchiveFilter: core.RoomArchiveAll})
	if err != nil {
		return nil, err
	}
	return apiRoomGroup(group), nil
}
