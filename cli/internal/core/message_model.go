package core

import (
	"context"
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

// MessageUpdateInput describes one user-facing message edit operation.
type MessageUpdateInput struct {
	ActorID           string
	RoomID            string
	EventID           string
	Body              string
	AlsoSendToChannel *bool
}

// MessageDeleteInput describes one user-facing message retraction operation.
type MessageDeleteInput struct {
	ActorID string
	RoomID  string
	EventID string
}

// MessageAttachmentDeleteInput describes removal of one attachment from a
// message body.
type MessageAttachmentDeleteInput struct {
	ActorID      string
	RoomID       string
	EventID      string
	AttachmentID string
}

// MessageLinkPreviewDeleteInput describes removal of one link preview from a
// message body.
type MessageLinkPreviewDeleteInput struct {
	ActorID string
	RoomID  string
	EventID string
	URL     string
}

// TypingIndicatorInput describes one live-only typing indicator publish.
type TypingIndicatorInput struct {
	ActorID           string
	RoomID            string
	ThreadRootEventID *string
}

// MessagePostResult is returned by MessageModel.PostMessage. Exactly one of
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

// Messages returns the operation-level model for message reads/writes that
// need shared public-API authorization and response semantics.
func (c *ChattoCore) Messages() *MessageModel {
	return c.messageModel
}

// MessageModel owns user-facing message operations. Lower-level ChattoCore
// helpers still perform the event-sourced write, while this model centralizes
// authZ, mention confirmation, and post-write sync behavior for public
// transports during the GraphQL-to-ConnectRPC migration.
type MessageModel struct {
	core *ChattoCore
}

// PostMessage posts a message as actorID and returns either the committed event
// or a mention confirmation challenge. Authorization: actor must be a room
// member and must have message.post or message.post-in-thread, plus
// message.echo/message.post when echoing a thread reply.
func (s *MessageModel) PostMessage(ctx context.Context, input MessagePostInput) (*MessagePostResult, error) {
	if strings.TrimSpace(input.ActorID) == "" {
		return nil, ErrNotAuthenticated
	}
	if strings.TrimSpace(input.RoomID) == "" {
		return nil, invalidArgument("room_id is required")
	}
	if !HasVisibleContent(input.Body) && len(input.AttachmentAssetIDs) == 0 {
		return nil, invalidArgument("message must have either body or attachments")
	}
	if input.AlsoSendToChannel && strings.TrimSpace(input.ThreadRootEventID) == "" {
		return nil, invalidArgument("also_send_to_channel requires thread_root_event_id")
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
			return nil, invalidArgument("alsoSendToChannel can only be used with thread replies (threadRootEventId must be set)")
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

	videoProcessingAssetIDs := s.videoProcessingAssetIDsForPost(input)
	options := make([]PostMessageOption, 0, 2)
	if len(videoProcessingAssetIDs) > 0 {
		options = append(options, WithVideoProcessingAssets(videoProcessingAssetIDs...))
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

// UpdateMessage edits an existing message. Authorization: actor must be a room
// member. Authors may edit their own messages subject to the core edit window.
// Non-authors need message.manage. Changing a thread reply's channel echo state
// is author-only and, when enabling the echo, additionally requires message.echo
// and message.post.
func (s *MessageModel) UpdateMessage(ctx context.Context, input MessageUpdateInput) error {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.EventID) == "" {
		return invalidArgument("event_id is required")
	}
	if _, err := s.requireMessagePostedEvent(ctx, kind, room.Id, input.EventID); err != nil {
		return err
	}

	body, err := s.core.GetFullMessageBodyByEventID(ctx, input.EventID)
	if err != nil {
		return err
	}
	if body == nil {
		return ErrMessageNotFound
	}
	if body.AuthorId != input.ActorID {
		can, err := s.core.CanManageOthersMessage(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return err
		}
		if !can {
			return ErrPermissionDenied
		}
	}

	var editOptions []EditMessageOption
	if input.AlsoSendToChannel != nil {
		if body.AuthorId != input.ActorID {
			return ErrNotMessageAuthor
		}
		if *input.AlsoSendToChannel {
			can, err := s.core.CanEchoMessage(ctx, input.ActorID, kind, room.Id)
			if err != nil {
				return err
			}
			if !can {
				return ErrPermissionDenied
			}
			can, err = s.core.CanPostMessage(ctx, input.ActorID, kind, room.Id)
			if err != nil {
				return err
			}
			if !can {
				return ErrPermissionDenied
			}
		}
		editOptions = append(editOptions, WithMessageChannelEcho(*input.AlsoSendToChannel))
	}

	return s.core.EditMessage(ctx, input.ActorID, kind, room.Id, input.EventID, input.Body, editOptions...)
}

// DeleteMessage retracts an existing message. Authorization: actor must be a
// room member. Authors may delete their own messages; non-authors need
// message.manage.
func (s *MessageModel) DeleteMessage(ctx context.Context, input MessageDeleteInput) error {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.EventID) == "" {
		return invalidArgument("event_id is required")
	}
	if _, err := s.requireMessagePostedEvent(ctx, kind, room.Id, input.EventID); err != nil {
		return err
	}

	authorID, err := s.core.GetMessageAuthorID(ctx, kind, input.EventID)
	if err != nil {
		return err
	}
	if authorID != "" && authorID != input.ActorID {
		can, err := s.core.CanManageOthersMessage(ctx, input.ActorID, kind, room.Id)
		if err != nil {
			return err
		}
		if !can {
			return ErrPermissionDenied
		}
	}

	return s.core.DeleteMessage(ctx, input.ActorID, kind, room.Id, input.EventID)
}

// DeleteAttachment removes one attachment from a message. Authorization:
// actor must be a room member; the core partial-edit helper keeps the operation
// author-only.
func (s *MessageModel) DeleteAttachment(ctx context.Context, input MessageAttachmentDeleteInput) error {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.EventID) == "" {
		return invalidArgument("event_id is required")
	}
	if strings.TrimSpace(input.AttachmentID) == "" {
		return invalidArgument("attachment_id is required")
	}
	if _, err := s.requireMessagePostedEvent(ctx, kind, room.Id, input.EventID); err != nil {
		return err
	}
	return s.core.DeleteAttachmentFromMessage(ctx, input.ActorID, kind, room.Id, input.EventID, input.AttachmentID)
}

// DeleteLinkPreview removes the selected link preview from a message.
// Authorization: actor must be a room member; the core partial-edit helper
// keeps the operation author-only.
func (s *MessageModel) DeleteLinkPreview(ctx context.Context, input MessageLinkPreviewDeleteInput) error {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.EventID) == "" {
		return invalidArgument("event_id is required")
	}
	if strings.TrimSpace(input.URL) == "" {
		return invalidArgument("url is required")
	}
	if _, err := s.requireMessagePostedEvent(ctx, kind, room.Id, input.EventID); err != nil {
		return err
	}
	return s.core.DeleteLinkPreviewFromMessage(ctx, input.ActorID, kind, room.Id, input.EventID, input.URL)
}

