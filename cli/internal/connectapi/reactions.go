package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func (s *messageService) AddReaction(ctx context.Context, req *connect.Request[apiv1.AddReactionRequest]) (*connect.Response[apiv1.AddReactionResponse], error) {
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
	return connect.NewResponse(&apiv1.AddReactionResponse{
		Added:    added,
		Reaction: s.reactionSummary(ctx, caller.UserID, req.Msg.MessageEventId, req.Msg.Emoji),
	}), nil
}

func (s *messageService) RemoveReaction(ctx context.Context, req *connect.Request[apiv1.RemoveReactionRequest]) (*connect.Response[apiv1.RemoveReactionResponse], error) {
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
	return connect.NewResponse(&apiv1.RemoveReactionResponse{
		Removed:  removed,
		Reaction: s.reactionSummary(ctx, caller.UserID, req.Msg.MessageEventId, req.Msg.Emoji),
	}), nil
}

func (s *messageService) reactionSummary(ctx context.Context, viewerID, messageEventID, emoji string) *apiv1.RoomTimelineReaction {
	summaries, err := s.api.core.GetReactions(ctx, messageEventID)
	if err != nil {
		return nil
	}
	for _, summary := range summaries {
		if summary.Emoji != emoji {
			continue
		}
		userIDs := firstN(summary.UserIDs, 5)
		return &apiv1.RoomTimelineReaction{
			Emoji:          summary.Emoji,
			Count:          int32(len(summary.UserIDs)),
			HasReacted:     containsString(summary.UserIDs, viewerID),
			PreviewUserIds: userIDs,
		}
	}
	return nil
}
