package core

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestReactionProjection_AddRemoveAndBatch(t *testing.T) {
	p := NewReactionProjection()

	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("E1", "M1", "U2", "heart", 2))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("E2", "M1", "U1", "heart", 3))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("E3", "M1", "U3", "thumbsup", 1))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("E4", "M2", "U1", "tada", 4))

	if !p.HasReaction("M1", "heart", "U1") {
		t.Fatal("expected U1 heart reaction on M1")
	}

	summaries := p.Reactions("M1")
	if len(summaries) != 2 {
		t.Fatalf("Reactions(M1) len = %d, want 2", len(summaries))
	}
	if summaries[0].Emoji != "thumbsup" {
		t.Fatalf("first summary emoji = %q, want thumbsup", summaries[0].Emoji)
	}
	if summaries[1].Emoji != "heart" {
		t.Fatalf("second summary emoji = %q, want heart", summaries[1].Emoji)
	}
	if got := summaries[1].UserIDs; len(got) != 2 || got[0] != "U1" || got[1] != "U2" {
		t.Fatalf("heart users = %v, want [U1 U2]", got)
	}

	batch := p.ReactionsBatch([]string{"M1", "M2", "M3"})
	if len(batch["M1"]) != 2 {
		t.Fatalf("batch[M1] len = %d, want 2", len(batch["M1"]))
	}
	if len(batch["M2"]) != 1 || batch["M2"][0].Emoji != "tada" {
		t.Fatalf("batch[M2] = %+v, want tada", batch["M2"])
	}
	if _, ok := batch["M3"]; ok {
		t.Fatalf("batch contains M3 with no reactions: %+v", batch["M3"])
	}

	applyReactionProjectionEvent(t, p, reactionRemovedProjectionEvent("E5", "M1", "U1", "heart"))
	if p.HasReaction("M1", "heart", "U1") {
		t.Fatal("expected removed U1 heart reaction")
	}
	if p.HasReaction("M1", "heart", "U2") == false {
		t.Fatal("expected U2 heart reaction to remain")
	}
}

func TestReactionProjection_IgnoresDuplicateEventID(t *testing.T) {
	p := NewReactionProjection()

	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("E1", "M1", "U1", "heart", 1))
	applyReactionProjectionEvent(t, p, reactionRemovedProjectionEvent("E1", "M1", "U1", "heart"))

	if !p.HasReaction("M1", "heart", "U1") {
		t.Fatal("duplicate event id should have been ignored")
	}
}

func TestReactionProjection_CanonicalizesEchoReactionReplay(t *testing.T) {
	p := NewReactionProjection()

	applyReactionProjectionEvent(t, p, messagePostedProjectionEvent("M1", ""))
	applyReactionProjectionEvent(t, p, messagePostedProjectionEvent("ECHO1", "M1"))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("R1", "ECHO1", "U1", "thumbsup", 1))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("R2", "M1", "U1", "thumbsup", 2))
	applyReactionProjectionEvent(t, p, reactionAddedProjectionEvent("R3", "M1", "U2", "heart", 3))

	if !p.HasReaction("M1", "thumbsup", "U1") {
		t.Fatal("expected echo-keyed add to appear on original")
	}
	if !p.HasReaction("ECHO1", "thumbsup", "U1") {
		t.Fatal("expected echo reads to resolve to original reaction state")
	}

	summaries := p.Reactions("M1")
	if len(summaries) != 2 {
		t.Fatalf("Reactions(M1) len = %d, want 2", len(summaries))
	}
	for _, summary := range summaries {
		if summary.Emoji == "thumbsup" && len(summary.UserIDs) != 1 {
			t.Fatalf("thumbsup users = %v, want duplicate collapsed to one user", summary.UserIDs)
		}
	}

	batch := p.ReactionsBatch([]string{"M1", "ECHO1"})
	if len(batch["M1"]) != 2 || len(batch["ECHO1"]) != 2 {
		t.Fatalf("batch = %+v, want original and echo keys hydrated", batch)
	}

	applyReactionProjectionEvent(t, p, reactionRemovedProjectionEvent("R4", "ECHO1", "U1", "thumbsup"))
	if p.HasReaction("M1", "thumbsup", "U1") {
		t.Fatal("expected echo-keyed remove to remove original reaction")
	}
	if p.HasReaction("ECHO1", "thumbsup", "U1") {
		t.Fatal("expected echo reads to reflect removed original reaction")
	}
	if !p.HasReaction("M1", "heart", "U2") {
		t.Fatal("expected unrelated original reaction to remain")
	}
}

func TestReactionProjection_MutationSnapshotTracksRoomSeq(t *testing.T) {
	p := NewReactionProjection()

	roomEvent := &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: "R1", Name: "general"},
		},
	}
	if err := p.Apply(roomEvent, 7); err != nil {
		t.Fatalf("apply room event: %v", err)
	}

	snapshot := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1")
	if snapshot.Exists {
		t.Fatal("fresh reaction snapshot unexpectedly exists")
	}
	if snapshot.Seq != 7 {
		t.Fatalf("fresh reaction snapshot seq = %d, want 7", snapshot.Seq)
	}

	event := reactionAddedProjectionEvent("E1", "M1", "U1", "heart", 1)
	if err := p.Apply(event, 8); err != nil {
		t.Fatalf("apply reaction event: %v", err)
	}

	snapshot = p.ReactionMutationSnapshot("R1", "M1", "heart", "U1")
	if !snapshot.Exists {
		t.Fatal("reaction snapshot should report existing reaction")
	}
	if snapshot.Seq != 8 {
		t.Fatalf("reaction snapshot seq = %d, want 8", snapshot.Seq)
	}

	otherRoomEvent := &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: "R2", Name: "other"},
		},
	}
	if err := p.Apply(otherRoomEvent, 9); err != nil {
		t.Fatalf("apply other room event: %v", err)
	}
	if got := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1").Seq; got != 8 {
		t.Fatalf("R1 reaction snapshot seq after R2 event = %d, want 8", got)
	}
	if got := p.ReactionMutationSnapshot("R2", "M1", "heart", "U1").Seq; got != 9 {
		t.Fatalf("R2 reaction snapshot seq = %d, want 9", got)
	}
}

