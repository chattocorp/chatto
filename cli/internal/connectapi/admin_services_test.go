package connectapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	discoveryv1 "hmans.de/chatto/internal/pb/chatto/discovery/v1"
	"hmans.de/chatto/internal/pb/chatto/discovery/v1/discoveryv1connect"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestServerDiscoveryServiceGetServerPublicMetadata(t *testing.T) {
	autoProvision := true
	api := New(nil, config.ChattoConfig{
		Auth: config.AuthConfig{
			Providers: []config.AuthProviderConfig{
				{ID: "hub provider", Type: config.AuthProviderTypeOpenIDConnect, Label: "Chatto Hub", IssuerURL: "https://id.example", AutoProvision: &autoProvision},
			},
		},
	}, "9.8.7")
	mux := http.NewServeMux()
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client := discoveryv1connect.NewServerDiscoveryServiceClient(ts.Client(), ts.URL)
	resp, err := client.GetServer(context.Background(), connect.NewRequest(&discoveryv1.GetServerRequest{}))
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}

	msg := resp.Msg
	if msg.GetProfile().GetName() != "Chatto" {
		t.Fatalf("profile name = %q, want Chatto", msg.GetProfile().GetName())
	}
	if msg.GetProfile().GetVersion() != "9.8.7" {
		t.Fatalf("profile version = %q, want 9.8.7", msg.GetProfile().GetVersion())
	}
	if !msg.GetLogin().GetDirectRegistrationEnabled() {
		t.Fatal("DirectRegistrationEnabled = false, want true")
	}
	if msg.GetLogin().GetAccountCreationPolicy() != apiv1.AccountCreationPolicy_ACCOUNT_CREATION_POLICY_OPEN {
		t.Fatalf("AccountCreationPolicy = %v, want OPEN", msg.GetLogin().GetAccountCreationPolicy())
	}
	if len(msg.GetLogin().GetProviders()) != 1 {
		t.Fatalf("providers len = %d, want 1", len(msg.GetLogin().GetProviders()))
	}
	provider := msg.GetLogin().GetProviders()[0]
	if provider.Id != "hub provider" {
		t.Fatalf("provider Id = %q, want hub provider", provider.Id)
	}
	if provider.LoginUrl != "/auth/providers/hub%20provider" {
		t.Fatalf("provider LoginUrl = %q, want escaped provider path", provider.LoginUrl)
	}
	if provider.GetIssuerUrl() != "https://id.example" {
		t.Fatalf("provider IssuerUrl = %q, want https://id.example", provider.GetIssuerUrl())
	}
	if !provider.GetAutoProvision() {
		t.Fatal("provider AutoProvision = false, want true")
	}
}

func TestExternalIdentityAccountCreationRechecksCurrentInvitationPolicy(t *testing.T) {
	env := newConnectAPITestEnv(t)
	createToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:   "oidc-main",
		ProviderType: config.AuthProviderTypeOpenIDConnect,
		Issuer:       "https://id.example",
		Subject:      "created-while-open",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}
	env.api.config.Auth.AccountCreationPolicy = config.AccountCreationPolicyInviteOnly
	_, err = env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: createToken,
		Login: "policy-change",
	}))
	requireConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestOpenExternalIdentityAccountCreationDoesNotRedeemCapturedInvitation(t *testing.T) {
	env := newConnectAPITestEnv(t)
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	maxUses := uint32(1)
	invitation, err := env.core.CreateInvitation(env.ctx, env.viewer.Id, &maxUses, nil)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	createToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:   "oidc-main",
		ProviderType: config.AuthProviderTypeOpenIDConnect,
		Issuer:       "https://id.example",
		Subject:      "created-while-invited",
		InvitationID: invitation.ID,
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}
	env.api.config.Auth.AccountCreationPolicy = config.AccountCreationPolicyOpen
	_, err = env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: createToken,
		Login: "now-open",
	}))
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount: %v", err)
	}
	state, err := env.core.GetInvitation(env.ctx, env.viewer.Id, invitation.ID)
	if err != nil {
		t.Fatalf("GetInvitation: %v", err)
	}
	if state.UseCount != 0 {
		t.Fatalf("invitation use count = %d, want 0", state.UseCount)
	}
}

