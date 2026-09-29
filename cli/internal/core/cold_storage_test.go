package core

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// CHATTO_TEST_COLD_STORAGE=1 runs the package with two-row blocks and a
// one-nanosecond cold window, so almost every projection row is frozen.
func init() {
	if os.Getenv("CHATTO_TEST_COLD_STORAGE") == "1" {
		coldBlockShift = 1
		coldStorageTestWindow = time.Nanosecond
		idLocationPageSize = 4
	}
}

type coldTestRow struct {
	seq     uint64
	at      int64
	handle  uint32
	flag    bool
	maximum uint64
}

type coldTestCodec struct{}

func (coldTestCodec) columns() int { return 5 }

func (coldTestCodec) encode(row coldTestRow) (fields coldFields) {
	fields[0] = row.seq
	fields[1] = uint64(row.at)
	fields[2] = uint64(row.handle)
	fields[3] = 0
	if row.flag {
		fields[3] = 1
	}
	fields[4] = row.maximum
	return fields
}

func (coldTestCodec) decode(fields coldFields) coldTestRow {
	return coldTestRow{seq: fields[0], at: int64(fields[1]), handle: uint32(fields[2]), flag: fields[3] == 1, maximum: fields[4]}
}

func randomColdRow(random *rand.Rand, i int) coldTestRow {
	row := coldTestRow{seq: uint64(i*3 + random.IntN(3)), at: 1_700_000_000_000_000_000 + int64(i)*int64(random.IntN(1_000_000_000))}
	switch random.IntN(4) {
	case 0:
		row.at = -row.at
	case 1:
		row.at = 0
	}
	row.handle = uint32(random.IntN(1 << 20))
	row.flag = random.IntN(2) == 1
	if random.IntN(10) == 0 {
		row.maximum = math.MaxUint64 - uint64(random.IntN(5))
	}
	return row
}

func TestPackColumns_RoundTripsWidthsUpTo64Bits(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	for _, rows := range []int{0, 1, 7, coldBlockRows()} {
		values := make([]coldFields, rows)
		for row := range values {
			values[row][0] = 42                     // constant: zero width
			values[row][1] = uint64(row)            // small width
			values[row][2] = random.Uint64()        // full 64-bit width
			values[row][3] = random.Uint64() >> 30  // unaligned width
			values[row][4] = uint64(random.IntN(2)) // one bit
		}
		packed := packColumns(5, values)
		for row := range values {
			for column := range 5 {
				if got := packed.get(row, column); got != values[row][column] {
					t.Fatalf("rows=%d get(%d, %d) = %d, want %d", rows, row, column, got, values[row][column])
				}
			}
		}
	}
}

func TestColdSlice_ReadsFrozenRowsAndOverlays(t *testing.T) {
	random := rand.New(rand.NewPCG(3, 4))
	slice := newColdSlice[coldTestRow](coldTestCodec{})
	var want []coldTestRow
	for i := range 3*coldBlockRows() + 17 {
		row := randomColdRow(random, i)
		slice.append(row)
		want = append(want, row)
	}
	check := func(stage string) {
		t.Helper()
		if slice.len() != len(want) {
			t.Fatalf("%s: len = %d, want %d", stage, slice.len(), len(want))
		}
		for i := range want {
			if got := slice.get(i); got != want[i] {
				t.Fatalf("%s: get(%d) = %+v, want %+v", stage, i, got, want[i])
			}
		}
		if got := slice.get(len(want)); got != (coldTestRow{}) {
			t.Fatalf("%s: get beyond end = %+v, want zero", stage, got)
		}
	}
	check("hot")

	freezeAt := 2*coldBlockRows() + 5
	slice.freezeBefore(freezeAt)
	if want := freezeAt / coldBlockRows() * coldBlockRows(); slice.frozenLen() != want {
		t.Fatalf("frozenLen = %d, want %d", slice.frozenLen(), want)
	}
	check("partly frozen")

	// A change to a frozen row goes to the overlay; a hot change stays hot.
	for _, i := range []int{0, coldBlockRows() + 3, 2*coldBlockRows() + 1} {
		want[i] = randomColdRow(random, i)
		slice.set(i, want[i])
	}
	check("changed")

	// Setting beyond the end extends the slice with zero rows.
	grown := len(want) + 5
	row := randomColdRow(random, grown)
	slice.set(grown, row)
	for len(want) < grown {
		want = append(want, coldTestRow{})
	}
	want = append(want, row)
	check("grown")

	slice.freezeBefore(slice.len() + coldBlockRows())
	if slice.frozenLen() != slice.len()/coldBlockRows()*coldBlockRows() {
		t.Fatalf("frozenLen after full freeze = %d, len = %d", slice.frozenLen(), slice.len())
	}
	check("fully frozen")
	if slice.estimatedBytes() <= 0 {
		t.Fatal("estimate is not positive")
	}
}

