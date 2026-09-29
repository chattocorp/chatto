package core

import (
	"cmp"
	"fmt"
	"hash/maphash"
	"maps"
	"slices"
	"sort"
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
// locks, so the table synchronizes itself. The hash index is split into
// eventIDShardCount shards with separate locks, so concurrent lookups of
// different IDs rarely touch the same lock. intern also takes the append lock
// to assign a handle, always after its shard lock. id takes no lock: a caller
// can hold a handle only after intern or lookup returned it, and the handle's
// location and bytes are written before that return and never change. The
// table never calls other code while it holds a lock, so callers can use it
// while they hold a component lock.
//
// The index and ID storage contain no Go pointers, so the garbage collector
// does not scan them. Strings returned by id alias immutable arena or cold
// page bytes; they need no allocation and stay valid after the table is
// released.
//
// freezeBelow moves the IDs of old, complete pages into compact cold pages
// with a sorted hash index (ADR-111). Handles stay valid, and every method
// returns the same results for frozen IDs.
type eventIDTable struct {
	seed   maphash.Seed
	shards [eventIDShardCount]eventIDShard
	// mu serializes handle assignment and freezing: count, pages, arena
	// writes, and chunkLastHandle.
	mu sync.Mutex
	// count is the number of interned IDs.
	count int
	// pages is the published directory of location pages indexed by
	// (handle-1)/idLocationPageSize. A page never changes after publication.
	// Frozen pages are nil; their IDs are in cold.
	pages atomic.Pointer[[]idLocationPage]
	arena idArena
	// chunkLastHandle holds the highest handle whose ID bytes are in each
	// arena chunk, so a freeze can release chunks that hold only frozen IDs.
	chunkLastHandle []uint32
	// cold is the published frozen part of the table.
	cold atomic.Pointer[eventIDColdState]
}

// eventIDColdState is the immutable frozen part of an event ID table.
type eventIDColdState struct {
	// pages holds the frozen pages, starting at page zero.
	pages []*eventIDColdPage
	// hashes and handles form the index of frozen IDs, sorted by hash.
	hashes  []uint64
	handles []uint32
}

// eventIDColdPage holds the IDs of one frozen page.
type eventIDColdPage struct {
	// data holds the ID bytes of the page in handle order.
	data []byte
	// ends holds the end offset of each ID in data, in one packed column.
	ends packedColumns
}

// id returns the ID in slot of the page without copying it.
func (p *eventIDColdPage) id(slot int) string {
	start := 0
	if slot > 0 {
		start = int(p.ends.get(slot-1, 0))
	}
	end := int(p.ends.get(slot, 0))
	if end == start {
		return ""
	}
	return unsafe.String(&p.data[start], end-start)
}

// eventIDShardCount is the number of independently locked index shards. It
// must be a power of two.
const eventIDShardCount = 64

// eventIDShard indexes the hot IDs whose hash selects it. Padding keeps each
// shard's lock on its own cache lines, so readers of different shards do not
// contend.
type eventIDShard struct {
	mu sync.RWMutex
	// byHash maps an ID's 64-bit hash to the handle of the first interned ID
	// with that hash.
	byHash map[uint64]uint32
	// collisions holds the rare IDs whose hash already names a different ID.
	collisions map[string]uint32
	// removed counts entries deleted from byHash since it was rebuilt. Go maps
	// do not shrink, so a freeze rebuilds the map when this exceeds its size.
	removed int
	_       [128 - (unsafe.Sizeof(sync.RWMutex{})+2*unsafe.Sizeof(map[uint64]uint32(nil))+unsafe.Sizeof(0))%128]byte
}

// idLocationPageSize is the number of handle locations in one page. Tests
// lower it.
var idLocationPageSize = 1 << 10

// idLocationPage holds the arena locations of one page of handles.
type idLocationPage []idLocation

func newEventIDTable() *eventIDTable {
	return &eventIDTable{seed: maphash.MakeSeed()}
}

// shard returns the index shard of an ID hash. The high bits select the shard
// because the map inside the shard uses the low bits.
func (t *eventIDTable) shard(hash uint64) *eventIDShard {
	return &t.shards[hash>>58&(eventIDShardCount-1)]
}

// intern returns the handle for id and adds id when it is new. An empty id
// returns handle zero.
func (t *eventIDTable) intern(id string) uint32 {
	if id == "" {
		return 0
	}
	hash := maphash.String(t.seed, id)
	shard := t.shard(hash)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if handle, ok := t.lookupLocked(shard, hash, id); ok {
		return handle
	}
	handle := t.appendID(id)
	if _, taken := shard.byHash[hash]; taken {
		if shard.collisions == nil {
			shard.collisions = make(map[string]uint32)
		}
		shard.collisions[id] = handle
	} else {
		if shard.byHash == nil {
			shard.byHash = make(map[uint64]uint32)
		}
		shard.byHash[hash] = handle
	}
	return handle
}

// appendID stores id and assigns it the next handle.
func (t *eventIDTable) appendID(id string) uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.count == int(^uint32(0)) {
		panic("core: event ID table handle space exhausted")
	}
	pageIndex, slot := t.count/idLocationPageSize, t.count%idLocationPageSize
	pages := t.loadPages()
	if pageIndex == len(pages) {
		// Publish a new directory instead of appending in place, so a reader
		// never observes a directory that is being modified.
		next := make([]idLocationPage, len(pages)+1)
		copy(next, pages)
		next[pageIndex] = make(idLocationPage, idLocationPageSize)
		t.pages.Store(&next)
		pages = next
	}
	location := t.arena.add(id)
	pages[pageIndex][slot] = location
	t.count++
	handle := uint32(t.count)
	for len(t.chunkLastHandle) <= location.chunk() {
		t.chunkLastHandle = append(t.chunkLastHandle, 0)
	}
	t.chunkLastHandle[location.chunk()] = handle
	return handle
}

