package connectapi

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

type operatorRoomService struct {
	api *API
}

func (s *operatorRoomService) ListRooms(ctx context.Context, req *connect.Request[operatorv1.ListRoomsRequest]) (*connect.Response[operatorv1.ListRoomsResponse], error) {
	rooms, err := s.api.core.ListRooms(ctx, core.KindChannel)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 20, 100)
	selected, total, more := operatorRoomPage(rooms, req.Msg.GetName(), limit, offset)
	result := make([]*apiv1.Room, 0, len(selected))
	for _, room := range selected {
		result = append(result, apiRoom(room))
	}
	return connect.NewResponse(&operatorv1.ListRoomsResponse{
		Rooms: result,
		Page:  apiPageInfo(total, more),
	}), nil
}

func (s *operatorRoomService) CreateRoom(ctx context.Context, req *connect.Request[operatorv1.CreateRoomRequest]) (*connect.Response[operatorv1.CreateRoomResponse], error) {
	room, err := s.api.core.CreateOperatorRoom(ctx, core.OperatorRoomCreateInput{
		Name: req.Msg.GetName(), Description: req.Msg.GetDescription(), GroupID: req.Msg.GetGroupId(),
		Source: req.Msg.GetSource(), SourceID: req.Msg.GetSourceId(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.CreateRoomResponse{Room: apiRoom(room)}), nil
}

// operatorRoomPage retains every exact-name match, including archived rooms,
// and applies offsets only after ordering by stable room ID.
func operatorRoomPage(rooms []*evtv1.Room, name string, limit, offset int) ([]*evtv1.Room, int, bool) {
	filtered := make([]*evtv1.Room, 0, len(rooms))
	for _, room := range rooms {
		if name == "" || room.GetName() == name {
			filtered = append(filtered, room)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].GetId() < filtered[j].GetId() })
	return apiSlicePage(filtered, limit, offset)
}
