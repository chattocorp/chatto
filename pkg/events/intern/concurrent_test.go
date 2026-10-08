package intern

import (
	"fmt"
	"hash/maphash"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type testKind struct{}

func TestConcurrentTableRejectsHandleOverflow(t *testing.T) {
	t.Parallel()
	table := NewConcurrentTable[testKind]()
	handle := table.Intern("existing")
	// Simulate exhaustion without allocating billions of entries.
	maxHandles := uint64(^uint32(0))
	if uint64(^uint(0)>>1) < maxHandles {
		t.Skip("the platform's int cannot represent the uint32 handle limit")
	}
	table.count = int(maxHandles)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("handle overflow did not panic")
			}
		}()
		table.Intern("new")
	}()
	if _, ok := table.Lookup("new"); ok {
		t.Fatal("failed insertion published a handle")
	}
	if table.Intern("existing") != handle || table.Resolve(handle) != "existing" {
		t.Fatal("handle exhaustion invalidated an existing ID")
	}
}

func TestConcurrentTableRejectsOversizedID(t *testing.T) {
	t.Parallel()
	table := NewConcurrentTable[testKind]()
	handle := table.Intern("existing")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("oversized ID did not panic")
			}
		}()
		table.Intern(strings.Repeat("x", idArenaMaxIDBytes+1))
	}()
	if table.Len() != 1 || table.Resolve(handle) != "existing" {
		t.Fatal("failed insertion changed the table")
	}
	if table.Intern("next") != 2 {
		t.Fatal("failed insertion consumed a handle")
	}
}

func TestConcurrentTable_InternsDenseHandles(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
	if got := table.Intern(""); got != 0 {
		t.Fatalf("intern empty = %d, want 0", got)
	}
	first := table.Intern("E1")
	second := table.Intern("E2")
	if first != 1 || second != 2 {
		t.Fatalf("handles = %d, %d; want 1, 2", first, second)
	}
	if again := table.Intern("E1"); again != first {
		t.Fatalf("repeated intern = %d, want %d", again, first)
	}
	if handle, ok := table.Lookup("E2"); !ok || handle != second {
		t.Fatalf("lookup E2 = %d, %v; want %d, true", handle, ok, second)
	}
	if _, ok := table.Lookup("E3"); ok {
		t.Fatal("lookup of unknown ID succeeded")
	}
	if got := table.Len(); got != 2 {
		t.Fatalf("len after lookup of unknown ID = %d, want 2", got)
	}
	if got := table.Resolve(second); got != "E2" {
		t.Fatalf("id(%d) = %q, want E2", second, got)
	}
	if got := table.Resolve(0); got != "" {
		t.Fatalf("id(0) = %q, want empty", got)
	}
}

func TestConcurrentTable_ResolvesHashCollisions(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
	first := table.Intern("E1")
	// Point the hash of E2 at E1 to simulate a 64-bit hash collision.
	hash := maphash.String(table.seed, "E2")
	shard := table.shard(hash)
	if shard.byHash == nil {
		shard.byHash = make(map[uint64]uint32)
	}
	shard.byHash[hash] = uint32(first)

	second := table.Intern("E2")
	if second == first {
		t.Fatal("colliding ID received the handle of a different ID")
	}
	if handle, ok := table.Lookup("E2"); !ok || handle != second {
		t.Fatalf("lookup E2 = %d, %v; want %d, true", handle, ok, second)
	}
	if handle, ok := table.Lookup("E1"); !ok || handle != first {
		t.Fatalf("lookup E1 = %d, %v; want %d, true", handle, ok, first)
	}
	if again := table.Intern("E2"); again != second {
		t.Fatalf("repeated colliding intern = %d, want %d", again, second)
	}
	if table.EstimatedBytes() <= 0 {
		t.Fatal("estimate does not include the table")
	}
}

