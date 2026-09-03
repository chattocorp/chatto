package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	viewerCapabilityManageInvites    = string(core.PermUserInvite)
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
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if credential, ok := authctx.CredentialForContext(ctx); ok && credential.Kind == authctx.RuntimeCredentialKindBotAPIKey {
		return nil, connectError(core.ErrHumanAccountRequired)
	}
	available, err := s.api.core.HasAnyPrivilegedModeEntitlement(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	if !available {
		return nil, connectError(core.ErrPrivilegedModeUnavailable)
	}
	deadline, err := s.api.setPrivilegedMode(ctx, caller.UserID, true)
	if err != nil {
		return nil, connectError(err)
	}
	ctx = privilegedModeContext(ctx, deadline)
	viewer, err := s.api.buildViewer(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ActivatePrivilegedModeResponse{
		PrivilegedMode:    privilegedModeState(true, deadline),
		Capabilities:      viewer.GetCapabilities(),
		ViewerPermissions: viewer.GetViewerPermissions(),
	}), nil
}

func (s *viewerService) DeactivatePrivilegedMode(ctx context.Context, _ *connect.Request[apiv1.DeactivatePrivilegedModeRequest]) (*connect.Response[apiv1.DeactivatePrivilegedModeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.api.setPrivilegedMode(ctx, caller.UserID, false); err != nil {
		return nil, connectError(err)
	}
	available, err := s.api.core.HasAnyPrivilegedModeEntitlement(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	ctx = privilegedModeContext(ctx, time.Time{})
	viewer, err := s.api.buildViewer(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.DeactivatePrivilegedModeResponse{
		PrivilegedMode:    privilegedModeState(available, time.Time{}),
		Capabilities:      viewer.GetCapabilities(),
		ViewerPermissions: viewer.GetViewerPermissions(),
	}), nil
}

func privilegedModeContext(ctx context.Context, deadline time.Time) context.Context {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok {
		return ctx
	}
	credential.PrivilegedModeExpiresAt = deadline
	return authctx.WithCredential(ctx, credential)
}

func (a *API) setPrivilegedMode(ctx context.Context, userID string, active bool) (time.Time, error) {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok || credential.UserID != userID {
		return time.Time{}, core.ErrNotAuthenticated
	}
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.SetBearerPrivilegedMode(ctx, credential.Handle, active)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.SetCookiePrivilegedMode(ctx, credential.Handle, active)
	case authctx.RuntimeCredentialKindBotAPIKey:
		return time.Time{}, core.ErrHumanAccountRequired
	default:
		return time.Time{}, core.ErrNotAuthenticated
	}
}

func privilegedModeState(available bool, deadline time.Time) *apiv1.PrivilegedModeState {
	active := available && time.Now().Before(deadline)
	state := &apiv1.PrivilegedModeState{Available: available, Active: active}
	if active {
		state.ExpiresAt = timestamppb.New(deadline)
	}
	return state
}

func (a *API) buildViewer(ctx context.Context, userID string) (*apiv1.GetViewerResponse, error) {
	user, err := a.core.GetUser(ctx, userID)
	if err != nil {
		return nil, connectError(err)
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
			privilegedMode = privilegedModeState(false, time.Time{})
			return nil
		}
		available, err := a.core.HasAnyPrivilegedModeEntitlement(groupCtx, userID)
		if err != nil {
			return connectError(err)
		}
		deadline := time.Time{}
		if hasCredential && credential.UserID == userID {
			deadline = credential.PrivilegedModeExpiresAt
		}
		privilegedMode = privilegedModeState(available, deadline)
		return nil
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

func viewerUser(ctx context.Context, api *API, user *evtv1.User) (*apiv1.ViewerUser, error) {
	var (
		hasVerifiedEmail bool
		settings         *evtv1.ServerUserPreferences
		apiUser          *apiv1.User
		canDeleteAccount bool
		lastLoginChange  time.Time
		hasPassword      bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		hasVerifiedEmail, err = api.core.HasVerifiedEmail(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		settings, err = api.core.GetUserSettings(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		apiUser, err = userSummary(groupCtx, api, user, nil)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canDeleteAccount, err = api.core.CanDeleteUser(groupCtx, user.GetId(), user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		lastLoginChange, err = api.core.GetLastLoginChange(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		hasPassword, err = api.core.HasPassword(groupCtx, user.GetId())
		return connectError(err)
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	response := &apiv1.ViewerUser{
		HasVerifiedEmail:       hasVerifiedEmail,
		HasPassword:            hasPassword,
		Settings:               coreUserSettingsToAPI(settings),
		ViewerCanDeleteAccount: canDeleteAccount,
		Profile:                apiUser,
	}
	if !lastLoginChange.IsZero() {
		response.LastLoginChange = timestamppb.New(lastLoginChange)
	}

	return response, nil
}

func viewerCapabilities(ctx context.Context, api *API, userID string) (*apiv1.ViewerCapabilities, error) {
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
		hasUnreadFollowedThreads bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		canManageInvites, err = api.core.HasServerPermission(groupCtx, userID, core.PermUserInvite)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canViewAdmin, err = api.core.HasAnyAdminPermission(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canStartDMs, err = api.core.CanStartDM(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAdminViewUsers, err = api.core.CanAdminUsersView(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAdminManageAccounts, err = api.core.CanManageUserAccounts(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAssignRoles, err = api.core.CanAssignRoles(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAdminManageRoles, err = api.core.CanManageRoles(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canManageUserPermissions, err = api.core.CanManageUserPermissions(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAdminViewSystem, err = api.core.CanAdminSystemView(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canAdminViewAudit, err = api.core.CanAdminAuditView(groupCtx, userID)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		hasUnreadFollowedThreads, err = api.core.HasUnreadFollowedThreads(groupCtx, userID, []string{core.LegacySpaceIDForRoomKind(core.KindChannel)})
		return connectError(err)
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	canAdminViewRoles := canAdminManageRoles || canAssignRoles || canManageUserPermissions

	return &apiv1.ViewerCapabilities{
		Grants: []*apiv1.CapabilityGrant{
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
		},
		HasUnreadFollowedThreads: hasUnreadFollowedThreads,
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

func stringPtr(value string) *string {
	return &value
}