// SendTypingIndicator publishes a live-only typing indicator. Authorization:
// actor must be a room member; there is intentionally no message-posting
// permission check.
func (s *MessageModel) SendTypingIndicator(ctx context.Context, input TypingIndicatorInput) error {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	return s.core.PublishTypingIndicator(ctx, input.ActorID, kind, room.Id, input.ThreadRootEventID)
}

func (s *MessageModel) requireMessagePostedEvent(ctx context.Context, kind RoomKind, roomID, eventID string) (*corev1.Event, error) {
	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.GetMessagePosted() == nil {
		return nil, ErrMessageNotFound
	}
	return event, nil
}

func (s *MessageModel) videoProcessingAssetIDsForPost(input MessagePostInput) []string {
	assetIDs := make([]string, 0, len(input.VideoProcessingAssetIDs)+len(input.AttachmentAssetIDs))
	seen := make(map[string]struct{}, len(input.VideoProcessingAssetIDs)+len(input.AttachmentAssetIDs))
	add := func(assetID string) {
		if assetID == "" {
			return
		}
		if _, ok := seen[assetID]; ok {
			return
		}
		seen[assetID] = struct{}{}
		assetIDs = append(assetIDs, assetID)
	}

	// Explicit IDs are still needed for upload-byte-derived decisions such as
	// animated GIF conversion. Transports that only submit attachment asset IDs
	// can infer ordinary video/* assets from durable asset metadata.
	for _, assetID := range input.VideoProcessingAssetIDs {
		add(assetID)
	}
	for _, assetID := range input.AttachmentAssetIDs {
		if _, ok := seen[assetID]; ok || assetID == "" {
			continue
		}
		declared, ok := s.core.assetLifecycle().AssetCreation(assetID)
		if !ok || declared == nil {
			continue
		}
		if AttachmentNeedsVideoProcessing(attachmentFromAsset(declared.GetAsset()), false) {
			add(assetID)
		}
	}
	return assetIDs
}