func TestConcurrentTable_KeepsIDsAcrossArenaChunks(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
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
	handles := make([]ID[testKind], len(ids))
	for i, id := range ids {
		handles[i] = table.Intern(id)
	}
	chunks := table.arena.loadChunks()
	if len(chunks) < 3 {
		t.Fatalf("arena used %d chunks, want several", len(chunks))
	}
	for i, id := range ids {
		if got := table.Resolve(handles[i]); got != id {
			t.Fatalf("id(%d) = %q, want %q", handles[i], got, id)
		}
		if handle, ok := table.Lookup(id); !ok || handle != handles[i] {
			t.Fatalf("lookup(%q) = %d, %v; want %d, true", id, handle, ok, handles[i])
		}
	}
	for _, chunk := range chunks {
		if len(chunk) > idArenaMaxChunkBytes && len(chunk) != len(long) && len(chunk) != len(long)+1 {
			t.Fatalf("chunk of %d bytes exceeds the shared chunk limit", len(chunk))
		}
	}
}

func TestConcurrentTable_ConcurrentInternAndRead(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
	var wg sync.WaitGroup
	for writer := range 4 {
		wg.Go(func() {
			for i := range 1_000 {
				id := fmt.Sprintf("E%d-%d", writer, i)
				handle := table.Intern(id)
				if got := table.Resolve(handle); got != id {
					t.Errorf("id(%d) = %q, want %q", handle, got, id)
					return
				}
			}
		})
	}
	wg.Go(func() {
		for i := range 4_000 {
			table.Lookup(fmt.Sprintf("E0-%d", i%1_000))
			table.Len()
		}
	})
	wg.Wait()
	if got := table.Len(); got != 4_000 {
		t.Fatalf("len = %d, want 4000", got)
	}
}

func TestConcurrentTable_ConcurrentInternOfSameIDsAgreesOnHandles(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
	const workers, ids = 8, 2_000
	results := make([][]ID[testKind], workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			handles := make([]ID[testKind], ids)
			for i := range ids {
				// Workers visit every ID, starting at different offsets.
				n := (i + worker*ids/workers) % ids
				handles[n] = table.Intern(fmt.Sprintf("E%05d", n))
			}
			results[worker] = handles
		})
	}
	wg.Wait()
	if got := table.Len(); got != ids {
		t.Fatalf("len = %d, want %d", got, ids)
	}
	for worker := 1; worker < workers; worker++ {
		if !slices.Equal(results[worker], results[0]) {
			t.Fatalf("worker %d received different handles than worker 0", worker)
		}
	}
	for n, handle := range results[0] {
		if got := table.Resolve(handle); got != fmt.Sprintf("E%05d", n) {
			t.Fatalf("id(%d) = %q, want E%05d", handle, got, n)
		}
	}
}

// TestConcurrentTable_ReadsPublishedHandlesDuringGrowth reads handles that other
// goroutines published while writers add location pages and arena chunks. It
// models components that apply and read under different locks.
func TestConcurrentTable_ReadsPublishedHandlesDuringGrowth(t *testing.T) {
	t.Parallel()

	table := NewConcurrentTable[testKind]()
	const writers, perWriter = 4, 3 * locationPageSize
	var published sync.Map
	var writing sync.WaitGroup
	var done atomic.Bool
	for writer := range writers {
		writing.Go(func() {
			for i := range perWriter {
				id := fmt.Sprintf("W%d-%06d", writer, i)
				published.Store(id, table.Intern(id))
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
					id, handle := key.(string), value.(ID[testKind])
					if got := table.Resolve(handle); got != id {
						t.Errorf("id(%d) = %q, want %q", handle, got, id)
						return false
					}
					if got, ok := table.Lookup(id); !ok || got != handle {
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
	if got := table.Len(); got != writers*perWriter {
		t.Fatalf("len = %d, want %d", got, writers*perWriter)
	}
	if pages := len(table.loadPages()); pages < writers*perWriter/locationPageSize {
		t.Fatalf("table used %d location pages, want at least %d", pages, writers*perWriter/locationPageSize)
	}
}
