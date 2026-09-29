package core

import (
	"cmp"
	"slices"
)

// projectionIDTable interns opaque IDs as dense, one-based uint32 handles.
// Handle zero means "no ID". A projection stores handles in its indexes instead
// of repeating ID strings, so each ID is held once and index entries stay
// small and pointer-free.
//
// The table is append-only: handles stay valid for the table's lifetime and
// are never reused. Handles are process-local and must not be persisted;
// snapshots store the ID strings. The owning projection's lock guards the
// table.
type projectionIDTable struct {
	handles map[string]uint32
	ids     []string
}

func newProjectionIDTable() projectionIDTable {
	return projectionIDTable{handles: make(map[string]uint32)}
}

// intern returns the handle for id and adds id when it is new. An empty id
// returns handle zero.
func (t *projectionIDTable) intern(id string) uint32 {
	if id == "" {
		return 0
	}
	if handle, ok := t.handles[id]; ok {
		return handle
	}
	t.ids = append(t.ids, id)
	handle := uint32(len(t.ids))
	t.handles[id] = handle
	return handle
}

// lookup returns the handle for an interned id without adding it. Read paths
// use lookup so a query for an unknown ID does not grow the table.
func (t *projectionIDTable) lookup(id string) (uint32, bool) {
	if id == "" {
		return 0, false
	}
	handle, ok := t.handles[id]
	return handle, ok
}

// id returns the ID for handle, or an empty string for handle zero.
func (t *projectionIDTable) id(handle uint32) string {
	if handle == 0 {
		return ""
	}
	return t.ids[handle-1]
}

// len returns the number of interned IDs.
func (t *projectionIDTable) len() int {
	return len(t.ids)
}

// estimatedBytes approximates the retained size of the table.
func (t *projectionIDTable) estimatedBytes() int64 {
	var bytes int64
	for _, id := range t.ids {
		// One map entry and one slice element share the same string data.
		bytes += projectionMapEntryOverhead + 4 + projectionSliceEntryOverhead + int64(len(id))
	}
	return bytes
}

// handleIDResolver resolves a handle from projectionIDTable or eventIDTable.
type handleIDResolver interface {
	id(handle uint32) string
}

// sortedHandleKeys returns the handle keys of values ordered by their IDs, so
// snapshots stay deterministic regardless of interning order.
func sortedHandleKeys[V any](ids handleIDResolver, values map[uint32]V) []uint32 {
	handles := make([]uint32, 0, len(values))
	for handle := range values {
		handles = append(handles, handle)
	}
	slices.SortFunc(handles, func(a, b uint32) int { return cmp.Compare(ids.id(a), ids.id(b)) })
	return handles
}

// handleSlice stores one value per ID-table handle in a dense slice indexed by
// handle minus one. The zero value of T means "no value", so handles of IDs
// without a value cost only one zero element.
type handleSlice[T comparable] []T

// get returns the value for handle and whether it is set.
func (s handleSlice[T]) get(handle uint32) (T, bool) {
	var zero T
	if handle == 0 || int(handle) > len(s) {
		return zero, false
	}
	value := s[handle-1]
	return value, value != zero
}

// set stores value for handle and grows the slice when necessary. Handle zero
// is ignored.
func (s *handleSlice[T]) set(handle uint32, value T) {
	if handle == 0 {
		return
	}
	if missing := int(handle) - len(*s); missing > 0 {
		*s = append(*s, make([]T, missing)...)
	}
	(*s)[handle-1] = value
}