func TestExternalIdentityFlowsAndAccountManagement(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Auth.Providers = []config.AuthProviderConfig{
		{ID: "github-main", Type: config.AuthProviderTypeGitHub, Label: "GitHub"},
		{ID: "discord-main", Type: config.AuthProviderTypeDiscord, Label: "Discord"},
		{ID: "gitlab-main", Type: config.AuthProviderTypeGitLab, Label: "GitLab"},
	}

	createToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:      "github-main",
		ProviderType:    config.AuthProviderTypeGitHub,
		ProviderLabel:   "GitHub",
		Issuer:          "github-main",
		Subject:         "12345",
		LoginHint:       "sso-user",
		DisplayNameHint: "SSO User",
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow: %v", err)
	}

	pending, err := env.externalAuth.GetPendingExternalIdentity(env.ctx, connect.NewRequest(&authv1.GetPendingExternalIdentityRequest{
		Token: createToken,
	}))
	if err != nil {
		t.Fatalf("GetPendingExternalIdentity: %v", err)
	}
	if pending.Msg.Pending.GetKind() != authv1.ExternalIdentityFlowKind_EXTERNAL_IDENTITY_FLOW_KIND_CREATE_ACCOUNT || pending.Msg.Pending.GetProviderId() != "github-main" {
		t.Fatalf("pending = %+v", pending.Msg.Pending)
	}

	created, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: createToken,
		Login: "sso-user",
	}))
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount: %v", err)
	}
	createdAuthToken := created.Msg.GetToken()
	userID, err := env.core.ValidateAuthToken(env.ctx, createdAuthToken)
	if err != nil {
		t.Fatalf("ValidateAuthToken: %v", err)
	}
	if userID != created.Msg.GetUserId() {
		t.Fatalf("created token user = %q, want %q", userID, created.Msg.GetUserId())
	}
	createdUser, err := env.core.GetUser(env.ctx, created.Msg.GetUserId())
	if err != nil {
		t.Fatalf("GetUser created: %v", err)
	}
	if createdUser.GetDisplayName() != "SSO User" {
		t.Fatalf("created display name = %q, want SSO User", createdUser.GetDisplayName())
	}
	if _, err := env.core.GetPendingExternalIdentityFlow(env.ctx, createToken); !errors.Is(err, core.ErrExternalIdentityFlowNotFound) {
		t.Fatalf("pending create flow after confirmation error = %v, want ErrExternalIdentityFlowNotFound", err)
	}

	fallbackToken, err := env.core.CreatePendingExternalIdentityCreateFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:      "discord-main",
		ProviderType:    config.AuthProviderTypeDiscord,
		ProviderLabel:   "Discord",
		Issuer:          "discord-main",
		Subject:         "fallback-display-name",
		LoginHint:       "fallback-user",
		DisplayNameHint: strings.Repeat("Provider ", 8),
	})
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityCreateFlow fallback: %v", err)
	}
	fallbackCreated, err := env.externalAuth.CreateExternalIdentityAccount(env.ctx, connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{
		Token: fallbackToken,
		Login: "fallback-user",
	}))
	if err != nil {
		t.Fatalf("CreateExternalIdentityAccount fallback: %v", err)
	}
	fallbackUser, err := env.core.GetUser(env.ctx, fallbackCreated.Msg.GetUserId())
	if err != nil {
		t.Fatalf("GetUser fallback: %v", err)
	}
	if fallbackUser.GetDisplayName() != "fallback-user" {
		t.Fatalf("fallback display name = %q, want login", fallbackUser.GetDisplayName())
	}

	createdUserRef := &corev1.User{Id: created.Msg.GetUserId()}
	createdCtx := withBearerCredential(env.ctx, createdUserRef, createdAuthToken)
	list, err := env.account.ListExternalIdentities(createdCtx, connect.NewRequest(&apiv1.ListExternalIdentitiesRequest{}))
	if err != nil {
		t.Fatalf("ListExternalIdentities: %v", err)
	}
	if len(list.Msg.GetProviders()) != 3 ||
		list.Msg.GetProviders()[0].GetLinkUrl() != "/auth/providers/github-main?intent=link" ||
		!list.Msg.GetProviders()[0].GetLinked() ||
		list.Msg.GetProviders()[0].GetLinkedIdentitySubjectHash() == "" {
		t.Fatalf("providers = %+v", list.Msg.GetProviders())
	}
	if len(list.Msg.GetLinkedIdentities()) != 1 || list.Msg.GetLinkedIdentities()[0].GetProviderId() != "github-main" {
		t.Fatalf("linked identities = %+v", list.Msg.GetLinkedIdentities())
	}
	_, err = env.account.DisconnectExternalIdentity(createdCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash: list.Msg.GetProviders()[0].GetLinkedIdentitySubjectHash(),
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	if _, err := env.account.StartExternalIdentityLink(withCaller(env.ctx, createdUserRef), connect.NewRequest(&apiv1.StartExternalIdentityLinkRequest{
		ProviderId:   "discord-main",
		RedirectPath: "/chat/-/settings/account",
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("StartExternalIdentityLink without credential code = %v, want failed_precondition", connect.CodeOf(err))
	}
	started, err := env.account.StartExternalIdentityLink(createdCtx, connect.NewRequest(&apiv1.StartExternalIdentityLinkRequest{
		ProviderId:   "discord-main",
		RedirectPath: "/chat/-/settings/account",
	}))
	if err != nil {
		t.Fatalf("StartExternalIdentityLink: %v", err)
	}
	startURL, err := url.Parse(started.Msg.GetStartUrl())
	if err != nil {
		t.Fatalf("start url parse: %v", err)
	}
	if startURL.Path != "/auth/providers/discord-main" || startURL.Query().Get("intent") != "link" || startURL.Query().Get("link_start") == "" {
		t.Fatalf("start url = %q", started.Msg.GetStartUrl())
	}
	linkStart, err := env.core.ConsumePendingExternalIdentityLinkStart(env.ctx, startURL.Query().Get("link_start"))
	if err != nil {
		t.Fatalf("ConsumePendingExternalIdentityLinkStart: %v", err)
	}
	if linkStart.BoundUserID != created.Msg.GetUserId() || linkStart.ProviderID != "discord-main" || linkStart.RedirectPath != "/chat/-/settings/account" {
		t.Fatalf("link start = %+v", linkStart)
	}

	linkToken, err := env.core.CreatePendingExternalIdentityLinkFlow(env.ctx, core.PendingExternalIdentityFlow{
		ProviderID:   "discord-main",
		ProviderType: config.AuthProviderTypeDiscord,
		Issuer:       "discord-main",
		Subject:      "abc123",
	}, env.viewer.Id)
	if err != nil {
		t.Fatalf("CreatePendingExternalIdentityLinkFlow: %v", err)
	}
	linked, err := env.externalAuth.ConfirmExternalIdentityLink(env.ctx, connect.NewRequest(&authv1.ConfirmExternalIdentityLinkRequest{
		Token: linkToken,
	}))
	if err != nil {
		t.Fatalf("ConfirmExternalIdentityLink: %v", err)
	}
	if linked.Msg.LinkedIdentity.GetProviderId() != "discord-main" || linked.Msg.LinkedIdentity.GetSubjectHash() == "" {
		t.Fatalf("linked identity = %+v", linked.Msg.LinkedIdentity)
	}
	oauthViewerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, env.viewer.Id, "oauth_code_exchange")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource oauth viewer: %v", err)
	}
	oauthCredentialCtx := withBearerCredential(env.ctx, env.viewer, oauthViewerToken)
	_, err = env.account.DisconnectExternalIdentity(oauthCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash:     linked.Msg.LinkedIdentity.GetSubjectHash(),
		CurrentPassword: "password",
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)

	staleViewerToken, err := env.core.CreateAuthTokenWithSource(env.ctx, env.viewer.Id, "unknown")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource stale viewer: %v", err)
	}
	viewerCredentialCtx := withBearerCredential(env.ctx, env.viewer, staleViewerToken)
	_, err = env.account.DisconnectExternalIdentity(viewerCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash: linked.Msg.LinkedIdentity.GetSubjectHash(),
	}))
	requireConnectCode(t, err, connect.CodeFailedPrecondition)
	disconnected, err := env.account.DisconnectExternalIdentity(viewerCredentialCtx, connect.NewRequest(&apiv1.DisconnectExternalIdentityRequest{
		SubjectHash:     linked.Msg.LinkedIdentity.GetSubjectHash(),
		CurrentPassword: "password",
	}))
	if err != nil {
		t.Fatalf("DisconnectExternalIdentity: %v", err)
	}
	if !disconnected.Msg.GetDisconnected() {
		t.Fatalf("DisconnectExternalIdentity disconnected = false")
	}
	found, err := env.core.GetUserByExternalIdentity(env.ctx, "discord-main", "abc123")
	if err != nil {
		t.Fatalf("GetUserByExternalIdentity after disconnect: %v", err)
	}
	if found != nil {
		t.Fatalf("GetUserByExternalIdentity after disconnect = %+v, want nil", found)
	}
	if _, err := env.core.GetPendingExternalIdentityFlow(env.ctx, linkToken); !errors.Is(err, core.ErrExternalIdentityFlowNotFound) {
		t.Fatalf("pending link flow after confirmation error = %v, want ErrExternalIdentityFlowNotFound", err)
	}

}

func TestExternalIdentityCreateDisplayName(t *testing.T) {
	tests := []struct {
		name  string
		login string
		hint  string
		want  string
	}{
		{
			name:  "valid hint",
			login: "sso-user",
			hint:  "SSO User",
			want:  "SSO User",
		},
		{
			name:  "empty hint falls back",
			login: "sso-user",
			hint:  " ",
			want:  "sso-user",
		},
		{
			name:  "invalid punctuation falls back",
			login: "sso-user",
			hint:  "User, Inc.",
			want:  "sso-user",
		},
		{
			name:  "too long falls back",
			login: "sso-user",
			hint:  strings.Repeat("A", core.MaxDisplayNameLength+1),
			want:  "sso-user",
		},
		{
			name:  "invalid start falls back",
			login: "sso-user",
			hint:  "😀 User",
			want:  "sso-user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := externalIdentityCreateDisplayName(tt.login, tt.hint); got != tt.want {
				t.Fatalf("externalIdentityCreateDisplayName(%q, %q) = %q, want %q", tt.login, tt.hint, got, tt.want)
			}
		})
	}
}

