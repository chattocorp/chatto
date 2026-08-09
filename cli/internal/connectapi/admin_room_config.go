package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type adminRoomConfigService struct {
	api *API
}

func (s *adminRoomConfigService) GetRoomConfig(ctx context.Context, req *connect.Request[adminv1.GetRoomConfigRequest]) (*connect.Response[adminv1.GetRoomConfigResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := coreRoomConfigScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	state, err := s.api.core.GetRoomConfig(ctx, caller.UserID, scope)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRoomConfigResponse{State: apiRoomConfigState(state)}), nil
}

func (s *adminRoomConfigService) UpdateRoomConfig(ctx context.Context, req *connect.Request[adminv1.UpdateRoomConfigRequest]) (*connect.Response[adminv1.UpdateRoomConfigResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := coreRoomConfigScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	mask := req.Msg.GetUpdateMask()
	if mask == nil || len(mask.GetPaths()) == 0 {
		return nil, connectError(core.ErrInvalidArgument)
	}
	coreMask := core.RoomConfigUpdateMask{}
	for _, path := range mask.GetPaths() {
		switch path {
		case "author_edit_window":
			coreMask.AuthorEditWindow = true
		default:
			return nil, connectError(core.ErrInvalidArgument)
		}
	}
	layer := core.RoomConfigLayer{}
	if req.Msg.GetLayer() != nil && req.Msg.GetLayer().AuthorEditWindow != nil {
		value := req.Msg.GetLayer().GetAuthorEditWindow()
		if err := value.CheckValid(); err != nil {
			return nil, connectError(core.ErrInvalidArgument)
		}
		window := value.AsDuration()
		layer.AuthorEditWindow = &window
	}
	state, err := s.api.core.UpdateRoomConfig(ctx, caller.UserID, scope, layer, coreMask)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdateRoomConfigResponse{State: apiRoomConfigState(state)}), nil
}

func coreRoomConfigScope(scope *adminv1.RoomConfigScope) (core.RoomConfigScope, error) {
	if scope == nil {
		return core.RoomConfigScope{}, core.ErrInvalidArgument
	}
	switch value := scope.GetScope().(type) {
	case *adminv1.RoomConfigScope_Server:
		if !value.Server {
			return core.RoomConfigScope{}, core.ErrInvalidArgument
		}
		return core.RoomConfigScope{Kind: core.RoomConfigScopeServer}, nil
	case *adminv1.RoomConfigScope_RoomGroupId:
		return core.RoomConfigScope{Kind: core.RoomConfigScopeRoomGroup, ID: value.RoomGroupId}, nil
	case *adminv1.RoomConfigScope_RoomId:
		return core.RoomConfigScope{Kind: core.RoomConfigScopeRoom, ID: value.RoomId}, nil
	default:
		return core.RoomConfigScope{}, core.ErrInvalidArgument
	}
}

func apiRoomConfigState(state core.RoomConfigState) *adminv1.RoomConfigState {
	layer := &adminv1.RoomConfigLayer{}
	if state.Layer.AuthorEditWindow != nil {
		layer.AuthorEditWindow = durationpb.New(*state.Layer.AuthorEditWindow)
	}
	return &adminv1.RoomConfigState{
		Layer:     layer,
		Effective: &apiv1.RoomConfig{AuthorEditWindow: durationpb.New(state.Effective.AuthorEditWindow)},
	}
}
