package intern

import (
	"hash/maphash"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ConcurrentTable interns opaque IDs as dense, one-based uint32 handles.
// Kind distinguishes this namespace from other ID kinds at compile time.
// It is safe for concurrent use. Construct it with NewConcurrentTable;
// the zero value is not ready for use. Do not copy a table.
//
// Handles belong to one table and remain valid for its lifetime. Zero means
// no ID. The table is append-only and never reuses handles. Store ID strings,
// not handles, in snapshots, events, or API responses.
//
// The table owns no projection state or replay position. Applications decide
// which models share it. Interning an ID does not mean that any model has
// processed the corresponding record.
//
// Each hash shard has its own lock. Intern also locks handle assignment,
// after the shard lock. Resolve does not lock: callers must obtain a handle from
// Intern or Lookup and synchronize its publication to other goroutines.
// Location entries and their arena bytes never change after publication.
// No method calls application code while it holds a lock.
//
// The primary hash entries and packed locations contain no Go pointers.
// Returned strings alias immutable arena bytes and remain valid after the
// table is released. All interned bytes are retained for the table's lifetime.
type ConcurrentTable[Kind any] struct {
	seed   maphash.Seed
	shards [shardCount]indexShard
	// mu serializes handle assignment: count, pages, and arena writes.
	mu sync.Mutex
	// count is the number of interned IDs.
	count int
	// pages is the published directory of fixed-size location pages indexed
	// by (handle-1)/locationPageSize. A page never moves after publication.
	pages atomic.Pointer[[]*locationPage]
	arena Arena
}

// shardCount is the number of independently locked index shards. It
// must be a power of two.
const shardCount = 64

// indexShard indexes the IDs whose hash selects it. Padding keeps each
// shard's lock on its own cache lines, so readers of different shards do not
// contend.
type indexShard struct {
	mu sync.RWMutex
	// byHash maps an ID's 64-bit hash to the handle of the first interned ID
	// with that hash.
	byHash map[uint64]uint32
	// collisions holds the rare IDs whose hash already names a different ID.
	collisions map[string]uint32
	_          [128 - (unsafe.Sizeof(sync.RWMutex{})+2*unsafe.Sizeof(map[uint64]uint32(nil)))%128]byte
}

// locationPageSize is the number of handle locations in one page.
const locationPageSize = 1 << 10

type locationPage [locationPageSize]Location

// NewConcurrentTable creates an empty table that can be shared by independent models.
func NewConcurrentTable[Kind any]() *ConcurrentTable[Kind] {
	return &ConcurrentTable[Kind]{seed: maphash.MakeSeed()}
}

// shard returns the index shard of an ID hash. The high bits select the shard
// because the map inside the shard uses the low bits.
func (t *ConcurrentTable[Kind]) shard(hash uint64) *indexShard {
	return &t.shards[hash>>58&(shardCount-1)]
}

// Intern returns the handle for id and adds id when it is new. An empty id
// returns handle zero.
// It panics on handle exhaustion or when an Arena storage limit is exceeded.
func (t *ConcurrentTable[Kind]) Intern(id string) ID[Kind] {
	if id == "" {
		return 0
	}
	hash := maphash.String(t.seed, id)
	shard := t.shard(hash)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if handle, ok := t.lookupLocked(shard, hash, id); ok {
		return ID[Kind](handle)
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
	return ID[Kind](handle)
}

// appendID stores id and assigns it the next handle.
func (t *ConcurrentTable[Kind]) appendID(id string) uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if uint64(t.count) >= uint64(^uint32(0)) {
		panic("intern: ID table handle space exhausted")
	}
	pageIndex, slot := t.count/locationPageSize, t.count%locationPageSize
	pages := t.loadPages()
	if pageIndex == len(pages) {
		// Publish a new directory instead of appending in place, so a reader
		// never observes a directory that is being modified.
		next := make([]*locationPage, len(pages)+1)
		copy(next, pages)
		next[pageIndex] = new(locationPage)
		t.pages.Store(&next)
		pages = next
	}
	pages[pageIndex][slot] = t.arena.Add(id)
	t.count++
	return uint32(t.count)
}

// Lookup returns the handle for an interned id without adding it. Read paths
// use Lookup so a query for an unknown ID does not grow the table.
func (t *ConcurrentTable[Kind]) Lookup(id string) (ID[Kind], bool) {
	if id == "" {
		return 0, false
	}
	hash := maphash.String(t.seed, id)
	shard := t.shard(hash)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	handle, found := t.lookupLocked(shard, hash, id)
	return ID[Kind](handle), found
}

func (t *ConcurrentTable[Kind]) lookupLocked(shard *indexShard, hash uint64, id string) (uint32, bool) {
	if handle, ok := shard.byHash[hash]; ok && t.Resolve(ID[Kind](handle)) == id {
		return handle, true
	}
	handle, ok := shard.collisions[id]
	return handle, ok
}

// Resolve returns the ID for handle, or an empty string for handle zero. handle
// must come from this table.
func (t *ConcurrentTable[Kind]) Resolve(handle ID[Kind]) string {
	if handle == 0 {
		return ""
	}
	index := int(handle - 1)
	return t.arena.String(t.loadPages()[index/locationPageSize][index%locationPageSize])
}

func (t *ConcurrentTable[Kind]) loadPages() []*locationPage {
	if pages := t.pages.Load(); pages != nil {
		return *pages
	}
	return nil
}

// Len returns the number of interned IDs.
func (t *ConcurrentTable[Kind]) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

// EstimatedBytes approximates retained memory, including spare arena capacity.
// It is a diagnostic estimate, not an allocator measurement or an atomic snapshot.
func (t *ConcurrentTable[Kind]) EstimatedBytes() int64 {
	bytes := int64(unsafe.Sizeof(*t))
	for i := range t.shards {
		shard := &t.shards[i]
		shard.mu.RLock()
		bytes += int64(len(shard.byHash)) * (compactMapEntryOverhead + 12)
		for id := range shard.collisions {
			bytes += mapEntryOverhead + int64(len(id))
		}
		shard.mu.RUnlock()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	pages := int64(len(t.loadPages()))
	return bytes + t.arena.EstimatedBytes() + pages*int64(unsafe.Sizeof(locationPage{})+unsafe.Sizeof((*locationPage)(nil)))
}
