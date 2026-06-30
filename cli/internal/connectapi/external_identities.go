package connectapi

import (
	"context"
	"net/url"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type externalIdentityFlowService struct {
	api *API
}

type externalIdentityService struct {
	api *API
}

func (s *externalIdentityFlowService) GetPendingExternalIdentity(ctx context.Context, req *connect.Request[apiv1.GetPendingExternalIdentityRequest]) (*connect.Response[apiv1.GetPendingExternalIdentityResponse], error) {
	flow, err := s.api.core.GetPendingExternalIdentityFlow(ctx, req.Msg.GetToken())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetPendingExternalIdentityResponse{
		Pending: apiPendingExternalIdentity(flow),
	}), nil
}

func (s *externalIdentityFlowService) CreateExternalIdentityAccount(ctx context.Context, req *connect.Request[apiv1.CreateExternalIdentityAccountRequest]) (*connect.Response[apiv1.CreateExternalIdentityAccountResponse], error) {
	flow, err := s.api.core.GetPendingExternalIdentityCreateFlow(ctx, req.Msg.GetToken())
	if err != nil {
		return nil, connectError(err)
	}
	displayName := externalIdentityCreateDisplayName(req.Msg.GetLogin(), flow.DisplayNameHint)
	user, err := s.api.core.CreateUserForExternalIdentity(ctx, req.Msg.GetLogin(), displayName, flow)
	if err != nil {
		return nil, connectError(err)
	}
	token, err := s.api.core.CreateAuthTokenWithSource(ctx, user.GetId(), "external_identity_create")
	if err != nil {
		return nil, connectError(err)
	}
	browserSession, err := createBrowserSessionFromContext(ctx, user.GetId(), "external_identity_create")
	if err != nil {
		_ = s.api.core.RevokeAuthTokenWithReason(ctx, token, "external_identity_create_session_failed")
		return nil, connectError(err)
	}
	if err := s.api.core.RecordLoginSucceeded(ctx, user.GetId(), flow.ProviderType+":"+flow.ProviderID); err != nil {
		_ = s.api.core.RevokeAuthTokenWithReason(ctx, token, "external_identity_create_audit_failed")
		if browserSession.Revoke != nil {
			_ = browserSession.Revoke(ctx)
		}
		return nil, connectError(err)
	}
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CreateExternalIdentityAccountResponse{
		UserId: user.GetId(),
		Login:  user.GetLogin(),
		Token:  token,
	}), nil
}

func externalIdentityCreateDisplayName(login, hint string) string {
	displayName := core.NormalizeDisplayName(hint)
	if displayName == "" ||
		utf8.RuneCountInString(displayName) > core.MaxDisplayNameLength ||
		core.ValidateDisplayName(displayName) != nil {
		return strings.TrimSpace(login)
	}
	return displayName
}

func (s *externalIdentityFlowService) ConfirmExternalIdentityLink(ctx context.Context, req *connect.Request[apiv1.ConfirmExternalIdentityLinkRequest]) (*connect.Response[apiv1.ConfirmExternalIdentityLinkResponse], error) {
	flow, err := s.api.core.GetPendingExternalIdentityFlow(ctx, req.Msg.GetToken())
	if err != nil {
		return nil, connectError(err)
	}
	identity, err := s.api.core.ConfirmPendingExternalIdentityLink(ctx, flow)
	if err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ConfirmExternalIdentityLinkResponse{
		LinkedIdentity: apiLinkedExternalIdentity(identity, s.api.providerLabels()),
	}), nil
}

func (s *externalIdentityFlowService) CancelExternalIdentityFlow(ctx context.Context, req *connect.Request[apiv1.CancelExternalIdentityFlowRequest]) (*connect.Response[apiv1.CancelExternalIdentityFlowResponse], error) {
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CancelExternalIdentityFlowResponse{Cancelled: true}), nil
}

func (s *externalIdentityService) ListExternalIdentities(ctx context.Context, _ *connect.Request[apiv1.ListExternalIdentitiesRequest]) (*connect.Response[apiv1.ListExternalIdentitiesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	identities, err := s.api.core.ExternalIdentitiesForUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListExternalIdentitiesResponse{
		Providers:        apiExternalIdentityProviders(s.api.config.Auth.PublicProviders(), identities),
		LinkedIdentities: apiLinkedExternalIdentities(identities, s.api.providerLabels()),
	}), nil
}

