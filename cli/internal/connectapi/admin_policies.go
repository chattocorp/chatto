package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type adminPolicyService struct {
	api *API
}

func (s *adminPolicyService) GetPolicyConfiguration(ctx context.Context, req *connect.Request[adminv1.GetPolicyConfigurationRequest]) (*connect.Response[adminv1.GetPolicyConfigurationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := corePolicyScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	configuration, err := s.api.core.GetPolicyConfiguration(ctx, caller.UserID, scope)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetPolicyConfigurationResponse{PolicyConfiguration: apiPolicyConfiguration(configuration)}), nil
}

func (s *adminPolicyService) UpdatePolicyConfiguration(ctx context.Context, req *connect.Request[adminv1.UpdatePolicyConfigurationRequest]) (*connect.Response[adminv1.UpdatePolicyConfigurationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := corePolicyScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	mask := req.Msg.GetUpdateMask()
	if mask == nil || len(mask.GetPaths()) != 1 || mask.GetPaths()[0] != "author_edit_window_seconds" {
		return nil, connectError(core.ErrInvalidArgument)
	}
	overrides := core.PolicyOverrides{}
	if req.Msg.GetOverrides() != nil && req.Msg.GetOverrides().AuthorEditWindowSeconds != nil {
		value := req.Msg.GetOverrides().GetAuthorEditWindowSeconds()
		overrides.AuthorEditWindowSeconds = &value
	}
	configuration, err := s.api.core.UpdatePolicyConfiguration(ctx, caller.UserID, scope, overrides, core.PolicyUpdateMask{AuthorEditWindow: true})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdatePolicyConfigurationResponse{PolicyConfiguration: apiPolicyConfiguration(configuration)}), nil
}

func corePolicyScope(scope *adminv1.PolicyScope) (core.PolicyScope, error) {
	if scope == nil {
		return core.PolicyScope{}, core.ErrInvalidArgument
	}
	switch value := scope.GetScope().(type) {
	case *adminv1.PolicyScope_Server:
		if !value.Server {
			return core.PolicyScope{}, core.ErrInvalidArgument
		}
		return core.PolicyScope{Kind: core.PolicyScopeServer}, nil
	case *adminv1.PolicyScope_RoomGroupId:
		return core.PolicyScope{Kind: core.PolicyScopeRoomGroup, ID: value.RoomGroupId}, nil
	case *adminv1.PolicyScope_RoomId:
		return core.PolicyScope{Kind: core.PolicyScopeRoom, ID: value.RoomId}, nil
	default:
		return core.PolicyScope{}, core.ErrInvalidArgument
	}
}

func apiPolicyConfiguration(configuration core.PolicyConfiguration) *adminv1.PolicyConfiguration {
	overrides := &adminv1.PolicyOverrides{}
	if configuration.Overrides.AuthorEditWindowSeconds != nil {
		value := *configuration.Overrides.AuthorEditWindowSeconds
		overrides.AuthorEditWindowSeconds = &value
	}
	return &adminv1.PolicyConfiguration{
		Scope:     apiPolicyScope(configuration.Scope),
		Overrides: overrides,
		Effective: &apiv1.EffectivePolicies{AuthorEditWindowSeconds: configuration.Effective.AuthorEditWindowSeconds},
		Sources:   &apiv1.EffectivePolicySources{AuthorEditWindow: apiPolicySource(configuration.Sources.AuthorEditWindow)},
	}
}

func apiPolicyScope(scope core.PolicyScope) *adminv1.PolicyScope {
	switch scope.Kind {
	case core.PolicyScopeServer:
		return &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_Server{Server: true}}
	case core.PolicyScopeRoomGroup:
		return &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_RoomGroupId{RoomGroupId: scope.ID}}
	case core.PolicyScopeRoom:
		return &adminv1.PolicyScope{Scope: &adminv1.PolicyScope_RoomId{RoomId: scope.ID}}
	default:
		return &adminv1.PolicyScope{}
	}
}

func apiPolicySource(source core.PolicySource) *apiv1.EffectivePolicySource {
	scope := apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_UNSPECIFIED
	switch {
	case source.ProductDefault:
		scope = apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_PRODUCT_DEFAULT
	case source.Kind == core.PolicyScopeServer:
		scope = apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_SERVER
	case source.Kind == core.PolicyScopeRoomGroup:
		scope = apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_ROOM_GROUP
	case source.Kind == core.PolicyScopeRoom:
		scope = apiv1.PolicySourceScope_POLICY_SOURCE_SCOPE_ROOM
	}
	return &apiv1.EffectivePolicySource{Scope: scope, ScopeId: source.ID}
}