func TestOperatorUserServiceLifecycle(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	createResp, err := operator.CreateUser(env.ctx, connect.NewRequest(&operatorv1.CreateUserRequest{
		Login:         "operator-api-user",
		DisplayName:   "Operator API User",
		Password:      "password123",
		VerifiedEmail: "operator-api@example.com",
		RoleNames:     []string{core.RoleAdmin},
	}))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	member := createResp.Msg.GetMember()
	user := member.GetUser()
	if user.GetId() == "" || user.GetLogin() != "operator-api-user" || user.GetDisplayName() != "Operator API User" {
		t.Fatalf("created member = %+v", member)
	}
	if got := strings.Join(member.GetRoles(), ","); got != core.RoleAdmin {
		t.Fatalf("created roles = %q, want %s", got, core.RoleAdmin)
	}
	if got := len(member.GetVerifiedEmails()); got != 1 {
		t.Fatalf("created verified email count = %d, want 1", got)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "operator-api-user", "password123"); err != nil {
		t.Fatalf("VerifyPassword initial: %v", err)
	}

	getByLoginResp, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Login: "operator-api-user"}))
	if err != nil {
		t.Fatalf("GetUser by login: %v", err)
	}
	if getByLoginResp.Msg.GetMember().GetUser().GetId() != user.GetId() {
		t.Fatalf("GetMember by login id = %q, want %q", getByLoginResp.Msg.GetMember().GetUser().GetId(), user.GetId())
	}
	getByEmailResp, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Email: "operator-api@example.com"}))
	if err != nil {
		t.Fatalf("GetUser by email: %v", err)
	}
	if getByEmailResp.Msg.GetMember().GetUser().GetId() != user.GetId() {
		t.Fatalf("GetMember by email id = %q, want %q", getByEmailResp.Msg.GetMember().GetUser().GetId(), user.GetId())
	}
	if _, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{Email: "missing-operator-api@example.com"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser by email err = %v, want not found", err)
	}

	updateResp, err := operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		Login:       stringPtr("operator-api-renamed"),
		DisplayName: stringPtr("Operator API Renamed"),
	}))
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got := updateResp.Msg.GetMember().GetUser().GetLogin(); got != "operator-api-renamed" {
		t.Fatalf("updated login = %q, want operator-api-renamed", got)
	}

	if _, err := operator.SetUserPassword(env.ctx, connect.NewRequest(&operatorv1.SetUserPasswordRequest{
		UserId:   user.GetId(),
		Password: "newpassword123",
	})); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if _, err := env.core.VerifyPassword(env.ctx, "operator-api-renamed", "newpassword123"); err != nil {
		t.Fatalf("VerifyPassword updated: %v", err)
	}

	emailResp, err := operator.AddVerifiedEmail(env.ctx, connect.NewRequest(&operatorv1.AddVerifiedEmailRequest{
		UserId: user.GetId(),
		Email:  "operator-api-alt@example.com",
	}))
	if err != nil {
		t.Fatalf("AddVerifiedEmail: %v", err)
	}
	if got := len(emailResp.Msg.GetMember().GetVerifiedEmails()); got != 2 {
		t.Fatalf("verified email count = %d, want 2", got)
	}

	if _, err := operator.AssignRole(env.ctx, connect.NewRequest(&operatorv1.AssignRoleRequest{
		UserId:   user.GetId(),
		RoleName: core.RoleModerator,
	})); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	roleResp, err := operator.RevokeRole(env.ctx, connect.NewRequest(&operatorv1.RevokeRoleRequest{
		UserId:   user.GetId(),
		RoleName: core.RoleAdmin,
	}))
	if err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if got := strings.Join(roleResp.Msg.GetMember().GetRoles(), ","); got != core.RoleModerator {
		t.Fatalf("roles after revoke = %q, want %s", got, core.RoleModerator)
	}

	if _, err := operator.DeleteUser(env.ctx, connect.NewRequest(&operatorv1.DeleteUserRequest{UserId: user.GetId()})); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := operator.GetUser(env.ctx, connect.NewRequest(&operatorv1.GetUserRequest{UserId: user.GetId()})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetUser after delete err = %v, want not found", err)
	}
}

func TestAdminUserServiceSelfCannotDeleteAccountFromMemberDetails(t *testing.T) {
	env := newConnectAPITestEnv(t)
	admin := &adminUserManagementService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "admin-api-self-delete", "Admin API Self Delete", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, user.GetId()); err != nil {
		t.Fatalf("AssignAdminRole setup: %v", err)
	}

	details, err := admin.GetMember(withCaller(env.ctx, user), connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: user.GetId()},
	}))
	if err != nil {
		t.Fatalf("GetMember self: %v", err)
	}
	if details.Msg.GetMember().GetViewerCanDeleteAccount() {
		t.Fatalf("ViewerCanDeleteAccount for self = true, want false")
	}
}

func TestOperatorUserServiceListUsesSharedPageInfo(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	for i := 1; i <= 2; i++ {
		login := fmt.Sprintf("operator-api-page-%d", i)
		if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, login, "Admin API Page", "password123"); err != nil {
			t.Fatalf("CreateUser %s: %v", login, err)
		}
	}

	defaultResp, err := operator.ListUsers(env.ctx, connect.NewRequest(&operatorv1.ListUsersRequest{Search: "operator-api-page"}))
	if err != nil {
		t.Fatalf("ListMembers default page: %v", err)
	}
	if got := len(defaultResp.Msg.GetUsers()); got != 2 {
		t.Fatalf("default ListMembers users = %d, want 2", got)
	}
	if page := defaultResp.Msg.GetPage(); page.GetTotalCount() != 2 || page.GetHasMore() {
		t.Fatalf("default ListMembers page = %+v, want total 2 has_more false", page)
	}

	firstPageResp, err := operator.ListUsers(env.ctx, connect.NewRequest(&operatorv1.ListUsersRequest{
		Search: "operator-api-page",
		Page:   &apiv1.PageRequest{Limit: 1},
	}))
	if err != nil {
		t.Fatalf("ListMembers first page: %v", err)
	}
	if got := len(firstPageResp.Msg.GetUsers()); got != 1 {
		t.Fatalf("first page ListMembers users = %d, want 1", got)
	}
	if page := firstPageResp.Msg.GetPage(); page.GetTotalCount() != 2 || !page.GetHasMore() {
		t.Fatalf("first page ListMembers page = %+v, want total 2 has_more true", page)
	}
}

func TestOperatorUserServiceUpdateUserValidatesAllFieldsBeforeWriting(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-update-rollback", "Original Display", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}

	_, err = operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		DisplayName: stringPtr("Changed Display"),
		Login:       stringPtr("bad login"),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateUser error = %v, want invalid argument", err)
	}

	got, err := env.core.GetUser(env.ctx, user.GetId())
	if err != nil {
		t.Fatalf("GetUser after rollback: %v", err)
	}
	if got.GetDisplayName() != "Original Display" {
		t.Fatalf("display name after rollback = %q, want Original Display", got.GetDisplayName())
	}
	if got.GetLogin() != "operator-api-update-rollback" {
		t.Fatalf("login after rollback = %q, want operator-api-update-rollback", got.GetLogin())
	}
}

func TestOperatorUserServiceUpdateUserEventsUseSystemActor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-actor", "Operator Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}

	if _, err := operator.UpdateUser(env.ctx, connect.NewRequest(&operatorv1.UpdateUserRequest{
		UserId:      user.GetId(),
		Login:       stringPtr("operator-api-actor-renamed"),
		DisplayName: stringPtr("Operator Actor Renamed"),
	})); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	loginEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserLoginChanged))
	if err != nil {
		t.Fatalf("SubjectEvents login changed: %v", err)
	}
	if len(loginEvents) != 1 {
		t.Fatalf("login changed events = %d, want 1", len(loginEvents))
	}
	if got := loginEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("login changed actor = %q, want %q", got, core.SystemActorID)
	}

	displayEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserDisplayNameChanged))
	if err != nil {
		t.Fatalf("SubjectEvents display name changed: %v", err)
	}
	if len(displayEvents) != 1 {
		t.Fatalf("display name changed events = %d, want 1", len(displayEvents))
	}
	if got := displayEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("display name changed actor = %q, want %q", got, core.SystemActorID)
	}
}

