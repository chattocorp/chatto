package core

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// =============================================================================
// Test helpers (shared with thread_projection_test.go)
// =============================================================================

func fixedTime(seed int) time.Time {
	return time.Date(2026, 1, 1, 12, 0, seed, 0, time.UTC)
}

type postedOpts struct {
	envelopeID                string
	eventID                   string
	roomID                    string
	actorID                   string
	body                      string
	inReplyTo                 string
	inThread                  string
	echoOfEventID             string
	echoFromThreadRootEventID string
	at                        int
}

func postedEvent(o postedOpts) *corev1.Event {
	envID := o.envelopeID
	if envID == "" {
		envID = o.eventID
	}
	return &corev1.Event{
		Id:        envID,
		ActorId:   o.actorID,
		CreatedAt: timestamppb.New(fixedTime(o.at)),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:                    o.roomID,
				InReplyTo:                 o.inReplyTo,
				InThread:                  o.inThread,
				EchoOfEventId:             o.echoOfEventID,
				EchoFromThreadRootEventId: o.echoFromThreadRootEventID,
				Body: &corev1.MessageBody{
					AuthorId:      o.actorID,
					EncryptedBody: []byte(o.body),
				},
			},
		},
	}
}

func editedEvent(envID, targetID, roomID, actorID, newBody string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  roomID,
				EventId: targetID,
				Body: &corev1.MessageBody{
					AuthorId:      actorID,
					EncryptedBody: []byte(newBody),
				},
			},
		},
	}
}

func retractedEvent(envID, targetID, roomID, actorID, reason string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{
				RoomId:  roomID,
				EventId: targetID,
				Reason:  reason,
			},
		},
	}
}

func joinedEvent(envID, roomID, userID string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   userID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID},
		},
	}
}

func leftEvent(envID, roomID, userID string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   userID,
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: roomID},
		},
	}
}

func roomCreatedTimelineEvent(envID, roomID, name string, at int) *corev1.Event {
	return &corev1.Event{
		Id:        envID,
		ActorId:   "SYSTEM",
		CreatedAt: timestamppb.New(fixedTime(at)),
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId: roomID,
				Name:   name,
				Kind:   corev1.RoomKind_ROOM_KIND_CHANNEL,
			},
		},
	}
}

// applyAll feeds events into a projection in order with seq starting at 1.
func applyAll(t *testing.T, p interface {
	Apply(*corev1.Event, uint64) error
}, events []*corev1.Event) {
	t.Helper()
	for i, e := range events {
		if err := p.Apply(e, uint64(i+1)); err != nil {
			t.Fatalf("Apply event %d: %v", i+1, err)
		}
	}
}

func attachmentDeclaredEvent(roomID, attachmentID, contentType string) *corev1.Event {
	return &corev1.Event{
		Id: "ENV-DECLARED-" + attachmentID,
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          attachmentID,
					ContentType: contentType,
				},
				RoomId: roomID,
			},
		},
	}
}

// =============================================================================
// RoomTimelineProjection
// =============================================================================

func TestRoomTimeline_Empty(t *testing.T) {
	p := NewRoomTimelineProjection()
	if got := p.RoomEvents("R1", 50, 0); len(got) != 0 {
		t.Errorf("RoomEvents on empty = %d entries, want 0", len(got))
	}
	if got := p.RoomEventCount("R1"); got != 0 {
		t.Errorf("RoomEventCount on empty = %d, want 0", got)
	}
	if _, ok := p.Get("nope"); ok {
		t.Error("Get on empty should return ok=false")
	}
}

