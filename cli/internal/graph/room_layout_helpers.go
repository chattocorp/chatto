package graph

import (
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// protoLayoutToModel converts a proto RoomLayout to the GraphQL model,
// attaching the pre-resolved room map (all rooms in the space) so
// sub-resolvers can efficiently resolve room IDs to Room objects.
func protoLayoutToModel(layout *corev1.RoomLayout, viewerRooms map[string]*corev1.Room) *model.RoomLayoutModel {
	sets := make([]*model.RoomSetModel, len(layout.Sets))
	for i, s := range layout.Sets {
		sets[i] = &model.RoomSetModel{
			ID:          s.Id,
			Name:        s.Name,
			Description: s.Description,
			RoomIds:     s.RoomIds,
			ViewerRooms: viewerRooms,
		}
	}

	return &model.RoomLayoutModel{
		Sets:        sets,
		ViewerRooms: viewerRooms,
	}
}
