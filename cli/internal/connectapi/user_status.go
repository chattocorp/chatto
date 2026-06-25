package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type userStatusService struct {
	api *API
}

func (s *userStatusService) SetCustomStatus(ctx context.Context, req *connect.Request[apiv1.SetCustomStatusRequest]) (*connect.Response[apiv1.SetCustomStatusResponse], error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	expiresAt, err := apiTimestampToTime(req.Msg.ExpiresAt)
	if err != nil {
		return nil, err
	}

	updated, err := s.api.core.SetUserCustomStatus(ctx, user.Id, req.Msg.Emoji, req.Msg.Text, expiresAt)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.SetCustomStatusResponse{
		Status: coreCustomStatusToAPI(updated.GetCustomStatus()),
	}), nil
}

func (s *userStatusService) ClearCustomStatus(ctx context.Context, _ *connect.Request[apiv1.ClearCustomStatusRequest]) (*connect.Response[apiv1.ClearCustomStatusResponse], error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.api.core.ClearUserCustomStatus(ctx, user.Id)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.ClearCustomStatusResponse{
		Status: coreCustomStatusToAPI(updated.GetCustomStatus()),
	}), nil
}

func apiTimestampToTime(ts *timestamppb.Timestamp) (*time.Time, error) {
	if ts == nil {
		return nil, nil
	}
	if err := ts.CheckValid(); err != nil {
		return nil, invalidArgument("expires_at is invalid")
	}
	expiresAt := ts.AsTime()
	return &expiresAt, nil
}

func coreCustomStatusToAPI(status *corev1.CustomUserStatus) *apiv1.CustomUserStatus {
	if status == nil {
		return nil
	}
	return &apiv1.CustomUserStatus{
		Emoji:     status.GetEmoji(),
		Text:      status.GetText(),
		ExpiresAt: status.GetExpiresAt(),
	}
}