func TestRoomTimeline_AppendsAllEventKinds(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		roomCreatedTimelineEvent("ENV-CREATE", "R1", "general", 1),
		joinedEvent("ENV-JOIN-U1", "R1", "U1", 2),
		postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 3}),
		editedEvent("ENV-EDIT-M1", "ENV-M1", "R1", "U1", "hello (edited)", 4),
		joinedEvent("ENV-JOIN-U2", "R1", "U2", 5),
		postedEvent(postedOpts{envelopeID: "ENV-M2", eventID: "M2", roomID: "R1", actorID: "U2", body: "hi", at: 6}),
		retractedEvent("ENV-RETRACT-M2", "ENV-M2", "R1", "MOD", "spam", 7),
		leftEvent("ENV-LEFT-U2", "R1", "U2", 8),
	})

	if got := p.RoomEventCount("R1"); got != 8 {
		t.Errorf("RoomEventCount = %d, want 8 (full event log including edits/retracts)", got)
	}

	// Newest-first ordering.
	got := p.RoomEvents("R1", 50, 0)
	if len(got) != 8 {
		t.Fatalf("RoomEvents len = %d, want 8", len(got))
	}
	wantOrder := []string{"ENV-LEFT-U2", "ENV-RETRACT-M2", "ENV-M2", "ENV-JOIN-U2", "ENV-EDIT-M1", "ENV-M1", "ENV-JOIN-U1", "ENV-CREATE"}
	for i, e := range got {
		if e.Event.GetId() != wantOrder[i] {
			t.Errorf("entry[%d] envelope id = %q, want %q", i, e.Event.GetId(), wantOrder[i])
		}
	}
}

func TestRoomTimeline_RoomIsolation(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-A", eventID: "A", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-B", eventID: "B", roomID: "R2", actorID: "U1", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-C", eventID: "C", roomID: "R1", actorID: "U1", at: 3}),
	})

	if got := p.RoomEventCount("R1"); got != 2 {
		t.Errorf("R1 count = %d, want 2", got)
	}
	if got := p.RoomEventCount("R2"); got != 1 {
		t.Errorf("R2 count = %d, want 1", got)
	}
	r1 := p.RoomEvents("R1", 10, 0)
	if len(r1) != 2 || r1[0].Event.GetId() != "ENV-C" || r1[1].Event.GetId() != "ENV-A" {
		t.Errorf("R1 timeline = %+v, want [ENV-C, ENV-A]", eventIDs(r1))
	}
}

func TestRoomTimeline_PaginationByStreamSeq(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "ENV-2", eventID: "M2", roomID: "R1", actorID: "U1", at: 2}),
		postedEvent(postedOpts{envelopeID: "ENV-3", eventID: "M3", roomID: "R1", actorID: "U1", at: 3}),
	})

	// limit
	if got := p.RoomEvents("R1", 1, 0); len(got) != 1 || got[0].Event.GetId() != "ENV-3" {
		t.Errorf("limit=1 = %v, want [ENV-3]", eventIDs(got))
	}
	// beforeStreamSeq excludes seq>=given
	if got := p.RoomEvents("R1", 10, 3); len(got) != 2 || got[0].Event.GetId() != "ENV-2" || got[1].Event.GetId() != "ENV-1" {
		t.Errorf("before=3 = %v, want [ENV-2, ENV-1]", eventIDs(got))
	}
	// beforeStreamSeq=1 means strictly older than seq 1 → empty
	if got := p.RoomEvents("R1", 10, 1); len(got) != 0 {
		t.Errorf("before=1 = %v, want []", eventIDs(got))
	}
}

func TestRoomTimeline_LookupByEnvelopeID(t *testing.T) {
	p := NewRoomTimelineProjection()
	applyAll(t, p, []*corev1.Event{
		postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", body: "hello", at: 1}),
		editedEvent("ENV-EDIT-M1", "ENV-M1", "R1", "U1", "hello (edited)", 2),
	})

	// Original post lookup.
	entry, ok := p.Get("ENV-M1")
	if !ok || entry.Event.GetId() != "ENV-M1" || entry.Event.GetMessagePosted() == nil {
		t.Errorf("Get(ENV-M1) = %v, want the post", entry)
	}
	// Edit also indexed.
	entry, ok = p.Get("ENV-EDIT-M1")
	if !ok || entry.Event.GetMessageEdited().GetEventId() != "ENV-M1" {
		t.Errorf("Get(ENV-EDIT-M1) = %v, want the edit", entry)
	}
	// Unknown.
	if _, ok := p.Get("nope"); ok {
		t.Error("Get(nope) should be ok=false")
	}
}

