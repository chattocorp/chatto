package connectapi

import (
	"connectrpc.com/connect"
	"context"
	"github.com/chattocorp/chatto/cli/internal/core"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func operatorHistoricalTime(t *timestamppb.Timestamp) (*time.Time, error) {
	if t == nil {
		return nil, nil
	}
	if t.CheckValid() != nil {
		return nil, invalidArgument("invalid occurred_at")
	}
	v := t.AsTime()
	return &v, nil
}

func (s *operatorMessageService) ImportReactionChanges(ctx context.Context, req *connect.Request[operatorv1.ImportReactionChangesRequest]) (*connect.Response[operatorv1.ImportReactionChangesResponse], error) {
	changes := make([]core.HistoricalMessageChange, 0, len(req.Msg.Operations))
	for _, operation := range req.Msg.Operations {
		if operation == nil || operation.Change == nil {
			return nil, invalidArgument("reaction operation is required")
		}
		at, err := operatorHistoricalTime(operation.OccurredAt)
		if err != nil {
			return nil, err
		}
		changes = append(changes, core.HistoricalMessageChange{
			RoomID: req.Msg.RoomId, MessageID: operation.MessageId, OperationKey: operation.OperationKey,
			ActorID: operation.ActorId, OccurredAt: at, Kind: "reaction",
			UserID: operation.Change.UserId, Emoji: operation.Change.Emoji, Present: operation.Change.Present,
		})
	}
	if err := s.api.core.ImportHistoricalReactionChanges(ctx, changes); err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportReactionChangesResponse{}), nil
}

func (s *operatorRoomService) ImportRoomEvent(ctx context.Context, req *connect.Request[operatorv1.ImportRoomEventRequest]) (*connect.Response[operatorv1.ImportRoomEventResponse], error) {
	r := req.Msg
	at, err := operatorHistoricalTime(r.OccurredAt)
	if err != nil {
		return nil, err
	}
	in := core.HistoricalRoomChange{RoomID: r.RoomId, OperationKey: r.OperationKey, ActorID: r.ActorId, OccurredAt: at, Hidden: r.Hidden}
	switch v := r.Change.(type) {
	case *operatorv1.ImportRoomEventRequest_JoinedUserId:
		in.Kind, in.Value = "joined", v.JoinedUserId
	case *operatorv1.ImportRoomEventRequest_LeftUserId:
		in.Kind, in.Value = "left", v.LeftUserId
	case *operatorv1.ImportRoomEventRequest_Name:
		in.Kind, in.Value = "name", v.Name
	case *operatorv1.ImportRoomEventRequest_Topic:
		in.Kind, in.Value = "topic", v.Topic
	case *operatorv1.ImportRoomEventRequest_Purpose:
		in.Kind, in.Value = "purpose", v.Purpose
	case *operatorv1.ImportRoomEventRequest_Archived:
		in.Kind = "unarchived"
		if v.Archived {
			in.Kind = "archived"
		}
	default:
		return nil, invalidArgument("room change is required")
	}
	id, err := s.api.core.ImportHistoricalRoomEvent(ctx, in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportRoomEventResponse{EventId: id}), nil
}

func (s *operatorMessageService) ImportMessageChange(ctx context.Context, req *connect.Request[operatorv1.ImportMessageChangeRequest]) (*connect.Response[operatorv1.ImportMessageChangeResponse], error) {
	r := req.Msg
	at, err := operatorHistoricalTime(r.OccurredAt)
	if err != nil {
		return nil, err
	}
	in := core.HistoricalMessageChange{RoomID: r.RoomId, MessageID: r.MessageId, OperationKey: r.OperationKey, ActorID: r.ActorId, OccurredAt: at}
	switch v := r.Change.(type) {
	case *operatorv1.ImportMessageChangeRequest_Edit:
		in.Kind, in.Body, in.AssetIDs = "edit", v.Edit.GetBody(), v.Edit.GetAttachmentAssetIds()
	case *operatorv1.ImportMessageChangeRequest_Deleted:
		if !v.Deleted {
			return nil, invalidArgument("deleted must be true")
		}
		in.Kind = "deleted"
	case *operatorv1.ImportMessageChangeRequest_Reaction:
		in.Kind, in.UserID, in.Emoji, in.Present = "reaction", v.Reaction.GetUserId(), v.Reaction.GetEmoji(), v.Reaction.GetPresent()
	case *operatorv1.ImportMessageChangeRequest_CreateThread:
		if !v.CreateThread {
			return nil, invalidArgument("create_thread must be true")
		}
		in.Kind = "thread"
	default:
		return nil, invalidArgument("message change is required")
	}
	if err = s.api.core.ImportHistoricalMessageChange(ctx, in); err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportMessageChangeResponse{}), nil
}
