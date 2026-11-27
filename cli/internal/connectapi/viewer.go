package connectapi

import (
	"context"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type viewerService struct {
	api *API
}

const (
	viewerCapabilityAdminView        = "admin.view"
	viewerCapabilityDMStart          = "dm.start"
	viewerCapabilityAdminViewUsers   = string(core.PermAdminUsersView)
	viewerCapabilityAdminManageUsers = string(core.PermUserManageAccounts)
	viewerCapabilityAssignRoles      = string(core.PermRoleAssign)
	viewerCapabilityAdminViewRoles   = "role.view"
	viewerCapabilityAdminManageRoles = string(core.PermRoleManage)
	viewerCapabilityAdminViewSystem  = "admin.view-system"
	viewerCapabilityAdminViewAudit   = string(core.PermAdminAuditView)
	viewerCapabilityManageUserPerms  = string(core.PermUserManagePermissions)
	viewerCapabilityManageInvites    = string(core.PermUserManageInvites)
)

func (s *viewerService) GetViewer(ctx context.Context, _ *connect.Request[apiv1.GetViewerRequest]) (*connect.Response[apiv1.GetViewerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	response, err := s.api.buildViewer(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(response), nil
}

func (s *viewerService) ActivatePrivilegedMode(ctx context.Context, _ *connect.Request[apiv1.ActivatePrivilegedModeRequest]) (*connect.Response[apiv1.ActivatePrivilegedModeResponse], error) {
	result, err := s.activatePrivilegedRoles(ctx, nil, core.ActivateAllPrivilegedRoles)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ActivatePrivilegedModeResponse{PrivilegedMode: result.PrivilegedMode, Capabilities: result.Capabilities, ViewerPermissions: result.ViewerPermissions}), nil
}

func (s *viewerService) ActivatePrivilegedRoles(ctx context.Context, req *connect.Request[apiv1.ActivatePrivilegedRolesRequest]) (*connect.Response[apiv1.ActivatePrivilegedRolesResponse], error) {
	result, err := s.activatePrivilegedRoles(ctx, req.Msg.RoleNames, core.ActivateSelectedPrivilegedRoles)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(result), nil
}

func (s *viewerService) activatePrivilegedRoles(ctx context.Context, requested []string, action core.PrivilegedModeAction) (*apiv1.ActivatePrivilegedRolesResponse, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if credential, ok := authctx.CredentialForContext(ctx); ok && credential.Kind == authctx.RuntimeCredentialKindBotAPIKey {
		return nil, core.ErrHumanAccountRequired
	}
	activation, err := s.api.setPrivilegedMode(ctx, caller.UserID, action, requested...)
	if err != nil {
		return nil, err
	}
	ctx = privilegedModeContext(ctx, activation.ExpiresAt, activation.Roles...)
	capabilities, permissions, err := s.api.buildViewerAuthorization(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	state := privilegedModeState(activation.ExpiresAt, s.api.core.ProtectedRoles(caller.UserID)...)
	if err := s.api.addPrivilegedPreview(ctx, caller.UserID, state); err != nil {
		return nil, err
	}
	return &apiv1.ActivatePrivilegedRolesResponse{
		PrivilegedMode:    state,
		Capabilities:      capabilities,
		ViewerPermissions: permissions,
	}, nil
}

func (s *viewerService) DeactivatePrivilegedMode(ctx context.Context, _ *connect.Request[apiv1.DeactivatePrivilegedModeRequest]) (*connect.Response[apiv1.DeactivatePrivilegedModeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.api.setPrivilegedMode(ctx, caller.UserID, core.DeactivatePrivilegedMode); err != nil {
		return nil, err
	}
	roles := s.api.core.ProtectedRoles(caller.UserID)
	ctx = privilegedModeContext(ctx, time.Time{})
	capabilities, permissions, err := s.api.buildViewerAuthorization(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	state := privilegedModeState(time.Time{}, roles...)
	if err := s.api.addPrivilegedPreview(ctx, caller.UserID, state); err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.DeactivatePrivilegedModeResponse{
		PrivilegedMode:    state,
		Capabilities:      capabilities,
		ViewerPermissions: permissions,
	}), nil
}

func privilegedModeContext(ctx context.Context, deadline time.Time, roles ...string) context.Context {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok {
		return ctx
	}
	credential.PrivilegedModeExpiresAt = deadline
	credential.PrivilegedRoles = roles
	return authctx.WithCredential(ctx, credential)
}

func (a *API) setPrivilegedMode(ctx context.Context, userID string, action core.PrivilegedModeAction, roles ...string) (core.PrivilegedModeActivation, error) {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok || credential.UserID != userID {
		return core.PrivilegedModeActivation{}, core.ErrNotAuthenticated
	}
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.SetBearerPrivilegedMode(ctx, credential.Handle, action, roles...)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.SetCookiePrivilegedMode(ctx, credential.Handle, action, roles...)
	case authctx.RuntimeCredentialKindBotAPIKey:
		return core.PrivilegedModeActivation{}, core.ErrHumanAccountRequired
	default:
		return core.PrivilegedModeActivation{}, core.ErrNotAuthenticated
	}
}

func privilegedModeState(deadline time.Time, roles ...string) *apiv1.PrivilegedModeState {
	available := len(roles) > 0
	active := time.Now().Before(deadline)
	state := &apiv1.PrivilegedModeState{Available: available, Active: active, RoleSelection: &apiv1.PrivilegedRoleSelection{AvailableRoles: roles}}
	if active {
		state.ExpiresAt = timestamppb.New(deadline)
	}
	return state
}

// addPrivilegedPreview evaluates only decisions, with a request-local role
// selection. It never changes the credential stored for this session.
func (a *API) addPrivilegedPreview(ctx context.Context, userID string, state *apiv1.PrivilegedModeState) error {
	if !state.Available {
		return nil
	}
	previewCtx := privilegedModeContext(ctx, time.Time{})
	if !state.Active {
		previewCtx = privilegedModeContext(ctx, time.Now().Add(core.PrivilegedModeWindow), state.RoleSelection.AvailableRoles...)
	}
	permissions, err := a.serverViewerPermissions(previewCtx, userID)
	if err != nil {
		return err
	}
	grants, err := viewerCapabilityGrants(previewCtx, a, userID)
	if err != nil {
		return err
	}
	preview := &apiv1.PrivilegedPermissionPreview{ViewerPermissions: permissions, Grants: grants}
	if state.Active {
		state.RoleSelection.WithoutActivation = preview
	} else {
		state.RoleSelection.Preview = preview
	}
	return nil
}

// buildViewerAuthorization assembles the fields returned by privilege mutations.
// It avoids profile, settings, and room-unread reads required only by GetViewer.
func (a *API) buildViewerAuthorization(ctx context.Context, userID string) (*apiv1.ViewerCapabilities, *apiv1.ServerViewerPermissions, error) {
	var capabilities *apiv1.ViewerCapabilities
	var permissions *apiv1.ServerViewerPermissions
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		capabilities, err = viewerCapabilities(groupCtx, a, userID)
		return err
	})
	group.Go(func() error {
		var err error
		permissions, err = a.serverViewerPermissions(groupCtx, userID)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}
	return capabilities, permissions, nil
}

func (a *API) buildViewer(ctx context.Context, userID string) (*apiv1.GetViewerResponse, error) {
	user, err := a.core.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	credential, hasCredential := authctx.CredentialForContext(ctx)

	// Assemble independent projection and runtime-state reads concurrently so
	// one slow source does not serialize the entire viewer response.
	var (
		responseUser      *apiv1.ViewerUser
		capabilities      *apiv1.ViewerCapabilities
		viewerPermissions *apiv1.ServerViewerPermissions
		viewerState       *apiv1.ServerViewerState
		privilegedMode    *apiv1.PrivilegedModeState
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		responseUser, err = viewerUser(groupCtx, a, user)
		return err
	})
	group.Go(func() error {
		var err error
		capabilities, err = viewerCapabilities(groupCtx, a, userID)
		return err
	})
	group.Go(func() error {
		var err error
		viewerPermissions, viewerState, err = a.serverViewerState(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		if hasCredential && credential.Kind == authctx.RuntimeCredentialKindBotAPIKey {
			privilegedMode = privilegedModeState(time.Time{})
			return nil
		}
		roles := a.core.ProtectedRoles(userID)
		deadline := time.Time{}
		if hasCredential && credential.UserID == userID {
			deadline = credential.PrivilegedModeExpiresAt
		}
		privilegedMode = privilegedModeState(deadline, roles...)
		return a.addPrivilegedPreview(groupCtx, userID, privilegedMode)
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return &apiv1.GetViewerResponse{
		User:              responseUser,
		Capabilities:      capabilities,
		ViewerPermissions: viewerPermissions,
		ViewerState:       viewerState,
		PrivilegedMode:    privilegedMode,
	}, nil
}

func viewerUser(ctx context.Context, api *API, user *readmodel.User) (*apiv1.ViewerUser, error) {
	var (
		hasVerifiedEmail bool
		settings         *evtv1.ServerUserPreferences
		apiUser          *apiv1.User
		canDeleteAccount bool
		lastLoginChange  time.Time
		hasPassword      bool
		usernameManaged  bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if user.GetIsBot() {
			return nil
		}
		var err error
		usernameManaged, err = api.core.UsernameManaged(groupCtx, user.GetId())
		return err
	})
	group.Go(func() error {
		var err error
		hasVerifiedEmail, err = api.core.HasVerifiedEmail(groupCtx, user.GetId())
		return err
	})
	group.Go(func() error {
		var err error
		settings, err = api.core.GetUserSettings(groupCtx, user.GetId())
		return err
	})
	group.Go(func() error {
		var err error
		apiUser, err = userSummary(groupCtx, api, user, nil)
		return err
	})
	group.Go(func() error {
		var err error
		canDeleteAccount, err = api.core.CanDeleteUser(groupCtx, user.GetId(), user.GetId())
		return err
	})
	group.Go(func() error {
		var err error
		lastLoginChange, err = api.core.GetLastLoginChange(groupCtx, user.GetId())
		return err
	})
	group.Go(func() error {
		var err error
		hasPassword, err = api.core.HasPassword(groupCtx, user.GetId())
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	actions, err := accountActions(ctx, api, user, usernameManaged, canDeleteAccount, lastLoginChange)
	if err != nil {
		return nil, err
	}
	response := &apiv1.ViewerUser{
		HasVerifiedEmail:       hasVerifiedEmail,
		HasPassword:            hasPassword,
		Actions:                actions,
		Settings:               coreUserSettingsToAPI(settings),
		ViewerCanDeleteAccount: actions.GetDeleteAccount() == apiv1.AccountDeletionAvailability_ACCOUNT_DELETION_AVAILABILITY_ALLOWED,
		Profile:                apiUser,
	}
	if !lastLoginChange.IsZero() {
		response.LastLoginChange = timestamppb.New(lastLoginChange)
	}

	return response, nil
}

func viewerCapabilities(ctx context.Context, api *API, userID string) (*apiv1.ViewerCapabilities, error) {
	var grants []*apiv1.CapabilityGrant
	var hasUnreadFollowedThreads bool
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		grants, err = viewerCapabilityGrants(groupCtx, api, userID)
		return err
	})
	group.Go(func() error {
		var err error
		hasUnreadFollowedThreads, err = api.core.HasUnreadFollowedThreads(groupCtx, userID, []string{
			core.LegacySpaceIDForRoomKind(core.KindChannel),
			core.LegacySpaceIDForRoomKind(core.KindDM),
		})
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return &apiv1.ViewerCapabilities{Grants: grants, HasUnreadFollowedThreads: hasUnreadFollowedThreads}, nil
}

// viewerCapabilityGrants resolves capability decisions without reading resource state.
func viewerCapabilityGrants(ctx context.Context, api *API, userID string) ([]*apiv1.CapabilityGrant, error) {
	var (
		canViewAdmin             bool
		canStartDMs              bool
		canAdminViewUsers        bool
		canAdminManageAccounts   bool
		canAssignRoles           bool
		canAdminManageRoles      bool
		canManageUserPermissions bool
		canAdminViewSystem       bool
		canAdminViewAudit        bool
		canManageInvites         bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		canManageInvites, err = api.core.HasServerPermission(groupCtx, userID, core.PermUserManageInvites)
		return err
	})
	group.Go(func() error {
		var err error
		canViewAdmin, err = api.core.HasAnyAdminPermission(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canStartDMs, err = api.core.CanStartDM(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAdminViewUsers, err = api.core.CanAdminUsersView(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAdminManageAccounts, err = api.core.CanManageUserAccounts(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAssignRoles, err = api.core.CanAssignRoles(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAdminManageRoles, err = api.core.CanManageRoles(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canManageUserPermissions, err = api.core.CanManageUserPermissions(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAdminViewSystem, err = api.core.CanAdminSystemView(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		canAdminViewAudit, err = api.core.CanAdminAuditView(groupCtx, userID)
		return err
	})

	if err := group.Wait(); err != nil {
		return nil, err
	}
	canAdminViewRoles := canAdminManageRoles || canAssignRoles || canManageUserPermissions

	return []*apiv1.CapabilityGrant{
		{Capability: viewerCapabilityAdminView, Granted: canViewAdmin},
		{Capability: viewerCapabilityDMStart, Granted: canStartDMs},
		{Capability: viewerCapabilityAdminViewUsers, Granted: canAdminViewUsers},
		{Capability: viewerCapabilityAdminManageUsers, Granted: canAdminManageAccounts},
		{Capability: viewerCapabilityAssignRoles, Granted: canAssignRoles},
		{Capability: viewerCapabilityAdminViewRoles, Granted: canAdminViewRoles},
		{Capability: viewerCapabilityAdminManageRoles, Granted: canAdminManageRoles},
		{Capability: viewerCapabilityAdminViewSystem, Granted: canAdminViewSystem},
		{Capability: viewerCapabilityAdminViewAudit, Granted: canAdminViewAudit},
		{Capability: viewerCapabilityManageUserPerms, Granted: canManageUserPermissions},
		{Capability: viewerCapabilityManageInvites, Granted: canManageInvites},
	}, nil
}

func coreUserSettingsToAPI(settings *evtv1.ServerUserPreferences) *apiv1.UserSettings {
	shareTimezone := false
	response := &apiv1.UserSettings{
		TimeFormat:    apiv1.TimeFormat_TIME_FORMAT_AUTO,
		ShareTimezone: &shareTimezone,
	}
	if settings == nil {
		return response
	}
	if settings.Timezone != nil {
		response.Timezone = settings.Timezone
	}
	response.TimeFormat = coreTimeFormatToAPI(settings.GetTimeFormat())
	shareTimezone = settings.GetShareTimezone()
	response.ShareTimezone = &shareTimezone
	return response
}

func coreTimeFormatToAPI(format evtv1.TimeFormat) apiv1.TimeFormat {
	switch format {
	case evtv1.TimeFormat_TIME_FORMAT_12H:
		return apiv1.TimeFormat_TIME_FORMAT_12_HOUR
	case evtv1.TimeFormat_TIME_FORMAT_24H:
		return apiv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return apiv1.TimeFormat_TIME_FORMAT_AUTO
	}
}
