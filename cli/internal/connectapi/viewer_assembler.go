package connectapi

import (
	"context"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// viewerAssembler owns the independent projected/runtime reads needed to
// assemble GetViewer without serializing unrelated data sources.
type viewerAssembler struct {
	service *viewerService
}

func (a *viewerAssembler) assemble(ctx context.Context, user *corev1.User, userID string) (*apiv1.GetViewerResponse, error) {
	var (
		responseUser      *apiv1.ViewerUser
		capabilities      *apiv1.ViewerCapabilities
		serverPreference  *apiv1.NotificationPreference
		roomPreferences   []*apiv1.RoomNotificationPreference
		viewerPermissions *apiv1.ServerViewerPermissions
		viewerState       *apiv1.ServerViewerState
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		responseUser, err = a.service.viewerUser(groupCtx, user)
		return err
	})
	group.Go(func() error {
		var err error
		capabilities, err = a.service.viewerCapabilities(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		serverPreference, err = a.service.serverNotificationPreference(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		roomPreferences, err = a.service.roomNotificationPreferences(groupCtx, userID)
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
		User:                         responseUser,
		Capabilities:                 capabilities,
		ServerNotificationPreference: serverPreference,
		RoomNotificationPreferences:  roomPreferences,
		ViewerPermissions:            viewerPermissions,
		ViewerState:                  viewerState,
	}, nil
}

func (a *viewerAssembler) serverViewerState(ctx context.Context, userID string) (*apiv1.ServerViewerPermissions, *apiv1.ServerViewerState, error) {
	var (
		hasUnreadRooms   bool
		permissionGrants []*apiv1.PermissionGrant
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		hasUnreadRooms, err = a.viewerHasUnreadRooms(groupCtx, userID)
		return err
	})
	group.Go(func() error {
		var err error
		permissionGrants, err = parallel.Map(
			groupCtx,
			maxConnectAPIHydrationConcurrency,
			core.AllPermissions(),
			func(ctx context.Context, _ int, meta core.PermissionMetadata) (*apiv1.PermissionGrant, error) {
				granted, err := a.service.api.core.HasUserPermissionViaRoles(ctx, userID, meta.Permission)
				if err != nil {
					return nil, connectError(err)
				}
				return &apiv1.PermissionGrant{
					Permission: string(meta.Permission),
					Granted:    granted,
				}, nil
			},
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}

	permissions := &apiv1.ServerViewerPermissions{Permissions: permissionGrants}
	return permissions, &apiv1.ServerViewerState{HasUnreadRooms: hasUnreadRooms}, nil
}

func (a *viewerAssembler) viewerHasUnreadRooms(ctx context.Context, userID string) (bool, error) {
	rooms, err := a.service.api.core.ListMemberRooms(ctx, core.KindChannel, userID, core.MemberRoomListOptions{})
	if err != nil {
		return false, connectError(err)
	}
	var found atomic.Bool
	_, err = parallel.Map(ctx, maxConnectAPIHydrationConcurrency, rooms, func(ctx context.Context, _ int, room *corev1.Room) (struct{}, error) {
		if found.Load() {
			return struct{}{}, nil
		}
		hasUnread, err := a.service.api.core.HasUnread(ctx, core.KindChannel, userID, room.GetId())
		// Preserve the summary's fail-soft behavior for an unavailable marker.
		if err != nil {
			return struct{}{}, nil
		}
		if hasUnread {
			found.Store(true)
		}
		return struct{}{}, nil
	})
	if err != nil {
		return false, connectError(err)
	}
	return found.Load(), nil
}
