package core

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// MaxRoomNoticeLength bounds notice Markdown in Unicode code points.
const MaxRoomNoticeLength = 4000

// RoomNoticeInput carries an editor's version. Empty ExpectedVersion means no
// notice has ever been published. Clear retains a version to prevent ABA edits.
type RoomNoticeInput struct {
	ActorID, RoomID, ExpectedVersion, Body string
	Republish                              bool
}

// GetNotice returns member-only channel content, including cleared state.
// Notice visibility does not depend on message-read or management permissions.
func (s *RoomCommandModel) GetNotice(ctx context.Context, input RoomIDInput) (*readmodel.RoomNotice, error) {
	room, kind, err := s.authorization.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	if kind != KindChannel {
		return nil, invalidArgument("DM rooms do not support notices")
	}
	if room.GetNotice() == nil {
		return &readmodel.RoomNotice{}, nil
	}
	return room.GetNotice(), nil
}

// PublishNotice replaces content and optionally resurfaces it. Corrections keep
// the publication ID; first publication and republish receive a fresh one.
func (s *RoomCommandModel) PublishNotice(ctx context.Context, input RoomNoticeInput) (*readmodel.RoomNotice, error) {
	if !utf8.ValidString(input.Body) || strings.TrimSpace(input.Body) == "" || utf8.RuneCountInString(input.Body) > MaxRoomNoticeLength {
		return nil, invalidArgument("notice must contain 1 to 4000 Unicode code points and must not be blank")
	}
	return s.mutateNotice(ctx, input)
}

// ClearNotice removes current content while retaining the concurrency version.
func (s *RoomCommandModel) ClearNotice(ctx context.Context, input RoomNoticeInput) (*readmodel.RoomNotice, error) {
	input.Body, input.Republish = "", false
	return s.mutateNotice(ctx, input)
}

func (s *RoomCommandModel) mutateNotice(ctx context.Context, input RoomNoticeInput) (*readmodel.RoomNotice, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.RoomID) == "" {
		return nil, invalidArgument("room_id is required")
	}
	aggregate := evtstream.RoomAggregate(input.RoomID)
	filter := aggregate.AllEventsFilter()
	for range maxPinnedMessageMutationAttempts {
		position, err := s.publisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return nil, err
		}
		if err := s.rooms.waitForDirectory(ctx, position); err != nil {
			return nil, err
		}
		var current *readmodel.RoomNotice
		if err := s.authorization.authorizeAtStableInputs(ctx, func() error {
			room, kind, err := s.authorization.requireRoomMember(ctx, input.ActorID, input.RoomID)
			if err != nil {
				return err
			}
			if kind != KindChannel {
				return invalidArgument("DM rooms do not support notices")
			}
			allowed, err := s.permissions.HasRoomPermission(ctx, input.ActorID, kind, input.RoomID, PermRoomManageNotice)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrPermissionDenied
			}
			if room.GetArchived() {
				return ErrRoomArchived
			}
			current = room.GetNotice()
			return nil
		}); err != nil {
			return nil, err
		}
		if current.GetVersion() != input.ExpectedVersion {
			return nil, events.ErrConflict
		}
		if current.GetBody() == input.Body && !input.Republish {
			if current == nil {
				return &readmodel.RoomNotice{}, nil
			}
			return current, nil
		}
		change := &evtv1.RoomNoticeChangedEvent{
			RoomId:    input.RoomID,
			Body:      input.Body,
			Announced: input.Body != "" && (current.GetBody() == "" || input.Republish),
		}
		event := newEvent(input.ActorID, &evtv1.Event{Event: &evtv1.Event_RoomNoticeChanged{RoomNoticeChanged: change}})
		subject := aggregate.SubjectFor(event)
		seqs, err := s.publisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: event, Expect: events.ExpectFilterSeq(filter, position.Seq)}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := s.rooms.waitForDirectory(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
			return nil, err
		}
		return noticeAfterChange(current, event), nil
	}
	return nil, events.ErrConflict
}

// noticeAfterChange derives publication identity and time from the announcement
// envelope. Corrections keep both; clear removes both but retains the write version.
func noticeAfterChange(current *readmodel.RoomNotice, event *evtv1.Event) *readmodel.RoomNotice {
	change := event.GetRoomNoticeChanged()
	publicationTime := current.GetPublishedAt()
	publicationID := current.GetPublicationId()
	if change.GetBody() == "" {
		publicationID = ""
		publicationTime = nil
	} else if change.GetAnnounced() {
		publicationID = event.GetId()
		publicationTime = event.GetCreatedAt()
	}
	return &readmodel.RoomNotice{Version: event.GetId(), PublicationId: publicationID, Body: change.GetBody(), PublishedAt: publicationTime}
}
