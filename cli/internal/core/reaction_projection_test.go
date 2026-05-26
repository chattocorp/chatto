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