// TestColdStorageTestModeFreezesTimelineRows proves that the cold test mode
// freezes rows of an ordinary projection.
func TestColdStorageTestModeFreezesTimelineRows(t *testing.T) {
	if os.Getenv("CHATTO_TEST_COLD_STORAGE") != "1" {
		t.Skip("set CHATTO_TEST_COLD_STORAGE=1")
	}
	p := NewRoomTimelineProjection()
	for i := range 10 {
		applyAll(t, p, nil)
		if err := p.Apply(postedEvent(postedOpts{envelopeID: fmt.Sprintf("M%d", i), roomID: "R1", actorID: "U1", at: i}), uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if p.entries.frozenLen() == 0 || p.bodyStates.frozenLen() == 0 || p.rowByEvent.frozenLen() == 0 {
		t.Fatalf("frozen rows=%d bodies=%d index=%d, want all above zero", p.entries.frozenLen(), p.bodyStates.frozenLen(), p.rowByEvent.frozenLen())
	}
	for i := range 10 {
		if entry, ok := p.Get(fmt.Sprintf("M%d", i)); !ok || !entry.CreatedAt.Equal(fixedTime(i)) {
			t.Fatalf("Get(M%d) = %+v, %v", i, entry, ok)
		}
	}
}

// TestColdStorageTestModeFreezesHandleIndexedState proves that the cold test
// mode freezes the handle-indexed state of Threads, Reactions, and the Badge
// index, and that reads of frozen messages keep their results.
func TestColdStorageTestModeFreezesHandleIndexedState(t *testing.T) {
	if os.Getenv("CHATTO_TEST_COLD_STORAGE") != "1" {
		t.Skip("set CHATTO_TEST_COLD_STORAGE=1")
	}
	threads := NewThreadProjection()
	reactions := NewReactionProjection()
	badges := newBadgeTestFixture(t)
	if err := threads.Apply(roomCreatedEvent("R1", "general", "", evtv1.RoomKind_ROOM_KIND_CHANNEL), 100); err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		id := fmt.Sprintf("M%d", i)
		if err := threads.Apply(postedEvent(postedOpts{envelopeID: id, roomID: "R1", actorID: "U1", at: i}), uint64(i+1)); err != nil {
			t.Fatal(err)
		}
		applyReactionProjectionEvent(t, reactions, messagePostedProjectionEvent(id, ""))
		applyReactionProjectionEvent(t, reactions, reactionAddedProjectionEvent("A"+id, id, "U1", "heart", i))
		badges.post(id, "U2", "")
	}
	if threads.messageRefs.rows.frozenLen() == 0 {
		t.Fatal("Threads froze no message references")
	}
	if reactions.messageRooms.rows.frozenLen() == 0 {
		t.Fatal("Reactions froze no message rooms")
	}
	if badges.p.badges.messages.rows.frozenLen() == 0 {
		t.Fatal("the Badge index froze no message records")
	}
	for i := range 10 {
		id := fmt.Sprintf("M%d", i)
		if root, ok := threads.ThreadRootForMessage("R1", id); !ok || root != id {
			t.Fatalf("ThreadRootForMessage(%s) = %q, %v", id, root, ok)
		}
		if got := reactions.Reactions(id); len(got) != 1 || got[0].Emoji != "heart" {
			t.Fatalf("Reactions(%s) = %+v", id, got)
		}
	}
	if !badges.unread("U1", "") {
		t.Fatal("frozen root messages gave no Badge attention")
	}
}
