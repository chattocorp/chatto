package http_server

import (
	"context"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetAccountDeletionStatus(ctx context.Context, userID, requestID string) (*apiv1.GetAccountDeletionStatusResponse, *wirev1.WireError) {
	canDelete, err := c.server.core.CanDeleteUser(ctx, userID, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetAccountDeletionStatusResponse{ViewerCanDeleteAccount: canDelete}, nil
}

func (c *wireConn) handleWireRequestAccountDeletion(ctx context.Context, userID, requestID string) (*apiv1.RequestAccountDeletionResponse, *wirev1.WireError) {
	canDelete, err := c.server.core.CanDeleteUser(ctx, userID, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canDelete {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	token, err := c.server.core.CreateAccountDeletionToken(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.RequestAccountDeletionResponse{ConfirmationToken: token}, nil
}

func (c *wireConn) handleWireDeleteMyAccount(ctx context.Context, userID, requestID string, body *apiv1.DeleteMyAccountRequest) (*apiv1.DeleteMyAccountResponse, *wirev1.WireError) {
	if body == nil || strings.TrimSpace(body.GetConfirmationToken()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: confirmation_token is required", errWireInvalidArgument))
	}

	canDelete, err := c.server.core.CanDeleteUser(ctx, userID, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canDelete {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	if err := c.server.core.ValidateAccountDeletionToken(ctx, body.GetConfirmationToken(), userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.DeleteUser(ctx, userID, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteMyAccountResponse{Deleted: true}, nil
}
