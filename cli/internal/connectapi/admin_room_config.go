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
	config := core.RoomConfig{}
	if req.Msg.GetConfig() != nil && req.Msg.GetConfig().AuthorEditWindow != nil {
		value := req.Msg.GetConfig().GetAuthorEditWindow()
		if err := value.CheckValid(); err != nil {
			return nil, connectError(core.ErrInvalidArgument)
		}
		window := value.AsDuration()
		config.AuthorEditWindow = &window
	}
	state, err := s.api.core.UpdateRoomConfig(ctx, caller.UserID, scope, config, req.Msg.GetUpdateMask())
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
	return &adminv1.RoomConfigState{
		Layer:     apiRoomConfig(state.Layer),
		Effective: apiRoomConfig(state.Effective),
	}
}

func apiRoomConfig(config core.RoomConfig) *apiv1.RoomConfig {
	result := &apiv1.RoomConfig{}
	if config.AuthorEditWindow != nil {
		result.AuthorEditWindow = durationpb.New(*config.AuthorEditWindow)
	}
	return result
}
