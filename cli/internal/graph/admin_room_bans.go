package graph

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func roomBanToModel(ban core.RoomBan) *model.RoomBan {
	var expiresAt *timestamppb.Timestamp
	if ban.ExpiresAt != nil {
		expiresAt = timestamppb.New(*ban.ExpiresAt)
	}
	return &model.RoomBan{
		ID:          ban.EventID,
		RoomID:      ban.RoomID,
		UserID:      ban.UserID,
		ModeratorID: ban.ModeratorID,
		Reason:      ban.Reason,
		CreatedAt:   timestamppb.New(ban.CreatedAt),
		ExpiresAt:   expiresAt,
	}
}
