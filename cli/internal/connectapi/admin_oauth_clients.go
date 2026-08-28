package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

const (
	defaultOAuthClientLimit = 20
	maxOAuthClientLimit     = 100
)

type adminOAuthClientService struct{ api *API }

func (s *adminOAuthClientService) ListOAuthClients(ctx context.Context, req *connect.Request[adminv1.ListOAuthClientsRequest]) (*connect.Response[adminv1.ListOAuthClientsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.api.core.ListOAuthClients(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultOAuthClientLimit, maxOAuthClientLimit)
	total := len(states)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	clients := make([]*adminv1.OAuthClient, 0, end-offset)
	for _, state := range states[offset:end] {
		clients = append(clients, apiOAuthClient(state))
	}
	return connect.NewResponse(&adminv1.ListOAuthClientsResponse{
		OauthClients: clients,
		Page:         apiPageInfo(total, end < total),
	}), nil
}

func (s *adminOAuthClientService) GetOAuthClient(ctx context.Context, req *connect.Request[adminv1.GetOAuthClientRequest]) (*connect.Response[adminv1.GetOAuthClientResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.GetOAuthClient(ctx, caller.UserID, req.Msg.GetClientId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetOAuthClientResponse{OauthClient: apiOAuthClient(state)}), nil
}

func (s *adminOAuthClientService) UpdateOAuthClientPolicy(ctx context.Context, req *connect.Request[adminv1.UpdateOAuthClientPolicyRequest]) (*connect.Response[adminv1.UpdateOAuthClientPolicyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := coreOAuthClientPolicy(req.Msg.GetPolicy())
	if err != nil {
		return nil, connectError(err)
	}
	state, err := s.api.core.UpdateOAuthClientPolicy(ctx, caller.UserID, req.Msg.GetClientId(), policy)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdateOAuthClientPolicyResponse{OauthClient: apiOAuthClient(state)}), nil
}

func apiOAuthClient(state core.OAuthClientState) *adminv1.OAuthClient {
	return &adminv1.OAuthClient{
		ClientId:             state.ClientID,
		ClientName:           state.ClientName,
		ClientOrigin:         state.ClientOrigin,
		Source:               apiOAuthClientSource(state.Source),
		Policy:               apiOAuthClientPolicy(state.Policy),
		FirstAuthorizationAt: timestamppb.New(state.FirstAuthorizationAt),
		LastAuthorizationAt:  timestamppb.New(state.LastAuthorizationAt),
		RedirectOrigins:      append([]string(nil), state.RedirectOrigins...),
		AuthorizedUserCount:  state.AuthorizedUserCount,
	}
}

func apiOAuthClientSource(source evtv1.OAuthClientSource) adminv1.OAuthClientSource {
	switch source {
	case evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD:
		return adminv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD
	case evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_BUILT_IN:
		return adminv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_BUILT_IN
	default:
		// The public and durable enums intentionally share numeric assignments.
		// Preserve future values so an older administration client can fail
		// closed instead of mislabelling an unsupported source.
		return adminv1.OAuthClientSource(source)
	}
}

func apiOAuthClientPolicy(policy evtv1.OAuthClientPolicy) adminv1.OAuthClientPolicy {
	switch policy {
	case evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT:
		return adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT
	case evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED:
		return adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED
	case evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED:
		return adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED
	default:
		// Preserve unknown numeric values across the durable/public boundary so
		// clients can render them as unsupported and disable policy editing.
		return adminv1.OAuthClientPolicy(policy)
	}
}

func coreOAuthClientPolicy(policy adminv1.OAuthClientPolicy) (evtv1.OAuthClientPolicy, error) {
	switch policy {
	case adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT:
		return evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT, nil
	case adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED:
		return evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED, nil
	case adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED:
		return evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED, nil
	default:
		return evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_UNSPECIFIED, core.ErrInvalidArgument
	}
}
