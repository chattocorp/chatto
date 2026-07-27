package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"
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

	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	response, err := (&viewerAssembler{service: s}).assemble(ctx, user, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(response), nil
}

func (s *viewerService) viewerUser(ctx context.Context, user *corev1.User) (*apiv1.ViewerUser, error) {
	var (
		hasVerifiedEmail bool
		settings         *corev1.ServerUserPreferences
		apiUser          *apiv1.User
		canDeleteAccount bool
		lastLoginChange  time.Time
		hasPassword      bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		hasVerifiedEmail, err = s.api.core.HasVerifiedEmail(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		settings, err = s.api.core.GetUserSettings(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		apiUser, err = (&userService{api: s.api}).userSummary(groupCtx, user, nil)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		canDeleteAccount, err = s.api.core.CanDeleteUser(groupCtx, user.GetId(), user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		lastLoginChange, err = s.api.core.GetLastLoginChange(groupCtx, user.GetId())
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		hasPassword, err = s.api.core.HasPassword(groupCtx, user.GetId())
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

func (s *viewerService) viewerCapabilities(ctx context.Context, userID string) (*apiv1.ViewerCapabilities, error) {
	canViewAdmin, err := s.api.core.HasAnyAdminPermission(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canStartDMs, err := s.api.core.CanStartDM(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewUsers, err := s.api.core.CanAdminUsersView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminManageAccounts, err := s.api.core.CanManageUserAccounts(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAssignRoles, err := s.api.core.CanAssignRoles(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminManageRoles, err := s.api.core.CanManageRoles(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canManageUserPermissions, err := s.api.core.CanManageUserPermissions(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewRoles := canAdminManageRoles || canAssignRoles || canManageUserPermissions
	canAdminViewSystem, err := s.api.core.CanAdminSystemView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	canAdminViewAudit, err := s.api.core.CanAdminAuditView(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	channelSpaces := []string{core.LegacySpaceIDForRoomKind(core.KindChannel)}
	var (
		hasUnreadFollowedThreads              bool
		hasPendingFollowedThreadNotifications bool
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		hasUnreadFollowedThreads, err = s.api.core.HasUnreadFollowedThreads(groupCtx, userID, channelSpaces)
		return connectError(err)
	})
	group.Go(func() error {
		var err error
		hasPendingFollowedThreadNotifications, err = s.api.core.HasPendingFollowedThreadNotifications(groupCtx, userID, channelSpaces)
		return connectError(err)
	})
	if err := group.Wait(); err != nil {
		return nil, err
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
		HasUnreadFollowedThreads:              hasUnreadFollowedThreads,
		HasPendingFollowedThreadNotifications: hasPendingFollowedThreadNotifications,
	}, nil
}

func (s *viewerService) serverNotificationPreference(ctx context.Context, userID string) (*apiv1.NotificationPreference, error) {
	level, err := s.api.core.GetSpaceNotificationLevel(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return apiNotificationPreference(level, effectiveLevel), nil
}

func (s *viewerService) roomNotificationPreferences(ctx context.Context, userID string) ([]*apiv1.RoomNotificationPreference, error) {
	prefs, err := s.api.core.GetAllRoomNotificationPreferences(ctx, userID)
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