func TestOperatorUserServiceClearUsernameCooldownUsesSystemActor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-api-cooldown", "Operator API Cooldown", "password123")
	if err != nil {
		t.Fatalf("CreateUser setup: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-renamed"); err != nil {
		t.Fatalf("UpdateUserLogin setup: %v", err)
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-blocked"); !errors.Is(err, core.ErrLoginChangeCooldown) {
		t.Fatalf("second UpdateUserLogin error = %v, want cooldown", err)
	}

	resp, err := operator.ClearUsernameCooldown(env.ctx, connect.NewRequest(&operatorv1.ClearUsernameCooldownRequest{
		UserId: user.GetId(),
	}))
	if err != nil {
		t.Fatalf("ClearUsernameCooldown: %v", err)
	}
	if !resp.Msg.GetCleared() {
		t.Fatal("Cleared = false, want true")
	}
	if _, err := env.core.UpdateUserLogin(env.ctx, user.GetId(), "operator-api-cooldown-unblocked"); err != nil {
		t.Fatalf("UpdateUserLogin after clear: %v", err)
	}

	clearEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.UserAggregate(user.GetId()).Subject(evtstream.EventUserLoginCooldownCleared))
	if err != nil {
		t.Fatalf("SubjectEvents login cooldown cleared: %v", err)
	}
	if len(clearEvents) != 1 {
		t.Fatalf("login cooldown cleared events = %d, want 1", len(clearEvents))
	}
	if got := clearEvents[0].GetActorId(); got != core.SystemActorID {
		t.Fatalf("login cooldown cleared actor = %q, want %q", got, core.SystemActorID)
	}
}

