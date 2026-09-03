package core

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type timelineEventReader interface {
	EventsAt(context.Context, []uint64) ([]*evtstream.SubjectEvent, error)
}

// RoomTimelineHydrator loads complete room events and message bodies from EVT
// for compact Room Timeline projection references.
type RoomTimelineHydrator struct {
	reader timelineEventReader
}

func newRoomTimelineHydrator(reader timelineEventReader) *RoomTimelineHydrator {
	return &RoomTimelineHydrator{reader: reader}
}

func (h *RoomTimelineHydrator) events(ctx context.Context, entries []*TimelineEntry) ([]*evtv1.Event, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if h == nil || h.reader == nil {
		return nil, fmt.Errorf("room timeline hydrator is unavailable")
	}
	sequences := make([]uint64, len(entries))
	for i, entry := range entries {
		if entry == nil {
			return nil, fmt.Errorf("hydrate room timeline entry %d: reference is nil", i)
		}
		sequences[i] = entry.StreamSeq
	}
	records, err := h.reader.EventsAt(ctx, sequences)
	if err != nil {
		return nil, fmt.Errorf("hydrate room timeline: %w", err)
	}
	if len(records) != len(entries) {
		return nil, fmt.Errorf("hydrate room timeline: reader returned %d records for %d references", len(records), len(entries))
	}
	events := make([]*evtv1.Event, len(entries))
	for i, record := range records {
		if err := validateTimelineEntryRecord(entries[i], record); err != nil {
			return nil, fmt.Errorf("hydrate room timeline entry %d: %w", i, err)
		}
		events[i] = record.Event
	}
	return events, nil
}

func (h *RoomTimelineHydrator) body(ctx context.Context, reference TimelineBodyReference) (*evtv1.MessageBody, error) {
	if reference.StreamSeq == 0 {
		return nil, nil
	}
	bodies, err := h.bodies(ctx, []TimelineBodyReference{reference})
	if err != nil {
		return nil, err
	}
	return bodies[0], nil
}

func (h *RoomTimelineHydrator) bodies(ctx context.Context, references []TimelineBodyReference) ([]*evtv1.MessageBody, error) {
	if len(references) == 0 {
		return nil, nil
	}
	if h == nil || h.reader == nil {
		return nil, fmt.Errorf("room timeline hydrator is unavailable")
	}
	sequences := make([]uint64, len(references))
	for i, reference := range references {
		if reference.StreamSeq == 0 {
			return nil, fmt.Errorf("hydrate message body %q: sequence must be positive", reference.MessageEventID)
		}
		sequences[i] = reference.StreamSeq
	}
	records, err := h.reader.EventsAt(ctx, sequences)
	if err != nil {
		return nil, fmt.Errorf("hydrate message bodies: %w", err)
	}
	if len(records) != len(references) {
		return nil, fmt.Errorf("hydrate message bodies: reader returned %d records for %d references", len(records), len(references))
	}
	bodies := make([]*evtv1.MessageBody, len(references))
	for i, record := range records {
		body, err := validateTimelineBodyRecord(references[i], record)
		if err != nil {
			return nil, fmt.Errorf("hydrate message body %q: %w", references[i].MessageEventID, err)
		}
		bodies[i] = body
	}
	return bodies, nil
}

func validateTimelineBodyRecord(reference TimelineBodyReference, record *evtstream.SubjectEvent) (*evtv1.MessageBody, error) {
	if record == nil || record.Event == nil {
		return nil, fmt.Errorf("record is nil")
	}
	if record.Sequence != reference.StreamSeq {
		return nil, fmt.Errorf("sequence is %d, want %d", record.Sequence, reference.StreamSeq)
	}
	if record.Subject != evtstream.RoomAggregate(reference.RoomID).Subject(evtstream.EventMessageBody) {
		return nil, fmt.Errorf("subject %q does not match room %q", record.Subject, reference.RoomID)
	}
	payload := record.Event.GetMessageBody()
	if payload == nil || payload.GetBody() == nil {
		return nil, fmt.Errorf("EVT sequence %d is not a message body", reference.StreamSeq)
	}
	if payload.GetEventId() != reference.MessageEventID || payload.GetRoomId() != reference.RoomID {
		return nil, fmt.Errorf("EVT identity does not match projection reference")
	}
	body := proto.Clone(payload.GetBody()).(*evtv1.MessageBody)
	bodyEventID := body.GetBodyEventId()
	if bodyEventID == "" {
		bodyEventID = record.Event.GetId()
		body.BodyEventId = bodyEventID
	}
	if record.Event.GetId() != reference.BodyEventID || bodyEventID != reference.BodyEventID {
		return nil, fmt.Errorf("body event ID does not match projection reference")
	}
	if body.GetAuthorId() != reference.AuthorID {
		return nil, fmt.Errorf("author does not match projection reference")
	}
	if messageBodyAttachmentCount(body) != reference.AttachmentCount {
		return nil, fmt.Errorf("attachment count does not match projection reference")
	}
	return body, nil
}

func validateTimelineEntryRecord(reference *TimelineEntry, record *evtstream.SubjectEvent) error {
	if reference == nil || record == nil || record.Event == nil {
		return fmt.Errorf("event record is nil")
	}
	if record.Sequence != reference.StreamSeq {
		return fmt.Errorf("sequence is %d, want %d", record.Sequence, reference.StreamSeq)
	}
	if record.Subject != evtstream.RoomAggregate(reference.RoomID).Subject(reference.EventType) {
		return fmt.Errorf("subject %q does not match room %q and event type %q", record.Subject, reference.RoomID, reference.EventType)
	}
	if record.Event.GetId() != reference.EventID || roomIDOfEvent(record.Event) != reference.RoomID || evtstream.EventTypeOf(record.Event) != reference.EventType {
		return fmt.Errorf("event identity does not match projection reference")
	}
	if record.Event.GetActorId() != reference.ActorID || !eventCreatedAt(record.Event).Equal(reference.CreatedAt) {
		return fmt.Errorf("event metadata does not match projection reference")
	}
	if posted := record.Event.GetMessagePosted(); reference.IsMessagePost() {
		if posted == nil {
			return fmt.Errorf("message routing does not match projection reference")
		}
		rootID := posted.GetInThread()
		if rootID == "" {
			rootID = posted.GetEchoFromThreadRootEventId()
		}
		if rootID == "" {
			rootID = reference.EventID
		}
		if posted.GetInThread() != reference.InThreadEventID || rootID != reference.ThreadRootEventID || posted.GetEchoOfEventId() != reference.EchoOfEventID {
			return fmt.Errorf("message routing does not match projection reference")
		}
	}
	return nil
}
