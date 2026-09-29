package core

import (
	"fmt"
	"hash/maphash"
	"sync"
	"sync/atomic"
	"unsafe"
)

// eventIDTable interns event IDs as dense, one-based uint32 handles. Handle
// zero means "no ID".
//
// ServerContentView components share one table, so an event ID that the room
// timeline, thread, and reaction components all index is held once. Each
// component keeps handle-indexed state, such as handleSlice values, in its own
// model. A projection created outside the content view owns a private table.
//
// The table is append-only: handles stay valid for the table's lifetime and
// are never reused. Handles are process-local and must not be persisted;
// snapshots store the ID strings. A restore or cold replay of one component
// interns IDs again and receives the existing handles.
//
// Components that share the table apply and read under different component
// locks, so the table synchronizes itself. intern and lookup use the table
// lock. id takes no lock: a caller can hold a handle only after intern or
// lookup returned it, and the handle's location and bytes are written before
// that return and never change. The table never calls other code while it
// holds its lock, so callers can use it while they hold a component lock.
//
// The index and ID storage contain no Go pointers, so the garbage collector
// does not scan them. Strings returned by id alias immutable arena bytes; they
// need no allocation and stay valid after the table is released.
type eventIDTable struct {
	mu   sync.RWMutex
	seed maphash.Seed
	// byHash maps an ID's 64-bit hash to the handle of the first interned ID
	// with that hash.
	byHash map[uint64]uint32
	// collisions holds the rare IDs whose hash already names a different ID.
	collisions map[string]uint32
	// count is the number of interned IDs.
	count int
	// pages is the published directory of fixed-size location pages indexed
	// by (handle-1)/idLocationPageSize. A page never moves after publication.
	pages atomic.Pointer[[]*idLocationPage]
	arena idArena
}

// idLocationPageSize is the number of handle locations in one page.
const idLocationPageSize = 1 << 10

type idLocationPage [idLocationPageSize]idLocation

func newEventIDTable() *eventIDTable {
	return &eventIDTable{seed: maphash.MakeSeed(), byHash: make(map[uint64]uint32)}
}

// intern returns the handle for id and adds id when it is new. An empty id
// returns handle zero.
func (t *eventIDTable) intern(id string) uint32 {
	if id == "" {
		return 0
	}
	hash := maphash.String(t.seed, id)
	t.mu.Lock()
	defer t.mu.Unlock()
	if handle, ok := t.lookupLocked(hash, id); ok {
		return handle
	}
	if t.count == int(^uint32(0)) {
		panic("core: event ID table handle space exhausted")
	}
	pageIndex, slot := t.count/idLocationPageSize, t.count%idLocationPageSize
	pages := t.loadPages()
	if pageIndex == len(pages) {
		// Publish a new directory instead of appending in place, so a reader
		// never observes a directory that is being modified.
		next := make([]*idLocationPage, len(pages)+1)
		copy(next, pages)
		next[pageIndex] = new(idLocationPage)
		t.pages.Store(&next)
		pages = next
	}
	pages[pageIndex][slot] = t.arena.add(id)
	t.count++
	handle := uint32(t.count)
	if _, taken := t.byHash[hash]; taken {
		if t.collisions == nil {
			t.collisions = make(map[string]uint32)
		}
		t.collisions[id] = handle
	} else {
		t.byHash[hash] = handle
	}
	return handle
}

// lookup returns the handle for an interned id without adding it. Read paths
// use lookup so a query for an unknown ID does not grow the table.
func (t *eventIDTable) lookup(id string) (uint32, bool) {
	if id == "" {
		return 0, false
	}
	hash := maphash.String(t.seed, id)
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lookupLocked(hash, id)
}

func (t *eventIDTable) lookupLocked(hash uint64, id string) (uint32, bool) {
	if handle, ok := t.byHash[hash]; ok && t.id(handle) == id {
		return handle, true
	}
	handle, ok := t.collisions[id]
	return handle, ok
}

// id returns the ID for handle, or an empty string for handle zero. handle
// must come from this table.
func (t *eventIDTable) id(handle uint32) string {
	if handle == 0 {
		return ""
	}
	index := int(handle - 1)
	return t.arena.string(t.loadPages()[index/idLocationPageSize][index%idLocationPageSize])
}

func (t *eventIDTable) loadPages() []*idLocationPage {
	if pages := t.pages.Load(); pages != nil {
		return *pages
	}
	return nil
}

