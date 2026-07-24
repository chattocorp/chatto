// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

// projectionStringTable interns repeated projection identifiers and gives
// dense projection-local handles to callers. Handle zero is reserved for the
// empty or absent value, which keeps optional relationships allocation-free.
//
// Handles are process-local indexes. Snapshot codecs must persist the stable
// string value and rebuild handles during restore.
type projectionStringTable struct {
	byValue map[string]uint32
	values  []string
}

func newProjectionStringTable() projectionStringTable {
	return projectionStringTable{
		byValue: make(map[string]uint32),
		values:  []string{""},
	}
}

func (t *projectionStringTable) intern(value string) uint32 {
	if value == "" {
		return 0
	}
	if ref, ok := t.byValue[value]; ok {
		return ref
	}
	ref := uint32(len(t.values))
	if int(ref) != len(t.values) {
		panic("projection identifier table exhausted uint32 handles")
	}
	t.values = append(t.values, value)
	// Use the canonical retained string as the map key. A repeated identifier
	// decoded from a later protobuf can then be released after this lookup.
	t.byValue[t.values[ref]] = ref
	return ref
}

func (t *projectionStringTable) lookup(value string) (uint32, bool) {
	if value == "" {
		return 0, false
	}
	ref, ok := t.byValue[value]
	return ref, ok
}

func (t *projectionStringTable) value(ref uint32) string {
	if ref == 0 || int(ref) >= len(t.values) {
		return ""
	}
	return t.values[ref]
}

func growProjectionSlice[T any](values []T, ref uint32) []T {
	if int(ref) < len(values) {
		return values
	}
	return append(values, make([]T, int(ref)-len(values)+1)...)
}
