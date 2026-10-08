package intern

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

const (
	// idArenaFirstChunkBytes keeps small and test projections small.
	idArenaFirstChunkBytes = 1 << 10
	// idArenaMaxChunkBytes bounds chunk growth so an offset fits in 16 bits.
	idArenaMaxChunkBytes = 1 << 16
	// idArenaMaxIDBytes is the longest ID that fits in a Location length.
	// Longer IDs cannot be represented by a packed location.
	idArenaMaxIDBytes = 1<<24 - 1
	idArenaMaxChunks  = 1 << 24
)

// Arena stores immutable ID bytes in append-only chunks without deduplication.
// The zero value is ready for use. Do not copy an arena after first use.
// Locations belong to one arena and must not be persisted. Bytes are retained
// until the arena and all returned strings are released.
//
// Add requires a single writer or an external lock. String can run concurrently
// with Add for a location published through caller-supplied synchronization.
// EstimatedBytes is safe to call concurrently but is only an estimate.
//
// Each chunk is allocated at its full length and never reallocated. Its index
// stays fixed. Add writes only bytes that no location names yet, so a returned
// string remains valid while the arena grows.
type Arena struct {
	// chunks is the published chunk directory. Add replaces it instead of
	// appending in place.
	chunks atomic.Pointer[[][]byte]
	// target is the one-based index of the chunk that receives short IDs, or
	// zero before the first short ID.
	target int
	// used is the number of written bytes in the target chunk.
	used int
}

// Location is an opaque, pointer-free position in an Arena. Zero names the
// empty string. A nonzero location must come from the same arena's Add method.
// Applications must not construct locations or persist their numeric values.
// Internally, it packs chunk, offset, and length into 24, 16, and 24 bits.
type Location uint64

func newIDLocation(chunk, offset, length int) Location {
	return Location(uint64(chunk)<<40 | uint64(offset)<<24 | uint64(length))
}

func (l Location) chunk() int  { return int(l >> 40) }
func (l Location) offset() int { return int(l>>24) & 0xffff }

// Len returns the number of bytes in the stored ID.
func (l Location) Len() int { return int(l & 0xffffff) }

// Add copies id into the arena and returns its location. Empty IDs return zero.
// It panics if id exceeds 16 MiB minus one byte or the arena exhausts its
// 24-bit chunk index.
func (a *Arena) Add(id string) Location {
	if id == "" {
		return 0
	}
	if len(id) > idArenaMaxIDBytes {
		panic(fmt.Sprintf("intern: ID of %d bytes exceeds the %d-byte arena limit", len(id), idArenaMaxIDBytes))
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
func (a *Arena) appendChunk(size int, id string) Location {
	chunks := a.loadChunks()
	if len(chunks) >= idArenaMaxChunks {
		panic("intern: ID arena chunk limit exceeded")
	}
	chunk := make([]byte, size)
	copy(chunk, id)
	next := make([][]byte, len(chunks)+1)
	copy(next, chunks)
	next[len(chunks)] = chunk
	a.chunks.Store(&next)
	return newIDLocation(len(chunks), 0, len(id))
}

func (a *Arena) loadChunks() [][]byte {
	if chunks := a.chunks.Load(); chunks != nil {
		return *chunks
	}
	return nil
}

// String returns the ID at location without copying it. The returned string
// aliases arena bytes that are never written again.
func (a *Arena) String(location Location) string {
	length := location.Len()
	if length == 0 {
		return ""
	}
	chunk := a.loadChunks()[location.chunk()]
	return unsafe.String(&chunk[location.offset()], length)
}

// EstimatedBytes approximates the arena's heap footprint.
func (a *Arena) EstimatedBytes() int64 {
	chunks := a.loadChunks()
	bytes := int64(cap(chunks)) * int64(unsafe.Sizeof([]byte(nil)))
	for _, chunk := range chunks {
		bytes += int64(cap(chunk))
	}
	return bytes
}
