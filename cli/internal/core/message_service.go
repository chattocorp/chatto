package core

import (
	"context"
	"fmt"
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MessagePostInput describes one user-facing message post operation.
type MessagePostInput struct {
	ActorID                  string
	RoomID                   string
	Body                     string
	AttachmentAssetIDs       []string
	VideoProcessingAssetIDs  []string
	ThreadRootEventID        string
	InReplyTo                string
	AlsoSendToChannel        bool
	MentionConfirmationToken string
	LinkPreview              *corev1.LinkPreview
}

// MessagePostResult is returned by MessageService.PostMessage. Exactly one of
// Event or MentionConfirmation is set.
type MessagePostResult struct {
	Event               *corev1.Event
	MentionConfirmation *MentionConfirmationChallenge
}

// MentionConfirmationChallenge asks the client to confirm a large mention send.
type MentionConfirmationChallenge struct {
	RecipientCount int
	Token          string
}

// Messages returns the operation-level service for message reads/writes that
// need shared public-API authorization and response semantics.
func (c *ChattoCore) Messages() *MessageService {
	return c.messageService
}

// MessageService owns user-facing message operations. Lower-level ChattoCore
// helpers still perform the event-sourced write, while this service centralizes
// authZ, mention confirmation, and post-write sync behavior for public
// transports during the GraphQL-to-ConnectRPC migration.
type MessageService struct {
	core *ChattoCore
}

// PostMessage posts a message as actorID and returns either the committed event
// or a mention confirmation challenge. Authorization: actor must be a room
// member and must have message.post or message.post-in-thread, plus
// message.echo/message.post when echoing a thread reply.
func (s *MessageService) PostMessage(ctx context.Context, input MessagePostInput) (*MessagePostResult, error) {
	if strings.TrimSpace(input.ActorID) == "" {
		return nil, ErrNotAuthenticated
	}
	if strings.TrimSpace(input.RoomID) == "" {
		return nil, fmt.Errorf("room_id is required")
	}

	room, err := s.core.FindRoomByID(ctx, input.RoomID)
	if err != nil {
		return nil, err
	}
	kind := KindOfRoom(room)

	isMember, err := s.core.RoomMembershipExists(ctx, kind, input.ActorID, room.Id)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotRoomMember
	}

	if input.ThreadRootEventID != "" {
		can, err := s.core.CanPostInThread(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return nil, err
		}
		if !can {
			return nil, ErrPermissionDenied
		}
	} else {
		can, err := s.core.CanPostMessage(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return nil, err
		}
		if !can {
			return nil, ErrPermissionDenied
		}
	}

	if len(input.AttachmentAssetIDs) > 0 {
		can, err := s.core.CanAttachFiles(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return nil, err
		}
		if !can {
			return nil, ErrPermissionDenied
		}
	}

	if input.AlsoSendToChannel {
		if input.ThreadRootEventID == "" {
			return nil, fmt.Errorf("alsoSendToChannel can only be used with thread replies (threadRootEventId must be set)")
		}
		can, err := s.core.CanEchoMessage(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return nil, err
		}
		if !can {
			return nil, ErrPermissionDenied
		}
		can, err = s.core.CanPostMessage(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return nil, err
		}
		if !can {
			return nil, ErrPermissionDenied
		}
	}

	mentionScope := MentionConfirmationScope{
		UserID:            input.ActorID,
		RoomID:            room.Id,
		Kind:              kind,
		Body:              input.Body,
		ThreadRootEventID: input.ThreadRootEventID,
		AlsoSendToChannel: input.AlsoSendToChannel,
	}
	mentionConfirmed := false
	if input.Body != "" {
		recipientCount, err := s.core.MentionNotificationRecipientCountForBody(ctx, kind, room.Id, input.ActorID, input.Body)
		if err != nil {
			return nil, err
		}
		if recipientCount > LargeMentionNotificationThreshold {
			if err := s.core.ValidateMentionConfirmationToken(input.MentionConfirmationToken, mentionScope); err != nil {
				token, err := s.core.CreateMentionConfirmationToken(mentionScope, recipientCount)
				if err != nil {
					return nil, err
				}
				return &MessagePostResult{MentionConfirmation: &MentionConfirmationChallenge{
					RecipientCount: recipientCount,
					Token:          token,
				}}, nil
			}
			mentionConfirmed = true
		}
	}

	options := make([]PostMessageOption, 0, 2)
	if len(input.VideoProcessingAssetIDs) > 0 {
		options = append(options, WithVideoProcessingAssets(input.VideoProcessingAssetIDs...))
	}
	if mentionConfirmed {
		options = append(options, WithLargeMentionConfirmed())
	}

	event, err := s.core.PostMessage(ctx, kind, room.Id, input.ActorID, input.Body, input.AttachmentAssetIDs, input.ThreadRootEventID, input.InReplyTo, input.LinkPreview, input.AlsoSendToChannel, options...)
	if err != nil {
		if confirmErr, ok := err.(*MentionConfirmationRequiredError); ok {
			token, tokenErr := s.core.CreateMentionConfirmationToken(mentionScope, confirmErr.RecipientCount)
			if tokenErr != nil {
				return nil, tokenErr
			}
			return &MessagePostResult{MentionConfirmation: &MentionConfirmationChallenge{
				RecipientCount: confirmErr.RecipientCount,
				Token:          token,
			}}, nil
		}
		return nil, err
	}

	s.core.NotifyRoomMarkedAsRead(ctx, input.ActorID, kind, room.Id)
	return &MessagePostResult{Event: event}, nil
}