func TestOperatorUserServiceAddVerifiedEmailRejectsMissingUserWithoutClaimingEmail(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	_, err := operator.AddVerifiedEmail(env.ctx, connect.NewRequest(&operatorv1.AddVerifiedEmailRequest{
		UserId: "UmissingVerifiedEmail",
		Email:  "missing-operator@example.com",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("AddVerifiedEmail error = %v, want not found", err)
	}
	if claimed, err := env.core.IsEmailClaimed(env.ctx, "missing-operator@example.com"); err != nil || claimed {
		t.Fatalf("IsEmailClaimed after missing user add = %t, %v; want false, nil", claimed, err)
	}
}

func TestOperatorUserServiceAssignRoleRejectsMissingUserWithoutPersistingRole(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}

	_, err := operator.AssignRole(env.ctx, connect.NewRequest(&operatorv1.AssignRoleRequest{
		UserId:   "UmissingAdminUser",
		RoleName: core.RoleAdmin,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("AssignRole error = %v, want not found", err)
	}
	roles, rolesErr := env.core.GetUserRoles(env.ctx, "UmissingAdminUser")
	if rolesErr != nil {
		t.Fatalf("GetUserRoles after missing user assignment: %v", rolesErr)
	}
	if len(roles) != 0 {
		t.Fatalf("missing user roles = %v after NotFound response, want none", roles)
	}

	beforeRevocations, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.RBACAggregate().Subject(evtstream.EventRBACRoleRevoked))
	if err != nil {
		t.Fatalf("SubjectEvents role revoked before: %v", err)
	}
	_, err = operator.RevokeRole(env.ctx, connect.NewRequest(&operatorv1.RevokeRoleRequest{
		UserId:   "UmissingAdminUser",
		RoleName: core.RoleAdmin,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("RevokeRole error = %v, want not found", err)
	}
	afterRevocations, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.RBACAggregate().Subject(evtstream.EventRBACRoleRevoked))
	if err != nil {
		t.Fatalf("SubjectEvents role revoked after: %v", err)
	}
	if len(afterRevocations) != len(beforeRevocations) {
		t.Fatalf("role revocation events changed from %d to %d for missing user", len(beforeRevocations), len(afterRevocations))
	}
}

func TestUserServiceGetUserReadsPublicUsers(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.users.GetUser(env.ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetUser code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.users.BatchGetUsers(env.ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{env.viewer.Id}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated BatchGetUsers code = %v, want unauthenticated", connect.CodeOf(err))
	}

	ctx := withCaller(env.ctx, env.viewer)
	offlineUser, err := env.core.CreateUser(env.ctx, core.SystemActorID, "offline-profile", "Offline Profile", "password")
	if err != nil {
		t.Fatalf("CreateUser offline profile: %v", err)
	}
	offlineResp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: offlineUser.Id}}))
	if err != nil {
		t.Fatalf("GetUser offline profile: %v", err)
	}
	if offlineResp.Msg.GetUser().GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE {
		t.Fatalf("offline profile presence = %v, want OFFLINE", offlineResp.Msg.GetUser().GetUser().GetPresenceStatus())
	}

	if _, err := env.core.SetUserCustomStatus(env.ctx, env.viewer.Id, "👋", "around", nil); err != nil {
		t.Fatalf("SetUserCustomStatus: %v", err)
	}
	if err := env.core.SetPresenceWithOptions(env.ctx, env.viewer.Id, "ONLINE", true); err != nil {
		t.Fatalf("SetPresenceWithOptions: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, "admin"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	resp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: env.viewer.Id}}))
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	userRow := resp.Msg.GetUser()
	user := userRow.GetUser()
	if user.GetId() != env.viewer.Id || user.GetLogin() != env.viewer.Login || user.GetDisplayName() != env.viewer.DisplayName {
		t.Fatalf("GetUser user = %+v, want viewer public profile", user)
	}
	if user.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE {
		t.Fatalf("PresenceStatus = %v, want ONLINE", user.GetPresenceStatus())
	}
	if user.GetCustomStatus().GetText() != "around" {
		t.Fatalf("CustomStatus = %+v, want status text", user.GetCustomStatus())
	}
	if roles := strings.Join(userRow.GetRoles(), ","); roles != "everyone,admin" {
		t.Fatalf("GetUser roles = %q, want everyone,admin", roles)
	}
	batchResp, err := env.users.BatchGetUsers(ctx, connect.NewRequest(&apiv1.BatchGetUsersRequest{
		UserIds: []string{env.viewer.Id, "missing-user", env.viewer.Id},
	}))
	if err != nil {
		t.Fatalf("BatchGetUsers: %v", err)
	}
	if got := batchResp.Msg.GetUsers(); len(got) != 1 {
		t.Fatalf("BatchGetUsers len = %d, want 1: %+v", len(got), got)
	} else if got[0].GetUser().GetId() != env.viewer.Id || got[0].GetUser().GetLogin() != env.viewer.Login || got[0].GetUser().GetDisplayName() != env.viewer.DisplayName || got[0].GetUser().GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE {
		t.Fatalf("BatchGetUsers user = %+v, want viewer user profile", got[0])
	}

	byLoginResp, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_Login{Login: env.viewer.Login}}))
	if err != nil {
		t.Fatalf("GetUser by login: %v", err)
	}
	if byLoginResp.Msg.GetUser().GetUser().GetId() != env.viewer.Id {
		t.Fatalf("GetUser by login id = %q, want %q", byLoginResp.Msg.GetUser().GetUser().GetId(), env.viewer.Id)
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_Login{Login: "missing-user"}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser by login code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{Target: &apiv1.GetUserRequest_UserId{UserId: "missing-user"}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUser code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.users.GetUser(ctx, connect.NewRequest(&apiv1.GetUserRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("missing GetUser target code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestAdminRoleServiceManagesRoles(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.roles.ListRoles(env.ctx, connect.NewRequest(&adminv1.ListRolesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.ListRoles(env.ctx, connect.NewRequest(&apiv1.ListRolesRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public ListRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.GetRole(env.ctx, connect.NewRequest(&apiv1.GetRoleRequest{Name: core.RoleEveryone})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public GetRole code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.publicRoles.BatchGetRoles(env.ctx, connect.NewRequest(&apiv1.BatchGetRolesRequest{Names: []string{core.RoleEveryone}})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated public BatchGetRoles code = %v, want unauthenticated", connect.CodeOf(err))
	}

	publicListResp, err := env.publicRoles.ListRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRolesRequest{}))
	if err != nil {
		t.Fatalf("public ListRoles regular: %v", err)
	}
	if len(publicListResp.Msg.GetRoles()) < 4 {
		t.Fatalf("public ListRoles regular len = %d, want default roles", len(publicListResp.Msg.GetRoles()))
	}
	if publicListResp.Msg.GetRoles()[0].GetName() == "" {
		t.Fatalf("public ListRoles first role = %+v, want role metadata", publicListResp.Msg.GetRoles()[0])
	}

	listResp, err := env.roles.ListRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListRolesRequest{}))
	if err != nil {
		t.Fatalf("ListRoles regular: %v", err)
	}
	if len(listResp.Msg.GetRoles()) < 4 {
		t.Fatalf("ListRoles regular len = %d, want default roles", len(listResp.Msg.GetRoles()))
	}
	if listResp.Msg.GetViewerCanManageRoles() || listResp.Msg.GetViewerCanAssignRoles() {
		t.Fatalf("regular capabilities manage=%v assign=%v, want false/false", listResp.Msg.GetViewerCanManageRoles(), listResp.Msg.GetViewerCanAssignRoles())
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Helpdesk",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular CreateRole code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "InvalidName",
		DisplayName: "Invalid",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid CreateRole code = %v, want invalid argument", connect.CodeOf(err))
	}

	createResp, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Helpdesk",
		Description: "Support queue",
		Pingable:    true,
	}))
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if got := createResp.Msg.GetRole().GetRole(); got.GetName() != "helpdesk" || !got.GetPingable() {
		t.Fatalf("created role = %+v, want helpdesk pingable", got)
	}

	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "helpdesk",
		DisplayName: "Duplicate",
	})); connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("duplicate CreateRole code = %v, want already exists", connect.CodeOf(err))
	}
	if _, err := env.roles.CreateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoleRequest{
		Name:        "triage",
		DisplayName: "Triage",
	})); err != nil {
		t.Fatalf("CreateRole triage: %v", err)
	}
	publicGetResp, err := env.publicRoles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("public GetRole: %v", err)
	}
	if got := publicGetResp.Msg.GetRole(); got.GetName() != "helpdesk" || got.GetDisplayName() != "Helpdesk" || !got.GetPingable() {
		t.Fatalf("public GetRole role = %+v, want helpdesk metadata", got)
	}
	if _, err := env.publicRoles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.GetRoleRequest{Name: "missing-role"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing public GetRole code = %v, want not found", connect.CodeOf(err))
	}
	publicBatchResp, err := env.publicRoles.BatchGetRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.BatchGetRolesRequest{
		Names: []string{"helpdesk", "missing-role", core.RoleEveryone, "helpdesk"},
	}))
	if err != nil {
		t.Fatalf("public BatchGetRoles: %v", err)
	}
	if got := publicBatchResp.Msg.GetRoles(); len(got) != 2 || got[0].GetName() != "helpdesk" || got[1].GetName() != core.RoleEveryone {
		t.Fatalf("public BatchGetRoles roles = %+v, want helpdesk,everyone", got)
	}
	reorderResp, err := env.roles.ReorderRoles(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ReorderRolesRequest{
		RoleNames: []string{"triage", "helpdesk"},
	}))
	if err != nil {
		t.Fatalf("ReorderRoles: %v", err)
	}
	var customOrder []string
	for _, role := range reorderResp.Msg.GetRoles() {
		if !role.GetRole().GetIsSystem() {
			customOrder = append(customOrder, role.GetRole().GetName())
		}
	}
	if strings.Join(customOrder, ",") != "triage,helpdesk" {
		t.Fatalf("custom role order = %v, want triage,helpdesk", customOrder)
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "role-service-member", "Role Service Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, member.Id, "helpdesk"); err != nil {
		t.Fatalf("AssignServerRole helpdesk: %v", err)
	}

	getResp, err := env.roles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if !getResp.Msg.GetViewerCanManageRoles() || !getResp.Msg.GetViewerCanAssignRoles() {
		t.Fatalf("GetRole capabilities manage=%v assign=%v, want true/true", getResp.Msg.GetViewerCanManageRoles(), getResp.Msg.GetViewerCanAssignRoles())
	}
	if len(getResp.Msg.GetUsers()) != 1 || getResp.Msg.GetUsers()[0].GetId() != member.Id {
		t.Fatalf("GetRole users = %+v, want member %s", getResp.Msg.GetUsers(), member.Id)
	}
	if _, err := env.roles.GetRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRoleRequest{Name: "missing-role"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetRole code = %v, want not found", connect.CodeOf(err))
	}

	if _, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name: "helpdesk",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateRole code = %v, want invalid argument", connect.CodeOf(err))
	}
	pingable := false
	updateResp, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name:        "helpdesk",
		DisplayName: stringPtr("Support"),
		Description: stringPtr("Support team"),
		Pingable:    &pingable,
	}))
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updateResp.Msg.GetRole().GetRole().GetDisplayName() != "Support" || updateResp.Msg.GetRole().GetRole().GetPingable() {
		t.Fatalf("updated role = %+v, want Support pingable false", updateResp.Msg.GetRole())
	}
	partialRoleResp, err := env.roles.UpdateRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoleRequest{
		Name:        "helpdesk",
		Description: stringPtr("Escalation queue"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRole: %v", err)
	}
	if got := partialRoleResp.Msg.GetRole().GetRole(); got.GetDisplayName() != "Support" || got.GetDescription() != "Escalation queue" || got.GetPingable() {
		t.Fatalf("partial role = %+v, want preserved display/pingable and updated description", got)
	}

	if _, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{
		Name: core.RoleOwner,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("DeleteRole owner code = %v, want failed precondition", connect.CodeOf(err))
	}
	deleteResp, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{Name: "helpdesk"}))
	if err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if !deleteResp.Msg.GetDeleted() {
		t.Fatal("DeleteRole Deleted = false, want true")
	}
	if _, err := env.roles.DeleteRole(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.DeleteRoleRequest{Name: "triage"})); err != nil {
		t.Fatalf("DeleteRole triage: %v", err)
	}
}

