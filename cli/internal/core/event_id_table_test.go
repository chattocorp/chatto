package core

import (
	"fmt"
	"hash/maphash"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestEventIDTable_InternsDenseHandles(t *testing.T) {
	table := newEventIDTable()
	if got := table.intern(""); got != 0 {
		t.Fatalf("intern empty = %d, want 0", got)
	}
	first := table.intern("E1")
	second := table.intern("E2")
	if first != 1 || second != 2 {
		t.Fatalf("handles = %d, %d; want 1, 2", first, second)
	}
	if again := table.intern("E1"); again != first {
		t.Fatalf("repeated intern = %d, want %d", again, first)
	}
	if handle, ok := table.lookup("E2"); !ok || handle != second {
		t.Fatalf("lookup E2 = %d, %v; want %d, true", handle, ok, second)
	}
	if _, ok := table.lookup("E3"); ok {
		t.Fatal("lookup of unknown ID succeeded")
	}
	if got := table.len(); got != 2 {
		t.Fatalf("len after lookup of unknown ID = %d, want 2", got)
	}
	if got := table.id(second); got != "E2" {
		t.Fatalf("id(%d) = %q, want E2", second, got)
	}
	if got := table.id(0); got != "" {
		t.Fatalf("id(0) = %q, want empty", got)
	}
}

func TestEventIDTable_ResolvesHashCollisions(t *testing.T) {
	table := newEventIDTable()
	first := table.intern("E1")
	// Point the hash of E2 at E1 to simulate a 64-bit hash collision.
	hash := maphash.String(table.seed, "E2")
	shard := table.shard(hash)
	if shard.byHash == nil {
		shard.byHash = make(map[uint64]uint32)
	}
	shard.byHash[hash] = first

	second := table.intern("E2")
	if second == first {
		t.Fatal("colliding ID received the handle of a different ID")
	}
	if handle, ok := table.lookup("E2"); !ok || handle != second {
		t.Fatalf("lookup E2 = %d, %v; want %d, true", handle, ok, second)
	}
	if handle, ok := table.lookup("E1"); !ok || handle != first {
		t.Fatalf("lookup E1 = %d, %v; want %d, true", handle, ok, first)
	}
	if again := table.intern("E2"); again != second {
		t.Fatalf("repeated colliding intern = %d, want %d", again, second)
	}
	if table.estimatedBytes() <= 0 {
		t.Fatal("estimate does not include the table")
	}
}

func TestEventIDTable_KeepsIDsAcrossArenaChunks(t *testing.T) {
	table := newEventIDTable()
	long := strings.Repeat("L", idArenaMaxChunkBytes/4+1)
	ids := make([]string, 0, 20_002)
	for i := range 10_000 {
		ids = append(ids, fmt.Sprintf("E%014d", i))
	}
	ids = append(ids, long)
	for i := range 10_000 {
		ids = append(ids, fmt.Sprintf("historical-import-%010d", i))
	}
	ids = append(ids, long+"2")
	handles := make([]uint32, len(ids))
	for i, id := range ids {
		handles[i] = table.intern(id)
	}
	chunks := table.arena.loadChunks()
	if len(chunks) < 3 {
		t.Fatalf("arena used %d chunks, want several", len(chunks))
	}
	for i, id := range ids {
		if got := table.id(handles[i]); got != id {
			t.Fatalf("id(%d) = %q, want %q", handles[i], got, id)
		}
		if handle, ok := table.lookup(id); !ok || handle != handles[i] {
			t.Fatalf("lookup(%q) = %d, %v; want %d, true", id, handle, ok, handles[i])
		}
	}
	for _, chunk := range chunks {
		if len(chunk) > idArenaMaxChunkBytes && len(chunk) != len(long) && len(chunk) != len(long)+1 {
			t.Fatalf("chunk of %d bytes exceeds the shared chunk limit", len(chunk))
		}
	}
}

func TestEventIDTable_ConcurrentInternAndRead(t *testing.T) {
	table := newEventIDTable()
	var wg sync.WaitGroup
	for writer := range 4 {
		wg.Go(func() {
			for i := range 1_000 {
				id := fmt.Sprintf("E%d-%d", writer, i)
				handle := table.intern(id)
				if got := table.id(handle); got != id {
					t.Errorf("id(%d) = %q, want %q", handle, got, id)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for i := range 4_000 {
			table.lookup(fmt.Sprintf("E0-%d", i%1_000))
			table.len()
		}
	})
	wg.Wait()
	if got := table.len(); got != 4_000 {
		t.Fatalf("len = %d, want 4000", got)
	}
}

// TestSharedEventIDTable_ComponentsShareHandlesAcrossRestore verifies that the
// ServerContentView components hold each message ID once and keep sharing the
// table after a snapshot restore.
func TestSharedEventIDTable_ComponentsShareHandlesAcrossRestore(t *testing.T) {
	eventIDs := newEventIDTable()
	timeline := newRoomTimelineProjection(eventIDs)
	threads := newThreadProjection(eventIDs)
	reactions := newReactionProjection(eventIDs)
	events := []*evtv1.Event{
		{
			Id: "ENV-ROOM", ActorId: "U1", CreatedAt: timestamppb.New(fixedTime(0)),
			Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}},
		},
		postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 1}),
		postedEvent(postedOpts{envelopeID: "M2", roomID: "R1", actorID: "U2", inThread: "M1", at: 2}),
		reactionAddedProjectionEvent("REACTION-1", "M1", "U2", "wave", 3),
	}
	for i, event := range events {
		for _, projection := range []testProjection{timeline, threads, reactions} {
			if err := projection.Apply(event, uint64(i+1)); err != nil {
				t.Fatalf("apply event %d to %T: %v", i+1, projection, err)
			}
		}
	}
	// ENV-ROOM, M1, and M2 are indexed once each for all three components.
	if got := eventIDs.len(); got != 3 {
		t.Fatalf("shared event IDs = %d, want 3", got)
	}

	for _, projection := range []snapshotProjection{timeline, threads, reactions} {
		payload, err := projection.Snapshot()
		if err != nil {
			t.Fatalf("snapshot %T: %v", projection, err)
		}
		if err := projection.Restore(payload); err != nil {
			t.Fatalf("restore %T: %v", projection, err)
		}
	}
	if got := eventIDs.len(); got != 3 {
		t.Fatalf("shared event IDs after restore = %d, want 3", got)
	}
	if timeline.eventIDs != eventIDs || threads.eventIDs != eventIDs || reactions.messages != eventIDs {
		t.Fatal("restore replaced the shared event ID table")
	}
	if entry, ok := timeline.Get("M2"); !ok || entry.ThreadRootEventID != "M1" || !entry.CreatedAt.Equal(fixedTime(2)) {
		t.Fatalf("timeline Get(M2) = %+v, %v; want thread root M1 at fixed time 2", entry, ok)
	}
	if root, ok := threads.ThreadRootForMessage("R1", "M2"); !ok || root != "M1" {
		t.Fatalf("ThreadRootForMessage(M2) = %q, %v; want M1, true", root, ok)
	}
	if got := reactions.Reactions("M1"); len(got) != 1 || got[0].Emoji != "wave" {
		t.Fatalf("Reactions(M1) = %+v, want one wave reaction", got)
	}
}

