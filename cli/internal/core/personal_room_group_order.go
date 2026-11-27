package core

import (
	"context"
	"slices"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// SetPersonalRoomGroupOrder replaces the authenticated user's personal order.
// Unknown IDs remain inert. An empty list restores server order. Writes use
// the user's config aggregate and wait for the local config projection.
func (c *ChattoCore) SetPersonalRoomGroupOrder(ctx context.Context, actorID string, groupIDs []string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	if err := validatePersonalRoomGroupOrder(groupIDs); err != nil {
		return err
	}
	ids := slices.Clone(groupIDs)
	return c.configModel.updateSubject(ctx, actorID, func(_ evtstream.Aggregate, _ string, _ uint64) ([]*evtv1.Event, error) {
		if slices.Equal(c.configModel.config.Projection().RoomGroupOrder(actorID), ids) {
			return nil, nil
		}
		return []*evtv1.Event{newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserRoomGroupOrderChanged{
			UserRoomGroupOrderChanged: &evtv1.UserRoomGroupOrderChangedEvent{UserId: actorID, GroupIds: ids},
		}})}, nil
	})
}

func validatePersonalRoomGroupOrder(ids []string) error {
	if len(ids) > 1000 {
		return invalidArgument("room group order exceeds 1000 groups")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if len(id) == 0 || len(id) > 128 {
			return invalidArgument("room group ID must contain 1 to 128 bytes")
		}
		if seen[id] {
			return invalidArgument("room group order contains duplicate IDs")
		}
		seen[id] = true
	}
	return nil
}

// mergePersonalRoomGroupOrder only returns groups from the authorized directory.
// Missing IDs are skipped and unlisted groups retain their server order.
func mergePersonalRoomGroupOrder(groups []*DirectoryRoomGroup, ids []string) []*DirectoryRoomGroup {
	byID := make(map[string]*DirectoryRoomGroup, len(groups))
	for _, group := range groups {
		byID[group.Group.Id] = group
	}
	result := make([]*DirectoryRoomGroup, 0, len(groups))
	for _, id := range ids {
		if group, ok := byID[id]; ok {
			result = append(result, group)
			delete(byID, id)
		}
	}
	for _, group := range groups {
		if _, ok := byID[group.Group.Id]; ok {
			result = append(result, group)
			delete(byID, group.Group.Id)
		}
	}
	return result
}