func TestAdminPermissionServiceMatricesAndWrites(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.permissions.GetRolePermissionTierMatrix(env.ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetRolePermissionTierMatrix code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.permissions.GetRolePermissionTierMatrix(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular GetRolePermissionTierMatrix code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	tierResp, err := env.permissions.GetRolePermissionTierMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix: %v", err)
	}
	if len(tierResp.Msg.GetMatrix().GetRoles()) == 0 || len(tierResp.Msg.GetMatrix().GetApplicablePermissions()) == 0 {
		t.Fatalf("tier matrix = %+v, want roles and permissions", tierResp.Msg.GetMatrix())
	}
	emptyScopeTierResp, err := env.permissions.GetRolePermissionTierMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{
		Scope: &adminv1.PermissionScope{},
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix empty scope: %v", err)
	}
	if len(emptyScopeTierResp.Msg.GetMatrix().GetRoles()) == 0 || len(emptyScopeTierResp.Msg.GetMatrix().GetApplicablePermissions()) == 0 {
		t.Fatalf("empty-scope tier matrix = %+v, want roles and permissions", emptyScopeTierResp.Msg.GetMatrix())
	}

	setResp, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
		Scope:      &adminv1.PermissionScope{},
	}))
	if err != nil {
		t.Fatalf("SetRolePermission empty scope allow: %v", err)
	}
	if decision := setResp.Msg.GetDecision(); decision.GetPermission() != string(core.PermMessagePost) || decision.GetDecision() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW || decision.GetScope().GetKind() != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER {
		t.Fatalf("SetRolePermission decision = %+v, want server allow", decision)
	}
	roleMatrixResp, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionMatrix: %v", err)
	}
	if cell := findAPIPermissionCell(roleMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermMessagePost)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW {
		t.Fatalf("server message.post cell = %+v, want allow override", cell)
	}
	roleDecisionsResp, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("ListRolePermissionDecisions: %v", err)
	}
	if roleDecisionsResp.Msg.GetRoleName() != core.RoleModerator {
		t.Fatalf("role decisions role name = %q, want %q", roleDecisionsResp.Msg.GetRoleName(), core.RoleModerator)
	}
	if decision := findAPIPermissionDecision(roleDecisionsResp.Msg.GetDecisions(), adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, "", string(core.PermMessagePost)); decision == nil || decision.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW {
		t.Fatalf("server message.post decision = %+v, want allow override", decision)
	}
	if _, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: "missing-role",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetRolePermissionMatrix code = %v, want not found", connect.CodeOf(err))
	}
	if _, err := env.permissions.SetRolePermission(env.ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_NONE,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated SetRolePermission clear code = %v, want unauthenticated", connect.CodeOf(err))
	}
	clearResp, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_NONE,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	}))
	if err != nil {
		t.Fatalf("SetRolePermission clear: %v", err)
	}
	if decision := clearResp.Msg.GetDecision(); decision.GetPermission() != string(core.PermMessagePost) || decision.GetDecision() != adminv1.PermissionDecision_PERMISSION_DECISION_NONE || decision.GetScope().GetKind() != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER {
		t.Fatalf("SetRolePermission clear decision = %+v, want server none", decision)
	}
	roleMatrixResp, err = env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
		RoleName: core.RoleModerator,
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionMatrix after revoke: %v", err)
	}
	if cell := findAPIPermissionCell(roleMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermMessagePost)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_NONE {
		t.Fatalf("server message.post cell after revoke = %+v, want no override", cell)
	}
	if _, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleModerator,
		Permission: string(core.PermMessagePost),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind(99), Id: "future"},
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("future scope SetRolePermission code = %v, want invalid_argument", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermUserManagePermissions); err != nil {
		t.Fatalf("GrantUserPermission user.manage-permissions: %v", err)
	}
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-target", "Permission Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := env.permissions.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{
		UserId:     target.Id,
		Permission: string(core.PermAdminUsersView),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_DENY,
		Scope:      &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
	})); err != nil {
		t.Fatalf("SetUserPermission server deny: %v", err)
	}
	userMatrixResp, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("GetUserPermissionMatrix: %v", err)
	}
	if cell := findAPIPermissionCell(userMatrixResp.Msg.GetMatrix().GetCells(), "server", string(core.PermAdminUsersView)); cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY {
		t.Fatalf("user server admin.users.view cell = %+v, want deny override", cell)
	}
	if _, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
		UserId: "missing-user",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetUserPermissionMatrix code = %v, want not found", connect.CodeOf(err))
	}
	userDecisionsResp, err := env.permissions.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{
		UserId: target.Id,
	}))
	if err != nil {
		t.Fatalf("ListUserPermissionDecisions: %v", err)
	}
	if userDecisionsResp.Msg.GetUserId() != target.Id {
		t.Fatalf("user decisions user ID = %q, want %q", userDecisionsResp.Msg.GetUserId(), target.Id)
	}
	if decision := findAPIPermissionDecision(userDecisionsResp.Msg.GetDecisions(), adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, "", string(core.PermAdminUsersView)); decision == nil || decision.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_DENY {
		t.Fatalf("user server admin.users.view decision = %+v, want deny override", decision)
	}
	if _, err := env.permissions.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{
		UserId: "missing-user",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing ListUserPermissionDecisions code = %v, want not found", connect.CodeOf(err))
	}
	if _, err := env.permissions.ExplainPermissions(env.ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ExplainPermissions code = %v, want unauthenticated", connect.CodeOf(err))
	}
	if _, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: env.viewer.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
	unprivileged, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-unprivileged", "Permission Unprivileged", "password")
	if err != nil {
		t.Fatalf("CreateUser unprivileged: %v", err)
	}
	if _, err := env.permissions.ExplainPermissions(withCaller(env.ctx, unprivileged), connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("unprivileged ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
	explainResp, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id}))
	if err != nil {
		t.Fatalf("ExplainPermissions: %v", err)
	}
	if len(explainResp.Msg.GetExplanations()) == 0 {
		t.Fatal("ExplainPermissions returned no explanations")
	}

	roomManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-room-manager", "Permission Room Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser room manager: %v", err)
	}
	room := env.createJoinedRoom("permission-room")
	if _, err := env.core.JoinRoom(env.ctx, roomManager.Id, core.KindChannel, roomManager.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom room manager: %v", err)
	}
	if err := env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, roomManager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	roomManagerCtx := withCaller(env.ctx, roomManager)
	if _, err := env.permissions.SetRolePermission(roomManagerCtx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleEveryone,
		Permission: string(core.PermMessageReact),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_DENY,
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM,
			Id:   room.Id,
		},
	})); err != nil {
		t.Fatalf("SetRolePermission room manager deny: %v", err)
	}
	roomTierResp, err := env.permissions.GetRolePermissionTierMatrix(roomManagerCtx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM,
			Id:   room.Id,
		},
	}))
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix room manager: %v", err)
	}
	everyone := findAPITierRole(roomTierResp.Msg.GetMatrix().GetRoles(), core.RoleEveryone)
	if everyone == nil || !stringSliceContains(everyone.GetOverride().GetPermissionDenials(), string(core.PermMessageReact)) {
		t.Fatalf("everyone room override = %+v, want message.react denial", everyone)
	}
	groupManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "permission-group-manager", "Permission Group Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser group manager: %v", err)
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, room.GetGroupId(), groupManager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	groupManagerCtx := withCaller(env.ctx, groupManager)
	if _, err := env.permissions.SetRolePermission(groupManagerCtx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName:   core.RoleEveryone,
		Permission: string(core.PermMessageReact),
		Decision:   adminv1.PermissionDecision_PERMISSION_DECISION_DENY,
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP,
			Id:   room.GetGroupId(),
		},
	})); err != nil {
		t.Fatalf("SetRolePermission group manager deny: %v", err)
	}
	if _, err := env.permissions.GetRolePermissionTierMatrix(groupManagerCtx, connect.NewRequest(&adminv1.GetRolePermissionTierMatrixRequest{
		Scope: &adminv1.PermissionScope{
			Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP,
			Id:   room.GetGroupId(),
		},
	})); err != nil {
		t.Fatalf("GetRolePermissionTierMatrix group manager: %v", err)
	}
	roomExplainResp, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{
		UserId: target.Id,
		RoomId: room.Id,
	}))
	if err != nil {
		t.Fatalf("ExplainPermissions room: %v", err)
	}
	if len(roomExplainResp.Msg.GetExplanations()) == 0 {
		t.Fatal("ExplainPermissions room returned no explanations")
	}
	if _, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{
		UserId: target.Id,
		RoomId: "missing-room",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("missing room ExplainPermissions code = %v, want permission denied", connect.CodeOf(err))
	}
}

