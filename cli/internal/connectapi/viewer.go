package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (a *API) buildViewer(ctx context.Context, userID string) (*apiv1.GetViewerResponse, error) {
	user, err := a.core.GetUser(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}

	// Assemble independent projection and runtime-state reads concurrently so
	// one slow source does not serialize the entire viewer response.
	var (
		responseUser      *apiv1.ViewerUser
		capabilities      *apiv1.ViewerCapabilities
		viewerPermissions *apiv1.ServerViewerPermissions
		viewerState       *apiv1.ServerViewerState
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
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return &apiv1.GetViewerResponse{
		User:              responseUser,
		Capabilities:      capabilities,
		ViewerPermissions: viewerPermissions,
		ViewerState:       viewerState,
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
	response := &apiv1.UserSettings{TimeFormat: apiv1.TimeFormat_TIME_FORMAT_AUTO}
	if settings == nil {
		return response
	}
	if settings.Timezone != nil {
		response.Timezone = settings.Timezone
	}
	response.TimeFormat = coreTimeFormatToAPI(settings.GetTimeFormat())
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