func TestEventIDTable_ConcurrentInternOfSameIDsAgreesOnHandles(t *testing.T) {
	table := newEventIDTable()
	const workers, ids = 8, 2_000
	results := make([][]uint32, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			handles := make([]uint32, ids)
			for i := range ids {
				// Workers visit every ID, starting at different offsets.
				n := (i + worker*ids/workers) % ids
				handles[n] = table.intern(fmt.Sprintf("E%05d", n))
			}
			results[worker] = handles
		})
	}
	wg.Wait()
	if got := table.len(); got != ids {
		t.Fatalf("len = %d, want %d", got, ids)
	}
	for worker := 1; worker < workers; worker++ {
		if !slices.Equal(results[worker], results[0]) {
			t.Fatalf("worker %d received different handles than worker 0", worker)
		}
	}
	for n, handle := range results[0] {
		if got := table.id(handle); got != fmt.Sprintf("E%05d", n) {
			t.Fatalf("id(%d) = %q, want E%05d", handle, got, n)
		}
	}
}

// TestEventIDTable_ReadsPublishedHandlesDuringGrowth reads handles that other
// goroutines published while writers add location pages and arena chunks. It
// models components that apply and read under different locks.
func TestEventIDTable_ReadsPublishedHandlesDuringGrowth(t *testing.T) {
	table := newEventIDTable()
	const writers, perWriter = 4, 3 * idLocationPageSize
	var published sync.Map
	var writing sync.WaitGroup
	var done atomic.Bool
	for writer := range writers {
		writing.Go(func() {
			for i := range perWriter {
				id := fmt.Sprintf("W%d-%06d", writer, i)
				published.Store(id, table.intern(id))
			}
		})
	}
	var reading sync.WaitGroup
	for range 4 {
		reading.Go(func() {
			// Read every published ID until the writers finish, then once more.
			for last := false; !last; {
				last = done.Load()
				published.Range(func(key, value any) bool {
					id, handle := key.(string), value.(uint32)
					if got := table.id(handle); got != id {
						t.Errorf("id(%d) = %q, want %q", handle, got, id)
						return false
					}
					if got, ok := table.lookup(id); !ok || got != handle {
						t.Errorf("lookup(%q) = %d, %v; want %d, true", id, got, ok, handle)
						return false
					}
					return true
				})
			}
		})
	}
	writing.Wait()
	done.Store(true)
	reading.Wait()
	if got := table.len(); got != writers*perWriter {
		t.Fatalf("len = %d, want %d", got, writers*perWriter)
	}
	if pages := len(table.loadPages()); pages < writers*perWriter/idLocationPageSize {
		t.Fatalf("table used %d location pages, want at least %d", pages, writers*perWriter/idLocationPageSize)
	}
}
