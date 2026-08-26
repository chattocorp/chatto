package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type notificationPolicyService struct {
	api *API
}

func coreNotificationPolicyScope(scope *apiv1.NotificationPolicyScope) (core.NotificationPolicyScope, error) {
	if scope == nil {
		return core.NotificationPolicyScope{}, core.ErrInvalidArgument
	}
	switch value := scope.GetScope().(type) {
	case *apiv1.NotificationPolicyScope_Server:
		return core.NotificationPolicyScope{Kind: core.NotificationPolicyScopeServer}, nil
	case *apiv1.NotificationPolicyScope_RoomGroupId:
		return core.NotificationPolicyScope{Kind: core.NotificationPolicyScopeRoomGroup, ID: value.RoomGroupId}, nil
	case *apiv1.NotificationPolicyScope_RoomId:
		return core.NotificationPolicyScope{Kind: core.NotificationPolicyScopeRoom, ID: value.RoomId}, nil
	default:
		return core.NotificationPolicyScope{}, core.ErrInvalidArgument
	}
}

func apiNotificationPolicyScope(scope core.NotificationPolicyScope) (*apiv1.NotificationPolicyScope, error) {
	switch scope.Kind {
	case core.NotificationPolicyScopeServer:
		return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_Server{Server: &emptypb.Empty{}}}, nil
	case core.NotificationPolicyScopeRoomGroup:
		return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_RoomGroupId{RoomGroupId: scope.ID}}, nil
	case core.NotificationPolicyScopeRoom:
		return &apiv1.NotificationPolicyScope{Scope: &apiv1.NotificationPolicyScope_RoomId{RoomId: scope.ID}}, nil
	default:
		return nil, core.ErrInvalidArgument
	}
}

func apiScopedNotificationPolicy(policy *core.NotificationPolicy) (*apiv1.ScopedNotificationPolicy, error) {
	scope, err := apiNotificationPolicyScope(policy.Scope)
	if err != nil {
		return nil, err
	}
	mapped, err := apiNotificationPolicy(policy)
	if err != nil {
		return nil, err
	}
	return &apiv1.ScopedNotificationPolicy{Scope: scope, Policy: mapped}, nil
}

func (s *notificationPolicyService) GetNotificationPolicy(ctx context.Context, req *connect.Request[apiv1.NotificationPolicyServiceGetNotificationPolicyRequest]) (*connect.Response[apiv1.NotificationPolicyServiceGetNotificationPolicyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := coreNotificationPolicyScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	policy, err := s.api.core.NotificationPolicy().GetScopedNotificationPolicy(ctx, caller.UserID, scope)
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiScopedNotificationPolicy(policy)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.NotificationPolicyServiceGetNotificationPolicyResponse{Policy: mapped}), nil
}

func (s *notificationPolicyService) BatchGetNotificationPolicies(ctx context.Context, req *connect.Request[apiv1.BatchGetNotificationPoliciesRequest]) (*connect.Response[apiv1.BatchGetNotificationPoliciesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]core.NotificationPolicyScope, 0, len(req.Msg.GetScopes()))
	for _, requested := range req.Msg.GetScopes() {
		scope, err := coreNotificationPolicyScope(requested)
		if err != nil {
			return nil, connectError(err)
		}
		scopes = append(scopes, scope)
	}
	policies, err := s.api.core.NotificationPolicy().BatchGetNotificationPolicies(ctx, caller.UserID, scopes)
	if err != nil {
		return nil, connectError(err)
	}
	result := make([]*apiv1.ScopedNotificationPolicy, 0, len(policies))
	for _, policy := range policies {
		mapped, err := apiScopedNotificationPolicy(policy)
		if err != nil {
			return nil, connectError(err)
		}
		result = append(result, mapped)
	}
	return connect.NewResponse(&apiv1.BatchGetNotificationPoliciesResponse{Policies: result}), nil
}

func (s *notificationPolicyService) UpdateNotificationPolicy(ctx context.Context, req *connect.Request[apiv1.NotificationPolicyServiceUpdateNotificationPolicyRequest]) (*connect.Response[apiv1.NotificationPolicyServiceUpdateNotificationPolicyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := coreNotificationPolicyScope(req.Msg.GetScope())
	if err != nil {
		return nil, connectError(err)
	}
	overrides, err := coreNotificationDeliveryModes(req.Msg.GetOverrides())
	if err != nil {
		return nil, connectError(err)
	}
	policy, err := s.api.core.NotificationPolicy().UpdateScopedNotificationPolicy(ctx, caller.UserID, scope, overrides, req.Msg.GetUpdateMask())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiScopedNotificationPolicy(policy)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.NotificationPolicyServiceUpdateNotificationPolicyResponse{Policy: mapped}), nil
}