func (s *externalIdentityService) StartExternalIdentityLink(ctx context.Context, req *connect.Request[apiv1.StartExternalIdentityLinkRequest]) (*connect.Response[apiv1.StartExternalIdentityLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	provider, ok := s.api.authProvider(req.Msg.GetProviderId())
	if !ok {
		return nil, connectError(core.ErrNotFound)
	}
	redirectPath := req.Msg.GetRedirectPath()
	if redirectPath != "" && !isValidInternalRedirectPath(redirectPath) {
		return nil, connectError(core.ErrInvalidArgument)
	}
	if err := s.api.requireFreshCredential(ctx, caller, req.Msg.GetCurrentPassword()); err != nil {
		return nil, connectError(err)
	}
	token, err := s.api.core.CreatePendingExternalIdentityLinkStart(ctx, provider.ID, redirectPath, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.StartExternalIdentityLinkResponse{
		StartUrl: s.api.externalIdentityLinkStartURL(ctx, provider.ID, token),
	}), nil
}

func (s *externalIdentityService) LinkExternalIdentity(ctx context.Context, req *connect.Request[apiv1.LinkExternalIdentityRequest]) (*connect.Response[apiv1.LinkExternalIdentityResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	flow, err := s.api.core.GetPendingExternalIdentityLinkFlow(ctx, req.Msg.GetToken(), caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	identity, err := s.api.core.LinkPendingExternalIdentity(ctx, caller.UserID, flow)
	if err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.LinkExternalIdentityResponse{
		LinkedIdentity: apiLinkedExternalIdentity(identity, s.api.providerLabels()),
	}), nil
}

func (s *externalIdentityService) DisconnectExternalIdentity(ctx context.Context, req *connect.Request[apiv1.DisconnectExternalIdentityRequest]) (*connect.Response[apiv1.DisconnectExternalIdentityResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.requireFreshCredential(ctx, caller, req.Msg.GetCurrentPassword()); err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.DisconnectExternalIdentity(ctx, caller.UserID, req.Msg.GetSubjectHash()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DisconnectExternalIdentityResponse{Disconnected: true}), nil
}

func apiPendingExternalIdentity(flow *core.PendingExternalIdentityFlow) *apiv1.PendingExternalIdentity {
	if flow == nil {
		return nil
	}
	kind := apiv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_UNSPECIFIED
	switch flow.Kind {
	case core.ExternalIdentityFlowKindCreate:
		kind = apiv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_CREATE_ACCOUNT
	case core.ExternalIdentityFlowKindLink:
		kind = apiv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_LINK_ACCOUNT
	}
	return &apiv1.PendingExternalIdentity{
		Kind:            kind,
		ProviderId:      flow.ProviderID,
		ProviderType:    flow.ProviderType,
		ProviderLabel:   flow.ProviderLabel,
		VerifiedEmail:   flow.VerifiedEmail,
		LoginHint:       flow.LoginHint,
		DisplayNameHint: flow.DisplayNameHint,
		BoundUserId:     flow.BoundUserID,
		RedirectPath:    flow.RedirectPath,
	}
}

func apiExternalIdentityProviders(providers []config.AuthProviderConfig, identities []core.ExternalIdentity) []*apiv1.ExternalIdentityProvider {
	result := make([]*apiv1.ExternalIdentityProvider, 0, len(providers))
	for _, provider := range providers {
		escapedID := url.PathEscape(provider.ID)
		linkedIdentity, linked := providerLinkedIdentity(provider, identities)
		result = append(result, &apiv1.ExternalIdentityProvider{
			Id:                        provider.ID,
			Type:                      provider.Type,
			Label:                     provider.LabelOrDefault(),
			LoginUrl:                  "/auth/providers/" + escapedID,
			LinkUrl:                   "/auth/providers/" + escapedID + "?intent=link",
			Linked:                    linked,
			LinkedIdentitySubjectHash: linkedIdentity.SubjectHash,
		})
	}
	return result
}

func apiLinkedExternalIdentities(identities []core.ExternalIdentity, labels map[string]string) []*apiv1.LinkedExternalIdentity {
	result := make([]*apiv1.LinkedExternalIdentity, 0, len(identities))
	for _, identity := range identities {
		result = append(result, apiLinkedExternalIdentity(identity, labels))
	}
	return result
}

func apiLinkedExternalIdentity(identity core.ExternalIdentity, labels map[string]string) *apiv1.LinkedExternalIdentity {
	label := labels[identity.ProviderID]
	if label == "" {
		label = identity.ProviderID
	}
	return &apiv1.LinkedExternalIdentity{
		ProviderId:    identity.ProviderID,
		ProviderType:  identity.ProviderType,
		ProviderLabel: label,
		SubjectHash:   identity.SubjectHash,
	}
}

func (a *API) providerLabels() map[string]string {
	labels := make(map[string]string, len(a.config.Auth.Providers))
	for _, provider := range a.config.Auth.Providers {
		labels[provider.ID] = provider.LabelOrDefault()
	}
	return labels
}

func (a *API) authProvider(providerID string) (config.AuthProviderConfig, bool) {
	for _, provider := range a.config.Auth.Providers {
		if provider.ID == providerID {
			return provider, true
		}
	}
	return config.AuthProviderConfig{}, false
}

func (a *API) externalIdentityLinkStartURL(ctx context.Context, providerID, token string) string {
	baseURL := strings.TrimRight(requestBaseURLFromContext(ctx), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(a.config.Webserver.URL, "/")
	}
	path := "/auth/providers/" + url.PathEscape(providerID)
	values := url.Values{}
	values.Set("intent", "link")
	values.Set("link_start", token)
	return baseURL + path + "?" + values.Encode()
}

func providerLinkedIdentity(provider config.AuthProviderConfig, identities []core.ExternalIdentity) (core.ExternalIdentity, bool) {
	for _, identity := range identities {
		if identity.ProviderID == provider.ID {
			return identity, true
		}
		if provider.Type == config.AuthProviderTypeOpenIDConnect &&
			identity.ProviderType == config.AuthProviderTypeOpenIDConnect &&
			identity.Issuer == provider.IssuerURL {
			return identity, true
		}
	}
	return core.ExternalIdentity{}, false
}

func isValidInternalRedirectPath(redirect string) bool {
	return strings.HasPrefix(redirect, "/") && !strings.HasPrefix(redirect, "//") && !strings.Contains(redirect, "\\")
}
