package connectapi

import (
	"connectrpc.com/connect"
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func apiReadMarker(marker *core.ReadMarker) *apiv1.ReadMarker {
	if marker == nil {
		return nil
	}
	result := &apiv1.ReadMarker{LastReadEventId: marker.EventID}
	if !marker.LastReadAt.IsZero() {
		result.LastReadAt = timestamppb.New(marker.LastReadAt)
	}
	return result
}

func (s *roomService) GetRoomReadState(ctx context.Context, req *connect.Request[apiv1.GetRoomReadStateRequest]) (*connect.Response[apiv1.GetRoomReadStateResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	marker, err := s.api.core.ReadState().GetRoomReadMarker(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRoomReadStateResponse{State: &apiv1.RoomReadState{RoomId: req.Msg.GetRoomId(), Marker: apiReadMarker(marker)}}), nil
}

func (s *roomService) BatchGetRoomReadStates(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomReadStatesRequest]) (*connect.Response[apiv1.BatchGetRoomReadStatesResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	result := &apiv1.BatchGetRoomReadStatesResponse{}
	for _, target := range req.Msg.GetRoomIds() {
		key := target
		if seen[key] {
			continue
		}
		seen[key] = true
		response, err := s.GetRoomReadState(ctx, connect.NewRequest(&apiv1.GetRoomReadStateRequest{RoomId: target}))
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound || connect.CodeOf(err) == connect.CodePermissionDenied {
				continue
			}
			return nil, err
		}
		result.States = append(result.States, response.Msg.State)
	}
	return connect.NewResponse(result), nil
}

func (s *threadService) GetThreadReadState(ctx context.Context, req *connect.Request[apiv1.GetThreadReadStateRequest]) (*connect.Response[apiv1.GetThreadReadStateResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	marker, err := s.api.core.ReadState().GetThreadReadMarker(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetThreadRootEventId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetThreadReadStateResponse{State: &apiv1.ThreadReadState{RoomId: req.Msg.GetRoomId(), ThreadRootEventId: req.Msg.GetThreadRootEventId(), Marker: apiReadMarker(marker)}}), nil
}

func (s *threadService) BatchGetThreadReadStates(ctx context.Context, req *connect.Request[apiv1.BatchGetThreadReadStatesRequest]) (*connect.Response[apiv1.BatchGetThreadReadStatesResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}
	seen := make(map[[2]string]bool)
	result := &apiv1.BatchGetThreadReadStatesResponse{}
	for _, target := range req.Msg.GetTargets() {
		key := [2]string{target.GetRoomId(), target.GetThreadRootEventId()}
		if seen[key] {
			continue
		}
		seen[key] = true
		response, err := s.GetThreadReadState(ctx, connect.NewRequest(&apiv1.GetThreadReadStateRequest{RoomId: target.GetRoomId(), ThreadRootEventId: target.GetThreadRootEventId()}))
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound || connect.CodeOf(err) == connect.CodePermissionDenied {
				continue
			}
			return nil, err
		}
		result.States = append(result.States, response.Msg.State)
	}
	return connect.NewResponse(result), nil
}
