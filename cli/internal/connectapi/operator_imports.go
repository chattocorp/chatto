package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

type operatorImportService struct{ api *API }

func (s *operatorImportService) ImportArchive(ctx context.Context, req *connect.Request[operatorv1.ImportArchiveRequest]) (*connect.Response[operatorv1.ImportArchiveResponse], error) {
	n := req.Msg.GetNaming()
	result, err := s.api.core.ImportArchive(ctx, &interchange.Archive{Metadata: req.Msg.GetMetadata(), Files: req.Msg.GetFiles()}, core.ImportNaming{
		ChannelPrefix: n.GetChannelPrefix(), ChannelSuffix: n.GetChannelSuffix(), UsernamePrefix: n.GetUsernamePrefix(), UsernameSuffix: n.GetUsernameSuffix(),
	}, req.Msg.GetValidateOnly(), req.Msg.GetAcceptOmissions())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportArchiveResponse{Users: uint32(result.Users), Rooms: uint32(result.Rooms), Assets: uint32(result.Assets), Events: uint32(result.Events), Omissions: uint32(result.Omissions), Complete: result.Complete}), nil
}
