package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
)

type reactionService struct {
	api *API
}

func (s *reactionService) AddReaction(ctx context.Context, req *connect.Request[appv1.AddReactionRequest]) (*connect.Response[appv1.AddReactionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	added, err := s.api.core.ReactionModel().AddReaction(ctx, core.ReactionMutationInput{
		ActorID:        caller.UserID,
		RoomID:         req.Msg.RoomId,
		MessageEventID: req.Msg.MessageEventId,
		Emoji:          req.Msg.Emoji,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.AddReactionResponse{Added: added}), nil
}

func (s *reactionService) RemoveReaction(ctx context.Context, req *connect.Request[appv1.RemoveReactionRequest]) (*connect.Response[appv1.RemoveReactionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	removed, err := s.api.core.ReactionModel().RemoveReaction(ctx, core.ReactionMutationInput{
		ActorID:        caller.UserID,
		RoomID:         req.Msg.RoomId,
		MessageEventID: req.Msg.MessageEventId,
		Emoji:          req.Msg.Emoji,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&appv1.RemoveReactionResponse{Removed: removed}), nil
}
