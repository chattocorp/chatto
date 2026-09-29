package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

func TestThreadProjection_ParticipantPreviewKeepsFirstReplyOrder(t *testing.T) {
	p := NewThreadProjection()
	events := []*evtv1.Event{postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "U-ROOT", at: 1})}
	var authors []string
	for i := range maxThreadParticipants + 10 {
		author := fmt.Sprintf("U%03d", i)
		authors = append(authors, author)
		events = append(events, postedEvent(postedOpts{envelopeID: "REPLY-" + author, roomID: "R1", actorID: author, inThread: "ROOT", at: 2 + i}))
	}
	// A second reply by the first author keeps that author after a retraction.
	events = append(events, postedEvent(postedOpts{envelopeID: "REPLY-AGAIN", roomID: "R1", actorID: authors[0], inThread: "ROOT", at: 100}))
	applyAll(t, p, events)

	root, _ := p.eventIDs.lookup("ROOT")
	if p.summaryByThread[root].participantIndex == nil {
		t.Fatalf("summary with %d authors has no participant index", len(authors))
	}
	metadata := p.ThreadMetadata("ROOT")
	if !slices.Equal(metadata.ParticipantIDs, authors[:maxThreadParticipants]) || metadata.ParticipantCount != len(authors) {
		t.Fatalf("preview=%v count=%d; want first %d authors and %d", metadata.ParticipantIDs, metadata.ParticipantCount, maxThreadParticipants, len(authors))
	}

	retractions := []*evtv1.Event{
		retractedEvent("RETRACT-FIRST", "REPLY-"+authors[0], "R1", "MOD", "", 200),
		retractedEvent("RETRACT-SECOND", "REPLY-"+authors[1], "R1", "MOD", "", 201),
	}
	for i, event := range retractions {
		if err := p.Apply(event, uint64(len(events)+i+1)); err != nil {
			t.Fatal(err)
		}
	}
	// The preview follows first visible replies, so the first author moves to
	// the position of their remaining reply.
	metadata = p.ThreadMetadata("ROOT")
	want := authors[2 : maxThreadParticipants+2]
	if !slices.Equal(metadata.ParticipantIDs, want) || metadata.ParticipantCount != len(authors)-1 || metadata.ReplyCount != len(authors)-1 {
		t.Fatalf("after retractions preview=%v count=%d replies=%d; want %v, %d, %d", metadata.ParticipantIDs, metadata.ParticipantCount, metadata.ReplyCount, want, len(authors)-1, len(authors)-1)
	}
	if got := p.ParticipantIDs("ROOT"); len(got) != len(authors)-1 || slices.Contains(got, authors[1]) || got[len(got)-1] != authors[0] {
		t.Fatalf("ParticipantIDs = %v, want all authors except %s", got, authors[1])
	}
}

func TestThreadProjection_FollowIndexesKeepFollowOrder(t *testing.T) {
	p := NewThreadProjection()
	for i, event := range []*evtv1.Event{
		threadFollowSnapshotTestEvent("F1", "R1", "ROOT", "U2", true),
		threadFollowSnapshotTestEvent("F2", "R1", "ROOT", "U1", true),
		threadFollowSnapshotTestEvent("F3", "R1", "ROOT-2", "U1", true),
		threadFollowSnapshotTestEvent("F4", "R1", "ROOT", "U2", false),
		threadFollowSnapshotTestEvent("F5", "R1", "ROOT", "U2", true),
		threadFollowSnapshotTestEvent("F6", "R1", "ROOT-3", "U3", false),
	} {
		if err := p.Apply(event, uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.ThreadFollowers("R1", "ROOT"); !slices.Equal(got, []string{"U1", "U2"}) {
		t.Fatalf("ThreadFollowers = %v, want [U1 U2]", got)
	}
	if got := p.FollowedThreadsForUser("U1"); !slices.Equal(got, []threadFollowRef{{roomID: "R1", threadRootEventID: "ROOT"}, {roomID: "R1", threadRootEventID: "ROOT-2"}}) {
		t.Fatalf("FollowedThreadsForUser(U1) = %v", got)
	}
	for _, check := range []struct {
		user, root string
		want       ThreadFollowState
	}{
		{"U2", "ROOT", ThreadFollowStateFollowing},
		{"U3", "ROOT-3", ThreadFollowStateUnfollowed},
		{"U3", "ROOT", ThreadFollowStateNone},
		{"U-UNKNOWN", "ROOT", ThreadFollowStateNone},
	} {
		if got := p.FollowState(check.user, "R1", check.root); got != check.want {
			t.Fatalf("FollowState(%s, %s) = %q, want %q", check.user, check.root, got, check.want)
		}
	}
	if got := p.FollowedThreadsForUser("U3"); got != nil {
		t.Fatalf("FollowedThreadsForUser(U3) = %v, want nil", got)
	}
}

func TestThreadProjectionSnapshotRestoreRejectsInconsistentReplies(t *testing.T) {
	source := NewThreadProjection()
	applyAll(t, source, []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
	})
	data, err := source.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*projectionv1.ThreadProjectionSnapshot)
		want   string
	}{
		{"missing reply row", func(s *projectionv1.ThreadProjectionSnapshot) { s.Replies = nil }, "has no matching reply"},
		{"reply outside timelines", func(s *projectionv1.ThreadProjectionSnapshot) {
			s.Replies = append(s.Replies, &projectionv1.ThreadReplySnapshot{EventId: "OTHER", ThreadRootEventId: "ROOT"})
		}, "outside thread timelines"},
		{"reply with another root", func(s *projectionv1.ThreadProjectionSnapshot) { s.Replies[0].ThreadRootEventId = "ROOT-2" }, "has no matching reply"},
		{"repeated reply", func(s *projectionv1.ThreadProjectionSnapshot) { s.Replies = append(s.Replies, s.Replies[0]) }, "repeats reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var snapshot projectionv1.ThreadProjectionSnapshot
			if err := proto.Unmarshal(data, &snapshot); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&snapshot)
			mutated, err := proto.Marshal(&snapshot)
			if err != nil {
				t.Fatal(err)
			}
			target := NewThreadProjection()
			applyAll(t, target, []*evtv1.Event{threadFollowSnapshotTestEvent("FOLLOW", "R1", "ROOT", "U1", true)})
			err = target.Restore(mutated)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Restore error = %v, want %q", err, tc.want)
			}
			if got := target.FollowState("U1", "R1", "ROOT"); got != ThreadFollowStateFollowing || target.ReplyCount("ROOT") != 0 {
				t.Fatalf("failed restore changed state: follow=%q replies=%d", got, target.ReplyCount("ROOT"))
			}
		})
	}
}