func TestRoomTimeline_Idempotency(t *testing.T) {
	p := NewRoomTimelineProjection()
	e := postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1})
	if err := p.Apply(e, 1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := p.Apply(e, 1); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if got := p.RoomEventCount("R1"); got != 1 {
		t.Errorf("RoomEventCount after duplicate Apply = %d, want 1", got)
	}
}

func TestRoomTimeline_VideoManifestLatestState(t *testing.T) {
	p := NewRoomTimelineProjection()
	processed := &corev1.Event{
		Id: "ENV-VIDEO-OK",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: "A-video",
				Video: &corev1.AssetProcessedVideo{
					DurationMs: 1200,
					Width:      640,
					Height:     360,
					Variants: []*corev1.AssetVideoVariant{{
						Quality: "480p",
						AssetId: "A-video-480",
					}},
				},
			},
		},
	}
	failed := &corev1.Event{
		Id: "ENV-VIDEO-FAIL",
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:     "A-video",
				FailureCode: corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING,
			},
		},
	}

	applyAll(t, p, []*corev1.Event{attachmentDeclaredEvent("R1", "A-video", "video/mp4"), failed, processed})
	manifest, ok := p.VideoAttachmentManifest("A-video")
	if !ok || manifest.Succeeded == nil {
		t.Fatalf("VideoAttachmentManifest = %#v, want processed manifest", manifest)
	}
	video := manifest.Succeeded.GetVideo()
	if video.GetDurationMs() != 1200 || len(video.GetVariants()) != 1 {
		t.Errorf("processed manifest = %+v, want duration and one variant", manifest.Succeeded)
	}

	manifest.Succeeded.GetVideo().Variants[0].Quality = "mutated"
	again, _ := p.VideoAttachmentManifest("A-video")
	if again.Succeeded.GetVideo().Variants[0].Quality != "480p" {
		t.Error("VideoAttachmentManifest should return clones")
	}
}

func TestRoomTimeline_UnmanifestedVideoAttachments(t *testing.T) {
	p := NewRoomTimelineProjection()
	post := postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1})
	post.GetMessagePosted().Body.Attachments = []*corev1.Attachment{
		{Id: "A-video", ContentType: "video/mp4"},
		{Id: "A-image", ContentType: "image/png"},
	}
	processed := &corev1.Event{
		Id: "ENV-VIDEO-OK",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: "A-video",
				Video:   &corev1.AssetProcessedVideo{},
			},
		},
	}

	// New uploads emit AssetCreatedEvent with an empty message_event_id
	// (the message doesn't exist yet at upload time). Message ownership is
	// reconstructed from the posting message's attachments, so recovery must
	// still find A-video without relying on the deprecated field.
	applyAll(t, p, []*corev1.Event{post, attachmentDeclaredEvent("R1", "A-video", "video/mp4"), attachmentDeclaredEvent("R1", "A-image", "image/png")})
	got := p.UnmanifestedVideoAttachments()
	if len(got) != 1 || got[0].Attachment.GetId() != "A-video" {
		t.Fatalf("UnmanifestedVideoAttachments before manifest = %+v, want A-video", got)
	}
	if got[0].RoomID != "R1" || got[0].MessageEventID != "ENV-M1" {
		t.Fatalf("UnmanifestedVideoAttachments ownership = room %q msg %q, want R1/ENV-M1", got[0].RoomID, got[0].MessageEventID)
	}
	applyAll(t, p, []*corev1.Event{processed})
	if got := p.UnmanifestedVideoAttachments(); len(got) != 0 {
		t.Fatalf("UnmanifestedVideoAttachments after manifest = %+v, want none", got)
	}
}