// lookup returns the handle for an interned id without adding it. Read paths
// use lookup so a query for an unknown ID does not grow the table.
func (t *eventIDTable) lookup(id string) (uint32, bool) {
	if id == "" {
		return 0, false
	}
	hash := maphash.String(t.seed, id)
	shard := t.shard(hash)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return t.lookupLocked(shard, hash, id)
}

func (t *eventIDTable) lookupLocked(shard *eventIDShard, hash uint64, id string) (uint32, bool) {
	if handle, ok := shard.byHash[hash]; ok && t.id(handle) == id {
		return handle, true
	}
	if handle, ok := shard.collisions[id]; ok {
		return handle, true
	}
	// A freeze swaps the cold index and the shard maps while it holds every
	// shard lock, so the caller's shard lock makes this read consistent.
	cold := t.cold.Load()
	if cold == nil {
		return 0, false
	}
	for i := sort.Search(len(cold.hashes), func(i int) bool { return cold.hashes[i] >= hash }); i < len(cold.hashes) && cold.hashes[i] == hash; i++ {
		if t.id(cold.handles[i]) == id {
			return cold.handles[i], true
		}
	}
	return 0, false
}

// id returns the ID for handle, or an empty string for handle zero. handle
// must come from this table.
func (t *eventIDTable) id(handle uint32) string {
	if handle == 0 {
		return ""
	}
	index := int(handle - 1)
	page, slot := index/idLocationPageSize, index%idLocationPageSize
	// Load the chunk directory before the page directory. A freeze removes a
	// page before it releases its chunks, so a hot page that this read sees
	// still has its chunks in the loaded chunk directory.
	chunks := t.arena.loadChunks()
	if locations := t.loadPages()[page]; locations != nil {
		return idArenaString(chunks, locations[slot])
	}
	// A freeze publishes a cold page before it removes the hot page.
	return t.cold.Load().pages[page].id(slot)
}