func TestReactionProjection_IgnoresNonRoomEventsForSnapshotSeq(t *testing.T) {
	p := NewReactionProjection()

	assetEvent := &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				Asset: &corev1.AssetRecord{Id: "A1"},
			},
		},
	}
	if err := p.Apply(assetEvent, 10); err != nil {
		t.Fatalf("apply asset aggregate event: %v", err)
	}
	if got := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1").Seq; got != 0 {
		t.Fatalf("snapshot seq after asset aggregate event = %d, want 0", got)
	}
}

func TestReactionProjection_MutationSnapshotTracksLegacyRoomAssetEvents(t *testing.T) {
	p := NewReactionProjection()

	message := &corev1.Event{
		Id: "M1",
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	}
	if err := p.Apply(message, 9); err != nil {
		t.Fatalf("apply message event: %v", err)
	}

	assetCreated := &corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				RoomId: "R1",
				Asset:  &corev1.AssetRecord{Id: "A1"},
			},
		},
	}
	if err := p.Apply(assetCreated, 10); err != nil {
		t.Fatalf("apply legacy room asset-created event: %v", err)
	}
	if got := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1").Seq; got != 10 {
		t.Fatalf("snapshot seq after legacy asset-created event = %d, want 10", got)
	}

	assetStarted := &corev1.Event{
		Event: &corev1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{
				AssetId:        "A2",
				MessageEventId: "M1",
			},
		},
	}
	if err := p.Apply(assetStarted, 11); err != nil {
		t.Fatalf("apply legacy room asset-processing event: %v", err)
	}
	if got := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1").Seq; got != 11 {
		t.Fatalf("snapshot seq after legacy asset-processing event = %d, want 11", got)
	}

	assetDeleted := &corev1.Event{
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: "A1"},
		},
	}
	if err := p.Apply(assetDeleted, 12); err != nil {
		t.Fatalf("apply legacy room asset-deleted event: %v", err)
	}
	if got := p.ReactionMutationSnapshot("R1", "M1", "heart", "U1").Seq; got != 12 {
		t.Fatalf("snapshot seq after legacy asset-deleted event = %d, want 12", got)
	}
}

func TestRoomLayoutProjection_ReorderCloneAndIgnore(t *testing.T) {
	p := NewRoomLayoutProjection()

	if got := p.Order(); len(got) != 0 {
		t.Fatalf("fresh order = %v, want empty", got)
	}

	if err := p.Apply(&corev1.Event{Event: &corev1.Event_RoomGroupsReordered{
		RoomGroupsReordered: &corev1.RoomGroupsReorderedEvent{GroupIds: []string{"G1", "G2"}},
	}}, 1); err != nil {
		t.Fatalf("apply reorder: %v", err)
	}

	order := p.Order()
	if len(order) != 2 || order[0] != "G1" || order[1] != "G2" {
		t.Fatalf("order = %v, want [G1 G2]", order)
	}
	order[0] = "mutated"
	if got := p.Order(); got[0] != "G1" {
		t.Fatalf("projection order mutated through returned slice: %v", got)
	}

	if err := p.Apply(&corev1.Event{Event: &corev1.Event_RoomGroupCreated{
		RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: "G3"},
	}}, 2); err != nil {
		t.Fatalf("apply unrelated event: %v", err)
	}
	if got := p.Order(); len(got) != 2 || got[0] != "G1" || got[1] != "G2" {
		t.Fatalf("order after unrelated event = %v, want [G1 G2]", got)
	}
}

func messagePostedProjectionEvent(id, echoOfEventID string) *corev1.Event {
	return &corev1.Event{
		Id: id,
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				RoomId:        "R1",
				EchoOfEventId: echoOfEventID,
			},
		},
	}
}

func reactionAddedProjectionEvent(id, messageID, actorID, emoji string, second int) *corev1.Event {
	return &corev1.Event{
		Id:        id,
		ActorId:   actorID,
		CreatedAt: timestamppb.New(time.Date(2026, 5, 26, 12, 0, second, 0, time.UTC)),
		Event: &corev1.Event_ReactionAdded{
			ReactionAdded: &corev1.ReactionAddedEvent{
				RoomId:         "R1",
				MessageEventId: messageID,
				Emoji:          emoji,
			},
		},
	}
}

func reactionRemovedProjectionEvent(id, messageID, actorID, emoji string) *corev1.Event {
	return &corev1.Event{
		Id:      id,
		ActorId: actorID,
		Event: &corev1.Event_ReactionRemoved{
			ReactionRemoved: &corev1.ReactionRemovedEvent{
				RoomId:         "R1",
				MessageEventId: messageID,
				Emoji:          emoji,
			},
		},
	}
}

func applyReactionProjectionEvent(t *testing.T, p *ReactionProjection, event *corev1.Event) {
	t.Helper()
	if err := p.Apply(event, 0); err != nil {
		t.Fatalf("apply event: %v", err)
	}
}
