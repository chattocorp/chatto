package connectapi

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
)

type externalIdentityAuthService struct {
	api *API
}

func (s *externalIdentityAuthService) GetPendingExternalIdentity(ctx context.Context, req *connect.Request[authv1.GetPendingExternalIdentityRequest]) (*connect.Response[authv1.GetPendingExternalIdentityResponse], error) {
	flow, err := s.api.core.GetPendingExternalIdentityFlow(ctx, req.Msg.GetToken())
	if err != nil {
		return nil, connectError(err)
	}
	if flow.Kind == core.ExternalIdentityFlowKindCreate {
		flow.LoginHint = availableExternalIdentityLogin(s.api.core, req.Msg.GetToken(), flow.LoginHint)
	}
	return connect.NewResponse(&authv1.GetPendingExternalIdentityResponse{
		Pending: apiPendingExternalIdentity(flow),
	}), nil
}

func availableExternalIdentityLogin(chattoCore *core.ChattoCore, token, hint string) string {
	hint = strings.TrimSpace(hint)
	if chattoCore.IsLoginAvailable(hint) {
		return hint
	}
	if core.ValidateLogin(hint) != nil {
		return hint
	}
	digest := sha256.Sum256([]byte(token))
	seed := binary.BigEndian.Uint32(digest[:4]) % 10000
	const suffixLength = 5 // hyphen plus four digits
	base := strings.TrimRight(hint[:min(len(hint), core.MaxLoginLength-suffixLength)], ".")
	for offset := range uint32(100) {
		candidate := fmt.Sprintf("%s-%04d", base, (seed+offset)%10000)
		if chattoCore.IsLoginAvailable(candidate) {
			return candidate
		}
	}
	return hint
}

func (s *externalIdentityAuthService) CreateExternalIdentityAccount(ctx context.Context, req *connect.Request[authv1.CreateExternalIdentityAccountRequest]) (*connect.Response[authv1.CreateExternalIdentityAccountResponse], error) {
	flow, err := s.api.core.GetPendingExternalIdentityCreateFlow(ctx, req.Msg.GetToken())
	if err != nil {
		return nil, connectError(err)
	}
	if s.api.config.Auth.InvitationRequired() {
		if strings.TrimSpace(flow.InvitationID) == "" {
			return nil, connectError(core.ErrInvitationInvalid)
		}
	} else {
		// Pending flows survive configuration rollouts. Re-evaluate admission
		// now, and never redeem an invitation while the server is open.
		flow.InvitationID = ""
	}
	displayName := externalIdentityCreateDisplayName(req.Msg.GetLogin(), req.Msg.GetDisplayName(), flow.DisplayNameHint)
	user, err := s.api.core.CreateUserForExternalIdentity(ctx, req.Msg.GetLogin(), displayName, flow)
	if err != nil {
		return nil, connectError(err)
	}
	credentials, err := s.api.core.CreateBearerSessionWithSource(ctx, user.GetId(), "external_identity_create")
	if err != nil {
		return nil, connectError(err)
	}
	browserSession, err := createBrowserSessionFromContext(ctx, user.GetId(), "external_identity_create")
	if err != nil {
		_ = s.api.core.RevokeRefreshTokenWithReason(ctx, credentials.RefreshToken, "external_identity_create_session_failed")
		return nil, connectError(err)
	}
	if err := s.api.core.RecordLoginSucceeded(ctx, user.GetId(), flow.ProviderType+":"+flow.ProviderID); err != nil {
		_ = s.api.core.RevokeRefreshTokenWithReason(ctx, credentials.RefreshToken, "external_identity_create_audit_failed")
		if browserSession.Revoke != nil {
			_ = browserSession.Revoke(ctx)
		}
		return nil, connectError(err)
	}
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&authv1.CreateExternalIdentityAccountResponse{
		UserId:                user.GetId(),
		Login:                 user.GetLogin(),
		Token:                 credentials.AccessToken,
		RefreshToken:          credentials.RefreshToken,
		ExpiresIn:             remainingLifetimeSeconds(credentials.AccessTokenExpiresAt),
		RefreshTokenExpiresIn: remainingLifetimeSeconds(credentials.SessionExpiresAt),
	}), nil
}

func remainingLifetimeSeconds(expiresAt time.Time) int64 {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return 0
	}
	return int64((remaining + time.Second - 1) / time.Second)
}

func externalIdentityCreateDisplayName(login, requested, hint string) string {
	displayName := core.NormalizeDisplayName(requested)
	if displayName != "" {
		return displayName
	}
	displayName = core.NormalizeDisplayName(hint)
	if displayName == "" ||
		utf8.RuneCountInString(displayName) > core.MaxDisplayNameLength ||
		core.ValidateDisplayName(displayName) != nil {
		return strings.TrimSpace(login)
	}
	return displayName
}

func (s *externalIdentityAuthService) ConfirmExternalIdentityLink(ctx context.Context, req *connect.Request[authv1.ConfirmExternalIdentityLinkRequest]) (*connect.Response[authv1.ConfirmExternalIdentityLinkResponse], error) {
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
	return connect.NewResponse(&authv1.ConfirmExternalIdentityLinkResponse{
		LinkedIdentity: apiLinkedExternalIdentity(identity, s.api.providerLabels()),
	}), nil
}

func (s *externalIdentityAuthService) CancelExternalIdentityFlow(ctx context.Context, req *connect.Request[authv1.CancelExternalIdentityFlowRequest]) (*connect.Response[authv1.CancelExternalIdentityFlowResponse], error) {
	if err := s.api.core.DeletePendingExternalIdentityFlow(ctx, req.Msg.GetToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&authv1.CancelExternalIdentityFlowResponse{Cancelled: true}), nil
}

func (s *accountService) ListExternalIdentities(ctx context.Context, _ *connect.Request[apiv1.ListExternalIdentitiesRequest]) (*connect.Response[apiv1.ListExternalIdentitiesResponse], error) {
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

func (s *accountService) StartExternalIdentityLink(ctx context.Context, req *connect.Request[apiv1.StartExternalIdentityLinkRequest]) (*connect.Response[apiv1.StartExternalIdentityLinkResponse], error) {
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

func (s *accountService) DisconnectExternalIdentity(ctx context.Context, req *connect.Request[apiv1.DisconnectExternalIdentityRequest]) (*connect.Response[apiv1.DisconnectExternalIdentityResponse], error) {
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

func apiPendingExternalIdentity(flow *core.PendingExternalIdentityFlow) *authv1.PendingExternalIdentity {
	if flow == nil {
		return nil
	}
	kind := authv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_UNSPECIFIED
	switch flow.Kind {
	case core.ExternalIdentityFlowKindCreate:
		kind = authv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_CREATE_ACCOUNT
	case core.ExternalIdentityFlowKindLink:
		kind = authv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_LINK_ACCOUNT
	}
	return &authv1.PendingExternalIdentity{
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
			LinkUrl:                   "/auth/providers/" + escapedID + "?intent=link",
			Linked:                    linked,
			LinkedIdentitySubjectHash: linkedIdentity.SubjectHash,
			Provider:                  apiProviderMetadata(provider),
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
