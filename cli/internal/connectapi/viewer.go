package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

	responseUser, err := viewerUser(ctx, a, user)
	if err != nil {
		return nil, err
	}
	capabilities, err := viewerCapabilities(ctx, a, userID)
	if err != nil {
		return nil, err
	}
	serverPreference, err := serverNotificationPreference(ctx, a, userID)
	if err != nil {
		return nil, err
	}
	roomPreferences, err := roomNotificationPreferences(ctx, a, userID)
	if err != nil {
		return nil, err
	}
	viewerPermissions, viewerState, err := a.serverViewerState(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apiv1.GetViewerResponse{
		User:                         responseUser,
		Capabilities:                 capabilities,
		ServerNotificationPreference: serverPreference,
		RoomNotificationPreferences:  roomPreferences,
		ViewerPermissions:            viewerPermissions,
		ViewerState:                  viewerState,
	}, nil
}

func viewerUser(ctx context.Context, api *API, user *corev1.User) (*apiv1.ViewerUser, error) {
	hasVerifiedEmail, err := api.core.HasVerifiedEmail(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	settings, err := api.core.GetUserSettings(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	apiUser, err := userSummary(ctx, api, user, nil)
	if err != nil {
		return nil, connectError(err)
	}
	canDeleteAccount, err := api.core.CanDeleteUser(ctx, user.GetId(), user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	lastLoginChange, err := api.core.GetLastLoginChange(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	hasPassword, err := api.core.HasPassword(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
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
	canViewAdmin, err := api.core.HasAnyAdminPermission(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canStartDMs, err := api.core.CanStartDM(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewUsers, err := api.core.CanAdminUsersView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminManageAccounts, err := api.core.CanManageUserAccounts(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAssignRoles, err := api.core.CanAssignRoles(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminManageRoles, err := api.core.CanManageRoles(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canManageUserPermissions, err := api.core.CanManageUserPermissions(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewRoles := canAdminManageRoles || canAssignRoles || canManageUserPermissions
	canAdminViewSystem, err := api.core.CanAdminSystemView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewAudit, err := api.core.CanAdminAuditView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	hasUnreadFollowedThreads, err := api.core.HasUnreadFollowedThreads(ctx, userID, []string{core.LegacySpaceIDForRoomKind(core.KindChannel)})
	if err != nil {
		return nil, connectError(err)
	}

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
		},
		HasUnreadFollowedThreads: hasUnreadFollowedThreads,
	}, nil
}

func serverNotificationPreference(ctx context.Context, api *API, userID string) (*apiv1.NotificationPreference, error) {
	level, err := api.core.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return apiNotificationPreference(level, effectiveLevel), nil
}

func roomNotificationPreferences(ctx context.Context, api *API, userID string) ([]*apiv1.RoomNotificationPreference, error) {
	prefs, err := api.core.GetAllRoomNotificationPreferences(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	result := make([]*apiv1.RoomNotificationPreference, 0, len(prefs))
	for _, pref := range prefs {
		result = append(result, &apiv1.RoomNotificationPreference{
			RoomId:     pref.RoomID,
			Preference: apiNotificationPreference(pref.Level, pref.EffectiveLevel),
		})
	}
	return result, nil
}

func coreUserSettingsToAPI(settings *corev1.ServerUserPreferences) *apiv1.UserSettings {
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

func coreTimeFormatToAPI(format corev1.TimeFormat) apiv1.TimeFormat {
	switch format {
	case corev1.TimeFormat_TIME_FORMAT_12H:
		return apiv1.TimeFormat_TIME_FORMAT_12_HOUR
	case corev1.TimeFormat_TIME_FORMAT_24H:
		return apiv1.TimeFormat_TIME_FORMAT_24_HOUR
	default:
		return apiv1.TimeFormat_TIME_FORMAT_AUTO
	}
}

func stringPtr(value string) *string {
	return &value
}