func TestAdminDiagnosticsServiceGetSystemInfoRequiresOwner(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.adminDiagnostics.GetSystemInfo(env.ctx, connect.NewRequest(&adminv1.GetSystemInfoRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated GetSystemInfo code = %v, want unauthenticated", connect.CodeOf(err))
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "diagnostics-member", "Diagnostics Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := env.adminDiagnostics.GetSystemInfo(withCaller(env.ctx, member), connect.NewRequest(&adminv1.GetSystemInfoRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-owner GetSystemInfo code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, env.viewer.Id, core.RoleOwner); err != nil {
		t.Fatalf("AssignServerRole owner: %v", err)
	}
	resp, err := env.adminDiagnostics.GetSystemInfo(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.GetSystemInfoRequest{}))
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}

	if resp.Msg.GetSystemInfo().GetConnection() == nil {
		t.Fatal("Connection = nil")
	}
	if resp.Msg.GetSystemInfo().GetAccount() == nil {
		t.Fatal("Account = nil")
	}
	if resp.Msg.GetSystemInfo().GetNats() == nil {
		t.Fatal("Nats = nil")
	}
	if resp.Msg.GetSystemInfo().GetStats() == nil {
		t.Fatal("Stats = nil")
	}
	if len(resp.Msg.GetProjections()) == 0 {
		t.Fatal("Projections len = 0, want projection diagnostics")
	}
	if !resp.Msg.GetProjectionsAvailable() {
		t.Fatal("ProjectionsAvailable = false, want true")
	}
	if resp.Msg.GetAssetCleanup() == nil {
		t.Fatal("AssetCleanup = nil")
	}
	if len(resp.Msg.GetDurableWorkers()) != 4 {
		t.Fatalf("DurableWorkers len = %d, want 4", len(resp.Msg.GetDurableWorkers()))
	}
}

func TestAdminDurableWorkerStatusMapping(t *testing.T) {
	mapped := adminDurableWorkerStatuses([]core.DurableWorkerAdminStatus{{
		Key:                   "call_key_cleanup",
		Health:                core.DurableWorkerHealthWorking,
		PendingCount:          3,
		AckPendingCount:       1,
		WaitingCount:          1,
		RedeliveredCount:      2,
		LastDeliveredSequence: 44,
		AckFloorSequence:      41,
	}})
	if len(mapped) != 1 {
		t.Fatalf("mapped len = %d, want 1", len(mapped))
	}
	if mapped[0].GetHealth() != adminv1.AdminDurableWorkerHealth_ADMIN_DURABLE_WORKER_HEALTH_WORKING || mapped[0].GetPendingCount() != "3" {
		t.Fatalf("mapped status = %+v", mapped[0])
	}
	if mapped[0].GetLastDeliveredSequence() != "44" || mapped[0].GetAckFloorSequence() != "41" {
		t.Fatalf("mapped sequences = %+v", mapped[0])
	}
}

func TestAdminNatsStatsPreservesUnavailablePresence(t *testing.T) {
	if got := adminNatsStats(nil); got != nil {
		t.Fatalf("adminNatsStats(nil) = %+v, want nil", got)
	}
}

func TestAdminAssetCleanupStatusMapping(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mapped := adminAssetCleanupStatus(core.AssetCleanupAdminStatus{
		Health:               core.AssetCleanupHealthRetrying,
		PendingCount:         2,
		OldestPendingAt:      now.Add(-time.Hour),
		PassInProgress:       true,
		LastPassAt:           now.Add(-time.Minute),
		LastSuccessfulPassAt: now.Add(-2 * time.Hour),
		UpdatedAt:            now,
		LastPassFailed:       true,
		LastInspectedSeq:     41,
		LatestDeletionSeq:    44,
	})
	if mapped.GetHealth() != adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_RETRYING {
		t.Fatalf("health = %v, want retrying", mapped.GetHealth())
	}
	if mapped.GetPendingCount() != 2 || mapped.GetLastInspectedSequence() != "41" || mapped.GetLatestDeletionSequence() != "44" {
		t.Fatalf("mapped status = %+v", mapped)
	}
	if !mapped.GetUpdatedAt().AsTime().Equal(now) || !mapped.GetOldestPendingAt().AsTime().Equal(now.Add(-time.Hour)) {
		t.Fatalf("mapped timestamps = %+v", mapped)
	}
}

func TestAdminAssetCleanupUnavailableStatusMapping(t *testing.T) {
	mapped := adminAssetCleanupStatus(core.AssetCleanupAdminStatus{
		Health: core.AssetCleanupHealthUnavailable,
	})
	if mapped.GetHealth() != adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_UNAVAILABLE {
		t.Fatalf("health = %v, want unavailable", mapped.GetHealth())
	}
}

func TestAdminEventLogServiceListsFiltersAndReadsEntries(t *testing.T) {
	env := newConnectAPITestEnv(t)

	if _, err := env.adminEventLog.ListEvents(env.ctx, connect.NewRequest(&adminv1.ListEventsRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ListEvents code = %v, want unauthenticated", connect.CodeOf(err))
	}

	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "event-log-member", "Event Log Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := env.adminEventLog.ListEvents(withCaller(env.ctx, member), connect.NewRequest(&adminv1.ListEventsRequest{})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("non-auditor ListEvents code = %v, want permission denied", connect.CodeOf(err))
	}

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermAdminAuditView); err != nil {
		t.Fatalf("GrantUserPermission admin.view-audit: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("event-log-connect")
	actor, err := env.core.CreateUser(env.ctx, core.SystemActorID, "event-log-actor", "Event Log Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, actor.Id, core.KindChannel, actor.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom actor: %v", err)
	}

	resp, err := env.adminEventLog.ListEvents(ctx, connect.NewRequest(&adminv1.ListEventsRequest{
		Limit: 2,
		Filter: &adminv1.AdminEventLogFilter{
			EventType: "UserJoinedRoomEvent",
			ActorId:   actor.Id,
		},
	}))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(resp.Msg.GetEntries()) != 1 {
		t.Fatalf("filtered entries len = %d, want 1 (%+v)", len(resp.Msg.GetEntries()), resp.Msg.GetEntries())
	}
	entry := resp.Msg.GetEntries()[0]
	if entry.GetEventType() != "UserJoinedRoomEvent" || entry.GetActorId() != actor.Id || entry.GetCreatedAt() == nil {
		t.Fatalf("filtered event entry = %+v, want actor join event", entry)
	}
	if resp.Msg.GetScanLimit() != core.FilteredEventLogScanLimit || resp.Msg.GetScannedCount() <= 0 {
		t.Fatalf("scan metadata = limit %d scanned %d, want filtered scan metadata", resp.Msg.GetScanLimit(), resp.Msg.GetScannedCount())
	}

	typesResp, err := env.adminEventLog.ListEventTypes(ctx, connect.NewRequest(&adminv1.ListEventTypesRequest{}))
	if err != nil {
		t.Fatalf("ListEventTypes: %v", err)
	}
	if !stringSliceContains(typesResp.Msg.GetEventTypes(), "UserJoinedRoomEvent") || !stringSliceContains(typesResp.Msg.GetEventTypes(), "decode-error") {
		t.Fatalf("event types = %v, want joined-room and decode-error", typesResp.Msg.GetEventTypes())
	}

	getResp, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: entry.GetSequence()}))
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if getResp.Msg.GetEntry().GetSequence() != entry.GetSequence() || getResp.Msg.GetEntry().GetPayloadJson() == "" {
		t.Fatalf("GetEvent entry = %+v, want payload for sequence %s", getResp.Msg.GetEntry(), entry.GetSequence())
	}

	if _, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: "9999999"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetEvent code = %v, want not_found", connect.CodeOf(err))
	}
	if _, err := env.adminEventLog.GetEvent(ctx, connect.NewRequest(&adminv1.GetEventRequest{Sequence: "not-a-number"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("invalid sequence code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestRoomDirectoryServiceListRoomGroupsIncludesSidebarItems(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	room := env.createJoinedRoom("layout-room")
	link, err := env.core.CreateSidebarLink(env.ctx, core.SystemActorID, groupID, "Docs", "/docs")
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}

	resp, err := env.directory.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRoomGroupsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomGroups: %v", err)
	}

	group := findDirectoryGroup(resp.Msg.GetGroups(), groupID)
	if group == nil {
		t.Fatalf("group %q missing from response", groupID)
	}
	if !roomGroupItemsContainRoom(group.GetItems(), room.Id) {
		t.Fatalf("room %q missing from group items", room.Id)
	}
	if !roomGroupItemsContainSidebarLink(group.GetItems(), link.Id) {
		t.Fatalf("sidebar link %q missing from group items", link.Id)
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	resp, err = env.directory.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRoomGroupsRequest{}))
	if err != nil {
		t.Fatalf("ListRoomGroups after room.manage grant: %v", err)
	}
	group = findDirectoryGroup(resp.Msg.GetGroups(), groupID)
	if group == nil {
		t.Fatalf("group %q missing after room.manage grant", groupID)
	}
	if !apiRoomGroupPermissionGranted(group, core.PermRoomManage) {
		t.Fatalf("group room.manage viewer grant = %+v, want granted", group.GetViewerState())
	}
}