// freezeBelow moves the IDs of complete pages whose handles are all below
// boundary into cold pages, and releases arena chunks that then hold only
// frozen IDs. Handles and results do not change. Freezing is idempotent and
// never moves the boundary back.
func (t *eventIDTable) freezeBelow(boundary uint32) {
	if boundary <= 1 {
		return
	}
	for i := range t.shards {
		t.shards[i].mu.Lock()
	}
	t.mu.Lock()
	defer func() {
		t.mu.Unlock()
		for i := range t.shards {
			t.shards[i].mu.Unlock()
		}
	}()
	previous := t.cold.Load()
	if previous == nil {
		previous = &eventIDColdState{}
	}
	frozen := len(previous.pages)
	limit := min(int(boundary-1), t.count) / idLocationPageSize
	if limit <= frozen {
		return
	}

	pages := t.loadPages()
	chunks := t.arena.loadChunks()
	next := &eventIDColdState{pages: append(slices.Clip(previous.pages), make([]*eventIDColdPage, 0, limit-frozen)...)}
	type entry struct {
		hash   uint64
		handle uint32
	}
	added := make([]entry, 0, (limit-frozen)*idLocationPageSize)
	ends := make([]coldFields, idLocationPageSize)
	for page := frozen; page < limit; page++ {
		var data []byte
		for slot, location := range pages[page] {
			id := idArenaString(chunks, location)
			data = append(data, id...)
			ends[slot][0] = uint64(len(data))
			handle := uint32(page*idLocationPageSize + slot + 1)
			hash := maphash.String(t.seed, id)
			added = append(added, entry{hash: hash, handle: handle})
			// Remove the hot index entry; the cold index replaces it.
			shard := t.shard(hash)
			if shard.byHash[hash] == handle {
				delete(shard.byHash, hash)
				shard.removed++
			} else {
				delete(shard.collisions, id)
			}
		}
		next.pages = append(next.pages, &eventIDColdPage{data: slices.Clip(data), ends: packColumns(1, ends)})
	}
	slices.SortFunc(added, func(a, b entry) int { return cmp.Compare(a.hash, b.hash) })
	next.hashes = make([]uint64, 0, len(previous.hashes)+len(added))
	next.handles = make([]uint32, 0, len(previous.hashes)+len(added))
	for i, j := 0, 0; i < len(previous.hashes) || j < len(added); {
		if j == len(added) || (i < len(previous.hashes) && previous.hashes[i] <= added[j].hash) {
			next.hashes = append(next.hashes, previous.hashes[i])
			next.handles = append(next.handles, previous.handles[i])
			i++
		} else {
			next.hashes = append(next.hashes, added[j].hash)
			next.handles = append(next.handles, added[j].handle)
			j++
		}
	}
	t.cold.Store(next)

	for i := range t.shards {
		shard := &t.shards[i]
		if shard.removed > len(shard.byHash) {
			shard.byHash = maps.Clone(shard.byHash)
			shard.removed = 0
		}
	}
	hot := make([]idLocationPage, len(pages))
	copy(hot, pages)
	for page := frozen; page < limit; page++ {
		hot[page] = nil
	}
	t.pages.Store(&hot)

	// Arena chunks follow handle order, except that the chunk for short IDs
	// can outlive chunks for long IDs. Release chunks whose last handle is
	// frozen, but never the chunk that receives new short IDs.
	firstHot := uint32(limit*idLocationPageSize + 1)
	t.arena.release(func(chunk int) bool {
		return chunk < len(t.chunkLastHandle) && t.chunkLastHandle[chunk] < firstHot && chunk != t.arena.target-1
	})
}

func (t *eventIDTable) loadPages() []idLocationPage {
	if pages := t.pages.Load(); pages != nil {
		return *pages
	}
	return nil
}

// len returns the number of interned IDs.
func (t *eventIDTable) len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

// estimatedBytes approximates the retained size of the table.
func (t *eventIDTable) estimatedBytes() int64 {
	bytes := int64(unsafe.Sizeof(*t))
	for i := range t.shards {
		shard := &t.shards[i]
		shard.mu.RLock()
		bytes += int64(len(shard.byHash)) * (projectionCompactMapEntryOverhead + 12)
		for id := range shard.collisions {
			bytes += projectionMapEntryOverhead + int64(len(id))
		}
		shard.mu.RUnlock()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	pages := t.loadPages()
	bytes += t.arena.retainedBytes() + int64(cap(pages))*int64(unsafe.Sizeof(idLocationPage(nil))) + int64(cap(t.chunkLastHandle))*4
	for _, locations := range pages {
		bytes += int64(cap(locations)) * 8
	}
	if cold := t.cold.Load(); cold != nil {
		bytes += int64(cap(cold.hashes))*8 + int64(cap(cold.handles))*4 + int64(cap(cold.pages))*8
		for _, page := range cold.pages {
			bytes += int64(cap(page.data)) + page.ends.estimatedBytes()
		}
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
	return idArenaString(a.loadChunks(), location)
}

// idArenaString returns the ID at location in a loaded chunk directory.
func idArenaString(chunks [][]byte, location idLocation) string {
	length := location.length()
	if length == 0 {
		return ""
	}
	chunk := chunks[location.chunk()]
	return unsafe.String(&chunk[location.offset()], length)
}

// release publishes a chunk directory without the chunks that drop selects.
// Strings that alias a released chunk stay valid; the garbage collector keeps
// the chunk while they exist. The owner serializes release with add.
func (a *idArena) release(drop func(chunk int) bool) {
	chunks := a.loadChunks()
	var next [][]byte
	for chunk := range chunks {
		if chunks[chunk] == nil || !drop(chunk) {
			continue
		}
		if next == nil {
			next = slices.Clone(chunks)
		}
		next[chunk] = nil
	}
	if next != nil {
		a.chunks.Store(&next)
	}
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