// A retracted message's video must not be re-enqueued by boot recovery —
// it's no longer visible, so transcoding it again is wasted work.
func TestRoomTimeline_UnmanifestedVideoAttachments_SkipsRetracted(t *testing.T) {
	p := NewRoomTimelineProjection()
	post := postedEvent(postedOpts{envelopeID: "ENV-M1", eventID: "M1", roomID: "R1", actorID: "U1", at: 1})
	post.GetMessagePosted().Body.Attachments = []*corev1.Attachment{{Id: "A-video", ContentType: "video/mp4"}}

	applyAll(t, p, []*corev1.Event{post, attachmentDeclaredEvent("R1", "A-video", "video/mp4")})
	if got := p.UnmanifestedVideoAttachments(); len(got) != 1 {
		t.Fatalf("UnmanifestedVideoAttachments before retract = %+v, want A-video", got)
	}

	retract := &corev1.Event{
		Id: "ENV-RETRACT",
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "ENV-M1"},
		},
	}
	applyAll(t, p, []*corev1.Event{retract})
	if got := p.UnmanifestedVideoAttachments(); len(got) != 0 {
		t.Fatalf("UnmanifestedVideoAttachments after retract = %+v, want none", got)
	}
}

// A cyclic derivative parent chain in (corrupt/replayed) EVT data must not
// loop forever while the projection mutex is held — the room walk is
// cycle-guarded.
func TestRoomTimeline_RoomIDOfAssetCreated_CycleGuardDoesNotHang(t *testing.T) {
	p := NewRoomTimelineProjection()
	// Two roomless derivatives that name each other as parent. Applying the
	// second triggers the full A→B→A walk; without the guard this recurses
	// forever. The test simply has to return.
	cyclicAsset := func(id, parentID string) *corev1.Event {
		return &corev1.Event{
			Id: "ENV-" + id,
			Event: &corev1.Event_AssetCreated{
				AssetCreated: &corev1.AssetCreatedEvent{
					Asset:         &corev1.AssetRecord{Id: id, ContentType: "video/mp4"},
					ParentAssetId: parentID,
				},
			},
		}
	}
	applyAll(t, p, []*corev1.Event{cyclicAsset("A", "B"), cyclicAsset("B", "A")})
	// A processing event for the cyclic asset resolves its room via the walk;
	// the guard yields "" and the event is dropped rather than hanging.
	started := &corev1.Event{
		Id: "ENV-START",
		Event: &corev1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{AssetId: "A"},
		},
	}
	if err := p.Apply(started, 3); err != nil {
		t.Fatalf("Apply started: %v", err)
	}
}

func TestRoomTimeline_NonRoomEventsSkipped(t *testing.T) {
	// SpaceMemberDeletedEvent is in the proto's "Room membership" block
	// (oneof tag 320) but carries no room_id. It's published to
	// server.member.> in practice, never to evt.room.>, but if one ever
	// slipped through the filter, roomIDOfEvent would return "" and the
	// entry would be skipped rather than crashing.
	p := NewRoomTimelineProjection()
	stray := &corev1.Event{
		Id:        "ENV-STRAY",
		CreatedAt: timestamppb.New(fixedTime(1)),
		Event: &corev1.Event_SpaceMemberDeleted{
			SpaceMemberDeleted: &corev1.SpaceMemberDeletedEvent{UserId: "U1"},
		},
	}
	if err := p.Apply(stray, 1); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := p.RoomEventCount("R1"); got != 0 {
		t.Errorf("non-room event should not land in any room timeline, got count=%d", got)
	}
}

func TestRoomTimeline_SubjectFilter(t *testing.T) {
	subjects := NewRoomTimelineProjection().Subjects()
	want := map[string]bool{"evt.room.>": true, "evt.user.>": true}
	if len(subjects) != len(want) {
		t.Fatalf("expected %d subject filters, got %d", len(want), len(subjects))
	}
	for _, subject := range subjects {
		if !want[subject] {
			t.Errorf("unexpected subject filter %q", subject)
		}
	}
}

// =============================================================================
// Helpers for assertion noise
// =============================================================================

func eventIDs(entries []*TimelineEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Event.GetId()
	}
	return out
}
