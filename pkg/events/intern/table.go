package intern

// Table interns opaque IDs as dense, one-based uint32 handles.
// Kind distinguishes this namespace from other ID kinds at compile time.
// Handle zero means "no ID". A projection stores handles in its indexes instead
// of repeating ID strings, so each ID is held once and index entries stay
// small and pointer-free.
//
// The table is append-only: handles stay valid for the table's lifetime and
// are never reused. Handles are process-local and must not be persisted;
// snapshots store the ID strings. The caller must prevent concurrent reads and
// writes. The zero value is ready for use. A copy shares storage with the
// original. After a copy, use only one of the two values.
type Table[Kind any] struct {
	handles map[string]ID[Kind]
	ids     []string
}

// Intern returns the handle for id and adds id when it is new. An empty id
// returns handle zero.
// It panics if a new ID would exceed 2^32-1 distinct nonempty IDs.
func (t *Table[Kind]) Intern(id string) ID[Kind] {
	if id == "" {
		return 0
	}
	if handle, ok := t.handles[id]; ok {
		return handle
	}
	if uint64(len(t.ids)) >= uint64(^uint32(0)) {
		panic("intern: ID table handle space exhausted")
	}
	if t.handles == nil {
		t.handles = make(map[string]ID[Kind])
	}
	t.ids = append(t.ids, id)
	handle := ID[Kind](len(t.ids))
	t.handles[id] = handle
	return handle
}

// Lookup returns the handle for an interned id without adding it. Read paths
// use Lookup so a query for an unknown ID does not grow the table.
func (t *Table[Kind]) Lookup(id string) (ID[Kind], bool) {
	if id == "" {
		return 0, false
	}
	handle, ok := t.handles[id]
	return handle, ok
}

// Resolve returns the ID for handle, or an empty string for handle zero.
// A nonzero handle must come from this table.
func (t *Table[Kind]) Resolve(handle ID[Kind]) string {
	if handle == 0 {
		return ""
	}
	return t.ids[handle-1]
}

// Len returns the number of interned IDs.
func (t *Table[Kind]) Len() int {
	return len(t.ids)
}

// EstimatedBytes approximates retained memory for diagnostics.
// It is not an allocator measurement.
func (t *Table[Kind]) EstimatedBytes() int64 {
	var bytes int64
	for _, id := range t.ids {
		// One map entry and one slice element share the same string data.
		bytes += mapEntryOverhead + 4 + sliceEntryOverhead + int64(len(id))
	}
	return bytes
}
