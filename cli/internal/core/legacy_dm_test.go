package core

import (
	"context"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// createLegacyDMForTest replays the facts written by servers that allowed groups.
// It does not bypass the creation rule in any production command.
func createLegacyDMForTest(t *testing.T, c *ChattoCore, ctx context.Context, ids []string) *readmodel.Room {
	t.Helper()
	id := DMRoomID(ids)
	agg := evtstream.RoomAggregate(id)
	created := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: id, Kind: evtv1.RoomKind_ROOM_KIND_DM}}})
	entries := []evtstream.BatchEntry{{Subject: agg.SubjectFor(created), Event: created, Expect: events.ExpectFilterSeq(agg.AllEventsFilter(), 0)}}
	for _, userID := range ids {
		joined := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: id}}})
		entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(joined), Event: joined})
	}
	seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		t.Fatal(err)
	}
	position := events.SubjectPosition(entries[len(entries)-1].Subject, seqs[len(seqs)-1])
	if err := c.roomCommands.rooms.waitForDirectory(ctx, position); err != nil {
		t.Fatal(err)
	}
	if err := c.roomCommands.rooms.waitForTimeline(ctx, position); err != nil {
		t.Fatal(err)
	}
	return &readmodel.Room{Id: id, Kind: evtv1.RoomKind_ROOM_KIND_DM}
}
