package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (s *accountService) UpdateCustomStatus(ctx context.Context, req *connect.Request[apiv1.UpdateCustomStatusRequest]) (*connect.Response[apiv1.UpdateCustomStatusResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	expiresAt, err := apiTimestampToTime(req.Msg.ExpiresAt)
	if err != nil {
		return nil, err
	}

	updated, err := s.api.core.SetUserCustomStatus(ctx, caller.UserID, req.Msg.Emoji, req.Msg.Text, expiresAt)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.UpdateCustomStatusResponse{
		Status: coreCustomStatusToAPI(updated.GetCustomStatus()),
	}), nil
}

func (s *accountService) DeleteCustomStatus(ctx context.Context, _ *connect.Request[apiv1.DeleteCustomStatusRequest]) (*connect.Response[apiv1.DeleteCustomStatusResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.api.core.ClearUserCustomStatus(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.DeleteCustomStatusResponse{
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
