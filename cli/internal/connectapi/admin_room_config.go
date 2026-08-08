package connectapi

import (
	"context"

	"connectrpc.com/connect"
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
		case "author_edit_window_seconds":
			coreMask.AuthorEditWindow = true
		default:
			return nil, connectError(core.ErrInvalidArgument)
		}
	}
	layer := core.RoomConfigLayer{}
	if req.Msg.GetLayer() != nil && req.Msg.GetLayer().AuthorEditWindowSeconds != nil {
		value := req.Msg.GetLayer().GetAuthorEditWindowSeconds()
		layer.AuthorEditWindowSeconds = &value
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
	if state.Layer.AuthorEditWindowSeconds != nil {
		value := *state.Layer.AuthorEditWindowSeconds
		layer.AuthorEditWindowSeconds = &value
	}
	return &adminv1.RoomConfigState{
		Scope:     apiRoomConfigScope(state.Scope),
		Layer:     layer,
		Effective: &apiv1.RoomConfig{AuthorEditWindowSeconds: state.Effective.AuthorEditWindowSeconds},
		Sources:   &apiv1.RoomConfigSources{AuthorEditWindow: apiRoomConfigSource(state.Sources.AuthorEditWindow)},
	}
}

func apiRoomConfigScope(scope core.RoomConfigScope) *adminv1.RoomConfigScope {
	switch scope.Kind {
	case core.RoomConfigScopeServer:
		return &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_Server{Server: true}}
	case core.RoomConfigScopeRoomGroup:
		return &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_RoomGroupId{RoomGroupId: scope.ID}}
	case core.RoomConfigScopeRoom:
		return &adminv1.RoomConfigScope{Scope: &adminv1.RoomConfigScope_RoomId{RoomId: scope.ID}}
	default:
		return &adminv1.RoomConfigScope{}
	}
}

func apiRoomConfigSource(source core.RoomConfigSource) *apiv1.RoomConfigSource {
	result := &apiv1.RoomConfigSource{}
	switch {
	case source.ProductDefault:
		result.Source = &apiv1.RoomConfigSource_ProductDefault{ProductDefault: true}
	case source.Kind == core.RoomConfigScopeServer:
		result.Source = &apiv1.RoomConfigSource_Server{Server: true}
	case source.Kind == core.RoomConfigScopeRoomGroup:
		result.Source = &apiv1.RoomConfigSource_RoomGroupId{RoomGroupId: source.ID}
	case source.Kind == core.RoomConfigScopeRoom:
		result.Source = &apiv1.RoomConfigSource_RoomId{RoomId: source.ID}
	}
	return result
}