// len returns the number of interned IDs.
func (t *eventIDTable) len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// estimatedBytes approximates the retained size of the table.
func (t *eventIDTable) estimatedBytes() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pages := int64(len(t.loadPages()))
	bytes := t.arena.retainedBytes() + pages*int64(unsafe.Sizeof(idLocationPage{})+unsafe.Sizeof((*idLocationPage)(nil)))
	bytes += int64(len(t.byHash)) * (projectionCompactMapEntryOverhead + 12)
	for id := range t.collisions {
		bytes += projectionMapEntryOverhead + int64(len(id))
	}
	return bytes
}

const (
	// idArenaFirstChunkBytes keeps small and test projections small.
	idArenaFirstChunkBytes = 1 << 10
	// idArenaMaxChunkBytes bounds chunk growth so an offset fits in 16 bits.
	idArenaMaxChunkBytes = 1 << 16
	// idArenaMaxIDBytes is the longest ID that fits in an idLocation length.
	// EVT record limits keep real IDs far below it.
	idArenaMaxIDBytes = 1<<24 - 1
	idArenaMaxChunks  = 1 << 24
)

// idArena stores immutable ID bytes in append-only chunks. Each chunk is
// allocated at its full length and never reallocated, a chunk keeps its index
// in the directory, and add writes only bytes that no location names yet. A
// string that aliases chunk bytes therefore stays valid, and string can read
// published locations while add runs. The owner serializes add calls.
type idArena struct {
	// chunks is the published chunk directory. add replaces it instead of
	// appending in place.
	chunks atomic.Pointer[[][]byte]
	// target is the one-based index of the chunk that receives short IDs, or
	// zero before the first short ID.
	target int
	// used is the number of written bytes in the target chunk.
	used int
}

// idLocation packs an arena position into one pointer-free word: the chunk
// index in the high 24 bits, the byte offset in the next 16 bits, and the
// length in the low 24 bits. The zero location is the empty string.
type idLocation uint64

func newIDLocation(chunk, offset, length int) idLocation {
	return idLocation(uint64(chunk)<<40 | uint64(offset)<<24 | uint64(length))
}

func (l idLocation) chunk() int  { return int(l >> 40) }
func (l idLocation) offset() int { return int(l>>24) & 0xffff }
func (l idLocation) length() int { return int(l & 0xffffff) }

// add copies id into the arena and returns its location.
func (a *idArena) add(id string) idLocation {
	if id == "" {
		return 0
	}
	if len(id) > idArenaMaxIDBytes {
		panic(fmt.Sprintf("core: ID of %d bytes exceeds the %d-byte arena limit", len(id), idArenaMaxIDBytes))
	}
	if len(id) > idArenaMaxChunkBytes/4 {
		// A long ID gets its own exact chunk instead of wasting the free tail
		// of the target chunk.
		return a.appendChunk(len(id), id)
	}
	size := idArenaFirstChunkBytes
	if a.target != 0 {
		chunk := a.loadChunks()[a.target-1]
		if len(chunk)-a.used >= len(id) {
			offset := a.used
			copy(chunk[offset:], id)
			a.used += len(id)
			return newIDLocation(a.target-1, offset, len(id))
		}
		size = min(len(chunk)*2, idArenaMaxChunkBytes)
	}
	location := a.appendChunk(max(size, len(id)), id)
	a.target = location.chunk() + 1
	a.used = len(id)
	return location
}

// appendChunk publishes a new chunk of size bytes that starts with id.
func (a *idArena) appendChunk(size int, id string) idLocation {
	chunks := a.loadChunks()
	if len(chunks) >= idArenaMaxChunks {
		panic("core: ID arena chunk limit exceeded")
	}
	chunk := make([]byte, size)
	copy(chunk, id)
	next := make([][]byte, len(chunks)+1)
	copy(next, chunks)
	next[len(chunks)] = chunk
	a.chunks.Store(&next)
	return newIDLocation(len(chunks), 0, len(id))
}

func (a *idArena) loadChunks() [][]byte {
	if chunks := a.chunks.Load(); chunks != nil {
		return *chunks
	}
	return nil
}

// string returns the ID at location without copying it. The returned string
// aliases arena bytes that are never written again.
func (a *idArena) string(location idLocation) string {
	length := location.length()
	if length == 0 {
		return ""
	}
	chunk := a.loadChunks()[location.chunk()]
	return unsafe.String(&chunk[location.offset()], length)
}

// retainedBytes approximates the arena's heap footprint.
func (a *idArena) retainedBytes() int64 {
	chunks := a.loadChunks()
	bytes := int64(cap(chunks)) * int64(unsafe.Sizeof([]byte(nil)))
	for _, chunk := range chunks {
		bytes += int64(cap(chunk))
	}
	return bytes
}