func TestAdminRoomLayoutServiceCreateRoomGroupRequiresRoomManage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-member", "Layout Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	_, err = env.adminLayout.ListRoomGroups(withCaller(env.ctx, member), connect.NewRequest(&adminv1.ListRoomGroupsRequest{}))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	_, err = env.adminLayout.CreateRoomGroup(withCaller(env.ctx, member), connect.NewRequest(&adminv1.CreateRoomGroupRequest{
		Name:        "Operations",
		Description: "Private operations rooms",
	}))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission room.manage: %v", err)
	}
	if _, err := env.adminLayout.ListRoomGroups(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListRoomGroupsRequest{})); err != nil {
		t.Fatalf("ListRoomGroups: %v", err)
	}
	resp, err := env.adminLayout.CreateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.CreateRoomGroupRequest{
		Name:        "Operations",
		Description: "Private operations rooms",
	}))
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if resp.Msg.GetGroup().GetName() != "Operations" {
		t.Fatalf("group name = %q, want Operations", resp.Msg.GetGroup().GetName())
	}
	if _, err := env.adminLayout.UpdateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomGroupRequest{
		GroupId: resp.Msg.GetGroup().GetId(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateRoomGroup code = %v, want invalid argument", connect.CodeOf(err))
	}
	partialResp, err := env.adminLayout.UpdateRoomGroup(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateRoomGroupRequest{
		GroupId:     resp.Msg.GetGroup().GetId(),
		Description: stringPtr("Updated operations description"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateRoomGroup: %v", err)
	}
	if got := partialResp.Msg.GetGroup(); got.GetName() != "Operations" || got.GetDescription() != "Updated operations description" {
		t.Fatalf("partial group = %+v, want preserved name and updated description", got)
	}
}

func TestAdminRoomLayoutServiceManagementReadsDoNotRequireDirectoryVisibility(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, groupID, "private-managed-room", "Private")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission room.list: %v", err)
	}
	roleManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "private-role-manager", "Private Role Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser role manager: %v", err)
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, roleManager.Id, core.PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	ctx := withCaller(env.ctx, roleManager)
	if _, err := env.directory.GetRoom(ctx, connect.NewRequest(&apiv1.GetRoomRequest{RoomId: room.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("directory GetRoom code = %v, want permission_denied", connect.CodeOf(err))
	}
	roomResp, err := env.adminLayout.GetRoom(ctx, connect.NewRequest(&adminv1.GetRoomRequest{RoomId: room.Id}))
	if err != nil {
		t.Fatalf("admin layout GetRoom: %v", err)
	}
	if roomResp.Msg.GetRoom().GetId() != room.Id || roomResp.Msg.GetViewerCanManageRoom() || !roomResp.Msg.GetViewerCanManagePermissions() {
		t.Fatalf("GetRoom response = %+v, want permission-only management access", roomResp.Msg)
	}
	groupResp, err := env.adminLayout.GetRoomGroup(ctx, connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: groupID}))
	if err != nil {
		t.Fatalf("admin layout GetRoomGroup role manager: %v", err)
	}
	if groupResp.Msg.GetViewerCanManageGroup() || !groupResp.Msg.GetViewerCanManagePermissions() {
		t.Fatalf("GetRoomGroup role-manager capabilities = %+v", groupResp.Msg)
	}
	if len(groupResp.Msg.GetGroup().GetItems()) != 0 {
		t.Fatalf("GetRoomGroup exposed %d private layout items to role manager", len(groupResp.Msg.GetGroup().GetItems()))
	}

	groupManager, err := env.core.CreateUser(env.ctx, core.SystemActorID, "private-group-manager", "Private Group Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser group manager: %v", err)
	}
	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, groupManager.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	groupResp, err = env.adminLayout.GetRoomGroup(withCaller(env.ctx, groupManager), connect.NewRequest(&adminv1.GetRoomGroupRequest{GroupId: groupID}))
	if err != nil {
		t.Fatalf("admin layout GetRoomGroup group manager: %v", err)
	}
	if !groupResp.Msg.GetViewerCanManageGroup() || !groupResp.Msg.GetViewerCanManagePermissions() {
		t.Fatalf("GetRoomGroup group-manager capabilities = %+v", groupResp.Msg)
	}
}

func TestAdminRoomLayoutServiceCreateSidebarLinkRequiresRoomManage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	groupID := env.defaultRoomGroupID(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "layout-link-member", "Layout Link Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}

	req := &adminv1.CreateSidebarLinkRequest{GroupId: groupID, Label: "Status", Url: "/status"}
	_, err = env.adminLayout.CreateSidebarLink(withCaller(env.ctx, member), connect.NewRequest(req))
	requireConnectCode(t, err, connect.CodePermissionDenied)

	if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, groupID, env.viewer.Id, core.PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	resp, err := env.adminLayout.CreateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("CreateSidebarLink: %v", err)
	}
	if resp.Msg.GetSidebarLink().GetUrl() != "/status" {
		t.Fatalf("sidebar link URL = %q, want /status", resp.Msg.GetSidebarLink().GetUrl())
	}
	if _, err := env.adminLayout.UpdateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateSidebarLinkRequest{
		LinkId: resp.Msg.GetSidebarLink().GetId(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty UpdateSidebarLink code = %v, want invalid argument", connect.CodeOf(err))
	}
	partialResp, err := env.adminLayout.UpdateSidebarLink(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.UpdateSidebarLinkRequest{
		LinkId: resp.Msg.GetSidebarLink().GetId(),
		Url:    stringPtr("/health"),
	}))
	if err != nil {
		t.Fatalf("partial UpdateSidebarLink: %v", err)
	}
	if got := partialResp.Msg.GetSidebarLink(); got.GetLabel() != "Status" || got.GetUrl() != "/health" {
		t.Fatalf("partial sidebar link = %+v, want preserved label and updated URL", got)
	}
}
