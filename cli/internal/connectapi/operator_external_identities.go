package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func (s *operatorUserService) ListUserExternalIdentities(ctx context.Context, req *connect.Request[operatorv1.ListUserExternalIdentitiesRequest]) (*connect.Response[operatorv1.ListUserExternalIdentitiesResponse], error) {
	identities, err := s.api.core.ExternalIdentitiesForUser(ctx, req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	policy := s.api.operatorIdentitySignInPolicy()
	rows := make([]*operatorv1.UserExternalIdentity, 0, len(identities))
	for _, identity := range identities {
		rows = append(rows, operatorExternalIdentity(identity, policy))
	}
	return connect.NewResponse(&operatorv1.ListUserExternalIdentitiesResponse{Identities: rows}), nil
}

func (s *operatorUserService) LinkUserExternalIdentity(ctx context.Context, req *connect.Request[operatorv1.LinkUserExternalIdentityRequest]) (*connect.Response[operatorv1.LinkUserExternalIdentityResponse], error) {
	if strings.TrimSpace(req.Msg.GetSubject()) == "" {
		return nil, core.ErrInvalidArgument
	}
	provider, ok := s.api.authProvider(req.Msg.GetProviderId())
	if !ok {
		return nil, core.ErrNotFound
	}
	issuer := operatorProviderIssuer(provider)
	identity, err := s.api.core.LinkExternalIdentityAs(ctx, core.SystemActorID, provider.ID, provider.Type, issuer, req.Msg.GetSubject(), req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.LinkUserExternalIdentityResponse{
		Identity: operatorExternalIdentity(identity, s.api.operatorIdentitySignInPolicy()),
	}), nil
}

func (s *operatorUserService) UnlinkUserExternalIdentity(ctx context.Context, req *connect.Request[operatorv1.UnlinkUserExternalIdentityRequest]) (*connect.Response[operatorv1.UnlinkUserExternalIdentityResponse], error) {
	if err := s.api.core.DisconnectExternalIdentityAs(ctx, core.SystemActorID, req.Msg.GetUserId(), req.Msg.GetSubjectHash(), s.api.operatorIdentitySignInPolicy()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.UnlinkUserExternalIdentityResponse{}), nil
}

// operatorIdentitySignInPolicy describes configuration, without contacting an
// external provider or assuming it is reachable. Provider labels and historic
// provider IDs are not part of identity ownership.
func (a *API) operatorIdentitySignInPolicy() core.ExternalIdentitySignInPolicy {
	policy := core.ExternalIdentitySignInPolicy{PasswordLoginEnabled: a.config.Auth.DirectLoginOrDefault()}
	for _, provider := range a.config.Auth.Providers {
		policy.Issuers = append(policy.Issuers, operatorProviderIssuer(provider))
	}
	return policy
}

func operatorProviderIssuer(provider config.AuthProviderConfig) string {
	if provider.Type == config.AuthProviderTypeOpenIDConnect {
		return provider.IssuerURL
	}
	return provider.ID
}

func operatorExternalIdentity(identity core.ExternalIdentity, policy core.ExternalIdentitySignInPolicy) *operatorv1.UserExternalIdentity {
	return &operatorv1.UserExternalIdentity{
		ProviderId: identity.ProviderID, ProviderType: identity.ProviderType,
		Issuer: identity.Issuer, Subject: identity.Subject, SubjectHash: identity.SubjectHash,
		LoginAvailable: policy.AllowsIdentity(identity),
	}
}
